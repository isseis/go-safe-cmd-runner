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
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrepareCommand_ChildStreamsAreOSFiles verifies that prepareCommand hands
// the child the pipe write ends as *os.File for both streams, with or without
// an OutputWriter: only *os.File keeps os/exec from starting a copy goroutine
// per stream, which is what keeps the start window free of the executor's own
// goroutines.
func TestPrepareCommand_ChildStreamsAreOSFiles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		writer OutputWriter
	}{
		{name: "with_output_writer", writer: &streamRecorder{}},
		{name: "without_output_writer", writer: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewDefaultExecutor().(*DefaultExecutor)
			cmd := createTestCommand("/bin/echo", []string{"hello"})

			pc, err := e.prepareCommand(context.Background(), nil, "/bin/echo", cmd, map[string]string{}, tc.writer, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = pc.release() })

			_, ok := pc.execCmd.Stdout.(*os.File)
			require.True(t, ok, "Stdout must be the *os.File pipe end, got %T", pc.execCmd.Stdout)
			_, ok = pc.execCmd.Stderr.(*os.File)
			require.True(t, ok, "Stderr must be the *os.File pipe end, got %T", pc.execCmd.Stderr)
		})
	}
}

// TestStageFromFD_ReportsFailuresWithoutLogging verifies that a failed
// staging-directory removal is reported as a return value, not through the
// logger: stageFromFD and its cleanup run inside the privilege window, where
// nothing may log (a slog handler is free to open a file, and it would do so
// at euid 0), so the failure travels out on the cleanup error while its
// last-resort line goes to the already-open stderr descriptor. Both halves
// are pinned here: the Logger stays silent and stderr carries the warning.
//
// This test must not call t.Parallel: it temporarily replaces os.Stderr,
// which would steal the output of other tests in the package.
func TestStageFromFD_ReportsFailuresWithoutLogging(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("removing a read-only directory succeeds as root; the cleanup failure cannot be induced")
	}

	logger, rec := tu.NewRecordingLogger()
	e := &DefaultExecutor{Logger: logger}

	identity := openVerifiedIdentityForTest(t, "/bin/echo")
	t.Cleanup(func() { _ = identity.FD.Close() })

	pc := &preparedCommand{}
	stagedPath, cleanup, err := e.stageFromFD(pc, identity, nil)
	require.NoError(t, err)
	dir := filepath.Dir(stagedPath)
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
		_ = os.RemoveAll(dir)
	})

	// A read-only directory blocks RemoveAll for a non-root caller: the
	// staged file inside cannot be unlinked.
	require.NoError(t, os.Chmod(dir, 0o500))

	// Capture stderr: the last-resort record goes there, not through the
	// logger.
	oldStderr := os.Stderr
	readEnd, writeEnd, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writeEnd
	t.Cleanup(func() {
		os.Stderr = oldStderr
		_ = writeEnd.Close()
		_ = readEnd.Close()
	})

	require.Error(t, cleanup(), "cleanup must return the failed removal, not swallow it")
	require.Empty(t, rec.Records(), "stageFromFD must not log inside the privilege window")

	_ = writeEnd.Close()
	captured, err := io.ReadAll(readEnd)
	require.NoError(t, err)
	_ = readEnd.Close()
	// Exactly one line: the last-resort record. A duplicated line (cleanup
	// invoked twice, or a log sneaking into stderr) would add a line.
	lines := strings.Split(strings.TrimSuffix(string(captured), "\n"), "\n")
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "WARNING: failed to remove staging directory")
}

// TestLogStagingWarnings_RecordsCarriedFailures verifies that the two
// non-fatal staging failures carried on preparedCommand are recorded when
// the caller calls logStagingWarnings: the carry-out exists so these
// failures are not lost, and gutting the function must fail this test.
func TestLogStagingWarnings_RecordsCarriedFailures(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()
	e := &DefaultExecutor{Logger: logger}

	stagingWarnErr := errors.New("close staging source fd failed")
	cleanupErr := &os.PathError{Op: "unlink", Path: "/tmp/scr-stage-test/echo", Err: os.ErrPermission}
	pc := &preparedCommand{stagingWarn: stagingWarnErr, stagingCleanupErr: cleanupErr}

	e.logStagingWarnings(pc)

	warns := rec.RecordsAtLevel(slog.LevelWarn)
	require.Len(t, warns, 2)
	warnErr, ok := warns[0].Attrs["error"].(error)
	require.True(t, ok, "the record must carry the carried error under the error attribute")
	assert.Equal(t, "Failed to close staging source fd", warns[0].Message)
	assert.ErrorIs(t, warnErr, stagingWarnErr)
	cleanupWarnErr, ok := warns[1].Attrs["error"].(error)
	require.True(t, ok, "the record must carry the carried error under the error attribute")
	assert.Equal(t, "Failed to remove staging directory", warns[1].Message)
	assert.ErrorIs(t, cleanupWarnErr, cleanupErr)
}

// TestExecute_StagedCopyRemovalFailureRecorded verifies end to end that a
// failed staged-copy removal is recorded after the run instead of inside
// the privilege window: the staged copy is a script that locks its own
// directory before exiting, so the parent's post-run removal fails for a
// non-root caller. The run itself still succeeds (a removal failure is not
// a run failure), and exactly one Warn record carries the removal error. It
// pins the executeNormal call site of logStagingWarnings; the
// executeWithUserGroup call site calls the same function, pinned by
// TestLogStagingWarnings_RecordsCarriedFailures (a run-as end-to-end cannot
// be exercised unprivileged: the kernel refuses the credential's
// setgroups without privilege). A removed call leaves no record and fails
// this test.
//
// This test must not call t.Parallel: it temporarily replaces os.Stderr.
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

	// The failed removal leaves the staging directory behind; restore it so
	// it can be cleaned up.
	before := scrStageDirs(t)

	// Capture stderr: the removal failure also writes its last-resort line
	// there.
	oldStderr := os.Stderr
	readEnd, writeEnd, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writeEnd
	t.Cleanup(func() {
		os.Stderr = oldStderr
		_ = writeEnd.Close()
		_ = readEnd.Close()
	})

	cmd := createTestCommand(scriptPath, []string{})
	result, err := e.Execute(context.Background(), plan, cmd, map[string]string{}, nil)
	require.NoError(t, err, "a staged-copy removal failure must not fail the run")
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "staged-ok")

	// Exactly one Warn record, carrying the removal failure, recorded
	// after the run.
	warns := rec.RecordsAtLevel(slog.LevelWarn)
	require.Len(t, warns, 1)
	assert.Equal(t, "Failed to remove staging directory", warns[0].Message)
	warnErr, ok := warns[0].Attrs["error"].(error)
	require.True(t, ok, "the record must carry the carried error under the error attribute")
	assert.ErrorIs(t, warnErr, os.ErrPermission)

	_ = writeEnd.Close()
	captured, err := io.ReadAll(readEnd)
	require.NoError(t, err)
	_ = readEnd.Close()
	lines := strings.Split(strings.TrimSuffix(string(captured), "\n"), "\n")
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "WARNING: failed to remove staging directory")

	after := scrStageDirs(t)
	var leaked []string
	for _, d := range after {
		found := false
		for _, b := range before {
			if b == d {
				found = true
			}
		}
		if !found {
			leaked = append(leaked, d)
		}
	}
	require.Len(t, leaked, 1, "the failed removal must leave exactly one staging directory")
	t.Cleanup(func() {
		for _, d := range leaked {
			_ = os.Chmod(filepath.Join(os.TempDir(), d), 0o700)
			_ = os.RemoveAll(filepath.Join(os.TempDir(), d))
		}
	})
}
