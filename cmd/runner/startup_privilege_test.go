package main

// Tests in this file must not call t.Parallel(): they mutate process-wide
// state -- the effective user and group IDs, os.Stdout and os.Stderr -- which
// would corrupt any test running concurrently, including the flag-manipulating
// tests in main_test.go that share this package.
//
// The two fail-closed tests skip when the effective user ID is 0, because root
// can reach every target and the failure they exercise cannot occur. Running
// `make test` as root therefore drops that coverage silently. GitHub Actions'
// ubuntu-latest runner executes as a non-root user, so CI does run them; if the
// CI configuration ever changes to a root container, this coverage disappears
// without any test failing to announce it.

import (
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropStartupPrivileges_FailsClosedOnSetegidFailure(t *testing.T) {
	if syscall.Geteuid() == 0 {
		t.Skip("running as root: setegid(0) succeeds, so the failure path cannot be exercised")
	}

	euidBefore := syscall.Geteuid()
	egidBefore := syscall.Getegid()

	err := dropStartupPrivileges(os.Getuid(), 0)

	privErr, ok := errors.AsType[*startupPrivilegeError](err)
	require.True(t, ok, "expected a *startupPrivilegeError, got %v", err)
	assert.Equal(t, stageSetegid, privErr.Stage)

	assert.Equal(t, egidBefore, syscall.Getegid(), "effective GID must be unchanged when setegid fails")
	assert.Equal(t, euidBefore, syscall.Geteuid(), "effective UID must be unchanged: seteuid must not run after setegid fails")
}

func TestDropStartupPrivileges_FailsClosedOnSeteuidFailure(t *testing.T) {
	if syscall.Geteuid() == 0 {
		t.Skip("running as root: seteuid(0) succeeds, so the failure path cannot be exercised")
	}

	euidBefore := syscall.Geteuid()

	// setegid to the current GID succeeds, so the run reaches seteuid(0), which
	// an unprivileged process cannot perform.
	err := dropStartupPrivileges(0, os.Getgid())

	privErr, ok := errors.AsType[*startupPrivilegeError](err)
	require.True(t, ok, "expected a *startupPrivilegeError, got %v", err)
	assert.Equal(t, stageSeteuid, privErr.Stage)

	assert.Equal(t, euidBefore, syscall.Geteuid(), "effective UID must be unchanged when seteuid fails")
}

func TestDropStartupPrivileges_SucceedsForCurrentIdentity(t *testing.T) {
	require.NoError(t, dropStartupPrivileges(syscall.Getuid(), syscall.Getgid()))

	assert.Equal(t, syscall.Getgid(), syscall.Getegid(), "effective GID must equal the real GID after the drop")
	assert.Equal(t, syscall.Getuid(), syscall.Geteuid(), "effective UID must equal the real UID after the drop")
}

func TestReportStartupPrivilegeFailure_UsesValidRunID(t *testing.T) {
	stdout, stderr := captureStdoutStderr(t, func() {
		assert.NotZero(t, reportStartupPrivilegeFailure(&startupPrivilegeError{
			Stage: stageSetegid,
			Err:   syscall.EPERM,
		}), "a failed privilege drop must exit non-zero")
	})

	runIDValue := runSummaryRunID(t, stdout)
	assert.NotEmpty(t, runIDValue, "the report must carry a run ID")
	assert.NoError(t, logging.ValidateRunID(runIDValue), "the generated run ID must satisfy the accepted format")

	assert.Contains(t, stderr, string(logging.ErrorTypePrivilegeDrop))
	assert.Contains(t, stderr, string(stageSetegid), "the failed stage must be identifiable from stderr")
}

// runSummaryRunID returns the run_id field of the RUN_SUMMARY line in stdout.
func runSummaryRunID(t *testing.T, stdout string) string {
	t.Helper()

	for line := range strings.SplitSeq(stdout, "\n") {
		if !strings.HasPrefix(line, "RUN_SUMMARY ") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if value, found := strings.CutPrefix(field, "run_id="); found {
				return value
			}
		}
	}
	t.Fatalf("no RUN_SUMMARY line with a run_id field in stdout: %q", stdout)
	return ""
}

// captureStdoutStderr runs fn with os.Stdout and os.Stderr redirected to pipes
// and returns what it wrote to each.
func captureStdoutStderr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	outReader, outWriter, err := os.Pipe()
	require.NoError(t, err)
	errReader, errWriter, err := os.Pipe()
	require.NoError(t, err)

	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outWriter, errWriter
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origStdout, origStderr
		// The write ends are normally already closed by the time fn returns;
		// closing again here only matters if fn panicked.
		_ = outWriter.Close()
		_ = errWriter.Close()
		_ = outReader.Close()
		_ = errReader.Close()
	})

	fn()

	// Close the write ends before reading so io.ReadAll sees EOF.
	require.NoError(t, outWriter.Close())
	require.NoError(t, errWriter.Close())
	os.Stdout, os.Stderr = origStdout, origStderr

	outBytes, err := io.ReadAll(outReader)
	require.NoError(t, err)
	errBytes, err := io.ReadAll(errReader)
	require.NoError(t, err)

	return string(outBytes), string(errBytes)
}
