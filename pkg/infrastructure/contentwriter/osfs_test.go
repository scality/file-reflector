package contentwriter_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/pkg/infrastructure/contentwriter"
	"github.com/scality/file-reflector/pkg/infrastructure/metadatawriter"
)

var _ = Describe("OSFS", func() {
	var (
		tmpDir     string
		root       *os.Root
		w          *contentwriter.OSFS
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

		w = contentwriter.NewOSFS(root, metadatawriter.NewOSFS(root))
		ctx = context.Background()
		currentUID = os.Getuid()
		currentGID = os.Getgid()
	})

	Describe("Mkdir", func() {
		It("creates the directory with the given mode and owner", func() {
			Expect(w.Mkdir(ctx, "sub", 0o750, currentUID, currentGID)).To(Succeed())

			info, err := os.Stat(filepath.Join(tmpDir, "sub"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o750)))
		})

		It("creates parent directories as needed", func() {
			Expect(w.Mkdir(ctx, "a/b/c", 0o750, currentUID, currentGID)).To(Succeed())

			info, err := os.Stat(filepath.Join(tmpDir, "a/b/c"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())
		})
	})

	Describe("WriteAtomic", func() {
		It("creates the file with the given content and mode", func() {
			body := []byte("payload")
			Expect(w.WriteAtomic(ctx, "f", bytes.NewReader(body), 0o600, currentUID, currentGID)).To(Succeed())

			got, err := os.ReadFile(filepath.Join(tmpDir, "f"))
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(body))

			info, err := os.Stat(filepath.Join(tmpDir, "f"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
		})

		It("overwrites an existing file", func() {
			Expect(os.WriteFile(filepath.Join(tmpDir, "f"), []byte("old"), 0o600)).To(Succeed())

			body := []byte("new content")
			Expect(w.WriteAtomic(ctx, "f", bytes.NewReader(body), 0o600, currentUID, currentGID)).To(Succeed())

			got, err := os.ReadFile(filepath.Join(tmpDir, "f"))
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(body))
		})

		It("returns an error when the parent directory does not exist", func() {
			err := w.WriteAtomic(ctx, "a/b/c.txt", bytes.NewReader([]byte("x")), 0o600, currentUID, currentGID)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, contentwriter.ErrContentWrite)).To(BeTrue())
		})

		It("leaves no temp file behind on success", func() {
			Expect(w.WriteAtomic(ctx, "f", bytes.NewReader([]byte("x")), 0o600, currentUID, currentGID)).To(Succeed())

			entries, err := os.ReadDir(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}

			Expect(names).To(ConsistOf("f"))
		})
	})
})
