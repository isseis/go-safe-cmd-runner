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
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
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
// process streams and the default logger captured, returning the stderr
// report and the structured error_message. groupName and commandName are the
// outer context the caller would attach in production (cmd/runner's
// executionErrorContext); pass empty strings for none.
func captureExecutionErrorReport(t *testing.T, execErr error, groupName, commandName string) (stderr, errorMessage string) {
	t.Helper()

	var records []slog.Record
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(tu.NewCallbackHandler(func(r slog.Record) {
		records = append(records, r)
	})))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	_, stderr = captureStdStreams(t, func() {
		logging.HandleExecutionError(&logging.ExecutionError{
			Message:     "error running commands",
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
// resource manager and returns the error executeSingleCommand produced.
// outputFile may be nil.
func newGroupFailureForTest(t *testing.T, groupName, commandName, script string, outputFile *string, limit int64) error {
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
		EffectiveTimeout:         30,
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

	group1Err := newGroupFailureForTest(t, "group-1", "fails", "exit 3", nil, 1<<20)
	require.Error(t, group1Err)
	group2Err := newGroupFailureForTest(t, "group-2", "output-heavy", "head -c 100 /dev/zero", &outputFile, 16)
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

	stderr, errorMessage := captureExecutionErrorReport(t, execErr, "", "")
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

	stderr, errorMessage := captureExecutionErrorReport(t, captureErr, "", "")
	details := detailsBlock(t, stderr)

	assert.Contains(t, details, captureErr.Error(),
		"the filesystem cause must reach the Details report")
	assert.Equal(t, details, errorMessage)
}

// TestRunner_SingleCommandFailureReportUnchanged pins that a single command
// failure of one group keeps the text the previous wrapping produced, including
// the outer context.
func TestRunner_SingleCommandFailureReportUnchanged(t *testing.T) {
	groupErr := newGroupFailureForTest(t, "solo-group", "fails", "exit 3", nil, 1<<20)
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
		groupErrs.Errors()[0].GroupName(), cmdErr.CommandName)
	details := detailsBlock(t, stderr)
	assert.Equal(t, wantMessage, errorMessage)
	assert.Equal(t, wantMessage, details)
}
