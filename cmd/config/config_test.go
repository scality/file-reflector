package config_test

import (
	"io/fs"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/scality/go-errors"

	"github.com/scality/file-reflector/cmd/config"
)

// parse calls config.Parse with a fixed program name; these specs do not
// assert on the usage text, so the name is irrelevant to them.
func parse(args []string) (*config.Config, error) {
	return config.Parse("file-reflector", args)
}

var _ = Describe("Parse", func() {
	It("parses the two required flags", func() {
		cfg, err := parse([]string{"--source", "/src", "--target", "/tgt"})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Source).To(Equal("/src"))
		Expect(cfg.Target).To(Equal("/tgt"))
	})

	It("applies the documented defaults when optional flags are unset", func() {
		cfg, err := parse([]string{"--source", "/src", "--target", "/tgt"})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Ignore).To(Equal([]string{"..*"}))
		Expect(cfg.FileMode).To(BeNil())
		Expect(cfg.DirMode).To(BeNil())
		Expect(cfg.Owner).To(BeNil())
		Expect(cfg.LogFormat).To(Equal("text"))
		Expect(cfg.LogLevel).To(Equal("info"))
		Expect(cfg.SymlinkPollInterval).To(Equal(10 * time.Second))
	})

	DescribeTable("rejects an invocation missing a required flag",
		func(args []string) {
			_, err := parse(args)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, config.ErrInvalidConfig)).To(BeTrue())
		},
		Entry("missing --source", []string{"--target", "/tgt"}),
		Entry("missing --target", []string{"--source", "/src"}),
		Entry("missing both", []string{}),
	)

	It("collects repeated --ignore patterns in order, then the built-in ones", func() {
		cfg, err := parse([]string{
			"--source", "/src", "--target", "/tgt",
			"--ignore", "*.tmp",
			"--ignore", "cache",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Ignore).To(Equal([]string{"*.tmp", "cache", "..*"}))
	})

	It("keeps the built-in ignore when the user supplies their own patterns", func() {
		cfg, err := parse([]string{
			"--source", "/src", "--target", "/tgt",
			"--ignore", "*.log",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Ignore).To(ContainElement("..*"))
	})

	It("parses --symlink-poll-interval as a duration", func() {
		cfg, err := parse([]string{
			"--source", "/src", "--target", "/tgt",
			"--symlink-poll-interval", "1s",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.SymlinkPollInterval).To(Equal(time.Second))
	})

	It("accepts a zero --symlink-poll-interval to disable polling", func() {
		cfg, err := parse([]string{
			"--source", "/src", "--target", "/tgt",
			"--symlink-poll-interval", "0",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.SymlinkPollInterval).To(Equal(time.Duration(0)))
	})

	It("rejects a negative --symlink-poll-interval", func() {
		_, err := parse([]string{
			"--source", "/src", "--target", "/tgt",
			"--symlink-poll-interval", "-5s",
		})
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, config.ErrInvalidConfig)).To(BeTrue())
	})

	It("parses --file-mode and --dir-mode as octal", func() {
		cfg, err := parse([]string{
			"--source", "/src", "--target", "/tgt",
			"--file-mode", "0640",
			"--dir-mode", "0750",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.FileMode).To(HaveValue(Equal(fs.FileMode(0o640))))
		Expect(cfg.DirMode).To(HaveValue(Equal(fs.FileMode(0o750))))
	})

	DescribeTable("rejects a malformed mode",
		func(flag, value string) {
			_, err := parse([]string{
				"--source", "/src", "--target", "/tgt",
				flag, value,
			})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, config.ErrInvalidConfig)).To(BeTrue())
		},
		Entry("file-mode not octal", "--file-mode", "abc"),
		Entry("file-mode out of range", "--file-mode", "01777777"),
		Entry("dir-mode not octal", "--dir-mode", "0x755"),
		// The Unix special bits don't map onto fs.FileMode's layout:
		// accepting them would silently drop the bit at chmod time and
		// leave the reconciliation re-chmodding forever.
		Entry("file-mode with setuid bit", "--file-mode", "4755"),
		Entry("dir-mode with setgid bit", "--dir-mode", "2750"),
		Entry("dir-mode with sticky bit", "--dir-mode", "1777"),
	)

	It("accepts a zero mode", func() {
		cfg, err := parse([]string{
			"--source", "/src", "--target", "/tgt",
			"--file-mode", "0",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.FileMode).To(HaveValue(Equal(fs.FileMode(0))))
	})

	It("parses --owner as uid:gid", func() {
		cfg, err := parse([]string{
			"--source", "/src", "--target", "/tgt",
			"--owner", "1234:5678",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Owner).To(HaveValue(Equal(config.Owner{UID: 1234, GID: 5678})))
	})

	DescribeTable("rejects a malformed --owner",
		func(value string) {
			_, err := parse([]string{
				"--source", "/src", "--target", "/tgt",
				"--owner", value,
			})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, config.ErrInvalidConfig)).To(BeTrue())
		},
		Entry("missing separator", "1234"),
		Entry("non-numeric uid", "user:100"),
		Entry("non-numeric gid", "100:group"),
		Entry("negative uid", "-1:100"),
		Entry("empty", ""),
		Entry("empty uid part", ":100"),
		Entry("empty gid part", "100:"),
	)

	DescribeTable("accepts every documented log format and level",
		func(flag, value string) {
			_, err := parse([]string{
				"--source", "/src", "--target", "/tgt",
				flag, value,
			})
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("text format", "--log-format", "text"),
		Entry("json format", "--log-format", "json"),
		Entry("debug level", "--log-level", "debug"),
		Entry("info level", "--log-level", "info"),
		Entry("warn level", "--log-level", "warn"),
		Entry("error level", "--log-level", "error"),
	)

	DescribeTable("rejects unknown log format or level",
		func(flag, value string) {
			_, err := parse([]string{
				"--source", "/src", "--target", "/tgt",
				flag, value,
			})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, config.ErrInvalidConfig)).To(BeTrue())
		},
		Entry("unknown format", "--log-format", "yaml"),
		Entry("unknown level", "--log-level", "verbose"),
	)

	It("rejects an unknown flag", func() {
		_, err := parse([]string{
			"--source", "/src", "--target", "/tgt",
			"--frobnicate",
		})
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, config.ErrInvalidConfig)).To(BeTrue())
		// The pflag cause is preserved, so its message still reaches the user.
		Expect(err.Error()).To(ContainSubstring("unknown flag: --frobnicate"))
	})

	It("signals --help with ErrHelpRequested instead of exiting", func() {
		_, err := parse([]string{"--help"})
		Expect(errors.Is(err, config.ErrHelpRequested)).To(BeTrue())
	})

	It("signals --version with ErrVersionRequested instead of exiting", func() {
		_, err := parse([]string{"--version"})
		Expect(errors.Is(err, config.ErrVersionRequested)).To(BeTrue())
	})
})
