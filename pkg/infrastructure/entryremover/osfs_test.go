package entryremover_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/file-reflector/pkg/infrastructure/entryremover"
)

var _ = Describe("OSFS", func() {
	var (
		tmpDir string
		root   *os.Root
		r      *entryremover.OSFS
		ctx    context.Context
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()

		var err error

		root, err = os.OpenRoot(tmpDir)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = root.Close() })

		r = entryremover.NewOSFS(root)
		ctx = context.Background()
	})

	It("unlinks a regular file", func() {
		Expect(os.WriteFile(filepath.Join(tmpDir, "f"), []byte("x"), 0o600)).To(Succeed())

		Expect(r.Remove(ctx, "f")).To(Succeed())

		_, err := os.Stat(filepath.Join(tmpDir, "f"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("recursively removes a directory and its contents", func() {
		Expect(os.MkdirAll(filepath.Join(tmpDir, "d/sub"), 0o750)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tmpDir, "d/sub/x"), []byte("x"), 0o600)).To(Succeed())

		Expect(r.Remove(ctx, "d")).To(Succeed())

		_, err := os.Stat(filepath.Join(tmpDir, "d"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("is idempotent on a missing path", func() {
		Expect(r.Remove(ctx, "absent")).To(Succeed())
	})
})
