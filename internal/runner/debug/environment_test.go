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
	PrintFinalEnvironment(&buf, envVars, origins, false)

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
	PrintFinalEnvironment(&buf, envVars, origins, false)

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
	PrintFinalEnvironment(&buf, envVars, origins, false)

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
	PrintFinalEnvironment(&buf, envVars, origins, false)

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
	PrintFinalEnvironment(&buf, envVars, origins, false)

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
	PrintFinalEnvironment(&buf, envVars, origins, false)

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

// TestPrintFinalEnvironment_MaskingSensitiveData_Default verifies that sensitive
// environment variables are masked by default (showSensitive=false)
func TestPrintFinalEnvironment_MaskingSensitiveData_Default(t *testing.T) {
	// Create environment with sensitive data (passwords, tokens, keys)
	envVars := map[string]string{
		"DB_PASSWORD":       "super_secret_password_123",
		"API_TOKEN":         "ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		"AWS_SECRET_KEY":    "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"SSH_PRIVATE_KEY":   "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...",
		"GITHUB_AUTH_TOKEN": "ghp_another_token_12345",
		"NORMAL_VAR":        "public_value",
	}

	origins := map[string]string{
		"DB_PASSWORD":       "Command[db-setup]",
		"API_TOKEN":         "Global",
		"AWS_SECRET_KEY":    "Group[aws-group]",
		"SSH_PRIVATE_KEY":   "Command[ssh-deploy]",
		"GITHUB_AUTH_TOKEN": "Global",
		"NORMAL_VAR":        "Global",
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envVars, origins, false) // showSensitive=false (default)
	output := buf.String()

	// Verify header is present
	assert.Contains(t, output, "===== Final Process Environment =====")

	// Verify sensitive values are MASKED
	assert.Contains(t, output, "DB_PASSWORD=[REDACTED]", "Password should be masked")
	assert.Contains(t, output, "API_TOKEN=[REDACTED]", "API token should be masked")
	assert.Contains(t, output, "AWS_SECRET_KEY=[REDACTED]", "AWS secret key should be masked")
	assert.Contains(t, output, "SSH_PRIVATE_KEY=[REDACTED]", "SSH private key should be masked")
	assert.Contains(t, output, "GITHUB_AUTH_TOKEN=[REDACTED]", "GitHub auth token should be masked")

	// Verify normal values are NOT masked
	assert.Contains(t, output, "NORMAL_VAR=public_value", "Normal values should not be masked")

	// Verify origins are shown
	assert.Contains(t, output, "Command[db-setup]")
	assert.Contains(t, output, "Global")
	assert.Contains(t, output, "Group[aws-group]")
	assert.Contains(t, output, "Command[ssh-deploy]")

	// Verify sensitive values are NOT displayed in plain text
	assert.NotContains(t, output, "super_secret_password_123", "Password should not be displayed in plain text")
	assert.NotContains(t, output, "ghp_1234567890abcdefghijklmnopqrstuvwxyz", "API token should not be displayed in plain text")
	assert.NotContains(t, output, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "AWS secret key should not be displayed in plain text")
	assert.NotContains(t, output, "-----BEGIN RSA PRIVATE KEY-----", "SSH private key should not be displayed in plain text")
	assert.NotContains(t, output, "ghp_another_token_12345", "GitHub auth token should not be displayed in plain text")
}

// TestPrintFinalEnvironment_ShowSensitiveData_Explicit verifies that sensitive
// environment variables are displayed when showSensitive=true
func TestPrintFinalEnvironment_ShowSensitiveData_Explicit(t *testing.T) {
	// Create environment with sensitive data (passwords, tokens, keys)
	envVars := map[string]string{
		"DB_PASSWORD":       "super_secret_password_123",
		"API_TOKEN":         "ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		"AWS_SECRET_KEY":    "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"SSH_PRIVATE_KEY":   "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...",
		"GITHUB_AUTH_TOKEN": "ghp_another_token_12345",
		"NORMAL_VAR":        "public_value",
	}

	origins := map[string]string{
		"DB_PASSWORD":       "Command[db-setup]",
		"API_TOKEN":         "Global",
		"AWS_SECRET_KEY":    "Group[aws-group]",
		"SSH_PRIVATE_KEY":   "Command[ssh-deploy]",
		"GITHUB_AUTH_TOKEN": "Global",
		"NORMAL_VAR":        "Global",
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envVars, origins, true) // showSensitive=true (explicit)
	output := buf.String()

	// Verify header is present
	assert.Contains(t, output, "===== Final Process Environment =====")

	// Verify all sensitive values are displayed WITHOUT masking
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

	// Verify no masking characters are present
	assert.NotContains(t, output, "[REDACTED]", "Values should not be redacted when showSensitive=true")
}
