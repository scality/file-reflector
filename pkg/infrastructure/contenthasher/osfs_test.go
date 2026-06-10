package contenthasher_test

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/infrastructure/contenthasher"
)

var _ = Describe("OSFS", func() {
	var (
		tmpDir string
		root   *os.Root
		h      *contenthasher.OSFS
		ctx    context.Context
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()

		var err error

		root, err = os.OpenRoot(tmpDir)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = root.Close() })

		h = contenthasher.NewOSFS(root)
		ctx = context.Background()
	})

	It("returns the SHA-256 of the file content", func() {
		content := []byte("hi")
		Expect(os.WriteFile(filepath.Join(tmpDir, "f"), content, 0o600)).To(Succeed())

		got, err := h.Hash(ctx, "f")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(sha256.Sum256(content)))
	})

	It("returns an error wrapping ErrContentHash on a missing path", func() {
		_, err := h.Hash(ctx, "missing")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, contenthasher.ErrContentHash)).To(BeTrue())
	})
	It("hashes a file through a symlink (resolution stays inside the root)", func() {
		content := []byte("via link")
		Expect(os.WriteFile(filepath.Join(tmpDir, "real"), content, 0o600)).To(Succeed())
		Expect(os.Symlink("real", filepath.Join(tmpDir, "link"))).To(Succeed())

		got, err := h.Hash(ctx, "link")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(sha256.Sum256(content)))
	})
})
