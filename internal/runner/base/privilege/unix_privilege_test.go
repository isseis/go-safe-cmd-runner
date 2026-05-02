//go:build !windows && test

package privilege

import (
	"errors"
	"log/slog"
	"syscall"
	"testing"
	"time"

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
		name                    string
		elevationCtx            runnertypes.ElevationContext
		expectedPrivEscalation  bool
		expectedUserGroupChange bool
	}{
		{
			name: "user_group_execution",
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationUserGroupExecution,
				CommandName: "test-command",
				RunAsUser:   "testuser",
				RunAsGroup:  "testgroup",
			},
			expectedPrivEscalation:  true,
			expectedUserGroupChange: true,
		},
		{
			name: "user_group_dryrun",
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationUserGroupDryRun,
				CommandName: "test-command",
				RunAsUser:   "testuser",
				RunAsGroup:  "testgroup",
			},
			expectedPrivEscalation:  false,
			expectedUserGroupChange: true,
		},
		{
			name: "file_validation",
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationFileValidation,
				CommandName: "test-command",
			},
			expectedPrivEscalation:  true,
			expectedUserGroupChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execCtx, err := manager.prepareExecution(tt.elevationCtx)
			require.NoError(t, err)
			require.NotNil(t, execCtx)

			assert.Equal(t, tt.expectedPrivEscalation, execCtx.needsPrivilegeEscalation,
				"needsPrivilegeEscalation mismatch")
			assert.Equal(t, tt.expectedUserGroupChange, execCtx.needsUserGroupChange,
				"needsUserGroupChange mismatch")
			assert.Equal(t, tt.elevationCtx, execCtx.elevationCtx)
			assert.NotZero(t, execCtx.start)
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

// TestPerformElevation_Success tests successful privilege elevation
func TestPerformElevation_Success(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false, // Set to false to skip actual syscalls
	}

	t.Run("no_privilege_escalation_needed", func(t *testing.T) {
		execCtx := &executionContext{
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationUserGroupDryRun,
				CommandName: "test-command",
				RunAsUser:   "", // Empty user should succeed in dry-run
			},
			needsPrivilegeEscalation: false,
			needsUserGroupChange:     true,
		}

		// This should succeed as it only does dry-run validation with empty user
		err := manager.performElevation(execCtx)
		assert.NoError(t, err)
	})

	t.Run("only_dryrun_validation", func(t *testing.T) {
		execCtx := &executionContext{
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationUserGroupDryRun,
				CommandName: "test-command",
				RunAsUser:   "",
				RunAsGroup:  "",
			},
			needsPrivilegeEscalation: false,
			needsUserGroupChange:     true,
		}

		err := manager.performElevation(execCtx)
		assert.NoError(t, err)
	})
}

// TestPerformElevation_Failure tests privilege elevation failures
func TestPerformElevation_Failure(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
	}

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
			needsUserGroupChange:     false,
		}

		err := managerNoPriv.performElevation(execCtx)
		assert.Error(t, err)
		assert.ErrorIs(t, err, runnertypes.ErrPrivilegedExecutionNotAvailable)
	})

	t.Run("invalid_user_in_dryrun", func(t *testing.T) {
		execCtx := &executionContext{
			elevationCtx: runnertypes.ElevationContext{
				Operation:   runnertypes.OperationUserGroupDryRun,
				CommandName: "test-command",
				RunAsUser:   "nonexistent_user_xyz123",
			},
			needsPrivilegeEscalation: false,
			needsUserGroupChange:     true,
		}

		err := manager.performElevation(execCtx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user/group change failed")
	})
}

// TestHandleCleanupAndMetrics_Success tests successful cleanup
func TestHandleCleanupAndMetrics_Success(_ *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationUserGroupDryRun,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: false,
		needsUserGroupChange:     false,
		start:                    time.Now(),
	}

	// This should not panic
	manager.handleCleanupAndMetrics(execCtx)

	// No metrics assertion needed since needsUserGroupChange is false
	// (metrics are only recorded when user/group changes are needed)
}

// TestHandleCleanupAndMetrics_WithError tests cleanup with errors
func TestHandleCleanupAndMetrics_WithError(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationUserGroupDryRun,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: false,
		needsUserGroupChange:     false,
		start:                    time.Now(),
	}

	// Test with simulated panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Panic should be re-raised after cleanup
				assert.Equal(t, "test panic", r)
			}
		}()

		// This will panic but handleCleanupAndMetrics should handle it
		defer manager.handleCleanupAndMetrics(execCtx)
		panic("test panic")
	}()
}

// TestRestorePrivilegesAndMetrics_Success tests successful privilege restoration
func TestRestorePrivilegesAndMetrics_Success(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationUserGroupDryRun,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: false,
		needsUserGroupChange:     true, // This will trigger success recording
	}

	// Test successful restoration
	duration := 10 * time.Millisecond
	manager.restorePrivilegesAndMetrics(execCtx, nil, "normal execution", duration)

	snapshot := manager.GetMetrics()
	// When needsUserGroupChange is true, success should be recorded
	assert.Equal(t, int64(1), snapshot.ElevationSuccesses)
}

// TestRestorePrivilegesAndMetrics_Failure tests privilege restoration failures
func TestRestorePrivilegesAndMetrics_Failure(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationUserGroupDryRun,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: false,
		needsUserGroupChange:     false,
	}

	// Test with panic value (simulating error during execution)
	testErr := errors.New("test error")
	duration := 5 * time.Millisecond
	manager.restorePrivilegesAndMetrics(execCtx, testErr, "after panic", duration)

	// Metrics should not record success when there's a panic
	snapshot := manager.GetMetrics()
	// When panicValue is not nil, success should not be recorded
	assert.Equal(t, int64(0), snapshot.ElevationSuccesses)
}

// TestRestoreUserGroupInternal tests user/group restoration
func TestRestoreUserGroupInternal(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
	}

	// Test restoration with original EGID
	// This is a unit test, so we're testing the logic flow, not actual syscalls
	// Use the current EGID (not UID) because restoreUserGroupInternal calls Setegid
	err := manager.restoreUserGroupInternal(syscall.Getegid())

	// In test environment without actual privilege escalation, this should succeed
	// as it's a no-op when not actually escalated
	assert.NoError(t, err)
}

// TestWithUserGroup tests the WithUserGroup functionality
func TestWithUserGroup(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
	}

	t.Run("with_empty_user_group", func(t *testing.T) {
		fn := func() error {
			return nil
		}

		// WithUserGroup doesn't take isDryRun parameter - it always tries to execute
		// With privilegeSupported=false, it should fail
		err := manager.WithUserGroup("", "", fn)
		// Empty user/group with privilege not supported should fail
		assert.Error(t, err)
	})

	t.Run("invalid_user", func(t *testing.T) {
		fn := func() error {
			return nil
		}

		err := manager.WithUserGroup("nonexistent_user_xyz123", "", fn)
		assert.Error(t, err)
	})
}

// TestIsUserGroupSupported tests user/group support detection
func TestIsUserGroupSupported(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
	}

	// On Unix systems, user/group should always be supported
	assert.True(t, manager.IsUserGroupSupported())

	// Test with privilege not supported
	managerNoPriv := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
	}
	// User/group support depends on privilege support
	assert.False(t, managerNoPriv.IsUserGroupSupported())
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

// TestChangeUserGroupInternal_SeteuidFailure_EgidRollbackSuccess tests that when Seteuid
// fails, Setegid is called with originalEGID to roll back (AC-M1-4).
func TestChangeUserGroupInternal_SeteuidFailure_EgidRollbackSuccess(t *testing.T) {
	logger := slog.Default()

	const originalEGID = 1234
	var setegidCalledWith []int
	seteuidErr := errors.New("seteuid failed")

	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
		osExit:             func(_ int) { t.Fatal("emergencyShutdown called unexpectedly") },
		syscallSeteuid:     func(_ int) error { return seteuidErr },
		syscallSetegid: func(gid int) error {
			setegidCalledWith = append(setegidCalledWith, gid)
			return nil
		},
	}

	err := manager.changeUserGroupInternal("", "", false, originalEGID)

	// Seteuid failure should be propagated
	assert.ErrorContains(t, err, "failed to set effective user ID")

	// Setegid must have been called twice:
	//   1st call: set targetGID (0 when no user/group specified → Getegid at call time)
	//   2nd call: rollback to originalEGID
	require.Len(t, setegidCalledWith, 2, "Setegid should be called twice")
	assert.Equal(t, originalEGID, setegidCalledWith[1], "second Setegid call should use originalEGID for rollback")
}

// TestChangeUserGroupInternal_SeteuidFailure_EgidRollbackFailure tests that when both
// Seteuid and the rollback Setegid fail, emergencyShutdown (osExit) is called (AC-M1-5).
func TestChangeUserGroupInternal_SeteuidFailure_EgidRollbackFailure(t *testing.T) {
	logger := slog.Default()

	const originalEGID = 5678
	seteuidErr := errors.New("seteuid failed")
	setegidErr := errors.New("setegid rollback failed")

	var exitCode int
	var exited bool
	testOsExit := func(code int) {
		exitCode = code
		exited = true
		panic("os.Exit called")
	}

	// syscallSetegid: succeed on first call (set targetGID), fail on second call (rollback).
	setegidCallCount := 0
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false,
		osExit:             testOsExit,
		syscallSeteuid:     func(_ int) error { return seteuidErr },
		syscallSetegid: func(_ int) error {
			setegidCallCount++
			if setegidCallCount == 1 {
				return nil // first call (targetGID) succeeds
			}
			return setegidErr // second call (rollback) fails → triggers emergencyShutdown
		},
	}

	assert.PanicsWithValue(t, "os.Exit called", func() {
		_ = manager.changeUserGroupInternal("", "", false, originalEGID)
	}, "emergencyShutdown should be called when rollback Setegid also fails")

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

// TestRestorePrivilegesAndMetrics_IdentityLeakTriggersShutdown verifies that when
// identityVerifier detects a mismatch after privilege restoration, emergencyShutdown
// (osExit) is called immediately.
func TestRestorePrivilegesAndMetrics_IdentityLeakTriggersShutdown(t *testing.T) {
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
		needsUserGroupChange:     false,
		originalEUID:             syscall.Geteuid(),
		originalEGID:             syscall.Getegid(),
		start:                    time.Now(),
	}

	assert.PanicsWithValue(t, "os.Exit called", func() {
		manager.restorePrivilegesAndMetrics(execCtx, nil, "test", 0)
	}, "emergencyShutdown should be called when identity verification fails")

	assert.True(t, exitCalled, "os.Exit should have been called")
	assert.Equal(t, 1, exitCode, "exit code should be 1")
}

// TestRestorePrivilegesAndMetrics_IdentityVerificationSkippedForDryRun verifies that
// the identity check is NOT performed for dry-run operations (which never change UID/GID).
func TestRestorePrivilegesAndMetrics_IdentityVerificationSkippedForDryRun(t *testing.T) {
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
			Operation:   runnertypes.OperationUserGroupDryRun,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: false,
		needsUserGroupChange:     true,
		start:                    time.Now(),
	}

	manager.restorePrivilegesAndMetrics(execCtx, nil, "test", 0)

	assert.False(t, verifierCalled, "identityVerifier should not be called for dry-run")
}

// TestRestorePrivilegesAndMetrics_IdentityVerificationPassesOnCleanRestore verifies that
// no shutdown occurs when identityVerifier confirms the identity is clean.
func TestRestorePrivilegesAndMetrics_IdentityVerificationPassesOnCleanRestore(t *testing.T) {
	manager := &UnixPrivilegeManager{
		logger:             slog.Default(),
		privilegeSupported: false,
		osExit:             func(_ int) { t.Fatal("emergencyShutdown called unexpectedly") },
		identityVerifier:   func() error { return nil },
	}

	execCtx := &executionContext{
		elevationCtx: runnertypes.ElevationContext{
			Operation:   runnertypes.OperationFileValidation,
			CommandName: "test-command",
		},
		needsPrivilegeEscalation: true,
		needsUserGroupChange:     false,
		start:                    time.Now(),
	}

	// Should complete without panic or osExit
	manager.restorePrivilegesAndMetrics(execCtx, nil, "test", 0)
}
