package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/terminal"
)

// Static errors for ConditionalTextHandler validation
var (
	ErrConditionalTextHandlerCapabilitiesRequired = errors.New("ConditionalTextHandler: Capabilities is required")
	ErrConditionalTextHandlerWriterRequired       = errors.New("ConditionalTextHandler: Writer is required")
)

// ConditionalTextHandler is a slog handler that wraps a standard text handler
// but only operates when not in an interactive environment. This allows for
// clean separation between interactive and non-interactive output.
type ConditionalTextHandler struct {
	capabilities terminal.Capabilities
	textHandler  slog.Handler
}

// ConditionalTextHandlerOptions configures the ConditionalTextHandler.
type ConditionalTextHandlerOptions struct {
	// Capabilities provides terminal feature detection
	Capabilities terminal.Capabilities

	// TextHandlerOptions will be passed to slog.NewTextHandler
	TextHandlerOptions *slog.HandlerOptions

	// Writer is the output destination for the text handler
	Writer io.Writer
}

// NewConditionalTextHandler creates a new ConditionalTextHandler that wraps
// a standard slog.TextHandler and only operates in non-interactive environments.
// Returns an error if any required options are missing.
func NewConditionalTextHandler(opts ConditionalTextHandlerOptions) (*ConditionalTextHandler, error) {
	if opts.Capabilities == nil {
		return nil, ErrConditionalTextHandlerCapabilitiesRequired
	}
	if opts.Writer == nil {
		return nil, ErrConditionalTextHandlerWriterRequired
	}

	// Create the underlying text handler
	textHandler := slog.NewTextHandler(opts.Writer, opts.TextHandlerOptions)

	return &ConditionalTextHandler{
		capabilities: opts.Capabilities,
		textHandler:  textHandler,
	}, nil
}

// Enabled reports whether the handler handles records at the given level.
// This handler only operates in non-interactive environments.
func (h *ConditionalTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Only enable if we're NOT in an interactive environment
	if h.capabilities.IsInteractive() {
		return false
	}

	// Delegate to the underlying text handler
	return h.textHandler.Enabled(ctx, level)
}

// Handle processes a log record by delegating to the underlying text handler
// if we're not in an interactive environment.
func (h *ConditionalTextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Only handle if we're NOT in an interactive environment
	if h.capabilities.IsInteractive() {
		return nil
	}

	// Delegate to the underlying text handler
	return h.textHandler.Handle(ctx, r)
}

// WithAttrs returns a new handler with additional attributes.
func (h *ConditionalTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ConditionalTextHandler{
		capabilities: h.capabilities,
		textHandler:  h.textHandler.WithAttrs(attrs),
	}
}

// WithGroup returns a new handler with an additional group.
func (h *ConditionalTextHandler) WithGroup(name string) slog.Handler {
	return &ConditionalTextHandler{
		capabilities: h.capabilities,
		textHandler:  h.textHandler.WithGroup(name),
	}
}
