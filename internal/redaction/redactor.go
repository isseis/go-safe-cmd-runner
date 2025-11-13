// Package redaction provides shared redaction functionality.
package redaction

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

// Config controls how sensitive information is redacted
type Config struct {
	// Placeholder is the placeholder used for redaction (e.g., "[REDACTED]")
	// Unified from LogPlaceholder and TextPlaceholder
	Placeholder string
	// Patterns contains the sensitive patterns to detect
	Patterns *SensitivePatterns
	// KeyValuePatterns contains keys for key=value or header redaction
	// e.g. ["password", "token", "Authorization: "]
	KeyValuePatterns []string
}

// DefaultConfig returns default redaction configuration
func DefaultConfig() *Config {
	return &Config{
		Placeholder:      "[REDACTED]",
		Patterns:         DefaultSensitivePatterns(),
		KeyValuePatterns: DefaultKeyValuePatterns(),
	}
}

// RedactText removes or redacts potentially sensitive information from text
func (c *Config) RedactText(text string) string {
	if text == "" {
		return text
	}

	result := text

	// Apply key=value pattern redaction
	for _, key := range c.KeyValuePatterns {
		result = c.performKeyValueRedaction(result, key, c.Placeholder)
	}

	return result
}

// RedactLogAttribute redacts sensitive information from a log attribute
func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
	key := attr.Key
	value := attr.Value

	// Check for sensitive patterns in the key
	if c.Patterns.IsSensitiveKey(key) {
		return slog.Attr{Key: key, Value: slog.StringValue(c.Placeholder)}
	}

	// Redact string values that match sensitive patterns
	if value.Kind() == slog.KindString {
		strValue := value.String()
		// First apply text-based redaction for key=value patterns within the string
		redactedText := c.RedactText(strValue)
		if redactedText != strValue {
			return slog.Attr{Key: key, Value: slog.StringValue(redactedText)}
		}
		// Then check if the entire value is sensitive (only if no key=value patterns were found)
		// This prevents strings like "password=secret" from being completely replaced with "[REDACTED]"
		if c.Patterns.IsSensitiveValue(strValue) {
			return slog.Attr{Key: key, Value: slog.StringValue(c.Placeholder)}
		}
	}

	// Handle group values recursively
	if value.Kind() == slog.KindGroup {
		groupAttrs := value.Group()
		redactedGroupAttrs := make([]slog.Attr, 0, len(groupAttrs))
		for _, groupAttr := range groupAttrs {
			redactedGroupAttrs = append(redactedGroupAttrs, c.RedactLogAttribute(groupAttr))
		}
		return slog.Attr{Key: key, Value: slog.GroupValue(redactedGroupAttrs...)}
	}

	return attr
}

// performKeyValueRedaction performs redaction on key=value patterns
func (c *Config) performKeyValueRedaction(text, key, placeholder string) string {
	if strings.Contains(key, ":") {
		// For header-like patterns such as "Authorization:" or "Authorization: "
		return c.performColonPatternRedaction(text, key, placeholder)
	}
	if strings.Contains(key, " ") {
		// For patterns like "Bearer ", "Basic " - replace the token after the space
		return c.performSpacePatternRedaction(text, key, placeholder)
	}
	// For regular key=value patterns
	return c.performKeyValuePatternRedaction(text, key, placeholder)
}

// performSpacePatternRedaction handles patterns like "Bearer ", "Basic "
func (c *Config) performSpacePatternRedaction(text, pattern, placeholder string) string {
	// Escape pattern for regex and create case-insensitive pattern
	// Match: pattern followed by one or more non-whitespace characters
	escapedPattern := regexp.QuoteMeta(pattern)
	regexPattern := `(?i)(` + escapedPattern + `)(\S+)`

	re, err := regexp.Compile(regexPattern)
	if err != nil {
		// Fallback to original text if regex compilation fails
		return text
	}

	// Replace matching tokens with pattern + placeholder
	return re.ReplaceAllStringFunc(text, func(match string) string {
		// Find the original pattern in the match (preserving case)
		patternLen := len(pattern)
		if len(match) < patternLen {
			return match
		}

		originalPattern := match[:patternLen]
		return originalPattern + placeholder
	})
}

// performColonPatternRedaction handles patterns like "Authorization:" or "Authorization: "
// It will redact everything after the pattern up to the end of line (or end of string).
func (c *Config) performColonPatternRedaction(text, pattern, placeholder string) string {
	// Escape pattern for regex and create case-insensitive pattern
	// Match: pattern + optional whitespace + optional auth scheme (Bearer/Basic) + value + line ending
	escapedPattern := regexp.QuoteMeta(pattern)
	regexPattern := `(?i)(` + escapedPattern + `)([ \t]*)((?:bearer |basic )?)[^\r\n]*`

	re, err := regexp.Compile(regexPattern)
	if err != nil {
		// Fallback to original text if regex compilation fails
		return text
	}

	// Replace matching headers with pattern + whitespace + scheme + placeholder
	return re.ReplaceAllStringFunc(text, func(match string) string {
		// Extract the original case-preserving parts using submatch
		submatches := re.FindStringSubmatch(match)
		const minSubmatchCount = 4
		if len(submatches) < minSubmatchCount {
			return match
		}

		originalPattern := submatches[1] // Original pattern preserving case
		whitespace := submatches[2]      // Whitespace after pattern
		scheme := submatches[3]          // Auth scheme (Bearer/Basic) if present

		return originalPattern + whitespace + scheme + placeholder
	})
}

// performKeyValuePatternRedaction handles patterns like "key=value"
func (c *Config) performKeyValuePatternRedaction(text, key, placeholder string) string {
	// Escape key for regex and create case-insensitive pattern
	// Match: key + optional equals sign + value (non-whitespace characters)
	escapedKey := regexp.QuoteMeta(key)
	var regexPattern string

	if strings.Contains(key, "=") {
		// Key already contains "=", match it exactly + value
		regexPattern = `(?i)(` + escapedKey + `)(\S+)`
	} else {
		// Key without "=", add it and match value
		regexPattern = `(?i)(` + escapedKey + `)(=)(\S+)`
	}

	re, err := regexp.Compile(regexPattern)
	if err != nil {
		// Fallback to original text if regex compilation fails
		return text
	}

	// Replace matching key=value pairs with key=placeholder
	return re.ReplaceAllStringFunc(text, func(match string) string {
		submatches := re.FindStringSubmatch(match)
		const minSubmatchCount = 3
		if len(submatches) < minSubmatchCount {
			return match
		}

		if strings.Contains(key, "=") {
			// Key already contains "=" (e.g., "Authorization=")
			originalKey := submatches[1] // Original key preserving case
			return originalKey + placeholder
		}
		// Key without "=" (e.g., "password")
		originalKey := submatches[1] // Original key preserving case
		equals := submatches[2]      // The "=" character
		return originalKey + equals + placeholder
	})
}

// RedactingHandler is a decorator that redacts sensitive information before forwarding to the underlying handler
type RedactingHandler struct {
	handler slog.Handler
	config  *Config
}

// NewRedactingHandler creates a new redacting handler that wraps the given handler
func NewRedactingHandler(handler slog.Handler, config *Config) *RedactingHandler {
	if config == nil {
		config = DefaultConfig()
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

// Handler returns the underlying handler
func (r *RedactingHandler) Handler() slog.Handler {
	return r.handler
}

// Handle redacts the log record and forwards it to the underlying handler
func (r *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	// Create a new record with redacted attributes
	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)

	record.Attrs(func(attr slog.Attr) bool {
		redactedAttr := r.config.RedactLogAttribute(attr)
		newRecord.AddAttrs(redactedAttr)
		return true
	})

	return r.handler.Handle(ctx, newRecord)
}

// WithAttrs returns a new RedactingHandler with the given attributes
func (r *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redactedAttrs := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redactedAttrs = append(redactedAttrs, r.config.RedactLogAttribute(attr))
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
