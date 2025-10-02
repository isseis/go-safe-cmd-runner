//go:build test

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
			expected: "this is a very long ...[truncated for security]",
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
