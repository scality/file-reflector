package metadatareader

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"syscall"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/domain"
	"github.com/scality/file-reflector/pkg/service"
)

// ErrMetadataRead is the sentinel returned for any failure reading
// filesystem entry metadata.
var ErrMetadataRead = errors.New("metadata read")

// OSFS implements service.MetadataReader against the local OS
// filesystem, rooted at a single directory.
type OSFS struct {
	root           *os.Root
	followSymlinks bool
	logger         *slog.Logger
}

// Compile-time interface assertion.
var _ service.MetadataReader = (*OSFS)(nil)

// NewOSFS returns an OSFS metadata reader rooted at root. Symbolic
// links are reported as such (NodeSymlink), without being followed.
func NewOSFS(root *os.Root) *OSFS {
	return &OSFS{root: root}
}

// NewFollowingOSFS returns an OSFS metadata reader rooted at root that
// resolves symbolic links to regular files: such a symlink is reported
// as its target (mode, owner, size), so Stat never yields NodeSymlink.
// Resolution is confined to the root by os.Root; a symlink that cannot
// be resolved — broken, looping, or escaping the root — or that
// resolves to a directory is reported as absent, with a warning, so the
// caller treats it like a missing entry instead of failing the walk.
// Directory targets are excluded because reporting them as directories
// would make tree walks recurse through the link: a link to an ancestor
// then mirrors the tree into itself unboundedly, and content changes
// behind the link are never re-reconciled (events name the real paths,
// and reconciling the link path alone does not recurse).
func NewFollowingOSFS(root *os.Root, logger *slog.Logger) *OSFS {
	return &OSFS{
		root:           root,
		followSymlinks: true,
		logger:         logger.With(slog.String("component_name", "metadata_reader")),
	}
}

// Stat returns a FileNode describing rel. Missing entries yield
// FileNode{Kind: NodeAbsent} with a nil error — the conventional
// "no such file" shape consumers expect.
func (o *OSFS) Stat(ctx context.Context, rel string) (domain.FileNode, error) {
	info, err := o.root.Lstat(osPath(rel))
	if err != nil {
		if os.IsNotExist(err) {
			return domain.FileNode{Kind: domain.NodeAbsent}, nil
		}

		return domain.FileNode{}, errors.Wrap(ErrMetadataRead,
			errors.WithDetail("lstat"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	if info.Mode()&fs.ModeSymlink != 0 && o.followSymlinks {
		resolved, rerr := o.root.Stat(osPath(rel))
		if rerr != nil {
			// An access or I/O error means we could not determine the
			// target, not that there is nothing to mirror — propagate it
			// so the caller requeues rather than removing the target.
			if isAccessError(rerr) {
				return domain.FileNode{}, errors.Wrap(ErrMetadataRead,
					errors.WithDetail("resolve symlink target"),
					errors.WithProperty("path", rel),
					errors.CausedBy(rerr),
				)
			}

			// Broken, looping, or escaping links have no syncable target;
			// report them absent so any stale target entry is removed.
			// Logged at debug because the poller re-stats every link each
			// sweep — a steady-state bad link must not flood the logs.
			o.logger.DebugContext(ctx, "unresolvable symlink; treating as absent",
				slog.String("path", rel),
				slog.Any("error", rerr),
			)

			return domain.FileNode{Kind: domain.NodeAbsent}, nil
		}

		if resolved.IsDir() {
			o.logger.DebugContext(ctx, "symlink to a directory; treating as absent",
				slog.String("path", rel),
			)

			return domain.FileNode{Kind: domain.NodeAbsent}, nil
		}

		info = resolved
	}

	node := domain.FileNode{
		Mode: info.Mode().Perm(),
		Size: info.Size(),
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		node.UID = int(stat.Uid)
		node.GID = int(stat.Gid)
	}

	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		node.Kind = domain.NodeSymlink
	case info.IsDir():
		node.Kind = domain.NodeDir
	default:
		node.Kind = domain.NodeFile
	}

	return node, nil
}

// ReadDir lists the immediate children of rel.
func (o *OSFS) ReadDir(_ context.Context, rel string) ([]string, error) {
	entries, err := fs.ReadDir(o.root.FS(), osPath(rel))
	if err != nil {
		return nil, errors.Wrap(ErrMetadataRead,
			errors.WithDetail("readdir"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	return names, nil
}

// isAccessError reports whether err is a permission or I/O failure —
// the agent could not read the entry, as opposed to the entry being
// absent, broken, or escaping the root. These must surface rather than
// be silently coerced to "absent", which would delete the target mirror.
func isAccessError(err error) bool {
	return errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.EIO)
}

// osPath maps the port's "" (root) to "." which is what the os.Root and
// fs.FS APIs expect when addressing the root itself.
func osPath(rel string) string {
	if rel == "" {
		return "."
	}

	return rel
}
