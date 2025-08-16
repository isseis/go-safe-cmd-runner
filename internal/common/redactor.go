// Package common provides shared redaction functionality.
package common

import (
	"context"
	"log/slog"
	"strings"
)

// RedactionOptions controls how sensitive information is redacted
type RedactionOptions struct {
	// LogPlaceholder is the placeholder used for log redaction (e.g., "***")
	LogPlaceholder string
	// TextPlaceholder is the placeholder used for text redaction (e.g., "[REDACTED]")
	TextPlaceholder string
	// Patterns contains the sensitive patterns to detect
	Patterns *SensitivePatterns
	// KeyValuePatterns contains keys for key=value or header redaction
	// e.g. ["password", "token", "Authorization: "]
	KeyValuePatterns []string
}

// DefaultRedactionOptions returns default redaction options
func DefaultRedactionOptions() *RedactionOptions {
	return &RedactionOptions{
		LogPlaceholder:   "***",
		TextPlaceholder:  "[REDACTED]",
		Patterns:         DefaultSensitivePatterns(),
		KeyValuePatterns: DefaultKeyValuePatterns(),
	}
}

// RedactText removes or redacts potentially sensitive information from text
func (ro *RedactionOptions) RedactText(text string) string {
	if text == "" {
		return text
	}

	result := text

	// Apply key=value pattern redaction
	for _, key := range ro.KeyValuePatterns {
		result = ro.performKeyValueRedaction(result, key, ro.TextPlaceholder)
	}

	return result
}

// RedactLogAttribute redacts sensitive information from a log attribute
func (ro *RedactionOptions) RedactLogAttribute(attr slog.Attr) slog.Attr {
	key := attr.Key
	value := attr.Value

	// Check for sensitive patterns in the key
	if ro.Patterns.IsSensitiveKey(key) {
		return slog.Attr{Key: key, Value: slog.StringValue(ro.LogPlaceholder)}
	}

	// Redact string values that match sensitive patterns
	if value.Kind() == slog.KindString {
		strValue := value.String()
		if ro.Patterns.IsSensitiveValue(strValue) {
			return slog.Attr{Key: key, Value: slog.StringValue(ro.LogPlaceholder)}
		}
	}

	// Handle group values recursively
	if value.Kind() == slog.KindGroup {
		groupAttrs := value.Group()
		redactedGroupAttrs := make([]slog.Attr, 0, len(groupAttrs))
		for _, groupAttr := range groupAttrs {
			redactedGroupAttrs = append(redactedGroupAttrs, ro.RedactLogAttribute(groupAttr))
		}
		return slog.Attr{Key: key, Value: slog.GroupValue(redactedGroupAttrs...)}
	}

	return attr
}

// performKeyValueRedaction performs redaction on key=value patterns
func (ro *RedactionOptions) performKeyValueRedaction(text, key, placeholder string) string {
	if strings.Contains(key, ":") {
		// For header-like patterns such as "Authorization:" or "Authorization: "
		return ro.performColonPatternRedaction(text, key, placeholder)
	}
	if strings.Contains(key, " ") {
		// For patterns like "Bearer ", "Basic " - replace the token after the space
		return ro.performSpacePatternRedaction(text, key, placeholder)
	}
	// For regular key=value patterns
	return ro.performKeyValuePatternRedaction(text, key, placeholder)
}

// performSpacePatternRedaction handles patterns like "Bearer ", "Basic "
func (ro *RedactionOptions) performSpacePatternRedaction(text, pattern, placeholder string) string {
	lowerPattern := strings.ToLower(pattern)
	result := text
	searchStart := 0

	for {
		lowerResult := strings.ToLower(result[searchStart:])
		relativeIdx := strings.Index(lowerResult, lowerPattern)
		if relativeIdx == -1 {
			break
		}

		startIdx := searchStart + relativeIdx
		originalPattern := result[startIdx : startIdx+len(pattern)]

		// Replace the token after the pattern
		replacement := originalPattern + placeholder
		valueStart := startIdx + len(pattern)
		valueEnd := valueStart

		// Find the end of the token (space, newline, or end of string)
		for valueEnd < len(result) && result[valueEnd] != ' ' && result[valueEnd] != '\n' && result[valueEnd] != '\t' {
			valueEnd++
		}

		result = result[:startIdx] + replacement + result[valueEnd:]
		searchStart = startIdx + len(replacement)
	}

	return result
}

// performColonPatternRedaction handles patterns like "Authorization:" or "Authorization: "
// It will redact everything after the pattern up to the end of line (or end of string).
func (ro *RedactionOptions) performColonPatternRedaction(text, pattern, placeholder string) string {
	lowerPattern := strings.ToLower(pattern)
	result := text
	searchStart := 0

	for {
		lowerResult := strings.ToLower(result[searchStart:])
		relativeIdx := strings.Index(lowerResult, lowerPattern)
		if relativeIdx == -1 {
			break
		}

		startIdx := searchStart + relativeIdx
		originalPattern := result[startIdx : startIdx+len(pattern)]

		// Determine start of the header value
		valueStart := startIdx + len(originalPattern)

		// Preserve any whitespace after the colon/pattern
		wsStart := valueStart
		for wsStart < len(result) && (result[wsStart] == ' ' || result[wsStart] == '\t') {
			wsStart++
		}

		// If the value starts with an auth scheme like Bearer/Basic, preserve the scheme
		scheme := ""
		lowerAfterWS := strings.ToLower(result[wsStart:])
		switch {
		case strings.HasPrefix(lowerAfterWS, "bearer "):
			scheme = result[wsStart : wsStart+len("Bearer ")]
		case strings.HasPrefix(lowerAfterWS, "basic "):
			scheme = result[wsStart : wsStart+len("Basic ")]
		}

		// Compute the end of the header value (end-of-line)
		valueEnd := wsStart
		for valueEnd < len(result) && result[valueEnd] != '\n' && result[valueEnd] != '\r' {
			valueEnd++
		}

		// Build replacement keeping original pattern, whitespace, optional scheme, then placeholder
		replacement := originalPattern + result[valueStart:wsStart] + scheme + placeholder

		result = result[:startIdx] + replacement + result[valueEnd:]
		searchStart = startIdx + len(replacement)
	}

	return result
}

// performKeyValuePatternRedaction handles patterns like "key=value"
func (ro *RedactionOptions) performKeyValuePatternRedaction(text, key, placeholder string) string {
	lowerKey := strings.ToLower(key)
	result := text
	searchStart := 0

	for {
		lowerResult := strings.ToLower(result[searchStart:])
		relativeIdx := strings.Index(lowerResult, lowerKey)
		if relativeIdx == -1 {
			break
		}

		startIdx := searchStart + relativeIdx
		originalKey := result[startIdx : startIdx+len(key)]

		// For regular key=value patterns
		keyPattern := originalKey
		if !strings.Contains(keyPattern, "=") {
			keyPattern += "="
		}
		replacement := keyPattern + placeholder

		valueStart := startIdx + len(keyPattern)
		valueEnd := valueStart
		for valueEnd < len(result) && result[valueEnd] != ' ' && result[valueEnd] != '\n' && result[valueEnd] != '\t' {
			valueEnd++
		}

		result = result[:startIdx] + replacement + result[valueEnd:]
		searchStart = startIdx + len(replacement)
	}

	return result
}

// RedactingHandler is a decorator that redacts sensitive information before forwarding to the underlying handler
type RedactingHandler struct {
	handler slog.Handler
	options *RedactionOptions
}

// NewRedactingHandler creates a new redacting handler that wraps the given handler
func NewRedactingHandler(handler slog.Handler, options *RedactionOptions) *RedactingHandler {
	if options == nil {
		options = DefaultRedactionOptions()
	}
	return &RedactingHandler{
		handler: handler,
		options: options,
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
		redactedAttr := r.options.RedactLogAttribute(attr)
		newRecord.AddAttrs(redactedAttr)
		return true
	})

	return r.handler.Handle(ctx, newRecord)
}

// WithAttrs returns a new RedactingHandler with the given attributes
func (r *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redactedAttrs := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redactedAttrs = append(redactedAttrs, r.options.RedactLogAttribute(attr))
	}
	return &RedactingHandler{
		handler: r.handler.WithAttrs(redactedAttrs),
		options: r.options,
	}
}

// WithGroup returns a new RedactingHandler with the given group name
func (r *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{
		handler: r.handler.WithGroup(name),
		options: r.options,
	}
}
