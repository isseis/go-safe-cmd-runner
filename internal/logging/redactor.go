package logging

import (
	"context"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// RedactingHandler is a decorator that redacts sensitive information before forwarding to the underlying handler
type RedactingHandler struct {
	// Use the new common redacting handler internally
	commonHandler *common.RedactingHandler
}

// NewRedactingHandler creates a new redacting handler that wraps the given handler
func NewRedactingHandler(handler slog.Handler, options *common.RedactionOptions) *RedactingHandler {
	if options == nil {
		options = common.DefaultRedactionOptions()
	}

	return &RedactingHandler{
		commonHandler: common.NewRedactingHandler(handler, options),
	}
}

// Enabled reports whether the handler handles records at the given level
func (r *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return r.commonHandler.Enabled(ctx, level)
}

// Handler returns the underlying handler
func (r *RedactingHandler) Handler() slog.Handler {
	return r.commonHandler.Handler()
}

// Handle redacts the log record and forwards it to the underlying handler
func (r *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	return r.commonHandler.Handle(ctx, record)
}

// WithAttrs returns a new RedactingHandler with the given attributes
func (r *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newCommonHandler := r.commonHandler.WithAttrs(attrs)
	if redactingHandler, ok := newCommonHandler.(*common.RedactingHandler); ok {
		return &RedactingHandler{
			commonHandler: redactingHandler,
		}
	}
	// Fallback: wrap with a new RedactingHandler
	return &RedactingHandler{
		commonHandler: common.NewRedactingHandler(newCommonHandler, common.DefaultRedactionOptions()),
	}
}

// WithGroup returns a new RedactingHandler with the given group name
func (r *RedactingHandler) WithGroup(name string) slog.Handler {
	newCommonHandler := r.commonHandler.WithGroup(name)
	if redactingHandler, ok := newCommonHandler.(*common.RedactingHandler); ok {
		return &RedactingHandler{
			commonHandler: redactingHandler,
		}
	}
	// Fallback: wrap with a new RedactingHandler
	return &RedactingHandler{
		commonHandler: common.NewRedactingHandler(newCommonHandler, common.DefaultRedactionOptions()),
	}
}
