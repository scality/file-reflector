package di

import (
	"log/slog"

	"github.com/scality/file-reflector/pkg/infrastructure/eventsource"
	"github.com/scality/file-reflector/pkg/service"
)

// getEventSources returns one EventSource per watched tree: the source
// root and the target root. The watcher merges them so the agent
// reconciles whichever side changed.
func (c *Container) getEventSources() ([]service.EventSource, error) {
	if c.eventSources == nil {
		sourceRoot, err := c.getSourceRoot()
		if err != nil {
			return nil, err
		}

		targetRoot, err := c.getTargetRoot()
		if err != nil {
			return nil, err
		}

		logger := c.GetLogger()
		c.eventSources = []service.EventSource{
			eventsource.NewFsnotify(sourceRoot, logger.With(slog.String("watcher_side", "source"))),
			eventsource.NewFsnotify(targetRoot, logger.With(slog.String("watcher_side", "target"))),
		}
	}

	return c.eventSources, nil
}
