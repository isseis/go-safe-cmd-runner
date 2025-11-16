package redaction

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
)

// panickingLogValuer is a helper struct that panics when LogValue is called.
type panickingLogValuer struct{}

// LogValue implements the slog.LogValuer interface and always panics.
func (p panickingLogValuer) LogValue() slog.Value {
	panic("test panic")
}

// sensitiveLogValuer is a helper struct for testing LogValuer redaction with sensitive data.
type sensitiveLogValuer struct {
	data string
}

// LogValue implements the slog.LogValuer interface.
func (v sensitiveLogValuer) LogValue() slog.Value {
	return slog.StringValue(v.data)
}

// TestRedactText_EmptyString tests that empty strings are handled correctly
func TestRedactText_EmptyString(t *testing.T) {
	config := DefaultConfig()
	result := config.RedactText("")
	assert.Equal(t, "", result, "Empty string should return empty string")
}

// TestRedactText_NoSensitiveInfo tests that text without sensitive info is unchanged
func TestRedactText_NoSensitiveInfo(t *testing.T) {
	config := DefaultConfig()
	input := "This is a normal log message with no sensitive data"
	result := config.RedactText(input)
	assert.Equal(t, input, result, "Non-sensitive text should remain unchanged")
}

// TestRedactText_KeyValuePatterns tests key=value pattern redaction
func TestRedactText_KeyValuePatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase password",
			input:    "password=secret123",
			expected: "password=[REDACTED]",
		},
		{
			name:     "uppercase TOKEN",
			input:    "TOKEN=abc",
			expected: "TOKEN=[REDACTED]",
		},
		{
			name:     "mixed case preserving",
			input:    "Password=test",
			expected: "Password=[REDACTED]",
		},
		{
			name:     "multiple key=value pairs",
			input:    "user=john password=secret token=abc123",
			expected: "user=john password=[REDACTED] token=[REDACTED]",
		},
		{
			name:     "key with equals in pattern",
			input:    "Set-Cookie: sessionid=xyz123",
			expected: "Set-Cookie: sessionid=xyz123", // Not matched by default patterns
		},
		{
			name:     "api_key pattern",
			input:    "api_key=1234567890abcdef",
			expected: "api_key=[REDACTED]",
		},
		{
			name:     "secret pattern",
			input:    "secret=my-secret-value",
			expected: "secret=[REDACTED]",
		},
		{
			name:     "key pattern",
			input:    "key=some-key-value",
			expected: "key=[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactText_SpacePatterns tests Bearer/Basic authentication pattern redaction
func TestRedactText_SpacePatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Bearer token",
			input:    "Bearer token123",
			expected: "Bearer [REDACTED]",
		},
		{
			name:     "Basic auth",
			input:    "Basic dGVzdA==",
			expected: "Basic [REDACTED]",
		},
		{
			name:     "lowercase bearer",
			input:    "bearer mytoken456",
			expected: "bearer [REDACTED]",
		},
		{
			name:     "mixed case Basic",
			input:    "BaSiC encoded123",
			expected: "BaSiC [REDACTED]",
		},
		{
			name:     "multiple Bearer tokens",
			input:    "Bearer abc123 and Bearer xyz789",
			expected: "Bearer [REDACTED] and Bearer [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactText_ColonPatterns tests Authorization header pattern redaction
func TestRedactText_ColonPatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Authorization header with Bearer",
			input:    "Authorization: Bearer token123",
			expected: "Authorization: Bearer [REDACTED]",
		},
		{
			name:     "Authorization header no space (not in default patterns)",
			input:    "Authorization:token456",
			expected: "Authorization:token456", // Not matched by default patterns which only include "Authorization: " with space
		},
		{
			name:     "lowercase authorization with Basic",
			input:    "authorization: Basic dGVzdA==",
			expected: "authorization: Basic [REDACTED]",
		},
		{
			name:     "mixed case Authorization",
			input:    "AuThOrIzAtIoN: bearer secret",
			expected: "AuThOrIzAtIoN: bearer [REDACTED]",
		},
		{
			name:     "multiline headers",
			input:    "Authorization: Bearer abc\nContent-Type: application/json",
			expected: "Authorization: Bearer [REDACTED]\nContent-Type: application/json",
		},
		{
			name:     "Authorization with tabs",
			input:    "Authorization:\t\tBearer secret123",
			expected: "Authorization:\t\tBearer [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactText_MixedPatterns tests multiple different patterns in same text
func TestRedactText_MixedPatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "mixed key=value and Bearer",
			input:    "password=secret Bearer token123",
			expected: "password=[REDACTED] Bearer [REDACTED]",
		},
		{
			name:     "all pattern types",
			input:    "token=abc Bearer xyz Authorization: Basic dGVzdA==",
			expected: "token=[REDACTED] Bearer [REDACTED] Authorization: Basic [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactText_SpecialCharacters tests handling of special characters in keys
func TestRedactText_SpecialCharacters(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "underscore in value",
			input:    "password=my_secret_123",
			expected: "password=[REDACTED]",
		},
		{
			name:     "hyphen in value",
			input:    "token=abc-def-123",
			expected: "token=[REDACTED]",
		},
		{
			name:     "equals in value (stops at space)",
			input:    "key=value=with=equals next=token",
			expected: "key=[REDACTED] next=token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactLogAttribute_SensitiveKeys tests redaction of sensitive key names
func TestRedactLogAttribute_SensitiveKeys(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "password key",
			key:      "password",
			value:    "secret123",
			expected: "[REDACTED]",
		},
		{
			name:     "token key",
			key:      "token",
			value:    "abc123",
			expected: "[REDACTED]",
		},
		{
			name:     "api_key",
			key:      "api_key",
			value:    "xyz",
			expected: "[REDACTED]",
		},
		{
			name:     "secret key",
			key:      "secret",
			value:    "mysecret",
			expected: "[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: slog.StringValue(tt.value)}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.expected, result.Value.String())
		})
	}
}

// TestRedactLogAttribute_NormalKeys tests that normal keys are preserved
func TestRedactLogAttribute_NormalKeys(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "username",
			key:   "username",
			value: "john",
		},
		{
			name:  "message",
			key:   "message",
			value: "hello world",
		},
		{
			name:  "count",
			key:   "count",
			value: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: slog.StringValue(tt.value)}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.value, result.Value.String())
		})
	}
}

// TestRedactLogAttribute_SensitiveValues tests redaction based on value content
func TestRedactLogAttribute_SensitiveValues(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "value contains 'password'",
			key:      "field",
			value:    "my_password_123",
			expected: "[REDACTED]",
		},
		{
			name:     "value contains 'token'",
			key:      "data",
			value:    "bearer_token_xyz",
			expected: "[REDACTED]",
		},
		{
			name:     "normal value",
			key:      "field",
			value:    "normal_value",
			expected: "normal_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: slog.StringValue(tt.value)}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.expected, result.Value.String())
		})
	}
}

// TestRedactLogAttribute_GroupValues tests nested group handling
func TestRedactLogAttribute_GroupValues(t *testing.T) {
	config := DefaultConfig()

	t.Run("simple group with sensitive data", func(t *testing.T) {
		innerAttrs := []slog.Attr{
			{Key: "password", Value: slog.StringValue("secret")},
			{Key: "username", Value: slog.StringValue("john")},
		}
		attr := slog.Attr{Key: "credentials", Value: slog.GroupValue(innerAttrs...)}

		result := config.RedactLogAttribute(attr)
		assert.Equal(t, "credentials", result.Key)
		assert.Equal(t, slog.KindGroup, result.Value.Kind())

		groupAttrs := result.Value.Group()
		require.Len(t, groupAttrs, 2)
		assert.Equal(t, "password", groupAttrs[0].Key)
		assert.Equal(t, "[REDACTED]", groupAttrs[0].Value.String())
		assert.Equal(t, "username", groupAttrs[1].Key)
		assert.Equal(t, "john", groupAttrs[1].Value.String())
	})

	t.Run("nested groups", func(t *testing.T) {
		deepInnerAttrs := []slog.Attr{
			{Key: "token", Value: slog.StringValue("abc123")},
		}
		innerAttrs := []slog.Attr{
			{Key: "auth", Value: slog.GroupValue(deepInnerAttrs...)},
			{Key: "id", Value: slog.StringValue("123")},
		}
		attr := slog.Attr{Key: "request", Value: slog.GroupValue(innerAttrs...)}

		result := config.RedactLogAttribute(attr)
		assert.Equal(t, "request", result.Key)

		groupAttrs := result.Value.Group()
		require.Len(t, groupAttrs, 2)

		// Check nested group
		authGroup := groupAttrs[0]
		assert.Equal(t, "auth", authGroup.Key)
		assert.Equal(t, slog.KindGroup, authGroup.Value.Kind())

		authAttrs := authGroup.Value.Group()
		require.Len(t, authAttrs, 1)
		assert.Equal(t, "token", authAttrs[0].Key)
		assert.Equal(t, "[REDACTED]", authAttrs[0].Value.String())
	})
}

// TestRedactLogAttribute_NonStringValues tests that non-string values are preserved
func TestRedactLogAttribute_NonStringValues(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name  string
		key   string
		value slog.Value
	}{
		{
			name:  "integer value",
			key:   "count",
			value: slog.IntValue(42),
		},
		{
			name:  "boolean value",
			key:   "enabled",
			value: slog.BoolValue(true),
		},
		{
			name:  "float value",
			key:   "ratio",
			value: slog.Float64Value(3.14),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: tt.value}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.value.Kind(), result.Value.Kind())
			assert.Equal(t, tt.value, result.Value)
		})
	}
}

// mockHandler is a simple mock implementation of slog.Handler for testing
type mockHandler struct {
	enabled      bool
	records      []slog.Record
	attrs        []slog.Attr
	groups       []string
	enabledLevel slog.Level
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		enabled:      true,
		records:      make([]slog.Record, 0),
		attrs:        make([]slog.Attr, 0),
		groups:       make([]string, 0),
		enabledLevel: slog.LevelInfo,
	}
}

func (m *mockHandler) Enabled(_ context.Context, level slog.Level) bool {
	return m.enabled && level >= m.enabledLevel
}

func (m *mockHandler) Handle(_ context.Context, record slog.Record) error {
	m.records = append(m.records, record)
	return nil
}

func (m *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := &mockHandler{
		enabled:      m.enabled,
		records:      m.records,
		attrs:        append(m.attrs, attrs...),
		groups:       m.groups,
		enabledLevel: m.enabledLevel,
	}
	return newHandler
}

func (m *mockHandler) WithGroup(name string) slog.Handler {
	newHandler := &mockHandler{
		enabled:      m.enabled,
		records:      m.records,
		attrs:        m.attrs,
		groups:       append(m.groups, name),
		enabledLevel: m.enabledLevel,
	}
	return newHandler
}

// TestNewRedactingHandler tests handler creation
func TestNewRedactingHandler(t *testing.T) {
	t.Run("with custom config", func(t *testing.T) {
		mock := newMockHandler()
		config := DefaultConfig()
		handler := NewRedactingHandler(mock, config, nil)

		assert.NotNil(t, handler)
		assert.Equal(t, mock, handler.handler)
		assert.Equal(t, config, handler.config)
	})

	t.Run("with nil config uses default", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, nil, nil)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler.config)
		assert.Equal(t, "[REDACTED]", handler.config.Placeholder)
	})
}

// TestRedactingHandler_Enabled tests Enabled method
func TestRedactingHandler_Enabled(t *testing.T) {
	mock := newMockHandler()
	mock.enabledLevel = slog.LevelWarn
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	ctx := context.Background()

	assert.False(t, handler.Enabled(ctx, slog.LevelDebug))
	assert.False(t, handler.Enabled(ctx, slog.LevelInfo))
	assert.True(t, handler.Enabled(ctx, slog.LevelWarn))
	assert.True(t, handler.Enabled(ctx, slog.LevelError))
}

// TestRedactingHandler_Handler tests Handler getter
func TestRedactingHandler_Handler(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	assert.Equal(t, mock, handler.Handler())
}

// TestRedactingHandler_Handle tests log record handling with redaction
func TestRedactingHandler_Handle(t *testing.T) {
	t.Run("redacts sensitive attributes", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		ctx := context.Background()
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
		record.AddAttrs(
			slog.String("username", "john"),
			slog.String("password", "secret123"),
		)

		err := handler.Handle(ctx, record)
		require.NoError(t, err)
		require.Len(t, mock.records, 1)

		handledRecord := mock.records[0]
		attrs := make([]slog.Attr, 0)
		handledRecord.Attrs(func(attr slog.Attr) bool {
			attrs = append(attrs, attr)
			return true
		})

		require.Len(t, attrs, 2)
		assert.Equal(t, "username", attrs[0].Key)
		assert.Equal(t, "john", attrs[0].Value.String())
		assert.Equal(t, "password", attrs[1].Key)
		assert.Equal(t, "[REDACTED]", attrs[1].Value.String())
	})

	t.Run("preserves record metadata", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		ctx := context.Background()
		originalTime := time.Now()
		record := slog.NewRecord(originalTime, slog.LevelWarn, "warning message", 123)

		err := handler.Handle(ctx, record)
		require.NoError(t, err)
		require.Len(t, mock.records, 1)

		handledRecord := mock.records[0]
		assert.Equal(t, originalTime, handledRecord.Time)
		assert.Equal(t, slog.LevelWarn, handledRecord.Level)
		assert.Equal(t, "warning message", handledRecord.Message)
		assert.Equal(t, uintptr(123), handledRecord.PC)
	})
}

// TestRedactingHandler_WithAttrs tests attribute addition with redaction
func TestRedactingHandler_WithAttrs(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	newAttrs := []slog.Attr{
		slog.String("token", "abc123"),
		slog.String("user", "alice"),
	}

	newHandler := handler.WithAttrs(newAttrs)

	// Should return a new RedactingHandler
	redactingHandler, ok := newHandler.(*RedactingHandler)
	require.True(t, ok)

	// Check that underlying handler received redacted attributes
	underlyingMock, ok := redactingHandler.handler.(*mockHandler)
	require.True(t, ok)
	require.Len(t, underlyingMock.attrs, 2)

	assert.Equal(t, "token", underlyingMock.attrs[0].Key)
	assert.Equal(t, "[REDACTED]", underlyingMock.attrs[0].Value.String())
	assert.Equal(t, "user", underlyingMock.attrs[1].Key)
	assert.Equal(t, "alice", underlyingMock.attrs[1].Value.String())

	// Original handler should be unchanged
	originalMock, ok := handler.handler.(*mockHandler)
	require.True(t, ok)
	assert.Len(t, originalMock.attrs, 0)
}

// TestRedactingHandler_WithAttrs_LogValuer tests WithAttrs with LogValuer attributes
func TestRedactingHandler_WithAttrs_LogValuer(t *testing.T) {
	tests := []struct {
		name           string
		attr           slog.Attr
		expectedKey    string
		expectedValue  string
		expectRedacted bool
	}{
		{
			name:           "LogValuer with sensitive key",
			attr:           slog.Any("token", sensitiveLogValuer{data: "secret123"}),
			expectedKey:    "token",
			expectedValue:  "[REDACTED]",
			expectRedacted: true,
		},
		{
			name:           "LogValuer with non-sensitive key",
			attr:           slog.Any("user", sensitiveLogValuer{data: "alice"}),
			expectedKey:    "user",
			expectedValue:  "alice",
			expectRedacted: false,
		},
		{
			name:           "LogValuer returning sensitive value",
			attr:           slog.Any("data", sensitiveLogValuer{data: "password=secret"}),
			expectedKey:    "data",
			expectedValue:  "password=[REDACTED]",
			expectRedacted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockHandler()
			handler := NewRedactingHandler(mock, DefaultConfig(), nil)

			newHandler := handler.WithAttrs([]slog.Attr{tt.attr})

			// Should return a new RedactingHandler
			redactingHandler, ok := newHandler.(*RedactingHandler)
			require.True(t, ok)

			// Check that underlying handler received redacted attributes
			underlyingMock, ok := redactingHandler.handler.(*mockHandler)
			require.True(t, ok)
			require.Len(t, underlyingMock.attrs, 1)

			assert.Equal(t, tt.expectedKey, underlyingMock.attrs[0].Key)
			assert.Equal(t, tt.expectedValue, underlyingMock.attrs[0].Value.String())
		})
	}
}

// TestRedactingHandler_WithAttrs_Slice tests WithAttrs with slice attributes
func TestRedactingHandler_WithAttrs_Slice(t *testing.T) {
	t.Run("slice with LogValuer elements", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		// Create a slice with LogValuer elements
		newHandler := handler.WithAttrs([]slog.Attr{
			slog.Any("users", []slog.LogValuer{
				sensitiveLogValuer{data: "alice"},
				sensitiveLogValuer{data: "bob"},
			}),
		})

		// Should return a new RedactingHandler
		redactingHandler, ok := newHandler.(*RedactingHandler)
		require.True(t, ok)

		// Check that underlying handler received the attribute
		underlyingMock, ok := redactingHandler.handler.(*mockHandler)
		require.True(t, ok)
		require.Len(t, underlyingMock.attrs, 1)

		assert.Equal(t, "users", underlyingMock.attrs[0].Key)

		// Check that the slice was processed - it should be []any now
		sliceValue := underlyingMock.attrs[0].Value.Any()
		require.NotNil(t, sliceValue)

		sliceAny, ok := sliceValue.([]any)
		require.True(t, ok, "expected []any after processing LogValuer slice, got %T", sliceValue)
		assert.Len(t, sliceAny, 2)
	})

	t.Run("slice with sensitive LogValuer - key based redaction", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		// Use "token" as the key which is sensitive
		newHandler := handler.WithAttrs([]slog.Attr{
			slog.Any("token", []slog.LogValuer{
				sensitiveLogValuer{data: "token123"},
			}),
		})

		// Should return a new RedactingHandler
		redactingHandler, ok := newHandler.(*RedactingHandler)
		require.True(t, ok)

		// Check that underlying handler received the attribute
		underlyingMock, ok := redactingHandler.handler.(*mockHandler)
		require.True(t, ok)
		require.Len(t, underlyingMock.attrs, 1)

		assert.Equal(t, "token", underlyingMock.attrs[0].Key)
		// The entire attribute should be redacted because "token" is a sensitive key
		assert.Equal(t, "[REDACTED]", underlyingMock.attrs[0].Value.String())
	})
}

// TestRedactingHandler_WithAttrs_PanicRecovery tests panic recovery in WithAttrs
func TestRedactingHandler_WithAttrs_PanicRecovery(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	// Should not panic even if LogValue() panics
	newHandler := handler.WithAttrs([]slog.Attr{
		slog.Any("panic_attr", panickingLogValuer{}),
	})

	// Should return a new RedactingHandler
	redactingHandler, ok := newHandler.(*RedactingHandler)
	require.True(t, ok)

	// Check that underlying handler received safe placeholder
	underlyingMock, ok := redactingHandler.handler.(*mockHandler)
	require.True(t, ok)
	require.Len(t, underlyingMock.attrs, 1)

	assert.Equal(t, "panic_attr", underlyingMock.attrs[0].Key)
	assert.Equal(t, RedactionFailurePlaceholder, underlyingMock.attrs[0].Value.String())
}

// TestRedactingHandler_WithGroup tests group creation
func TestRedactingHandler_WithGroup(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	newHandler := handler.WithGroup("request")

	// Should return a new RedactingHandler
	redactingHandler, ok := newHandler.(*RedactingHandler)
	require.True(t, ok)

	// Check that underlying handler has the group
	underlyingMock, ok := redactingHandler.handler.(*mockHandler)
	require.True(t, ok)
	require.Len(t, underlyingMock.groups, 1)
	assert.Equal(t, "request", underlyingMock.groups[0])

	// Original handler should be unchanged
	originalMock, ok := handler.handler.(*mockHandler)
	require.True(t, ok)
	assert.Len(t, originalMock.groups, 0)
}

// TestPerformKeyValueRedaction tests the routing logic
func TestPerformKeyValueRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		text        string
		key         string
		placeholder string
		expected    string
	}{
		{
			name:        "colon pattern",
			text:        "Authorization: Bearer token",
			key:         "Authorization: ",
			placeholder: "[REDACTED]",
			expected:    "Authorization: Bearer [REDACTED]",
		},
		{
			name:        "space pattern",
			text:        "Bearer token123",
			key:         "Bearer ",
			placeholder: "[REDACTED]",
			expected:    "Bearer [REDACTED]",
		},
		{
			name:        "key=value pattern",
			text:        "password=secret",
			key:         "password",
			placeholder: "[REDACTED]",
			expected:    "password=[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.performKeyValueRedaction(tt.text, tt.key, tt.placeholder)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPerformSpacePatternRedaction tests space pattern handling details
func TestPerformSpacePatternRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		text        string
		pattern     string
		placeholder string
		expected    string
	}{
		{
			name:        "simple Bearer",
			text:        "Bearer abc123",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "Bearer ***",
		},
		{
			name:        "case insensitive",
			text:        "bearer token",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "bearer ***",
		},
		{
			name:        "preserves original case",
			text:        "BeArEr secret",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "BeArEr ***",
		},
		{
			name:        "multiple occurrences",
			text:        "Bearer abc Bearer xyz",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "Bearer *** Bearer ***",
		},
		{
			name:        "no match returns original",
			text:        "no match here",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "no match here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.performSpacePatternRedaction(tt.text, tt.pattern, tt.placeholder)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPerformColonPatternRedaction tests colon pattern handling details
func TestPerformColonPatternRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		text        string
		pattern     string
		placeholder string
		expected    string
	}{
		{
			name:        "with Bearer scheme",
			text:        "Authorization: Bearer token123",
			pattern:     "Authorization: ",
			placeholder: "***",
			expected:    "Authorization: Bearer ***",
		},
		{
			name:        "with Basic scheme",
			text:        "Authorization: Basic dGVzdA==",
			pattern:     "Authorization: ",
			placeholder: "***",
			expected:    "Authorization: Basic ***",
		},
		{
			name:        "no scheme",
			text:        "Authorization: token123",
			pattern:     "Authorization: ",
			placeholder: "***",
			expected:    "Authorization: ***",
		},
		{
			name:        "no space after colon",
			text:        "Authorization:token",
			pattern:     "Authorization:",
			placeholder: "***",
			expected:    "Authorization:***",
		},
		{
			name:        "with space after colon",
			text:        "Authorization: token",
			pattern:     "Authorization:",
			placeholder: "***",
			expected:    "Authorization: ***",
		},
		{
			name:        "case insensitive pattern",
			text:        "authorization: bearer secret",
			pattern:     "Authorization: ",
			placeholder: "***",
			expected:    "authorization: bearer ***",
		},
		{
			name:        "preserves whitespace",
			text:        "Authorization:\t\tBearer token",
			pattern:     "Authorization:",
			placeholder: "***",
			expected:    "Authorization:\t\tBearer ***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.performColonPatternRedaction(tt.text, tt.pattern, tt.placeholder)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPerformKeyValuePatternRedaction tests key=value pattern handling details
func TestPerformKeyValuePatternRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		text        string
		key         string
		placeholder string
		expected    string
	}{
		{
			name:        "simple key=value",
			text:        "password=secret",
			key:         "password",
			placeholder: "***",
			expected:    "password=***",
		},
		{
			name:        "key with equals",
			text:        "Authorization=Bearer token",
			key:         "Authorization=",
			placeholder: "***",
			expected:    "Authorization=*** token", // Only "Bearer" is redacted, " token" remains
		},
		{
			name:        "case insensitive",
			text:        "PASSWORD=secret",
			key:         "password",
			placeholder: "***",
			expected:    "PASSWORD=***",
		},
		{
			name:        "preserves case",
			text:        "PaSsWoRd=test",
			key:         "password",
			placeholder: "***",
			expected:    "PaSsWoRd=***",
		},
		{
			name:        "multiple matches",
			text:        "password=abc token=xyz password=def",
			key:         "password",
			placeholder: "***",
			expected:    "password=*** token=xyz password=***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.performKeyValuePatternRedaction(tt.text, tt.key, tt.placeholder)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactLogAttribute_StringWithKeyValuePatterns tests that log attributes containing
// key=value patterns in their string values are properly redacted
func TestRedactLogAttribute_StringWithKeyValuePatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "stdout with password",
			key:      "stdout",
			value:    "Connecting with password=secret123 to server",
			expected: "Connecting with password=[REDACTED] to server",
		},
		{
			name:     "stderr with token",
			key:      "stderr",
			value:    "Error: token=abc123 is invalid",
			expected: "Error: token=[REDACTED] is invalid",
		},
		{
			name:     "output with Bearer token",
			key:      "output",
			value:    "Authorization: Bearer token123",
			expected: "Authorization: Bearer [REDACTED]",
		},
		{
			name:     "output with multiple secrets",
			key:      "message",
			value:    "password=pass123 api_key=key456 normal=value",
			expected: "password=[REDACTED] api_key=[REDACTED] normal=value",
		},
		{
			name:     "normal output without secrets",
			key:      "stdout",
			value:    "Build completed successfully",
			expected: "Build completed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: slog.StringValue(tt.value)}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.expected, result.Value.String())
		})
	}
}

// TestRedactingHandler_LogValuerSingle tests redaction of a single LogValuer
func TestRedactingHandler_LogValuerSingle(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test data with a LogValuer that contains sensitive information
	testValuer := sensitiveLogValuer{data: "password=secret123"}

	// Execute
	logger.Info("Command executed", "result", testValuer)

	// Verify the sensitive data is redacted
	output := buf.String()
	assert.Contains(t, output, "password=[REDACTED]")
	assert.NotContains(t, output, "secret123")
}

// commandResultMock is a helper struct for testing LogValuer with CommandResult-like data.
type commandResultMock struct {
	Name   string
	Output string
}

// LogValue implements the slog.LogValuer interface.
func (c commandResultMock) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("name", c.Name),
		slog.String("output", c.Output),
	)
}

// Test LogValuer with actual CommandResult-like struct
func TestRedactingHandler_LogValuerWithCommandResult(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Create a LogValuer that returns sensitive data
	result := commandResultMock{
		Name:   "test_cmd",
		Output: "password=secret123 and token=abc456",
	}

	// Log the CommandResult using LogValuer interface
	logger.Info("Command executed", "result", result)

	// Verify
	output := buf.String()
	assert.Contains(t, output, "password=[REDACTED]")
	assert.Contains(t, output, "token=[REDACTED]")
	assert.NotContains(t, output, "secret123")
	assert.NotContains(t, output, "abc456")
}

func TestRedactingHandler_CommandResults_Integration(t *testing.T) {
	tests := []struct {
		name     string
		results  common.CommandResults
		validate func(t *testing.T, output string)
	}{
		{
			name: "redact password in output",
			results: common.CommandResults{
				{CommandResultFields: common.CommandResultFields{
					Name:     "setup",
					ExitCode: 0,
					Output:   "Database password=secret123 configured",
					Stderr:   "",
				}},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "[REDACTED]")
				assert.NotContains(t, output, "secret123")
				assert.Contains(t, output, "Database")
				assert.Contains(t, output, "configured")
			},
		},
		{
			name: "redact multiple sensitive fields",
			results: common.CommandResults{
				{CommandResultFields: common.CommandResultFields{
					Name:     "deploy",
					ExitCode: 0,
					Output:   "API key=sk-1234567890abcdef set",
					Stderr:   "",
				}},
				{CommandResultFields: common.CommandResultFields{
					Name:     "configure",
					ExitCode: 0,
					Output:   "",
					Stderr:   "Warning: token=ghp_xxxxxxxxxxxx expired",
				}},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "[REDACTED]")
				assert.NotContains(t, output, "sk-1234567890abcdef")
				assert.NotContains(t, output, "ghp_xxxxxxxxxxxx")
				assert.Contains(t, output, "API")
				assert.Contains(t, output, "Warning")
			},
		},
		{
			name: "preserve non-sensitive output",
			results: common.CommandResults{
				{CommandResultFields: common.CommandResultFields{
					Name:     "test",
					ExitCode: 0,
					Output:   "All tests passed",
					Stderr:   "",
				}},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "All tests passed")
				assert.NotContains(t, output, "[REDACTED]")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := slog.NewJSONHandler(&buf, nil)
			config := DefaultConfig()
			redactingHandler := NewRedactingHandler(handler, config, nil)
			logger := slog.New(redactingHandler)

			logger.Info("test",
				slog.String(common.GroupSummaryAttrs.Status, "success"),
				slog.String(common.GroupSummaryAttrs.Group, "test_group"),
				slog.Any(common.GroupSummaryAttrs.Commands, tt.results),
			)

			output := buf.String()
			tt.validate(t, output)
		})
	}
}

// TestRedactingHandler_DeepRecursion tests recursion depth limiting
func TestRedactingHandler_DeepRecursion(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Create deeply nested group structure (depth > 10)
	var createNestedGroup func(int) slog.Value
	createNestedGroup = func(depth int) slog.Value {
		if depth == 0 {
			return slog.StringValue("leaf_value")
		}
		return slog.GroupValue(
			slog.Attr{Key: "level", Value: slog.Int64Value(int64(depth))},
			slog.Attr{Key: "nested", Value: createNestedGroup(depth - 1)},
		)
	}

	// Create structure with depth 15
	deepGroup := createNestedGroup(15)

	// Execute
	logger.Info("Deep structure", "data", deepGroup)

	// Verify: Should handle without panic
	// The depth limit prevents infinite recursion
	output := buf.String()
	assert.Contains(t, output, "level")
}

// TestRedactingHandler_PanicHandling tests our panic recovery when logging a panicking LogValuer
func TestRedactingHandler_PanicHandling(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()

	// Create failure logger to capture panic logs
	var failureBuf bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureBuf, nil)
	failureLogger := slog.New(failureHandler)

	redactingHandler := NewRedactingHandler(handler, config, failureLogger)
	logger := slog.New(redactingHandler)

	// Execute with a LogValuer that is designed to panic.
	// Note: The `panickingLogValuer` type with its `LogValue()` method
	// is defined at the top level of this test file.
	//
	// Our RedactingHandler now handles KindLogValuer and catches panics,
	// replacing them with RedactionFailurePlaceholder and logging to failureLogger.
	logger.Info("Test message", "data", panickingLogValuer{})

	// Verify main log contains placeholder and not the panic message
	output := buf.String()
	assert.Contains(t, output, RedactionFailurePlaceholder)
	assert.NotContains(t, output, "test panic")
	assert.NotContains(t, output, "LogValue panicked")

	// Verify failure log contains detailed panic info
	failureOutput := failureBuf.String()
	assert.Contains(t, failureOutput, "Redaction failed - detailed log")
	assert.Contains(t, failureOutput, "test panic")
	assert.Contains(t, failureOutput, "panic_value")
	assert.Contains(t, failureOutput, "panic_type")
	assert.Contains(t, failureOutput, "stack_trace")
}

// TestRedactingHandler_PanicInProcessKindAny tests our custom panic recovery
// when we manually process KindAny LogValuers (e.g., in groups)
func TestRedactingHandler_PanicInProcessKindAny(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()

	// Create failure logger to capture panic logs
	var failureBuf bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureBuf, nil)
	failureLogger := slog.New(failureHandler)

	redactingHandler := NewRedactingHandler(handler, config, failureLogger)

	// Create a KindAny attribute with an unresolved panicking LogValuer
	// We use slog.Attr directly with AnyValue to avoid premature resolution
	attr := slog.Attr{
		Key:   "test_data",
		Value: slog.AnyValue(panickingLogValuer{}),
	}

	// Process the attribute through WithAttrs public API
	// This will trigger our panic recovery code in processLogValuer()
	handlerWithAttrs := redactingHandler.WithAttrs([]slog.Attr{attr})
	logger := slog.New(handlerWithAttrs)

	// Log a message to trigger the attribute rendering
	logger.Info("Test message")

	// Verify main log contains placeholder
	output := buf.String()
	assert.Contains(t, output, "Test message")
	assert.Contains(t, output, RedactionFailurePlaceholder)
	assert.NotContains(t, output, "test panic")

	// Verify failure log contains detailed panic info
	failureOutput := failureBuf.String()
	assert.Contains(t, failureOutput, "Redaction failed - detailed log")
	assert.Contains(t, failureOutput, "test panic")
	assert.Contains(t, failureOutput, "panic_value")
	assert.Contains(t, failureOutput, "panic_type")
	assert.Contains(t, failureOutput, "stack_trace")
}

// TestRedactingHandler_NilValue tests nil value handling
func TestRedactingHandler_NilValue(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test with nil value
	logger.Info("Test message", "data", slog.AnyValue(nil))

	// Verify: Should handle nil gracefully
	output := buf.String()
	assert.Contains(t, output, "Test message")
}

// TestRedactingHandler_EmptySlice tests empty slice handling
func TestRedactingHandler_EmptySlice(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test with empty slice
	emptySlice := []string{}
	logger.Info("Test message", "data", slog.AnyValue(emptySlice))

	// Verify: Should handle empty slice gracefully
	output := buf.String()
	assert.Contains(t, output, "Test message")
}

// TestRedactingHandler_MixedSlice tests slice with mixed types
func TestRedactingHandler_MixedSlice(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test with mixed slice (interfaces)
	mixedSlice := []interface{}{
		"string_value",
		123,
		true,
	}
	logger.Info("Test message", "data", slog.AnyValue(mixedSlice))

	// Verify: Should handle mixed slice gracefully
	output := buf.String()
	assert.Contains(t, output, "Test message")
	assert.Contains(t, output, "string_value")
}

// TestRedactingHandler_NonLogValuer tests non-LogValuer types pass through
func TestRedactingHandler_NonLogValuer(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test with various non-LogValuer types
	logger.Info("Test message",
		"int", slog.IntValue(123),
		"bool", slog.BoolValue(true),
		"float", slog.Float64Value(3.14),
	)

	// Verify: Should pass through without modification
	output := buf.String()
	assert.Contains(t, output, "123")
	assert.Contains(t, output, "true")
	assert.Contains(t, output, "3.14")
}

// TestRedactionContext_DepthTracking tests depth tracking
func TestRedactionContext_DepthTracking(t *testing.T) {
	ctx1 := redactionContext{depth: 0}
	assert.Equal(t, 0, ctx1.depth)

	ctx2 := redactionContext{depth: 5}
	assert.Equal(t, 5, ctx2.depth)

	// Test depth limit
	assert.True(t, ctx2.depth < maxRedactionDepth)

	ctxLimit := redactionContext{depth: maxRedactionDepth}
	assert.Equal(t, maxRedactionDepth, ctxLimit.depth)
}

// TestRedactionFailurePlaceholder tests the failure placeholder constant
func TestRedactionFailurePlaceholder(t *testing.T) {
	assert.Equal(t, "[REDACTION FAILED - OUTPUT SUPPRESSED]", RedactionFailurePlaceholder)
	assert.NotEqual(t, "[REDACTED]", RedactionFailurePlaceholder)
}

// TestMaxRedactionDepth tests the depth limit constant
func TestMaxRedactionDepth(t *testing.T) {
	assert.Equal(t, 10, maxRedactionDepth)
	assert.True(t, maxRedactionDepth > 0)
}

// TestRedactingHandler_SliceTypeConversion tests and documents the type conversion behavior
// for slices processed by the redacting handler
func TestRedactingHandler_SliceTypeConversion(t *testing.T) {
	t.Run("typed slice without LogValuer converts to []any", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)
		logger := slog.New(handler)

		// Test with a typed slice that has no LogValuer elements
		stringSlice := []string{"alice", "bob", "charlie"}
		logger.Info("Test message", "users", slog.AnyValue(stringSlice))

		// Verify: Even without LogValuer, processSlice converts to []any
		require.Len(t, mock.records, 1)
		record := mock.records[0]

		var usersAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "users" {
				usersAttr = attr
				return false
			}
			return true
		})

		// ALL slices are processed and converted to []any
		sliceValue := usersAttr.Value.Any()
		anySlice, ok := sliceValue.([]any)
		assert.True(t, ok, "Expected []any after processSlice, got %T", sliceValue)
		assert.Len(t, anySlice, 3)

		// Verify semantic content is preserved
		assert.Equal(t, "alice", anySlice[0])
		assert.Equal(t, "bob", anySlice[1])
		assert.Equal(t, "charlie", anySlice[2])
	})

	t.Run("slice with LogValuer converts to []any", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)
		logger := slog.New(handler)

		// Test with a slice containing LogValuer elements
		logValuerSlice := []slog.LogValuer{
			sensitiveLogValuer{data: "alice"},
			sensitiveLogValuer{data: "bob"},
		}
		logger.Info("Test message", "users", slog.AnyValue(logValuerSlice))

		// Verify: Should convert to []any after processing
		require.Len(t, mock.records, 1)
		record := mock.records[0]

		var usersAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "users" {
				usersAttr = attr
				return false
			}
			return true
		})

		// After processSlice, the type should be []any
		sliceValue := usersAttr.Value.Any()
		anySlice, ok := sliceValue.([]any)
		assert.True(t, ok, "Expected []any after processing LogValuer slice, got %T", sliceValue)
		assert.Len(t, anySlice, 2)

		// Verify that the semantic content is preserved
		// (even though the type changed from []slog.LogValuer to []any)
		assert.Equal(t, "alice", anySlice[0])
		assert.Equal(t, "bob", anySlice[1])
	})

	t.Run("mixed slice type conversion", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)
		logger := slog.New(handler)

		// Test with interface slice containing some LogValuers
		mixedSlice := []interface{}{
			sensitiveLogValuer{data: "alice"},
			"plain_string",
			123,
		}
		logger.Info("Test message", "data", slog.AnyValue(mixedSlice))

		// Verify: []interface{} is similar to []any, should handle gracefully
		require.Len(t, mock.records, 1)
		record := mock.records[0]

		var dataAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "data" {
				dataAttr = attr
				return false
			}
			return true
		})

		sliceValue := dataAttr.Value.Any()
		anySlice, ok := sliceValue.([]any)
		assert.True(t, ok, "Expected []any for processed mixed slice, got %T", sliceValue)
		assert.Len(t, anySlice, 3)

		// First element was LogValuer -> resolved to its string value
		assert.Equal(t, "alice", anySlice[0])
		// Other elements preserved as-is
		assert.Equal(t, "plain_string", anySlice[1])
		assert.Equal(t, 123, anySlice[2])
	})
}

// TestRedactingHandler_TwoTierLogging tests that panic handling produces
// two log entries: detailed (to failureLogger) and summary (to slog.Default)
func TestRedactingHandler_TwoTierLogging(t *testing.T) {
	var mainBuf bytes.Buffer
	mainHandler := slog.NewJSONHandler(&mainBuf, nil)
	config := DefaultConfig()

	// Create failure logger (simulates file/stderr, excludes Slack)
	var failureBuf bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureBuf, nil)
	failureLogger := slog.New(failureHandler)

	// Create redacting handler
	redactingHandler := NewRedactingHandler(mainHandler, config, failureLogger)
	logger := slog.New(redactingHandler)

	// Set this logger as default so slog.Warn() in panic handler works
	oldDefault := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(oldDefault)

	// Trigger panic in LogValuer
	logger.Info("Test message", "data", panickingLogValuer{})

	// Parse main output
	mainLines := strings.Split(strings.TrimSpace(mainBuf.String()), "\n")
	require.GreaterOrEqual(t, len(mainLines), 2, "Expected at least 2 log entries (placeholder + summary)")

	// Parse failure output
	failureLines := strings.Split(strings.TrimSpace(failureBuf.String()), "\n")
	require.GreaterOrEqual(t, len(failureLines), 1, "Expected at least 1 detailed log entry")

	// Verify detailed log (in failureLogger)
	var detailedLog map[string]interface{}
	err := json.Unmarshal([]byte(failureLines[0]), &detailedLog)
	require.NoError(t, err)

	assert.Equal(t, "Redaction failed - detailed log", detailedLog["msg"])
	assert.Contains(t, detailedLog, "panic_value")
	assert.Contains(t, detailedLog, "panic_type")
	assert.Contains(t, detailedLog, "stack_trace")
	assert.Equal(t, "redaction_failure_detail", detailedLog["log_category"])

	// Verify summary log (in main logger via slog.Default)
	// Find the summary log in main output
	var summaryLog map[string]interface{}
	for _, line := range mainLines {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			if msg, ok := entry["msg"].(string); ok && strings.Contains(msg, "see logs for details") {
				summaryLog = entry
				break
			}
		}
	}

	require.NotNil(t, summaryLog, "Expected to find summary log in main output")
	assert.Equal(t, "Redaction failed - see logs for details", summaryLog["msg"])
	assert.Contains(t, summaryLog, "panic_type")
	assert.Equal(t, "redaction_failure_summary", summaryLog["log_category"])
	assert.Equal(t, true, summaryLog["details_in_log"])

	// Verify sensitive information is NOT in summary
	assert.NotContains(t, summaryLog, "panic_value")
	assert.NotContains(t, summaryLog, "stack_trace")
}

// TestContainsRedactingHandler tests the containsRedactingHandler helper function
func TestContainsRedactingHandler(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() slog.Handler
		expected bool
	}{
		{
			name: "nil handler",
			setup: func() slog.Handler {
				return nil
			},
			expected: false,
		},
		{
			name: "simple text handler without RedactingHandler",
			setup: func() slog.Handler {
				return slog.NewTextHandler(os.Stderr, nil)
			},
			expected: false,
		},
		{
			name: "simple JSON handler without RedactingHandler",
			setup: func() slog.Handler {
				return slog.NewJSONHandler(os.Stderr, nil)
			},
			expected: false,
		},
		{
			name: "direct RedactingHandler",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				return NewRedactingHandler(baseHandler, nil, nil)
			},
			expected: true,
		},
		{
			name: "RedactingHandler wrapped in another RedactingHandler",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redacting1 := NewRedactingHandler(baseHandler, nil, nil)
				return NewRedactingHandler(redacting1, nil, nil)
			},
			expected: true,
		},
		{
			name: "RedactingHandler accessed via Handler() method",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redacting := NewRedactingHandler(baseHandler, nil, nil)
				// The Handler() method should expose the underlying handler
				return redacting
			},
			expected: true,
		},
		{
			name: "MultiHandler without RedactingHandler",
			setup: func() slog.Handler {
				textHandler := slog.NewTextHandler(os.Stderr, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(textHandler, jsonHandler)
				require.NoError(t, err)
				return multiHandler
			},
			expected: false,
		},
		{
			name: "MultiHandler with RedactingHandler in first position",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(redactingHandler, jsonHandler)
				require.NoError(t, err)
				return multiHandler
			},
			expected: true,
		},
		{
			name: "MultiHandler with RedactingHandler in middle position",
			setup: func() slog.Handler {
				textHandler := slog.NewTextHandler(os.Stderr, nil)
				baseHandler := slog.NewJSONHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				anotherTextHandler := slog.NewTextHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(textHandler, redactingHandler, anotherTextHandler)
				require.NoError(t, err)
				return multiHandler
			},
			expected: true,
		},
		{
			name: "MultiHandler with RedactingHandler in last position",
			setup: func() slog.Handler {
				textHandler := slog.NewTextHandler(os.Stderr, nil)
				baseHandler := slog.NewJSONHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				multiHandler, err := logging.NewMultiHandler(textHandler, redactingHandler)
				require.NoError(t, err)
				return multiHandler
			},
			expected: true,
		},
		{
			name: "MultiHandler with nested RedactingHandler",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redacting1 := NewRedactingHandler(baseHandler, nil, nil)
				redacting2 := NewRedactingHandler(redacting1, nil, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(jsonHandler, redacting2)
				require.NoError(t, err)
				return multiHandler
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.setup()
			result := containsRedactingHandler(handler)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNewRedactingHandler_FailureLoggerValidation tests that NewRedactingHandler
// panics when failureLogger contains a RedactingHandler in its chain
func TestNewRedactingHandler_FailureLoggerValidation(t *testing.T) {
	tests := []struct {
		name        string
		setupLogger func() *slog.Logger
		expectPanic bool
	}{
		{
			name: "failureLogger without RedactingHandler - no panic",
			setupLogger: func() *slog.Logger {
				// Create a simple logger without RedactingHandler
				handler := slog.NewTextHandler(os.Stderr, nil)
				return slog.New(handler)
			},
			expectPanic: false,
		},
		{
			name: "failureLogger with RedactingHandler - panic expected",
			setupLogger: func() *slog.Logger {
				// Create a logger with RedactingHandler in the chain
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				return slog.New(redactingHandler)
			},
			expectPanic: true,
		},
		{
			name: "failureLogger with nested RedactingHandler - panic expected",
			setupLogger: func() *slog.Logger {
				// Create a logger with nested RedactingHandler
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redacting1 := NewRedactingHandler(baseHandler, nil, nil)
				redacting2 := NewRedactingHandler(redacting1, nil, nil)
				return slog.New(redacting2)
			},
			expectPanic: true,
		},
		{
			name: "nil failureLogger (uses default) - no panic in this specific case",
			setupLogger: func() *slog.Logger {
				return nil
			},
			expectPanic: false,
		},
		{
			name: "failureLogger with MultiHandler containing RedactingHandler - panic expected",
			setupLogger: func() *slog.Logger {
				// Create a MultiHandler that contains a RedactingHandler
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(jsonHandler, redactingHandler)
				require.NoError(t, err)
				return slog.New(multiHandler)
			},
			expectPanic: true,
		},
		{
			name: "failureLogger with MultiHandler without RedactingHandler - no panic",
			setupLogger: func() *slog.Logger {
				// Create a MultiHandler without RedactingHandler
				textHandler := slog.NewTextHandler(os.Stderr, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(textHandler, jsonHandler)
				require.NoError(t, err)
				return slog.New(multiHandler)
			},
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseHandler := slog.NewTextHandler(os.Stderr, nil)
			failureLogger := tt.setupLogger()

			if tt.expectPanic {
				// Expect a panic
				assert.Panics(t, func() {
					NewRedactingHandler(baseHandler, nil, failureLogger)
				}, "Expected NewRedactingHandler to panic with RedactingHandler in failureLogger chain")
			} else {
				// Should not panic
				assert.NotPanics(t, func() {
					NewRedactingHandler(baseHandler, nil, failureLogger)
				}, "NewRedactingHandler should not panic with valid failureLogger")
			}
		})
	}
}

// TestProductionLoggerSetup verifies that the production logger setup
// (as used in internal/runner/bootstrap/logger.go) does not violate
// the constraint that failureLogger must not contain RedactingHandler
func TestProductionLoggerSetup(t *testing.T) {
	// Simulate the production setup from internal/runner/bootstrap/logger.go

	// 1. Create base handlers (text and JSON)
	textHandler := slog.NewTextHandler(os.Stderr, nil)
	jsonHandler := slog.NewJSONHandler(os.Stderr, nil)

	// 2. Create failureLogger from base handlers (NO RedactingHandler)
	failureHandlers := []slog.Handler{textHandler, jsonHandler}
	failureMultiHandler, err := logging.NewMultiHandler(failureHandlers...)
	require.NoError(t, err)
	failureLogger := slog.New(failureMultiHandler)

	// 3. Verify failureLogger does not contain RedactingHandler
	assert.False(t, containsRedactingHandler(failureLogger.Handler()),
		"Production failureLogger should not contain RedactingHandler")

	// 4. Create main handler with RedactingHandler
	// Should not panic with valid failureLogger
	mainHandler, err := logging.NewMultiHandler(textHandler, jsonHandler)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		_ = NewRedactingHandler(mainHandler, nil, failureLogger)
	}, "Production setup should not panic - failureLogger is correctly configured without RedactingHandler")
}

// BenchmarkRedactingHandler_String benchmarks RedactingHandler with simple string attributes
func BenchmarkRedactingHandler_String(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	timestamp := time.Now().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message",
			"user", "testuser",
			"action", "login",
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactingHandler_String_WithSensitiveData benchmarks with sensitive data redaction
func BenchmarkRedactingHandler_String_WithSensitiveData(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	timestamp := time.Now().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message",
			"user", "testuser",
			"credentials", "password=secret123 token=abc456",
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactingHandler_LogValuer benchmarks RedactingHandler with LogValuer attributes
func BenchmarkRedactingHandler_LogValuer(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	// Create LogValuer with sensitive data
	valuer := sensitiveLogValuer{data: "password=secret123"}
	timestamp := time.Now().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message",
			"user", "testuser",
			"data", valuer,
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactingHandler_Slice benchmarks RedactingHandler with slice attributes
func BenchmarkRedactingHandler_Slice(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	// Create slice of LogValuers with sensitive data
	slice := []slog.LogValuer{
		sensitiveLogValuer{data: "password=secret1"},
		sensitiveLogValuer{data: "token=secret2"},
		sensitiveLogValuer{data: "api_key=secret3"},
	}
	timestamp := time.Now().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message",
			"user", "testuser",
			"items", slice,
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactingHandler_Mixed benchmarks RedactingHandler with mixed attribute types
func BenchmarkRedactingHandler_Mixed(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	valuer := sensitiveLogValuer{data: "password=secret123"}
	slice := []slog.LogValuer{
		sensitiveLogValuer{data: "token=abc"},
		sensitiveLogValuer{data: "api_key=xyz"},
	}
	timestamp := time.Now().String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message",
			"user", "testuser",
			"simple_string", "normal data",
			"sensitive_string", "password=mypass",
			"logvaluer", valuer,
			"slice", slice,
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactText benchmarks the RedactText function
func BenchmarkRedactText(b *testing.B) {
	config := DefaultConfig()
	text := "User logged in with password=secret123 and token=abc456xyz"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.RedactText(text)
	}
}

// BenchmarkRedactText_NoSensitiveData benchmarks RedactText with non-sensitive data
func BenchmarkRedactText_NoSensitiveData(b *testing.B) {
	config := DefaultConfig()
	text := "User logged in successfully at 2024-01-01 12:00:00"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.RedactText(text)
	}
}

// BenchmarkRedactLogAttribute_String benchmarks RedactLogAttribute with string values
func BenchmarkRedactLogAttribute_String(b *testing.B) {
	config := DefaultConfig()
	attr := slog.String("message", "password=secret123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.RedactLogAttribute(attr)
	}
}

// BenchmarkRedactLogAttribute_Group benchmarks RedactLogAttribute with group values
func BenchmarkRedactLogAttribute_Group(b *testing.B) {
	config := DefaultConfig()
	attr := slog.Group("user",
		slog.String("name", "testuser"),
		slog.String("password", "secret123"),
		slog.String("token", "abc456"),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.RedactLogAttribute(attr)
	}
}

// BenchmarkRedactLogAttribute_Any_LogValuer benchmarks RedactLogAttribute with LogValuer
func BenchmarkRedactLogAttribute_Any_LogValuer(b *testing.B) {
	config := DefaultConfig()
	valuer := sensitiveLogValuer{data: "password=secret123"}
	attr := slog.Any("data", valuer)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.RedactLogAttribute(attr)
	}
}
