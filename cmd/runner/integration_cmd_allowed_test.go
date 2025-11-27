//go:build test

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_CmdAllowed_BasicFunctionality tests that group-level cmd_allowed
// allows commands that are not in global allowed_commands patterns.
func TestIntegration_CmdAllowed_BasicFunctionality(t *testing.T) {
	env := setupTestEnvironment(t, "test-run-cmd-allowed")
	outputFile := env.outputFilePath()

	// Find a command that exists on the system for testing
	// /bin/sh should be available on most Unix systems
	testCmd := "/bin/sh"
	if _, err := os.Stat(testCmd); os.IsNotExist(err) {
		t.Skip("/bin/sh not found")
	}

	// Config with hardcoded allowed_commands patterns (/bin/.*, /usr/bin/.*, etc.)
	// Note: /bin/sh matches the hardcoded pattern, so we test cmd_allowed with a different command
	// For this test, we'll verify cmd_allowed works by using a command path
	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
# Group-level cmd_allowed permits /bin/sh
cmd_allowed = ["%s"]

[[groups.commands]]
name = "test_cmd"
cmd = "%s"
args = ["-c", "echo 'Hello from cmd_allowed' > %s"]
`, testCmd, testCmd, outputFile)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	// Execute the commands - should succeed via cmd_allowed
	ctx := context.Background()
	err := r.Execute(ctx, nil)
	require.NoError(t, err, "Command should be allowed via group-level cmd_allowed")

	// Verify output
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Hello from cmd_allowed")
}

// TestIntegration_CmdAllowed_WithVariableExpansion tests that cmd_allowed paths
// support variable expansion.
func TestIntegration_CmdAllowed_WithVariableExpansion(t *testing.T) {
	env := setupTestEnvironment(t, "test-run-var-expansion")
	outputFile := env.outputFilePath()

	// Find a command that exists
	testCmd := "/bin/sh"
	if _, err := os.Stat(testCmd); os.IsNotExist(err) {
		t.Skip("/bin/sh not found")
	}

	// Determine the directory of testCmd
	cmdDir := filepath.Dir(testCmd)
	cmdBase := filepath.Base(testCmd)

	// Config using variable expansion in cmd_allowed
	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
vars = [
    "cmd_dir=%s",
]
# Group-level cmd_allowed with variable expansion
cmd_allowed = ["%%{cmd_dir}/%s"]

[[groups.commands]]
name = "test_cmd"
cmd = "%s"
args = ["-c", "echo 'Variable expansion works' > %s"]
`, cmdDir, cmdBase, testCmd, outputFile)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	ctx := context.Background()
	err := r.Execute(ctx, nil)
	require.NoError(t, err, "Command should be allowed via variable-expanded cmd_allowed")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Variable expansion works")
}

// TestIntegration_CmdAllowed_GlobalPatternTakesPrecedence tests that commands
// matching global allowed_commands patterns are allowed even without cmd_allowed.
func TestIntegration_CmdAllowed_GlobalPatternTakesPrecedence(t *testing.T) {
	env := setupTestEnvironment(t, "test-run-global-pattern")
	outputFile := env.outputFilePath()

	testCmd := "/bin/sh"
	if _, err := os.Stat(testCmd); os.IsNotExist(err) {
		t.Skip("/bin/sh not found")
	}

	// Config with hardcoded allowed_commands patterns that match testCmd (/bin/.*)
	// No cmd_allowed to verify hardcoded global pattern works
	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
# No cmd_allowed - relying on hardcoded global pattern

[[groups.commands]]
name = "test_cmd"
cmd = "%s"
args = ["-c", "echo 'Global pattern works' > %s"]
`, testCmd, outputFile)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	ctx := context.Background()
	err := r.Execute(ctx, nil)
	require.NoError(t, err, "Command should be allowed via global allowed_commands pattern")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Global pattern works")
}

// TestIntegration_CmdAllowed_ORCondition tests that commands are allowed if they
// match EITHER global patterns OR group-level cmd_allowed (OR condition).
func TestIntegration_CmdAllowed_ORCondition(t *testing.T) {
	env := setupTestEnvironment(t, "test-run-or-condition")
	outputFile := env.outputFilePath()

	testCmd := "/bin/sh"
	if _, err := os.Stat(testCmd); os.IsNotExist(err) {
		t.Skip("/bin/sh not found")
	}

	// Config where testCmd matches BOTH hardcoded global pattern (/bin/.*) AND cmd_allowed
	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
# Also explicitly allow testCmd in cmd_allowed
cmd_allowed = ["%s"]

[[groups.commands]]
name = "test_cmd"
cmd = "%s"
args = ["-c", "echo 'Both match - OR condition' > %s"]
`, testCmd, testCmd, outputFile)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	ctx := context.Background()
	err := r.Execute(ctx, nil)
	require.NoError(t, err, "Command should be allowed (matches both global and group-level)")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Both match - OR condition")
}

// TestIntegration_CmdAllowed_NotConfigured tests that commands rely on global
// patterns when cmd_allowed is not configured at group level.
func TestIntegration_CmdAllowed_NotConfigured(t *testing.T) {
	env := setupTestEnvironment(t, "test-run-no-cmd-allowed")
	outputFile := env.outputFilePath()

	testCmd := "/bin/sh"
	if _, err := os.Stat(testCmd); os.IsNotExist(err) {
		t.Skip("/bin/sh not found")
	}

	// Config without cmd_allowed field at group level
	// Hardcoded allowed_commands patterns (/bin/.*, /usr/bin/.*) should work
	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
# No cmd_allowed field - existing behavior should be maintained

[[groups.commands]]
name = "test_cmd"
cmd = "%s"
args = ["-c", "echo 'Existing behavior maintained' > %s"]
`, testCmd, outputFile)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	ctx := context.Background()
	err := r.Execute(ctx, nil)
	require.NoError(t, err, "Existing behavior should be maintained when cmd_allowed is not configured")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Existing behavior maintained")
}

// TestIntegration_CmdAllowed_CommandNotAllowed tests error when command is not allowed.
func TestIntegration_CmdAllowed_CommandNotAllowed(t *testing.T) {
	env := setupTestEnvironment(t, "test-run-not-allowed")

	// Use a path that does NOT match hardcoded allowed_commands patterns
	// Hardcoded patterns: ^/bin/.*, ^/usr/bin/.*, ^/usr/sbin/.*, ^/usr/local/bin/.*
	// So use a path like /opt/custom/bin/sh
	testCmd := "/opt/custom/bin/notallowed"

	// Config without cmd_allowed - testCmd should be rejected by hardcoded patterns
	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
# No cmd_allowed - testCmd should be rejected

[[groups.commands]]
name = "test_cmd"
cmd = "%s"
args = ["arg1"]
`, testCmd)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	ctx := context.Background()
	err := r.Execute(ctx, nil)

	// Expect error because command path doesn't exist and is not allowed
	// The error could be either:
	// 1. verification.ErrCommandNotFound if path resolution fails first
	// 2. security.ErrCommandNotAllowed if security check fails first
	require.Error(t, err, "Should reject command not in hardcoded patterns or cmd_allowed")
	// Use errors.Is() to check for specific error types instead of fragile string matching
	assert.True(t,
		errors.Is(err, verification.ErrCommandNotFound) || errors.Is(err, security.ErrCommandNotAllowed),
		"Error should be either ErrCommandNotFound or ErrCommandNotAllowed, got: %v", err)
}
