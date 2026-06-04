package eventsource

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/service"
)

// ErrEventSource is the sentinel returned when the fsnotify-backed
// event source cannot be created or extended.
var ErrEventSource = errors.New("event source")

// Fsnotify implements service.EventSource using fsnotify. It watches a
// root recursively, adding new subdirectories to the watch set as soon
// as they appear so events deep in the tree are not missed.
type Fsnotify struct {
	root   *os.Root
	logger *slog.Logger
}

// Compile-time interface assertion.
var _ service.EventSource = (*Fsnotify)(nil)

// NewFsnotify returns an Fsnotify event source rooted at root.
func NewFsnotify(root *os.Root, logger *slog.Logger) *Fsnotify {
	return &Fsnotify{
		root:   root,
		logger: logger.With(slog.String("component_name", "fsnotify")),
	}
}

// Run installs the watcher over the root and returns a channel of
// relative paths whose state may have changed. The channel is closed
// when ctx is cancelled or the underlying watcher shuts down.
func (f *Fsnotify) Run(ctx context.Context) (<-chan string, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrap(ErrEventSource,
			errors.WithDetail("create watcher"),
			errors.CausedBy(err),
		)
	}

	if err := f.addRecursive(fsw, "."); err != nil {
		_ = fsw.Close()

		return nil, err
	}

	out := make(chan string, 128)

	go f.loop(ctx, fsw, out)

	return out, nil
}

// addRecursive walks rel within the root and adds every directory it
// finds to the watcher. fsnotify is non-recursive by design — without
// this, events inside subdirectories would never be observed.
func (f *Fsnotify) addRecursive(fsw *fsnotify.Watcher, rel string) error {
	walkErr := fs.WalkDir(f.root.FS(), rel, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// The entry vanished between the Create event and this walk:
			// a short-lived file such as our own write temp (created then
			// renamed into place), or anything a third party creates then
			// removes. That is a normal race on a tree we actively mutate,
			// not a failure.
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}

			return err //nolint:wrapcheck // WalkDir callback; the outer call re-wraps
		}

		if d.IsDir() {
			// fsnotify operates on absolute paths.
			if err := fsw.Add(filepath.Join(f.root.Name(), p)); err != nil {
				return errors.Wrap(ErrEventSource,
					errors.WithDetail("fsnotify add"),
					errors.WithProperty("path", p),
					errors.CausedBy(err),
				)
			}
		}

		return nil
	})
	if walkErr != nil {
		return errors.Wrap(ErrEventSource,
			errors.WithDetail("walk for initial watch"),
			errors.WithProperty("path", rel),
			errors.CausedBy(walkErr),
		)
	}

	return nil
}

func (f *Fsnotify) loop(ctx context.Context, fsw *fsnotify.Watcher, out chan<- string) {
	var wg sync.WaitGroup

	defer func() {
		wg.Wait()  // drain in-flight emitters before closing out
		close(out) // signal end-of-stream to the consumer

		_ = fsw.Close() // release the underlying watcher
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}

			f.logger.ErrorContext(ctx, "fsnotify error", slog.Any("err", err))
		case ev, ok := <-fsw.Events:
			if !ok {
				return
			}

			f.handleEvent(ctx, fsw, ev, out, &wg)
		}
	}
}

func (f *Fsnotify) handleEvent(
	ctx context.Context,
	fsw *fsnotify.Watcher,
	ev fsnotify.Event,
	out chan<- string,
	wg *sync.WaitGroup,
) {
	rel, err := f.toRel(ev.Name)
	if err != nil {
		f.logger.ErrorContext(ctx, "compute relative path failed",
			slog.String("path", ev.Name),
			slog.Any("err", err),
		)

		return
	}

	select {
	case out <- rel:
	case <-ctx.Done():
		return
	}

	// On Create events the new entry may be a directory: extend the
	// watch set to its subtree and synthesise events for whatever is
	// already underneath (the kernel only fires Create on the top-level
	// entry, not on its pre-existing contents in the case of a move-in).
	// For a regular file the walk visits a single entry that is not a
	// directory, so addRecursive adds nothing and emitInitial emits
	// nothing — no need to discriminate up front.
	if ev.Op&fsnotify.Create != 0 {
		// fs.WalkDir wants "." for the root; rel is "" for that case.
		walkRel := rel
		if walkRel == "" {
			walkRel = "."
		}

		if err := f.addRecursive(fsw, walkRel); err != nil {
			f.logger.ErrorContext(ctx, "recursive add failed",
				slog.String("path", ev.Name),
				slog.Any("err", err),
			)
		}

		// Emit in a separate goroutine so a large moved-in tree does
		// not block the event loop from draining fsnotify's buffer.
		wg.Go(func() { f.emitInitial(ctx, walkRel, out) })
	}
}

// emitInitial walks rel within the root and emits an event for every
// entry strictly under it (the walk root itself is the caller's
// responsibility, since handleEvent already emits the event path).
func (f *Fsnotify) emitInitial(ctx context.Context, rel string, out chan<- string) {
	walkErr := fs.WalkDir(f.root.FS(), rel, func(p string, _ fs.DirEntry, err error) error {
		if err != nil || p == rel {
			return nil //nolint:nilerr // tolerate vanished paths; root is emitted by the caller
		}

		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err() //nolint:wrapcheck // sentinel-style cancellation, propagated via WalkDir
		}

		return nil
	})
	if walkErr != nil {
		f.logger.ErrorContext(ctx, "emit initial events failed",
			slog.String("path", rel),
			slog.Any("err", walkErr),
		)
	}
}

// toRel returns abs as a path relative to the watcher's root. Returns
// ("", nil) when abs *is* the root itself (the port's convention for
// the root). Returns a non-nil error when abs cannot be expressed
// relative to root — distinct from the root case so callers can log
// instead of silently dropping the event.
func (f *Fsnotify) toRel(abs string) (string, error) {
	rel, err := filepath.Rel(f.root.Name(), abs)
	if err != nil {
		return "", errors.Wrap(ErrEventSource,
			errors.WithDetail("compute relative path"),
			errors.WithProperty("root", f.root.Name()),
			errors.WithProperty("path", abs),
			errors.CausedBy(err),
		)
	}

	if rel == "." {
		return "", nil
	}

	return rel, nil
}
