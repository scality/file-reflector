package watcher

import (
	"context"
	"log/slog"

	"github.com/scality/file-reflector/pkg/service"
	"github.com/scality/file-reflector/pkg/usecase"
)

// Watcher is the long-running orchestrator: it runs the initial sync,
// then drains events from the EventSource, delegating every event to
// the SyncPath usecase. A single bad path is logged and skipped; one
// failure does not bring down the agent.
type Watcher struct {
	logger             *slog.Logger
	eventSource        service.EventSource
	initialSyncUsecase *usecase.InitialSync
	syncPathUsecase    *usecase.SyncPath
}

// NewWatcher constructs a Watcher with its dependencies.
func NewWatcher(
	logger *slog.Logger,
	eventSource service.EventSource,
	initialSyncUsecase *usecase.InitialSync,
	syncPathUsecase *usecase.SyncPath,
) *Watcher {
	return &Watcher{
		logger:             logger.With(slog.String("component_name", "watcher")),
		eventSource:        eventSource,
		initialSyncUsecase: initialSyncUsecase,
		syncPathUsecase:    syncPathUsecase,
	}
}

// Run runs the initial reconciliation, then drains the event channel
// until ctx is cancelled or the source closes its channel.
//
// TODO: real implementation. The skeleton returns nil immediately.
func (w *Watcher) Run(_ context.Context) error {
	return nil
}
