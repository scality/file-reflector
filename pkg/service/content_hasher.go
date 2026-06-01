package service

import "context"

// ContentHasher computes the content hash of a filesystem entry.
type ContentHasher interface {
	Hash(ctx context.Context, relPath string) ([32]byte, error)
}
