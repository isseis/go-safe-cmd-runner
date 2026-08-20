//go:build !windows && test

package privilege

import (
	"errors"
	"log/slog"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
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

		err := manager.escalatePrivileges(elevationCtx)
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

		err := manager.escalatePrivileges(elevationCtx)
		// Should succeed without actual seteuid call
		assert.NoError(t, err)
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
