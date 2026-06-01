package eventsource

import (
	"context"
	"log/slog"

	"github.com/scality/file-reflector/pkg/service"
)

// Fsnotify implements service.EventSource using fsnotify.
type Fsnotify struct {
	root   string
	logger *slog.Logger
}

// Compile-time interface assertion.
var _ service.EventSource = (*Fsnotify)(nil)

// NewFsnotify returns an Fsnotify event source rooted at root.
func NewFsnotify(root string, logger *slog.Logger) *Fsnotify {
	return &Fsnotify{
		root:   root,
		logger: logger.With(slog.String("component_name", "fsnotify")),
	}
}

// Run starts the event source.
//
// TODO: real implementation. The skeleton returns an already-closed
// channel so the consumer's drain loop exits cleanly.
func (f *Fsnotify) Run(_ context.Context) (<-chan string, error) {
	ch := make(chan string)
	close(ch)

	return ch, nil
}
