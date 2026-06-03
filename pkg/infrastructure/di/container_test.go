package di_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/cmd/config"
	"github.com/scality/file-reflector/pkg/infrastructure/di"
)

var _ = Describe("Container", func() {
	newConfig := func() *config.Config {
		return &config.Config{
			Source:    GinkgoT().TempDir(),
			Target:    GinkgoT().TempDir(),
			LogFormat: "text",
			LogLevel:  "info",
		}
	}

	Describe("GetWatcher", func() {
		It("surfaces a missing source directory", func() {
			cfg := newConfig()
			cfg.Source = "/does/not/exist"

			c := di.NewContainer(cfg)

			DeferCleanup(func() { _ = c.Close() })

			_, err := c.GetWatcher()
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, di.ErrOpenSourceRoot)).To(BeTrue())
		})

		It("surfaces a missing target directory", func() {
			cfg := newConfig()
			cfg.Target = "/does/not/exist"

			c := di.NewContainer(cfg)

			DeferCleanup(func() { _ = c.Close() })

			_, err := c.GetWatcher()
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, di.ErrOpenTargetRoot)).To(BeTrue())
		})
	})

	Describe("Close", func() {
		It("releases the root descriptors after a full wire-up", func() {
			c := di.NewContainer(newConfig())

			_, err := c.GetWatcher()
			Expect(err).NotTo(HaveOccurred())

			Expect(c.Close()).To(Succeed())
		})

		It("is a no-op when nothing was opened", func() {
			c := di.NewContainer(newConfig())

			Expect(c.Close()).To(Succeed())
		})

		It("is idempotent", func() {
			c := di.NewContainer(newConfig())

			_, err := c.GetWatcher()
			Expect(err).NotTo(HaveOccurred())

			Expect(c.Close()).To(Succeed())
			Expect(c.Close()).To(Succeed())
		})
	})
})
