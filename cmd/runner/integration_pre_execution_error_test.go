//go:build test

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
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

// TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope drives
// the production reporting boundary (mainWithExitCode) up to the global target
// file verification failure. The config hash is recorded in a temporary hash
// directory, so the failure comes from the unrecorded global verify file after
// the Slack handler is registered -- the same call site main.go reports with a
// global scope. The in-process handler-factory seam is used because a separate
// process cannot be handed the mock server's TLS client.
func TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	unhashedFile := filepath.Join(tmpDir, "unhashed.txt")
	require.NoError(t, os.WriteFile(unhashedFile, []byte("no hash record for this file"), 0o600))

	var (
		mu       sync.Mutex
		payloads []logging.SlackMessage
	)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var message logging.SlackMessage
		if err := json.Unmarshal(body, &message); err == nil {
			mu.Lock()
			payloads = append(payloads, message)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	configFile := filepath.Join(tmpDir, "config.toml")
	configBody := fmt.Sprintf(`
version = "1.0"

[global]
slack_allowed_host = %q
verify_files = [%q]

[[groups]]
name = "unused_group"

[[groups.commands]]
name = "unused-cmd"
cmd = "/bin/true"
`, serverURL.Hostname(), unhashedFile)
	require.NoError(t, os.WriteFile(configFile, []byte(configBody), 0o600))
	configFile, err = filepath.EvalSymlinks(configFile)
	require.NoError(t, err)

	// Record the config hash with the same validator the production manager
	// builds, so the run gets past pre-registration verification and fails on
	// the unrecorded global target file instead.
	hashDir := tu.SafeTempDir(t)
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(configFile, true)
	require.NoError(t, err, "recording the config hash must succeed")

	restoreHashDir := cmdcommon.DefaultHashDirectory
	cmdcommon.DefaultHashDirectory = hashDir
	t.Cleanup(func() { cmdcommon.DefaultHashDirectory = restoreHashDir })

	t.Setenv(logging.SlackWebhookURLErrorEnvVar, "https://hooks.slack.com/services/error")

	restoreFactory := bootstrap.SetSlackHandlerFactory(func(opts logging.SlackHandlerOptions) (*logging.SlackHandler, error) {
		opts.WebhookURL = server.URL
		opts.AllowedHost = serverURL.Hostname()
		opts.HTTPClient = server.Client()
		return logging.NewSlackHandler(opts)
	})
	t.Cleanup(restoreFactory)

	originalLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	// The package-level flag values production reads.
	originalConfigPath, originalLogLevel, originalLogDir := configPath, logLevel, logDir
	originalDryRun, originalGroups, originalRunID := dryRun, groups, runID
	t.Cleanup(func() {
		configPath, logLevel, logDir = originalConfigPath, originalLogLevel, originalLogDir
		dryRun, groups, runID = originalDryRun, originalGroups, originalRunID
	})
	configPath = configFile
	logLevel = "info"
	logDir = tu.SafeTempDir(t)
	dryRun = false
	groups = ""
	runID = ""

	const runIDValue = "test-global-scope-001"
	require.Equal(t, 1, mainWithExitCode(runIDValue), "the global verification failure must exit non-zero")
	bootstrap.FlushSlackNotifications()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, payloads, 1, "the global pre-execution error should reach Slack")
	message := payloads[0]
	assert.Equal(t, "[go-safe-cmd-runner] ❌ *ERROR* — (global) : file_access_failed", message.Text)

	require.Len(t, message.Attachments, 1)
	fields := message.Attachments[0].Fields
	require.GreaterOrEqual(t, len(fields), 3, "the envelope always appends three fields")
	assert.Equal(t, "Scope", fields[len(fields)-3].Title)
	assert.Equal(t, "(global)", fields[len(fields)-3].Value)
	assert.Equal(t, "Hostname", fields[len(fields)-2].Title)
	assert.Equal(t, "Run ID", fields[len(fields)-1].Title)
	assert.Equal(t, runIDValue, fields[len(fields)-1].Value)
}
