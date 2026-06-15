package integration_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These specs wire the full production graph through the DI container —
// real filesystem adapters, usecases, and watcher — against temporary
// directories, so they exercise behaviour no single layer can pin alone.
var _ = Describe("Symlink handling (full graph)", func() {
	It("completes the initial sync without recursing through a directory symlink to an ancestor", func() {
		source := GinkgoT().TempDir()
		target := GinkgoT().TempDir()

		Expect(os.WriteFile(filepath.Join(source, "f.txt"), []byte("x"), 0o600)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(source, "sub"), 0o750)).To(Succeed())
		Expect(os.Symlink("..", filepath.Join(source, "sub", "up"))).To(Succeed())

		// A regression that followed the directory symlink would recurse
		// through sub/up, mirroring the tree into itself until the path
		// length limit failed the initial sync. startAgent's cleanup
		// asserts Run returned without error, so that failure surfaces
		// there rather than relying on a timeout.
		startAgent(source, target)

		Eventually(fileContent(target, "f.txt"), 2*time.Second).Should(Equal("x"))
		Expect(filepath.Join(target, "sub")).To(BeADirectory())
		Expect(filepath.Join(target, "sub", "up")).NotTo(BeAnExistingFile())
	})

	It("syncs a kubelet ConfigMap volume with no extra options", func() {
		source := GinkgoT().TempDir()
		target := GinkgoT().TempDir()

		// The kubelet AtomicWriter layout, publishing version 1.
		Expect(os.Mkdir(filepath.Join(source, "..ts1"), 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(source, "..ts1", "myfile.txt"), []byte("v1"), 0o640)).To(Succeed())
		Expect(os.Symlink("..ts1", filepath.Join(source, "..data"))).To(Succeed())
		Expect(os.Symlink("..data/myfile.txt", filepath.Join(source, "myfile.txt"))).To(Succeed())

		startAgent(source, target, "--symlink-poll-interval", "100ms")

		Eventually(fileContent(target, "myfile.txt"), 2*time.Second).Should(Equal("v1"))

		// Only the published key is mirrored, never the ..* plumbing.
		entries, err := os.ReadDir(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))

		// The kubelet publishes version 2: new timestamped directory,
		// atomic ..data swap, old directory removed. No filesystem event
		// ever names myfile.txt; only the poller can see the change.
		Expect(os.Mkdir(filepath.Join(source, "..ts2"), 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(source, "..ts2", "myfile.txt"), []byte("v2"), 0o640)).To(Succeed())
		Expect(os.Symlink("..ts2", filepath.Join(source, "..data_tmp"))).To(Succeed())
		Expect(os.Rename(filepath.Join(source, "..data_tmp"), filepath.Join(source, "..data"))).To(Succeed())
		Expect(os.RemoveAll(filepath.Join(source, "..ts1"))).To(Succeed())

		Eventually(fileContent(target, "myfile.txt"), 2*time.Second).Should(Equal("v2"))
	})

	It("propagates an in-place edit behind a symlink into an ignored directory", func() {
		source := GinkgoT().TempDir()
		target := GinkgoT().TempDir()

		Expect(os.Mkdir(filepath.Join(source, "cache"), 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(source, "cache", "file.txt"), []byte("v1"), 0o640)).To(Succeed())
		Expect(os.Symlink("cache/file.txt", filepath.Join(source, "link"))).To(Succeed())

		startAgent(source, target, "--ignore", "cache", "--symlink-poll-interval", "100ms")

		Eventually(fileContent(target, "link"), 2*time.Second).Should(Equal("v1"))

		// The write event names cache/file.txt, which reconciliation
		// ignores; only the poller re-emits the link path.
		Expect(os.WriteFile(filepath.Join(source, "cache", "file.txt"), []byte("v2"), 0o640)).To(Succeed())

		Eventually(fileContent(target, "link"), 2*time.Second).Should(Equal("v2"))
		Expect(filepath.Join(target, "cache")).NotTo(BeAnExistingFile())
	})
})
