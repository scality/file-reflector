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

// getSourceReader returns the singleton SourceReader rooted at --source.
func (c *Container) getSourceReader() (service.SourceReader, error) {
	if c.sourceReader == nil {
		root, err := os.OpenRoot(c.cfg.Source)
		if err != nil {
			return nil, errors.Wrap(ErrOpenSourceRoot,
				errors.WithProperty("path", c.cfg.Source),
				errors.CausedBy(err),
			)
		}

		c.sourceReader = &sourceReader{
			MetadataReader: metadatareader.NewOSFS(root),
			ContentReader:  contentreader.NewOSFS(root),
			ContentHasher:  contenthasher.NewOSFS(root),
		}
	}

	return c.sourceReader, nil
}
