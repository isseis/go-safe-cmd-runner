package logging

import (
	"context"
	"log/slog"
	"regexp"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// RedactionConfig contains configuration for redacting sensitive information
// Deprecated: Use common.RedactionOptions instead
type RedactionConfig struct {
	// AllowedEnvKeys contains environment variable keys that are allowed in cleartext
	AllowedEnvKeys []string
	// CredentialPatterns contains regex patterns to match credentials that should be redacted
	// Deprecated: Use common.SensitivePatterns instead
	CredentialPatterns []*regexp.Regexp
}

// DefaultRedactionConfig returns a default redaction configuration
// Deprecated: Use common.DefaultRedactionOptions instead
func DefaultRedactionConfig() *RedactionConfig {
	patterns := common.DefaultSensitivePatterns()

	// Convert allowed env vars map to slice for backward compatibility
	allowedEnv := make([]string, 0, len(patterns.AllowedEnvVars))
	for key := range patterns.AllowedEnvVars {
		allowedEnv = append(allowedEnv, key)
	}

	return &RedactionConfig{
		AllowedEnvKeys:     allowedEnv,
		CredentialPatterns: patterns.CredentialPatterns,
	}
}

// RedactingHandler is a decorator that redacts sensitive information before forwarding to the underlying handler
type RedactingHandler struct {
	handler slog.Handler
	config  *RedactionConfig
	// Use the new common redacting handler internally
	commonHandler *common.RedactingHandler
}

// NewRedactingHandler creates a new redacting handler that wraps the given handler
func NewRedactingHandler(handler slog.Handler, config *RedactionConfig) *RedactingHandler {
	if config == nil {
		config = DefaultRedactionConfig()
	}

	// Create options for the new common handler
	options := common.DefaultRedactionOptions()

	return &RedactingHandler{
		handler:       handler,
		config:        config,
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
	// Create a new handler with attrs applied to the underlying handler
	newUnderlyingHandler := r.handler.WithAttrs(attrs)

	// Create new common handler with the new underlying handler
	options := common.DefaultRedactionOptions()
	newCommonHandler := common.NewRedactingHandler(newUnderlyingHandler, options)

	return &RedactingHandler{
		handler:       newUnderlyingHandler,
		config:        r.config,
		commonHandler: newCommonHandler,
	}
}

// WithGroup returns a new RedactingHandler with the given group name
func (r *RedactingHandler) WithGroup(name string) slog.Handler {
	// Create a new handler with group applied to the underlying handler
	newUnderlyingHandler := r.handler.WithGroup(name)

	// Create new common handler with the new underlying handler
	options := common.DefaultRedactionOptions()
	newCommonHandler := common.NewRedactingHandler(newUnderlyingHandler, options)

	return &RedactingHandler{
		handler:       newUnderlyingHandler,
		config:        r.config,
		commonHandler: newCommonHandler,
	}
}
