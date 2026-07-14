//go:build linux && test

package privilege

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readSavedIDsFromProcStatus reads the saved-set-uid/gid from /proc/self/status
// for cross-checking against readSavedIDs(). The Uid: and Gid: lines are laid out
// as "Uid:\t<real>\t<effective>\t<saved>\t<filesystem>", so strings.Fields yields
// five tokens where index 0 is the "Uid:"/"Gid:" label and the saved-set is the
// fourth value (index 3).
func readSavedIDsFromProcStatus() (suid, sgid int, err error) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Uid:"):
			fields := strings.Fields(line)
			if len(fields) < 5 {
				return 0, 0, fmt.Errorf("unexpected Uid line format: %s", line)
			}
			// fields[0]="Uid:", fields[1]=real, fields[2]=effective, fields[3]=saved, fields[4]=filesystem
			n, err := strconv.Atoi(fields[3])
			if err != nil {
				return 0, 0, fmt.Errorf("parse Uid saved (field 3): %w", err)
			}
			suid = n
		case strings.HasPrefix(line, "Gid:"):
			fields := strings.Fields(line)
			if len(fields) < 5 {
				return 0, 0, fmt.Errorf("unexpected Gid line format: %s", line)
			}
			// fields[0]="Gid:", fields[1]=real, fields[2]=effective, fields[3]=saved, fields[4]=filesystem
			n, err := strconv.Atoi(fields[3])
			if err != nil {
				return 0, 0, fmt.Errorf("parse Gid saved (field 3): %w", err)
			}
			sgid = n
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return suid, sgid, nil
}

// TestReadSavedIDs_MatchesProcStatus verifies that readSavedIDs() returns the
// same saved-set-uid/gid as /proc/self/status. This is a Linux-specific test
// that exercises the production code path.
func TestReadSavedIDs_MatchesProcStatus(t *testing.T) {
	suid, sgid, err := readSavedIDs()
	require.NoError(t, err, "should read saved-set IDs")

	procSuid, procSgid, err := readSavedIDsFromProcStatus()
	require.NoError(t, err, "should read /proc/self/status")

	assert.Equal(t, procSuid, suid, "saved-set-uid matches /proc/self/status")
	assert.Equal(t, procSgid, sgid, "saved-set-gid matches /proc/self/status")
}

// TestRestorePrivilegesAndMetrics_IdentityVerificationPassesOnCleanRestore_WithGroundTruth
// verifies that the saved-set invariant check in restorePrivilegesAndMetrics passes
// against an independently-obtained ground truth (/proc/self/status), not against
// readSavedIDs()'s own output.
//
// This avoids the tautological trap of the previous test design, which captured
// the value via readSavedIDs() and then compared against readSavedIDs() again
// in restorePrivilegesAndMetrics — meaning the test could never fail regardless
// of whether readSavedIDs() is correct.
func TestRestorePrivilegesAndMetrics_IdentityVerificationPassesOnCleanRestore_WithGroundTruth(t *testing.T) {
	// Obtain ground-truth saved-set IDs from /proc/self/status (independent source).
	procSuid, procSgid, err := readSavedIDsFromProcStatus()
	require.NoError(t, err, "should read /proc/self/status for ground truth")

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
		// Use ground-truth values from /proc/self/status, NOT from readSavedIDs().
		originalSUID: procSuid,
		originalSGID: procSgid,
		start:        time.Now(),
	}

	// Should complete without panic or osExit. If readSavedIDs() returns values
	// that differ from the ground truth, the internal comparison will trigger
	// emergencyShutdown and the test will fail.
	manager.restorePrivilegesAndMetrics(execCtx, nil, "test", 0)
}
