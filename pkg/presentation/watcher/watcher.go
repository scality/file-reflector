package watcher

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/service"
)

// ErrWatcher is the sentinel returned when the watcher cannot start its
// event sources or the initial reconciliation fails.
var ErrWatcher = errors.New("watcher")

// DefaultRetryDelay is the recommended delay before a failed path is
// reconciled again.
const DefaultRetryDelay = 5 * time.Second

// retryBuffer caps the number of paths waiting for a retry slot. When
// the buffer is full further retries are dropped (and logged): the path
// will be reconciled again on its next event or at the next restart.
const retryBuffer = 128

// initialSyncer is the narrow surface the watcher needs from the
// InitialSync usecase.
type initialSyncer interface {
	Execute(ctx context.Context) error
}

// pathSyncer is the narrow surface the watcher needs from the SyncPath
// usecase.
type pathSyncer interface {
	Execute(ctx context.Context, relPath string) error
}

// Watcher is the long-running orchestrator: it runs the initial sync,
// then drains events from every EventSource and dispatches each to the
// SyncPath usecase. A failed path is logged and retried after
// retryDelay; one failure does not bring down the agent.
type Watcher struct {
	logger       *slog.Logger
	eventSources []service.EventSource
	initialSync  initialSyncer
	syncPath     pathSyncer
	retryDelay   time.Duration
}

// NewWatcher constructs a Watcher with its dependencies. retryDelay is
// how long a failed path waits before being reconciled again
// (DefaultRetryDelay is a sensible production value). The variadic
// eventSources lets the caller wire one source per watched tree (e.g.
// one for --source and one for --target); the watcher merges them
// internally.
func NewWatcher(
	logger *slog.Logger,
	initialSync initialSyncer,
	syncPath pathSyncer,
	retryDelay time.Duration,
	eventSources ...service.EventSource,
) *Watcher {
	return &Watcher{
		logger:       logger.With(slog.String("component_name", "watcher")),
		eventSources: eventSources,
		initialSync:  initialSync,
		syncPath:     syncPath,
		retryDelay:   retryDelay,
	}
}

// Run starts every event source, runs the initial reconciliation, then
// drains the merged event stream until ctx is cancelled or every source
// closes its channel. A nil return means a clean shutdown.
//
// The event sources are started BEFORE the initial sync on purpose:
// changes made while the initial reconciliation walks the trees are
// buffered by the watch and replayed once draining starts, narrowing
// the window where a change could be missed. The guarantee is bounded
// by the event-source buffers — under sustained load during a long
// initial sync the kernel queue can still overflow (surfaced as an
// "fsnotify error" log), and those events are reconciled only on their
// next occurrence or at the next restart.
//
// On return every source goroutine has stopped: drain consumes the
// merged stream until it closes, which (per the EventSource contract,
// the channel closes once ctx is cancelled) happens only after each
// source has released the shared roots — so the caller can close them.
func (w *Watcher) Run(ctx context.Context) error {
	// Everything started below (event sources, merge forwarders) hangs
	// off this derived context so it is torn down when Run returns,
	// whatever the exit path — including an initial-sync failure after
	// the sources have already been started.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	channels := make([]<-chan string, 0, len(w.eventSources))

	for _, es := range w.eventSources {
		ch, err := es.Run(ctx)
		if err != nil {
			return errors.Wrap(ErrWatcher,
				errors.WithDetail("start event source"),
				errors.CausedBy(err),
			)
		}

		channels = append(channels, ch)
	}

	if err := w.initialSync.Execute(ctx); err != nil {
		return errors.Wrap(ErrWatcher,
			errors.WithDetail("initial reconciliation"),
			errors.CausedBy(err),
		)
	}

	w.logger.InfoContext(ctx, "initial reconciliation complete; watching for changes")

	w.drain(ctx, merge(channels))

	return nil
}

// drain consumes the merged event stream, delegating every path to the
// SyncPath usecase. A failed path is requeued after retryDelay so a
// transient error (file mid-write, temporary permission issue) does not
// lose the event; a permanent error keeps logging at each retry, which
// is the operational signal to go look.
func (w *Watcher) drain(ctx context.Context, events <-chan string) {
	retries := make(chan string, retryBuffer)

	for {
		select {
		case <-ctx.Done():
			w.logger.InfoContext(ctx, "context cancelled; draining remaining events before stopping")
			drainUntilClosed(events)

			return
		case relPath, ok := <-events:
			if !ok {
				w.logger.InfoContext(ctx, "all event sources closed; stopping watcher")

				return
			}

			w.handle(ctx, relPath, retries)
		case relPath := <-retries:
			w.handle(ctx, relPath, retries)
		}
	}
}

// drainUntilClosed consumes and discards the merged stream until every
// event source has closed its channel. It is the shutdown join: only
// once the stream closes have the source goroutines stopped touching the
// shared roots, so the caller may close them.
func drainUntilClosed(events <-chan string) {
	for range events {
		// discard: shutting down, just letting the sources drain to close
	}
}

// handle reconciles one path and schedules a retry on failure.
func (w *Watcher) handle(ctx context.Context, relPath string, retries chan<- string) {
	if ctx.Err() != nil {
		// Already shutting down: do not process or schedule a retry.
		return
	}

	err := w.syncPath.Execute(ctx, relPath)
	if err == nil {
		return
	}

	w.logger.ErrorContext(ctx, "path reconciliation failed; will retry",
		slog.String("path", relPath),
		slog.Duration("retry_delay", w.retryDelay),
		slog.Any("error", err),
	)

	time.AfterFunc(w.retryDelay, func() {
		select {
		case retries <- relPath:
		default:
			w.logger.ErrorContext(ctx, "retry queue full; dropping path until its next event",
				slog.String("path", relPath),
			)
		}
	})
}

// merge fans the per-source channels into a single stream. Forwarders
// exit only when their input channel closes, so a closed merged channel
// means every source goroutine has stopped — the consumer must keep
// reading until then, which drain does even while shutting down.
func merge(channels []<-chan string) <-chan string {
	out := make(chan string)

	var wg sync.WaitGroup

	for _, ch := range channels {
		wg.Go(func() {
			for ev := range ch {
				out <- ev
			}
		})
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
