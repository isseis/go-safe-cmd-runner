// Package logging provides a flexible and extensible logging framework built on top of slog.
// It supports advanced features such as multi-handler log distribution, redaction of sensitive information,
// and integration with external services like Slack for real-time alerting. The package is designed to help
// applications manage log output efficiently, ensure sensitive data is protected, and enable seamless notification
// workflows through pluggable handlers.
package logging

import (
	"context"
	"errors"
	"log/slog"
)

// MultiHandler is a slog.Handler that dispatches log records to multiple handlers.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a new MultiHandler that wraps the given handlers.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{
		handlers: handlers,
	}
}

// Enabled reports whether the handler handles records at the given level.
// The handler is enabled if at least one of its underlying handlers is enabled.
func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle handles the log record by passing it to all underlying handlers.
func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	var multiErr error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r.Clone()); err != nil {
				// Aggregate all errors (first error + wrap)
				if multiErr == nil {
					multiErr = err
				} else {
					multiErr = errors.Join(multiErr, err)
				}
			}
		}
	}
	return multiErr
}

// Handlers returns a copy of the underlying handlers slice
func (h *MultiHandler) Handlers() []slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	copy(handlers, h.handlers)
	return handlers
}

// WithAttrs returns a new MultiHandler whose handlers have the given attributes.
func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: newHandlers}
}

// WithGroup returns a new MultiHandler whose handlers have the given group name.
func (h *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &MultiHandler{handlers: newHandlers}
}
