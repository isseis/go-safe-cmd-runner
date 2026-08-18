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

// TestE2E_PreExecutionError_TOMLParseError verifies that a TOML parse error
// reaches the user through HandlePreExecutionError. Dry-run mode skips hash
// verification, so the parse error is what fails the run.
func TestE2E_PreExecutionError_TOMLParseError(t *testing.T) {
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

	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run")
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	require.Error(t, err, "runner should fail with invalid TOML")

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

// TestE2E_PreExecutionError_HashNotFound verifies that a failed hash
// verification reaches the user through HandlePreExecutionError. Hashes are
// looked up in cmdcommon.DefaultHashDirectory, so a config in a temp directory
// has none and verification fails.
func TestE2E_PreExecutionError_HashNotFound(t *testing.T) {
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

	// No -dry-run: hash verification only runs on a real execution.
	cmd := exec.Command("go", "run", ".", "-config", configFile)
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	require.Error(t, err, "runner should fail when config file hash is not found")

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1")

	stderrOutput := stderr.String()
	assert.Contains(t, stderrOutput, "Error:", "stderr should contain 'Error:' prefix")
	// The wording depends on which verification step rejects it first.
	assert.True(t,
		strings.Contains(stderrOutput, "file_access_failed") ||
			strings.Contains(stderrOutput, "verification") ||
			strings.Contains(stderrOutput, "hash"),
		"stderr should indicate file access or verification failure: %s", stderrOutput)

	stdoutOutput := stdout.String()
	assert.Contains(t, stdoutOutput, "RUN_SUMMARY", "stdout should contain RUN_SUMMARY")
	assert.Contains(t, stdoutOutput, "status=pre_execution_error", "stdout should indicate pre_execution_error status")
}

// TestE2E_PreExecutionError_MissingConfigFile verifies that omitting -config
// reaches the user through HandlePreExecutionError.
func TestE2E_PreExecutionError_MissingConfigFile(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	require.Error(t, err, "runner should fail without config file")

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1")

	stderrOutput := stderr.String()
	assert.Contains(t, stderrOutput, "Error:", "stderr should contain 'Error:' prefix")
	assert.Contains(t, stderrOutput, "required_argument_missing", "stderr should indicate required argument missing")

	stdoutOutput := stdout.String()
	assert.Contains(t, stdoutOutput, "RUN_SUMMARY", "stdout should contain RUN_SUMMARY")
	assert.Contains(t, stdoutOutput, "status=pre_execution_error", "stdout should indicate pre_execution_error status")
}

// TestE2E_PreExecutionError_NonExistentConfigFile verifies that a config path
// that does not exist reaches the user through HandlePreExecutionError.
func TestE2E_PreExecutionError_NonExistentConfigFile(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "-config", "/nonexistent/path/to/config.toml")
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	require.Error(t, err, "runner should fail with non-existent config file")

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1")

	stderrOutput := stderr.String()
	assert.Contains(t, stderrOutput, "Error:", "stderr should contain 'Error:' prefix")
	// The wording depends on which verification step rejects it first.
	assert.True(t,
		strings.Contains(stderrOutput, "file_access_failed") ||
			strings.Contains(stderrOutput, "verification"),
		"stderr should indicate file access failure: %s", stderrOutput)

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
	// boundary check is removed, so no earlier helper may abort the test before
	// them. The rejection happens before logging is set up, so nothing is written.
	entries, err := os.ReadDir(logDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no file should be created in the log directory")

	// Without the boundary check the path is filepath.Join(logDir,
	// "<hostname>_<timestamp>_../../etc/cron.d/evil.json"); Join collapses the
	// "<hostname>_<timestamp>_.." segment against the following "..", so the
	// escape lands inside logDir — hence checking logDir, not its parent.
	_, err = os.Stat(filepath.Join(logDir, "etc"))
	assert.True(t, os.IsNotExist(err), "no 'etc' entry should be created in the log directory")

	assert.Contains(t, stderrOutput, string(logging.ErrorTypeInvalidRunID),
		"stderr should identify the error as an invalid run ID")
	assert.Contains(t, stderrOutput, logging.RunIDFormatDescription(),
		"stderr should tell the user which format is accepted")

	// The rejected value would surface on stderr (the console log stream); stdout
	// is checked too because RUN_SUMMARY goes there.
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

// slackEnvVarPrefix covers every Slack variable the runner reads (declared in
// internal/logging). Stripping by prefix rather than by an enumerated list also
// removes variables added later, so keep new ones under this prefix.
const slackEnvVarPrefix = "GSCR_SLACK_"

// envWithoutSlackVars returns the environment with every Slack variable removed.
// Tests that set up a specific Slack configuration must start from this: a
// machine that exports GSCR_SLACK_WEBHOOK_URL_ERROR would otherwise satisfy the
// validation under test and send the run down a different failure path.
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
// configuration error reaches the user exactly once as human-readable text,
// with its remediation instructions intact.
//
// The same message also appears as the error_message attribute of a structured
// log line, so the count is taken over the lines without that attribute key
// (duplication tracked in
// https://github.com/isseis/go-safe-cmd-runner/issues/1020). The filter cannot
// key on "level=ERROR" instead: this line comes from slog's default handler,
// before SetupLogging installs a TextHandler, and it writes the level as a bare
// word.
func TestE2E_SlackWebhookEnvErrorPrintedOnce(t *testing.T) {
	// ValidateSlackWebhookEnv runs before the config path is read; the config
	// file and -dry-run are here only for parity with the sibling tests.
	configFile := setupTempConfig(t, validRunIDTestConfig)

	cmd := newGoRunCmd(t, "-config", configFile, "-dry-run")
	cmd.Env = append(envWithoutSlackVars(),
		logging.SlackWebhookURLSuccessEnvVar+"=https://hooks.slack.com/services/T000/B000/SUCCESS")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.Error(t, err, "runner should fail when only the success webhook is set")
	// Checked first: a stray Slack variable in the child would fail the run
	// elsewhere, leaving the assertions below reading another path's output.
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
	// Self-check: if the filter stopped matching the structured line, the count
	// below would include it and no longer measure the human-readable block.
	require.NotEqual(t, len(allLines), len(humanLines),
		"expected at least one structured log line carrying %q, got:\n%s", structuredAttrKey, stderrOutput)

	occurrences := 0
	for _, line := range humanLines {
		occurrences += strings.Count(line, humanMessage)
	}
	assert.Equal(t, 1, occurrences,
		"the guidance should reach the user exactly once outside the structured log line, got:\n%s", stderrOutput)

	// Also checked over the filtered block: the structured line carries the whole
	// message as an attribute value, so asserting over raw stderr would pass even
	// if the human-readable block were truncated to its first line — the very
	// loss these assertions exist to catch.
	humanOutput := strings.Join(humanLines, "\n")
	assert.Contains(t, humanOutput, "  export "+logging.SlackWebhookURLErrorEnvVar+`="<your_webhook_url>"`,
		"the human-readable block should keep the remediation command")
	assert.Contains(t, humanOutput, "To use the same webhook for both success and error notifications:",
		"the human-readable block should keep the whole guidance, not just its first line")
}
