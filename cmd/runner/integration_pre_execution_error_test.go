//go:build test

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
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

// TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted pins that
// startup accepts group and command names the redaction transformation
// rewrites. Identifying and rejecting those names at startup is the behavior
// this task removed: a group named "monkey" and a command named
// AKIAIOSFODNN7EXAMPLE must reach dry-run verification, which fails there with
// DryRunExitVerificationUnavailable (no hash records exist for this temp
// config), not with a config_parsing_failed pre-execution error.
func TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted(t *testing.T) {
	const (
		groupName   = "monkey"
		commandName = "AKIAIOSFODNN7EXAMPLE"
	)

	configFile := setupTempConfig(t, `
version = "1.0"

[[groups]]
name = "`+groupName+`"

[[groups.commands]]
name = "`+commandName+`"
cmd = "/bin/echo"
args = ["hello"]
`)

	cmd := newGoRunCmd(t, "-config", configFile, "-dry-run")
	cmd.Env = envWithoutSlackVars()

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.Error(t, err, "runner should stop at dry-run verification, not proceed without hashes")
	requireExitCode(t, cmd, resource.DryRunExitVerificationUnavailable)

	stderrOutput := stderr.String()
	assert.NotContains(t, stderrOutput, string(logging.ErrorTypeConfigParsing),
		"a name redaction rewrites must not be rejected as a config parsing error")
	assert.NotContains(t, stderrOutput, "Identifier redaction validation failed")
	assert.NotContains(t, stdout.String(), "status=pre_execution_error")
}

// TestE2E_PreExecutionError_MissingSlackAllowedHost verifies that runner startup fails
// with a config parsing error when Slack webhook env vars are configured but
// global.slack_allowed_host is missing from TOML.
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

// tomlStringArray returns the elements of a TOML array of the given strings,
// without the brackets. Each string is quoted with %q, which matches TOML
// basic-string escaping only for the control-free temporary paths these tests
// use.
func tomlStringArray(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return strings.Join(quoted, ", ")
}

// globalVerificationConfig returns a configuration that lists verifyFiles as
// global verify files and carries one group that never runs, the shape the
// global verification tests share. Global verification fails before any group
// is verified, so the group's command is not part of the failure list.
func globalVerificationConfig(slackHost string, verifyFiles []string) string {
	return fmt.Sprintf(`
version = "1.0"

[global]
slack_allowed_host = %q
verify_files = [%s]

[[groups]]
name = "unused_group"

[[groups.commands]]
name = "unused-cmd"
cmd = "/bin/true"
`, slackHost, tomlStringArray(verifyFiles))
}

// TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope drives
// the production reporting boundary (mainWithExitCode) up to the global target
// file verification failure. The config hash is recorded in a temporary hash
// directory, so the failure comes from the unrecorded global verify file after
// the Slack handler is registered -- the same call site main.go reports with a
// global scope. The report must use the template and Component the group
// report uses, name the failed file only in the Files section, and keep the
// path out of the human-readable stderr report.
func TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	unhashedFile := filepath.Join(tmpDir, "unhashed.txt")
	require.NoError(t, os.WriteFile(unhashedFile, []byte("no hash record for this file"), 0o600))

	const runIDValue = "test-global-scope-001"
	run := runMainWithSlackMock(t, slackRunSpec{
		configBody: func(slackHost string) string {
			return globalVerificationConfig(slackHost, []string{unhashedFile})
		},
		runID: runIDValue,
	})
	require.Equal(t, 1, run.exitCode, "the global verification failure must exit non-zero")

	// Checked before the require-based payload assertions below, so a path in
	// the human-readable report is reported even when the payload is also wrong.
	details := stderrDetailsLine(t, run.stderr)
	assert.NotContains(t, details, unhashedFile)
	assert.NotContains(t, details, "Files:")

	message, fields := requireSinglePreExecutionError(t, run)
	assert.Equal(t, "[go-safe-cmd-runner] ❌ *ERROR* — (global) : file_access_failed", message.Text)

	require.GreaterOrEqual(t, len(fields), 3, "the envelope always appends three fields")
	assert.Equal(t, "Scope", fields[len(fields)-3].Title)
	assert.Equal(t, "(global)", fields[len(fields)-3].Value)
	assert.Equal(t, "Hostname", fields[len(fields)-2].Title)
	assert.Equal(t, "Run ID", fields[len(fields)-1].Title)
	assert.Equal(t, runIDValue, fields[len(fields)-1].Value)

	assert.Equal(t, "verification", attachmentField(t, fields, "Component"))
	assert.Equal(t,
		fmt.Sprintf("Total: 1, Verified: 0, Failed: 1, Error: global file verification failed, Files: %q", unhashedFile),
		attachmentField(t, fields, "Error Message"))
}

// TestIntegration_GlobalTargetFileVerificationFailureTruncatesLongList drives
// a global verification failure whose failed-file list exceeds the free-text
// limit: the global report must apply the same budget as the group report,
// showing sorted elements whole and counting the rest in "(+m more)".
func TestIntegration_GlobalTargetFileVerificationFailureTruncatesLongList(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	unhashed := unhashedLongNameFiles(t, tmpDir)

	run := runMainWithSlackMock(t, slackRunSpec{
		configBody: func(slackHost string) string {
			return globalVerificationConfig(slackHost, unhashed)
		},
		runID: "test-global-long-list-001",
	})
	require.Equal(t, 1, run.exitCode, "the global verification failure must exit non-zero")

	// Checked before the require-based list assertions below, so a path in
	// the human-readable report is reported even when the list is also wrong.
	details := stderrDetailsLine(t, run.stderr)
	assert.NotContains(t, details, tmpDir)

	_, fields := requireSinglePreExecutionError(t, run)
	n := len(unhashed)
	requireSortedPrefixWithOmissionCount(t,
		attachmentField(t, fields, "Error Message"),
		fmt.Sprintf("Total: %d, Verified: 0, Failed: %d, Error: global file verification failed, Files: ", n, n),
		slices.Sorted(slices.Values(unhashed)))
}

// unhashedLongNameFiles creates, in dir, more same-length unrecorded files
// than the free-text limit can list and returns them in descending order, so
// a sorted report order must come from sorting. The equal lengths keep the
// builder from skipping one path and keeping a later one, which is what lets
// requireSortedPrefixWithOmissionCount expect a prefix of the sorted list.
func unhashedLongNameFiles(t *testing.T, dir string) []string {
	t.Helper()
	const n = 16
	files := make([]string, n)
	for i := range files {
		files[i] = filepath.Join(dir, fmt.Sprintf("snapshot-with-a-long-name-%02d.tar", n-1-i))
		require.NoError(t, os.WriteFile(files[i], []byte("no hash record"), 0o600))
	}
	return files
}

// requireSortedPrefixWithOmissionCount asserts that errorMessage is prefix
// followed by a leading run of sorted, each element quoted whole, and a
// " (+m more)" notice whose m counts the elements left out.
func requireSortedPrefixWithOmissionCount(t *testing.T, errorMessage, prefix string, sorted []string) {
	t.Helper()
	require.True(t, strings.HasPrefix(errorMessage, prefix), "unexpected Error Message: %q", errorMessage)

	match := regexp.MustCompile(`^(.*) \(\+(\d+) more\)$`).FindStringSubmatch(strings.TrimPrefix(errorMessage, prefix))
	require.NotNil(t, match, "a list this long must carry an omission notice: %q", errorMessage)
	omitted, err := strconv.Atoi(match[2])
	require.NoError(t, err)
	shown := strings.Split(match[1], ", ")
	require.LessOrEqual(t, len(shown), len(sorted), "more paths shown than failed: %q", errorMessage)
	for i, q := range shown {
		assert.Equal(t, fmt.Sprintf("%q", sorted[i]), q, "shown path %d must be the sorted element, whole", i)
	}
	assert.Equal(t, len(sorted), len(shown)+omitted, "shown + omitted must equal the list length")
	assert.Positive(t, omitted)
}

// groupVerificationConfig returns a configuration whose only group lists
// verifyFiles and runs the "true" coreutil, the shape the group verification
// tests share. The command's resolved path is what the group verifies, so the
// caller records its hash to keep the failure list to verifyFiles.
func groupVerificationConfig(slackHost string, verifyFiles []string) string {
	return fmt.Sprintf(`
version = "1.0"

[global]
slack_allowed_host = %q

[[groups]]
name = "backup"
verify_files = [%s]

[[groups.commands]]
name = "noop"
cmd = %q
`, slackHost, tomlStringArray(verifyFiles), trueCmdPath())
}

// resolvedTruePath returns the canonical path of the "true" coreutil, the
// path the group verification records and verifies.
func resolvedTruePath(t *testing.T) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(trueCmdPath())
	require.NoError(t, err)
	return resolved
}

// TestIntegration_GroupFileVerificationFailureListsFailedFiles drives a group
// verification failure end to end: two unrecorded verify files, listed in a
// non-ascending order, must reach Slack as a sorted Files section of the Error
// Message while the human-readable stderr report names no path.
func TestIntegration_GroupFileVerificationFailureListsFailedFiles(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	unhashedB := filepath.Join(tmpDir, "b.txt")
	unhashedA := filepath.Join(tmpDir, "a.txt")
	for _, file := range []string{unhashedB, unhashedA} {
		require.NoError(t, os.WriteFile(file, []byte("no hash record"), 0o600))
	}

	run := runMainWithSlackMock(t, slackRunSpec{
		configBody: func(slackHost string) string {
			return groupVerificationConfig(slackHost, []string{unhashedB, unhashedA})
		},
		hashedFiles: []string{resolvedTruePath(t)},
		runID:       "test-group-files-001",
	})
	require.Equal(t, 0, run.exitCode, "a group verification failure is reported and the run continues")

	message, fields := requireSinglePreExecutionError(t, run)
	assert.Equal(t, "[go-safe-cmd-runner] ❌ *ERROR* — group=backup : group_file_verification_failed", message.Text)
	assert.Equal(t, "group=backup", attachmentField(t, fields, "Scope"))
	assert.Equal(t, "verification", attachmentField(t, fields, "Component"))

	errorMessage := attachmentField(t, fields, "Error Message")
	assert.Equal(t,
		fmt.Sprintf("Total: 3, Verified: 1, Failed: 2, Error: group file verification failed, Files: %q, %q", unhashedA, unhashedB),
		errorMessage)
	assert.NotContains(t, errorMessage, "Group: backup", "the group name belongs to the scope only")

	details := stderrDetailsLine(t, run.stderr)
	assert.NotContains(t, details, unhashedA)
	assert.NotContains(t, details, unhashedB)
	assert.NotContains(t, details, "Files:")
}

// TestIntegration_GroupFileVerificationFailureTruncatesLongList drives a group
// verification failure whose failed-file list exceeds the free-text limit:
// the Error Message must show sorted elements whole and count the rest in
// "(+m more)" with m = n - shown.
func TestIntegration_GroupFileVerificationFailureTruncatesLongList(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	unhashed := unhashedLongNameFiles(t, tmpDir)

	run := runMainWithSlackMock(t, slackRunSpec{
		configBody: func(slackHost string) string {
			return groupVerificationConfig(slackHost, unhashed)
		},
		hashedFiles: []string{resolvedTruePath(t)},
		runID:       "test-group-long-list-001",
	})
	require.Equal(t, 0, run.exitCode, "a group verification failure is reported and the run continues")

	// Checked before the require-based list assertions below, so a path in
	// the human-readable report is reported even when the list is also wrong.
	details := stderrDetailsLine(t, run.stderr)
	assert.NotContains(t, details, tmpDir)

	_, fields := requireSinglePreExecutionError(t, run)
	n := len(unhashed)
	requireSortedPrefixWithOmissionCount(t,
		attachmentField(t, fields, "Error Message"),
		fmt.Sprintf("Total: %d, Verified: 1, Failed: %d, Error: group file verification failed, Files: ", n+1, n),
		slices.Sorted(slices.Values(unhashed)))
}

// TestIntegration_GroupCollectionFailureListsUnresolvedTargets drives the
// collection-failure path: two commands whose absolute paths do not exist make
// the group fail before any file is verified, and the report must name the
// unresolved targets only in the Files section, with the collection-stage
// template instead of a verification summary.
func TestIntegration_GroupCollectionFailureListsUnresolvedTargets(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	missingB := filepath.Join(tmpDir, "missing-b")
	missingA := filepath.Join(tmpDir, "missing-a")

	// A third, resolvable command keeps the unresolved count below the total,
	// so the two counts of the template cannot be swapped unnoticed. No hash
	// is recorded for it: collection aborts before any file is verified.
	run := runMainWithSlackMock(t, slackRunSpec{
		configBody: func(slackHost string) string {
			return fmt.Sprintf(`
version = "1.0"

[global]
slack_allowed_host = %q

[[groups]]
name = "backup"

[[groups.commands]]
name = "first"
cmd = %q

[[groups.commands]]
name = "second"
cmd = %q

[[groups.commands]]
name = "resolvable"
cmd = %q
`, slackHost, missingB, missingA, trueCmdPath())
		},
		runID: "test-group-collection-001",
	})
	require.Equal(t, 0, run.exitCode, "a collection failure is reported and the run continues")

	message, fields := requireSinglePreExecutionError(t, run)
	assert.Equal(t, "[go-safe-cmd-runner] ❌ *ERROR* — group=backup : group_file_verification_failed", message.Text)
	assert.Equal(t, "group=backup", attachmentField(t, fields, "Scope"))
	assert.Equal(t, "verification", attachmentField(t, fields, "Component"))

	errorMessage := attachmentField(t, fields, "Error Message")
	assert.Equal(t,
		fmt.Sprintf("Collection failed: 2 of 3 targets unresolved, Error: failed to collect verification files, Files: %q, %q", missingA, missingB),
		errorMessage)
	assert.NotContains(t, errorMessage, "Total:")
	assert.NotContains(t, errorMessage, "Verified:")

	details := stderrDetailsLine(t, run.stderr)
	assert.NotContains(t, details, missingA)
	assert.NotContains(t, details, missingB)
}
