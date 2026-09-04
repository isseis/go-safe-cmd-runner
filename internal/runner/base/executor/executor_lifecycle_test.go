//go:build test

package executor

import (
	"context"
	"os"
	"strings"
	"testing"

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

// TestStageFromFD_ReportsFailuresWithoutLogging verifies that stageFromFD's
// cleanup function reports a removal failure through its return value (not a
// Logger call, since stageFromFD runs inside the privilege window in this
// phase) while still writing a single WARNING line directly to stderr as a
// last resort against emergencyShutdown ending the process first.
func TestStageFromFD_ReportsFailuresWithoutLogging(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping read-only-directory assertion when running as root")
	}

	logger, rec := tu.NewRecordingLogger()
	e := &DefaultExecutor{Logger: logger}

	identity := openVerifiedIdentityForTest(t, "/bin/echo")
	defer func() { _ = identity.FD.Close() }()

	stagedPath, cleanupFn, _, err := e.stageFromFD(identity, nil)
	require.NoError(t, err)
	dir := stagedPath[:strings.LastIndex(stagedPath, "/")]

	// Make the staging directory read-only so os.RemoveAll cannot delete the
	// staged file inside it, forcing cleanupFn to fail (same technique as
	// TestStageFromFD_ChownFailure_CleansUpStagingDir).
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
		_ = os.RemoveAll(dir)
	})

	stderrR, stderrW, err := os.Pipe()
	require.NoError(t, err)
	origStderr := os.Stderr
	os.Stderr = stderrW
	cleanupErr := cleanupFn()
	os.Stderr = origStderr
	require.NoError(t, stderrW.Close())

	var buf strings.Builder
	buf.Grow(4096)
	chunk := make([]byte, 4096)
	for {
		n, readErr := stderrR.Read(chunk)
		buf.Write(chunk[:n])
		if readErr != nil {
			break
		}
	}
	_ = stderrR.Close()

	require.Error(t, cleanupErr, "cleanupFn must report the removal failure through its return value")
	assert.Empty(t, rec.Records(), "stageFromFD's cleanup must not call Logger")
	assert.Contains(t, buf.String(), "WARNING: failed to remove staging directory", "cleanupFn must still write a last-resort line to stderr")
}
