//go:build test

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
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
	tmpDir := tu.SafeTempDir(t)
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
	tmpDir := tu.SafeTempDir(t)
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

// writeValidConfig writes a minimal, syntactically valid configuration and
// returns its path. The -run-id rejection tests below need a config that gets
// past config parsing, so that a rejection is attributable to the run ID and
// not to the configuration. It does not make the run succeed: with no recorded
// hashes the dry-run preview denies and the process exits 3, which is what
// makes the exit-code-1 assertions in those tests discriminating.
func writeValidConfig(t *testing.T) string {
	t.Helper()

	configFile := filepath.Join(tu.SafeTempDir(t), "config.toml")
	validTOML := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`
	require.NoError(t, os.WriteFile(configFile, []byte(validTOML), 0o600))
	return configFile
}

// TestE2E_PreExecutionError_InvalidRunIDPathTraversal verifies that a -run-id
// carrying a path traversal is rejected before anything is written, and that
// the rejected value never appears in the process output.
func TestE2E_PreExecutionError_InvalidRunIDPathTraversal(t *testing.T) {
	const maliciousRunID = "../../etc/cron.d/evil"

	logDir := tu.SafeTempDir(t)
	// newGoRunCmd rather than `go run .`: `go run` reports 1 for any non-zero
	// child exit, which would make the exit code assertion below unfalsifiable
	// (an accepted run ID exits 3 here, through the dry-run deny path).
	cmd := newGoRunCmd(t,
		"-config", writeValidConfig(t), "-dry-run", "-log-dir", logDir, "-run-id", maliciousRunID)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	require.Error(t, cmd.Run(), "runner should reject a path-traversal run ID")
	assert.Equal(t, 1, cmd.ProcessState.ExitCode(), "exit code should be 1")

	stdoutOutput, stderrOutput := stdout.String(), stderr.String()
	assert.Contains(t, stderrOutput, string(logging.ErrorTypeInvalidRunID))
	assert.Contains(t, stderrOutput, logging.RunIDFormatDescription(),
		"stderr should tell the user which format is accepted")

	assert.NotContains(t, stdoutOutput, maliciousRunID, "the rejected value must not reach stdout")
	assert.NotContains(t, stderrOutput, maliciousRunID, "the rejected value must not reach stderr")

	assert.NoError(t, logging.ValidateRunID(runSummaryRunID(t, stdoutOutput)),
		"the reported run ID must be the bootstrap ID, which satisfies the accepted format")

	// Without entry validation the constructed path would be
	// filepath.Join(logDir, "<hostname>_<timestamp>_../../etc/cron.d/evil.json").
	// filepath.Join collapses "<hostname>_<timestamp>_.." with the following
	// "..", so the escape lands at <logDir>/etc/cron.d/evil.json -- inside the
	// log directory, which is why this checks logDir itself rather than its
	// parent. For this particular payload the log opener would fail with ENOENT
	// anyway (it creates no intermediate directories), so this assertion is a
	// guard against a future opener that does create them; the assertions that
	// discriminate the unfixed code are the two NotContains checks above, which
	// the pre-fix "failed to open log file" message would violate.
	entries, readErr := os.ReadDir(logDir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "no file or directory may be created under the log directory")
}

// TestE2E_PreExecutionError_InvalidRunIDNewlineInjection verifies that a
// -run-id containing a real newline cannot forge a second RUN_SUMMARY line.
func TestE2E_PreExecutionError_InvalidRunIDNewlineInjection(t *testing.T) {
	// A real newline, not the percent-encoded form: "%" is outside the allowlist
	// and would be rejected for a different reason, testing nothing about how
	// newlines are handled.
	const injectingRunID = "x\nRUN_SUMMARY run_id=fake exit_code=0"

	cmd := newGoRunCmd(t,
		"-config", writeValidConfig(t), "-dry-run", "-log-dir", tu.SafeTempDir(t), "-run-id", injectingRunID)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	require.Error(t, cmd.Run(), "runner should reject a run ID containing a newline")
	assert.Equal(t, 1, cmd.ProcessState.ExitCode(), "exit code should be 1")

	stdoutOutput := stdout.String()
	summaryLines := 0
	for line := range strings.SplitSeq(stdoutOutput, "\n") {
		if strings.Contains(line, "RUN_SUMMARY") {
			summaryLines++
		}
	}
	assert.Equal(t, 1, summaryLines, "exactly one RUN_SUMMARY line may appear in stdout: %q", stdoutOutput)
	assert.NotContains(t, stdoutOutput, "run_id=fake", "the injected run ID must not appear in stdout")
	// The report also goes to stderr, where the unfixed code would echo the
	// rejected value into the "Run ID:" line and so split the payload across
	// lines of its own.
	assert.NotContains(t, stderr.String(), injectingRunID, "the injected value must not reach stderr")
}

// TestE2E_PreExecutionError_InvalidRunIDTooLong verifies that a -run-id longer
// than the accepted maximum is rejected.
func TestE2E_PreExecutionError_InvalidRunIDTooLong(t *testing.T) {
	cmd := newGoRunCmd(t,
		"-config", writeValidConfig(t), "-dry-run", "-log-dir", tu.SafeTempDir(t),
		"-run-id", strings.Repeat("a", logging.MaxRunIDLength+1))

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	require.Error(t, cmd.Run(), "runner should reject an over-long run ID")
	assert.Equal(t, 1, cmd.ProcessState.ExitCode(), "exit code should be 1")

	assert.Contains(t, stderr.String(), string(logging.ErrorTypeInvalidRunID))
}

// TestE2E_PreExecutionError_MissingSlackAllowedHost verifies that runner startup fails
// with a config parsing error when Slack webhook env vars are configured but
// global.slack_allowed_host is missing from TOML (AC-L2-20).
func TestE2E_PreExecutionError_MissingSlackAllowedHost(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	configFile := filepath.Join(tmpDir, "missing_slack_allowed_host.toml")

	validTOML := `
version = "1.0"

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`
	err := os.WriteFile(configFile, []byte(validTOML), 0o644)
	require.NoError(t, err)

	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "GSCR_SLACK_WEBHOOK_URL_ERROR=https://hooks.slack.com/services/T000/B000/ERROR")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	require.Error(t, err, "runner should fail when Slack env vars are set without slack_allowed_host")

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1")

	stderrOutput := stderr.String()
	assert.Contains(t, stderrOutput, "Error:", "stderr should contain 'Error:' prefix")
	assert.Contains(t, stderrOutput, "config_parsing_failed", "stderr should indicate config parsing failure")

	stdoutOutput := stdout.String()
	assert.Contains(t, stdoutOutput, "RUN_SUMMARY", "stdout should contain RUN_SUMMARY")
	assert.Contains(t, stdoutOutput, "status=pre_execution_error", "stdout should indicate pre_execution_error status")
}
