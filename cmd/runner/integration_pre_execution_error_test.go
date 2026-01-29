//go:build test

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_PreExecutionError_TOMLParseError tests that TOML parse errors
// result in HandlePreExecutionError being called (which outputs to stderr/stdout).
// This verifies the complete error path from main.go through to the user-visible output.
//
// Note: We use dry-run mode to skip hash verification, allowing us to test the TOML
// parsing error path specifically.
func TestE2E_PreExecutionError_TOMLParseError(t *testing.T) {
	// Create a config file with invalid TOML syntax
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "invalid.toml")

	invalidTOML := `
# Invalid TOML: missing quotes around string value
[[groups]]
name = test_group_without_quotes

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
`
	err := os.WriteFile(configFile, []byte(invalidTOML), 0o644)
	require.NoError(t, err)

	// Run the runner with the invalid config in dry-run mode
	// Dry-run mode skips hash verification, allowing us to test TOML parsing errors
	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run")
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	// Command should fail with exit code 1
	require.Error(t, err, "runner should fail with invalid TOML")

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1")

	// Verify stderr contains error information from HandlePreExecutionError
	stderrOutput := stderr.String()
	assert.Contains(t, stderrOutput, "Error:", "stderr should contain 'Error:' prefix")
	assert.Contains(t, stderrOutput, "config_parsing_failed", "stderr should indicate config parsing failure")

	// Verify stdout contains RUN_SUMMARY (from HandlePreExecutionError)
	stdoutOutput := stdout.String()
	assert.Contains(t, stdoutOutput, "RUN_SUMMARY", "stdout should contain RUN_SUMMARY")
	assert.Contains(t, stdoutOutput, "status=pre_execution_error", "stdout should indicate pre_execution_error status")
}

// TestE2E_PreExecutionError_HashNotFound tests that hash file not found errors
// result in HandlePreExecutionError being called.
// This verifies the complete error path from main.go through to the user-visible output.
//
// Note: The runner uses cmdcommon.DefaultHashDirectory (a fixed path like /usr/local/etc/...)
// for hash verification. This test creates a config file in a temp directory, which won't
// have a corresponding hash file in the default hash directory, causing a "hash file not found" error.
func TestE2E_PreExecutionError_HashNotFound(t *testing.T) {
	// Create a valid config file in a temp directory
	// This file won't have a corresponding hash in the default hash directory
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	validTOML := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`
	err := os.WriteFile(configFile, []byte(validTOML), 0o644)
	require.NoError(t, err)

	// Run the runner in non-dry-run mode (requires hash verification)
	// Since the config file is in a temp directory, there's no hash file for it
	// in the default hash directory, causing verification to fail
	cmd := exec.Command("go", "run", ".", "-config", configFile)
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	// Command should fail with exit code 1 due to hash file not found
	require.Error(t, err, "runner should fail when config file hash is not found")

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1")

	// Verify stderr contains error information from HandlePreExecutionError
	stderrOutput := stderr.String()
	assert.Contains(t, stderrOutput, "Error:", "stderr should contain 'Error:' prefix")
	// The error could be file_access_failed (hash not found) or similar
	assert.True(t,
		strings.Contains(stderrOutput, "file_access_failed") ||
			strings.Contains(stderrOutput, "verification") ||
			strings.Contains(stderrOutput, "hash"),
		"stderr should indicate file access or verification failure: %s", stderrOutput)

	// Verify stdout contains RUN_SUMMARY (from HandlePreExecutionError)
	stdoutOutput := stdout.String()
	assert.Contains(t, stdoutOutput, "RUN_SUMMARY", "stdout should contain RUN_SUMMARY")
	assert.Contains(t, stdoutOutput, "status=pre_execution_error", "stdout should indicate pre_execution_error status")
}

// TestE2E_PreExecutionError_MissingConfigFile tests that missing config file errors
// result in HandlePreExecutionError being called.
func TestE2E_PreExecutionError_MissingConfigFile(t *testing.T) {
	// Run the runner without -config flag
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Command should fail with exit code 1
	require.Error(t, err, "runner should fail without config file")

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1")

	// Verify stderr contains error information from HandlePreExecutionError
	stderrOutput := stderr.String()
	assert.Contains(t, stderrOutput, "Error:", "stderr should contain 'Error:' prefix")
	assert.Contains(t, stderrOutput, "required_argument_missing", "stderr should indicate required argument missing")

	// Verify stdout contains RUN_SUMMARY (from HandlePreExecutionError)
	stdoutOutput := stdout.String()
	assert.Contains(t, stdoutOutput, "RUN_SUMMARY", "stdout should contain RUN_SUMMARY")
	assert.Contains(t, stdoutOutput, "status=pre_execution_error", "stdout should indicate pre_execution_error status")
}

// TestE2E_PreExecutionError_NonExistentConfigFile tests that non-existent config file errors
// result in HandlePreExecutionError being called.
func TestE2E_PreExecutionError_NonExistentConfigFile(t *testing.T) {
	// Run the runner with a non-existent config file
	cmd := exec.Command("go", "run", ".", "-config", "/nonexistent/path/to/config.toml")
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Command should fail with exit code 1
	require.Error(t, err, "runner should fail with non-existent config file")

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1")

	// Verify stderr contains error information from HandlePreExecutionError
	stderrOutput := stderr.String()
	assert.Contains(t, stderrOutput, "Error:", "stderr should contain 'Error:' prefix")
	// Could be file_access_failed or similar
	assert.True(t,
		strings.Contains(stderrOutput, "file_access_failed") ||
			strings.Contains(stderrOutput, "verification"),
		"stderr should indicate file access failure: %s", stderrOutput)

	// Verify stdout contains RUN_SUMMARY (from HandlePreExecutionError)
	stdoutOutput := stdout.String()
	assert.Contains(t, stdoutOutput, "RUN_SUMMARY", "stdout should contain RUN_SUMMARY")
	assert.Contains(t, stdoutOutput, "status=pre_execution_error", "stdout should indicate pre_execution_error status")
}
