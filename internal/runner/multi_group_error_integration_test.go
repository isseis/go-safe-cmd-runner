//go:build test

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	securitytestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/security/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	resourcetestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/resource/testutil"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// allowOutputWriteValidator accepts every output path, so a group can capture
// output without the production security validator's host-specific policy.
type allowOutputWriteValidator struct{}

func (allowOutputWriteValidator) ValidateOutputWritePermission(string, int) error { return nil }

// executionErrorMessage returns the error_message attribute of the single
// "Execution error occurred" record, failing if the record is absent.
func executionErrorMessage(t *testing.T, records []slog.Record) string {
	t.Helper()

	for _, record := range records {
		if record.Message != "Execution error occurred" {
			continue
		}
		var message string
		record.Attrs(func(a slog.Attr) bool {
			if a.Key == common.PreExecErrorAttrs.ErrorMessage {
				message = a.Value.String()
			}
			return true
		})
		return message
	}
	t.Fatalf("no execution error record among %d records", len(records))
	return ""
}

// detailsBlock returns the Details field of the stderr report, including its
// continuation lines but without the prefix and the continuation indent.
func detailsBlock(t *testing.T, stderr string) string {
	t.Helper()

	const prefix = "  Details: "
	indent := strings.Repeat(" ", len(prefix))
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")

	start := -1
	var block []string
	for i, line := range lines {
		if start < 0 {
			if strings.HasPrefix(line, prefix) {
				start = i
				block = append(block, strings.TrimPrefix(line, prefix))
			}
			continue
		}
		if rest, ok := strings.CutPrefix(line, indent); ok {
			block = append(block, rest)
			continue
		}
		break
	}
	require.GreaterOrEqual(t, start, 0, "no \"  Details:\" line in stderr: %q", stderr)
	return strings.Join(block, "\n")
}

// captureStdStreams runs fn with os.Stdout and os.Stderr redirected to pipes
// and returns what it wrote to each. handleErrorCommon reads the package-level
// os.Stdout/os.Stderr at call time, so reassigning them captures the report.
func captureStdStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	outReader, outWriter, err := os.Pipe()
	require.NoError(t, err)
	errReader, errWriter, err := os.Pipe()
	require.NoError(t, err)

	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outWriter, errWriter
	// Restore even if fn panics or t.Fatal unwinds, so a failed assertion
	// cannot leak the redirected streams into later tests.
	defer func() { os.Stdout, os.Stderr = origStdout, origStderr }()

	var wg sync.WaitGroup
	var outBuf, errBuf strings.Builder
	wg.Go(func() { _, _ = io.Copy(&outBuf, outReader) })
	wg.Go(func() { _, _ = io.Copy(&errBuf, errReader) })

	fn()

	os.Stdout, os.Stderr = origStdout, origStderr
	require.NoError(t, outWriter.Close())
	require.NoError(t, errWriter.Close())
	wg.Wait()
	require.NoError(t, outReader.Close())
	require.NoError(t, errReader.Close())

	return outBuf.String(), errBuf.String()
}

// captureExecutionErrorReport runs HandleExecutionError for execErr with the
// process streams captured and the default logger behind the production
// RedactingHandler, returning the stderr report and the redacted
// error_message. message is the report's summary; groupName and commandName
// are the outer context the caller would attach in production (cmd/runner's
// executionErrorContext); pass empty strings for none.
func captureExecutionErrorReport(t *testing.T, execErr error, message errmsg.Summary, groupName, commandName string) (stderr, errorMessage string) {
	t.Helper()

	var records []slog.Record
	originalLogger := slog.Default()
	recorder := tu.NewCallbackHandler(func(r slog.Record) {
		records = append(records, r)
	})
	slog.SetDefault(slog.New(redaction.NewRedactingHandler(recorder, redaction.DefaultConfig(), nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	_, stderr = captureStdStreams(t, func() {
		logging.HandleExecutionError(&logging.ExecutionError{
			Message:     message,
			Component:   string(resource.ComponentRunner),
			RunID:       "test-run-attribution",
			GroupName:   groupName,
			CommandName: commandName,
			Err:         execErr,
		})
	})
	return stderr, executionErrorMessage(t, records)
}

// newGroupFailureForTest runs one shell script through a real executor and
// resource manager with the given timeout in seconds and returns the error
// executeSingleCommand produced. outputFile may be nil.
func newGroupFailureForTest(t *testing.T, groupName, commandName, script string, outputFile *string, limit int64, timeout int32) error {
	t.Helper()

	exec := executor.NewDefaultExecutor()
	mockValidator := new(securitytestutil.MockValidator)
	pathResolver := &mockPathResolver{}
	pathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	outputMgr := output.NewDefaultOutputCaptureManager(allowOutputWriteValidator{})
	rm, err := resourcetestutil.NewDefaultResourceManager(
		exec,
		common.NewDefaultFileSystem(),
		nil, // no privilege manager: the commands carry no run_as
		pathResolver,
		slog.Default(),
		resource.ExecutionModeNormal,
		nil, // dry-run disabled
		outputMgr,
		0,
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:          &runnertypes.ConfigSpec{},
		Executor:        exec,
		ResourceManager: rm,
		Validator:       mockValidator,
		RunID:           "test-run-attribution",
	})
	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)
	mockValidator.On("ValidateCommandAllowed", mock.Anything, mock.Anything).Return(nil)
	mockValidator.On("SanitizeOutputForLogging", mock.Anything).Return("")

	limitValue, err := common.NewOutputSizeLimit(limit)
	require.NoError(t, err)

	groupSpec := &runnertypes.GroupSpec{Name: groupName}
	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name:       commandName,
			Cmd:        "/bin/sh",
			Args:       []string{"-c", script},
			OutputFile: outputFile,
			RiskLevel:  runnertypes.RiskLevelMediumPtr,
		},
		ExpandedCmd:              "/bin/sh",
		ExpandedArgs:             []string{"-c", script},
		EffectiveTimeout:         timeout,
		EffectiveOutputSizeLimit: limitValue,
	}

	_, _, _, err = ge.executeSingleCommand(
		context.Background(), cmd, groupSpec, newDefaultRuntimeGroup(groupSpec), newDefaultRuntimeGlobal())
	return err
}

// TestRunner_MultiGroupFailureAttribution drives two real command failures
// through executeGroups and the production report. group-1 exits non-zero;
// group-2 exceeds a small output size limit with an output file. Each Details
// line must name its own group and carry the cause's raw Error() text. Whether
// several failures suppress the single outer context is decided by
// executionErrorContext in cmd/runner, so that part is verified there.
func TestRunner_MultiGroupFailureAttribution(t *testing.T) {
	outputFile := fmt.Sprintf("%s/group2.out", t.TempDir())

	group1Err := newGroupFailureForTest(t, "group-1", "fails", "exit 3", nil, 1<<20, 30)
	require.Error(t, group1Err)
	group2Err := newGroupFailureForTest(t, "group-2", "output-heavy", "head -c 100 /dev/zero", &outputFile, 16, 30)
	require.Error(t, group2Err)

	_, hasCapErr := errors.AsType[*output.CaptureError](group2Err)
	require.True(t, hasCapErr, "the size-limit failure must carry a CaptureError, got %T", group2Err)

	_, execErr := executeWithGroupFailures(t, false,
		groupFailure{group: "group-1", err: group1Err},
		groupFailure{group: "group-2", err: group2Err})
	require.Error(t, execErr)

	groupErrs, ok := errors.AsType[*GroupErrors](execErr)
	require.True(t, ok, "executeGroups must return a *GroupErrors, got %T", execErr)
	require.Len(t, groupErrs.Errors(), 2)

	capErr, ok := errors.AsType[*output.CaptureError](execErr)
	require.True(t, ok, "the report chain must carry the size-limit CaptureError")

	stderr, errorMessage := captureExecutionErrorReport(t, execErr, errmsg.ConstSummary("error running commands"), "", "")
	details := detailsBlock(t, stderr)

	// Each Details line names its own group.
	lines := strings.Split(details, "\n")
	require.Len(t, lines, 2)
	assert.True(t, strings.HasPrefix(lines[0], "error running commands: failed to execute group group-1: "),
		"line 0: %q", lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "failed to execute group group-2: "),
		"line 1: %q", lines[1])

	// The group-2 line names its command and appends the raw CaptureError text.
	assert.Contains(t, lines[1], "command output-heavy in group group-2 failed: ",
		"the group-2 line must name its command")
	assert.Contains(t, lines[1], capErr.Error(),
		"the group-2 line must carry the raw CaptureError text")

	// The structured error_message agrees with the Details block.
	assert.Equal(t, details, errorMessage)
}

// TestHandleExecutionError_FilesystemCaptureErrorKeepsCause pins that a
// filesystem CaptureError's Cause reaches the report. The old UserMessage
// dropped it, so this fails if the substitution returns. The error is handed
// straight to the report rather than through the executor chain, isolating the
// cause-rendering property; the size-limit case exercises the full chain.
func TestHandleExecutionError_FilesystemCaptureErrorKeepsCause(t *testing.T) {
	closed, err := os.CreateTemp(t.TempDir(), "closed")
	require.NoError(t, err)
	require.NoError(t, closed.Close())

	captureErr := (&output.Capture{OutputPath: "/tmp/out", MaxSize: 1 << 20, FileHandle: closed}).
		WriteOutput([]byte("data"))
	require.Error(t, captureErr, "writing to a closed handle must fail")
	require.Contains(t, captureErr.Error(), "file already closed",
		"the real filesystem cause must be present in the error")

	stderr, errorMessage := captureExecutionErrorReport(t, captureErr, errmsg.ConstSummary("error running commands"), "", "")
	details := detailsBlock(t, stderr)

	assert.Contains(t, details, captureErr.Error(),
		"the filesystem cause must reach the Details report")
	assert.Equal(t, details, errorMessage)
}

// TestRunner_SingleCommandFailureReportUnchanged pins that a single command
// failure of one group keeps the text the previous wrapping produced, including
// the outer context.
func TestRunner_SingleCommandFailureReportUnchanged(t *testing.T) {
	groupErr := newGroupFailureForTest(t, "solo-group", "fails", "exit 3", nil, 1<<20, 30)
	require.Error(t, groupErr)

	_, execErr := executeWithGroupFailures(t, false, groupFailure{group: "solo-group", err: groupErr})
	require.Error(t, execErr)

	groupErrs, ok := errors.AsType[*GroupErrors](execErr)
	require.True(t, ok)
	require.Len(t, groupErrs.Errors(), 1)
	cmdErr, ok := errors.AsType[*CommandExecutionError](execErr)
	require.True(t, ok)

	// The unchanged text is the previous assembly applied to the same cause.
	legacy := fmt.Errorf("failed to execute group %s: %w", groupErrs.Errors()[0].GroupName(), cmdErr).Error()
	wantMessage := fmt.Sprintf("error running commands (group: %s, command: %s): %s",
		"solo-group", cmdErr.CommandName, legacy)

	stderr, errorMessage := captureExecutionErrorReport(t, execErr,
		errmsg.ConstSummary("error running commands"),
		groupErrs.Errors()[0].GroupName(), cmdErr.CommandName)
	details := detailsBlock(t, stderr)
	assert.Equal(t, wantMessage, errorMessage)
	assert.Equal(t, wantMessage, details)
}

// TestRunner_TimeoutAttributionIntegration drives a real command timeout
// through executeGroups and the production report. group-1 exits non-zero and
// group-2's command is killed by its own timeout. The timeout is collected
// rather than discarding group-1, so Details carries a line for each group, and
// the timeout's continuation line is indented under group-2's line.
func TestRunner_TimeoutAttributionIntegration(t *testing.T) {
	group1Err := newGroupFailureForTest(t, "group-1", "fails", "exit 3", nil, 1<<20, 30)
	require.Error(t, group1Err)
	// exec replaces the shell, so the kill on timeout reaches the sleep itself
	// and leaves no grandchild behind.
	group2Err := newGroupFailureForTest(t, "group-2", "slow", "exec sleep 10", nil, 1<<20, 1)
	require.ErrorIs(t, group2Err, context.DeadlineExceeded, "group-2 must fail by its own timeout")
	_, ok := errors.AsType[*CommandExecutionError](group2Err)
	require.True(t, ok, "the timeout must be wrapped in a *CommandExecutionError, got %T", group2Err)

	_, execErr := executeWithGroupFailures(t, false,
		groupFailure{group: "group-1", err: group1Err},
		groupFailure{group: "group-2", err: group2Err})
	assert.Equal(t, []string{"group-1", "group-2"}, groupNames(t, execErr))

	stderr, errorMessage := captureExecutionErrorReport(t, execErr, errmsg.ConstSummary("error running commands"), "", "")
	details := detailsBlock(t, stderr)
	lines := strings.Split(details, "\n")

	require.GreaterOrEqual(t, len(lines), 3, "a timeout cause has continuation lines: %q", details)
	assert.True(t, strings.HasPrefix(lines[0], "error running commands: failed to execute group group-1: "),
		"line 0: %q", lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "failed to execute group group-2: command slow in group group-2 failed: "),
		"line 1: %q", lines[1])
	for i, line := range lines[2:] {
		assert.True(t, strings.HasPrefix(line, "  "),
			"continuation line %d of group-2 must be indented under it: %q", i+2, line)
	}
	assert.Equal(t, details, errorMessage)
}

// TestRunner_MultiGroupSensitiveNamesSurvive is the multi-group end-to-end scenario:
// two groups fail with a non-zero exit code, one group name contains a
// whole-value trigger word, and the recorded error_message keeps every
// Identifier the two group errors declare. A free-text control summary that
// carries only a whole-value trigger becomes the placeholder in the same
// record, which proves the report went through the production redaction rather
// than being compared unredacted.
func TestRunner_MultiGroupSensitiveNamesSurvive(t *testing.T) {
	const (
		sensitiveGroup = "token-rotate"
		plainGroup     = "backup"
	)
	// Layer isolation: the group name trips only the whole-value layer, so it
	// is a value that would disappear if identifiers were not exempt.
	require.True(t, redaction.DefaultSensitivePatterns().IsSensitiveValue(sensitiveGroup),
		"the sensitive group name must trip the whole-value layer, or surviving it proves nothing")
	require.Equal(t, sensitiveGroup, redaction.DefaultConfig().RedactText(sensitiveGroup),
		"RedactText must not react to the group name on its own")

	group1Err := newGroupFailureForTest(t, sensitiveGroup, "renew", "exit 3", nil, 1<<20, 30)
	require.Error(t, group1Err)
	group2Err := newGroupFailureForTest(t, plainGroup, "dump", "exit 3", nil, 1<<20, 30)
	require.Error(t, group2Err)

	_, execErr := executeWithGroupFailures(t, false,
		groupFailure{group: sensitiveGroup, err: group1Err},
		groupFailure{group: plainGroup, err: group2Err})
	require.Error(t, execErr)
	require.Equal(t, []string{sensitiveGroup, plainGroup}, groupNames(t, execErr))

	// The control summary carries only a whole-value trigger and no key=value
	// or value-format shape, so only the per-segment whole-value layer can
	// replace it.
	const trigger = "api_key"
	control := errmsg.TextSummary("error running commands for " + trigger + " target")
	require.True(t, redaction.DefaultSensitivePatterns().IsSensitiveValue(control.String()),
		"the control summary must trip the whole-value layer")
	require.Equal(t, control.String(), redaction.DefaultConfig().RedactText(control.String()),
		"the control summary must not trip the text layer on its own")

	_, errorMessage := captureExecutionErrorReport(t, execErr, control, "", "")

	// Every Identifier the two group errors declare survives.
	for _, want := range []string{sensitiveGroup, plainGroup, "renew", "dump"} {
		assert.Contains(t, errorMessage, want,
			"an Identifier the group errors declare must survive redaction: %q", errorMessage)
	}
	assert.NotEqual(t, redaction.DefaultPlaceholder, errorMessage,
		"the whole body must not be replaced; the group errors must stay readable")
	assert.Contains(t, errorMessage, redaction.DefaultPlaceholder,
		"the Text control summary must be replaced")
}
