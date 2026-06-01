package metadatawriter_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/file-reflector/pkg/infrastructure/metadatawriter"
)

var _ = Describe("OSFS", func() {
	var (
		tmpDir     string
		root       *os.Root
		w          *metadatawriter.OSFS
		ctx        context.Context
		currentUID int
		currentGID int
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()

		var err error

		root, err = os.OpenRoot(tmpDir)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = root.Close() })

		w = metadatawriter.NewOSFS(root)
		ctx = context.Background()
		currentUID = os.Getuid()
		currentGID = os.Getgid()
	})

	Describe("Chmod", func() {
		It("leaves the mode unchanged when it already matches the requested value", func() {
			Expect(os.WriteFile(filepath.Join(tmpDir, "f"), []byte("x"), 0o600)).To(Succeed())

			Expect(w.Chmod(ctx, "f", 0o600)).To(Succeed())

			info, err := os.Stat(filepath.Join(tmpDir, "f"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
		})

		It("changes the mode to the requested value", func() {
			Expect(os.WriteFile(filepath.Join(tmpDir, "f"), []byte("x"), 0o600)).To(Succeed())

			Expect(w.Chmod(ctx, "f", 0o400)).To(Succeed())

			info, err := os.Stat(filepath.Join(tmpDir, "f"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o400)))
		})

		It("returns an error on a missing path", func() {
			Expect(w.Chmod(ctx, "missing", 0o600)).To(HaveOccurred())
		})
	})

	Describe("Chown", func() {
		It("leaves ownership unchanged when chowning to the current uid/gid", func() {
			Expect(os.WriteFile(filepath.Join(tmpDir, "f"), []byte("x"), 0o600)).To(Succeed())

			Expect(w.Chown(ctx, "f", currentUID, currentGID)).To(Succeed())

			var st syscall.Stat_t
			Expect(syscall.Stat(filepath.Join(tmpDir, "f"), &st)).To(Succeed())
			Expect(int(st.Uid)).To(Equal(currentUID))
			Expect(int(st.Gid)).To(Equal(currentGID))
		})

		It("changes the gid to one of the caller's supplementary groups", func() {
			groups, err := os.Getgroups()
			Expect(err).NotTo(HaveOccurred())

			differentGID := -1

			for _, g := range groups {
				if g != currentGID {
					differentGID = g

					break
				}
			}

			if differentGID == -1 {
				Skip("caller has no supplementary group different from its primary gid")
			}

			Expect(os.WriteFile(filepath.Join(tmpDir, "f"), []byte("x"), 0o600)).To(Succeed())

			Expect(w.Chown(ctx, "f", currentUID, differentGID)).To(Succeed())

			var st syscall.Stat_t
			Expect(syscall.Stat(filepath.Join(tmpDir, "f"), &st)).To(Succeed())
			Expect(int(st.Gid)).To(Equal(differentGID))
		})
	})
})
