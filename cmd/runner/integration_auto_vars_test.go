//go:build test

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/variable"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_AutoVariables(t *testing.T) {
	// Test that __runner_datetime and __runner_pid are properly expanded in commands
	testDir := t.TempDir()
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")
	outputFile := filepath.Join(testDir, "output.txt")

	err := os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 60

[global.vars]
OutputFile = "%s"
BackupFile = "%%{OutputFile}.%%{__runner_datetime}.bak"

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_auto_vars"
cmd = "/bin/sh"
args = ["-c", "echo 'Executed at %%{__runner_datetime} by PID %%{__runner_pid}' > %%{OutputFile}"]
risk_level = "medium"

[[groups.commands]]
name = "test_backup_file"
cmd = "/bin/sh"
args = ["-c", "echo 'backup' > %%{BackupFile}"]
risk_level = "medium"
`, outputFile)

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Load and expand config
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-autovar")
	require.NoError(t, err)

	// Expand global configuration to get runtime global
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// Verify __runner_datetime is present and has correct format
	datetime, ok := runtimeGlobal.ExpandedVars[variable.DatetimeKey()]
	assert.True(t, ok, "__runner_datetime should be present")
	assert.NotEmpty(t, datetime, "__runner_datetime should not be empty")

	matched, err := regexp.MatchString(`^\d{14}\.\d{3}$`, datetime)
	require.NoError(t, err)
	assert.True(t, matched, "DateTime should match format YYYYMMDDHHmmSS.msec, got: %s", datetime)

	// Verify __runner_pid is present
	pid, ok := runtimeGlobal.ExpandedVars[variable.PIDKey()]
	assert.True(t, ok, "__runner_pid should be present")
	assert.Equal(t, fmt.Sprintf("%d", os.Getpid()), pid)

	// Verify vars expansion using auto variables
	backupFile := runtimeGlobal.ExpandedVars["BackupFile"]
	expectedBackup := fmt.Sprintf("%s.%s.bak", outputFile, datetime)
	assert.Equal(t, expectedBackup, backupFile, "BackupFile should be expanded with __runner_datetime")

	// Create and execute runner
	r, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID("test-run-autovar"),
	)
	require.NoError(t, err)

	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	// Execute the commands
	ctx := context.Background()
	err = r.Execute(ctx, nil)
	require.NoError(t, err)

	// Verify the output file contains the expanded values
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	outputStr := string(content)
	assert.Contains(t, outputStr, "Executed at")
	assert.Contains(t, outputStr, datetime, "Output should contain expanded __runner_datetime")
	assert.Contains(t, outputStr, pid, "Output should contain expanded __runner_pid")
	assert.NotContains(t, outputStr, "%{__runner_datetime}", "Output should not contain template variable")
	assert.NotContains(t, outputStr, "%{__runner_pid}", "Output should not contain template variable")

	// Verify backup file was created with correct name
	assert.FileExists(t, expectedBackup, "Backup file should be created with auto variable in filename")
	backupContent, err := os.ReadFile(expectedBackup)
	require.NoError(t, err)
	assert.Contains(t, string(backupContent), "backup")
}
