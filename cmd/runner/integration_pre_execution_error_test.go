//go:build test

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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
	// Same isolation as TestE2E_SlackWebhookEnvErrorPrintedOnce: a machine that
	// exports another GSCR_SLACK_ variable would otherwise send this run down a
	// different failure path.
	cmd.Env = append(envWithoutSlackVars(), logging.SlackWebhookURLErrorEnvVar+"=https://hooks.slack.com/services/T000/B000/ERROR")

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

// validRunIDTestConfig is a config the runner can parse and preview, so that a
// --run-id rejection is the only reason these tests see a failure.
const validRunIDTestConfig = `
version = "1.0"

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

// TestE2E_PreExecutionError_InvalidRunIDPathTraversal verifies that a --run-id
// attempting to escape the log directory is rejected before any output path
// sees it, and that no file is written anywhere under the log directory.
func TestE2E_PreExecutionError_InvalidRunIDPathTraversal(t *testing.T) {
	const maliciousRunID = "../../etc/cron.d/evil"

	configFile := setupTempConfig(t, validRunIDTestConfig)
	logDir := tu.SafeTempDir(t)

	cmd := newGoRunCmd(t, "-config", configFile, "-dry-run",
		"-log-dir", logDir, "-run-id", maliciousRunID)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.Error(t, err, "runner should reject a path-traversal run ID")
	requireExitCode(t, cmd, 1)

	stdoutOutput := stdout.String()
	stderrOutput := stderr.String()

	// The filesystem assertions come first: they are the ones that fail if the
	// boundary check is removed, and a later helper that aborts the test must
	// not be able to skip them.
	//
	// The log directory must be untouched, because the rejection happens before
	// logging is set up at all.
	entries, err := os.ReadDir(logDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no file should be created in the log directory")

	// Without the boundary check, the constructed path would be
	// filepath.Join(logDir, "<hostname>_<timestamp>_../../etc/cron.d/evil.json").
	// filepath.Join collapses "<hostname>_<timestamp>_.." against the following
	// "..", so the escape lands at <logDir>/etc/cron.d/evil.json — inside the
	// log directory, which is why this checks logDir itself rather than its parent.
	_, err = os.Stat(filepath.Join(logDir, "etc"))
	assert.True(t, os.IsNotExist(err), "no 'etc' entry should be created in the log directory")

	assert.Contains(t, stderrOutput, string(logging.ErrorTypeInvalidRunID),
		"stderr should identify the error as an invalid run ID")
	assert.Contains(t, stderrOutput, logging.RunIDFormatDescription(),
		"stderr should tell the user which format is accepted")

	// The console log stream is stderr, so stderr is where the rejected value
	// would surface; stdout is checked too because RUN_SUMMARY goes there.
	assert.NotContains(t, stderrOutput, maliciousRunID, "stderr must not echo the rejected value")
	assert.NotContains(t, stdoutOutput, maliciousRunID, "stdout must not echo the rejected value")

	assert.NoError(t, logging.ValidateRunID(runSummaryRunID(t, stdoutOutput)),
		"the run ID reported in RUN_SUMMARY must itself be a valid run ID")
}

// TestE2E_PreExecutionError_InvalidRunIDNewlineInjection verifies that a
// --run-id carrying a real newline cannot forge a second RUN_SUMMARY line.
func TestE2E_PreExecutionError_InvalidRunIDNewlineInjection(t *testing.T) {
	const injectedRunID = "x\nRUN_SUMMARY run_id=fake exit_code=0"

	configFile := setupTempConfig(t, validRunIDTestConfig)

	cmd := newGoRunCmd(t, "-config", configFile, "-dry-run", "-run-id", injectedRunID)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.Error(t, err, "runner should reject a run ID containing a newline")
	requireExitCode(t, cmd, 1)

	stdoutOutput := stdout.String()

	var summaryLines int
	for line := range strings.SplitSeq(stdoutOutput, "\n") {
		if strings.Contains(line, "RUN_SUMMARY") {
			summaryLines++
		}
	}
	assert.Equal(t, 1, summaryLines, "exactly one RUN_SUMMARY line should be printed, got:\n%s", stdoutOutput)
	assert.NotContains(t, stdoutOutput, "run_id=fake", "the injected run ID must not appear")
}

// TestE2E_PreExecutionError_InvalidRunIDTooLong verifies that the length bound
// is enforced at the process boundary, not just in unit tests.
func TestE2E_PreExecutionError_InvalidRunIDTooLong(t *testing.T) {
	configFile := setupTempConfig(t, validRunIDTestConfig)

	cmd := newGoRunCmd(t, "-config", configFile, "-dry-run",
		"-run-id", strings.Repeat("a", logging.MaxRunIDLength+1))

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.Error(t, err, "runner should reject an over-long run ID")
	requireExitCode(t, cmd, 1)

	assert.Contains(t, stderr.String(), string(logging.ErrorTypeInvalidRunID),
		"stderr should identify the error as an invalid run ID")
}

// slackEnvVarPrefix is the prefix every Slack-related environment variable the
// runner reads currently carries (the webhook URLs, the send and flush timeouts,
// and the sync switch, all declared in internal/logging). Stripping by prefix
// rather than by an enumerated list is deliberate: it also removes a Slack
// variable added later, which an enumerated list would silently miss. A future
// Slack variable named outside this prefix would defeat it -- keep new ones
// under GSCR_SLACK_.
const slackEnvVarPrefix = "GSCR_SLACK_"

// envWithoutSlackVars returns the current environment with every Slack-related
// variable removed. Tests that depend on a specific Slack configuration must
// start from this rather than os.Environ(), because a developer or CI machine
// that exports GSCR_SLACK_WEBHOOK_URL_ERROR would otherwise make the validation
// under test succeed and send the run down a different failure path.
func envWithoutSlackVars() []string {
	env := os.Environ()
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, slackEnvVarPrefix) {
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}

// TestE2E_SlackWebhookEnvErrorPrintedOnce verifies that the Slack webhook
// configuration error reaches the user exactly once as human-readable text, and
// that folding the direct print into the pre-execution error path did not lose
// the remediation instructions or change the exit code.
//
// The message also appears a second time inside the structured log line that
// the same path emits (as the value of the error_message attribute), so the
// count is taken over the lines that do not carry that attribute key. That
// remaining duplication is tracked in
// https://github.com/isseis/go-safe-cmd-runner/issues/1020; when it is fixed the
// filter becomes a no-op rather than wrong. The filter must key on the
// attribute name and not on "level=ERROR": this output is produced by slog's
// built-in default handler, before SetupLogging installs a TextHandler, and
// that handler writes the level as a bare word with no "level=" prefix.
func TestE2E_SlackWebhookEnvErrorPrintedOnce(t *testing.T) {
	// The config file and -dry-run are here only for parity with the sibling
	// tests: ValidateSlackWebhookEnv runs before the config path is even looked
	// at, so this run fails before anything reads them.
	configFile := setupTempConfig(t, validRunIDTestConfig)

	cmd := newGoRunCmd(t, "-config", configFile, "-dry-run")
	cmd.Env = append(envWithoutSlackVars(),
		logging.SlackWebhookURLSuccessEnvVar+"=https://hooks.slack.com/services/T000/B000/SUCCESS")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.Error(t, err, "runner should fail when only the success webhook is set")
	// Checked first: if a stray Slack variable leaked into the child, the run
	// fails somewhere else entirely and the assertions below would be reading
	// output from a different code path.
	requireExitCode(t, cmd, 1)

	stderrOutput := stderr.String()
	const humanMessage = "GSCR_SLACK_WEBHOOK_URL_SUCCESS is set but GSCR_SLACK_WEBHOOK_URL_ERROR is not."
	structuredAttrKey := common.PreExecErrorAttrs.ErrorMessage + "="

	allLines := strings.Split(stderrOutput, "\n")
	humanLines := make([]string, 0, len(allLines))
	for _, line := range allLines {
		if strings.Contains(line, structuredAttrKey) {
			continue
		}
		humanLines = append(humanLines, line)
	}
	// Self-check on the filter: if it stopped matching the structured line, the
	// count below would silently include it and this test would no longer be
	// measuring the human-readable block.
	require.NotEqual(t, len(allLines), len(humanLines),
		"expected at least one structured log line carrying %q, got:\n%s", structuredAttrKey, stderrOutput)

	occurrences := 0
	for _, line := range humanLines {
		occurrences += strings.Count(line, humanMessage)
	}
	assert.Equal(t, 1, occurrences,
		"the guidance should reach the user exactly once outside the structured log line, got:\n%s", stderrOutput)

	// The remediation lines are checked over the filtered block for the same
	// reason as the count: the structured log line carries the whole message as
	// an escaped attribute value, so asserting over the raw stderr would still
	// pass if the human-readable block were truncated to its first line - which
	// is exactly the loss this assertion exists to catch. More than one line is
	// checked because the guidance is multi-line.
	humanOutput := strings.Join(humanLines, "\n")
	assert.Contains(t, humanOutput, "  export "+logging.SlackWebhookURLErrorEnvVar+`="<your_webhook_url>"`,
		"the human-readable block should keep the remediation command")
	assert.Contains(t, humanOutput, "To use the same webhook for both success and error notifications:",
		"the human-readable block should keep the whole guidance, not just its first line")
}
