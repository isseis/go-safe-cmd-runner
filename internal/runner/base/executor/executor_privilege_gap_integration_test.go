//go:build integration

// These tests need -tags "test integration": executor/testutil requires test.
package executor_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSetuidExecutor drops to the invoker before execution and restores entry euid
// at cleanup, including test failure; these process-wide changes must stay serial.
func newSetuidExecutor(t *testing.T) (executor.CommandExecutor, *tu.LogRecorder) {
	t.Helper()
	ok, reason := canRunSetuidModelIntegrationTest(os.Getuid(), os.Geteuid(), os.Getenv("TEST_RUNAS_TARGET_USER"))
	require.True(t, ok, reason)
	target, err := user.Lookup(os.Getenv("TEST_RUNAS_TARGET_USER"))
	require.NoError(t, err)
	require.NotEqual(t, strconv.Itoa(os.Getuid()), target.Uid, "target must differ from invoker to exercise kill re-elevation")
	require.NotEqual(t, "0", target.Uid, "use a non-root fixture")
	logger, recorder := tu.NewRecordingLogger()
	manager := privilege.NewManager(logger)
	require.True(t, manager.IsPrivilegedExecutionSupported())
	entryEUID := os.Geteuid()
	t.Cleanup(func() { require.NoError(t, syscall.Seteuid(entryEUID)) })
	require.NoError(t, syscall.Seteuid(os.Getuid()))
	return executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(manager),
		executor.WithLogger(logger),
		executor.WithAuditLogger(audit.NewAuditLoggerWithCustom(logger)),
	), recorder
}

func TestPrivilegeGap_StartWindowIndependentOfCommandDuration(t *testing.T) {
	requireSetuidModel(t)
	e, recorder := newSetuidExecutor(t)
	var durations []int64
	for _, seconds := range []string{"1", "5"} {
		cmd := executortestutil.CreateRuntimeCommand(executortestutil.ResolveCommand("sleep"), []string{seconds},
			executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser(os.Getenv("TEST_RUNAS_TARGET_USER")))
		result, err := e.Execute(context.Background(), nil, cmd, nil, nil)
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
	}
	records := recorder.FindRecords(slog.LevelInfo, "User/group command executed successfully")
	require.Len(t, records, 2)
	for _, record := range records {
		duration, ok := record.Attrs["privilege_duration_user_group_execution_us"].(int64)
		require.True(t, ok, "start-window metric must be present")
		assert.Positive(t, duration)
		assert.Less(t, duration, int64(5000))
		durations = append(durations, duration)
	}
	difference := durations[1] - durations[0]
	if difference < 0 {
		difference = -difference
	}
	assert.Less(t, difference, int64(2000))
	t.Logf("start windows: %v us; difference: %d us", durations, difference)
}

func TestPrivilegeGap_TimeoutKillsChild(t *testing.T) {
	requireSetuidModel(t)
	e, _ := newSetuidExecutor(t)
	cmd := executortestutil.CreateRuntimeCommand(executortestutil.ResolveCommand("sleep"), []string{"10"},
		executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser(os.Getenv("TEST_RUNAS_TARGET_USER")))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	before := os.Geteuid()
	start := time.Now()
	result, err := e.Execute(ctx, nil, cmd, nil, nil)
	assert.Equal(t, before, os.Geteuid())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotNil(t, result)
	_, exited := errors.AsType[*exec.ExitError](err)
	require.True(t, exited, "child must have started and exited")
	assert.NotErrorIs(t, err, executor.ErrChildNotReaped)
	assert.NotErrorIs(t, err, executor.ErrKillAfterCancel)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestPrivilegeGap_CancelKillsChild(t *testing.T) {
	requireSetuidModel(t)
	e, _ := newSetuidExecutor(t)
	cmd := executortestutil.CreateRuntimeCommand(executortestutil.ResolveCommand("sleep"), []string{"10"},
		executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser(os.Getenv("TEST_RUNAS_TARGET_USER")))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(200*time.Millisecond, cancel)
	defer timer.Stop()
	before := os.Geteuid()
	start := time.Now()
	result, err := e.Execute(ctx, nil, cmd, nil, nil)
	assert.Equal(t, before, os.Geteuid())
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	_, exited := errors.AsType[*exec.ExitError](err)
	require.True(t, exited, "child must have started and exited")
	assert.NotErrorIs(t, err, executor.ErrChildNotReaped)
	assert.NotErrorIs(t, err, executor.ErrKillAfterCancel)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestPrivilegeGap_OutputLimitAbortsRunningChild(t *testing.T) {
	requireSetuidModel(t)
	e, _ := newSetuidExecutor(t)
	file, err := os.CreateTemp(t.TempDir(), "capture-")
	require.NoError(t, err)
	capture := &output.Capture{FileHandle: file, MaxSize: 1024}
	t.Cleanup(func() { require.NoError(t, capture.Close()) })
	// A direct yes child writes indefinitely; closing its pipe must stop it.
	cmd := executortestutil.CreateRuntimeCommand(executortestutil.ResolveCommand("yes"), []string{"x"},
		executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser(os.Getenv("TEST_RUNAS_TARGET_USER")))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	result, err := e.Execute(ctx, nil, cmd, nil, capture)
	require.ErrorIs(t, err, output.ErrOutputSizeExceeded)
	require.NotNil(t, result)
	assert.NotErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}
