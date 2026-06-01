package service

import (
	"context"
	"io"
	"io/fs"
)

// ContentWriter creates directories and writes file content on a filesystem
// tree.
type ContentWriter interface {
	Mkdir(ctx context.Context, relPath string, mode fs.FileMode, uid, gid int) error
	WriteAtomic(ctx context.Context, relPath string, src io.Reader, mode fs.FileMode, uid, gid int) error
}
