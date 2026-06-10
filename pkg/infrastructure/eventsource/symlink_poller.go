package eventsource

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/service"
)

// SymlinkPoller implements service.EventSource by periodically sweeping
// the root and emitting the path of every symlink it finds. Symlinks
// need this: inotify is inode-based, so a change *behind* a link — its
// target swapped, or the pointed-to file edited in place, possibly in
// an ignored subtree — never produces an event naming the link's path.
// Re-emitting each link on a fixed interval bounds how stale its mirror
// can get, with no state to maintain: a sweep that misses nothing can
// also leak nothing.
type SymlinkPoller struct {
	root     *os.Root
	interval time.Duration
	matcher  service.Matcher
	logger   *slog.Logger
}

// Compile-time interface assertion.
var _ service.EventSource = (*SymlinkPoller)(nil)

// NewSymlinkPoller returns a SymlinkPoller sweeping root every interval.
// matcher prunes ignored paths from the sweep: an ignored link is never
// emitted and an ignored directory is not descended into.
func NewSymlinkPoller(
	root *os.Root,
	interval time.Duration,
	matcher service.Matcher,
	logger *slog.Logger,
) *SymlinkPoller {
	return &SymlinkPoller{
		root:     root,
		interval: interval,
		matcher:  matcher,
		logger:   logger.With(slog.String("component_name", "symlink_poller")),
	}
}

// Run starts the periodic sweep and returns the channel of symlink
// paths. The channel is closed once ctx is cancelled.
func (p *SymlinkPoller) Run(ctx context.Context) (<-chan string, error) {
	out := make(chan string)

	// A non-positive interval means polling is disabled; return an
	// already-closed channel rather than letting time.NewTicker panic.
	if p.interval <= 0 {
		close(out)

		return out, nil
	}

	go func() {
		defer close(out)

		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.sweep(ctx, out)
			}
		}
	}()

	return out, nil
}

// sweep walks the root once and emits every non-ignored symlink. An
// unreadable subtree is skipped with a warning rather than aborting the
// sweep, so one bad directory cannot starve the links elsewhere.
func (p *SymlinkPoller) sweep(ctx context.Context, out chan<- string) {
	walkErr := fs.WalkDir(p.root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			// fs.ErrNotExist is the entry vanishing between listing and
			// visiting — a normal race on a live tree, not worth a log.
			if !errors.Is(err, fs.ErrNotExist) {
				p.logger.WarnContext(ctx, "skipping unreadable path during symlink sweep",
					slog.String("path", rel),
					slog.Any("err", err),
				)
			}

			return nil
		}

		if rel == "." {
			return nil
		}

		if p.matcher.Matches(rel) {
			if d.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}

		select {
		case out <- rel:
		case <-ctx.Done():
			return fs.SkipAll
		}

		return nil
	})
	// The callback never returns an error, so this only guards against
	// future regressions.
	if walkErr != nil {
		p.logger.ErrorContext(ctx, "symlink sweep failed; will retry at next tick",
			slog.Any("err", walkErr),
		)
	}
}
