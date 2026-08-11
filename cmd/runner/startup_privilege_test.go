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
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
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

// TestDropStartupPrivileges_SucceedsForCurrentIdentity pins the production
// call: dropping to the identity the process already has returns nil and
// leaves both effective IDs equal to the real ones.
//
// A test binary is never started setuid or setgid, so the effective IDs
// already equal the real ones before the call; these assertions therefore
// cannot distinguish a real drop from a no-op, and no assertion available to
// an unprivileged process can. What rules out a stub implementation is the
// pair of failure tests above: an unconditional `return nil` fails both. An
// unprivileged process also cannot setegid to a supplementary group, so there
// is no reachable target whose effect could be observed instead.
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
		for field := range strings.FieldsSeq(line) {
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

	// Drain both pipes while fn runs: a pipe holds only a fixed kernel buffer,
	// and a writer that fills it would block forever with nobody reading.
	var wg sync.WaitGroup
	var outBuf, errBuf bytes.Buffer
	for _, drain := range []struct {
		dst *bytes.Buffer
		src *os.File
	}{{&outBuf, outReader}, {&errBuf, errReader}} {
		wg.Go(func() {
			_, _ = io.Copy(drain.dst, drain.src)
		})
	}

	fn()

	// Restore before closing, so os.Stdout and os.Stderr never name a closed
	// pipe -- not even for the two statements below, and not if a Close error
	// aborts this function early.
	os.Stdout, os.Stderr = origStdout, origStderr

	// Close the write ends so the drain goroutines see EOF and finish.
	require.NoError(t, outWriter.Close())
	require.NoError(t, errWriter.Close())
	wg.Wait()

	return outBuf.String(), errBuf.String()
}
