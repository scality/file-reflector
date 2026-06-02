package di

import (
	"log/slog"
	"os"

	"github.com/scality/file-reflector/cmd/config"
	"github.com/scality/file-reflector/pkg/presentation/watcher"
	"github.com/scality/file-reflector/pkg/service"
	"github.com/scality/file-reflector/pkg/usecase"
)

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
