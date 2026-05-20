package service

import "context"

// EventSource exposes a stream of "something changed at this path" hints.
// Consumers must not trust the payload to describe what changed; the
// agent re-stats both sides on every event.
type EventSource interface {
	// Run starts the underlying watcher and returns a receive-only channel
	// of relative paths (empty string for the root). The channel is closed
	// when ctx is cancelled or an unrecoverable error occurs.
	Run(ctx context.Context) (<-chan string, error)
}
