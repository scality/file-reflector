package di

import (
	"github.com/scality/file-reflector/pkg/infrastructure/targetwriter"
	"github.com/scality/file-reflector/pkg/service"
)

// getTargetWriter returns the singleton TargetWriter rooted at --target.
func (c *Container) getTargetWriter() service.TargetWriter {
	if c.targetWriter == nil {
		c.targetWriter = targetwriter.NewOSFS(c.cfg.Target)
	}

	return c.targetWriter
}
