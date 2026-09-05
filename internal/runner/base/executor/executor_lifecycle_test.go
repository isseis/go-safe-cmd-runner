//go:build test

package executor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	privilegetestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Command paths the lifecycle tests drive, resolved the way the executor
// requires them: absolute and symlink-free. executortestutil.ResolveCommand
// does the same thing but cannot be imported here -- that package imports
// executor, and these tests are in package executor.
var (
	shPath    = resolveTestCommand("sh")
	sleepPath = resolveTestCommand("sleep")
)

func resolveTestCommand(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		panic("resolveTestCommand: " + name + ": " + err.Error())
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		panic("resolveTestCommand: " + path + ": " + err.Error())
	}
	return resolved
}

// TestPrepareCommand_ChildStreamsAreOSFiles verifies that prepareCommand
// hands the child *os.File pipe ends (not an io.Writer), which is what keeps
// os/exec from starting a copy goroutine of its own -- both when an
// OutputWriter is supplied and when it is nil.
func TestPrepareCommand_ChildStreamsAreOSFiles(t *testing.T) {
	tests := []struct {
		name         string
		outputWriter OutputWriter
	}{
		{name: "with_output_writer", outputWriter: &noopOutputWriter{}},
		{name: "nil_output_writer", outputWriter: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewDefaultExecutor().(*DefaultExecutor)
			cmd := createTestCommand("/bin/echo", []string{"hello"}, withOutputSizeLimit(1<<20))

			pc, err := e.prepareCommand(context.Background(), nil, "/bin/echo", cmd, nil, tt.outputWriter, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = pc.release() })

			_, stdoutIsFile := pc.execCmd.Stdout.(*os.File)
			_, stderrIsFile := pc.execCmd.Stderr.(*os.File)
			assert.True(t, stdoutIsFile, "Stdout must be an *os.File so os/exec starts no copy goroutine")
			assert.True(t, stderrIsFile, "Stderr must be an *os.File so os/exec starts no copy goroutine")
		})
	}
}

// noopOutputWriter is a minimal OutputWriter that discards everything, used
// where the test only cares about the type handed to exec.Cmd, not the
// bytes written through it.
type noopOutputWriter struct{}

func (*noopOutputWriter) Write(OutputStream, []byte) error { return nil }
func (*noopOutputWriter) Close() error                     { return nil }

// captureStderrLines runs fn with os.Stderr replaced by a pipe and returns
// the lines it wrote, with the trailing newline dropped so an empty write
// yields no lines. Callers assert on the line count, not just the content:
// a duplicated last-resort record (cleanup invoked twice, or a log sneaking
// into stderr) shows up as an extra line and nothing else would catch it.
//
// A test using this helper must not call t.Parallel: replacing os.Stderr
// process-wide would steal the output of other tests in the package.
func captureStderrLines(t *testing.T, fn func()) []string {
	t.Helper()

	readEnd, writeEnd, err := os.Pipe()
	require.NoError(t, err)

	origStderr := os.Stderr
	os.Stderr = writeEnd
	// Restore before reading: io.ReadAll below blocks until the write end is
	// closed, and a panic in fn must not leave os.Stderr pointing at a pipe
	// nobody drains.
	defer func() {
		os.Stderr = origStderr
		_ = writeEnd.Close()
		_ = readEnd.Close()
	}()

	fn()

	os.Stderr = origStderr
	require.NoError(t, writeEnd.Close())

	captured, err := io.ReadAll(readEnd)
	require.NoError(t, err)

	trimmed := strings.TrimSuffix(string(captured), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// TestStageFromFD_ReportsFailuresWithoutLogging verifies that stageFromFD's
// cleanup function reports a removal failure through its return value (not a
// Logger call, since stageFromFD runs inside the privilege window in this
// phase) while still writing a single WARNING line directly to stderr as a
// last resort against emergencyShutdown ending the process first. Both halves
// are pinned: the Logger stays silent and stderr carries exactly one line.
//
// This test must not call t.Parallel: captureStderrLines replaces os.Stderr.
func TestStageFromFD_ReportsFailuresWithoutLogging(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping read-only-directory assertion when running as root")
	}

	logger, rec := tu.NewRecordingLogger()
	e := &DefaultExecutor{Logger: logger}

	identity := openVerifiedIdentityForTest(t, "/bin/echo")
	t.Cleanup(func() { _ = identity.FD.Close() })

	stagedPath, cleanupFn, _, err := e.stageFromFD(identity, nil)
	require.NoError(t, err)
	dir := filepath.Dir(stagedPath)

	// Make the staging directory read-only so os.RemoveAll cannot delete the
	// staged file inside it, forcing cleanupFn to fail (same technique as
	// TestStageFromFD_ChownFailure_CleansUpStagingDir).
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
		_ = os.RemoveAll(dir)
	})

	var cleanupErr error
	lines := captureStderrLines(t, func() { cleanupErr = cleanupFn() })

	require.Error(t, cleanupErr, "cleanupFn must report the removal failure through its return value")
	assert.Empty(t, rec.Records(), "stageFromFD's cleanup must not call Logger")
	require.Len(t, lines, 1, "cleanupFn must write exactly one last-resort line to stderr")
	assert.Contains(t, lines[0], "WARNING: failed to remove staging directory")
}

// TestLogDeferredWarnings_RecordsCarriedFailures verifies that every failure
// preparedCommand carries out of the privilege window is recorded when the
// caller calls logDeferredWarnings. The carry-out exists so these failures --
// each of which the pre-refactor code logged at its own close site -- are not
// lost now that they happen behind release(); dropping any one branch must
// fail this test.
func TestLogDeferredWarnings_RecordsCarriedFailures(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()
	e := &DefaultExecutor{Logger: logger}

	stagingWarnErr := errors.New("close staging source descriptor failed")
	stagingCleanupErr := &os.PathError{Op: "unlink", Path: "/tmp/scr-stage-test/echo", Err: os.ErrPermission}
	devNullCloseErr := errors.New("close null device failed")
	verifiedFDCloseErr := errors.New("close duplicated verified fd failed")
	pumpReleaseErr := errors.New("release output pump failed")

	pc := &preparedCommand{
		stagingWarn:        stagingWarnErr,
		stagingCleanupErr:  stagingCleanupErr,
		devNullCloseErr:    devNullCloseErr,
		verifiedFDCloseErr: verifiedFDCloseErr,
		pumpReleaseErr:     pumpReleaseErr,
	}

	e.logDeferredWarnings(pc)

	want := []struct {
		message string
		err     error
	}{
		{"Failed to close staging source descriptor", stagingWarnErr},
		{"Failed to remove staging directory", stagingCleanupErr},
		{"Failed to close null device", devNullCloseErr},
		{"Failed to close duplicated verified fd", verifiedFDCloseErr},
		{"Failed to release output pump", pumpReleaseErr},
	}

	warns := rec.RecordsAtLevel(slog.LevelWarn)
	require.Len(t, warns, len(want), "every carried failure must produce exactly one Warn record")
	for i, w := range want {
		assert.Equal(t, w.message, warns[i].Message)
		gotErr, ok := warns[i].Attrs["error"].(error)
		require.True(t, ok, "record %d must carry the carried error under the error attribute", i)
		assert.ErrorIs(t, gotErr, w.err)
	}
}

// TestLogDeferredWarnings_SilentWhenNothingCarried verifies the companion
// half: a preparedCommand that carries no failure produces no records. Without
// it, a logDeferredWarnings that logged unconditionally would still satisfy
// TestLogDeferredWarnings_RecordsCarriedFailures's count.
func TestLogDeferredWarnings_SilentWhenNothingCarried(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()
	e := &DefaultExecutor{Logger: logger}

	e.logDeferredWarnings(&preparedCommand{})
	e.logDeferredWarnings(nil)

	assert.Empty(t, rec.Records(), "a preparedCommand carrying no failure must produce no records")
}

// TestExecute_StagedCopyRemovalFailureRecorded verifies end to end that a
// failed staged-copy removal is recorded after the run instead of inside the
// privilege window: the staged copy is a script that locks its own directory
// before exiting, so the parent's post-run removal fails for a non-root
// caller. The run itself still succeeds (a removal failure is not a run
// failure), and exactly one Warn record carries the removal error.
//
// It pins the executeNormal call site of logDeferredWarnings; the
// executeWithUserGroup call site calls the same function, pinned by
// TestLogDeferredWarnings_RecordsCarriedFailures (a run-as end-to-end cannot
// be exercised unprivileged: the kernel refuses the credential's setgroups
// without privilege). Removing the call leaves no record and fails this test.
//
// This test must not call t.Parallel: captureStderrLines replaces os.Stderr.
func TestExecute_StagedCopyRemovalFailureRecorded(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("removing a read-only directory succeeds as root; the removal failure cannot be induced")
	}

	// The staged copy is a script that makes its own directory read-only
	// before exiting, so the parent's post-run RemoveAll fails.
	src := t.TempDir()
	scriptPath := filepath.Join(src, "lockdir.sh")
	script := "#!/bin/sh\nchmod 0500 \"$(dirname \"$0\")\"\necho staged-ok\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	logger, rec := tu.NewRecordingLogger()
	e := NewDefaultExecutor(WithLogger(logger), WithFdExecDisabled())

	identity := openVerifiedIdentityForTest(t, scriptPath)
	plan := &risktypes.VerifiedCommandPlan{
		ResolvedPath: scriptPath,
		Identity:     identity,
		Assessment:   risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
	}
	t.Cleanup(func() { _ = plan.Close() })

	// The failed removal leaves the staging directory behind; note what was
	// there beforehand so the leak can be identified, and arm the sweep now
	// so an assertion failure below cannot leak it.
	before := scrStageDirs(t)
	sweepNewStageDirs(t, before)

	cmd := createTestCommand(scriptPath, []string{})
	var (
		result *Result
		err    error
	)
	lines := captureStderrLines(t, func() {
		result, err = e.Execute(context.Background(), plan, cmd, map[string]string{}, nil)
	})

	require.NoError(t, err, "a staged-copy removal failure must not fail the run")
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "staged-ok")

	// Exactly one Warn record, carrying the removal failure, recorded after
	// the run rather than from inside the privilege window.
	warns := rec.RecordsAtLevel(slog.LevelWarn)
	require.Len(t, warns, 1)
	assert.Equal(t, "Failed to remove staging directory", warns[0].Message)
	warnErr, ok := warns[0].Attrs["error"].(error)
	require.True(t, ok, "the record must carry the carried error under the error attribute")
	assert.ErrorIs(t, warnErr, os.ErrPermission)

	require.Len(t, lines, 1, "the removal failure must also write exactly one last-resort line to stderr")
	assert.Contains(t, lines[0], "WARNING: failed to remove staging directory")

	require.Len(t, newStageDirs(before, scrStageDirs(t)), 1, "the failed removal must leave exactly one staging directory")
}

// TestExecute_CancelledContextReleasesAndRecordsWarnings verifies the two
// halves of prepareCommand's last step. An already-cancelled context must be
// refused before anything is started -- exec.CommandContext used to do that,
// and nothing else does now -- and the release that refusal performs must not
// discard the warnings it collects on the way out: prepareCommand returns a
// non-nil preparedCommand alongside its error precisely so executeNormal can
// hand it to logDeferredWarnings.
//
// The pump's read end is closed behind its back so release fails with EBADF
// rather than the os.ErrClosed it treats as its idempotent success case. It is
// closed only once the second pipe pair exists, so the freed descriptor number
// cannot be handed straight back to it and turn the sabotage into a
// legitimate close.
//
// Dropping the ctx.Err() check leaves no error to report and fails this test;
// dropping executeNormal's logDeferredWarnings call leaves no record and fails
// it too.
//
// This test must not call t.Parallel: it replaces the package-level pipeFn.
func TestExecute_CancelledContextReleasesAndRecordsWarnings(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()
	e := NewDefaultExecutor(WithLogger(logger))

	origPipeFn := pipeFn
	t.Cleanup(func() { pipeFn = origPipeFn })
	var readEnds []*os.File
	pipeFn = func() (*os.File, *os.File, error) {
		r, w, err := origPipeFn()
		if err != nil {
			return r, w, err
		}
		readEnds = append(readEnds, r)
		if len(readEnds) == 2 {
			require.NoError(t, syscall.Close(int(readEnds[0].Fd())))
		}
		return r, w, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := createTestCommand(sleepPath, []string{"10"})
	result, err := e.Execute(ctx, nil, cmd, map[string]string{}, nil)

	require.ErrorIs(t, err, context.Canceled, "an already-cancelled context must be refused before the command starts")
	require.NotNil(t, result, "a refused run must still report a Result the caller can read an exit code from")
	assert.Equal(t, ExitCodeUnknown, result.ExitCode)

	warns := rec.RecordsAtLevel(slog.LevelWarn)
	require.Len(t, warns, 1, "the release failure collected while refusing the run must not be discarded")
	assert.Equal(t, "Failed to release output pump", warns[0].Message)
}

// TestPreparedCommand_ReleaseRecordsPumpFailure verifies that release records
// the output pump's release failure on preparedCommand instead of dropping
// it. release runs inside the privilege window, so its return value is
// discarded by superviseCommand; the recorded field is the only path by which
// this failure reaches a log. It is the one release branch no end-to-end test
// reaches, since closing an already-closed descriptor is not an error.
func TestPreparedCommand_ReleaseRecordsPumpFailure(t *testing.T) {
	pump, err := newOutputPump(nil, 0)
	require.NoError(t, err)

	// Close the read ends behind the pump's back so its own close of them
	// fails with EBADF rather than the os.ErrClosed that release treats as
	// its idempotent success case.
	for _, f := range []*os.File{pump.stdout.parentEnd, pump.stderr.parentEnd} {
		require.NoError(t, syscall.Close(int(f.Fd())))
	}
	t.Cleanup(func() {
		_ = pump.stdout.childEnd.Close()
		_ = pump.stderr.childEnd.Close()
	})

	pc := &preparedCommand{pump: pump}
	releaseErr := pc.release()

	require.Error(t, releaseErr, "release must report the pump failure to callers that keep its return value")
	require.Error(t, pc.pumpReleaseErr, "release must also record the pump failure for logDeferredWarnings")
	assert.ErrorIs(t, releaseErr, pc.pumpReleaseErr)
}

// TestStartPrepared_RejectsSpentCommand verifies that the preparedCommand
// prepareCommand hands back alongside an error cannot be started. Its pump and
// descriptors are already released, so startPrepared must refuse it rather
// than reach execCmd.Start and have runCommand nil-deref the released pump one
// line later. Only the spent flag can catch this: the binding was assigned
// before the failure, so the bindingUnset guard waves such a command through.
func TestStartPrepared_RejectsSpentCommand(t *testing.T) {
	e := NewDefaultExecutor().(*DefaultExecutor)

	pc := &preparedCommand{binding: bindingResolvedPath, spent: true}
	started, err := e.startPrepared(pc)

	assert.False(t, started, "a released command must not be reported as started")
	require.ErrorIs(t, err, ErrPreparedCommandSpent)
}

// sweepNewStageDirs registers a cleanup that chmods and removes every staging
// directory that appears after before was captured. Tests that deliberately
// induce a removal failure leave an unwritable scr-stage-* directory behind;
// registering the sweep up front, rather than after the assertions, means a
// failing assertion cannot abort the test before the cleanup exists and leak
// that directory into os.TempDir() permanently.
func sweepNewStageDirs(t *testing.T, before []string) {
	t.Helper()
	t.Cleanup(func() {
		for _, d := range newStageDirs(before, scrStageDirs(t)) {
			path := filepath.Join(os.TempDir(), d)
			_ = os.Chmod(path, 0o700)
			_ = os.RemoveAll(path)
		}
	})
}

// newStageDirs returns the staging directories present in after but not in
// before.
func newStageDirs(before, after []string) []string {
	existing := make(map[string]struct{}, len(before))
	for _, d := range before {
		existing[d] = struct{}{}
	}
	var added []string
	for _, d := range after {
		if _, ok := existing[d]; !ok {
			added = append(added, d)
		}
	}
	return added
}

// runInsideStartWindow runs pc through runCommand with mockPriv's privilege
// window wrapped around the start phase alone, the way executeWithUserGroup
// does for a run-as command.
//
// pc carries no run-as credential: an unprivileged test cannot start a child
// under one, since the kernel refuses SysProcAttr.Credential's setgroups
// without CAP_SETGID (see the note at the top of executor_supervise_test.go).
// The window is therefore the mock's while the child is a real process that
// really starts inside it -- which is what the goroutine and ordering
// assertions below need, and what a run-as run that fails at Start could not
// give them.
func runInsideStartWindow(t *testing.T, e *DefaultExecutor, mockPriv *privilegetestutil.MockPrivilegeManager, pc *preparedCommand) (*Result, error) {
	t.Helper()
	elevationCtx := runnertypes.ElevationContext{
		Operation:   runnertypes.OperationUserGroupExecution,
		CommandName: "test_cmd",
		RunAsUser:   "testuser",
	}
	return e.runCommand(context.Background(), pc, func(fn func() error) error {
		return mockPriv.WithPrivileges(elevationCtx, fn)
	})
}

// goroutinesExcludedFromWindowCheck lists, by the function named in a
// goroutine's top frame, the goroutines that may come and go at any moment
// for reasons that have nothing to do with the privilege window. Each entry
// is matched as a prefix of the top frame's function name.
//
// The list must hold under both conditions make test runs: with -race and
// with CGO_ENABLED=0 and no race detector.
var goroutinesExcludedFromWindowCheck = []string{
	// Go runtime goroutines the scheduler starts on demand: a GC cycle
	// beginning inside the window would otherwise be reported as a goroutine
	// the window started.
	"runtime.gcBgMarkWorker",
	"runtime.bgsweep",
	"runtime.bgscavenge",
	"runtime.forcegchelper",
	// Started by the runtime the first time an object with a finalizer or
	// cleanup is collected, which any allocation in the window can trigger.
	"runtime.runfinq",
	"runtime.runCleanups",
	// The race detector's own background goroutine, present only in the
	// -race half of make test.
	"runtime.racefini",
	// The testing package's per-test goroutine, plus the sampling goroutine
	// runtime.Stack itself reports.
	"testing.tRunner",
	"testing.(*T).Run",
	"runtime/pprof.writeGoroutineStacks",
	// The logging package's Slack notification worker, which the default
	// logger may start while the window is open.
	"github.com/isseis/go-safe-cmd-runner/internal/logging",
}

// liveGoroutineIDs returns the IDs of the goroutines that exist right now,
// mapped to the function named in each one's top frame, with
// goroutinesExcludedFromWindowCheck left out.
//
// Comparing IDs rather than whole stack strings is deliberate: a goroutine
// that merely changes state ([running] to [chan receive]) would otherwise
// register as a difference. The buffer is grown until runtime.Stack fits in
// it, since a truncated dump silently loses goroutines and would make the
// comparison below pass for the wrong reason.
func liveGoroutineIDs(t *testing.T) map[int]string {
	t.Helper()

	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}

	ids := make(map[int]string)
	for block := range strings.SplitSeq(string(buf), "\n\n") {
		header, rest, ok := strings.Cut(block, "\n")
		if !ok || !strings.HasPrefix(header, "goroutine ") {
			continue
		}
		idField, _, ok := strings.Cut(strings.TrimPrefix(header, "goroutine "), " ")
		if !ok {
			continue
		}
		id, err := strconv.Atoi(idField)
		require.NoError(t, err, "unparsable goroutine header %q", header)

		topFrame, _, _ := strings.Cut(rest, "\n")
		fn, _, _ := strings.Cut(topFrame, "(")
		if slices.ContainsFunc(goroutinesExcludedFromWindowCheck, func(prefix string) bool {
			return strings.HasPrefix(fn, prefix)
		}) {
			continue
		}
		ids[id] = fn
	}
	return ids
}

// newGoroutines returns the entries of got whose IDs are absent from baseline.
func newGoroutines(baseline, got map[int]string) map[int]string {
	added := make(map[int]string)
	for id, fn := range got {
		if _, ok := baseline[id]; !ok {
			added[id] = fn
		}
	}
	return added
}

// TestStartPrepared_NoGoroutineInsideWindow verifies that no goroutine comes
// into existence while the privilege window is open: every goroutine the run
// needs -- the output pump's two readers and the wait goroutine -- belongs to
// the supervision phase, which runs after the window has closed.
//
// The sample taken after the start phase returns is the one that matters.
// os/exec starts a copy goroutine per stream from inside Start when Stdout or
// Stderr is an io.Writer rather than an *os.File, and such a goroutine does
// not exist yet when the before-fn sample is taken: a test that sampled only
// before fn would stay green after reverting the *os.File binding.
//
// Goroutines are compared by ID, and the comparison deliberately does not
// filter by executor frames -- the os/exec copy goroutine's stack names only
// io.Copy and internal/poll, so filtering would hide exactly the regression
// this test exists to catch.
func TestStartPrepared_NoGoroutineInsideWindow(t *testing.T) {
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	e := NewDefaultExecutor(WithPrivilegeManager(mockPriv)).(*DefaultExecutor)

	samples := make(map[privilegetestutil.MockWindowPhase]map[int]string)
	mockPriv.InWindow = func(phase privilegetestutil.MockWindowPhase) {
		samples[phase] = liveGoroutineIDs(t)
	}

	pc := prepareForSupervise(t, e, nil, shPath, "-c", "echo window-ok")

	baseline := liveGoroutineIDs(t)
	result, err := runInsideStartWindow(t, e, mockPriv, pc)

	// Asserted before the run's outcome: a copy goroutine started inside the
	// window also breaks the run (it writes to a pipe end the caller closes as
	// soon as the window shuts), and checking the outcome first would report
	// that consequence instead of the goroutine this test is about.
	for _, phase := range []privilegetestutil.MockWindowPhase{
		privilegetestutil.MockWindowPhaseBeforeFn,
		privilegetestutil.MockWindowPhaseAfterFn,
	} {
		sample, ok := samples[phase]
		require.Truef(t, ok, "the window must have been sampled at phase %d", phase)
		assert.Emptyf(t, newGoroutines(baseline, sample),
			"no goroutine may come into existence inside the privilege window (phase %d)", phase)
	}

	require.NoError(t, err)
	assert.Equal(t, "window-ok\n", result.Stdout)
}

// TestStartPrepared_WaitAndPumpRunOutsideWindow verifies the time ordering the
// static window guard cannot state: reading the child's output and waiting for
// it to exit do not begin until the privilege window has closed. Where the
// guard fixes what the window may call, this fixes when the rest of the run
// starts.
//
// Both phases are sampled because the regression each catches is different: a
// pump started before the start phase shows up in the before-fn sample, while
// one started by startPrepared itself only exists once fn has returned.
func TestStartPrepared_WaitAndPumpRunOutsideWindow(t *testing.T) {
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)

	var waitCalled atomic.Bool
	e := NewDefaultExecutor(
		WithPrivilegeManager(mockPriv),
		WithWaitFn(func(cmd *exec.Cmd) error {
			waitCalled.Store(true)
			return cmd.Wait()
		}),
	).(*DefaultExecutor)

	pc := prepareForSupervise(t, e, nil, shPath, "-c", "echo ordering-ok")

	type sample struct {
		pumpStarted bool
		waitCalled  bool
	}
	samples := make(map[privilegetestutil.MockWindowPhase]sample)
	mockPriv.InWindow = func(phase privilegetestutil.MockWindowPhase) {
		samples[phase] = sample{pumpStarted: pc.pump.started, waitCalled: waitCalled.Load()}
	}

	result, err := runInsideStartWindow(t, e, mockPriv, pc)

	require.NoError(t, err)
	assert.Equal(t, "ordering-ok\n", result.Stdout)
	assert.True(t, waitCalled.Load(), "the wait goroutine must have run by the end of the run")

	for _, phase := range []privilegetestutil.MockWindowPhase{
		privilegetestutil.MockWindowPhaseBeforeFn,
		privilegetestutil.MockWindowPhaseAfterFn,
	} {
		got, ok := samples[phase]
		require.Truef(t, ok, "the window must have been sampled at phase %d", phase)
		assert.Falsef(t, got.pumpStarted, "the output pump must not be reading inside the window (phase %d)", phase)
		assert.Falsef(t, got.waitCalled, "the child must not be waited on inside the window (phase %d)", phase)
	}
}

// prepareWithVerifiedIdentity builds a preparedCommand bound to path's
// verified inode, so the binding under test is the one the risk evaluator's
// plan would produce rather than the plain resolved path.
func prepareWithVerifiedIdentity(t *testing.T, e *DefaultExecutor, path string, args ...string) *preparedCommand {
	t.Helper()
	identity := openVerifiedIdentityForTest(t, path)
	t.Cleanup(func() { _ = identity.FD.Close() })
	plan := &risktypes.VerifiedCommandPlan{
		ResolvedPath: path,
		Identity:     identity,
		Assessment:   risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
	}
	pc, err := e.prepareCommand(context.Background(), plan, path, createTestCommand(path, args), nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.release() })
	return pc
}

// TestExecute_SingleElevationPairPerRun verifies how many privilege windows a
// run opens: exactly one for the start phase, plus one more -- and only in the
// staging fallback -- to remove the staged copy the start window created under
// root ownership. Every elevation/restore pair the run performs is one call to
// the privilege manager, so the mock's call list is the count.
//
// The run-as credential is stood in for rather than applied, because an
// unprivileged test cannot complete a run under one (see runInsideStartWindow).
// The credential's own effect -- which cleanup strategy prepareCommand
// declares from it -- is asserted separately below, and the audit metrics
// assembled from these windows are asserted end to end by the privileged
// integration tests.
func TestExecute_SingleElevationPairPerRun(t *testing.T) {
	t.Run("fd_bound_opens_only_the_start_window", func(t *testing.T) {
		if !fdExecSupported() {
			t.Skip("fd-bound execution is not supported on this platform")
		}
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
		e := NewDefaultExecutor(WithPrivilegeManager(mockPriv)).(*DefaultExecutor)

		pc := prepareWithVerifiedIdentity(t, e, shPath, "-c", "echo fd-bound-ok")
		require.Equal(t, bindingVerifiedFD, pc.binding)

		result, err := runInsideStartWindow(t, e, mockPriv, pc)

		require.NoError(t, err)
		assert.Equal(t, "fd-bound-ok\n", result.Stdout)
		assert.Equal(t, []string{"user_group_change:testuser:"}, mockPriv.ElevationCalls,
			"an fd-bound run must open the start window and nothing else")
		assert.Empty(t, pc.privilegeWindows, "the supervision phase must open no window of its own")
	})

	t.Run("staging_fallback_adds_the_cleanup_window", func(t *testing.T) {
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
		e := NewDefaultExecutor(WithPrivilegeManager(mockPriv), WithFdExecDisabled()).(*DefaultExecutor)

		pc := prepareWithVerifiedIdentity(t, e, shPath, "-c", "echo staged-ok")
		require.Equal(t, bindingStagedCopy, pc.binding)
		// Stands in for the run-as credential: the copy the start window makes
		// would then be root-owned, which is what the cleanup window is for.
		pc.cleanup = cleanupElevated

		result, err := runInsideStartWindow(t, e, mockPriv, pc)

		require.NoError(t, err)
		assert.Equal(t, "staged-ok\n", result.Stdout)
		assert.Equal(t, []string{"user_group_change:testuser:", string(runnertypes.OperationStagingCleanup)},
			mockPriv.ElevationCalls,
			"the staging fallback must open the start window and then the cleanup window")

		require.Len(t, pc.privilegeWindows, 1, "the cleanup window must be reported to the audit metrics")
		assert.Equal(t, runnertypes.OperationStagingCleanup, pc.privilegeWindows[0].op)
		assert.Positive(t, pc.privilegeWindows[0].duration, "the window's duration is what the audit log reports")

		assert.NoDirExists(t, filepath.Dir(pc.stagedPath), "the staged copy must be gone once the run has finished")
	})

	t.Run("prepare_declares_the_cleanup_strategy_from_the_credential", func(t *testing.T) {
		tests := []struct {
			name string
			cred *syscall.Credential
			want stagingCleanupStrategy
		}{
			{name: "run_as", cred: &syscall.Credential{Uid: 1000, Gid: 1000}, want: cleanupElevated},
			{name: "normal", cred: nil, want: cleanupDirect},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				e := NewDefaultExecutor(WithFdExecDisabled()).(*DefaultExecutor)
				identity := openVerifiedIdentityForTest(t, shPath)
				t.Cleanup(func() { _ = identity.FD.Close() })
				plan := &risktypes.VerifiedCommandPlan{
					ResolvedPath: shPath,
					Identity:     identity,
					Assessment:   risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
				}

				pc, err := e.prepareCommand(context.Background(), plan, shPath, createTestCommand(shPath, nil), nil, nil, tt.cred)
				require.NoError(t, err)
				t.Cleanup(func() { _ = pc.release() })

				require.Equal(t, bindingStagedCopy, pc.binding)
				assert.Equal(t, tt.want, pc.cleanup)
			})
		}
	})
}

// TestExecute_ShebangScriptRunsUnderStagingFallback verifies that the staged
// copy outlives execve. A "#!" script's interpreter opens the script by path
// after execve has returned, so removing the copy as soon as Start succeeded
// would break such a script at an undefined moment; removing it only once the
// child has exited is what keeps the staging fallback able to run the same
// commands as fd-bound execution.
func TestExecute_ShebangScriptRunsUnderStagingFallback(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "shebang.sh")
	script := "#!" + shPath + "\necho shebang-ok\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	e := NewDefaultExecutor(WithFdExecDisabled())

	identity := openVerifiedIdentityForTest(t, scriptPath)
	plan := &risktypes.VerifiedCommandPlan{
		ResolvedPath: scriptPath,
		Identity:     identity,
		Assessment:   risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
	}
	t.Cleanup(func() { _ = plan.Close() })

	before := scrStageDirs(t)
	result, err := e.Execute(context.Background(), plan, createTestCommand(scriptPath, nil), map[string]string{}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "shebang-ok\n", result.Stdout)
	assert.Empty(t, newStageDirs(before, scrStageDirs(t)), "the staged copy must be removed once the child has exited")
}

// TestRemoveStagedCopy_NormalExecutionDoesNotElevate verifies the narrow half
// of the cleanup strategy: a staged copy the invoking user already owns is
// removed without asking for privilege. Widening the condition to "whenever a
// copy was staged" would make every non-run-as staging run request an
// elevation it has no reason to need -- and fail outright on a binary that was
// never installed setuid, turning runs that succeed today into errors.
func TestRemoveStagedCopy_NormalExecutionDoesNotElevate(t *testing.T) {
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	e := NewDefaultExecutor(WithPrivilegeManager(mockPriv), WithFdExecDisabled()).(*DefaultExecutor)

	pc := prepareWithVerifiedIdentity(t, e, shPath, "-c", "echo unprivileged-ok")
	require.Equal(t, bindingStagedCopy, pc.binding)
	// Left exactly as prepareCommand declared it for a run with no run-as
	// credential; nothing here stands in for one.
	require.Equal(t, cleanupDirect, pc.cleanup)

	result, err := runInsideStartWindow(t, e, mockPriv, pc)

	require.NoError(t, err)
	assert.Equal(t, "unprivileged-ok\n", result.Stdout)
	assert.Equal(t, []string{"user_group_change:testuser:"}, mockPriv.ElevationCalls,
		"removing a staged copy the invoking user owns must open no window")
	assert.Empty(t, pc.privilegeWindows, "no cleanup window opened, so none may be recorded")
	assert.NoDirExists(t, filepath.Dir(pc.stagedPath), "the staged copy must still be removed")
}

// TestStartPrepared_StartFailureRemovesStagedCopyInsideWindow verifies where
// the staged copy goes when Start fails. It must be removed while the window
// is still open: for a run-as command the staging directory is root-owned, and
// the caller is back at the invoking user's effective uid by the time it sees
// the failure.
//
// The assertion is made from inside the window rather than after the run,
// because preparedCommand.release removes the copy too on the unprivileged
// path every unit test takes -- an "it is gone afterwards" check would pass
// with the in-window removal deleted.
func TestStartPrepared_StartFailureRemovesStagedCopyInsideWindow(t *testing.T) {
	// A file with neither a shebang nor a recognized binary format: staging
	// copies it happily, and execve then refuses it with ENOEXEC, which is a
	// Start failure reached after the copy exists.
	notAProgram := filepath.Join(t.TempDir(), "not-a-program")
	require.NoError(t, os.WriteFile(notAProgram, []byte("this is not a program\n"), 0o755))

	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	e := NewDefaultExecutor(WithPrivilegeManager(mockPriv), WithFdExecDisabled()).(*DefaultExecutor)

	pc := prepareWithVerifiedIdentity(t, e, notAProgram)
	require.Equal(t, bindingStagedCopy, pc.binding)

	type sample struct {
		cleanupPending bool
		dirExists      bool
	}
	var afterFn sample
	var sampled bool
	mockPriv.InWindow = func(phase privilegetestutil.MockWindowPhase) {
		if phase != privilegetestutil.MockWindowPhaseAfterFn {
			return
		}
		sampled = true
		_, statErr := os.Stat(filepath.Dir(pc.stagedPath))
		afterFn = sample{cleanupPending: pc.stagingCleanup != nil, dirExists: statErr == nil}
	}

	before := scrStageDirs(t)
	result, err := runInsideStartWindow(t, e, mockPriv, pc)

	require.True(t, sampled, "the window must have been sampled after the start phase returned")
	assert.False(t, afterFn.cleanupPending, "the staged copy must be removed before the window closes")
	assert.False(t, afterFn.dirExists, "the staging directory must be gone before the window closes")

	require.Error(t, err, "a command execve refuses must fail the run")
	require.NotNil(t, result)
	assert.Equal(t, ExitCodeUnknown, result.ExitCode)
	assert.Empty(t, newStageDirs(before, scrStageDirs(t)))
}

// TestRemoveStagedCopy_RejectsUndeclaredAndUnavailableStrategies verifies the
// two fail-secure arms of the cleanup path, mirroring what
// TestKillChild_RejectsUndeclaredAndUnavailableStrategies pins for the kill
// path. Neither is reachable while prepareCommand is the only constructor;
// they are tested because the switch they guard sits on a privilege boundary,
// where a silent default would either leak a root-owned copy of a verified
// binary into $TMPDIR or elevate a run that had no reason to.
//
// Each arm must also leave a trace: the caller discards this return value
// (the removal may run inside the cleanup window), so a failure that is not
// recorded on the preparedCommand reaches no log at all.
func TestRemoveStagedCopy_RejectsUndeclaredAndUnavailableStrategies(t *testing.T) {
	tests := []struct {
		name    string
		cleanup stagingCleanupStrategy
		privMgr runnertypes.PrivilegeManager
		wantErr error
	}{
		{
			name:    "undeclared_strategy",
			cleanup: cleanupUnset,
			privMgr: privilegetestutil.NewMockPrivilegeManager(true),
			wantErr: ErrStagingCleanupStrategyUnset,
		},
		{
			name:    "elevated_without_privilege_manager",
			cleanup: cleanupElevated,
			privMgr: nil,
			wantErr: ErrNoPrivilegeManager,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewDefaultExecutor(WithPrivilegeManager(tt.privMgr)).(*DefaultExecutor)

			removed := false
			pc := &preparedCommand{
				cleanup:        tt.cleanup,
				stagingCleanup: func() error { removed = true; return nil },
			}

			err := e.removeStagedCopy(pc)

			require.ErrorIs(t, err, tt.wantErr)
			assert.False(t, removed, "a rejected cleanup must not have removed anything")
			assert.ErrorIs(t, pc.stagingWindowErr, tt.wantErr,
				"the reason the copy was left behind must be recorded for the caller to log")
		})
	}
}

// TestRunCommand_VerifiedFDCloseFailureDoesNotKillChild verifies that a
// failure to close the duplicated verified descriptor is reported as a warning
// and nothing more. Start has already copied that descriptor into the child by
// the time it is closed, so unlike the pipe write ends -- whose read ends would
// never reach EOF -- it has no bearing on a child that is running. Folding it
// into the error handed to the supervision phase would send a healthy command
// straight to the kill path and report it as failed.
//
// The descriptor is closed behind the preparedCommand's back from inside the
// window, after the start phase has returned, so the close that follows fails
// with EBADF rather than the os.ErrClosed release treats as success.
func TestRunCommand_VerifiedFDCloseFailureDoesNotKillChild(t *testing.T) {
	if !fdExecSupported() {
		t.Skip("fd-bound execution is not supported on this platform")
	}

	logger, rec := tu.NewRecordingLogger()
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	e := NewDefaultExecutor(WithPrivilegeManager(mockPriv), WithLogger(logger)).(*DefaultExecutor)

	pc := prepareWithVerifiedIdentity(t, e, shPath, "-c", "echo fd-close-ok")
	require.Equal(t, bindingVerifiedFD, pc.binding)

	mockPriv.InWindow = func(phase privilegetestutil.MockWindowPhase) {
		if phase == privilegetestutil.MockWindowPhaseAfterFn {
			require.NoError(t, syscall.Close(int(pc.verifiedFD.Fd())))
		}
	}

	result, err := runInsideStartWindow(t, e, mockPriv, pc)

	require.NoError(t, err, "a verified-fd close failure must not fail the run")
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode, "the child must be reaped normally, not killed")
	assert.Equal(t, "fd-close-ok\n", result.Stdout)

	warns := rec.RecordsAtLevel(slog.LevelWarn)
	require.Len(t, warns, 1, "the close failure must still be reported")
	assert.Equal(t, "Failed to close duplicated verified fd", warns[0].Message)
}

// TestRunCommand_StartWindowThatRunsNothingIsRejected verifies that a start
// window returning without having run the start phase, and without reporting
// why, is refused rather than passed on. Such a window leaves nothing started
// and nothing to report, so the alternative is a nil Result alongside a nil
// error -- which executeWithUserGroup's audit block would dereference.
//
// No PrivilegeManager in this repository behaves this way; the guard sits
// where a caller-supplied implementation could, and fails closed.
func TestRunCommand_StartWindowThatRunsNothingIsRejected(t *testing.T) {
	e := NewDefaultExecutor().(*DefaultExecutor)
	pc := prepareForSupervise(t, e, nil, shPath, "-c", "echo never-runs")

	result, err := e.runCommand(context.Background(), pc, func(func() error) error {
		return nil // never calls fn, and reports no reason
	})

	assert.Nil(t, result, "no Result may be reported for a command that was never attempted")
	require.ErrorIs(t, err, ErrStartPhaseNotRun)
}
