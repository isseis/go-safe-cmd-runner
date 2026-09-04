//go:build test

package executor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestExecute_PrepareFailureRecordsCarriedWarnings verifies that a failure in
// the prepare phase does not discard the warnings the phase had already
// collected: prepareCommand returns a non-nil preparedCommand alongside its
// error precisely so executeNormal can hand it to logDeferredWarnings.
//
// The failure is induced after staging has succeeded by replacing pipeFn with
// one that first makes the fresh staging directory read-only (so the
// subsequent cleanup cannot remove it) and then fails, which is the only
// ordering that produces a carried warning on a failing prepare. Making
// prepareCommand return nil on failure, or dropping executeNormal's
// logDeferredWarnings call, leaves no record and fails this test.
//
// This test must not call t.Parallel: it replaces the package-level pipeFn
// and, via captureStderrLines, os.Stderr.
func TestExecute_PrepareFailureRecordsCarriedWarnings(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("removing a read-only directory succeeds as root; the removal failure cannot be induced")
	}

	src := t.TempDir()
	scriptPath := filepath.Join(src, "noop.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	logger, rec := tu.NewRecordingLogger()
	e := NewDefaultExecutor(WithLogger(logger), WithFdExecDisabled())

	identity := openVerifiedIdentityForTest(t, scriptPath)
	plan := &risktypes.VerifiedCommandPlan{
		ResolvedPath: scriptPath,
		Identity:     identity,
		Assessment:   risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
	}
	t.Cleanup(func() { _ = plan.Close() })

	before := scrStageDirs(t)
	sweepNewStageDirs(t, before)

	errPipe := errors.New("pipe creation refused by the test")
	origPipeFn := pipeFn
	t.Cleanup(func() { pipeFn = origPipeFn })
	pipeFn = func() (*os.File, *os.File, error) {
		// Staging has already run by the time prepareCommand creates the
		// pipes, so the staged copy's directory exists and can be locked
		// against the cleanup that the failure below triggers.
		for _, d := range newStageDirs(before, scrStageDirs(t)) {
			_ = os.Chmod(filepath.Join(os.TempDir(), d), 0o500)
		}
		return nil, nil, errPipe
	}

	cmd := createTestCommand(scriptPath, []string{})
	var err error
	lines := captureStderrLines(t, func() {
		_, err = e.Execute(context.Background(), plan, cmd, map[string]string{}, nil)
	})

	require.ErrorIs(t, err, errPipe, "the prepare failure itself must still be reported")

	warns := rec.RecordsAtLevel(slog.LevelWarn)
	require.Len(t, warns, 1, "the warning collected before the prepare failure must not be discarded")
	assert.Equal(t, "Failed to remove staging directory", warns[0].Message)
	warnErr, ok := warns[0].Attrs["error"].(error)
	require.True(t, ok, "the record must carry the carried error under the error attribute")
	assert.ErrorIs(t, warnErr, os.ErrPermission)

	require.Len(t, lines, 1, "the removal failure must also write exactly one last-resort line to stderr")
	assert.Contains(t, lines[0], "WARNING: failed to remove staging directory")
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
