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
	"github.com/scality/file-reflector/pkg/infrastructure/matcher"
)

var _ = Describe("SymlinkPoller", func() {
	const interval = 20 * time.Millisecond

	var (
		tmpDir  string
		root    *os.Root
		ignores []string
		ctx     context.Context
		cancel  context.CancelFunc
		mu      sync.Mutex
		counts  map[string]int
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()

		var err error

		root, err = os.OpenRoot(tmpDir)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = root.Close() })

		ignores = nil
		ctx, cancel = context.WithCancel(context.Background())
		counts = map[string]int{}
	})

	AfterEach(func() {
		cancel()
	})

	// start builds the poller against the current ignores and drains its
	// stream into counts, so assertions can observe repeated emissions.
	start := func() <-chan string {
		m, err := matcher.NewGlob(ignores)
		Expect(err).NotTo(HaveOccurred())

		p := eventsource.NewSymlinkPoller(root, interval, m, slog.New(slog.DiscardHandler))

		events, err := p.Run(ctx)
		Expect(err).NotTo(HaveOccurred())

		go func() {
			for ev := range events {
				mu.Lock()

				counts[ev]++

				mu.Unlock()
			}
		}()

		return events
	}

	count := func(rel string) func() int {
		return func() int {
			mu.Lock()
			defer mu.Unlock()

			return counts[rel]
		}
	}

	It("emits a symlink path on every tick", func() {
		Expect(os.Symlink("missing", filepath.Join(tmpDir, "link"))).To(Succeed())

		start()

		Eventually(count("link")).Should(BeNumerically(">=", 2))
	})

	It("emits symlinks found in subdirectories", func() {
		Expect(os.MkdirAll(filepath.Join(tmpDir, "a/b"), 0o755)).To(Succeed())
		Expect(os.Symlink("missing", filepath.Join(tmpDir, "a/b/link"))).To(Succeed())

		start()

		Eventually(count("a/b/link")).Should(BeNumerically(">=", 1))
	})

	It("does not emit regular files or directories", func() {
		Expect(os.WriteFile(filepath.Join(tmpDir, "f.txt"), []byte("x"), 0o600)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(tmpDir, "d"), 0o755)).To(Succeed())
		Expect(os.Symlink("missing", filepath.Join(tmpDir, "link"))).To(Succeed())

		start()

		Eventually(count("link")).Should(BeNumerically(">=", 2))
		Expect(count("f.txt")()).To(BeZero())
		Expect(count("d")()).To(BeZero())
	})

	It("skips ignored symlinks", func() {
		ignores = []string{"..*"}

		Expect(os.Symlink("missing", filepath.Join(tmpDir, "..data"))).To(Succeed())
		Expect(os.Symlink("missing", filepath.Join(tmpDir, "link"))).To(Succeed())

		start()

		Eventually(count("link")).Should(BeNumerically(">=", 2))
		Expect(count("..data")()).To(BeZero())
	})

	It("does not descend into ignored directories", func() {
		ignores = []string{"cache"}

		Expect(os.Mkdir(filepath.Join(tmpDir, "cache"), 0o755)).To(Succeed())
		Expect(os.Symlink("missing", filepath.Join(tmpDir, "cache/link"))).To(Succeed())
		Expect(os.Symlink("missing", filepath.Join(tmpDir, "link"))).To(Succeed())

		start()

		Eventually(count("link")).Should(BeNumerically(">=", 2))
		Expect(count("cache/link")()).To(BeZero())
	})

	It("skips an unreadable directory and still emits the other symlinks", func() {
		if os.Geteuid() == 0 {
			Skip("root bypasses directory permissions, so the unreadable path cannot be exercised")
		}

		Expect(os.Mkdir(filepath.Join(tmpDir, "locked"), 0o000)).To(Succeed())
		DeferCleanup(func() { _ = os.Chmod(filepath.Join(tmpDir, "locked"), 0o755) })
		Expect(os.Symlink("missing", filepath.Join(tmpDir, "zlink"))).To(Succeed())

		start()

		// "locked" sorts before "zlink": only a sweep that survives the
		// unreadable directory can reach the link.
		Eventually(count("zlink")).Should(BeNumerically(">=", 2))
	})

	It("picks up symlinks created after it started", func() {
		start()

		// Let at least one empty sweep happen before the link appears.
		time.Sleep(2 * interval)
		Expect(os.Symlink("missing", filepath.Join(tmpDir, "late"))).To(Succeed())

		Eventually(count("late")).Should(BeNumerically(">=", 1))
	})

	It("closes the channel when the context is cancelled", func() {
		Expect(os.Symlink("missing", filepath.Join(tmpDir, "link"))).To(Succeed())

		events := start()

		cancel()

		Eventually(events).Should(BeClosed())
	})
})
