package contentreader

import (
	"context"
	"io"
	"os"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/service"
)

// ErrContentRead is the sentinel returned for any failure opening
// filesystem entry content for reading.
var ErrContentRead = errors.New("content read")

// OSFS implements service.ContentReader against the local OS
// filesystem, rooted at a single directory.
type OSFS struct {
	root *os.Root
}

// Compile-time interface assertion.
var _ service.ContentReader = (*OSFS)(nil)

// NewOSFS returns an OSFS content reader rooted at root.
func NewOSFS(root *os.Root) *OSFS {
	return &OSFS{root: root}
}

// Open returns a ReadCloser over the file at rel. Caller closes.
func (o *OSFS) Open(_ context.Context, rel string) (io.ReadCloser, error) {
	f, err := o.root.Open(rel)
	if err != nil {
		return nil, errors.Wrap(ErrContentRead,
			errors.WithDetail("open"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	return f, nil
}
