package eventsource_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/file-reflector/pkg/infrastructure/eventsource"
)

// errorCapturingHandler is a slog.Handler that records the messages
// logged at error level, so a test can assert the watcher stayed quiet.
type errorCapturingHandler struct {
	mu       sync.Mutex
	messages []string
}

func (h *errorCapturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *errorCapturingHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		h.mu.Lock()
		h.messages = append(h.messages, r.Message)
		h.mu.Unlock()
	}

	return nil
}

func (h *errorCapturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *errorCapturingHandler) WithGroup(string) slog.Handler      { return h }

func (h *errorCapturingHandler) errors() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	return append([]string(nil), h.messages...)
}

var _ = Describe("Fsnotify", func() {
	var (
		tmpDir   string
		root     *os.Root
		f        *eventsource.Fsnotify
		ctx      context.Context
		cancel   context.CancelFunc
		received []string
		mu       sync.Mutex
		collect  func() []string
		logs     *errorCapturingHandler
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()

		var err error

		root, err = os.OpenRoot(tmpDir)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = root.Close() })

		logs = &errorCapturingHandler{}
		f = eventsource.NewFsnotify(root, slog.New(logs))
		ctx, cancel = context.WithCancel(context.Background())
		received = nil
		collect = func() []string {
			mu.Lock()
			defer mu.Unlock()

			seen := make(map[string]struct{}, len(received))
			out := make([]string, 0, len(received))

			for _, ev := range received {
				if _, ok := seen[ev]; ok {
					continue
				}

				seen[ev] = struct{}{}
				out = append(out, ev)
			}

			return out
		}
	})

	AfterEach(func() {
		cancel()
	})

	startAndDrain := func() <-chan string {
		events, err := f.Run(ctx)
		Expect(err).NotTo(HaveOccurred())

		go func() {
			for ev := range events {
				mu.Lock()

				received = append(received, ev)

				mu.Unlock()
			}
		}()

		// Give the kernel a moment to install the watch.
		time.Sleep(50 * time.Millisecond)

		return events
	}

	It("emits an event when a file appears at the root", func() {
		_ = startAndDrain()

		Expect(os.WriteFile(filepath.Join(tmpDir, "f.txt"), []byte("x"), 0o600)).To(Succeed())

		Eventually(collect, "2s").Should(ConsistOf("f.txt"))
		Consistently(collect, "200ms").Should(ConsistOf("f.txt"))
	})

	It("emits events for files inside a newly created subdirectory", func() {
		_ = startAndDrain()

		Expect(os.Mkdir(filepath.Join(tmpDir, "sub"), 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tmpDir, "sub", "g.txt"), []byte("y"), 0o600)).To(Succeed())

		Eventually(collect, "2s").Should(ConsistOf("sub", "sub/g.txt"))
		Consistently(collect, "200ms").Should(ConsistOf("sub", "sub/g.txt"))
	})

	It("does not log an error when a created entry is renamed away before it is walked", func() {
		_ = startAndDrain()

		// Mimic WriteAtomic: create a temp file then rename it over the
		// final name. The temp's Create event reaches the watcher after
		// the rename has removed it, so the recursive add walks a path
		// that no longer exists — a normal race, not an error.
		tmp := filepath.Join(tmpDir, "f.txt.file-reflector-tmp-deadbeef")
		final := filepath.Join(tmpDir, "f.txt")

		Expect(os.WriteFile(tmp, []byte("payload"), 0o600)).To(Succeed())
		Expect(os.Rename(tmp, final)).To(Succeed())

		Eventually(collect, "2s").Should(ContainElement("f.txt"))
		Consistently(logs.errors, "200ms").Should(BeEmpty())
	})

	It("emits an event when an entry is moved out of the watched tree", func() {
		Expect(os.WriteFile(filepath.Join(tmpDir, "leaving.txt"), []byte("x"), 0o600)).To(Succeed())

		elsewhere := GinkgoT().TempDir()

		_ = startAndDrain()

		Expect(os.Rename(
			filepath.Join(tmpDir, "leaving.txt"),
			filepath.Join(elsewhere, "leaving.txt"),
		)).To(Succeed())

		Eventually(collect, "2s").Should(ConsistOf("leaving.txt"))
		Consistently(collect, "200ms").Should(ConsistOf("leaving.txt"))
	})

	It("emits events for the entry and its pre-existing contents when a directory is moved into the watched tree", func() {
		staging := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(staging, "incoming", "inner"), 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(staging, "incoming", "a.txt"), []byte("a"), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(staging, "incoming", "inner", "b.txt"), []byte("b"), 0o600)).To(Succeed())

		_ = startAndDrain()

		Expect(os.Rename(
			filepath.Join(staging, "incoming"),
			filepath.Join(tmpDir, "incoming"),
		)).To(Succeed())

		expected := []string{
			"incoming",
			"incoming/a.txt",
			"incoming/inner",
			"incoming/inner/b.txt",
		}
		Eventually(collect, "2s").Should(ConsistOf(expected))
		Consistently(collect, "200ms").Should(ConsistOf(expected))
	})

	It("emits events at both the old and the new location on an in-tree move", func() {
		Expect(os.MkdirAll(filepath.Join(tmpDir, "from"), 0o750)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(tmpDir, "to"), 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tmpDir, "from", "x.txt"), []byte("x"), 0o600)).To(Succeed())

		_ = startAndDrain()

		Expect(os.Rename(
			filepath.Join(tmpDir, "from", "x.txt"),
			filepath.Join(tmpDir, "to", "x.txt"),
		)).To(Succeed())

		Eventually(collect, "2s").Should(ConsistOf("from/x.txt", "to/x.txt"))
		Consistently(collect, "200ms").Should(ConsistOf("from/x.txt", "to/x.txt"))
	})

	It("emits an event when a file is removed", func() {
		Expect(os.WriteFile(filepath.Join(tmpDir, "doomed.txt"), []byte("x"), 0o600)).To(Succeed())

		_ = startAndDrain()

		Expect(os.Remove(filepath.Join(tmpDir, "doomed.txt"))).To(Succeed())

		Eventually(collect, "2s").Should(ConsistOf("doomed.txt"))
		Consistently(collect, "200ms").Should(ConsistOf("doomed.txt"))
	})

	It("emits an event when an empty directory is removed", func() {
		Expect(os.Mkdir(filepath.Join(tmpDir, "doomed"), 0o750)).To(Succeed())

		_ = startAndDrain()

		Expect(os.Remove(filepath.Join(tmpDir, "doomed"))).To(Succeed())

		Eventually(collect, "2s").Should(ConsistOf("doomed"))
		Consistently(collect, "200ms").Should(ConsistOf("doomed"))
	})

	It("emits an event when an existing file's content is modified", func() {
		Expect(os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("old"), 0o600)).To(Succeed())

		_ = startAndDrain()

		Expect(os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("new"), 0o600)).To(Succeed())

		Eventually(collect, "2s").Should(ConsistOf("file.txt"))
		Consistently(collect, "200ms").Should(ConsistOf("file.txt"))
	})

	It("emits an event when a file's mode is changed", func() {
		Expect(os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("x"), 0o600)).To(Succeed())

		_ = startAndDrain()

		Expect(os.Chmod(filepath.Join(tmpDir, "file.txt"), 0o400)).To(Succeed())

		Eventually(collect, "2s").Should(ConsistOf("file.txt"))
		Consistently(collect, "200ms").Should(ConsistOf("file.txt"))
	})

	It("emits an event when a directory's mode is changed", func() {
		Expect(os.Mkdir(filepath.Join(tmpDir, "sub"), 0o750)).To(Succeed())

		_ = startAndDrain()

		Expect(os.Chmod(filepath.Join(tmpDir, "sub"), 0o500)).To(Succeed())

		Eventually(collect, "2s").Should(ConsistOf("sub"))
		Consistently(collect, "200ms").Should(ConsistOf("sub"))
	})

	It("closes the event channel when ctx is cancelled", func() {
		events := startAndDrain()

		cancel()

		Eventually(events).Should(BeClosed())
	})
})
