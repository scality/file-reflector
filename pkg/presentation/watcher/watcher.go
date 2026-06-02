package watcher

import (
	"context"
	"log/slog"

	"github.com/scality/file-reflector/pkg/service"
	"github.com/scality/file-reflector/pkg/usecase"
)

// Watcher is the long-running orchestrator: it runs the initial sync,
// then drains events from every EventSource and dispatches each to the
// SyncPath usecase. A single bad path is logged and skipped; one
// failure does not bring down the agent.
type Watcher struct {
	logger             *slog.Logger
	eventSources       []service.EventSource
	initialSyncUsecase *usecase.InitialSync
	syncPathUsecase    *usecase.SyncPath
}

// NewWatcher constructs a Watcher with its dependencies. The variadic
// eventSources lets the caller wire one source per watched tree (e.g.
// one for --source and one for --target); the watcher merges them
// internally.
func NewWatcher(
	logger *slog.Logger,
	initialSyncUsecase *usecase.InitialSync,
	syncPathUsecase *usecase.SyncPath,
	eventSources ...service.EventSource,
) *Watcher {
	return &Watcher{
		logger:             logger.With(slog.String("component_name", "watcher")),
		eventSources:       eventSources,
		initialSyncUsecase: initialSyncUsecase,
		syncPathUsecase:    syncPathUsecase,
	}
}

// Run runs the initial reconciliation, then drains the event channels
// until ctx is cancelled or every source closes its channel.
//
// TODO: real implementation. The skeleton returns nil immediately.
func (w *Watcher) Run(_ context.Context) error {
	return nil
}
