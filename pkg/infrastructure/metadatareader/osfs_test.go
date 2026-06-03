package metadatareader_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/domain"
	"github.com/scality/file-reflector/pkg/infrastructure/metadatareader"
)

var _ = Describe("OSFS", func() {
	var (
		tmpDir string
		root   *os.Root
		r      *metadatareader.OSFS
		ctx    context.Context
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()

		var err error

		root, err = os.OpenRoot(tmpDir)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = root.Close() })

		r = metadatareader.NewOSFS(root)
		ctx = context.Background()
	})

	Describe("Stat", func() {
		It("returns NodeFile with size and mode for a regular file", func() {
			Expect(os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("hello"), 0o600)).To(Succeed())

			node, err := r.Stat(ctx, "a.txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeFile))
			Expect(node.Size).To(Equal(int64(5)))
			Expect(node.Mode.Perm()).To(Equal(os.FileMode(0o600)))
		})

		It("returns NodeAbsent (no error) for a missing path", func() {
			node, err := r.Stat(ctx, "missing")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeAbsent))
		})

		It("returns NodeDir for a directory", func() {
			Expect(os.Mkdir(filepath.Join(tmpDir, "sub"), 0o750)).To(Succeed())

			node, err := r.Stat(ctx, "sub")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeDir))
		})

		It("returns NodeSymlink for a symlink (does not follow)", func() {
			Expect(os.Symlink("nowhere", filepath.Join(tmpDir, "link"))).To(Succeed())

			node, err := r.Stat(ctx, "link")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeSymlink))
		})

		It("populates uid/gid from the underlying inode", func() {
			Expect(os.WriteFile(filepath.Join(tmpDir, "f"), []byte("x"), 0o600)).To(Succeed())

			node, err := r.Stat(ctx, "f")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.UID).To(Equal(os.Getuid()))
			Expect(node.GID).To(Equal(os.Getgid()))
		})
	})

	Describe("ReadDir", func() {
		It("lists the entries in a directory", func() {
			for _, name := range []string{"a", "b", "c"} {
				Expect(os.Mkdir(filepath.Join(tmpDir, name), 0o750)).To(Succeed())
			}

			entries, err := r.ReadDir(ctx, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(ConsistOf("a", "b", "c"))
		})

		It("returns an error wrapping ErrMetadataRead on a missing path", func() {
			_, err := r.ReadDir(ctx, "missing")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, metadatareader.ErrMetadataRead)).To(BeTrue())
		})

		It("returns an error wrapping ErrMetadataRead when the directory is unreadable", func() {
			if os.Geteuid() == 0 {
				Skip("permission test skipped when running as root")
			}

			path := filepath.Join(tmpDir, "locked")
			Expect(os.Mkdir(path, 0o750)).To(Succeed())
			Expect(os.Chmod(path, 0)).To(Succeed())
			DeferCleanup(func() { _ = os.Chmod(path, 0o750) })

			_, err := r.ReadDir(ctx, "locked")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, metadatareader.ErrMetadataRead)).To(BeTrue())
		})
	})
})
