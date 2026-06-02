package di

import (
	"os"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/infrastructure/contenthasher"
	"github.com/scality/file-reflector/pkg/infrastructure/contentwriter"
	"github.com/scality/file-reflector/pkg/infrastructure/entryremover"
	"github.com/scality/file-reflector/pkg/infrastructure/metadatareader"
	"github.com/scality/file-reflector/pkg/infrastructure/metadatawriter"
	"github.com/scality/file-reflector/pkg/service"
)

// ErrOpenTargetRoot is the sentinel returned when the container cannot
// open the target root passed on the command line.
var ErrOpenTargetRoot = errors.New("open target root")

// targetWriter composes the five role-based ports a TargetWriter
// covers. The OSFS adapters share the same *os.Root so a single file
// descriptor on the target directory backs them all.
type targetWriter struct {
	service.MetadataReader
	service.ContentHasher
	service.ContentWriter
	service.MetadataWriter
	service.EntryRemover
}

// getTargetRoot returns the singleton *os.Root scoped to --target. It
// is shared between the target-side adapters and the target-side event
// source so they back onto the same file descriptor.
func (c *Container) getTargetRoot() (*os.Root, error) {
	if c.targetRoot == nil {
		root, err := os.OpenRoot(c.cfg.Target)
		if err != nil {
			return nil, errors.Wrap(ErrOpenTargetRoot,
				errors.WithProperty("path", c.cfg.Target),
				errors.CausedBy(err),
			)
		}

		c.targetRoot = root
	}

	return c.targetRoot, nil
}

// getTargetWriter returns the singleton TargetWriter rooted at --target.
func (c *Container) getTargetWriter() (service.TargetWriter, error) {
	if c.targetWriter == nil {
		root, err := c.getTargetRoot()
		if err != nil {
			return nil, err
		}

		mw := metadatawriter.NewOSFS(root)
		c.targetWriter = &targetWriter{
			MetadataReader: metadatareader.NewOSFS(root),
			ContentHasher:  contenthasher.NewOSFS(root),
			ContentWriter:  contentwriter.NewOSFS(root, mw),
			MetadataWriter: mw,
			EntryRemover:   entryremover.NewOSFS(root),
		}
	}

	return c.targetWriter, nil
}
