package usecase

import (
	"context"
	"log/slog"
	"path"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/domain"
	"github.com/scality/file-reflector/pkg/service"
)

// ErrInitialSync is the sentinel returned for any failure during the
// initial reconciliation walk.
var ErrInitialSync = errors.New("initial sync")

// pathSyncer is the narrow surface InitialSync needs from SyncPath; it
// exposes only the per-path reconciliation entry point so tests can
// substitute a recorder without instantiating a full SyncPath.
type pathSyncer interface {
	Execute(ctx context.Context, relPath string) error
}

// InitialSync walks the source tree top-down and the target tree
// bottom-up at startup, delegating every visited path to a pathSyncer.
type InitialSync struct {
	logger       *slog.Logger
	sourceReader service.SourceReader
	targetWriter service.TargetWriter
	matcher      service.Matcher
	syncPath     pathSyncer
}

// NewInitialSync constructs an InitialSync with its dependencies.
func NewInitialSync(
	logger *slog.Logger,
	sourceReader service.SourceReader,
	targetWriter service.TargetWriter,
	matcher service.Matcher,
	syncPath pathSyncer,
) *InitialSync {
	return &InitialSync{
		logger:       logger.With(slog.String("usecase_name", "initial_sync")),
		sourceReader: sourceReader,
		targetWriter: targetWriter,
		matcher:      matcher,
		syncPath:     syncPath,
	}
}

// Execute performs the initial reconciliation in two phases. First it
// walks the source tree top-down and applies SyncPath at every visited
// path, recording which paths exist in source. Then it walks the target
// tree bottom-up and applies SyncPath to every target path that is not
// in source (which causes their removal). Ignored paths are skipped
// entirely at both phases.
func (uc *InitialSync) Execute(ctx context.Context) error {
	uc.logger.InfoContext(ctx, "starting initial reconciliation")

	visited := make(map[string]struct{})

	if err := uc.walkSource(ctx, "", visited); err != nil {
		return err
	}

	if err := uc.cleanupTarget(ctx, "", visited); err != nil {
		return err
	}

	uc.logger.InfoContext(ctx, "initial reconciliation complete",
		slog.Int("source_paths", len(visited)),
	)

	return nil
}

// walkSource visits rel top-down, applies SyncPath, then recurses into
// children when rel is a directory. Ignored paths are skipped entirely
// - neither reconciled nor recursed into - since SyncPath would no-op
// on them anyway.
func (uc *InitialSync) walkSource(ctx context.Context, rel string, visited map[string]struct{}) error {
	if rel != "" && uc.matcher.Matches(rel) {
		return nil
	}

	if err := uc.syncPath.Execute(ctx, rel); err != nil {
		return errors.Wrap(ErrInitialSync,
			errors.WithDetail("syncpath during source walk"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	visited[rel] = struct{}{}

	node, err := uc.sourceReader.Stat(ctx, rel)
	if err != nil {
		return errors.Wrap(ErrInitialSync,
			errors.WithDetail("stat source during walk"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	if node.Kind != domain.NodeDir {
		return nil
	}

	entries, err := uc.sourceReader.ReadDir(ctx, rel)
	if err != nil {
		return errors.Wrap(ErrInitialSync,
			errors.WithDetail("readdir source during walk"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	for _, name := range entries {
		if err := uc.walkSource(ctx, path.Join(rel, name), visited); err != nil {
			return err
		}
	}

	return nil
}

// cleanupTarget visits rel bottom-up: it recurses into children first,
// then calls SyncPath on rel itself if rel was not seen during the
// source walk. SyncPath will see source absent and remove the entry
// (recursively, via the EntryRemover's RemoveAll). The bottom-up
// ordering exists so each orphan is logged by SyncPath individually
// instead of being swallowed by the parent's recursive remove.
// Ignored paths are skipped entirely along with their subtree.
func (uc *InitialSync) cleanupTarget(ctx context.Context, rel string, visited map[string]struct{}) error {
	if rel != "" && uc.matcher.Matches(rel) {
		return nil
	}

	node, err := uc.targetWriter.Stat(ctx, rel)
	if err != nil {
		return errors.Wrap(ErrInitialSync,
			errors.WithDetail("stat target during cleanup"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	if node.Kind == domain.NodeAbsent {
		return nil
	}

	if node.Kind == domain.NodeDir {
		entries, err := uc.targetWriter.ReadDir(ctx, rel)
		if err != nil {
			return errors.Wrap(ErrInitialSync,
				errors.WithDetail("readdir target during cleanup"),
				errors.WithProperty("path", rel),
				errors.CausedBy(err),
			)
		}

		for _, name := range entries {
			if err := uc.cleanupTarget(ctx, path.Join(rel, name), visited); err != nil {
				return err
			}
		}
	}

	if _, inSource := visited[rel]; inSource {
		return nil
	}

	if err := uc.syncPath.Execute(ctx, rel); err != nil {
		return errors.Wrap(ErrInitialSync,
			errors.WithDetail("syncpath during target cleanup"),
			errors.WithProperty("path", rel),
			errors.CausedBy(err),
		)
	}

	return nil
}
