package di

import "github.com/scality/file-reflector/pkg/usecase"

// getSyncPathUsecase returns the singleton SyncPath usecase.
func (c *Container) getSyncPathUsecase() (*usecase.SyncPath, error) {
	if c.syncPathUsecase == nil {
		m, err := c.getMatcher()
		if err != nil {
			return nil, err
		}

		sr, err := c.getSourceReader()
		if err != nil {
			return nil, err
		}

		tw, err := c.getTargetWriter()
		if err != nil {
			return nil, err
		}

		c.syncPathUsecase = usecase.NewSyncPath(c.getLogger(), sr, tw, m)
	}

	return c.syncPathUsecase, nil
}

// getInitialSyncUsecase returns the singleton InitialSync usecase.
func (c *Container) getInitialSyncUsecase() (*usecase.InitialSync, error) {
	if c.initialSyncUsecase == nil {
		sp, err := c.getSyncPathUsecase()
		if err != nil {
			return nil, err
		}

		m, err := c.getMatcher()
		if err != nil {
			return nil, err
		}

		sr, err := c.getSourceReader()
		if err != nil {
			return nil, err
		}

		tw, err := c.getTargetWriter()
		if err != nil {
			return nil, err
		}

		c.initialSyncUsecase = usecase.NewInitialSync(c.getLogger(), sr, tw, m, sp)
	}

	return c.initialSyncUsecase, nil
}
