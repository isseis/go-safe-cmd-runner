//go:build test

package executor

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege"
	privilegetestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file drive prepareCommand, startPrepared and
// superviseCommand directly rather than Execute, and set pc.kill by hand where
// a re-elevated kill is under test. An unprivileged test cannot start a real
// run-as child -- the kernel refuses SysProcAttr.Credential's setgroups
// without CAP_SETGID -- so declaring killReelevated over a child that actually
// runs as the test user is the only way to exercise the kill window off a
// privileged host. The real credential is covered by the privileged
// integration tests.

// prepareForSupervise builds a preparedCommand for path and args, with no
// run-as credential.
func prepareForSupervise(t *testing.T, e *DefaultExecutor, writer OutputWriter, path string, args ...string) *preparedCommand {
	t.Helper()
	pc, err := e.prepareCommand(context.Background(), nil, path, createTestCommand(path, args), nil, writer, nil)
	require.NoError(t, err)
	return pc
}

// startForSupervise starts pc's child and releases the pipe write ends,
// mirroring what runCommand does between the start and supervision phases. It
// returns the child's pid so a cleanup can make sure nothing is left behind.
func startForSupervise(t *testing.T, e *DefaultExecutor, pc *preparedCommand) int {
	t.Helper()
	started, err := e.startPrepared(pc)
	require.NoError(t, err)
	require.True(t, started)
	require.NoError(t, pc.pump.releaseChildEnds())
	pid := pc.execCmd.Process.Pid
	t.Cleanup(func() { killTestProcess(pid) })
	return pid
}

// killTestProcess sends SIGKILL to pid, ignoring a process that is already
// gone.
func killTestProcess(pid int) {
	if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// processIsRunning reports whether pid names a process that has not yet
// exited. A killed-but-not-yet-reaped child is a zombie, which signal 0 still
// reaches, so the state field of /proc/<pid>/stat is what tells the two apart.
// Linux-only; callers skip elsewhere.
func processIsRunning(pid int) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	// The comm field is parenthesized and may itself contain spaces, so the
	// state is the first field after the last ')'.
	stat := string(data)
	fields := strings.Fields(stat[strings.LastIndex(stat, ")")+1:])
	return len(fields) > 0 && fields[0] != "Z" && fields[0] != "X"
}

// waitForProcessExit blocks until pid is no longer running or the deadline
// passes, and reports which happened. SIGKILL delivery is asynchronous, so an
// assertion that the kill has already been issued has to wait for its effect
// rather than sample once.
func waitForProcessExit(pid int, deadline time.Duration) bool {
	until := time.Now().Add(deadline)
	for time.Now().Before(until) {
		if !processIsRunning(pid) {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return !processIsRunning(pid)
}

// failingWriter is an OutputWriter that fails once more than limit bytes have
// passed through it, standing in for what output.Capture does when a command
// exceeds its output size limit. It is mutex-guarded because the pump reaches
// an OutputWriter from one reader goroutine per stream.
type failingWriter struct {
	mu    sync.Mutex
	limit int
	total int
	err   error
	hit   bool
}

func (w *failingWriter) Write(_ OutputStream, p []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.total += len(p)
	if w.total > w.limit {
		w.hit = true
		return w.err
	}
	return nil
}

func (w *failingWriter) Close() error { return nil }

// failed reports whether the writer has already refused a write.
func (w *failingWriter) failed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hit
}

// TestSupervise_TimeoutJoinsContextAndWaitErrors verifies the one behavior
// this task deliberately changes: a run ended by a timeout reports both the
// context's error and Wait()'s. os/exec drops the context error, which is why
// a timed-out command currently reports only "signal: killed" and the group
// executor's errors.Is(err, context.DeadlineExceeded) branch never fires.
func TestSupervise_TimeoutJoinsContextAndWaitErrors(t *testing.T) {
	e := NewDefaultExecutor().(*DefaultExecutor)
	pc := prepareForSupervise(t, e, nil, sleepPath, "30")
	startForSupervise(t, e, pc)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	t.Cleanup(cancel)

	result, err := e.superviseCommand(ctx, pc, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "the timeout must stay reachable through the returned error")
	_, isExitErr := errors.AsType[*exec.ExitError](err)
	assert.True(t, isExitErr, "Wait()'s own error must stay reachable alongside it, got: %v", err)
	require.NotNil(t, result)
}

// TestSupervise_CancelKillsChild verifies that an explicit cancellation stops
// a child that would otherwise outlive the test by far, and that
// context.Canceled is reachable from the returned error.
func TestSupervise_CancelKillsChild(t *testing.T) {
	e := NewDefaultExecutor().(*DefaultExecutor)
	pc := prepareForSupervise(t, e, nil, sleepPath, "30")
	pid := startForSupervise(t, e, pc)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := e.superviseCommand(ctx, pc, nil)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, elapsed, 5*time.Second, "the child must be killed, not waited out")
	assert.False(t, processIsRunning(pid), "the child must not survive the cancellation")
}

// TestSupervise_KillOpensExactlyOneReelevation verifies that a re-elevated
// kill opens exactly one privilege window, under its own operation, and that
// the kill happens inside it: the child is still alive when the window opens
// and has exited before it closes. What the window contains beyond that is the
// static guard's job, not this test's.
func TestSupervise_KillOpensExactlyOneReelevation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process liveness is sampled via /proc")
	}

	var pid int
	var aliveBefore, exitedByAfter bool
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	mockPriv.InWindow = func(phase privilegetestutil.MockWindowPhase) {
		switch phase {
		case privilegetestutil.MockWindowPhaseBeforeFn:
			aliveBefore = processIsRunning(pid)
		case privilegetestutil.MockWindowPhaseAfterFn:
			// SIGKILL is delivered asynchronously, so the effect of a kill
			// issued inside the window may land just after fn returns; the
			// window is still open while this waits.
			exitedByAfter = waitForProcessExit(pid, 5*time.Second)
		case privilegetestutil.MockWindowPhaseUnset:
			t.Error("InWindow must never be called with the unset phase")
		}
	}

	e := NewDefaultExecutor(WithPrivilegeManager(mockPriv)).(*DefaultExecutor)
	pc := prepareForSupervise(t, e, nil, sleepPath, "30")
	pc.kill = killReelevated
	pid = startForSupervise(t, e, pc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.superviseCommand(ctx, pc, nil)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrKillAfterCancel, "the kill window must succeed")
	assert.Equal(t, []string{string(runnertypes.OperationKillAfterCancel)}, mockPriv.ElevationCalls,
		"a cancelled run-as command must open exactly one window, for the kill")
	assert.True(t, aliveBefore, "the child must still be running when the kill window opens")
	assert.True(t, exitedByAfter, "the child must have been killed before the kill window closes")
}

// TestSupervise_NormalExecutionDoesNotReelevate verifies the companion half of
// the kill-strategy rule: a command that applied no credential is killed
// directly, with no privilege window at all -- its child shares the parent's
// uid, so no re-elevation is needed to signal it. Without it, a kill path that always re-elevated
// would still satisfy TestSupervise_KillOpensExactlyOneReelevation.
func TestSupervise_NormalExecutionDoesNotReelevate(t *testing.T) {
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	e := NewDefaultExecutor(WithPrivilegeManager(mockPriv)).(*DefaultExecutor)

	pc := prepareForSupervise(t, e, nil, sleepPath, "30")
	require.Equal(t, killDirect, pc.kill, "a command with no credential must declare a direct kill")
	pid := startForSupervise(t, e, pc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.superviseCommand(ctx, pc, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, mockPriv.ElevationCalls, "a kill that needs no re-elevation must open no window")
	assert.False(t, processIsRunning(pid), "the child must still be killed")
}

// TestSupervise_KillRunsOnExecutingGoroutine verifies that the kill window is
// opened by the same goroutine that opened the start window. The privilege
// manager's elevation is process-wide and its re-entrancy guard is per
// manager, so a kill handed to another goroutine would either race the start
// window's restore or be refused as re-entrant.
//
// The start window is opened by the test rather than by executeWithUserGroup,
// which still wraps the whole run in this phase; that is the shape the window
// takes once it is narrowed to startPrepared.
func TestSupervise_KillRunsOnExecutingGoroutine(t *testing.T) {
	type sample struct {
		phase privilegetestutil.MockWindowPhase
		gid   string
	}
	var samples []sample

	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	mockPriv.InWindow = func(phase privilegetestutil.MockWindowPhase) {
		samples = append(samples, sample{phase: phase, gid: goroutineHeader()})
	}

	e := NewDefaultExecutor(WithPrivilegeManager(mockPriv)).(*DefaultExecutor)
	pc := prepareForSupervise(t, e, nil, sleepPath, "30")
	pc.kill = killReelevated

	startCtx := runnertypes.ElevationContext{Operation: runnertypes.OperationUserGroupExecution}
	var started bool
	require.NoError(t, mockPriv.WithPrivileges(startCtx, func() error {
		var startErr error
		started, startErr = e.startPrepared(pc)
		return startErr
	}))
	require.True(t, started)
	require.NoError(t, pc.pump.releaseChildEnds())
	t.Cleanup(func() { killTestProcess(pc.execCmd.Process.Pid) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.superviseCommand(ctx, pc, nil)

	require.Error(t, err)
	assert.NotErrorIs(t, err, privilege.ErrReentrantPrivilegeCall,
		"the kill window must not be nested inside the start window")
	assert.NotErrorIs(t, err, ErrKillAfterCancel)

	require.Len(t, samples, 4, "two windows, sampled before and after fn each")
	assert.Equal(t, privilegetestutil.MockWindowPhaseBeforeFn, samples[0].phase)
	assert.Equal(t, privilegetestutil.MockWindowPhaseBeforeFn, samples[2].phase)
	assert.Equal(t, samples[0].gid, samples[2].gid,
		"the kill window must be opened by the goroutine that opened the start window")
}

// goroutineHeader returns the "goroutine N [" prefix of the current
// goroutine's stack dump, which identifies the goroutine without depending on
// any way of reading its id directly.
func goroutineHeader() string {
	buf := make([]byte, 64)
	n := runtime.Stack(buf, false)
	header := string(buf[:n])
	if prefix, _, found := strings.Cut(header, "["); found {
		return prefix
	}
	return header
}

// TestSupervise_ProcessAlreadyDoneIsNotAnError verifies that killing a child
// that has already been reaped is not reported as a failure: the cancellation
// and the child's own exit raced, and the child is gone either way.
//
// The race is removed rather than won: the test reaps the child itself, then
// forces the kill path with a start-phase error, so Process.Kill is guaranteed
// to see an already-finished process.
func TestSupervise_ProcessAlreadyDoneIsNotAnError(t *testing.T) {
	e := NewDefaultExecutor(WithWaitFn(func(*exec.Cmd) error { return nil })).(*DefaultExecutor)
	pc := prepareForSupervise(t, e, nil, shPath, "-c", "exit 0")
	startForSupervise(t, e, pc)

	require.NoError(t, pc.execCmd.Wait(), "the test reaps the child so the kill below sees a finished process")

	startupErr := errors.New("release failed after start")
	result, err := e.superviseCommand(context.Background(), pc, startupErr)

	require.Error(t, err)
	assert.ErrorIs(t, err, startupErr)
	assert.NotErrorIs(t, err, os.ErrProcessDone, "killing an already-exited child is not a failure")
	assert.NotErrorIs(t, err, ErrKillAfterCancel)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode, "the exit status still comes from Wait()")
}

// TestSupervise_ChildNotReapedReportsUnknownExitCode verifies the reap
// deadline: when Wait() does not return within killGraceDelay of the kill, the
// run gives up and says so rather than blocking, and reports no exit code --
// the wait goroutine is still writing ProcessState, so it must not be read.
//
// Wait() is injected because a real child killed with SIGKILL is always
// reaped; there is no command that can hold this path open.
func TestSupervise_ChildNotReapedReportsUnknownExitCode(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	logger, rec := tu.NewRecordingLogger()
	e := NewDefaultExecutor(
		WithLogger(logger),
		WithKillGraceDelay(50*time.Millisecond),
		WithWaitFn(func(cmd *exec.Cmd) error {
			<-release
			return cmd.Wait()
		}),
	).(*DefaultExecutor)

	pc := prepareForSupervise(t, e, nil, sleepPath, "30")
	pid := startForSupervise(t, e, pc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := e.superviseCommand(ctx, pc, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChildNotReaped)
	assert.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, ExitCodeUnknown, result.ExitCode, "an unreaped child has no exit code to report")

	errs := rec.FindRecords(slog.LevelError, "Command did not exit after kill")
	require.Len(t, errs, 1, "a child that may still be running must be recorded at Error")
	assert.EqualValues(t, pid, errs[0].Attrs["pid"])

	releaseOnce.Do(func() { close(release) })
}

// TestSupervise_GrandchildHoldingPipeDoesNotBlockCompletion verifies that the
// output-drain deadline is a separate concern from the reap deadline. The
// child is reaped promptly, but a grandchild keeps the pipe's write end open,
// so the readers never see EOF. Giving up on the output is recorded, not
// reported as an error: the exit status came from Wait(), so this is not
// ErrChildNotReaped.
func TestSupervise_GrandchildHoldingPipeDoesNotBlockCompletion(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	t.Cleanup(func() { killTestProcessFromFile(pidFile) })

	logger, rec := tu.NewRecordingLogger()
	e := NewDefaultExecutor(WithLogger(logger), WithKillGraceDelay(50*time.Millisecond)).(*DefaultExecutor)

	// The grandchild inherits the pipe's write end and outlives the child,
	// which exec replaces so the killed pid is the sleeping one.
	script := "sleep 30 & echo $! > " + pidFile + "; exec sleep 30"
	pc := prepareForSupervise(t, e, nil, shPath, "-c", script)
	startForSupervise(t, e, pc)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	result, err := e.superviseCommand(ctx, pc, nil)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 10*time.Second, "the grandchild's lifetime must not extend the run")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrChildNotReaped, "the child itself was reaped; only its output was not drained")
	_, isExitErr := errors.AsType[*exec.ExitError](err)
	assert.True(t, isExitErr, "the exit status must still come from Wait(), got: %v", err)
	require.NotNil(t, result)

	assert.Len(t, rec.FindRecords(slog.LevelWarn, "Could not finish reading command output after kill"), 1,
		"giving up on the output must be recorded")
}

// TestSupervise_SizeLimitErrorOutranksExitError verifies that a write error --
// what an exceeded output size limit looks like from here -- is reported as
// the cause, and that the broken-pipe exit it provokes in the child does not
// displace it. Reporting *exec.ExitError instead would tell an operator the
// command failed, not that its output was cut off.
func TestSupervise_SizeLimitErrorOutranksExitError(t *testing.T) {
	limitErr := errors.New("output size limit exceeded")
	writer := &failingWriter{limit: 0, err: limitErr}

	e := NewDefaultExecutor().(*DefaultExecutor)
	// yes(1) keeps writing until the reader closes the pipe, so the child is
	// guaranteed to die of SIGPIPE rather than exit on its own.
	pc := prepareForSupervise(t, e, writer, shPath, "-c", "yes")
	startForSupervise(t, e, pc)

	result, err := e.superviseCommand(context.Background(), pc, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, limitErr)
	_, isExitErr := errors.AsType[*exec.ExitError](err)
	assert.False(t, isExitErr, "the broken-pipe exit must not displace the write error, got: %v", err)
	require.NotNil(t, result)
}

// TestSupervise_KillFailureIsJoinedWithWriteError verifies that a failed kill
// is reported alongside the ranked cause rather than under it. Both hold at
// once here: the output limit was exceeded and the child could not be killed,
// so a process is still running under another uid -- the fact the ranking
// would otherwise hide.
func TestSupervise_KillFailureIsJoinedWithWriteError(t *testing.T) {
	elevationErr := errors.New("kill window refused")
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	mockPriv.FailFor = map[runnertypes.Operation]error{
		runnertypes.OperationKillAfterCancel: elevationErr,
	}

	limitErr := errors.New("output size limit exceeded")
	writer := &failingWriter{limit: 0, err: limitErr}

	logger, rec := tu.NewRecordingLogger()
	e := NewDefaultExecutor(
		WithLogger(logger),
		WithPrivilegeManager(mockPriv),
		WithKillGraceDelay(50*time.Millisecond),
	).(*DefaultExecutor)

	pc := prepareForSupervise(t, e, writer, shPath, "-c", "echo overflow; sleep 30")
	pc.kill = killReelevated
	startForSupervise(t, e, pc)

	// The pump only starts inside superviseCommand, so the write error can
	// only be waited for from alongside it.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		until := time.Now().Add(5 * time.Second)
		for !writer.failed() && time.Now().Before(until) {
			time.Sleep(time.Millisecond)
		}
	}()

	_, err := e.superviseCommand(ctx, pc, nil)

	require.True(t, writer.failed(), "the write error must have been raised before the cancellation")

	require.Error(t, err)
	assert.ErrorIs(t, err, limitErr, "the ranked cause must still be the write error")
	assert.ErrorIs(t, err, ErrKillAfterCancel, "a child that could not be killed must not be hidden behind it")
	assert.ErrorIs(t, err, elevationErr)
	assert.Empty(t, rec.RecordsAtLevel(slog.LevelInfo),
		"a kill that failed must not also be recorded as a kill that happened")
}

// TestStartPrepared_ReleaseFailureStillKillsChild verifies that a failure
// after Start has succeeded does not abandon the child: the run enters the
// kill path without waiting for a cancellation, reaps the child, and reports
// the failure.
//
// The write ends are closed behind the pump's back so releaseChildEnds fails
// with EBADF rather than the os.ErrClosed it treats as its idempotent success
// case.
func TestStartPrepared_ReleaseFailureStillKillsChild(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()
	e := NewDefaultExecutor(WithLogger(logger)).(*DefaultExecutor)
	pc := prepareForSupervise(t, e, nil, sleepPath, "30")

	started, err := e.startPrepared(pc)
	require.NoError(t, err)
	require.True(t, started)
	pid := pc.execCmd.Process.Pid
	t.Cleanup(func() { killTestProcess(pid) })

	// Closing the descriptors behind the *os.File's back means the numbers
	// are free until releaseChildEnds runs, and a number handed back out
	// meanwhile would turn the sabotaged close into a legitimate one. Nothing
	// between these lines opens a descriptor, and no test in this package runs
	// in parallel, so the window stays closed.
	for _, f := range []*os.File{pc.pump.stdout.childEnd, pc.pump.stderr.childEnd} {
		require.NoError(t, syscall.Close(int(f.Fd())))
	}
	closeErr := pc.pump.releaseChildEnds()
	require.Error(t, closeErr, "the sabotaged write ends must fail to close")

	start := time.Now()
	result, err := e.superviseCommand(context.Background(), pc, closeErr)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, syscall.EBADF, "the start-phase failure must reach the caller")
	assert.Less(t, elapsed, 5*time.Second, "a started child must be killed, not waited out")
	assert.False(t, processIsRunning(pid), "a child that cannot be supervised must not be left running")
	require.NotNil(t, result)

	assert.Len(t, rec.FindRecords(slog.LevelInfo, "Killed command after start-phase failure"), 1,
		"the record must name what forced the kill; nothing cancelled this run")
}

// TestSupervise_UnreapedChildDoesNotSpendASecondDrainDeadline verifies that
// giving up on the child also gives up on its output. The child that could not
// be reaped is the one holding the pipe's write end, so waiting out a second
// full killGraceDelay for output that cannot arrive only delays the report
// that a process may still be running.
//
// A grandchild holds the write end open so the drain would genuinely block,
// and Wait() is injected so the reap times out with the child still on the
// books -- the combination the doubled deadline needs.
func TestSupervise_UnreapedChildDoesNotSpendASecondDrainDeadline(t *testing.T) {
	const grace = 300 * time.Millisecond

	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	t.Cleanup(func() { killTestProcessFromFile(pidFile) })

	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	e := NewDefaultExecutor(
		WithKillGraceDelay(grace),
		WithWaitFn(func(cmd *exec.Cmd) error {
			<-release
			return cmd.Wait()
		}),
	).(*DefaultExecutor)

	script := "sleep 30 & echo $! > " + pidFile + "; exec sleep 30"
	pc := prepareForSupervise(t, e, nil, shPath, "-c", script)
	startForSupervise(t, e, pc)

	// Cancel only once the grandchild exists and has the pipe's write end;
	// cancelling sooner can kill the shell before it forks, leaving nothing
	// to hold the drain open and making the assertion below pass vacuously.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		until := time.Now().Add(5 * time.Second)
		for time.Now().Before(until) {
			if _, statErr := os.Stat(pidFile); statErr == nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	start := time.Now()
	_, err := e.superviseCommand(ctx, pc, nil)
	elapsed := time.Since(start)

	require.FileExists(t, pidFile, "the grandchild must have been started before the cancellation")
	require.ErrorIs(t, err, ErrChildNotReaped)
	// The reap deadline alone is `grace`; spending the drain deadline after it
	// would take twice that. Half a deadline of headroom keeps the bound clear
	// of scheduling noise while still failing on a doubled wait.
	assert.Less(t, elapsed, grace+grace/2,
		"the reap deadline and the drain deadline must not be spent one after the other")

	releaseOnce.Do(func() { close(release) })
}

// killTestProcessFromFile kills the pid recorded in path, if the file exists.
func killTestProcessFromFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil {
		killTestProcess(pid)
	}
}

// TestKillChild_RejectsUndeclaredAndUnavailableStrategies verifies the two
// fail-secure arms of the kill path. A preparedCommand that never declared
// what a kill requires must not be guessed at, and a re-elevated kill with no
// privilege manager must not be attempted -- in both cases no signal is sent
// and the reason is reported.
//
// Neither is reachable while prepareCommand is the only constructor; they are
// tested because the switch they guard sits on a privilege boundary, where a
// silent default would reproduce the EPERM-on-kill failure this task exists to
// fix.
func TestKillChild_RejectsUndeclaredAndUnavailableStrategies(t *testing.T) {
	tests := []struct {
		name    string
		kill    killStrategy
		privMgr runnertypes.PrivilegeManager
		wantErr error
	}{
		{
			name:    "undeclared_strategy",
			kill:    killUnset,
			privMgr: privilegetestutil.NewMockPrivilegeManager(true),
			wantErr: ErrKillStrategyUnset,
		},
		{
			name:    "reelevation_without_privilege_manager",
			kill:    killReelevated,
			privMgr: nil,
			wantErr: ErrNoPrivilegeManager,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewDefaultExecutor(WithPrivilegeManager(tt.privMgr)).(*DefaultExecutor)
			pc := prepareForSupervise(t, e, nil, sleepPath, "30")
			pc.kill = tt.kill
			pid := startForSupervise(t, e, pc)

			err := e.killChild(pc, pc.execCmd.Process, pid)

			require.ErrorIs(t, err, tt.wantErr)
			assert.True(t, processIsRunning(pid), "a rejected kill must not have signalled the child")
			assert.Empty(t, pc.privilegeWindows, "a kill that never opened a window must record none")
		})
	}
}

// TestKillChild_RecordsOnlyWindowsThatOpened verifies what the kill path hands
// to the audit metrics. A window that ran must be recorded under its own
// operation, and a window the privilege manager refused must not be: the
// metrics become the audit log's elevation record, and a phantom escalation
// there is indistinguishable from a real one.
func TestKillChild_RecordsOnlyWindowsThatOpened(t *testing.T) {
	tests := []struct {
		name       string
		failFor    map[runnertypes.Operation]error
		wantWindow bool
	}{
		{name: "window_opened", wantWindow: true},
		{
			name:    "window_refused",
			failFor: map[runnertypes.Operation]error{runnertypes.OperationKillAfterCancel: errors.New("elevation refused")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
			mockPriv.FailFor = tt.failFor
			e := NewDefaultExecutor(WithPrivilegeManager(mockPriv)).(*DefaultExecutor)

			pc := prepareForSupervise(t, e, nil, sleepPath, "30")
			pc.kill = killReelevated
			pid := startForSupervise(t, e, pc)

			err := e.killChild(pc, pc.execCmd.Process, pid)

			if !tt.wantWindow {
				require.ErrorIs(t, err, ErrKillAfterCancel)
				assert.Empty(t, pc.privilegeWindows, "a refused elevation must not be recorded as an opened window")
				return
			}
			require.NoError(t, err)
			require.Len(t, pc.privilegeWindows, 1)
			assert.Equal(t, runnertypes.OperationKillAfterCancel, pc.privilegeWindows[0].op)
			assert.Positive(t, pc.privilegeWindows[0].duration, "the window's duration is what the audit log reports")
		})
	}
}
