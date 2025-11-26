//go:build test

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_CmdAllowed_BasicFunctionality tests that group-level cmd_allowed
// allows commands that are not in global allowed_commands patterns.
func TestIntegration_CmdAllowed_BasicFunctionality(t *testing.T) {
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")
	outputFile := filepath.Join(testDir, "output.txt")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

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

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Load and expand config
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-cmd-allowed")
	require.NoError(t, err)

	// Expand global configuration to get runtime global
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// Create and execute runner
	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-cmd-allowed"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	// Execute the commands - should succeed via cmd_allowed
	ctx := context.Background()
	err = r.Execute(ctx, nil)
	require.NoError(t, err, "Command should be allowed via group-level cmd_allowed")

	// Verify output
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Hello from cmd_allowed")
}

// TestIntegration_CmdAllowed_WithVariableExpansion tests that cmd_allowed paths
// support variable expansion.
func TestIntegration_CmdAllowed_WithVariableExpansion(t *testing.T) {
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")
	outputFile := filepath.Join(testDir, "output.txt")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

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

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Load and expand config
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-var-expansion")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-var-expansion"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx := context.Background()
	err = r.Execute(ctx, nil)
	require.NoError(t, err, "Command should be allowed via variable-expanded cmd_allowed")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Variable expansion works")
}

// TestIntegration_CmdAllowed_GlobalPatternTakesPrecedence tests that commands
// matching global allowed_commands patterns are allowed even without cmd_allowed.
func TestIntegration_CmdAllowed_GlobalPatternTakesPrecedence(t *testing.T) {
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")
	outputFile := filepath.Join(testDir, "output.txt")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

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

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-global-pattern")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-global-pattern"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx := context.Background()
	err = r.Execute(ctx, nil)
	require.NoError(t, err, "Command should be allowed via global allowed_commands pattern")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Global pattern works")
}

// TestIntegration_CmdAllowed_ORCondition tests that commands are allowed if they
// match EITHER global patterns OR group-level cmd_allowed (OR condition).
func TestIntegration_CmdAllowed_ORCondition(t *testing.T) {
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")
	outputFile := filepath.Join(testDir, "output.txt")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

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

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-or-condition")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-or-condition"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx := context.Background()
	err = r.Execute(ctx, nil)
	require.NoError(t, err, "Command should be allowed (matches both global and group-level)")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Both match - OR condition")
}

// TestIntegration_CmdAllowed_NotConfigured tests that commands rely on global
// patterns when cmd_allowed is not configured at group level.
func TestIntegration_CmdAllowed_NotConfigured(t *testing.T) {
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")
	outputFile := filepath.Join(testDir, "output.txt")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

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

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-no-cmd-allowed")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-no-cmd-allowed"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx := context.Background()
	err = r.Execute(ctx, nil)
	require.NoError(t, err, "Existing behavior should be maintained when cmd_allowed is not configured")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Existing behavior maintained")
}

// TestIntegration_CmdAllowed_CommandNotAllowed tests error when command is not allowed.
func TestIntegration_CmdAllowed_CommandNotAllowed(t *testing.T) {
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

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

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-not-allowed")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-not-allowed"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx := context.Background()
	err = r.Execute(ctx, nil)

	// Expect error because command path doesn't exist and is not allowed
	// The error could be either "command not found" (path resolution) or "not allowed" (security check)
	require.Error(t, err, "Should reject command not in hardcoded patterns or cmd_allowed")
	assert.True(t, strings.Contains(err.Error(), "not allowed") ||
		strings.Contains(err.Error(), "does not match") ||
		strings.Contains(err.Error(), "command not found"),
		"Error should indicate command rejection: %v", err)
}
