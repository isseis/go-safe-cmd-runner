//go:build test

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_SingleGroupStageFailureGetsOuterContext drives a single
// group-level pre-execution failure through the production reporting boundary.
// Before the dedicated error type, the outer context was empty for a failure
// that was not a *CommandExecutionError; now the group name is attached to
// both the stderr Details line and the structured error_message.
func TestIntegration_SingleGroupStageFailureGetsOuterContext(t *testing.T) {
	run := runMainWithSlackMock(t, slackRunSpec{
		configBody: func(slackHost string) string {
			return fmt.Sprintf(`
version = "1.0"

[global]
slack_allowed_host = %q

[[groups]]
name = "backup"
env_vars = ["MULTI=first line\n%%{UNDEFINED_VAR}\nlast line"]

[[groups.commands]]
name = "noop"
cmd = %q
`, slackHost, trueCmdPath())
		},
		runID: "test-single-stage-context-001",
	})
	require.Equal(t, 1, run.exitCode, "a group pre-execution failure still fails the run")
	assert.Equal(t, 1, countLinesWithPrefix(run.stdout, "RUN_SUMMARY "), "stdout:\n%s", run.stdout)

	details := stderrDetailsLine(t, run.stderr)
	assert.Contains(t, details, "(group: backup)",
		"a single group failure must attach its group to the outer context: %q", details)
	assert.Contains(t, details, "failed to execute group backup: ",
		"the cause must still name the group: %q", details)

	reports := jsonLogRecords(t, run, "Execution error occurred")
	require.Len(t, reports, 1)
	errorMessage, ok := reports[0][common.PreExecErrorAttrs.ErrorMessage].(string)
	require.True(t, ok, "the execution error record must carry an error_message string: %v", reports[0])
	assert.Contains(t, errorMessage, "(group: backup)",
		"the structured error_message must carry the same outer context: %q", errorMessage)
	// stderrDetailsLine returns only the Details block's first line, so only
	// that prefix can be compared with the structured field.
	assert.True(t, strings.HasPrefix(errorMessage, strings.TrimPrefix(details, "  Details: ")),
		"the structured error_message must start with the Details line: details=%q message=%q", details, errorMessage)
}

// TestIntegration_SingleCommandFailureKeepsOuterContext drives a single command
// failure through the production reporting boundary. The outer context (group
// and command names) must still be attached, the process must exit 1, and the
// run must print exactly one failing RUN_SUMMARY line.
func TestIntegration_SingleCommandFailureKeepsOuterContext(t *testing.T) {
	lsPath, err := exec.LookPath("ls")
	require.NoError(t, err, "the ls command must be available")
	resolvedLs, err := filepath.EvalSymlinks(lsPath)
	require.NoError(t, err)

	run := runMainWithSlackMock(t, slackRunSpec{
		configBody: func(slackHost string) string {
			return fmt.Sprintf(`
version = "1.0"

[global]
slack_allowed_host = %q

[[groups]]
name = "solo"

[[groups.commands]]
name = "fails"
cmd = %q
args = ["/nonexistent-target-for-0177"]
`, slackHost, lsPath)
		},
		hashedFiles: []string{resolvedLs},
		runID:       "test-single-command-context-001",
	})
	require.Equal(t, 1, run.exitCode, "a command failure must exit non-zero")
	assert.Equal(t, 1, countLinesWithPrefix(run.stdout, "RUN_SUMMARY "), "stdout:\n%s", run.stdout)

	details := stderrDetailsLine(t, run.stderr)
	assert.Contains(t, details, "(group: solo, command: fails)",
		"a single command failure must attach its group and command: %q", details)
	assert.Contains(t, details, "failed to execute group solo: ",
		"the cause must name the group: %q", details)

	reports := jsonLogRecords(t, run, "Execution error occurred")
	require.Len(t, reports, 1)
	errorMessage, ok := reports[0][common.PreExecErrorAttrs.ErrorMessage].(string)
	require.True(t, ok, "the execution error record must carry an error_message string: %v", reports[0])
	assert.Contains(t, errorMessage, "(group: solo, command: fails)",
		"the structured error_message must carry the same outer context: %q", errorMessage)
}
