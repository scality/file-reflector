package service

import "context"

// EntryRemover deletes a filesystem entry.
type EntryRemover interface {
	// Remove deletes the entry at relPath. It handles both regular files and
	// directories (the directory case is recursive).
	Remove(ctx context.Context, relPath string) error
}
