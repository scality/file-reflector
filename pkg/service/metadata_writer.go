package service

import (
	"context"
	"io/fs"
)

// MetadataWriter adjusts the mode and ownership of a filesystem entry.
type MetadataWriter interface {
	Chmod(ctx context.Context, relPath string, mode fs.FileMode) error
	Chown(ctx context.Context, relPath string, uid, gid int) error
}
