//go:build test

package main

import (
	"fmt"
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

// TestIntegration_SingleCommandFailureKeepsOuterContext drives a single
// command-level failure (an undefined variable in a command env_vars value)
// through the production reporting boundary. The outer context (group and
// command names) must still be attached, the process must exit 1, and the run
// must print exactly one failing RUN_SUMMARY line. The exact
// *CommandExecutionError text is pinned by the runner-level
// TestRunner_SingleCommandFailureReportUnchanged; this test adds the process
// boundary (exit code and RUN_SUMMARY).
func TestIntegration_SingleCommandFailureKeepsOuterContext(t *testing.T) {
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
env_vars = ["X=%%{MISSING}"]
`, slackHost, trueCmdPath())
		},
		runID: "test-single-command-context-001",
	})
	require.Equal(t, 1, run.exitCode, "a command failure must exit non-zero")
	assert.Equal(t, 1, countLinesWithPrefix(run.stdout, "RUN_SUMMARY "), "stdout:\n%s", run.stdout)
	assert.Contains(t, run.stdout, "status=execution_error",
		"the RUN_SUMMARY line must report the failure status: %s", run.stdout)

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

// TestIntegration_MultiGroupFailureHasNoOuterContext drives two group
// preparation failures through the production reporting boundary. Several
// failures cannot be described by one group/command pair, so the outer context
// is suppressed while each report line names its own group. The process must
// exit 1 with one failing RUN_SUMMARY line.
func TestIntegration_MultiGroupFailureHasNoOuterContext(t *testing.T) {
	run := runMainWithSlackMock(t, slackRunSpec{
		configBody: func(slackHost string) string {
			return fmt.Sprintf(`
version = "1.0"

[global]
slack_allowed_host = %q

[[groups]]
name = "group_one"
env_vars = ["X=%%{MISSING_ONE}"]

[[groups.commands]]
name = "noop_one"
cmd = %q

[[groups]]
name = "group_two"
env_vars = ["X=%%{MISSING_TWO}"]

[[groups.commands]]
name = "noop_two"
cmd = %q
`, slackHost, trueCmdPath(), trueCmdPath())
		},
		runID: "test-multi-command-context-001",
	})
	require.Equal(t, 1, run.exitCode, "group failures must exit non-zero")
	assert.Equal(t, 1, countLinesWithPrefix(run.stdout, "RUN_SUMMARY "), "stdout:\n%s", run.stdout)
	assert.Contains(t, run.stdout, "status=execution_error", "stdout:\n%s\nstderr:\n%s", run.stdout, run.stderr)

	assert.NotContains(t, stderrDetailsLine(t, run.stderr), "(group:",
		"several failures must not attach one outer context")

	reports := jsonLogRecords(t, run, "Execution error occurred")
	require.Len(t, reports, 1)
	errorMessage, ok := reports[0][common.PreExecErrorAttrs.ErrorMessage].(string)
	require.True(t, ok, "the execution error record must carry an error_message string: %v", reports[0])
	assert.NotContains(t, errorMessage, "(group:")
	lines := strings.Split(errorMessage, "\n")
	require.Len(t, lines, 2)
	assert.True(t, strings.HasPrefix(lines[0], "error running commands: failed to execute group group_one: "),
		"line 0: %q", lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "failed to execute group group_two: "),
		"line 1: %q", lines[1])
}

// TestIntegration_NegativeOutputSizeLimitRejectedInDryRun drives a
// configuration that carries negative output_size_limit values through the
// production reporting boundary in dry-run mode. A negative limit is rejected
// while loading the configuration, before any group is previewed, and both the
// stderr Details line and the structured error_message must name each value and
// its location. The run asserts the rejection on its own; dry-run never
// executes a group, so "no group ran" is not evidence here.
func TestIntegration_NegativeOutputSizeLimitRejectedInDryRun(t *testing.T) {
	run := runMainWithSlackMock(t, slackRunSpec{
		dryRun: true,
		configBody: func(slackHost string) string {
			return fmt.Sprintf(`
version = "1.0"

[global]
slack_allowed_host = %q
output_size_limit = -1

[[groups]]
name = "backup"

[[groups.commands]]
name = "noop"
cmd = %q
output_size_limit = -2048
`, slackHost, trueCmdPath())
		},
		runID: "test-negative-output-size-dryrun-001",
	})
	require.Equal(t, 1, run.exitCode, "a negative output_size_limit must fail the run")

	details := stderrDetailsLine(t, run.stderr)
	assert.Contains(t, details, "output_size_limit must not be negative",
		"the Details line must carry the load-time rejection: %q", details)
	assert.Contains(t, details, "global output_size_limit got -1",
		"the Details line must name the global value: %q", details)
	assert.Contains(t, details, "command 'noop' in group 'backup' (groups[0].commands[0]) got -2048",
		"the Details line must name the command and its position: %q", details)

	reports := jsonLogRecords(t, run, "Pre-execution error occurred")
	require.Len(t, reports, 1)
	errorMessage, ok := reports[0][common.PreExecErrorAttrs.ErrorMessage].(string)
	require.True(t, ok, "the pre-execution error record must carry an error_message string: %v", reports[0])
	assert.Contains(t, errorMessage, "output_size_limit must not be negative")
	assert.Contains(t, errorMessage, "global output_size_limit got -1")
	assert.Contains(t, errorMessage, "command 'noop' in group 'backup' (groups[0].commands[0]) got -2048")
}
