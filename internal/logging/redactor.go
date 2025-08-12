package logging

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

// RedactionConfig contains configuration for redacting sensitive information
type RedactionConfig struct {
	// AllowedEnvKeys contains environment variable keys that are allowed in cleartext
	AllowedEnvKeys []string
	// CredentialPatterns contains regex patterns to match credentials that should be redacted
	CredentialPatterns []*regexp.Regexp
}

// DefaultRedactionConfig returns a default redaction configuration
func DefaultRedactionConfig() *RedactionConfig {
	// Common credential patterns
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|token|secret|key)`),
		regexp.MustCompile(`(?i)aws_access_key_id`),
		regexp.MustCompile(`(?i)aws_secret_access_key`),
		regexp.MustCompile(`(?i)aws_session_token`),
		regexp.MustCompile(`(?i)google_application_credentials`),
		regexp.MustCompile(`(?i)gcp_service_account_key`),
	}

	// Common safe environment variables
	allowedEnv := []string{
		"PATH", "HOME", "USER", "LANG", "SHELL", "TERM",
		"PWD", "OLDPWD", "HOSTNAME", "LOGNAME", "TZ",
	}

	return &RedactionConfig{
		AllowedEnvKeys:     allowedEnv,
		CredentialPatterns: patterns,
	}
}

// RedactingHandler is a decorator that redacts sensitive information before forwarding to the underlying handler
type RedactingHandler struct {
	handler slog.Handler
	config  *RedactionConfig
}

// NewRedactingHandler creates a new redacting handler that wraps the given handler
func NewRedactingHandler(handler slog.Handler, config *RedactionConfig) *RedactingHandler {
	if config == nil {
		config = DefaultRedactionConfig()
	}
	return &RedactingHandler{
		handler: handler,
		config:  config,
	}
}

// Enabled reports whether the handler handles records at the given level
func (r *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return r.handler.Enabled(ctx, level)
}

// Handle redacts the log record and forwards it to the underlying handler
func (r *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	// Create a new record with redacted attributes
	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)

	record.Attrs(func(attr slog.Attr) bool {
		redactedAttr := r.redactAttr(attr)
		newRecord.AddAttrs(redactedAttr)
		return true
	})

	return r.handler.Handle(ctx, newRecord)
}

// WithAttrs returns a new RedactingHandler with the given attributes
func (r *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redactedAttrs := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redactedAttrs = append(redactedAttrs, r.redactAttr(attr))
	}
	return &RedactingHandler{
		handler: r.handler.WithAttrs(redactedAttrs),
		config:  r.config,
	}
}

// WithGroup returns a new RedactingHandler with the given group name
func (r *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{
		handler: r.handler.WithGroup(name),
		config:  r.config,
	}
}

// redactAttr redacts sensitive information from a single attribute
func (r *RedactingHandler) redactAttr(attr slog.Attr) slog.Attr {
	key := attr.Key
	value := attr.Value

	// Check if this is an environment variable that should be redacted
	if strings.HasPrefix(key, "env_") {
		envKey := strings.TrimPrefix(key, "env_")
		if !r.isAllowedEnvKey(envKey) {
			return slog.Attr{Key: key, Value: slog.StringValue("***")}
		}
	}

	// Check for credential patterns in the key
	for _, pattern := range r.config.CredentialPatterns {
		if pattern.MatchString(key) {
			return slog.Attr{Key: key, Value: slog.StringValue("***")}
		}
	}

	// Redact string values that match credential patterns
	if value.Kind() == slog.KindString {
		strValue := value.String()
		for _, pattern := range r.config.CredentialPatterns {
			if pattern.MatchString(strValue) {
				return slog.Attr{Key: key, Value: slog.StringValue("***")}
			}
		}
	}

	// Handle group values recursively
	if value.Kind() == slog.KindGroup {
		groupAttrs := value.Group()
		redactedGroupAttrs := make([]slog.Attr, 0, len(groupAttrs))
		for _, groupAttr := range groupAttrs {
			redactedGroupAttrs = append(redactedGroupAttrs, r.redactAttr(groupAttr))
		}
		return slog.Attr{Key: key, Value: slog.GroupValue(redactedGroupAttrs...)}
	}

	return attr
}

// isAllowedEnvKey checks if an environment variable key is in the allowed list
func (r *RedactingHandler) isAllowedEnvKey(key string) bool {
	for _, allowedKey := range r.config.AllowedEnvKeys {
		if strings.EqualFold(key, allowedKey) {
			return true
		}
	}
	return false
}
