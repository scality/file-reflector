package usecase

import (
	"context"
	"log/slog"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/domain"
	"github.com/scality/file-reflector/pkg/service"
)

// ErrSyncPath is the sentinel returned for any failure during a per-path
// reconciliation.
var ErrSyncPath = errors.New("sync path")

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

// Execute reconciles the path at relPath: stat both sides, branch on the
// kind combination, and apply the right TargetWriter operations to make
// target reflect source. The watched root itself and ignored paths are
// no-ops; symlinks on the source side are reported as absent so any
// stale entry under the same target path is cleaned up.
func (uc *SyncPath) Execute(ctx context.Context, relPath string) error {
	if relPath == "" {
		return nil
	}

	if uc.matcher.Matches(relPath) {
		return nil
	}

	src, err := uc.sourceReader.Stat(ctx, relPath)
	if err != nil {
		return errors.Wrap(ErrSyncPath,
			errors.WithDetail("stat source"),
			errors.WithProperty("path", relPath),
			errors.CausedBy(err),
		)
	}

	if src.Kind == domain.NodeSymlink {
		uc.logger.WarnContext(ctx, "source is a symlink; treating as absent so any stale target entry is removed",
			slog.String("path", relPath),
		)

		src = domain.FileNode{Kind: domain.NodeAbsent}
	}

	tgt, err := uc.targetWriter.Stat(ctx, relPath)
	if err != nil {
		return errors.Wrap(ErrSyncPath,
			errors.WithDetail("stat target"),
			errors.WithProperty("path", relPath),
			errors.CausedBy(err),
		)
	}

	switch src.Kind {
	case domain.NodeAbsent:
		return uc.reconcileAbsent(ctx, relPath, tgt)
	case domain.NodeFile:
		return uc.reconcileFile(ctx, relPath, src, tgt)
	case domain.NodeDir:
		return uc.reconcileDir(ctx, relPath, src, tgt)
	default:
		return errors.Wrap(ErrSyncPath,
			errors.WithDetail("unsupported source kind"),
			errors.WithProperty("path", relPath),
			errors.WithProperty("kind", src.Kind.String()),
		)
	}
}

func (uc *SyncPath) reconcileAbsent(ctx context.Context, relPath string, tgt domain.FileNode) error {
	if tgt.Kind == domain.NodeAbsent {
		return nil
	}

	if err := uc.targetWriter.Remove(ctx, relPath); err != nil {
		return errors.Wrap(ErrSyncPath,
			errors.WithDetail("remove target"),
			errors.WithProperty("path", relPath),
			errors.CausedBy(err),
		)
	}

	uc.logger.InfoContext(ctx, "removed target entry absent in source",
		slog.String("path", relPath),
	)

	return nil
}

func (uc *SyncPath) reconcileFile(ctx context.Context, relPath string, src, tgt domain.FileNode) error {
	tgt, err := uc.coerceTargetKind(ctx, relPath, tgt, domain.NodeFile)
	if err != nil {
		return err
	}

	rewrite, err := uc.fileNeedsRewrite(ctx, relPath, src, tgt)
	if err != nil {
		return err
	}

	if rewrite {
		return uc.writeFile(ctx, relPath, src)
	}

	return uc.syncMetadata(ctx, relPath, src, tgt)
}

func (uc *SyncPath) reconcileDir(ctx context.Context, relPath string, src, tgt domain.FileNode) error {
	tgt, err := uc.coerceTargetKind(ctx, relPath, tgt, domain.NodeDir)
	if err != nil {
		return err
	}

	if tgt.Kind == domain.NodeAbsent {
		return uc.makeDir(ctx, relPath, src)
	}

	return uc.syncMetadata(ctx, relPath, src, tgt)
}

// coerceTargetKind removes the target entry when its kind is neither the
// wanted one nor absent, so the caller can proceed against a clean slate
// (either the right kind or nothing at all).
func (uc *SyncPath) coerceTargetKind(
	ctx context.Context,
	relPath string,
	tgt domain.FileNode,
	wanted domain.NodeKind,
) (domain.FileNode, error) {
	if tgt.Kind == wanted || tgt.Kind == domain.NodeAbsent {
		return tgt, nil
	}

	if err := uc.targetWriter.Remove(ctx, relPath); err != nil {
		return tgt, errors.Wrap(ErrSyncPath,
			errors.WithDetail("remove wrong-kind target"),
			errors.WithProperty("path", relPath),
			errors.WithProperty("target_kind", tgt.Kind.String()),
			errors.WithProperty("wanted_kind", wanted.String()),
			errors.CausedBy(err),
		)
	}

	uc.logger.InfoContext(ctx, "removed wrong-kind target before recreation",
		slog.String("path", relPath),
		slog.String("target_kind", tgt.Kind.String()),
		slog.String("wanted_kind", wanted.String()),
	)

	return domain.FileNode{Kind: domain.NodeAbsent}, nil
}

func (uc *SyncPath) fileNeedsRewrite(
	ctx context.Context,
	relPath string,
	src, tgt domain.FileNode,
) (bool, error) {
	if tgt.Kind == domain.NodeAbsent || src.Size != tgt.Size {
		return true, nil
	}

	identical, err := uc.contentIdentical(ctx, relPath)
	if err != nil {
		return false, err
	}

	return !identical, nil
}

func (uc *SyncPath) makeDir(ctx context.Context, relPath string, src domain.FileNode) error {
	if err := uc.targetWriter.Mkdir(ctx, relPath, src.Mode, src.UID, src.GID); err != nil {
		return errors.Wrap(ErrSyncPath,
			errors.WithDetail("mkdir target"),
			errors.WithProperty("path", relPath),
			errors.CausedBy(err),
		)
	}

	uc.logger.InfoContext(ctx, "created target directory",
		slog.String("path", relPath),
	)

	return nil
}

func (uc *SyncPath) contentIdentical(ctx context.Context, relPath string) (bool, error) {
	srcHash, err := uc.sourceReader.Hash(ctx, relPath)
	if err != nil {
		return false, errors.Wrap(ErrSyncPath,
			errors.WithDetail("hash source"),
			errors.WithProperty("path", relPath),
			errors.CausedBy(err),
		)
	}

	tgtHash, err := uc.targetWriter.Hash(ctx, relPath)
	if err != nil {
		return false, errors.Wrap(ErrSyncPath,
			errors.WithDetail("hash target"),
			errors.WithProperty("path", relPath),
			errors.CausedBy(err),
		)
	}

	return srcHash == tgtHash, nil
}

func (uc *SyncPath) writeFile(ctx context.Context, relPath string, src domain.FileNode) error {
	body, err := uc.sourceReader.Open(ctx, relPath)
	if err != nil {
		return errors.Wrap(ErrSyncPath,
			errors.WithDetail("open source"),
			errors.WithProperty("path", relPath),
			errors.CausedBy(err),
		)
	}

	defer func() { _ = body.Close() }()

	if err := uc.targetWriter.WriteAtomic(ctx, relPath, body, src.Mode, src.UID, src.GID); err != nil {
		return errors.Wrap(ErrSyncPath,
			errors.WithDetail("write atomic"),
			errors.WithProperty("path", relPath),
			errors.CausedBy(err),
		)
	}

	uc.logger.InfoContext(ctx, "wrote target file from source",
		slog.String("path", relPath),
	)

	return nil
}

func (uc *SyncPath) syncMetadata(ctx context.Context, relPath string, src, tgt domain.FileNode) error {
	if src.Mode != tgt.Mode {
		if err := uc.targetWriter.Chmod(ctx, relPath, src.Mode); err != nil {
			return errors.Wrap(ErrSyncPath,
				errors.WithDetail("chmod target to match source"),
				errors.WithProperty("path", relPath),
				errors.CausedBy(err),
			)
		}
	}

	if src.UID != tgt.UID || src.GID != tgt.GID {
		if err := uc.targetWriter.Chown(ctx, relPath, src.UID, src.GID); err != nil {
			return errors.Wrap(ErrSyncPath,
				errors.WithDetail("chown target to match source"),
				errors.WithProperty("path", relPath),
				errors.CausedBy(err),
			)
		}
	}

	return nil
}
