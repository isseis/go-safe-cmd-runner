//go:build test

package executor

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

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
	assert.Contains(t, string(captured), "WARNING: failed to remove staging directory")
}
