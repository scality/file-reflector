package di

import (
	"github.com/scality/file-reflector/pkg/infrastructure/eventsource"
	"github.com/scality/file-reflector/pkg/service"
)

// getEventSource returns the singleton EventSource watching the source
// tree for changes.
func (c *Container) getEventSource() service.EventSource {
	if c.eventSource == nil {
		c.eventSource = eventsource.NewFsnotify(c.cfg.Source, c.getLogger())
	}

	return c.eventSource
}
