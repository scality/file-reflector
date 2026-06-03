package entryremover

import (
	"context"
	"os"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/service"
)

// ErrEntryRemove is the sentinel returned for any failure removing a
// filesystem entry.
var ErrEntryRemove = errors.New("entry remove")

// OSFS implements service.EntryRemover against the local OS
// filesystem, rooted at a single directory.
type OSFS struct {
	root *os.Root
}

// Compile-time interface assertion.
var _ service.EntryRemover = (*OSFS)(nil)

// NewOSFS returns an OSFS entry remover rooted at root.
func NewOSFS(root *os.Root) *OSFS {
	return &OSFS{root: root}
}

// Remove deletes the entry at rel. Recursive for directories,
// idempotent on missing paths.
func (o *OSFS) Remove(_ context.Context, rel string) error {
	if err := o.root.RemoveAll(rel); err != nil {
		return errors.Wrap(ErrEntryRemove,
			errors.WithDetail("remove"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	return nil
}
