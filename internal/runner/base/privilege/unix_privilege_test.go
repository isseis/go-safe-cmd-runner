//go:build !windows && test

// Some tests in this file (TestEscalatePrivileges/seteuid_failure and
// TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown) call the real
// syscall.Seteuid and depend on the process's real/effective/saved UID being
// unchanged by any concurrently running test. They must not call t.Parallel(),
// and neither may any other test in this package that shares process-wide
// identity state.
//
// Those two tests skip when running as root (syscall.Getuid() == 0 ||
// syscall.Geteuid() == 0), because a root process can seteuid to any UID and
// the failure path they exercise cannot occur. GitHub Actions' ubuntu-latest
// runner and this project's dev container both run as a non-root user, so CI
// does run them; if that ever changes to a root container, this coverage
// disappears without any test failing to announce it.

package privilege

import (
	"errors"
	"log/slog"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrepareExecution_Success tests the successful preparation of execution context
func TestPrepareExecution_Success(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
	}

	tests := []struct {
		name                   string
		elevationCtx           runnertypes.ElevationContext
		expectedPrivEscalation bool
	}{
		{
			name: "user_group_execution",
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationUserGroupExecution,
				CommandName: "test-command",
				RunAsUser:   "testuser",
				RunAsGroup:  "testgroup",
			},
			expectedPrivEscalation: true,
		},
		{
			name: "file_validation",
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationFileValidation,
				CommandName: "test-command",
			},
			expectedPrivEscalation: true,
		},
		{
			name: "kill_after_cancel",
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationKillAfterCancel,
				CommandName: "test-command",
			},
			expectedPrivEscalation: true,
		},
		{
			name: "staging_cleanup",
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationStagingCleanup,
				CommandName: "test-command",
			},
			expectedPrivEscalation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execCtx, err := manager.prepareExecution(tt.elevationCtx)
			require.NoError(t, err)
			require.NotNil(t, execCtx)

			assert.Equal(t, tt.expectedPrivEscalation, execCtx.needsPrivilegeEscalation,
				"needsPrivilegeEscalation mismatch")
			assert.Equal(t, tt.elevationCtx, execCtx.elevationCtx)
		})
	}
}

// TestPrepareExecution_NotSupported tests unsupported operations
func TestPrepareExecution_NotSupported(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
	}

	elevationCtx := runnertypes.ElevationContext{
		Operation:   runnertypes.Operation("unsupported"),
		CommandName: "test-command",
	}

	execCtx, err := manager.prepareExecution(elevationCtx)
	assert.Error(t, err)
	assert.Nil(t, execCtx)
	assert.ErrorIs(t, err, ErrUnsupportedOperationType)
}

// TestWithPrivileges_UserGroupExecutionDoesNotChangeIdentity verifies that
// OperationUserGroupExecution does not change the parent process's identity;
// that is delegated to SysProcAttr.Credential in the executor.
//
// privilegeSupported: true is set so that WithPrivileges does not short-circuit
// at the IsPrivilegedExecutionSupported check. actual seteuid calls are avoided
// because originalUID: 0 triggers an early-return in escalatePrivileges, not via
// the privilegeSupported flag.
func TestWithPrivileges_UserGroupExecutionDoesNotChangeIdentity(t *testing.T) {
	logger := slog.Default()

	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
		originalUID:        0,
		osExit:             func(_ int) { t.Fatal("emergencyShutdown called unexpectedly") },
		identityVerifier:   func() error { return nil },
		readSavedIDs:       func() (int, int, error) { return -1, -1, ErrSavedSetNotSupported },
	}

	euidBefore := syscall.Geteuid()
	egidBefore := syscall.Getegid()

	elevationCtx := runnertypes.ElevationContext{
		Operation:  runnertypes.OperationUserGroupExecution,
		RunAsUser:  "testuser",
		RunAsGroup: "testgroup",
	}
	fnExecuted := false
	fn := func() error {
		fnExecuted = true
		return nil
	}

	err := manager.WithPrivileges(elevationCtx, fn)
	assert.NoError(t, err)
	assert.True(t, fnExecuted, "fn should have been executed")

	assert.Equal(t, euidBefore, syscall.Geteuid())
	assert.Equal(t, egidBefore, syscall.Getegid())
}

// TestPerformElevation_Failure tests privilege elevation failures
func TestPerformElevation_Failure(t *testing.T) {
	logger := slog.Default()

	t.Run("privilege_escalation_not_supported", func(t *testing.T) {
		// Manager with privilege support disabled
		managerNoPriv := &UnixPrivilegeManager{
			logger:             logger,
			privilegeSupported: false,
		}

		execCtx := &executionContext{
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationFileValidation,
				CommandName: "test-command",
			},
			needsPrivilegeEscalation: true,
		}

		err := managerNoPriv.performElevation(execCtx)
		assert.Error(t, err)
		assert.ErrorIs(t, err, runnertypes.ErrPrivilegedExecutionNotAvailable)
	})
}

// TestHandleCleanup_WithError tests cleanup with errors
func TestHandleCleanup_WithError(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: false,
	}

	// Test with simulated panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Panic should be re-raised after cleanup
				assert.Equal(t, "test panic", r)
			}
		}()

		// This will panic but handleCleanup should handle it
		defer manager.handleCleanup(execCtx)
		panic("test panic")
	}()
}

// TestIsPrivilegedExecutionSupported tests privileged execution support detection
func TestIsPrivilegedExecutionSupported(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
	}

	assert.True(t, manager.IsPrivilegedExecutionSupported())

	managerNoPriv := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
	}
	assert.False(t, managerNoPriv.IsPrivilegedExecutionSupported())
}

// TestEscalatePrivileges tests privilege escalation
func TestEscalatePrivileges(t *testing.T) {
	logger := slog.Default()

	t.Run("not_supported", func(t *testing.T) {
		manager := &UnixPrivilegeManager{
			logger:             logger,
			privilegeSupported: false,
		}

		elevationCtx := runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		}

		err := manager.escalatePrivileges(&executionContext{elevationCtx: elevationCtx})
		assert.Error(t, err)
		assert.ErrorIs(t, err, runnertypes.ErrPrivilegedExecutionNotAvailable)
	})

	t.Run("native_root", func(t *testing.T) {
		manager := &UnixPrivilegeManager{
			logger:             logger,
			originalUID:        0, // Simulate running as root
			privilegeSupported: true,
		}

		elevationCtx := runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		}

		execCtx := &executionContext{elevationCtx: elevationCtx}
		err := manager.escalatePrivileges(execCtx)
		// Should succeed without actual seteuid call
		assert.NoError(t, err)
		assert.Equal(t, elevationNativeRoot, execCtx.elevation,
			"what happened must be recorded for WithPrivileges to report after the window")
	})

	t.Run("seteuid_failure", func(t *testing.T) {
		if syscall.Getuid() == 0 || syscall.Geteuid() == 0 {
			t.Skip("running as root: seteuid(0) succeeds and the native-root path returns early, so the failure path cannot be exercised")
		}

		euidBefore := syscall.Geteuid()
		egidBefore := syscall.Getegid()

		manager := &UnixPrivilegeManager{
			logger:             logger,
			originalUID:        syscall.Getuid(),
			privilegeSupported: true,
		}

		elevationCtx := runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		}

		execCtx := &executionContext{elevationCtx: elevationCtx}
		err := manager.escalatePrivileges(execCtx)

		assert.Equal(t, elevationNone, execCtx.elevation,
			"an escalation that changed nothing must record nothing to report")

		privErr, ok := errors.AsType[*Error](err)
		require.True(t, ok, "expected a *Error, got %v", err)
		assert.Equal(t, runnertypes.OperationFileValidation, privErr.Operation)
		assert.Equal(t, "test-command", privErr.CommandName)
		assert.Equal(t, syscall.Getuid(), privErr.OriginalUID)
		assert.Equal(t, 0, privErr.TargetUID)
		_, ok = errors.AsType[syscall.Errno](privErr.SyscallErr)
		require.True(t, ok, "SyscallErr should be a syscall.Errno, got %v (%T)", privErr.SyscallErr, privErr.SyscallErr)

		assert.Equal(t, egidBefore, syscall.Getegid(), "effective GID must be unchanged after a failed seteuid")
		assert.Equal(t, euidBefore, syscall.Geteuid(), "effective UID must be unchanged after a failed seteuid")
	})
}

// TestEmergencyShutdown tests emergency shutdown handling
func TestEmergencyShutdown(t *testing.T) {
	logger := slog.Default()

	// Set up a test exit function to capture exit behavior
	var exitCode int
	var exited bool
	testOsExit := func(code int) {
		exitCode = code
		exited = true
		// Use panic to stop execution flow within the function under test.
		panic("os.Exit called")
	}

	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
		osExit:             testOsExit,
	}

	// We can now call emergencyShutdown and assert its behavior.
	assert.PanicsWithValue(t, "os.Exit called", func() {
		manager.emergencyShutdown(errors.New("test error"), "test_context")
	}, "emergencyShutdown should call os.Exit")

	// Verify that os.Exit was called with the correct code.
	assert.True(t, exited, "os.Exit should have been called")
	assert.Equal(t, 1, exitCode, "Expected exit code 1")
}

// TestDefaultIdentityVerifier tests that defaultIdentityVerifier passes in a normal
// test environment where EUID == UID and EGID == GID.
func TestDefaultIdentityVerifier(t *testing.T) {
	// In a regular test run (no setuid binary), effective and real IDs are equal.
	err := defaultIdentityVerifier()
	assert.NoError(t, err, "defaultIdentityVerifier should pass when EUID==UID and EGID==GID")
}

// TestRestorePrivilegesAndVerify_IdentityLeakTriggersShutdown verifies that when
// identityVerifier detects a mismatch after privilege restoration, emergencyShutdown
// (osExit) is called immediately.
func TestRestorePrivilegesAndVerify_IdentityLeakTriggersShutdown(t *testing.T) {
	var exitCode int
	exitCalled := false
	testOsExit := func(code int) {
		exitCode = code
		exitCalled = true
		panic("os.Exit called")
	}

	manager := &UnixPrivilegeManager{
		logger:             slog.Default(),
		privilegeSupported: false,
		osExit:             testOsExit,
		identityVerifier: func() error {
			return errors.New("effective UID 0 does not match real UID 1000 after privilege restoration")
		},
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: true,
	}

	assert.PanicsWithValue(t, "os.Exit called", func() {
		manager.restorePrivilegesAndVerify(execCtx, "test")
	}, "emergencyShutdown should be called when identity verification fails")

	assert.True(t, exitCalled, "os.Exit should have been called")
	assert.Equal(t, 1, exitCode, "exit code should be 1")
}

// TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown verifies that
// when restorePrivileges's syscall.Seteuid fails, restorePrivilegesAndVerify
// calls emergencyShutdown (osExit) instead of proceeding to the identity
// verification block below it.
func TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown(t *testing.T) {
	if syscall.Getuid() == 0 || syscall.Geteuid() == 0 {
		t.Skip("running as root: seteuid to an arbitrary UID succeeds, so the restoration failure path cannot be exercised")
	}

	euidBefore := syscall.Geteuid()
	egidBefore := syscall.Getegid()

	var exitCode int
	exitCalled := false
	testOsExit := func(code int) {
		exitCode = code
		exitCalled = true
		panic("os.Exit called")
	}

	manager := &UnixPrivilegeManager{
		logger: slog.Default(),
		// syscall.Getuid()+1 matches none of the real/effective/saved UIDs of a
		// non-setuid test binary (all three equal syscall.Getuid()), so Seteuid
		// below is guaranteed to fail.
		originalUID:        syscall.Getuid() + 1,
		privilegeSupported: true,
		osExit:             testOsExit,
		// identityVerifier and readSavedIDs are never called: restorePrivileges
		// fails first and emergencyShutdown short-circuits via panic before the
		// verification block below it runs. They are set only so the manager is
		// safe to use if that ordering ever changes.
		identityVerifier: func() error { return nil },
		readSavedIDs:     func() (int, int, error) { return -1, -1, ErrSavedSetNotSupported },
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: true,
		originalSUID:             -1,
		originalSGID:             -1,
	}

	assert.PanicsWithValue(t, "os.Exit called", func() {
		manager.restorePrivilegesAndVerify(execCtx, "test")
	}, "emergencyShutdown should be called when restorePrivileges fails")

	assert.True(t, exitCalled, "os.Exit should have been called")
	assert.Equal(t, 1, exitCode, "exit code should be 1")

	assert.Equal(t, egidBefore, syscall.Getegid(), "effective GID must be unchanged after a failed seteuid")
	assert.Equal(t, euidBefore, syscall.Geteuid(), "effective UID must be unchanged after a failed seteuid")
}

// TestRestorePrivilegesAndVerify_IdentityVerificationSkippedWithoutEscalation
// verifies that the identity check is NOT performed for operations that did
// not escalate (which never change UID/GID). Verification is gated on
// escalation, not on any specific operation type.
func TestRestorePrivilegesAndVerify_IdentityVerificationSkippedWithoutEscalation(t *testing.T) {
	verifierCalled := false

	manager := &UnixPrivilegeManager{
		logger:             slog.Default(),
		privilegeSupported: false,
		osExit:             func(_ int) { t.Fatal("emergencyShutdown called unexpectedly") },
		identityVerifier: func() error {
			verifierCalled = true
			return errors.New("should not be called")
		},
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: false,
	}

	manager.restorePrivilegesAndVerify(execCtx, "test")

	assert.False(t, verifierCalled, "identityVerifier should not be called when there was no escalation")
}

// TestRestorePrivilegesAndVerify_SavedSetUnchanged_Passes verifies the
// setuid-root scenario: the saved-set-uid/gid captured before the operation
// (suid=0, the setuid-root binary's saved set) is still 0 after restore, and
// this must be compared against the captured value -- not against the real
// UID -- so a legitimately root-owned saved-set does not trip the invariant.
func TestRestorePrivilegesAndVerify_SavedSetUnchanged_Passes(t *testing.T) {
	manager := &UnixPrivilegeManager{
		logger:             slog.Default(),
		privilegeSupported: true,
		osExit:             func(code int) { t.Fatalf("emergencyShutdown called unexpectedly with code %d", code) },
		identityVerifier:   func() error { return nil },
		readSavedIDs:       func() (suid, sgid int, err error) { return 0, 0, nil },
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: true,
		originalSUID:             0,
		originalSGID:             0,
	}

	manager.restorePrivilegesAndVerify(execCtx, "test")
	// No assertion beyond "did not panic/exit": osExit fails the test if called.
}

// TestRestorePrivilegesAndVerify_SavedSetChanged_TriggersShutdown verifies
// that when the saved-set-uid/gid read after restoration differs from the
// value captured at operation start, emergencyShutdown fires even though the
// EUID==UID/EGID==GID check (identityVerifier) alone reports success -- the
// saved-set check is a strictly stronger invariant than the real/effective
// check.
func TestRestorePrivilegesAndVerify_SavedSetChanged_TriggersShutdown(t *testing.T) {
	var exitCode int
	exitCalled := false
	testOsExit := func(code int) {
		exitCode = code
		exitCalled = true
		panic("os.Exit called")
	}

	manager := &UnixPrivilegeManager{
		logger:             slog.Default(),
		privilegeSupported: true,
		osExit:             testOsExit,
		identityVerifier:   func() error { return nil },
		readSavedIDs:       func() (suid, sgid int, err error) { return 1000, 1000, nil },
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: true,
		originalSUID:             0,
		originalSGID:             0,
	}

	assert.PanicsWithValue(t, "os.Exit called", func() {
		manager.restorePrivilegesAndVerify(execCtx, "test")
	}, "emergencyShutdown should be called when saved-set-uid/gid changed since capture")

	assert.True(t, exitCalled, "os.Exit should have been called")
	assert.Equal(t, 1, exitCode, "exit code should be 1")
}

// TestRestorePrivilegesAndVerify_SavedSetCheckSkipped_NonLinux verifies that
// the saved-set invariant check is structurally skipped (not merely
// coincidentally passing) on platforms where it cannot be read: a captured
// sentinel of -1 (what prepareExecution stores when readSavedIDs reports
// ErrSavedSetNotSupported) must prevent readSavedIDs from being consulted
// again during restore, even though the injected mock is preconfigured to
// report a "changed" value that would otherwise trigger emergencyShutdown.
func TestRestorePrivilegesAndVerify_SavedSetCheckSkipped_NonLinux(t *testing.T) {
	readSavedIDsCalled := false
	manager := &UnixPrivilegeManager{
		logger:             slog.Default(),
		privilegeSupported: true,
		osExit:             func(code int) { t.Fatalf("emergencyShutdown called unexpectedly with code %d", code) },
		identityVerifier:   func() error { return nil },
		readSavedIDs: func() (suid, sgid int, err error) {
			readSavedIDsCalled = true
			return 1000, 1000, nil
		},
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: true,
		originalSUID:             -1,
		originalSGID:             -1,
	}

	manager.restorePrivilegesAndVerify(execCtx, "test")

	assert.False(t, readSavedIDsCalled, "readSavedIDs must not be consulted during restore when the saved-set is not supported")
}

// TestWithPrivileges_ReentrantCallIsRejected verifies the reentrancy guard that
// replaced the manager's lock: a call made from within fn on the same manager
// returns ErrReentrantPrivilegeCall without running the inner fn, so the inner
// call never opens a privilege window and never restores the euid while the
// outer fn is still running.
//
// The manager is constructed the same way as
// TestWithPrivileges_UserGroupExecutionDoesNotChangeIdentity so that fn is
// reached as an unprivileged user: privilegeSupported: true clears the
// "not supported" error and originalUID: 0 makes escalatePrivileges take its
// native-root early return instead of calling syscall.Seteuid. This test must
// therefore never skip.
func TestWithPrivileges_ReentrantCallIsRejected(t *testing.T) {
	newManager := func(t *testing.T) *UnixPrivilegeManager {
		t.Helper()
		return &UnixPrivilegeManager{
			logger:             slog.Default(),
			privilegeSupported: true,
			originalUID:        0,
			osExit:             func(_ int) { t.Fatal("emergencyShutdown called unexpectedly") },
			identityVerifier:   func() error { return nil },
			readSavedIDs:       func() (int, int, error) { return -1, -1, ErrSavedSetNotSupported },
		}
	}

	elevationCtx := runnertypes.ElevationContext{
		Operation:   runnertypes.OperationUserGroupExecution,
		CommandName: "test-command",
	}

	t.Run("reentrant call is rejected and the inner fn never runs", func(t *testing.T) {
		manager := newManager(t)

		innerCalls := 0
		outerCompleted := false
		var innerErr error

		outerErr := manager.WithPrivileges(elevationCtx, func() error {
			innerErr = manager.WithPrivileges(elevationCtx, func() error {
				innerCalls++
				return nil
			})
			outerCompleted = true
			return nil
		})

		assert.ErrorIs(t, innerErr, ErrReentrantPrivilegeCall, "the reentrant call should be rejected")
		assert.Equal(t, 0, innerCalls, "the inner fn must not run")
		assert.True(t, outerCompleted, "the outer fn should run to completion")
		assert.NoError(t, outerErr, "the outer call reports its own fn's result, not the inner rejection")
	})

	// The guard is sticky fail-closed: if the reset defer ever stopped running,
	// every later privileged execution in the process would be rejected. The
	// two subtests below pin the reset on the paths that could strand it -- a
	// panic in fn, which must unwind past handleCleanup's own recover-and-
	// re-panic, and an early return before any window is opened.
	t.Run("the flag is cleared when fn panics", func(t *testing.T) {
		manager := newManager(t)

		assert.Panics(t, func() {
			_ = manager.WithPrivileges(elevationCtx, func() error {
				panic("boom")
			})
		}, "the panic should be re-raised to the caller")

		called := false
		require.NoError(t, manager.WithPrivileges(elevationCtx, func() error {
			called = true
			return nil
		}))
		assert.True(t, called, "a call after a panicking one must not be rejected as reentrant")
	})

	t.Run("the flag is cleared when prepareExecution rejects the operation", func(t *testing.T) {
		manager := newManager(t)

		err := manager.WithPrivileges(runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileAccess,
			CommandName: "test-command",
		}, func() error {
			t.Fatal("fn must not run for an unsupported operation")
			return nil
		})
		require.ErrorIs(t, err, ErrUnsupportedOperationType)

		called := false
		require.NoError(t, manager.WithPrivileges(elevationCtx, func() error {
			called = true
			return nil
		}))
		assert.True(t, called, "a call after a rejected operation must not be rejected as reentrant")
	})

	t.Run("consecutive non-reentrant calls both run fn", func(t *testing.T) {
		manager := newManager(t)

		calls := 0
		fn := func() error {
			calls++
			return nil
		}

		require.NoError(t, manager.WithPrivileges(elevationCtx, fn))
		require.NoError(t, manager.WithPrivileges(elevationCtx, fn))
		assert.Equal(t, 2, calls, "the guard must not fire on a call made after the previous one returned")
	})
}

// newLoggingOrderTestManager builds a native-root manager whose restore and
// identity checks are hermetic, so the tests below can observe which records
// are written and in what order without touching process-wide identity.
func newLoggingOrderTestManager(t *testing.T, logger *slog.Logger) *UnixPrivilegeManager {
	t.Helper()
	return &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
		originalUID:        0,
		identityVerifier:   func() error { return nil },
		osExit:             func(_ int) { t.Fatal("emergencyShutdown called unexpectedly") },
		readSavedIDs: func() (int, int, error) {
			return -1, -1, ErrSavedSetNotSupported
		},
	}
}

// TestWithPrivileges_WritesNoRecordWhileElevated verifies that the escalation
// is reported only once the window has closed.
//
// The reason is not tidiness: the slog handler is supplied by the caller --
// this project installs a redaction handler, a file handler and a Slack
// notification worker -- and a handler invoked while the window is open does
// whatever it does at euid 0, which is exactly the exposure narrowing the
// window is meant to remove. A record whose text says "Privileges elevated"
// is worth nothing if writing it is itself the privileged side effect.
//
// Only the "Entering privileged operation callback" record may exist by the
// time fn runs, and it is written before the escalation.
func TestWithPrivileges_WritesNoRecordWhileElevated(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()
	manager := newLoggingOrderTestManager(t, logger)

	var duringWindow []tu.RecordSnapshot
	err := manager.WithPrivileges(runnertypes.ElevationContext{
		Operation:   runnertypes.OperationFileValidation,
		CommandName: "test-command",
	}, func() error {
		duringWindow = rec.Records()
		return nil
	})
	require.NoError(t, err)

	require.Len(t, duringWindow, 1, "only the pre-escalation record may exist while the window is open")
	assert.Equal(t, slog.LevelDebug, duringWindow[0].Level)
	assert.Equal(t, "Entering privileged operation callback", duringWindow[0].Message)

	assert.Len(t, rec.FindRecords(slog.LevelInfo, "Native root execution - no privilege escalation needed"), 1,
		"the escalation must still be reported, after the window has closed")
}

// TestHandleCleanup_ReportsPanicAfterRestore verifies that a panic inside the
// window is reported only after privileges have been restored. The record is
// written by a caller-supplied handler, so writing it while the panic still
// has the process at euid 0 would run that handler elevated -- and a panic is
// precisely when the process is least able to bound what happens next.
//
// The order is asserted against the restore's own record, which
// restorePrivilegesAndVerify writes on the way out.
func TestHandleCleanup_ReportsPanicAfterRestore(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()
	manager := newLoggingOrderTestManager(t, logger)
	execCtx := &executionContext{
		elevationCtx:             runnertypes.ElevationContext{Operation: runnertypes.OperationFileValidation},
		needsPrivilegeEscalation: true,
		originalSUID:             -1,
		originalSGID:             -1,
	}

	panicValue := "boom"
	func() {
		defer func() {
			assert.Equal(t, panicValue, recover(), "handleCleanup must re-raise the panic it recovered")
		}()
		defer manager.handleCleanup(execCtx)
		panic(panicValue)
	}()

	// First occurrence of each, and the panic must be reported exactly once:
	// taking the last index would let a record written both before and after
	// the restore satisfy the ordering assertion below.
	records := rec.Records()
	restoreIdx := -1
	panicIdx := -1
	panicCount := 0
	for i, r := range records {
		switch r.Message {
		case "Native root execution - no privilege restoration needed":
			if restoreIdx == -1 {
				restoreIdx = i
			}
		case "Panic occurred during privileged operation, privileges restored":
			if panicIdx == -1 {
				panicIdx = i
			}
			panicCount++
		}
	}
	assert.Equal(t, 1, panicCount, "the panic must be reported exactly once")
	require.NotEqual(t, -1, restoreIdx, "the restore must have been attempted")
	require.NotEqual(t, -1, panicIdx, "the panic must still be reported")
	assert.Greater(t, panicIdx, restoreIdx, "the panic must be reported after privileges are restored, not before")
}
