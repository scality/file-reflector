package service

import (
	"context"
	"io"
)

// ContentReader opens a filesystem entry for reading its content.
type ContentReader interface {
	Open(ctx context.Context, relPath string) (io.ReadCloser, error)
}
