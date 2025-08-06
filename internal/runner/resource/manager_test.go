package resource

import (
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

func TestNewManager(t *testing.T) {
	manager := NewManager("")
	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
	if manager.tempDirs == nil {
		t.Error("Manager tempDirs map is nil")
	}
	if manager.baseDir == "" {
		t.Error("Manager baseDir should be set to temp dir when empty")
	}

	// Test with custom base directory
	customDir := "/tmp/test"
	manager2 := NewManager(customDir)
	if manager2.baseDir != customDir {
		t.Errorf("Manager baseDir = %v, want %v", manager2.baseDir, customDir)
	}
}

func TestNewManagerWithFS(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	if manager == nil {
		t.Fatal("NewManagerWithFS() returned nil")
	}
	if manager.fs != mockFS {
		t.Error("Manager filesystem should be set to provided filesystem")
	}
}

func TestCreateTempDir(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	path, err := manager.CreateTempDir("test-command")
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	if path == "" {
		t.Fatal("CreateTempDir() returned empty path")
	}

	// Check that directory was actually created in mock filesystem
	exists, err := mockFS.FileExists(path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Errorf("Temporary directory was not created: %s", path)
	}

	// Check that path is under the base directory
	if !strings.HasPrefix(path, "/tmp") {
		t.Errorf("Resource path %s is not under base directory %s", path, "/tmp")
	}

	// Test IsTempDirManaged
	if !manager.IsTempDirManaged(path) {
		t.Errorf("Path %s should be managed by manager", path)
	}
}

func TestCleanupTempDir(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Create a temp directory
	path, err := manager.CreateTempDir("test-command")
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Verify directory exists
	exists, err := mockFS.FileExists(path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Fatalf("Temporary directory was not created: %s", path)
	}

	// Clean up the temp directory
	err = manager.CleanupTempDir(path)
	if err != nil {
		t.Errorf("CleanupTempDir() failed: %v", err)
	}

	// Verify directory was removed
	exists, err = mockFS.FileExists(path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("Temp directory should have been removed: %s", path)
	}

	// Verify it's no longer managed
	if manager.IsTempDirManaged(path) {
		t.Error("Path should no longer be managed after cleanup")
	}

	// Try to cleanup non-existent temp directory
	err = manager.CleanupTempDir("/non/existent/path")
	if err == nil {
		t.Error("CleanupTempDir() should return error for non-existent resource")
	}
}

func TestCleanupAll(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Create multiple temp directories
	path1, err := manager.CreateTempDir("test-command-1")
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	path2, err := manager.CreateTempDir("test-command-2")
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Verify directories exist
	exists, err := mockFS.FileExists(path1)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Errorf("TempDir1 directory should exist: %s", path1)
	}

	exists, err = mockFS.FileExists(path2)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Errorf("TempDir2 directory should exist: %s", path2)
	}

	// Clean up all temp directories
	err = manager.CleanupAll()
	if err != nil {
		t.Errorf("CleanupAll() failed: %v", err)
	}

	// Verify directories were removed
	exists, err = mockFS.FileExists(path1)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("TempDir1 directory should have been removed: %s", path1)
	}

	exists, err = mockFS.FileExists(path2)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("TempDir2 directory should have been removed: %s", path2)
	}

	// Verify both directories are no longer managed
	if manager.IsTempDirManaged(path1) {
		t.Error("Path1 should no longer be managed after CleanupAll")
	}

	if manager.IsTempDirManaged(path2) {
		t.Error("Path2 should no longer be managed after CleanupAll")
	}
}
