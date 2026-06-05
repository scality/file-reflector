package watcher_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"
	"github.com/stretchr/testify/mock"

	"github.com/scality/file-reflector/pkg/presentation/watcher"
	"github.com/scality/file-reflector/pkg/service/mocks"
)

// retryDelay keeps the requeue loop fast in tests.
const retryDelay = 10 * time.Millisecond

type initialSyncRecorder struct {
	mu     sync.Mutex
	calls  int
	err    error
	onCall func()
}

func (r *initialSyncRecorder) Execute(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++

	if r.onCall != nil {
		r.onCall()
	}

	return r.err
}

func (r *initialSyncRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.calls
}

type pathSyncRecorder struct {
	mu       sync.Mutex
	paths    []string
	failures map[string]int
}

func (r *pathSyncRecorder) Execute(_ context.Context, relPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.paths = append(r.paths, relPath)

	if r.failures[relPath] > 0 {
		r.failures[relPath]--

		return errors.New("transient failure")
	}

	return nil
}

func (r *pathSyncRecorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.paths...)
}

// fakeEventSource is a test EventSource whose channel the spec drives. It
// honours the port contract — the channel closes once ctx is cancelled —
// so the watcher's shutdown join completes. close is also exposed so a
// spec can simulate a source terminating on its own; sync.Once keeps the
// two paths from double-closing.
type fakeEventSource struct {
	ch        chan string
	closeOnce sync.Once
	mock      *mocks.MockEventSource
}

func newEventSource(buffer int) *fakeEventSource {
	f := &fakeEventSource{ch: make(chan string, buffer)}
	f.mock = mocks.NewMockEventSource(GinkgoT())
	f.mock.EXPECT().Run(mock.Anything).RunAndReturn(
		func(ctx context.Context) (<-chan string, error) {
			go func() {
				<-ctx.Done()
				f.close()
			}()

			return f.ch, nil
		})

	return f
}

func (f *fakeEventSource) emit(path string) { f.ch <- path }

func (f *fakeEventSource) close() { f.closeOnce.Do(func() { close(f.ch) }) }

var _ = Describe("Watcher", func() {
	var (
		logger      *slog.Logger
		initialSync *initialSyncRecorder
		pathSync    *pathSyncRecorder
		ctx         context.Context
		cancel      context.CancelFunc
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		initialSync = &initialSyncRecorder{}
		pathSync = &pathSyncRecorder{}
		ctx, cancel = context.WithCancel(context.Background())

		DeferCleanup(func() { cancel() })
	})

	run := func(w *watcher.Watcher) <-chan error {
		done := make(chan error, 1)

		go func() { done <- w.Run(ctx) }()

		return done
	}

	It("runs the initial sync once", func() {
		src := newEventSource(0)
		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, src.mock)

		done := run(w)

		Eventually(initialSync.count).Should(Equal(1))

		src.close()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("starts the event sources before running the initial sync so no event is missed", func() {
		ch := make(chan string)

		var closeOnce sync.Once

		sourceStarted := false

		es := mocks.NewMockEventSource(GinkgoT())
		es.EXPECT().Run(mock.Anything).RunAndReturn(
			func(ctx context.Context) (<-chan string, error) {
				sourceStarted = true

				go func() {
					<-ctx.Done()
					closeOnce.Do(func() { close(ch) })
				}()

				return ch, nil
			})

		startedBeforeInitial := false
		initialSync.onCall = func() { startedBeforeInitial = sourceStarted }

		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, es)
		done := run(w)

		Eventually(initialSync.count).Should(Equal(1))
		Expect(startedBeforeInitial).To(BeTrue())

		cancel()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("fails fast when an event source cannot start", func() {
		es := mocks.NewMockEventSource(GinkgoT())
		es.EXPECT().Run(mock.Anything).Return(nil, errors.New("inotify limit"))

		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, es)

		err := w.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, watcher.ErrWatcher)).To(BeTrue())
		Expect(initialSync.count()).To(BeZero())
	})

	It("fails when the initial sync fails", func() {
		initialSync.err = errors.New("disk full")
		src := newEventSource(0)

		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, src.mock)

		err := w.Run(ctx)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, watcher.ErrWatcher)).To(BeTrue())
	})

	It("dispatches every event from every source to the path syncer", func() {
		src1 := newEventSource(2)
		src2 := newEventSource(2)
		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, src1.mock, src2.mock)

		done := run(w)

		src1.emit("a")
		src2.emit("b")
		src1.emit("c")

		Eventually(pathSync.seen).Should(ConsistOf("a", "b", "c"))

		src1.close()
		src2.close()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("keeps draining after a per-path sync failure", func() {
		src := newEventSource(2)
		pathSync.failures = map[string]int{"bad": 1}

		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, src.mock)
		done := run(w)

		src.emit("bad")
		src.emit("good")

		Eventually(pathSync.seen).Should(ContainElements("bad", "good"))

		src.close()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("requeues a failed path until it eventually succeeds", func() {
		src := newEventSource(1)
		pathSync.failures = map[string]int{"flaky": 2}

		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, src.mock)
		done := run(w)

		src.emit("flaky")

		// Two transient failures then one success: three attempts total.
		Eventually(pathSync.seen).Should(Equal([]string{"flaky", "flaky", "flaky"}))
		Consistently(pathSync.seen, "200ms").Should(HaveLen(3))

		src.close()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("processes new events while a retry is pending", func() {
		src := newEventSource(2)
		pathSync.failures = map[string]int{"flaky": 1}

		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, src.mock)
		done := run(w)

		src.emit("flaky")
		src.emit("fine")

		Eventually(pathSync.seen).Should(ConsistOf("flaky", "fine", "flaky"))

		src.close()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("returns nil when the context is cancelled", func() {
		src := newEventSource(0)
		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, src.mock)

		done := run(w)

		Eventually(initialSync.count).Should(Equal(1))
		cancel()

		Eventually(done).Should(Receive(BeNil()))
	})

	It("does not return on cancellation until every source has closed its channel", func() {
		// A source that has not yet closed its channel models one whose
		// goroutine is still finishing (and still touching the shared
		// roots). Run must stay blocked until it closes, otherwise the
		// caller could close those roots out from under it.
		ch := make(chan string)
		es := mocks.NewMockEventSource(GinkgoT())
		es.EXPECT().Run(mock.Anything).Return(ch, nil)

		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, es)
		done := run(w)

		Eventually(initialSync.count).Should(Equal(1))
		cancel()
		Consistently(done, "100ms").ShouldNot(Receive())

		close(ch)
		Eventually(done).Should(Receive(BeNil()))
	})

	It("returns nil when every event channel is closed", func() {
		src1 := newEventSource(0)
		src2 := newEventSource(0)
		w := watcher.NewWatcher(logger, initialSync, pathSync, retryDelay, src1.mock, src2.mock)

		done := run(w)

		src1.close()
		Consistently(done).ShouldNot(Receive())

		src2.close()
		Eventually(done).Should(Receive(BeNil()))
	})
})
