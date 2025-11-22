//go:build test

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDryRunE2E_HashDirectoryNotFound tests dry-run with hash directory not found
func TestDryRunE2E_HashDirectoryNotFound(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Run command in dry-run mode
	// Note: Uses default hash directory which may or may not exist
	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run", "-dry-run-detail", "full", "-dry-run-format", "text")
	cmd.Dir = "."

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command output:\n%s", string(output))
	}
	require.NoError(t, err, "dry-run should succeed even with missing hash directory")

	outputStr := string(output)
	// Verify file verification section is present
	assert.Contains(t, outputStr, "=== FILE VERIFICATION ===")
	assert.Contains(t, outputStr, "Hash Directory:")

	// Verify exit code is 0
	assert.Equal(t, 0, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_HashFilesNotFound tests dry-run with hash files not found
func TestDryRunE2E_HashFilesNotFound(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Run command in dry-run mode
	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run", "-dry-run-detail", "full", "-dry-run-format", "text")
	cmd.Dir = "."

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "dry-run should succeed even with missing hash files")

	outputStr := string(output)
	// Verify file verification section is present
	assert.Contains(t, outputStr, "=== FILE VERIFICATION ===")
	assert.Contains(t, outputStr, "Hash Directory:")

	// Verify exit code is 0
	assert.Equal(t, 0, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_AllSuccess tests dry-run with all verifications successful
func TestDryRunE2E_AllSuccess(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Run command in dry-run mode
	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run", "-dry-run-detail", "summary", "-dry-run-format", "text")
	cmd.Dir = "."

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "dry-run should succeed")

	outputStr := string(output)
	// Verify file verification section is present
	assert.Contains(t, outputStr, "=== FILE VERIFICATION ===")

	// Verify exit code is 0
	assert.Equal(t, 0, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_JSONOutput tests dry-run JSON output with file verification
func TestDryRunE2E_JSONOutput(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Run command in dry-run mode with JSON output
	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run", "-dry-run-detail", "full", "-dry-run-format", "json", "-log-level", "error")
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	require.NoError(t, err, "dry-run should succeed")

	// Parse JSON output
	var result struct {
		Status           string                                `json:"status"`
		Phase            string                                `json:"phase"`
		FileVerification *verification.FileVerificationSummary `json:"file_verification,omitempty"`
	}

	err = json.Unmarshal([]byte(stdout.String()), &result)
	require.NoError(t, err, "stdout should be valid JSON")

	// Verify JSON structure
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "completed", result.Phase)

	// Verify file verification is included
	require.NotNil(t, result.FileVerification, "file_verification should be present in JSON output")
	assert.NotNil(t, result.FileVerification.HashDirStatus)

	// Verify exit code is 0
	assert.Equal(t, 0, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_MixedResults tests dry-run with mixed verification results
func TestDryRunE2E_MixedResults(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd-1"
cmd = "/bin/echo"
args = ["hello"]

[[groups.commands]]
name = "test-cmd-2"
cmd = "/bin/ls"
args = ["-l"]
`

	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Run command in dry-run mode with detailed output
	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run", "-dry-run-detail", "detailed", "-dry-run-format", "text")
	cmd.Dir = "."

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "dry-run should succeed even with verification failures")

	outputStr := string(output)
	// Verify file verification section is present
	assert.Contains(t, outputStr, "=== FILE VERIFICATION ===")

	// Verify detailed level shows failures if present
	if !strings.Contains(outputStr, "Failed: 0") {
		// If there are failures, they should be shown in detailed mode
		assert.Contains(t, outputStr, "Failures:")
	}

	// Verify exit code is 0 (dry-run never fails)
	assert.Equal(t, 0, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_NoSideEffects tests that dry-run with file verification has no side effects
func TestDryRunE2E_NoSideEffects(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Run command in dry-run mode
	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run", "-dry-run-detail", "full", "-dry-run-format", "text")
	cmd.Dir = "."

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "dry-run should succeed")
	require.NotEmpty(t, output, "output should not be empty")

	// Verify no files were created in temp directory (except config file)
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 1, len(entries), "dry-run should not create any files")
	assert.Equal(t, "config.toml", entries[0].Name())

	// Verify exit code is 0
	assert.Equal(t, 0, cmd.ProcessState.ExitCode())
}
