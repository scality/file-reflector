package di

import (
	"github.com/scality/file-reflector/pkg/infrastructure/sourcereader"
	"github.com/scality/file-reflector/pkg/service"
)

// getSourceReader returns the singleton SourceReader rooted at --source.
func (c *Container) getSourceReader() service.SourceReader {
	if c.sourceReader == nil {
		c.sourceReader = sourcereader.NewOSFS(c.cfg.Source)
	}

	return c.sourceReader
}
