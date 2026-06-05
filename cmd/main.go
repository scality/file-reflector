package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/scality/file-reflector/cmd/config"
	"github.com/scality/file-reflector/pkg/infrastructure/di"
)

func main() {
	os.Exit(run())
}

// run carries the real main logic and returns the process exit code, so
// deferred cleanups execute before the process exits (os.Exit skips
// deferred calls when invoked directly from main).
func run() int {
	prog := filepath.Base(os.Args[0])

	cfg, err := config.Parse(prog, os.Args[1:])
	if err != nil {
		// --help / --version are not failures: usage/version has already
		// been printed, just exit zero.
		if errors.Is(err, config.ErrHelpRequested) || errors.Is(err, config.ErrVersionRequested) {
			return 0
		}

		// %s renders go-errors' human-readable form; the default %v
		// (what Fprintln would use) emits JSON, which is for logs.
		fmt.Fprintf(os.Stderr, "%s: %s\n", prog, err)
		fmt.Fprintf(os.Stderr, "\nRun '%s --help' for usage.\n", prog)

		return 2
	}

	container := di.NewContainer(cfg)

	defer func() { _ = container.Close() }()

	logger := container.GetLogger()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	watcher, err := container.GetWatcher()
	if err != nil {
		logger.ErrorContext(ctx, "startup failed", slog.Any("error", err))

		return 1
	}

	logger.InfoContext(ctx, "starting file-reflector",
		slog.String("source", cfg.Source),
		slog.String("target", cfg.Target),
		slog.Int("ignore_patterns", len(cfg.Ignore)),
	)

	if err := watcher.Run(ctx); err != nil {
		logger.ErrorContext(ctx, "watcher exited with error", slog.Any("error", err))

		return 1
	}

	logger.InfoContext(ctx, "file-reflector stopped")

	return 0
}
