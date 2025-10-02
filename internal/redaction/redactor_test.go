package redaction

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			expected: "***",
		},
		{
			name:     "token key",
			key:      "token",
			value:    "abc123",
			expected: "***",
		},
		{
			name:     "api_key",
			key:      "api_key",
			value:    "xyz",
			expected: "***",
		},
		{
			name:     "secret key",
			key:      "secret",
			value:    "mysecret",
			expected: "***",
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
			expected: "***",
		},
		{
			name:     "value contains 'token'",
			key:      "data",
			value:    "bearer_token_xyz",
			expected: "***",
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
		assert.Equal(t, "***", groupAttrs[0].Value.String())
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
		assert.Equal(t, "***", authAttrs[0].Value.String())
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
		handler := NewRedactingHandler(mock, config)

		assert.NotNil(t, handler)
		assert.Equal(t, mock, handler.handler)
		assert.Equal(t, config, handler.config)
	})

	t.Run("with nil config uses default", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, nil)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler.config)
		assert.Equal(t, "***", handler.config.LogPlaceholder)
		assert.Equal(t, "[REDACTED]", handler.config.TextPlaceholder)
	})
}

// TestRedactingHandler_Enabled tests Enabled method
func TestRedactingHandler_Enabled(t *testing.T) {
	mock := newMockHandler()
	mock.enabledLevel = slog.LevelWarn
	handler := NewRedactingHandler(mock, DefaultConfig())

	ctx := context.Background()

	assert.False(t, handler.Enabled(ctx, slog.LevelDebug))
	assert.False(t, handler.Enabled(ctx, slog.LevelInfo))
	assert.True(t, handler.Enabled(ctx, slog.LevelWarn))
	assert.True(t, handler.Enabled(ctx, slog.LevelError))
}

// TestRedactingHandler_Handler tests Handler getter
func TestRedactingHandler_Handler(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig())

	assert.Equal(t, mock, handler.Handler())
}

// TestRedactingHandler_Handle tests log record handling with redaction
func TestRedactingHandler_Handle(t *testing.T) {
	t.Run("redacts sensitive attributes", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig())

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
		assert.Equal(t, "***", attrs[1].Value.String())
	})

	t.Run("preserves record metadata", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig())

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
	handler := NewRedactingHandler(mock, DefaultConfig())

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
	assert.Equal(t, "***", underlyingMock.attrs[0].Value.String())
	assert.Equal(t, "user", underlyingMock.attrs[1].Key)
	assert.Equal(t, "alice", underlyingMock.attrs[1].Value.String())

	// Original handler should be unchanged
	originalMock, ok := handler.handler.(*mockHandler)
	require.True(t, ok)
	assert.Len(t, originalMock.attrs, 0)
}

// TestRedactingHandler_WithGroup tests group creation
func TestRedactingHandler_WithGroup(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig())

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
