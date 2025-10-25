package debug

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPrintFinalEnvironment_WithOrigins tests that PrintFinalEnvironment correctly uses the origins map
func TestPrintFinalEnvironment_WithOrigins(t *testing.T) {
	envVars := map[string]string{
		"HOME":       "/home/test",
		"PATH":       "/usr/bin:/bin",
		"GLOBAL_VAR": "global_value",
		"GROUP_VAR":  "group_value",
		"CMD_VAR":    "cmd_value",
	}

	origins := map[string]string{
		"HOME":       "System (filtered by allowlist)",
		"PATH":       "System (filtered by allowlist)",
		"GLOBAL_VAR": "Global",
		"GROUP_VAR":  "Group[test-group]",
		"CMD_VAR":    "Command[test-command]",
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envVars, origins)

	output := buf.String()

	// Verify header
	assert.Contains(t, output, "===== Final Process Environment =====")

	// Verify count
	assert.Contains(t, output, "Environment variables (5):")

	// Verify each variable and its origin
	assert.Contains(t, output, "CMD_VAR=cmd_value")
	assert.Contains(t, output, "(from Command[test-command])")

	assert.Contains(t, output, "GLOBAL_VAR=global_value")
	assert.Contains(t, output, "(from Global)")

	assert.Contains(t, output, "GROUP_VAR=group_value")
	assert.Contains(t, output, "(from Group[test-group])")

	assert.Contains(t, output, "HOME=/home/test")
	assert.Contains(t, output, "(from System (filtered by allowlist))")

	assert.Contains(t, output, "PATH=/usr/bin:/bin")
}

// TestPrintFinalEnvironment_MultipleOrigins tests output with multiple different origins
func TestPrintFinalEnvironment_MultipleOrigins(t *testing.T) {
	envVars := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
		"VAR3": "value3",
		"VAR4": "value4",
	}

	origins := map[string]string{
		"VAR1": "System (filtered by allowlist)",
		"VAR2": "Global",
		"VAR3": "Group[my-group]",
		"VAR4": "Command[my-command]",
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envVars, origins)

	output := buf.String()

	// Verify all origins are displayed
	assert.Contains(t, output, "(from System (filtered by allowlist))")
	assert.Contains(t, output, "(from Global)")
	assert.Contains(t, output, "(from Group[my-group])")
	assert.Contains(t, output, "(from Command[my-command])")
}

// TestPrintFinalEnvironment_LongValue tests that long values are truncated
func TestPrintFinalEnvironment_LongValue(t *testing.T) {
	// Create a value longer than MaxDisplayLength (60)
	longValue := strings.Repeat("a", 100)

	envVars := map[string]string{
		"LONG_VAR": longValue,
	}

	origins := map[string]string{
		"LONG_VAR": "Global",
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envVars, origins)

	output := buf.String()

	// Verify the value is truncated
	// MaxDisplayLength=60, EllipsisLength=3, so we expect 57 chars + "..."
	expectedTruncated := longValue[:MaxDisplayLength-EllipsisLength] + "..."
	assert.Contains(t, output, expectedTruncated)

	// Verify the full long value is NOT in the output
	assert.NotContains(t, output, longValue)
}

// TestPrintFinalEnvironment_EmptyEnv tests with empty environment
func TestPrintFinalEnvironment_EmptyEnv(t *testing.T) {
	envVars := map[string]string{}
	origins := map[string]string{}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envVars, origins)

	output := buf.String()

	// Verify appropriate message for empty environment
	assert.Contains(t, output, "===== Final Process Environment =====")
	assert.Contains(t, output, "No environment variables set.")
}

// TestPrintFinalEnvironment_SpecialCharacters tests handling of special characters
func TestPrintFinalEnvironment_SpecialCharacters(t *testing.T) {
	envVars := map[string]string{
		"VAR_WITH_NEWLINE": "value\nwith\nnewlines",
		"VAR_WITH_TAB":     "value\twith\ttabs",
		"VAR_WITH_QUOTES":  `value "with" quotes`,
		"VAR_WITH_SPACES":  "value with spaces",
	}

	origins := map[string]string{
		"VAR_WITH_NEWLINE": "Global",
		"VAR_WITH_TAB":     "Global",
		"VAR_WITH_QUOTES":  "Global",
		"VAR_WITH_SPACES":  "Global",
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envVars, origins)

	output := buf.String()

	// Verify special characters are displayed as-is (not escaped)
	assert.Contains(t, output, "VAR_WITH_NEWLINE=value\nwith\nnewlines")
	assert.Contains(t, output, "VAR_WITH_TAB=value\twith\ttabs")
	assert.Contains(t, output, `VAR_WITH_QUOTES=value "with" quotes`)
	assert.Contains(t, output, "VAR_WITH_SPACES=value with spaces")
}

// TestPrintFinalEnvironment_SortedOutput tests that output is sorted alphabetically
func TestPrintFinalEnvironment_SortedOutput(t *testing.T) {
	envVars := map[string]string{
		"ZEBRA":  "z",
		"ALPHA":  "a",
		"MIDDLE": "m",
		"BETA":   "b",
	}

	origins := map[string]string{
		"ZEBRA":  "Global",
		"ALPHA":  "Global",
		"MIDDLE": "Global",
		"BETA":   "Global",
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envVars, origins)

	output := buf.String()

	// Find positions of each variable in output
	alphaPos := strings.Index(output, "ALPHA=")
	betaPos := strings.Index(output, "BETA=")
	middlePos := strings.Index(output, "MIDDLE=")
	zebraPos := strings.Index(output, "ZEBRA=")

	// Verify alphabetical order
	assert.True(t, alphaPos < betaPos, "ALPHA should come before BETA")
	assert.True(t, betaPos < middlePos, "BETA should come before MIDDLE")
	assert.True(t, middlePos < zebraPos, "MIDDLE should come before ZEBRA")
}

// TestPrintFinalEnvironment_SensitiveData verifies that all environment variables
// including sensitive information are displayed without masking in dry-run mode
func TestPrintFinalEnvironment_SensitiveData(t *testing.T) {
	// Create environment with sensitive data (passwords, tokens, keys)
	envVars := map[string]string{
		"DB_PASSWORD":    "super_secret_password_123",
		"API_TOKEN":      "ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		"AWS_SECRET_KEY": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"SSH_PRIVATE":    "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...",
		"NORMAL_VAR":     "public_value",
	}

	origins := map[string]string{
		"DB_PASSWORD":    "Command[db-setup]",
		"API_TOKEN":      "Global",
		"AWS_SECRET_KEY": "Group[aws-group]",
		"SSH_PRIVATE":    "Command[ssh-deploy]",
		"NORMAL_VAR":     "Global",
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envVars, origins)
	output := buf.String()

	// Verify header is present
	assert.Contains(t, output, "===== Final Process Environment =====")

	// Verify all sensitive values are displayed WITHOUT masking
	// This is by design for dry-run mode audit purposes
	assert.Contains(t, output, "super_secret_password_123", "Password should be displayed without masking")
	assert.Contains(t, output, "ghp_1234567890abcdefghijklmnopqrstuvwxyz", "API token should be displayed without masking")
	assert.Contains(t, output, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "AWS secret key should be displayed without masking")
	assert.Contains(t, output, "-----BEGIN RSA PRIVATE KEY-----", "SSH private key should be displayed without masking")
	assert.Contains(t, output, "public_value", "Normal values should be displayed")

	// Verify origins are shown
	assert.Contains(t, output, "Command[db-setup]")
	assert.Contains(t, output, "Global")
	assert.Contains(t, output, "Group[aws-group]")
	assert.Contains(t, output, "Command[ssh-deploy]")

	// Verify no masking characters (like asterisks) are present
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "DB_PASSWORD") ||
			strings.Contains(line, "API_TOKEN") ||
			strings.Contains(line, "AWS_SECRET_KEY") ||
			strings.Contains(line, "SSH_PRIVATE") {
			// These lines should NOT contain masking patterns like "***" or "[REDACTED]"
			assert.NotContains(t, line, "***", "Sensitive values should not be masked")
			assert.NotContains(t, line, "[REDACTED]", "Sensitive values should not be redacted")
			assert.NotContains(t, line, "[MASKED]", "Sensitive values should not be marked as masked")
		}
	}
}
