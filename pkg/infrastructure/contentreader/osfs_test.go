package contentreader_test

import (
	"context"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/infrastructure/contentreader"
)

var _ = Describe("OSFS", func() {
	var (
		tmpDir string
		root   *os.Root
		r      *contentreader.OSFS
		ctx    context.Context
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()

		var err error

		root, err = os.OpenRoot(tmpDir)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = root.Close() })

		r = contentreader.NewOSFS(root)
		ctx = context.Background()
	})

	It("returns a ReadCloser whose content matches the file", func() {
		content := []byte("the quick brown fox")
		Expect(os.WriteFile(filepath.Join(tmpDir, "f"), content, 0o600)).To(Succeed())

		rc, err := r.Open(ctx, "f")
		Expect(err).NotTo(HaveOccurred())

		defer func() { _ = rc.Close() }()

		got, err := io.ReadAll(rc)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(content))
	})

	It("returns an error wrapping ErrContentRead on a missing path", func() {
		_, err := r.Open(ctx, "missing")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, contentreader.ErrContentRead)).To(BeTrue())
	})

	It("returns an error wrapping ErrContentRead when the file is unreadable", func() {
		if os.Geteuid() == 0 {
			Skip("permission test skipped when running as root")
		}

		path := filepath.Join(tmpDir, "locked")
		Expect(os.WriteFile(path, []byte("x"), 0o600)).To(Succeed())
		Expect(os.Chmod(path, 0)).To(Succeed())
		DeferCleanup(func() { _ = os.Chmod(path, 0o600) })

		_, err := r.Open(ctx, "locked")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, contentreader.ErrContentRead)).To(BeTrue())
	})
	It("opens a file through a symlink (resolution stays inside the root)", func() {
		content := []byte("via link")
		Expect(os.WriteFile(filepath.Join(tmpDir, "real"), content, 0o600)).To(Succeed())
		Expect(os.Symlink("real", filepath.Join(tmpDir, "link"))).To(Succeed())

		rc, err := r.Open(ctx, "link")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = rc.Close() })

		got, err := io.ReadAll(rc)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(content))
	})

	It("refuses to open a symlink escaping the root", func() {
		Expect(os.Symlink("/etc/hostname", filepath.Join(tmpDir, "escape"))).To(Succeed())

		_, err := r.Open(ctx, "escape")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, contentreader.ErrContentRead)).To(BeTrue())
	})
})
