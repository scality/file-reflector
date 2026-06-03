package contentwriter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/service"
)

// ErrContentWrite is the sentinel returned for any failure creating
// directories or writing file content on the filesystem.
var ErrContentWrite = errors.New("content write")

// OSFS implements service.ContentWriter against the local OS
// filesystem, rooted at a single directory. It delegates mode and
// ownership adjustments to a MetadataWriter so the chmod/chown logic
// lives in a single place.
type OSFS struct {
	root           *os.Root
	metadataWriter service.MetadataWriter
}

// Compile-time interface assertion.
var _ service.ContentWriter = (*OSFS)(nil)

// NewOSFS returns an OSFS content writer rooted at root, delegating
// mode and ownership writes to metadataWriter (which must operate on
// the same root for the paths to resolve correctly).
func NewOSFS(root *os.Root, metadataWriter service.MetadataWriter) *OSFS {
	return &OSFS{root: root, metadataWriter: metadataWriter}
}

// Mkdir creates the directory at rel and every missing ancestor,
// applying mode + ownership to each component it creates. Components
// that already exist are left alone (their attributes are the
// orchestrator's responsibility via the metadata writer).
//
// Per-component creation lets us chmod+chown each new directory.
// MkdirAll would only let us fix the leaf, leaving any intermediate
// parents with umask-stripped mode and the process's uid/gid.
func (o *OSFS) Mkdir(ctx context.Context, rel string, mode fs.FileMode, uid, gid int) error {
	parts := strings.Split(rel, "/")
	for i := 1; i <= len(parts); i++ {
		partial := strings.Join(parts[:i], "/")

		info, statErr := o.root.Stat(partial)
		if statErr == nil {
			if !info.IsDir() {
				return errors.Wrap(ErrContentWrite,
					errors.WithDetail("ancestor exists and is not a directory"),
					errors.WithProperty("path", partial),
				)
			}

			continue
		}

		if !os.IsNotExist(statErr) {
			return errors.Wrap(ErrContentWrite,
				errors.WithDetail("stat ancestor"),
				errors.WithProperty("path", partial),
				errors.CausedBy(statErr),
			)
		}

		if err := o.root.Mkdir(partial, mode); err != nil {
			return errors.Wrap(ErrContentWrite,
				errors.WithDetail("mkdir"),
				errors.WithProperty("path", partial),
				errors.CausedBy(err),
			)
		}

		if err := o.metadataWriter.Chmod(ctx, partial, mode); err != nil {
			return errors.Wrap(ErrContentWrite,
				errors.WithDetail("force mode on new dir"),
				errors.WithProperty("path", partial),
				errors.CausedBy(err),
			)
		}

		if err := o.metadataWriter.Chown(ctx, partial, uid, gid); err != nil {
			return errors.Wrap(ErrContentWrite,
				errors.WithDetail("set ownership on new dir"),
				errors.WithProperty("path", partial),
				errors.CausedBy(err),
			)
		}
	}

	return nil
}

// WriteAtomic writes src into rel via a sibling temp file followed by
// rename, so a concurrent reader never sees a torn or partial file.
// The parent directory must already exist; Mkdir is the caller's
// responsibility. Any failure cleans up the temp.
func (o *OSFS) WriteAtomic(ctx context.Context, rel string, src io.Reader, mode fs.FileMode, uid, gid int) error {
	suffix, err := randomSuffix()
	if err != nil {
		return errors.Wrap(ErrContentWrite,
			errors.WithDetail("generate temp suffix"),
			errors.CausedBy(err),
		)
	}

	tmp := rel + ".file-reflector-tmp-" + suffix

	f, err := o.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return errors.Wrap(ErrContentWrite,
			errors.WithDetail("create temp file"),
			errors.WithProperty("path", tmp),
			errors.CausedBy(err),
		)
	}

	// Any return without committing = true triggers a temp-file cleanup.
	committed := false

	defer func() {
		if !committed {
			_ = o.root.Remove(tmp)
		}
	}()

	if _, err := io.Copy(f, src); err != nil {
		_ = f.Close()

		return errors.Wrap(ErrContentWrite,
			errors.WithDetail("copy into temp file"),
			errors.WithProperty("path", tmp),
			errors.CausedBy(err),
		)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()

		return errors.Wrap(ErrContentWrite,
			errors.WithDetail("fsync temp file"),
			errors.CausedBy(err),
		)
	}

	if err := f.Close(); err != nil {
		return errors.Wrap(ErrContentWrite,
			errors.WithDetail("close temp file"),
			errors.CausedBy(err),
		)
	}

	if err := o.metadataWriter.Chmod(ctx, tmp, mode); err != nil {
		return errors.Wrap(ErrContentWrite,
			errors.WithDetail("force mode on temp file"),
			errors.WithProperty("path", tmp),
			errors.CausedBy(err),
		)
	}

	if err := o.metadataWriter.Chown(ctx, tmp, uid, gid); err != nil {
		return errors.Wrap(ErrContentWrite,
			errors.WithDetail("set ownership on temp file"),
			errors.WithProperty("path", tmp),
			errors.CausedBy(err),
		)
	}

	if err := o.root.Rename(tmp, rel); err != nil {
		return errors.Wrap(ErrContentWrite,
			errors.WithDetail("rename temp into place"),
			errors.WithProperty("from", tmp),
			errors.WithProperty("to", rel),
			errors.CausedBy(err),
		)
	}

	committed = true

	return nil
}

func randomSuffix() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err //nolint:wrapcheck // private helper; caller wraps with the sentinel
	}

	return hex.EncodeToString(b[:]), nil
}
