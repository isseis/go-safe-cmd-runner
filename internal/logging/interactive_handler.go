package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/terminal"
)

// Static errors for InteractiveHandler validation
var (
	ErrInteractiveHandlerWriterRequired       = errors.New("InteractiveHandler: Writer is required")
	ErrInteractiveHandlerCapabilitiesRequired = errors.New("InteractiveHandler: Capabilities is required")
	ErrInteractiveHandlerFormatterRequired    = errors.New("InteractiveHandler: Formatter is required")
	ErrInteractiveHandlerLineTrackerRequired  = errors.New("InteractiveHandler: LineTracker is required")
)

// InteractiveHandler is a slog handler that provides enhanced output for interactive terminals.
// It integrates with the terminal package to detect capabilities and provide colored output
// with log file hints for error-level messages.
type InteractiveHandler struct {
	capabilities terminal.Capabilities
	formatter    MessageFormatter
	lineTracker  LogLineTracker
	writer       io.Writer
	level        slog.Level
	attrs        []slog.Attr
	groups       []string
}

// InteractiveHandlerOptions configures the InteractiveHandler.
type InteractiveHandlerOptions struct {
	// Level is the minimum log level to handle
	Level slog.Level

	// Writer is the output destination (typically os.Stderr for interactive output)
	Writer io.Writer

	// Capabilities provides terminal feature detection
	Capabilities terminal.Capabilities

	// Formatter handles message formatting and coloring
	Formatter MessageFormatter

	// LineTracker tracks log line numbers for file hints
	LineTracker LogLineTracker
}

// NewInteractiveHandler creates a new InteractiveHandler with the given options.
// Returns an error if any required options are missing.
func NewInteractiveHandler(opts InteractiveHandlerOptions) (*InteractiveHandler, error) {
	if opts.Writer == nil {
		return nil, ErrInteractiveHandlerWriterRequired
	}
	if opts.Capabilities == nil {
		return nil, ErrInteractiveHandlerCapabilitiesRequired
	}
	if opts.Formatter == nil {
		return nil, ErrInteractiveHandlerFormatterRequired
	}
	if opts.LineTracker == nil {
		return nil, ErrInteractiveHandlerLineTrackerRequired
	}

	return &InteractiveHandler{
		capabilities: opts.Capabilities,
		formatter:    opts.Formatter,
		lineTracker:  opts.LineTracker,
		writer:       opts.Writer,
		level:        opts.Level,
	}, nil
}

// Enabled reports whether the handler handles records at the given level.
func (h *InteractiveHandler) Enabled(_ context.Context, level slog.Level) bool {
	// Only enable if we're in an interactive environment and the level is sufficient
	return h.capabilities.IsInteractive() && level >= h.level
}

// Handle processes a log record.
func (h *InteractiveHandler) Handle(_ context.Context, r slog.Record) error {
	if !h.capabilities.IsInteractive() {
		return nil
	}

	// Create a copy of the record and apply accumulated context
	record := r.Clone()

	// Apply group context to attributes by prefixing keys
	attrs := h.attrs
	if len(h.groups) > 0 {
		// Build group prefix from all groups
		prefix := ""
		for _, group := range h.groups {
			if prefix != "" {
				prefix += "."
			}
			prefix += group
		}
		prefix += "."

		// Apply prefix to all accumulated attributes
		prefixedAttrs := make([]slog.Attr, len(attrs))
		for i, attr := range attrs {
			prefixedAttrs[i] = slog.Attr{
				Key:   prefix + attr.Key,
				Value: attr.Value,
			}
		}
		attrs = prefixedAttrs
	}

	record.AddAttrs(attrs...)

	// Format the main message
	message := h.formatter.FormatRecordWithColor(record, h.capabilities.SupportsColor())

	// Write the main message
	if _, err := h.writer.Write([]byte(message + "\n")); err != nil {
		return err
	}

	// For error-level messages, add log file hint
	if record.Level >= slog.LevelError {
		lineNum := h.lineTracker.GetCurrentLine()
		hint := h.formatter.FormatLogFileHint(lineNum, h.capabilities.SupportsColor())
		if hint != "" {
			if _, err := h.writer.Write([]byte(hint + "\n")); err != nil {
				return err
			}
		}
	}

	return nil
}

// WithAttrs returns a new handler with additional attributes.
func (h *InteractiveHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &InteractiveHandler{
		capabilities: h.capabilities,
		formatter:    h.formatter,
		lineTracker:  h.lineTracker,
		writer:       h.writer,
		level:        h.level,
		attrs:        newAttrs,
		groups:       h.groups,
	}
}

// WithGroup returns a new handler with an additional group.
func (h *InteractiveHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &InteractiveHandler{
		capabilities: h.capabilities,
		formatter:    h.formatter,
		lineTracker:  h.lineTracker,
		writer:       h.writer,
		level:        h.level,
		attrs:        h.attrs,
		groups:       newGroups,
	}
}
