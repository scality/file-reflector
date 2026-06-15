package metadatareader_test

import (
	"context"
	"io"
	"log/slog"
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
		It("returns NodeDir for the root itself (empty relative path)", func() {
			node, err := r.Stat(ctx, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeDir))
		})

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

var _ = Describe("OSFS following symlinks", func() {
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

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		r = metadatareader.NewFollowingOSFS(root, logger)
		ctx = context.Background()
	})

	Describe("Stat", func() {
		It("reports a symlink as its target file", func() {
			Expect(os.WriteFile(filepath.Join(tmpDir, "real.txt"), []byte("hello"), 0o640)).To(Succeed())
			Expect(os.Symlink("real.txt", filepath.Join(tmpDir, "link"))).To(Succeed())

			node, err := r.Stat(ctx, "link")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeFile))
			Expect(node.Size).To(Equal(int64(5)))
			Expect(node.Mode.Perm()).To(Equal(os.FileMode(0o640)))
		})

		It("resolves chained symlinks (the kubelet atomic-writer layout)", func() {
			Expect(os.Mkdir(filepath.Join(tmpDir, "..2026_06_10"), 0o750)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "..2026_06_10", "myfile.txt"), []byte("server"), 0o640)).To(Succeed())
			Expect(os.Symlink("..2026_06_10", filepath.Join(tmpDir, "..data"))).To(Succeed())
			Expect(os.Symlink("..data/myfile.txt", filepath.Join(tmpDir, "myfile.txt"))).To(Succeed())

			node, err := r.Stat(ctx, "myfile.txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeFile))
			Expect(node.Size).To(Equal(int64(6)))
		})

		It("treats a symlink to a directory as absent", func() {
			Expect(os.Mkdir(filepath.Join(tmpDir, "realdir"), 0o750)).To(Succeed())
			Expect(os.Symlink("realdir", filepath.Join(tmpDir, "dirlink"))).To(Succeed())

			node, err := r.Stat(ctx, "dirlink")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeAbsent))
		})

		It("reports a broken symlink as absent", func() {
			Expect(os.Symlink("nowhere", filepath.Join(tmpDir, "broken"))).To(Succeed())

			node, err := r.Stat(ctx, "broken")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeAbsent))
		})

		It("reports a symlink loop as absent", func() {
			Expect(os.Symlink("b", filepath.Join(tmpDir, "a"))).To(Succeed())
			Expect(os.Symlink("a", filepath.Join(tmpDir, "b"))).To(Succeed())

			node, err := r.Stat(ctx, "a")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeAbsent))
		})

		It("reports a symlink escaping the root as absent", func() {
			Expect(os.Symlink("/etc/hostname", filepath.Join(tmpDir, "escape"))).To(Succeed())

			node, err := r.Stat(ctx, "escape")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeAbsent))
		})

		It("propagates an access error instead of reporting absent", func() {
			if os.Geteuid() == 0 {
				Skip("root bypasses directory permissions, so the access error cannot be provoked")
			}

			// A link to a file behind an unsearchable directory: resolving
			// it fails with EACCES, which must surface as an error so the
			// caller requeues rather than deleting the mirror.
			Expect(os.Mkdir(filepath.Join(tmpDir, "vault"), 0o750)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "vault", "secret"), []byte("x"), 0o600)).To(Succeed())
			Expect(os.Symlink("vault/secret", filepath.Join(tmpDir, "link"))).To(Succeed())
			Expect(os.Chmod(filepath.Join(tmpDir, "vault"), 0o000)).To(Succeed())
			DeferCleanup(func() { _ = os.Chmod(filepath.Join(tmpDir, "vault"), 0o750) })

			_, err := r.Stat(ctx, "link")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, metadatareader.ErrMetadataRead)).To(BeTrue())
		})

		It("still reports a regular file directly", func() {
			Expect(os.WriteFile(filepath.Join(tmpDir, "plain"), []byte("x"), 0o600)).To(Succeed())

			node, err := r.Stat(ctx, "plain")
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Kind).To(Equal(domain.NodeFile))
		})
	})
})
