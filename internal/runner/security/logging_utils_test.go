package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityValidator_SanitizeErrorForLogging(t *testing.T) {
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
			name:     "nil error",
			opts:     DefaultLoggingOptions(),
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create security validator with the test options
			config := DefaultConfig()
			config.LoggingOptions = tt.opts
			validator, err := NewValidator(config)
			require.NoError(t, err)

			result := validator.SanitizeErrorForLogging(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSecurityValidator_SanitizeOutputForLogging(t *testing.T) {
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
			// Create security validator with the test options
			config := DefaultConfig()
			config.LoggingOptions = tt.opts
			validator, err := NewValidator(config)
			require.NoError(t, err)

			result := validator.SanitizeOutputForLogging(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSecurityValidator_CreateSafeLogFields(t *testing.T) {
	config := DefaultConfig()
	config.LoggingOptions.RedactSensitiveInfo = true
	config.LoggingOptions.IncludeErrorDetails = true // Enable error details to see redaction
	config.LoggingOptions.MaxStdoutLength = 100      // Increase length to see full redacted output
	config.LoggingOptions.TruncateStdout = true

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

func TestDefaultLoggingOptions(t *testing.T) {
	opts := DefaultLoggingOptions()

	// Verify secure defaults
	assert.False(t, opts.IncludeErrorDetails, "Default should not include error details for security")
	assert.True(t, opts.RedactSensitiveInfo, "Default should redact sensitive info")
	assert.True(t, opts.TruncateStdout, "Default should truncate stdout")
	assert.Greater(t, opts.MaxErrorMessageLength, 0, "Should have reasonable error message limit")
	assert.Greater(t, opts.MaxStdoutLength, 0, "Should have reasonable stdout limit")
}

func TestVerboseLoggingOptions(t *testing.T) {
	opts := VerboseLoggingOptions()

	// Verify verbose settings
	assert.True(t, opts.IncludeErrorDetails, "Verbose should include error details")
	assert.True(t, opts.RedactSensitiveInfo, "Even verbose should redact sensitive info")
	assert.True(t, opts.TruncateStdout, "Even verbose should truncate stdout")

	// Verify higher limits
	defaultOpts := DefaultLoggingOptions()
	assert.Greater(t, opts.MaxErrorMessageLength, defaultOpts.MaxErrorMessageLength)
	assert.Greater(t, opts.MaxStdoutLength, defaultOpts.MaxStdoutLength)
}
