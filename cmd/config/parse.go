package config

import (
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/scality/go-errors"
	"github.com/spf13/pflag"

	"github.com/scality/file-reflector/pkg/version"
)

// ErrInvalidConfig is the sentinel returned when the command line cannot
// be parsed or fails validation.
var ErrInvalidConfig = errors.New("invalid configuration")

// ErrHelpRequested and ErrVersionRequested are returned when the user
// asked for --help or --version. They are not failures: parsing
// short-circuits and the caller decides to exit zero. Keeping the exit
// out of Parse leaves process control to main and keeps these paths
// testable.
var (
	ErrHelpRequested    = errors.New("help requested")
	ErrVersionRequested = errors.New("version requested")
)

var (
	logFormats = []string{"text", "json"}
	logLevels  = []string{"debug", "info", "warn", "error"}
)

// builtinIgnores are always appended to the user's --ignore patterns.
// Entries whose name starts with ".." are the plumbing of atomic
// publishers — the kubelet's AtomicWriter exposes ConfigMap and Secret
// volumes through `..data` and `..<timestamp>` entries and reserves the
// prefix by rejecting user keys that start with ".." — so mirroring
// them would duplicate the published content. They are appended (not a
// flag default) so user-supplied --ignore patterns never displace them.
var builtinIgnores = []string{"..*"}

// Parse turns the raw argument list (os.Args[1:]) into a validated
// Config. prog is the program name as invoked (filepath.Base(os.Args[0]))
// and is used in the usage text. There are no environment-variable
// fallbacks: flags only.
func Parse(prog string, args []string) (*Config, error) {
	flags := pflag.NewFlagSet(prog, pflag.ContinueOnError)
	flags.SortFlags = false

	source := flags.String("source", "",
		"path of the directory to watch (required)")
	target := flags.String("target", "",
		"path of the directory to sync to (required)")
	ignore := flags.StringArray("ignore", nil,
		"pattern of paths to never touch (repeatable)")
	fileMode := flags.String("file-mode", "",
		"octal mode forced on synced files (e.g. 0644); default: preserve source")
	dirMode := flags.String("dir-mode", "",
		"octal mode forced on synced directories (e.g. 0755); default: preserve source")
	owner := flags.String("owner", "",
		"uid:gid forced on synced entries; default: preserve source")
	symlinkPollInterval := flags.Duration("symlink-poll-interval", 10*time.Second,
		"how often source symlinks are re-checked for changes behind them; 0 disables")
	logFormat := flags.String("log-format", "text",
		"log output format: text|json")
	logLevel := flags.String("log-level", "info",
		"log verbosity: debug|info|warn|error")
	showVersion := flags.Bool("version", false,
		"print the version and exit")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			// pflag has already printed the usage text; asking for help
			// is not a configuration error.
			return nil, ErrHelpRequested
		}

		return nil, errors.Wrap(ErrInvalidConfig, errors.CausedBy(err))
	}

	if *showVersion {
		// Like --help, asking for the version short-circuits the run and
		// must not require the mandatory flags.
		fmt.Println(version.Version)

		return nil, ErrVersionRequested
	}

	if *source == "" || *target == "" {
		return nil, errors.Wrap(ErrInvalidConfig,
			errors.WithDetail("--source and --target are required"),
		)
	}

	if !slices.Contains(logFormats, *logFormat) {
		return nil, errors.Wrap(ErrInvalidConfig,
			errors.WithDetail("unknown log format"),
			errors.WithProperty("log_format", *logFormat),
		)
	}

	if !slices.Contains(logLevels, *logLevel) {
		return nil, errors.Wrap(ErrInvalidConfig,
			errors.WithDetail("unknown log level"),
			errors.WithProperty("log_level", *logLevel),
		)
	}

	if *symlinkPollInterval < 0 {
		return nil, errors.Wrap(ErrInvalidConfig,
			errors.WithDetail("--symlink-poll-interval must not be negative"),
			errors.WithProperty("symlink_poll_interval", symlinkPollInterval.String()),
		)
	}

	cfg := &Config{
		Source: *source,
		Target: *target,
		// Copy into a fresh slice so cfg.Ignore never aliases pflag's
		// backing array nor the package-level builtinIgnores.
		Ignore:              append(append([]string{}, *ignore...), builtinIgnores...),
		SymlinkPollInterval: *symlinkPollInterval,
		LogFormat:           *logFormat,
		LogLevel:            *logLevel,
	}

	if flags.Changed("file-mode") {
		if err := parseMode(*fileMode, "--file-mode", &cfg.FileMode); err != nil {
			return nil, err
		}
	}

	if flags.Changed("dir-mode") {
		if err := parseMode(*dirMode, "--dir-mode", &cfg.DirMode); err != nil {
			return nil, err
		}
	}

	if flags.Changed("owner") {
		if err := parseOwner(*owner, &cfg.Owner); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// parseMode parses value as an octal file mode into *out. The flag was
// explicitly provided, so an empty or malformed value is an error; an
// absent flag (handled by the caller) leaves *out nil ("preserve the
// source's mode").
//
// Only the nine permission bits are accepted. The Unix special bits
// (setuid 04000, setgid 02000, sticky 01000) do not occupy those
// positions in fs.FileMode — Go carries them as ModeSetuid/ModeSetgid/
// ModeSticky in the high bits — so an octal value above 0777 would
// produce a FileMode that chmod silently truncates while the
// reconciliation keeps seeing a divergence, re-chmodding (and re-firing
// a target event) forever.
func parseMode(value, flagName string, out **fs.FileMode) error {
	parsed, err := strconv.ParseUint(value, 8, 32)
	if err != nil {
		return errors.Wrap(ErrInvalidConfig,
			errors.WithDetail("not a valid octal mode"),
			errors.WithProperty("flag", flagName),
			errors.WithProperty("value", value),
			errors.CausedBy(err),
		)
	}

	if parsed > 0o777 {
		return errors.Wrap(ErrInvalidConfig,
			errors.WithDetail("only permission bits are supported (mode must be within 0777)"),
			errors.WithProperty("flag", flagName),
			errors.WithProperty("value", value),
		)
	}

	mode := fs.FileMode(parsed)
	*out = &mode

	return nil
}

// parseOwner parses value as "uid:gid" into *out. The flag was
// explicitly provided, so an empty or malformed value is an error; an
// absent flag (handled by the caller) leaves *out nil ("preserve the
// source's owner").
func parseOwner(value string, out **Owner) error {
	uidPart, gidPart, found := strings.Cut(value, ":")
	if !found {
		return errors.Wrap(ErrInvalidConfig,
			errors.WithDetail("owner must be uid:gid"),
			errors.WithProperty("value", value),
		)
	}

	uid, err := strconv.ParseUint(uidPart, 10, 32)
	if err != nil {
		return errors.Wrap(ErrInvalidConfig,
			errors.WithDetail("owner uid is not a valid unsigned integer"),
			errors.WithProperty("value", value),
			errors.CausedBy(err),
		)
	}

	gid, err := strconv.ParseUint(gidPart, 10, 32)
	if err != nil {
		return errors.Wrap(ErrInvalidConfig,
			errors.WithDetail("owner gid is not a valid unsigned integer"),
			errors.WithProperty("value", value),
			errors.CausedBy(err),
		)
	}

	*out = &Owner{UID: int(uid), GID: int(gid)}

	return nil
}
