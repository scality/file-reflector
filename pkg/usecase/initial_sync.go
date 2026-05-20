package usecase

import (
	"context"
	"log/slog"

	"github.com/scality/file-reflector/pkg/service"
)

// InitialSync walks the source tree top-down and the target tree
// bottom-up at startup, delegating every visited path to SyncPath.
type InitialSync struct {
	logger       *slog.Logger
	sourceReader service.SourceReader
	targetWriter service.TargetWriter
	matcher      service.Matcher
	syncPath     *SyncPath
}

// NewInitialSync constructs an InitialSync with its dependencies.
func NewInitialSync(
	logger *slog.Logger,
	sourceReader service.SourceReader,
	targetWriter service.TargetWriter,
	matcher service.Matcher,
	syncPath *SyncPath,
) *InitialSync {
	return &InitialSync{
		logger:       logger.With(slog.String("usecase_name", "initial_sync")),
		sourceReader: sourceReader,
		targetWriter: targetWriter,
		matcher:      matcher,
		syncPath:     syncPath,
	}
}

// Execute performs the initial reconciliation: a top-down walk of source
// applying SyncPath at each visited path, then a bottom-up walk of target
// applying SyncPath to every path not present in source (which results in
// deletion).
//
// TODO: real implementation. The skeleton returns nil.
func (uc *InitialSync) Execute(_ context.Context) error {
	return nil
}
