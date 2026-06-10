package di

import (
	"log/slog"

	"github.com/scality/file-reflector/pkg/infrastructure/eventsource"
	"github.com/scality/file-reflector/pkg/service"
)

// getEventSources returns one EventSource per watched tree — the source
// root and the target root — plus, unless disabled, the symlink poller
// on the source root, which covers the changes happening behind source
// symlinks that fsnotify structurally cannot report. The watcher merges
// them so the agent reconciles whichever side changed.
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
		sources := []service.EventSource{
			eventsource.NewFsnotify(sourceRoot, logger.With(slog.String("watcher_side", "source"))),
			eventsource.NewFsnotify(targetRoot, logger.With(slog.String("watcher_side", "target"))),
		}

		if c.cfg.SymlinkPollInterval > 0 {
			m, err := c.getMatcher()
			if err != nil {
				return nil, err
			}

			sources = append(sources,
				eventsource.NewSymlinkPoller(sourceRoot, c.cfg.SymlinkPollInterval, m,
					logger.With(slog.String("watcher_side", "source"))),
			)
		}

		// Assigned only once everything succeeded, so a mid-init error
		// never leaves a partial set cached (dropping the poller).
		c.eventSources = sources
	}

	return c.eventSources, nil
}
