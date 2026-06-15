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

// startAgent runs the agent the way main would — flags parsed by the
// config package, graph wired by the DI container, watcher running in
// the background — and registers a cleanup that stops it and verifies
// the shutdown was clean.
func startAgent(source, target string, extraArgs ...string) {
	GinkgoHelper()

	args := append([]string{"--source", source, "--target", target}, extraArgs...)

	cfg, err := config.Parse("file-reflector", args)
	Expect(err).NotTo(HaveOccurred())

	c := di.NewContainer(cfg)

	w, err := c.GetWatcher()
	Expect(err).NotTo(HaveOccurred())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- w.Run(ctx) }()

	DeferCleanup(func() {
		cancel()

		var runErr error

		Eventually(done, 5*time.Second).Should(Receive(&runErr))
		Expect(runErr).NotTo(HaveOccurred())
		Expect(c.Close()).To(Succeed())
	})
}

// fileContent returns a polling function for Eventually that reads rel
// under dir, yielding "" until the file exists.
func fileContent(dir, rel string) func() string {
	return func() string {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return ""
		}

		return string(b)
	}
}

// fileGone returns a polling function for Eventually that reports
// whether rel under dir no longer exists.
func fileGone(dir, rel string) func() bool {
	return func() bool {
		_, err := os.Lstat(filepath.Join(dir, rel))

		return os.IsNotExist(err)
	}
}
