package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Verify path is not empty
	if path == "" {
		t.Error("Create() returned empty path")
	}

	// Verify path matches the manager's stored path
	if mgr.Path() != path {
		t.Errorf("Path() = %s, want %s", mgr.Path(), path)
	}

	// Verify directory exists
	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("Directory does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("Path is not a directory")
	}

	// Verify permissions (0700)
	if info.Mode().Perm() != 0o700 {
		t.Errorf("Permissions = %o, want 0700", info.Mode().Perm())
	}

	// Verify path contains group name
	if !strings.Contains(filepath.Base(path), "test-group") {
		t.Errorf("Path does not contain group name: %s", path)
	}
}

// TestTempDirManager_Create_DryRunMode tests directory creation in dry-run mode
func TestTempDirManager_Create_DryRunMode(t *testing.T) {
	mgr := NewTempDirManager("test-group", true)

	path, err := mgr.Create()
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Verify path is not empty
	if path == "" {
		t.Error("Create() returned empty path")
	}

	// Verify path contains "dryrun"
	if !strings.Contains(path, "dryrun") {
		t.Errorf("Dry-run path does not contain 'dryrun': %s", path)
	}

	// Verify directory does NOT exist (dry-run)
	_, err = os.Stat(path)
	if err == nil {
		t.Errorf("Directory should not exist in dry-run mode: %s", path)
	}

	// Verify path matches the manager's stored path
	if mgr.Path() != path {
		t.Errorf("Path() = %s, want %s", mgr.Path(), path)
	}
}

// TestTempDirManager_Cleanup_Success tests successful cleanup
func TestTempDirManager_Cleanup_Success(t *testing.T) {
	mgr := NewTempDirManager("test-group", false)

	path, err := mgr.Create()
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Verify directory exists before cleanup
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Directory does not exist before cleanup: %v", err)
	}

	// Cleanup
	err = mgr.Cleanup()
	if err != nil {
		t.Errorf("Cleanup() failed: %v", err)
	}

	// Verify directory does not exist after cleanup
	_, err = os.Stat(path)
	if err == nil {
		t.Errorf("Directory still exists after cleanup: %s", path)
	}
}

// TestTempDirManager_Cleanup_DryRun tests cleanup in dry-run mode
func TestTempDirManager_Cleanup_DryRun(t *testing.T) {
	mgr := NewTempDirManager("test-group", true)

	path, err := mgr.Create()
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Cleanup should succeed without error
	err = mgr.Cleanup()
	if err != nil {
		t.Errorf("Cleanup() in dry-run mode should not fail: %v", err)
	}

	// Path should still be accessible
	if mgr.Path() != path {
		t.Errorf("Path changed after cleanup: got %s, want %s", mgr.Path(), path)
	}
}

// TestTempDirManager_Cleanup_NoCreate tests cleanup without create
func TestTempDirManager_Cleanup_NoCreate(t *testing.T) {
	mgr := NewTempDirManager("test-group", false)

	// Cleanup without creating should not fail
	err := mgr.Cleanup()
	if err != nil {
		t.Errorf("Cleanup() without Create() should not fail: %v", err)
	}
}

// TestTempDirManager_Path_BeforeCreate tests Path() before Create()
func TestTempDirManager_Path_BeforeCreate(t *testing.T) {
	mgr := NewTempDirManager("test-group", false)

	path := mgr.Path()
	if path != "" {
		t.Errorf("Path() before Create() should return empty string, got: %s", path)
	}
}

// TestTempDirManager_MultipleCreates tests that multiple creates fail or update path
func TestTempDirManager_MultipleCreates(t *testing.T) {
	mgr := NewTempDirManager("test-group", false).(*DefaultTempDirManager)
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
