//go:build test

package security

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
)

// TestAuditDirectoryPermissions_NoViolations verifies that clean directories
// produce no violations.
func TestAuditDirectoryPermissions_NoViolations(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	err := os.Chmod(tmpDir, 0o755)
	require.NoError(t, err)

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := AuditDirectoryPermissions(v, []string{tmpDir}, slog.Default())
	assert.Empty(t, result.Violations, "no violations expected for secure directory")
}

// TestAuditDirectoryPermissions_ViolationDetected verifies that world-writable
// directories are detected as violations.
func TestAuditDirectoryPermissions_ViolationDetected(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)

	// Make the directory world-writable (violates security policy)
	err := os.Chmod(tmpDir, 0o777)
	require.NoError(t, err)
	// Restore permissions after test so cleanup succeeds
	t.Cleanup(func() {
		_ = os.Chmod(tmpDir, 0o755)
	})

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := AuditDirectoryPermissions(v, []string{tmpDir}, slog.Default())
	require.Len(t, result.Violations, 1, "expected exactly one violation for world-writable directory")
	assert.Equal(t, filepath.Clean(tmpDir), result.Violations[0].Path)
	assert.True(t, errors.Is(result.Violations[0].Err, ErrInvalidDirPermissions), "violation error should be about directory permissions")
}

// TestAuditDirectoryPermissions_MultipleViolations verifies that multiple
// violations are all returned.
func TestAuditDirectoryPermissions_MultipleViolations(t *testing.T) {
	dir1 := tu.SafeTempDir(t)
	dir2 := tu.SafeTempDir(t)

	for _, d := range []string{dir1, dir2} {
		err := os.Chmod(d, 0o777)
		require.NoError(t, err)
		dd := d
		t.Cleanup(func() {
			_ = os.Chmod(dd, 0o755)
		})
	}

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := AuditDirectoryPermissions(v, []string{dir1, dir2}, slog.Default())
	assert.Len(t, result.Violations, 2, "expected two violations")
}

// TestAuditDirectoryPermissions_EmptyDirs verifies that an empty directory list
// produces no violations.
func TestAuditDirectoryPermissions_EmptyDirs(t *testing.T) {
	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := AuditDirectoryPermissions(v, []string{}, slog.Default())
	assert.Empty(t, result.Violations)
}

// TestAuditDirectoryPermissions_CountsCheckedAndSkipped pins the counting rule: a
// directory that violates the policy was inspected, so it counts as checked, while a
// directory that does not exist counts as skipped.
func TestAuditDirectoryPermissions_CountsCheckedAndSkipped(t *testing.T) {
	cleanDir := tu.SafeTempDir(t)
	require.NoError(t, os.Chmod(cleanDir, 0o755))

	violatingDir := tu.SafeTempDir(t)
	require.NoError(t, os.Chmod(violatingDir, 0o777))
	t.Cleanup(func() { _ = os.Chmod(violatingDir, 0o755) })

	missingDir := filepath.Join(cleanDir, "missing")

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := AuditDirectoryPermissions(v, []string{cleanDir, missingDir, violatingDir}, slog.Default())

	assert.Equal(t, 2, result.Checked)
	assert.Equal(t, 1, result.Skipped)
	assert.Len(t, result.Violations, 1)
}

// TestAuditDirectoryPermissions_MissingDirIsNotLoggedAsAnError pins the log level
// of the skipped case against the verdict it produces. A directory that does not
// exist is counted as skipped, so reporting it at ERROR would tell log-based
// alerting that a routine run — record before the hash directory is created,
// verify against a host that has none — needs attention. A directory that exists
// but cannot be stat'ed is the opposite case and keeps ERROR. The checker logs
// through the default logger, not the one handed to AuditDirectoryPermissions, so
// this test replaces the process-wide default and must not run in parallel.
func TestAuditDirectoryPermissions_MissingDirIsNotLoggedAsAnError(t *testing.T) {
	skipIfRoot(t)
	cleanDir := tu.SafeTempDir(t)
	missingDir := filepath.Join(cleanDir, "missing")
	unreadable := blockedPathUnder(t, cleanDir, "blocked")

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	logger, rec := tu.NewRecordingLogger()
	original := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(original) })

	result := AuditDirectoryPermissions(v, []string{missingDir}, logger)
	require.Equal(t, 1, result.Skipped)
	assert.Empty(t, rec.RecordsAtLevel(slog.LevelError), "a directory that is merely absent is not a fault")
	// Not a fault is not the same as not worth recording: the skip narrows what
	// the check established, so the path must stay traceable at debug level
	// rather than disappearing along with the ERROR line.
	skipped := rec.RequireRecord(t, slog.LevelDebug, "Failed to get directory info")
	assert.Equal(t, missingDir, skipped.Attrs["path"])

	logger, rec = tu.NewRecordingLogger()
	slog.SetDefault(logger)
	AuditDirectoryPermissions(v, []string{unreadable}, logger)
	assert.NotEmpty(t, rec.RecordsAtLevel(slog.LevelError), "a directory that cannot be inspected still is one")
}
