package service

import (
	"context"

	"github.com/scality/file-reflector/pkg/domain"
)

// MetadataReader reads the metadata of a filesystem entry without touching
// its content.
type MetadataReader interface {
	// Stat returns a FileNode describing relPath. A missing entry yields
	// FileNode{Kind: NodeAbsent} with a nil error — implementations must
	// not treat "no such file" as a failure, so callers can branch on
	// Kind rather than error-sniffing.
	Stat(ctx context.Context, relPath string) (domain.FileNode, error)
	ReadDir(ctx context.Context, relPath string) ([]string, error)
}
