package di

import (
	"os"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/infrastructure/contenthasher"
	"github.com/scality/file-reflector/pkg/infrastructure/contentreader"
	"github.com/scality/file-reflector/pkg/infrastructure/metadatareader"
	"github.com/scality/file-reflector/pkg/service"
)

// ErrOpenSourceRoot is the sentinel returned when the container cannot
// open the source root passed on the command line.
var ErrOpenSourceRoot = errors.New("open source root")

// sourceReader composes the three role-based ports a SourceReader
// covers. The OSFS adapters share the same *os.Root so a single file
// descriptor on the source directory backs them all.
type sourceReader struct {
	service.MetadataReader
	service.ContentReader
	service.ContentHasher
}

// getSourceRoot returns the singleton *os.Root scoped to --source. It
// is shared between the source-side adapters and the source-side event
// source so they back onto the same file descriptor.
func (c *Container) getSourceRoot() (*os.Root, error) {
	if c.sourceRoot == nil {
		root, err := os.OpenRoot(c.cfg.Source)
		if err != nil {
			return nil, errors.Wrap(ErrOpenSourceRoot,
				errors.WithProperty("path", c.cfg.Source),
				errors.CausedBy(err),
			)
		}

		c.sourceRoot = root
	}

	return c.sourceRoot, nil
}

// getSourceReader returns the singleton SourceReader rooted at --source.
func (c *Container) getSourceReader() (service.SourceReader, error) {
	if c.sourceReader == nil {
		root, err := c.getSourceRoot()
		if err != nil {
			return nil, err
		}

		// The source side resolves symlinks: a symlink is mirrored as its
		// target's content. The target side (see getTargetWriter) keeps
		// lstat semantics so symlinks found in the target are replaced.
		c.sourceReader = &sourceReader{
			MetadataReader: metadatareader.NewFollowingOSFS(root, c.GetLogger()),
			ContentReader:  contentreader.NewOSFS(root),
			ContentHasher:  contenthasher.NewOSFS(root),
		}
	}

	return c.sourceReader, nil
}
