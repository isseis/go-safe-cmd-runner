package security

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Static errors to satisfy err113 linter
var (
	errPasswordTest = errors.New("password=secret123 failed")
	errLongTest     = errors.New("this is a very long error message that should be truncated")
	errSecretTest   = errors.New("password=mysecret failed")
)

func TestValidator_SanitizeErrorForLogging(t *testing.T) {
	tests := []struct {
		name     string
		opts     LoggingOptions
		err      error
		expected string
	}{
		{
			name: "redacted when include details false",
			opts: LoggingOptions{
				IncludeErrorDetails: false,
			},
			err:      errPasswordTest,
			expected: "[error details redacted for security]",
		},
		{
			name: "redacts sensitive patterns",
			opts: LoggingOptions{
				IncludeErrorDetails: true,
				RedactSensitiveInfo: true,
			},
			err:      errPasswordTest,
			expected: "password=[REDACTED] failed",
		},
		{
			name: "truncates long messages",
			opts: LoggingOptions{
				IncludeErrorDetails:   true,
				MaxErrorMessageLength: 20,
			},
			err:      errLongTest,
			expected: "this is a very long ...[truncated]",
		},
		{
			name:     "nil error returns empty string",
			opts:     DefaultLoggingOptions(),
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create validator with test options
			config := DefaultConfig()
			config.LoggingOptions = tt.opts
			validator, err := NewValidator(config)
			require.NoError(t, err)

			result := validator.SanitizeErrorForLogging(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_SanitizeOutputForLogging(t *testing.T) {
	tests := []struct {
		name     string
		opts     LoggingOptions
		output   string
		expected string
	}{
		{
			name: "redacts API keys",
			opts: LoggingOptions{
				RedactSensitiveInfo: true,
			},
			output:   "API call failed: api_key=abc123def",
			expected: "API call failed: api_key=[REDACTED]",
		},
		{
			name: "truncates long output",
			opts: LoggingOptions{
				TruncateStdout:  true,
				MaxStdoutLength: 20,
			},
			output:   "this is a very long output that should be truncated for security reasons",
			expected: "this is a very long ...[truncated]",
		},
		{
			name: "handles bearer tokens",
			opts: LoggingOptions{
				RedactSensitiveInfo: true,
			},
			output:   "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: "Authorization: Bearer [REDACTED]",
		},
		{
			name:     "empty output returns empty string",
			opts:     DefaultLoggingOptions(),
			output:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create validator with test options
			config := DefaultConfig()
			config.LoggingOptions = tt.opts
			validator, err := NewValidator(config)
			require.NoError(t, err)

			result := validator.SanitizeOutputForLogging(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_CreateSafeLogFields(t *testing.T) {
	config := DefaultConfig()
	config.LoggingOptions.RedactSensitiveInfo = true
	config.LoggingOptions.IncludeErrorDetails = true // Enable error details to see redaction
	validator, err := NewValidator(config)
	require.NoError(t, err)

	fields := map[string]any{
		"command":   "curl -H 'Authorization: Bearer secret123'",
		"error":     errSecretTest,
		"exit_code": 1,
		"timeout":   "30s",
	}

	result := validator.CreateSafeLogFields(fields)

	// Check that sensitive data is redacted
	assert.Contains(t, result["command"], "Bearer [REDACTED]")
	assert.Contains(t, result["error"], "password=[REDACTED]")

	// Check that non-sensitive fields are preserved
	assert.Equal(t, 1, result["exit_code"])
	assert.Equal(t, "30s", result["timeout"])
}

func TestValidator_LogFieldsWithError(t *testing.T) {
	tests := []struct {
		name       string
		opts       LoggingOptions
		baseFields map[string]any
		err        error
		expected   map[string]any
		checkError bool // whether to check error field
	}{
		{
			name: "adds sanitized error to base fields",
			opts: LoggingOptions{
				IncludeErrorDetails: true,
				RedactSensitiveInfo: true,
			},
			baseFields: map[string]any{
				"command": "curl https://api.example.com",
				"pid":     1234,
			},
			err: errPasswordTest,
			expected: map[string]any{
				"command": "curl https://api.example.com",
				"pid":     1234,
				"error":   "password=[REDACTED] failed",
			},
			checkError: true,
		},
		{
			name: "sanitizes base fields with sensitive data",
			opts: LoggingOptions{
				IncludeErrorDetails: true,
				RedactSensitiveInfo: true,
			},
			baseFields: map[string]any{
				"command": "curl -H 'Authorization: Bearer secret123'",
				"output":  "API response: token=abc123def",
				"status":  "failed",
			},
			err: errSecretTest,
			expected: map[string]any{
				"command": "curl -H 'Authorization: Bearer [REDACTED]",
				"output":  "API response: token=[REDACTED]",
				"status":  "failed",
				"error":   "password=[REDACTED] failed",
			},
			checkError: true,
		},
		{
			name: "handles nil error gracefully",
			opts: LoggingOptions{
				IncludeErrorDetails: true,
				RedactSensitiveInfo: true,
			},
			baseFields: map[string]any{
				"command":   "echo 'hello world'",
				"exit_code": 0,
			},
			err: nil,
			expected: map[string]any{
				"command":   "echo 'hello world'",
				"exit_code": 0,
			},
			checkError: false,
		},
		{
			name: "redacts error details when disabled",
			opts: LoggingOptions{
				IncludeErrorDetails: false,
				RedactSensitiveInfo: true,
			},
			baseFields: map[string]any{
				"command": "sensitive-command",
			},
			err: errPasswordTest,
			expected: map[string]any{
				"command": "sensitive-command",
				"error":   "[error details redacted for security]",
			},
			checkError: true,
		},
		{
			name: "handles error fields in base fields",
			opts: LoggingOptions{
				IncludeErrorDetails: true,
				RedactSensitiveInfo: true,
			},
			baseFields: map[string]any{
				"command":      "test-command",
				"previous_err": errSecretTest,
				"attempt":      2,
			},
			err: errPasswordTest,
			expected: map[string]any{
				"command":      "test-command",
				"previous_err": "password=[REDACTED] failed",
				"attempt":      2,
				"error":        "password=[REDACTED] failed",
			},
			checkError: true,
		},
		{
			name: "handles empty base fields",
			opts: LoggingOptions{
				IncludeErrorDetails: true,
				RedactSensitiveInfo: false,
			},
			baseFields: map[string]any{},
			err:        errLongTest,
			expected: map[string]any{
				"error": "this is a very long error message that should be truncated",
			},
			checkError: true,
		},
		{
			name: "truncates long error messages",
			opts: LoggingOptions{
				IncludeErrorDetails:   true,
				RedactSensitiveInfo:   false,
				MaxErrorMessageLength: 20,
			},
			baseFields: map[string]any{
				"operation": "file_write",
			},
			err: errLongTest,
			expected: map[string]any{
				"operation": "file_write",
				"error":     "this is a very long ...[truncated]",
			},
			checkError: true,
		},
		{
			name: "handles mixed field types",
			opts: LoggingOptions{
				IncludeErrorDetails: true,
				RedactSensitiveInfo: true,
			},
			baseFields: map[string]any{
				"string_field": "password=secret123",
				"error_field":  errSecretTest,
				"int_field":    42,
				"bool_field":   true,
				"float_field":  3.14,
				"slice_field":  []string{"a", "b", "c"},
			},
			err: errPasswordTest,
			expected: map[string]any{
				"string_field": "password=[REDACTED]",
				"error_field":  "password=[REDACTED] failed",
				"int_field":    42,
				"bool_field":   true,
				"float_field":  3.14,
				"slice_field":  []string{"a", "b", "c"},
				"error":        "password=[REDACTED] failed",
			},
			checkError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create validator with test options
			config := DefaultConfig()
			config.LoggingOptions = tt.opts
			validator, err := NewValidator(config)
			require.NoError(t, err)

			result := validator.LogFieldsWithError(tt.baseFields, tt.err)

			// Check all expected fields
			for key, expectedValue := range tt.expected {
				actualValue, exists := result[key]
				assert.True(t, exists, "Expected field %q to exist in result", key)
				assert.Equal(t, expectedValue, actualValue, "Field %q has unexpected value", key)
			}

			// Check error field presence/absence
			if tt.checkError {
				_, hasError := result["error"]
				assert.True(t, hasError, "Expected error field to be present")
			} else {
				_, hasError := result["error"]
				assert.False(t, hasError, "Expected error field to be absent when error is nil")
			}

			// Ensure no extra fields are added beyond base fields and error
			expectedFieldCount := len(tt.baseFields)
			if tt.err != nil {
				expectedFieldCount++
			}
			assert.Equal(t, expectedFieldCount, len(result), "Result has unexpected number of fields")

			// Ensure original base fields are not modified
			if tt.baseFields != nil {
				for key, originalValue := range tt.baseFields {
					assert.Equal(t, originalValue, tt.baseFields[key], "Original base field %q was modified", key)
				}
			}
		})
	}
}
