package usecase

import (
	"context"
	"log/slog"

	"github.com/scality/file-reflector/pkg/service"
)

// SyncPath reconciles a single relative path between source and target.
// It is the workhorse used both by InitialSync (during startup) and by
// the watcher (per fsnotify event at runtime).
type SyncPath struct {
	logger       *slog.Logger
	sourceReader service.SourceReader
	targetWriter service.TargetWriter
	matcher      service.Matcher
}

// NewSyncPath constructs a SyncPath with its dependencies.
func NewSyncPath(
	logger *slog.Logger,
	sourceReader service.SourceReader,
	targetWriter service.TargetWriter,
	matcher service.Matcher,
) *SyncPath {
	return &SyncPath{
		logger:       logger.With(slog.String("usecase_name", "sync_path")),
		sourceReader: sourceReader,
		targetWriter: targetWriter,
		matcher:      matcher,
	}
}

// Execute reconciles the path at relPath: it stats both sides and applies
// whatever sequence of TargetWriter calls is needed to make target reflect
// source. Ignored paths are no-ops.
//
// TODO: real implementation. The skeleton returns nil.
func (uc *SyncPath) Execute(_ context.Context, _ string) error {
	return nil
}
