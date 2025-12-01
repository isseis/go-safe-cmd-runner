package safefileio

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFile is a test implementation of File interface
type mockFile struct {
	content     *bytes.Buffer
	statErr     error
	writeErr    error
	closeErr    error
	truncateErr error
	fileInfo    os.FileInfo
	isClosed    bool
	closeMutex  sync.Mutex
}

func newMockFile(content []byte, fileInfo os.FileInfo) *mockFile {
	return &mockFile{
		content:  bytes.NewBuffer(content),
		fileInfo: fileInfo,
	}
}

func (m *mockFile) Read(p []byte) (n int, err error) {
	return m.content.Read(p)
}

func (m *mockFile) Write(p []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return m.content.Write(p)
}

func (m *mockFile) Close() error {
	m.closeMutex.Lock()
	defer m.closeMutex.Unlock()
	m.isClosed = true
	return m.closeErr
}

func (m *mockFile) Stat() (os.FileInfo, error) {
	if m.statErr != nil {
		return nil, m.statErr
	}
	return m.fileInfo, nil
}

func (m *mockFile) Truncate(size int64) error {
	if m.truncateErr != nil {
		return m.truncateErr
	}
	// Simulate truncate by resetting the buffer
	if size == 0 {
		m.content.Reset()
	}
	return nil
}

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct {
	name string
	mode os.FileMode
	size int64
	uid  uint32
	gid  uint32
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() interface{} {
	// Return a syscall.Stat_t for compatibility with getFileStatInfo
	return &syscall.Stat_t{
		Uid: m.uid,
		Gid: m.gid,
	}
}

// mockFileSystem is a test implementation of FileSystem interface
type mockFileSystem struct {
	openFunc        func(name string, flag int, perm os.FileMode) (File, error)
	removeFunc      func(name string) error
	groupMembership *groupmembership.GroupMembership
	removedFiles    []string
	removeCallCount int
	mu              sync.Mutex
}

func newMockFileSystem() *mockFileSystem {
	return &mockFileSystem{
		groupMembership: groupmembership.New(),
		removedFiles:    []string{},
	}
}

func (m *mockFileSystem) SafeOpenFile(name string, flag int, perm os.FileMode) (File, error) {
	if m.openFunc != nil {
		return m.openFunc(name, flag, perm)
	}
	return nil, errors.New("mock open not implemented")
}

func (m *mockFileSystem) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeCallCount++
	m.removedFiles = append(m.removedFiles, name)
	if m.removeFunc != nil {
		return m.removeFunc(name)
	}
	return nil
}

func (m *mockFileSystem) GetGroupMembership() *groupmembership.GroupMembership {
	return m.groupMembership
}

func (m *mockFileSystem) getRemovedFiles() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.removedFiles...)
}

func (m *mockFileSystem) getRemoveCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeCallCount
}

// TestSafeWriteFile_CleanupOnValidationError tests that newly created files are removed when validation fails
func TestSafeWriteFile_CleanupOnValidationError(t *testing.T) {
	t.Run("file is cleaned up when validation fails after creation", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "test_cleanup.txt")
		absPath, err := filepath.Abs(filePath)
		require.NoError(t, err)

		mockFS := newMockFileSystem()

		// Setup: SafeOpenFile succeeds (simulating file creation with O_EXCL)
		// but Stat returns an error to trigger validation failure
		mockFS.openFunc = func(_ string, _ int, _ os.FileMode) (File, error) {
			// Simulate file creation that will fail validation
			mockFile := &mockFile{
				content:  bytes.NewBuffer(nil),
				statErr:  errors.New("stat error to trigger validation failure"),
				isClosed: false,
			}
			return mockFile, nil
		}

		content := []byte("test content")

		// Execute: Try to write file with O_EXCL (new file creation)
		err = safeWriteFileCommon(filePath, content, 0o644, mockFS, os.O_WRONLY|os.O_CREATE|os.O_EXCL)

		// Verify: Operation failed
		require.Error(t, err)

		// Verify: Remove was called once to clean up the created file
		assert.Equal(t, 1, mockFS.getRemoveCallCount(), "Remove should be called once to clean up")
		removedFiles := mockFS.getRemovedFiles()
		require.Len(t, removedFiles, 1, "Exactly one file should be removed")
		assert.Equal(t, absPath, removedFiles[0], "The created file should be removed")
	})

	t.Run("file is cleaned up when write fails after creation", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "test_write_error.txt")
		absPath, err := filepath.Abs(filePath)
		require.NoError(t, err)

		mockFS := newMockFileSystem()

		// Get current user's UID and GID for validation
		currentUID := uint32(os.Getuid())
		currentGID := uint32(os.Getgid())

		// Setup: Create a file that passes validation but fails on write
		mockFS.openFunc = func(name string, _ int, perm os.FileMode) (File, error) {
			fileInfo := &mockFileInfo{
				name: filepath.Base(name),
				mode: perm,
				size: 0,
				uid:  currentUID,
				gid:  currentGID,
			}
			mockFile := newMockFile(nil, fileInfo)
			mockFile.writeErr = errors.New("write error")
			return mockFile, nil
		}

		content := []byte("test content")

		// Execute: Try to write file with O_EXCL
		err = safeWriteFileWithFS(filePath, content, 0o644, mockFS)

		// Verify: Operation failed
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write")

		// Verify: File was removed during cleanup
		assert.Equal(t, 1, mockFS.getRemoveCallCount(), "Remove should be called once")
		removedFiles := mockFS.getRemovedFiles()
		require.Len(t, removedFiles, 1)
		assert.Equal(t, absPath, removedFiles[0])
	})
}

// TestSafeWriteFileOverwrite_NoCleanupOnError tests that existing files are NOT deleted when overwrite fails
func TestSafeWriteFileOverwrite_NoCleanupOnError(t *testing.T) {
	t.Run("existing file is NOT deleted when overwrite validation fails", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "existing_file.txt")

		mockFS := newMockFileSystem()

		// Setup: SafeOpenFile succeeds (overwrite mode - no O_TRUNC, no O_EXCL)
		// but validation fails
		mockFS.openFunc = func(_ string, flag int, _ os.FileMode) (File, error) {
			// Verify this is an overwrite operation (no O_TRUNC, no O_EXCL)
			assert.False(t, flag&os.O_TRUNC != 0, "Should NOT be using O_TRUNC for overwrite")
			assert.False(t, flag&os.O_EXCL != 0, "Should NOT be using O_EXCL for overwrite")

			mockFile := &mockFile{
				content:  bytes.NewBuffer(nil),
				statErr:  errors.New("validation error"),
				isClosed: false,
			}
			return mockFile, nil
		}

		content := []byte("new content")

		// Execute: Try to overwrite (truncate happens after validation)
		err := safeWriteFileOverwriteWithFS(filePath, content, 0o644, mockFS)

		// Verify: Operation failed
		require.Error(t, err)

		// Verify: Remove was NOT called (fileCreated should be false when not using O_EXCL)
		assert.Equal(t, 0, mockFS.getRemoveCallCount(),
			"Remove should NOT be called when overwriting existing file fails")
	})

	t.Run("existing file is NOT deleted when overwrite write fails", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "existing_write_fail.txt")

		mockFS := newMockFileSystem()

		// Get current user's UID and GID
		currentUID := uint32(os.Getuid())
		currentGID := uint32(os.Getgid())

		// Setup: File passes validation but write fails
		mockFS.openFunc = func(name string, _ int, perm os.FileMode) (File, error) {
			fileInfo := &mockFileInfo{
				name: filepath.Base(name),
				mode: perm,
				size: 100, // Non-zero size indicates existing file
				uid:  currentUID,
				gid:  currentGID,
			}
			mockFile := newMockFile([]byte("old content"), fileInfo)
			mockFile.writeErr = errors.New("disk full")
			return mockFile, nil
		}

		content := []byte("new content")

		// Execute
		err := safeWriteFileOverwriteWithFS(filePath, content, 0o644, mockFS)

		// Verify
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write")

		// Verify: File was NOT removed
		assert.Equal(t, 0, mockFS.getRemoveCallCount(),
			"Existing file should NOT be removed when write fails during overwrite")
	})

	t.Run("existing file is NOT deleted when truncate fails", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "existing_truncate_fail.txt")

		mockFS := newMockFileSystem()

		// Get current user's UID and GID
		currentUID := uint32(os.Getuid())
		currentGID := uint32(os.Getgid())

		// Setup: File passes validation but truncate fails
		mockFS.openFunc = func(name string, _ int, perm os.FileMode) (File, error) {
			fileInfo := &mockFileInfo{
				name: filepath.Base(name),
				mode: perm,
				size: 100, // Non-zero size indicates existing file
				uid:  currentUID,
				gid:  currentGID,
			}
			mockFile := newMockFile([]byte("old content"), fileInfo)
			mockFile.truncateErr = errors.New("truncate failed")
			return mockFile, nil
		}

		content := []byte("new content")

		// Execute
		err := safeWriteFileOverwriteWithFS(filePath, content, 0o644, mockFS)

		// Verify
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to truncate")

		// Verify: File was NOT removed (truncate failure should not trigger cleanup)
		assert.Equal(t, 0, mockFS.getRemoveCallCount(),
			"Existing file should NOT be removed when truncate fails during overwrite")
	})
}

// TestFileCleanup_RemoveFailureWarning tests that warnings are logged when file removal fails
func TestFileCleanup_RemoveFailureWarning(t *testing.T) {
	t.Run("warning is logged when file removal fails during cleanup", func(t *testing.T) {
		// Setup: Capture log output
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))
		oldDefault := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(oldDefault)

		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "test_remove_fail.txt")
		absPath, err := filepath.Abs(filePath)
		require.NoError(t, err)

		mockFS := newMockFileSystem()

		// Setup: File creation and validation succeed, but Remove fails
		mockFS.openFunc = func(_ string, _ int, _ os.FileMode) (File, error) {
			mockFile := &mockFile{
				content:  bytes.NewBuffer(nil),
				statErr:  errors.New("validation error"), // Trigger cleanup
				isClosed: false,
			}
			return mockFile, nil
		}

		removeErr := errors.New("permission denied during remove")
		mockFS.removeFunc = func(_ string) error {
			return removeErr
		}

		content := []byte("test content")

		// Execute: This should trigger cleanup, which will fail
		err = safeWriteFileCommon(filePath, content, 0o644, mockFS, os.O_WRONLY|os.O_CREATE|os.O_EXCL)

		// Verify: Original error is returned (not the remove error)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation error")

		// Verify: Remove was attempted
		assert.Equal(t, 1, mockFS.getRemoveCallCount())

		// Verify: Warning was logged
		logOutput := logBuf.String()
		assert.Contains(t, logOutput, "failed to remove file after error",
			"Should log warning about remove failure")
		assert.Contains(t, logOutput, absPath,
			"Log should contain the file path")
		assert.Contains(t, logOutput, "permission denied during remove",
			"Log should contain the remove error message")
	})
}

// TestFileCleanup_Integration tests the cleanup behavior with real filesystem
func TestFileCleanup_Integration(t *testing.T) {
	t.Run("real file is cleaned up on validation error", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "cleanup_test.txt")

		// Create a custom filesystem that will fail validation after file creation
		fs := NewFileSystem(FileSystemConfig{})

		// This will fail because we're trying to create a file with world-writable permissions
		// which will be rejected during validation
		content := []byte("test content")
		err := safeWriteFileWithFS(filePath, content, 0o666, fs)

		// Verify: Operation failed
		require.Error(t, err)
		assert.ErrorIs(t, err, groupmembership.ErrPermissionsExceedMaximum)

		// Verify: File was cleaned up (does not exist)
		_, statErr := os.Stat(filePath)
		assert.True(t, os.IsNotExist(statErr),
			"File should be cleaned up and not exist after validation failure")
	})

	t.Run("existing file is NOT deleted on overwrite error", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "existing.txt")

		// Create an existing file with valid permissions
		originalContent := []byte("original content")
		require.NoError(t, os.WriteFile(filePath, originalContent, 0o644))

		fs := NewFileSystem(FileSystemConfig{})

		// Try to overwrite with invalid permissions (should fail validation)
		newContent := []byte("new content")
		err := safeWriteFileOverwriteWithFS(filePath, newContent, 0o666, fs)

		// Verify: Operation failed
		require.Error(t, err)

		// Verify: File still exists (was not deleted)
		_, statErr := os.Stat(filePath)
		assert.NoError(t, statErr, "File should still exist after overwrite failure")

		// Verify: File content is preserved (truncate happens after validation)
		content, readErr := os.ReadFile(filePath)
		require.NoError(t, readErr)
		assert.Equal(t, originalContent, content, "Original content should be preserved when validation fails")
	})
}
