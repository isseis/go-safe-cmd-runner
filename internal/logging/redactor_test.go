package logging

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
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

	if options.Patterns == nil {
		t.Error("Expected patterns to be set")
	}
	if len(options.Patterns.CredentialPatterns) == 0 {
		t.Error("Expected non-empty credential patterns")
	}
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
	if redactor.commonHandler == nil {
		t.Error("Expected common handler to be set")
	}

	// Test with nil options (should use default)
	redactor2 := NewRedactingHandler(mockHandler, nil)
	if redactor2.commonHandler == nil {
		t.Error("Expected common handler to be set with default options")
	}
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
		if err != nil {
			t.Errorf("Unexpected error for %s: %v", test.key, err)
			continue
		}

		if len(mockHandler.records) != 1 {
			t.Errorf("Expected 1 record for %s, got %d", test.key, len(mockHandler.records))
			continue
		}

		handledRecord := mockHandler.records[0]
		var actualValue string
		handledRecord.Attrs(func(attr slog.Attr) bool {
			if attr.Key == test.key {
				actualValue = attr.Value.String()
			}
			return true
		})

		if actualValue != test.expected {
			t.Errorf("For key %s, expected %s, got %s", test.key, test.expected, actualValue)
		}
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
	if newRedactor == redactor {
		t.Error("WithAttrs should return a new RedactingHandler instance")
	}

	// Verify the new handler is properly typed
	if _, ok := newRedactor.(*RedactingHandler); !ok {
		t.Error("WithAttrs should return a RedactingHandler")
	}
}

func TestRedactingHandler_WithGroup(t *testing.T) {
	mockHandler := newMockRedactorHandler()
	redactor := NewRedactingHandler(mockHandler, nil)

	newRedactor := redactor.WithGroup("testgroup")

	// Verify it returns a new RedactingHandler
	if newRedactor == redactor {
		t.Error("WithGroup should return a new RedactingHandler instance")
	}

	// Verify the new handler is properly typed
	if _, ok := newRedactor.(*RedactingHandler); !ok {
		t.Error("WithGroup should return a RedactingHandler")
	}
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
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(mockHandler.records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(mockHandler.records))
	}

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

			if groupValues["safe_field"] != "safe_value" {
				t.Errorf("Expected safe_field to be preserved in group, got %s", groupValues["safe_field"])
			}
			if groupValues["password_field"] != "***" {
				t.Errorf("Expected password_field to be redacted in group, got %s", groupValues["password_field"])
			}
		}
		return true
	})

	if !foundGroup {
		t.Error("Expected to find auth group in handled record")
	}
}
