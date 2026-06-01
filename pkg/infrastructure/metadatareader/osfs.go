package metadatareader

import (
	"context"
	"io/fs"
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
	root *os.Root
}

// Compile-time interface assertion.
var _ service.MetadataReader = (*OSFS)(nil)

// NewOSFS returns an OSFS metadata reader rooted at root.
func NewOSFS(root *os.Root) *OSFS {
	return &OSFS{root: root}
}

// Stat returns a FileNode describing rel. Missing entries yield
// FileNode{Kind: NodeAbsent} with a nil error — the conventional
// "no such file" shape consumers expect.
func (o *OSFS) Stat(_ context.Context, rel string) (domain.FileNode, error) {
	info, err := o.root.Lstat(rel)
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
	entries, err := fs.ReadDir(o.root.FS(), readDirName(rel))
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

// readDirName maps the port's "" (root) to "." which is what fs.ReadDir
// expects for the root of an fs.FS.
func readDirName(rel string) string {
	if rel == "" {
		return "."
	}

	return rel
}
