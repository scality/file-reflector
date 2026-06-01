package di

import (
	"log/slog"
	"os"
	"strings"

	"github.com/scality/file-reflector/pkg/version"
)

// getLogger returns the process-wide logger configured per --log-format
// and --log-level. Output goes to stderr. Unknown levels fall back to
// info; unknown formats fall back to text.
func (c *Container) getLogger() *slog.Logger {
	if c.logger == nil {
		opts := &slog.HandlerOptions{Level: parseLogLevel(c.cfg.LogLevel)}

		var handler slog.Handler

		switch strings.ToLower(c.cfg.LogFormat) {
		case "json":
			handler = slog.NewJSONHandler(os.Stderr, opts)
		default:
			handler = slog.NewTextHandler(os.Stderr, opts)
		}

		c.logger = slog.New(handler).With(
			slog.String("application_name", "file-reflector"),
			slog.String("application_version", version.Version),
		)
	}

	return c.logger
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
