package di

import "github.com/scality/file-reflector/pkg/presentation/watcher"

// GetWatcher returns the singleton long-running Watcher.
func (c *Container) GetWatcher() (*watcher.Watcher, error) {
	if c.watcher == nil {
		initial, err := c.getInitialSyncUsecase()
		if err != nil {
			return nil, err
		}

		syncPath, err := c.getSyncPathUsecase()
		if err != nil {
			return nil, err
		}

		c.watcher = watcher.NewWatcher(
			c.getLogger(),
			c.getEventSource(),
			initial,
			syncPath,
		)
	}

	return c.watcher, nil
}
