//go:build !linux && !windows && test

package privilege

import (
	"log/slog"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadSavedIDs_ReturnsSentinelErrorOnNonLinux verifies that the non-Linux
// implementation of readSavedIDs returns (0, 0, ErrSavedSetNotSupported). This
// code path is reachable on darwin (the development platform) and must not panic.
// The explicit error return ensures the saved-set invariant check is structurally
// skipped rather than relying on implicit equality of constant zero values.
func TestReadSavedIDs_ReturnsSentinelErrorOnNonLinux(t *testing.T) {
	suid, sgid, err := readSavedIDs()
	require.ErrorIs(t, err, ErrSavedSetNotSupported, "should return ErrSavedSetNotSupported on non-Linux")
	assert.Equal(t, 0, suid, "saved-set-uid is 0 on non-Linux")
	assert.Equal(t, 0, sgid, "saved-set-gid is 0 on non-Linux")
}

// TestRestorePrivilegesAndMetrics_SkipsSavedSetCheckOnNonLinux verifies that the
// saved-set invariant check in restorePrivilegesAndMetrics is structurally skipped
// when originalSUID/originalSGID carry the -1 sentinel that prepareExecution assigns
// on platforms returning ErrSavedSetNotSupported. Without the originalSUID >= 0 gate,
// the post-restore readSavedIDs() call would return ErrSavedSetNotSupported and trigger
// emergencyShutdown; this test asserts that path is never taken (osExit is not invoked).
func TestRestorePrivilegesAndMetrics_SkipsSavedSetCheckOnNonLinux(t *testing.T) {
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
		// Sentinel values assigned by prepareExecution when readSavedIDs returns
		// ErrSavedSetNotSupported; the saved-set check must be skipped structurally.
		originalSUID: -1,
		originalSGID: -1,
		start:        time.Now(),
	}

	// Should complete without invoking emergencyShutdown (osExit). If the skip-gate
	// were absent, the non-Linux readSavedIDs() would error and force a shutdown.
	manager.restorePrivilegesAndMetrics(execCtx, nil, "test", 0)
}
