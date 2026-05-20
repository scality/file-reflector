package sourcereader

import (
	"context"
	"io"

	"github.com/scality/file-reflector/pkg/domain"
	"github.com/scality/file-reflector/pkg/service"
)

// OSFS implements service.SourceReader against the local OS filesystem.
// All paths passed to its methods are interpreted relative to the
// configured root.
type OSFS struct {
	root string
}

// Compile-time interface assertion.
var _ service.SourceReader = (*OSFS)(nil)

// NewOSFS returns an OSFS source reader rooted at root.
func NewOSFS(root string) *OSFS {
	return &OSFS{root: root}
}

// TODO: real implementation. The skeleton methods return zero values.

func (o *OSFS) Stat(_ context.Context, _ string) (domain.FileNode, error) {
	return domain.FileNode{}, nil
}

func (o *OSFS) ReadDir(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (o *OSFS) Open(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, nil
}

func (o *OSFS) Hash(_ context.Context, _ string) ([32]byte, error) {
	return [32]byte{}, nil
}
