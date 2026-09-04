//go:build test

package executor_test

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errWaitFnSentinel = errors.New("wait replaced")

// TestExecute_WaitFnReplacesWait pins the wiring of WithWaitFn: the wait
// goroutine must call the injected function instead of execCmd.Wait(), which is
// what makes the reap-timeout path reachable at all. Without the injection
// point being consulted, a test that blocks in waitFn would silently get the
// real Wait() and pass for the wrong reason.
func TestExecute_WaitFnReplacesWait(t *testing.T) {
	called := false
	e := executor.NewDefaultExecutor(executor.WithWaitFn(func(cmd *exec.Cmd) error {
		called = true
		// Still reap the child: only the error reported for the wait is
		// under test, not whether the process is left behind.
		_ = cmd.Wait()
		return errWaitFnSentinel
	}))

	plan := openVerifiedPlan(t, echoCmd, []string{"waitfntest"})
	defer func() { _ = plan.Close() }()
	cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"waitfntest"}, executortestutil.WithWorkDir(""))

	_, err := e.Execute(context.Background(), plan, cmd, map[string]string{}, &executortestutil.MockOutputWriter{})

	assert.True(t, called, "injected waitFn was not called")
	require.ErrorIs(t, err, errWaitFnSentinel, "the injected waitFn's error must reach the caller")
}
