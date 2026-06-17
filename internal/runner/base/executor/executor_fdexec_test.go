package executor_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openVerifiedPlan opens path read-only and returns a VerifiedCommandPlan bound to
// that descriptor, mirroring what the risk evaluator produces for an allowed
// command. The caller owns the plan and must Close it.
func openVerifiedPlan(t *testing.T, path string, args []string) *risktypes.VerifiedCommandPlan {
	t.Helper()
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	require.NoError(t, err)
	return &risktypes.VerifiedCommandPlan{
		ResolvedPath: path,
		ResolvedArgv: append([]string{path}, args...),
		Identity: &risktypes.VerifiedIdentity{
			FD:           risktypes.NewVerifiedFD(fd),
			ResolvedPath: path,
			ContentHash:  "sha256:test",
		},
		Assessment: risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
	}
}

// numOpenFDs counts the current process's open descriptors via /proc (Linux only).
func numOpenFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	return len(entries)
}

// TestExecute_FdBoundOrStaging verifies that execution is bound to the verified
// descriptor: the fd-bound path (/proc/self/fd, default) and the read-only
// staging fallback both run the verified inode and produce its output.
func TestExecute_FdBoundOrStaging(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fd-bound execution path is Linux-specific")
	}

	tests := []struct {
		name string
		opts []executor.Option
	}{
		{name: "fd-bound (/proc/self/fd)"},
		{name: "staging fallback", opts: []executor.Option{executor.WithFdExecDisabled()}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := executor.NewDefaultExecutor(tt.opts...)
			plan := openVerifiedPlan(t, echoCmd, []string{"fdboundtest"})
			defer func() { _ = plan.Close() }()

			cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"fdboundtest"}, executortestutil.WithWorkDir(""))

			result, err := e.Execute(context.Background(), plan, cmd, map[string]string{}, &executortestutil.MockOutputWriter{})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, 0, result.ExitCode)
			assert.Contains(t, result.Stdout, "fdboundtest")
		})
	}
}

// TestExecute_FdBoundNoLeak runs many fd-bound executions and asserts the process
// does not accumulate descriptors: the executor duplicates the verified fd for the
// child and must close that duplicate after each run.
func TestExecute_FdBoundNoLeak(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fd counting via /proc is Linux-specific")
	}

	e := executor.NewDefaultExecutor()
	cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"x"}, executortestutil.WithWorkDir(""))

	// Warm up once so one-time allocations (e.g. devNull) do not skew the count.
	warm := openVerifiedPlan(t, echoCmd, []string{"x"})
	_, err := e.Execute(context.Background(), warm, cmd, map[string]string{}, &executortestutil.MockOutputWriter{})
	require.NoError(t, err)
	require.NoError(t, warm.Close())

	before := numOpenFDs(t)
	for range 30 {
		plan := openVerifiedPlan(t, echoCmd, []string{"x"})
		_, err := e.Execute(context.Background(), plan, cmd, map[string]string{}, &executortestutil.MockOutputWriter{})
		require.NoError(t, err)
		require.NoError(t, plan.Close())
	}
	after := numOpenFDs(t)
	assert.LessOrEqual(t, after, before+1, "fd-bound execution leaked descriptors")
}

// TestExecute_FdBoundStartFailureNoLeak verifies that when exec fails to start
// (the verified inode is not executable), the duplicated descriptor is still
// closed, so repeated failures do not leak descriptors.
func TestExecute_FdBoundStartFailureNoLeak(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fd counting via /proc is Linux-specific")
	}

	dir := t.TempDir()
	notExec := filepath.Join(dir, "notexec")
	require.NoError(t, os.WriteFile(notExec, []byte("not a binary"), 0o644))

	e := executor.NewDefaultExecutor()
	cmd := executortestutil.CreateRuntimeCommand(notExec, nil, executortestutil.WithWorkDir(""))

	before := numOpenFDs(t)
	for range 20 {
		plan := openVerifiedPlan(t, notExec, nil)
		_, err := e.Execute(context.Background(), plan, cmd, map[string]string{}, &executortestutil.MockOutputWriter{})
		require.Error(t, err, "exec of a non-executable inode should fail")
		require.NoError(t, plan.Close())
	}
	after := numOpenFDs(t)
	assert.LessOrEqual(t, after, before+1, "failed fd-bound start leaked descriptors")
}
