package integration_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("End-to-end sync (full graph)", func() {
	var source, target string

	BeforeEach(func() {
		source = GinkgoT().TempDir()
		target = GinkgoT().TempDir()
	})

	It("mirrors the source tree and removes target orphans at startup", func() {
		Expect(os.MkdirAll(filepath.Join(source, "a", "b"), 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(source, "a", "b", "f.txt"), []byte("payload"), 0o640)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(target, "orphan.txt"), []byte("stale"), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(target, "kept.txt"), []byte("external"), 0o600)).To(Succeed())

		startAgent(source, target, "--ignore", "kept.txt")

		Eventually(fileContent(target, "a/b/f.txt"), 2*time.Second).Should(Equal("payload"))
		Eventually(fileGone(target, "orphan.txt"), 2*time.Second).Should(BeTrue())
		Expect(fileContent(target, "kept.txt")()).To(Equal("external"))
	})

	It("propagates source creations, updates, and deletions while running", func() {
		startAgent(source, target)

		Expect(os.WriteFile(filepath.Join(source, "f.txt"), []byte("v1"), 0o600)).To(Succeed())
		Eventually(fileContent(target, "f.txt"), 2*time.Second).Should(Equal("v1"))

		Expect(os.WriteFile(filepath.Join(source, "f.txt"), []byte("v2"), 0o600)).To(Succeed())
		Eventually(fileContent(target, "f.txt"), 2*time.Second).Should(Equal("v2"))

		Expect(os.Remove(filepath.Join(source, "f.txt"))).To(Succeed())
		Eventually(fileGone(target, "f.txt"), 2*time.Second).Should(BeTrue())
	})

	It("heals out-of-band drift in the target", func() {
		Expect(os.WriteFile(filepath.Join(source, "f.txt"), []byte("good"), 0o600)).To(Succeed())

		startAgent(source, target)

		Eventually(fileContent(target, "f.txt"), 2*time.Second).Should(Equal("good"))

		Expect(os.WriteFile(filepath.Join(target, "f.txt"), []byte("tampered"), 0o600)).To(Succeed())
		Eventually(fileContent(target, "f.txt"), 2*time.Second).Should(Equal("good"))

		Expect(os.Remove(filepath.Join(target, "f.txt"))).To(Succeed())
		Eventually(fileContent(target, "f.txt"), 2*time.Second).Should(Equal("good"))
	})

	It("forces --file-mode on synced files and heals chmod drift", func() {
		Expect(os.WriteFile(filepath.Join(source, "f.txt"), []byte("x"), 0o640)).To(Succeed())

		startAgent(source, target, "--file-mode", "0600")

		mode := func() (fs.FileMode, error) {
			info, err := os.Stat(filepath.Join(target, "f.txt"))
			if err != nil {
				return 0, err
			}

			return info.Mode().Perm(), nil
		}

		Eventually(mode, 2*time.Second).Should(Equal(fs.FileMode(0o600)))

		Expect(os.Chmod(filepath.Join(target, "f.txt"), 0o644)).To(Succeed())
		Eventually(mode, 2*time.Second).Should(Equal(fs.FileMode(0o600)))
	})
})
