package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/file-reflector/cmd/config"
	"github.com/scality/file-reflector/pkg/infrastructure/di"
)

// These specs wire the full production graph through the DI container —
// real filesystem adapters, usecases, and watcher — against temporary
// directories, so they exercise behaviour no single layer can pin alone.
var _ = Describe("Symlink handling (full graph)", func() {
	newConfig := func() *config.Config {
		return &config.Config{
			Source:    GinkgoT().TempDir(),
			Target:    GinkgoT().TempDir(),
			LogFormat: "text",
			LogLevel:  "info",
		}
	}

	It("completes the initial sync without recursing through a directory symlink to an ancestor", func() {
		cfg := newConfig()
		Expect(os.WriteFile(filepath.Join(cfg.Source, "f.txt"), []byte("x"), 0o600)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(cfg.Source, "sub"), 0o750)).To(Succeed())
		Expect(os.Symlink("..", filepath.Join(cfg.Source, "sub", "up"))).To(Succeed())

		c := di.NewContainer(cfg)

		DeferCleanup(func() { _ = c.Close() })

		w, err := c.GetWatcher()
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		// A regression that follows directory symlinks would recurse
		// through sub/up, mirroring the tree into itself until the
		// path length limit fails the initial sync — and Run with it.
		Expect(w.Run(ctx)).To(Succeed())

		Expect(filepath.Join(cfg.Target, "f.txt")).To(BeARegularFile())
		Expect(filepath.Join(cfg.Target, "sub")).To(BeADirectory())
		Expect(filepath.Join(cfg.Target, "sub", "up")).NotTo(BeAnExistingFile())
	})
})
