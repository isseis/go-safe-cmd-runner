package tempdir

import (
	"fmt"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestNewTempDirManager(t *testing.T) {
	manager := NewTempDirManager("")
	assert.NotNil(t, manager, "NewManager should not return nil")
	assert.NotNil(t, manager.tempDirs, "Manager tempDirs map should not be nil")
	assert.NotEmpty(t, manager.baseDir, "Manager baseDir should not be empty")

	// Test with custom base directory
	customDir := "/tmp/test"
	manager2 := NewTempDirManager(customDir)
	assert.Equal(t, customDir, manager2.baseDir, "Manager baseDir should match custom directory")
}

func TestNewTempDirManagerWithFS(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewTempDirManagerWithFS("/tmp", mockFS)

	assert.NotNil(t, manager, "NewManagerWithFS should not return nil")
	assert.Equal(t, mockFS, manager.fs, "Manager filesystem should be set to provided filesystem")
}

func TestCreateTempDir(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewTempDirManagerWithFS("/tmp", mockFS)

	path, err := manager.CreateTempDir("test-command")
	assert.Nil(t, err, "CreateTempDir should not return an error")
	assert.NotEmpty(t, path, "CreateTempDir should return a valid path")

	// Check that directory was actually created in mock filesystem
	exists, err := mockFS.FileExists(path)
	assert.Nil(t, err, "FileExists should not return an error")
	assert.True(t, exists, "Temporary directory should exist after creation")

	// Check that path is under the base directory
	assert.True(t, strings.HasPrefix(path, "/tmp"), "Resource path %s is not under base directory %s", path, "/tmp")
}

func TestCleanupTempDir(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewTempDirManagerWithFS("/tmp", mockFS)

	// Create a temp directory
	path, err := manager.CreateTempDir("test-command")
	assert.Nil(t, err, "CreateTempDir should not return an error")

	// Verify directory exists
	exists, err := mockFS.FileExists(path)
	assert.Nil(t, err, "FileExists should not return an error")
	assert.True(t, exists, "Temporary directory should exist after creation")

	// Clean up the temp directory
	err = manager.CleanupTempDir(path)
	assert.Nil(t, err, "CleanupTempDir() failed")

	// Verify directory was removed
	exists, err = mockFS.FileExists(path)
	assert.Nil(t, err, "FileExists should not return an error")
	assert.False(t, exists, "Temp directory should have been removed: %s", path)

	// Try to cleanup non-existent temp directory
	err = manager.CleanupTempDir("/non/existent/path")
	assert.Error(t, err, "CleanupTempDir() should return error for non-existent resource")
}

var errSimulatedFailure = fmt.Errorf("simulated failure")

// failingFS is a mock file system that always fails RemoveAll
// Used for testing error handling in CleanupTempDir
type failingFS struct{ *common.MockFileSystem }

func (fs *failingFS) RemoveAll(_ string) error { return errSimulatedFailure }

func TestCleanupTempDir_ErrorCases(t *testing.T) {
	t.Run("non-managed path", func(t *testing.T) {
		// Test error when path is not managed
		mockFS := common.NewMockFileSystem()
		manager := NewTempDirManagerWithFS("/tmp", mockFS)

		err := manager.CleanupTempDir("/not/managed/path")
		assert.ErrorIs(t, err, ErrTempDirNotFound)
	})

	t.Run("error when RemoveAll fails", func(t *testing.T) {
		// Test error when RemoveAll fails
		failingFS := &failingFS{MockFileSystem: common.NewMockFileSystem()}
		manager := NewTempDirManagerWithFS("/tmp", failingFS)
		path, err := manager.CreateTempDir("fail-case")
		assert.Nil(t, err, "CreateTempDir should succeed for setup")

		err = manager.CleanupTempDir(path)
		assert.ErrorIs(t, err, ErrCleanupFailed)
	})
}

func TestCleanupAll(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewTempDirManagerWithFS("/tmp", mockFS)

	// Create multiple temp directories
	path1, err := manager.CreateTempDir("test-command-1")
	assert.Nil(t, err, "CreateTempDir should not return an error")

	path2, err := manager.CreateTempDir("test-command-2")
	assert.Nil(t, err, "CreateTempDir should not return an error")

	// Verify directories exist
	exists, err := mockFS.FileExists(path1)
	assert.Nil(t, err, "FileExists should not return an error")
	assert.True(t, exists, "TempDir1 directory should exist: %s", path1)

	exists, err = mockFS.FileExists(path2)
	assert.Nil(t, err, "FileExists should not return an error")
	assert.True(t, exists, "TempDir2 directory should exist: %s", path2)

	// Clean up all temp directories
	err = manager.CleanupAll()
	assert.Nil(t, err, "CleanupAll() failed")

	// Verify directories were removed
	exists, err = mockFS.FileExists(path1)
	assert.Nil(t, err, "FileExists should not return an error")
	assert.False(t, exists, "TempDir1 directory should have been removed: %s", path1)

	exists, err = mockFS.FileExists(path2)
	assert.Nil(t, err, "FileExists should not return an error")
	assert.False(t, exists, "TempDir2 directory should have been removed: %s", path2)

	exists, err = mockFS.FileExists(path2)
	assert.Nil(t, err, "FileExists should not return an error")
	assert.False(t, exists, "TempDir2 directory should have been removed: %s", path2)

	// Try to cleanup non-existent temp directory
	err = manager.CleanupTempDir(path1)
	assert.Error(t, err, "CleanupTempDir() should return error for non-existent resource")
	err = manager.CleanupTempDir(path2)
	assert.Error(t, err, "CleanupTempDir() should return error for non-existent resource")
}
