package executor

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTempDirManager_Create_NormalMode tests directory creation in normal mode
func TestTempDirManager_Create_NormalMode(t *testing.T) {
	mgr := NewTempDirManager("test-group", false)
	defer func() {
		if mgr.Path() != "" {
			os.RemoveAll(mgr.Path())
		}
	}()

	path, err := mgr.Create()
	assert.NoError(t, err, "Create() failed")

	// Verify path is not empty
	assert.NotEmpty(t, path, "Create() returned empty path")

	// Verify path matches the manager's stored path
	assert.Equal(t, mgr.Path(), path, "Path() does not match created path")

	// Verify directory exists
	info, err := os.Stat(path)
	assert.NoError(t, err, "Stat() failed for created directory")
	assert.True(t, info.IsDir(), "Path is not a directory")

	// Verify permissions (0700)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o700), "Permissions = %o, want 0700", info.Mode().Perm())

	// Verify path contains group name
	assert.True(t, strings.Contains(filepath.Base(path), "test-group"), "Path does not contain group name: %s", path)
}

// TestTempDirManager_Create_WrapTexts pins the wording of the two creation
// wraps and that each keeps the *fs.PathError's path as a Path segment, so a
// group name inside the path is not exempted from redaction.
func TestTempDirManager_Create_WrapTexts(t *testing.T) {
	const tempPath = "/tmp/scr-group-abc123"

	restoreMkdir, restoreChmod := mkdirTemp, chmodDir
	t.Cleanup(func() { mkdirTemp, chmodDir = restoreMkdir, restoreChmod })

	pathSegments := func(t *testing.T, err error) []string {
		t.Helper()
		structured, ok := errors.AsType[errmsg.Structured](err)
		require.True(t, ok, "the wrap must be a structured error; got %T", err)
		var paths []string
		for _, segment := range structured.StructuredMessage().Segments() {
			if segment.Role == errmsg.RolePath {
				paths = append(paths, segment.Text)
			}
		}
		return paths
	}

	t.Run("mkdir failure", func(t *testing.T) {
		mkdirErr := &fs.PathError{Op: "mkdir", Path: tempPath, Err: syscall.ENOSPC}
		mkdirTemp = func(_, _ string) (string, error) { return "", mkdirErr }
		chmodDir = os.Chmod

		_, err := NewTempDirManager("group", false).Create()
		require.Error(t, err)
		assert.Equal(t, "failed to create temporary directory: "+mkdirErr.Error(), err.Error())
		assert.Equal(t, []string{tempPath}, pathSegments(t, err))
	})

	t.Run("chmod failure", func(t *testing.T) {
		chmodErr := &fs.PathError{Op: "chmod", Path: tempPath, Err: syscall.EPERM}
		mkdirTemp = func(_, _ string) (string, error) { return tempPath, nil }
		chmodDir = func(string, os.FileMode) error { return chmodErr }

		_, err := NewTempDirManager("group", false).Create()
		require.Error(t, err)
		assert.Equal(t, "failed to set permissions on temporary directory: "+chmodErr.Error(), err.Error())
		assert.Equal(t, []string{tempPath}, pathSegments(t, err))
	})
}

// TestTempDirManager_Create_DryRunMode tests directory creation in dry-run mode
func TestTempDirManager_Create_DryRunMode(t *testing.T) {
	mgr := NewTempDirManager("test-group", true)

	path, err := mgr.Create()
	assert.NoError(t, err, "Create() failed")

	// Verify path is not empty
	assert.NotEmpty(t, path, "Create() returned empty path")

	// Verify path contains "dryrun"
	assert.True(t, strings.Contains(path, "dryrun"), "Dry-run path does not contain 'dryrun': %s", path)

	// Verify directory does NOT exist (dry-run)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "Directory should not exist in dry-run mode: %s", path)

	// Verify path matches the manager's stored path
	assert.Equal(t, mgr.Path(), path, "Path() does not match created path")
}

// TestTempDirManager_Cleanup_Success tests successful cleanup
func TestTempDirManager_Cleanup_Success(t *testing.T) {
	mgr := NewTempDirManager("test-group", false)

	path, err := mgr.Create()
	assert.NoError(t, err, "Create() failed")
	assert.NotEmpty(t, path, "Create() returned empty path")

	// Verify directory exists before cleanup
	_, err = os.Stat(path)
	assert.NoError(t, err, "Directory does not exist before cleanup")

	// Cleanup
	err = mgr.Cleanup()
	assert.NoError(t, err, "Cleanup() failed")

	// Verify directory does not exist after cleanup
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "Directory should not exist after cleanup: %s", path)

	// Path should be cleared after cleanup
	assert.Empty(t, mgr.Path(), "Path is not cleared after cleanup")
}

// TestTempDirManager_Cleanup_DryRun tests cleanup in dry-run mode
func TestTempDirManager_Cleanup_DryRun(t *testing.T) {
	mgr := NewTempDirManager("test-group", true)

	path, err := mgr.Create()
	assert.NoError(t, err, "Create() in dry-run mode failed")
	assert.NotEmpty(t, path, "Create() returned empty path")

	// Cleanup should succeed without error
	err = mgr.Cleanup()
	assert.NoError(t, err, "Cleanup() in dry-run mode should not fail")

	// Path should be cleared after cleanup
	assert.Empty(t, mgr.Path(), "Path is not cleared after cleanup")
}

// TestTempDirManager_Cleanup_NoCreate tests cleanup without create
func TestTempDirManager_Cleanup_NoCreate(t *testing.T) {
	mgr := NewTempDirManager("test-group", false)

	// Cleanup without creating should not fail
	err := mgr.Cleanup()
	assert.NoError(t, err, "Cleanup() without Create() should not fail")
}

// TestTempDirManager_Path_BeforeCreate tests Path() before Create()
func TestTempDirManager_Path_BeforeCreate(t *testing.T) {
	mgr := NewTempDirManager("test-group", false)

	path := mgr.Path()
	assert.Empty(t, path, "Path() before Create() should return empty string")
}

// TestTempDirManager_MultipleCreates tests that multiple creates fail or update path
func TestTempDirManager_MultipleCreates(t *testing.T) {
	mgr := NewTempDirManager("test-group", false)
	defer func() {
		if mgr.Path() != "" {
			os.RemoveAll(mgr.Path())
		}
	}()

	path1, err := mgr.Create()
	assert.NoError(t, err, "First Create() failed")
	assert.NotEqual(t, "", path1, "First Create() returned empty path")

	// Second create should fail
	path2, err := mgr.Create()
	assert.Error(t, err, "Second Create() should fail but succeeded")
	assert.Equal(t, "", path2, "Second Create() should return empty path on failure")

	// Manager should track the first path
	assert.Equal(t, mgr.Path(), path1, "Path() should return the first created path")
}
