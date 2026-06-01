package metadatawriter

import (
	"context"
	"io/fs"
	"os"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/service"
)

// ErrMetadataWrite is the sentinel returned for any failure changing
// the mode or ownership of a filesystem entry.
var ErrMetadataWrite = errors.New("metadata write")

// OSFS implements service.MetadataWriter against the local OS
// filesystem, rooted at a single directory.
type OSFS struct {
	root *os.Root
}

// Compile-time interface assertion.
var _ service.MetadataWriter = (*OSFS)(nil)

// NewOSFS returns an OSFS metadata writer rooted at root.
func NewOSFS(root *os.Root) *OSFS {
	return &OSFS{root: root}
}

// Chmod sets the mode on rel.
func (o *OSFS) Chmod(_ context.Context, rel string, mode fs.FileMode) error {
	if err := o.root.Chmod(rel, mode); err != nil {
		return errors.Wrap(ErrMetadataWrite,
			errors.WithDetail("chmod"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	return nil
}

// Chown sets the owner on rel.
func (o *OSFS) Chown(_ context.Context, rel string, uid, gid int) error {
	if err := o.root.Chown(rel, uid, gid); err != nil {
		return errors.Wrap(ErrMetadataWrite,
			errors.WithDetail("chown"),
			errors.WithProperty("path", rel),
			errors.WithProperty("uid", uid),
			errors.WithProperty("gid", gid),
			errors.CausedBy(err),
		)
	}

	return nil
}
