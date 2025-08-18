package logging

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRedactorHandler for testing redactor
type mockRedactorHandler struct {
	records []slog.Record
}

func newMockRedactorHandler() *mockRedactorHandler {
	return &mockRedactorHandler{
		records: make([]slog.Record, 0),
	}
}

func (m *mockRedactorHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (m *mockRedactorHandler) Handle(_ context.Context, r slog.Record) error {
	m.records = append(m.records, r.Clone())
	return nil
}

func (m *mockRedactorHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return m
}

func (m *mockRedactorHandler) WithGroup(_ string) slog.Handler {
	return m
}

func TestDefaultRedactionConfig(t *testing.T) {
	options := redaction.DefaultOptions()

	assert.NotNil(t, options.Patterns, "Expected patterns to be set")
	assert.NotEmpty(t, options.Patterns.CredentialPatterns, "Expected non-empty credential patterns")
}

func TestNewRedactingHandler(t *testing.T) {
	mockHandler := newMockRedactorHandler()

	// Test with custom options
	options := &redaction.Options{
		LogPlaceholder:  "***",
		TextPlaceholder: "[REDACTED]",
		Patterns:        redaction.DefaultSensitivePatterns(),
	}

	redactor := NewRedactingHandler(mockHandler, options)
	assert.NotNil(t, redactor.commonHandler, "Expected common handler to be set")

	// Test with nil options (should use default)
	redactor2 := NewRedactingHandler(mockHandler, nil)
	assert.NotNil(t, redactor2.commonHandler, "Expected common handler to be set with default options")
}

// Removed env_ prefix specific test as production no longer uses env_-prefixed attributes

func TestRedactingHandler_RedactCredentialPatterns(t *testing.T) {
	mockHandler := newMockRedactorHandler()
	options := redaction.DefaultOptions()

	redactor := NewRedactingHandler(mockHandler, options)

	tests := []struct {
		key      string
		value    string
		expected string
	}{
		{"safe_field", "safe_value", "safe_value"}, // Changed from "safe_key" to avoid "key" pattern
		{"password", "secret123", "***"},
		{"api_token", "token123", "***"},
		{"normal_field", "password123", "***"}, // value contains pattern
		{"TOKEN_FIELD", "value", "***"},        // key contains pattern (case insensitive)
	}

	for _, test := range tests {
		mockHandler.records = make([]slog.Record, 0) // Reset

		record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
		record.AddAttrs(slog.String(test.key, test.value))

		err := redactor.Handle(context.Background(), record)
		require.NoError(t, err, "Unexpected error for %s", test.key)

		require.Len(t, mockHandler.records, 1, "Expected 1 record for %s", test.key)

		handledRecord := mockHandler.records[0]
		var actualValue string
		handledRecord.Attrs(func(attr slog.Attr) bool {
			if attr.Key == test.key {
				actualValue = attr.Value.String()
			}
			return true
		})

		assert.Equal(t, test.expected, actualValue, "For key %s", test.key)
	}
}

func TestRedactingHandler_WithAttrs(t *testing.T) {
	mockHandler := newMockRedactorHandler()
	options := redaction.DefaultOptions()

	redactor := NewRedactingHandler(mockHandler, options)

	attrs := []slog.Attr{
		slog.String("safe_attr", "safe_value"),
		slog.String("secret_attr", "secret_value"),
	}

	newRedactor := redactor.WithAttrs(attrs)

	// Verify it returns a new RedactingHandler
	assert.NotSame(t, redactor, newRedactor, "WithAttrs should return a new RedactingHandler instance")

	// Verify the new handler is properly typed
	assert.IsType(t, &RedactingHandler{}, newRedactor, "WithAttrs should return a RedactingHandler")
}

func TestRedactingHandler_WithGroup(t *testing.T) {
	mockHandler := newMockRedactorHandler()
	redactor := NewRedactingHandler(mockHandler, nil)

	newRedactor := redactor.WithGroup("testgroup")

	// Verify it returns a new RedactingHandler
	assert.NotSame(t, redactor, newRedactor, "WithGroup should return a new RedactingHandler instance")

	// Verify the new handler is properly typed
	assert.IsType(t, &RedactingHandler{}, newRedactor, "WithGroup should return a RedactingHandler")
}

func TestRedactingHandler_GroupedAttributes(t *testing.T) {
	mockHandler := newMockRedactorHandler()
	options := redaction.DefaultOptions()

	redactor := NewRedactingHandler(mockHandler, options)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.Group("auth",
		slog.String("safe_field", "safe_value"),
		slog.String("password_field", "secret123"),
	))

	err := redactor.Handle(context.Background(), record)
	require.NoError(t, err, "Unexpected error")

	require.Len(t, mockHandler.records, 1, "Expected 1 record")

	// The group structure should be preserved, but sensitive values redacted
	handledRecord := mockHandler.records[0]
	var foundGroup bool
	handledRecord.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "auth" && attr.Value.Kind() == slog.KindGroup {
			foundGroup = true
			groupAttrs := attr.Value.Group()

			groupValues := make(map[string]string)
			for _, gAttr := range groupAttrs {
				groupValues[gAttr.Key] = gAttr.Value.String()
			}

			assert.Equal(t, "safe_value", groupValues["safe_field"], "Expected safe_field to be preserved in group")
			assert.Equal(t, "***", groupValues["password_field"], "Expected password_field to be redacted in group")
		}
		return true
	})

	assert.True(t, foundGroup, "Expected to find auth group in handled record")
}
