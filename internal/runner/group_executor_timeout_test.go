//go:build test

package runner

import (
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
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

// TestExecuteSingleCommand_TimeoutLogsTimeoutExceeded verifies the visible
// effect of joining the context error with Wait()'s: a command killed by its
// timeout is reported as a timeout.
//
// executeSingleCommand's errors.Is(err, context.DeadlineExceeded) branch is
// the only place that difference shows up -- executor.Execute's return value
// alone cannot distinguish "killed by the timeout" from "killed by someone
// else", because os/exec used to drop the context error and report only
// "signal: killed". The existing timeout tests in group_executor_test.go use a
// mock that returns context.DeadlineExceeded directly and so would pass either
// way; this one runs the real executor.
func TestExecuteSingleCommand_TimeoutLogsTimeoutExceeded(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()
	// SecurityLogger captures slog.Default() at construction, so the default
	// has to be in place before the group executor is built.
	origDefault := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(origDefault) })

	exec := executor.NewDefaultExecutor()
	mockValidator := new(securitytestutil.MockValidator)
	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)
	mockValidator.On("ValidateCommandAllowed", mock.Anything, mock.Anything).Return(nil)

	pathResolver := &mockPathResolver{}
	pathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	var outputMgr output.CaptureManager
	rm, err := resourcetestutil.NewDefaultResourceManager(
		exec,
		common.NewDefaultFileSystem(),
		nil, // no privilege manager: the command carries no run_as
		pathResolver,
		logger,
		resource.ExecutionModeNormal,
		nil, // dry-run disabled
		outputMgr,
		0, // default max output size
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:          &runnertypes.ConfigSpec{},
		Executor:        exec,
		ResourceManager: rm,
		Validator:       mockValidator,
		RunID:           "test-run-timeout",
	})

	sleepPath := executortestutil.ResolveCommand("sleep")
	groupSpec := &runnertypes.GroupSpec{Name: "timeout-group"}
	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name:      "sleeps-past-its-timeout",
			Cmd:       sleepPath,
			Args:      []string{"10"},
			RiskLevel: runnertypes.RiskLevelLowPtr,
		},
		ExpandedCmd:      sleepPath,
		ExpandedArgs:     []string{"10"},
		EffectiveTimeout: 1,
	}

	_, _, exitCode, err := ge.executeSingleCommand(context.Background(), cmd, groupSpec, newDefaultRuntimeGroup(groupSpec), newDefaultRuntimeGlobal())

	require.Error(t, err, "a command killed by its timeout must fail")
	assert.Equal(t, executor.ExitCodeUnknown, exitCode)

	records := rec.FindRecords(slog.LevelError, "Command exceeded timeout")
	require.Len(t, records, 1, "the timeout must be reported as a timeout, not as a bare signal death")
	assert.Equal(t, "sleeps-past-its-timeout", records[0].Attrs["command"])
	assert.Equal(t, "timeout_exceeded", records[0].Attrs["security_event"])
}
