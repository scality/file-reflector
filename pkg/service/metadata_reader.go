package service

import (
	"context"

	"github.com/scality/file-reflector/pkg/domain"
)

// MetadataReader reads the metadata of a filesystem entry without touching
// its content.
type MetadataReader interface {
	Stat(ctx context.Context, relPath string) (domain.FileNode, error)
	ReadDir(ctx context.Context, relPath string) ([]string, error)
}
