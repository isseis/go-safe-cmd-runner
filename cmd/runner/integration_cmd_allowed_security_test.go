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

// TestIntegration_CmdAllowed_RelativePathRejected tests that relative paths in cmd_allowed
// are rejected (path traversal prevention).
func TestIntegration_CmdAllowed_RelativePathRejected(t *testing.T) {
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

	// Config with relative path in cmd_allowed - should fail at config expansion
	configContent := `
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
# Relative path - security violation
cmd_allowed = ["bin/echo", "../bin/echo"]

[[groups.commands]]
name = "test_cmd"
cmd = "/bin/echo"
args = ["test"]
`

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-relative-path")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// Try to create runner - should fail during group expansion due to relative path
	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-relative-path"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx := context.Background()
	err = r.Execute(ctx, nil)

	// Expect error due to relative path in cmd_allowed
	require.Error(t, err, "Should reject relative paths in cmd_allowed")
	assert.Contains(t, err.Error(), "absolute", "Error message should mention absolute path requirement")
}

// TestIntegration_CmdAllowed_SymlinkResolution tests that symlinks in cmd_allowed
// are properly resolved and checked. Commands executed via symlink are allowed
// when the resolved real path matches cmd_allowed.
func TestIntegration_CmdAllowed_SymlinkResolution(t *testing.T) {
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")
	outputFile := filepath.Join(testDir, "output.txt")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

	// Find actual sh command
	testCmd := "/bin/sh"
	if _, err := os.Stat(testCmd); os.IsNotExist(err) {
		t.Skip("/bin/sh not found")
	}

	// Resolve symlink to get actual path (in case /bin/sh itself is a symlink)
	resolvedPath, err := filepath.EvalSymlinks(testCmd)
	require.NoError(t, err)

	// Config that allows the RESOLVED path (not the symlink itself)
	// This tests that when executing /bin/sh, if it's allowed in cmd_allowed,
	// the validation will resolve /bin/sh and check against cmd_allowed
	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
# Allow the resolved (real) path of the command
cmd_allowed = ["%s"]

[[groups.commands]]
name = "test_cmd"
# Execute via original path - should work because resolved path is allowed
cmd = "%s"
args = ["-c", "echo 'Symlink resolution works' > %s"]
`, resolvedPath, testCmd, outputFile)

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-symlink")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-symlink"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx := context.Background()
	err = r.Execute(ctx, nil)
	require.NoError(t, err, "Should allow command when resolved path matches cmd_allowed")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Symlink resolution works")
}

// TestIntegration_CmdAllowed_NonexistentPath tests that non-existent paths in cmd_allowed
// cause an error during config expansion.
func TestIntegration_CmdAllowed_NonexistentPath(t *testing.T) {
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

	// Config with non-existent path in cmd_allowed
	configContent := `
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
# Path that doesn't exist - should fail during symlink resolution
cmd_allowed = ["/this/path/does/not/exist"]

[[groups.commands]]
name = "test_cmd"
cmd = "/bin/echo"
args = ["test"]
`

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-nonexistent")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-nonexistent"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx := context.Background()
	err = r.Execute(ctx, nil)

	// Expect error because path doesn't exist
	require.Error(t, err, "Should fail when cmd_allowed contains non-existent path")
	assert.True(t, strings.Contains(err.Error(), "no such file") ||
		strings.Contains(err.Error(), "failed to resolve"),
		"Error should indicate path resolution failure: %v", err)
}

// TestIntegration_CmdAllowed_OtherSecurityChecksRemain tests that other security
// checks (file permissions, etc.) are still applied even when cmd_allowed matches.
func TestIntegration_CmdAllowed_OtherSecurityChecksRemain(t *testing.T) {
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

	// Config that allows testCmd via cmd_allowed
	// This test verifies that just because cmd_allowed permits a command,
	// other security validations (e.g., environment variable validation) still run
	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 60

[[groups]]
name = "test_group"
cmd_allowed = ["%s"]

[[groups.commands]]
name = "test_cmd"
cmd = "%s"
args = ["-c", "echo 'Security checks remain active' > %s"]
`, testCmd, testCmd, outputFile)

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-security-checks")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-security-checks"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	// Execute should succeed - this test mainly verifies that the command
	// runs through all security validation steps
	ctx := context.Background()
	err = r.Execute(ctx, nil)
	require.NoError(t, err, "Command should succeed when allowed via cmd_allowed and passes other security checks")

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Security checks remain active")
}
