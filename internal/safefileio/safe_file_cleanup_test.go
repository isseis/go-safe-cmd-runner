package safefileio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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

// mockFile is a test implementation of File interface.
// Thread-safety note: The pos and data fields are protected by mu to prevent race conditions
// when the mock is used in concurrent test scenarios (e.g., with t.Parallel()).
// ReadAt is an exception: it reads data but does NOT modify pos per io.ReaderAt contract.
type mockFile struct {
	mu          sync.Mutex
	data        []byte // raw data for ReadAt/Seek support; protected by mu
	pos         int64  // current position; protected by mu
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
		data:     content,
		pos:      0,
		fileInfo: fileInfo,
	}
}

func (m *mockFile) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pos >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.pos:])
	m.pos += int64(n)
	return n, nil
}

func (m *mockFile) Write(p []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Extend data if needed
	endPos := m.pos + int64(len(p))
	if endPos > int64(len(m.data)) {
		newData := make([]byte, endPos)
		copy(newData, m.data)
		m.data = newData
	}
	n = copy(m.data[m.pos:], p)
	m.pos += int64(n)
	return n, nil
}

func (m *mockFile) Seek(offset int64, whence int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = m.pos + offset
	case io.SeekEnd:
		newPos = int64(len(m.data)) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if newPos < 0 {
		return 0, fmt.Errorf("negative position: %d", newPos)
	}
	m.pos = newPos
	return m.pos, nil
}

func (m *mockFile) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, fmt.Errorf("negative offset: %d", off)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Note: ReadAt does NOT modify pos per io.ReaderAt contract
	if off >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n = copy(p, m.data[off:])
	if n < len(p) {
		err = io.EOF
	}
	return n, err
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
	m.mu.Lock()
	defer m.mu.Unlock()
	// Reject negative size (matches os.File.Truncate behavior)
	if size < 0 {
		return fmt.Errorf("negative size: %d", size)
	}
	// Handle shrinking
	if size < int64(len(m.data)) {
		m.data = m.data[:size]
		if m.pos > size {
			m.pos = size
		}
	} else if size > int64(len(m.data)) {
		// Handle extension with null bytes (matches os.File.Truncate behavior)
		diff := size - int64(len(m.data))
		m.data = append(m.data, make([]byte, diff)...)
		// Position stays unchanged when extending
	}
	// size == len(m.data) - no change needed
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
func (m *mockFileInfo) Sys() any {
	// Return a syscall.Stat_t for compatibility with getFileStatInfo
	return &syscall.Stat_t{
		Uid: m.uid,
		Gid: m.gid,
	}
}

// mockFileSystem is a test implementation of FileSystem interface
type mockFileSystem struct {
	openFunc           func(name string, flag int, perm os.FileMode) (File, error)
	removeFunc         func(name string) error
	atomicMoveFileFunc func(srcPath, dstPath string, requiredPerm os.FileMode) error
	groupMembership    *groupmembership.GroupMembership
	removedFiles       []string
	removeCallCount    int
	mu                 sync.Mutex
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

func (m *mockFileSystem) AtomicMoveFile(srcPath, dstPath string, requiredPerm os.FileMode) error {
	if m.atomicMoveFileFunc != nil {
		return m.atomicMoveFileFunc(srcPath, dstPath, requiredPerm)
	}
	return errors.New("mock AtomicMoveFile not implemented")
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
				data:     nil,
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
				data:     nil,
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
				data:     nil,
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

// Test suite for mockFile.Truncate method enhancements
func TestMockFileTruncate(t *testing.T) {
	t.Run("truncate_with_negative_size_returns_error", func(t *testing.T) {
		mockFile := newMockFile([]byte("test content"), &mockFileInfo{name: "test.txt", size: 12})
		err := mockFile.Truncate(-1)
		assert.Error(t, err, "Truncate with negative size should return error")
		assert.Equal(t, []byte("test content"), mockFile.data, "Data should not change on error")
	})

	t.Run("truncate_shrinks_file", func(t *testing.T) {
		mockFile := newMockFile([]byte("test content"), &mockFileInfo{name: "test.txt", size: 12})
		err := mockFile.Truncate(4)
		require.NoError(t, err)
		assert.Equal(t, []byte("test"), mockFile.data, "File should be truncated to 4 bytes")
	})

	t.Run("truncate_shrinks_file_and_resets_position", func(t *testing.T) {
		mockFile := newMockFile([]byte("test content"), &mockFileInfo{name: "test.txt", size: 12})
		mockFile.pos = 10
		err := mockFile.Truncate(5)
		require.NoError(t, err)
		assert.Equal(t, int64(5), mockFile.pos, "Position should be reset to truncate size when beyond")
		assert.Equal(t, []byte("test "), mockFile.data, "File should be truncated to 5 bytes")
	})

	t.Run("truncate_extends_file_with_null_bytes", func(t *testing.T) {
		mockFile := newMockFile([]byte("test"), &mockFileInfo{name: "test.txt", size: 4})
		err := mockFile.Truncate(8)
		require.NoError(t, err)
		assert.Equal(t, 8, len(mockFile.data), "File should be extended to 8 bytes")
		// First 4 bytes should be original content
		assert.Equal(t, []byte("test"), mockFile.data[:4], "Original content should be preserved")
		// Last 4 bytes should be null bytes
		assert.Equal(t, []byte{0, 0, 0, 0}, mockFile.data[4:], "Extended portion should be null bytes")
	})

	t.Run("truncate_extends_file_preserves_position", func(t *testing.T) {
		mockFile := newMockFile([]byte("test"), &mockFileInfo{name: "test.txt", size: 4})
		mockFile.pos = 2
		err := mockFile.Truncate(10)
		require.NoError(t, err)
		assert.Equal(t, int64(2), mockFile.pos, "Position should not change when extending")
		assert.Equal(t, 10, len(mockFile.data), "File should be extended to 10 bytes")
	})

	t.Run("truncate_to_same_size_is_noop", func(t *testing.T) {
		original := []byte("test content")
		mockFile := newMockFile(original, &mockFileInfo{name: "test.txt", size: 12})
		mockFile.pos = 5
		err := mockFile.Truncate(12)
		require.NoError(t, err)
		assert.Equal(t, original, mockFile.data, "Data should not change when truncating to same size")
		assert.Equal(t, int64(5), mockFile.pos, "Position should not change")
	})

	t.Run("truncate_to_zero", func(t *testing.T) {
		mockFile := newMockFile([]byte("test content"), &mockFileInfo{name: "test.txt", size: 12})
		mockFile.pos = 5
		err := mockFile.Truncate(0)
		require.NoError(t, err)
		assert.Equal(t, 0, len(mockFile.data), "File should be empty after truncate to 0")
		assert.Equal(t, int64(0), mockFile.pos, "Position should be reset to 0")
	})

	t.Run("truncate_error_handling", func(t *testing.T) {
		mockFile := newMockFile([]byte("test"), &mockFileInfo{name: "test.txt", size: 4})
		mockFile.truncateErr = errors.New("permission denied")
		err := mockFile.Truncate(8)
		assert.Error(t, err)
		assert.Equal(t, "permission denied", err.Error())
		assert.Equal(t, []byte("test"), mockFile.data, "Data should not change when error is set")
	})
}

// TestMockFileSeek tests the Seek method of mockFile for all whence values and error cases.
func TestMockFileSeek(t *testing.T) {
	t.Run("seek_start", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		pos, err := mf.Seek(5, io.SeekStart)
		require.NoError(t, err)
		assert.Equal(t, int64(5), pos)
		assert.Equal(t, int64(5), mf.pos)
	})

	t.Run("seek_start_to_beginning", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		mf.pos = 7
		pos, err := mf.Seek(0, io.SeekStart)
		require.NoError(t, err)
		assert.Equal(t, int64(0), pos)
	})

	t.Run("seek_start_beyond_end", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		pos, err := mf.Seek(100, io.SeekStart)
		require.NoError(t, err)
		assert.Equal(t, int64(100), pos, "Seek beyond end should be allowed (like os.File)")
	})

	t.Run("seek_current_forward", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		mf.pos = 3
		pos, err := mf.Seek(4, io.SeekCurrent)
		require.NoError(t, err)
		assert.Equal(t, int64(7), pos)
	})

	t.Run("seek_current_backward", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		mf.pos = 8
		pos, err := mf.Seek(-3, io.SeekCurrent)
		require.NoError(t, err)
		assert.Equal(t, int64(5), pos)
	})

	t.Run("seek_current_zero", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		mf.pos = 6
		pos, err := mf.Seek(0, io.SeekCurrent)
		require.NoError(t, err)
		assert.Equal(t, int64(6), pos, "Seek(0, SeekCurrent) should return current position")
	})

	t.Run("seek_end", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		pos, err := mf.Seek(0, io.SeekEnd)
		require.NoError(t, err)
		assert.Equal(t, int64(11), pos, "Seek(0, SeekEnd) should return file length")
	})

	t.Run("seek_end_negative_offset", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		pos, err := mf.Seek(-5, io.SeekEnd)
		require.NoError(t, err)
		assert.Equal(t, int64(6), pos)
	})

	t.Run("seek_negative_position_from_start", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		_, err := mf.Seek(-1, io.SeekStart)
		assert.Error(t, err, "Negative position should return error")
		assert.Contains(t, err.Error(), "negative position")
	})

	t.Run("seek_negative_position_from_current", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		mf.pos = 2
		_, err := mf.Seek(-5, io.SeekCurrent)
		assert.Error(t, err, "Resulting negative position should return error")
		assert.Contains(t, err.Error(), "negative position")
	})

	t.Run("seek_negative_position_from_end", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		_, err := mf.Seek(-10, io.SeekEnd)
		assert.Error(t, err, "Resulting negative position should return error")
		assert.Contains(t, err.Error(), "negative position")
	})

	t.Run("seek_invalid_whence", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		_, err := mf.Seek(0, 99)
		assert.Error(t, err, "Invalid whence should return error")
		assert.Contains(t, err.Error(), "invalid whence")
	})

	t.Run("seek_then_read", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		_, err := mf.Seek(6, io.SeekStart)
		require.NoError(t, err)

		buf := make([]byte, 5)
		n, err := mf.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("world"), buf)
	})
}

// TestMockFileReadAt tests the ReadAt method of mockFile.
func TestMockFileReadAt(t *testing.T) {
	t.Run("read_at_beginning", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		buf := make([]byte, 5)
		n, err := mf.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hello"), buf)
	})

	t.Run("read_at_offset", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		buf := make([]byte, 5)
		n, err := mf.ReadAt(buf, 6)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("world"), buf)
	})

	t.Run("read_at_partial_eof", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		buf := make([]byte, 10)
		n, err := mf.ReadAt(buf, 3)
		assert.ErrorIs(t, err, io.EOF, "Partial read should return io.EOF")
		assert.Equal(t, 2, n, "Should read remaining bytes")
		assert.Equal(t, []byte("lo"), buf[:n])
	})

	t.Run("read_at_exact_end", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		buf := make([]byte, 5)
		n, err := mf.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hello"), buf)
	})

	t.Run("read_at_beyond_end", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		buf := make([]byte, 5)
		n, err := mf.ReadAt(buf, 10)
		assert.ErrorIs(t, err, io.EOF)
		assert.Equal(t, 0, n)
	})

	t.Run("read_at_negative_offset", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		buf := make([]byte, 5)
		_, err := mf.ReadAt(buf, -1)
		assert.Error(t, err, "Negative offset should return error")
		assert.Contains(t, err.Error(), "negative offset")
	})

	t.Run("read_at_does_not_modify_position", func(t *testing.T) {
		mf := newMockFile([]byte("hello world"), &mockFileInfo{name: "test.txt", size: 11})
		mf.pos = 3
		buf := make([]byte, 5)
		_, err := mf.ReadAt(buf, 6)
		require.NoError(t, err)
		assert.Equal(t, int64(3), mf.pos, "ReadAt should not modify the file position")
	})

	t.Run("read_at_empty_buffer", func(t *testing.T) {
		mf := newMockFile([]byte("hello"), &mockFileInfo{name: "test.txt", size: 5})
		buf := make([]byte, 0)
		n, err := mf.ReadAt(buf, 0)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
	})
}
