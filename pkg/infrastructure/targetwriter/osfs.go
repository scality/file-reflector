package targetwriter

import (
	"context"
	"io"
	"io/fs"

	"github.com/scality/file-reflector/pkg/domain"
	"github.com/scality/file-reflector/pkg/service"
)

// OSFS implements service.TargetWriter against the local OS filesystem.
// All paths are interpreted relative to the configured root.
type OSFS struct {
	root string
}

// Compile-time interface assertion.
var _ service.TargetWriter = (*OSFS)(nil)

// NewOSFS returns an OSFS target writer rooted at root.
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

func (o *OSFS) Hash(_ context.Context, _ string) ([32]byte, error) {
	return [32]byte{}, nil
}

func (o *OSFS) Mkdir(_ context.Context, _ string, _ fs.FileMode, _, _ int) error {
	return nil
}

func (o *OSFS) WriteAtomic(_ context.Context, _ string, _ io.Reader, _ fs.FileMode, _, _ int) error {
	return nil
}

func (o *OSFS) Chmod(_ context.Context, _ string, _ fs.FileMode) error {
	return nil
}

func (o *OSFS) Chown(_ context.Context, _ string, _, _ int) error {
	return nil
}

func (o *OSFS) Remove(_ context.Context, _ string) error {
	return nil
}
