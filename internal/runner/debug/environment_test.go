package debug

import (
	"bytes"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/stretchr/testify/assert"
)

// TestPrintFinalEnvironment_WithOrigins tests that PrintFinalEnvironment correctly uses the origins map
func TestPrintFinalEnvironment_WithOrigins(t *testing.T) {
	envMap := map[string]executor.EnvVar{
		"HOME": {
			Value:  "/home/test",
			Origin: "System (filtered by allowlist)",
		},
		"PATH": {
			Value:  "/usr/bin:/bin",
			Origin: "System (filtered by allowlist)",
		},
		"GLOBAL_VAR": {
			Value:  "global_value",
			Origin: "Global",
		},
		"GROUP_VAR": {
			Value:  "group_value",
			Origin: "Group[test-group]",
		},
		"CMD_VAR": {
			Value:  "cmd_value",
			Origin: "Command[test-command]",
		},
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envMap, false)

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
	envMap := map[string]executor.EnvVar{
		"VAR1": {
			Value:  "value1",
			Origin: "System (filtered by allowlist)",
		},
		"VAR2": {
			Value:  "value2",
			Origin: "Global",
		},
		"VAR3": {
			Value:  "value3",
			Origin: "Group[my-group]",
		},
		"VAR4": {
			Value:  "value4",
			Origin: "Command[my-command]",
		},
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envMap, false)

	output := buf.String()

	// Verify all origins are displayed
	assert.Contains(t, output, "(from System (filtered by allowlist))")
	assert.Contains(t, output, "(from Global)")
	assert.Contains(t, output, "(from Group[my-group])")
	assert.Contains(t, output, "(from Command[my-command])")
}

// TestPrintFinalEnvironment_LongValue tests that long values are displayed in full
// (no truncation for dry-run verification purposes)
func TestPrintFinalEnvironment_LongValue(t *testing.T) {
	// Create a value longer than 100 characters
	longValue := strings.Repeat("a", 150)

	envMap := map[string]executor.EnvVar{
		"LONG_VAR": {
			Value:  longValue,
			Origin: "Global",
		},
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envMap, false)

	output := buf.String()

	// Verify the FULL value is displayed (no truncation)
	assert.Contains(t, output, longValue, "Long values should be displayed in full for dry-run verification")

	// Verify no ellipsis is present
	assert.NotContains(t, output, "...", "Long values should not be truncated")
}

// TestPrintFinalEnvironment_EmptyEnv tests with empty environment
func TestPrintFinalEnvironment_EmptyEnv(t *testing.T) {
	envMap := map[string]executor.EnvVar{}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envMap, false)

	output := buf.String()

	// Verify appropriate message for empty environment
	assert.Contains(t, output, "===== Final Process Environment =====")
	assert.Contains(t, output, "No environment variables set.")
}

// TestPrintFinalEnvironment_ControlCharacters tests that control characters are escaped
func TestPrintFinalEnvironment_ControlCharacters(t *testing.T) {
	envMap := map[string]executor.EnvVar{
		"VAR_WITH_NEWLINE": {
			Value:  "value\nwith\nnewlines",
			Origin: "Global",
		},
		"VAR_WITH_TAB": {
			Value:  "value\twith\ttabs",
			Origin: "Global",
		},
		"VAR_WITH_CR": {
			Value:  "value\rwith\rcarriage",
			Origin: "Global",
		},
		"VAR_WITH_NULL": {
			Value:  "value\x00with\x00null",
			Origin: "Global",
		},
		"VAR_WITH_ESC": {
			Value:  "value\x1b[31mwith\x1b[0mcolor",
			Origin: "Global",
		},
		"VAR_WITH_QUOTES": {
			Value:  `value "with" quotes`,
			Origin: "Global",
		},
		"VAR_WITH_SPACES": {
			Value:  "value with spaces",
			Origin: "Global",
		},
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envMap, false)

	output := buf.String()

	// Verify control characters are ESCAPED for safe display
	assert.Contains(t, output, `VAR_WITH_NEWLINE=value\nwith\nnewlines`, "Newlines should be escaped")
	assert.Contains(t, output, `VAR_WITH_TAB=value\twith\ttabs`, "Tabs should be escaped")
	assert.Contains(t, output, `VAR_WITH_CR=value\rwith\rcarriage`, "Carriage returns should be escaped")
	assert.Contains(t, output, `VAR_WITH_NULL=value\x00with\x00null`, "Null bytes should be escaped")
	assert.Contains(t, output, `VAR_WITH_ESC=value\x1b[31mwith\x1b[0mcolor`, "Escape sequences should be escaped")

	// Verify normal characters are NOT escaped
	assert.Contains(t, output, `VAR_WITH_QUOTES=value "with" quotes`, "Quotes should not be escaped")
	assert.Contains(t, output, "VAR_WITH_SPACES=value with spaces", "Spaces should not be escaped")

	// Verify raw control characters are NOT in output (they should be escaped)
	assert.NotContains(t, output, "value\nwith\nnewlines", "Raw newlines should not be in output")
	assert.NotContains(t, output, "value\twith\ttabs", "Raw tabs should not be in output")
}

// TestPrintFinalEnvironment_SortedOutput tests that output is sorted alphabetically
func TestPrintFinalEnvironment_SortedOutput(t *testing.T) {
	envMap := map[string]executor.EnvVar{
		"ZEBRA": {
			Value:  "z",
			Origin: "Global",
		},
		"ALPHA": {
			Value:  "a",
			Origin: "Global",
		},
		"MIDDLE": {
			Value:  "m",
			Origin: "Global",
		},
		"BETA": {
			Value:  "b",
			Origin: "Global",
		},
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envMap, false)

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
	envMap := map[string]executor.EnvVar{
		"DB_PASSWORD": {
			Value:  "super_secret_password_123",
			Origin: "Command[db-setup]",
		},
		"API_TOKEN": {
			Value:  "ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			Origin: "Global",
		},
		"AWS_SECRET_KEY": {
			Value:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			Origin: "Group[aws-group]",
		},
		"SSH_PRIVATE_KEY": {
			Value:  "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...",
			Origin: "Command[ssh-deploy]",
		},
		"GITHUB_AUTH_TOKEN": {
			Value:  "ghp_another_token_12345",
			Origin: "Global",
		},
		"NORMAL_VAR": {
			Value:  "public_value",
			Origin: "Global",
		},
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envMap, false) // showSensitive=false (default)
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
	envMap := map[string]executor.EnvVar{
		"DB_PASSWORD": {
			Value:  "super_secret_password_123",
			Origin: "Command[db-setup]",
		},
		"API_TOKEN": {
			Value:  "ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			Origin: "Global",
		},
		"AWS_SECRET_KEY": {
			Value:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			Origin: "Group[aws-group]",
		},
		"SSH_PRIVATE_KEY": {
			Value:  "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...",
			Origin: "Command[ssh-deploy]",
		},
		"GITHUB_AUTH_TOKEN": {
			Value:  "ghp_another_token_12345",
			Origin: "Global",
		},
		"NORMAL_VAR": {
			Value:  "public_value",
			Origin: "Global",
		},
	}

	var buf bytes.Buffer
	PrintFinalEnvironment(&buf, envMap, true) // showSensitive=true (explicit)
	output := buf.String()

	// Verify header is present
	assert.Contains(t, output, "===== Final Process Environment =====")

	// Verify all sensitive values are displayed WITHOUT masking (but with escaping)
	assert.Contains(t, output, "super_secret_password_123", "Password should be displayed without masking")
	assert.Contains(t, output, "ghp_1234567890abcdefghijklmnopqrstuvwxyz", "API token should be displayed without masking")
	assert.Contains(t, output, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "AWS secret key should be displayed without masking")
	// SSH private key contains \n which should be escaped
	assert.Contains(t, output, `-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...`, "SSH private key should be displayed with escaped newlines")
	assert.Contains(t, output, "public_value", "Normal values should be displayed")

	// Verify origins are shown
	assert.Contains(t, output, "Command[db-setup]")
	assert.Contains(t, output, "Global")
	assert.Contains(t, output, "Group[aws-group]")
	assert.Contains(t, output, "Command[ssh-deploy]")

	// Verify no masking characters are present
	assert.NotContains(t, output, "[REDACTED]", "Values should not be redacted when showSensitive=true")
}
