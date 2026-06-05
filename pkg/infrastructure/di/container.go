package di

import (
	stderrors "errors"
	"log/slog"
	"os"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/cmd/config"
	"github.com/scality/file-reflector/pkg/presentation/watcher"
	"github.com/scality/file-reflector/pkg/service"
	"github.com/scality/file-reflector/pkg/usecase"
)

// ErrCloseContainer is the sentinel returned when releasing the
// container's resources fails.
var ErrCloseContainer = errors.New("close container")

// Container is the composition root's dependency container. Each Get*
// method lazily constructs its target the first time it is called and
// caches the result for subsequent calls.
type Container struct {
	cfg *config.Config

	logger *slog.Logger

	sourceRoot *os.Root
	targetRoot *os.Root

	matcher      service.Matcher
	sourceReader service.SourceReader
	targetWriter service.TargetWriter
	eventSources []service.EventSource

	syncPathUsecase    *usecase.SyncPath
	initialSyncUsecase *usecase.InitialSync

	watcher *watcher.Watcher
}

// NewContainer returns a Container ready to wire the application's
// dependencies from cfg. No work is performed until the first Get*
// call.
func NewContainer(cfg *config.Config) *Container {
	return &Container{cfg: cfg}
}

// Close releases the operating-system resources the container opened
// (the source and target root descriptors). It is safe to call when
// nothing was opened and is idempotent.
func (c *Container) Close() error {
	var errs []error

	if c.sourceRoot != nil {
		if err := c.sourceRoot.Close(); err != nil {
			errs = append(errs, errors.Wrap(ErrCloseContainer,
				errors.WithDetail("close source root"),
				errors.CausedBy(err),
			))
		}

		c.sourceRoot = nil
	}

	if c.targetRoot != nil {
		if err := c.targetRoot.Close(); err != nil {
			errs = append(errs, errors.Wrap(ErrCloseContainer,
				errors.WithDetail("close target root"),
				errors.CausedBy(err),
			))
		}

		c.targetRoot = nil
	}

	return stderrors.Join(errs...)
}
