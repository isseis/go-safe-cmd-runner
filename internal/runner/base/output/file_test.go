package output

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio/testutil"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errRenameFailed is the failure injected into the atomic-move seam.
var errRenameFailed = errors.New("rename failed")

func TestSafeFileManager_CreateTempFile(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		pattern string
		wantErr error
	}{
		{
			name:    "valid_temp_file_creation",
			dir:     "", // Will use default temp dir
			pattern: "output_*.tmp",
			wantErr: nil,
		},
		{
			name:    "valid_temp_file_with_specific_dir",
			pattern: "test_*.tmp",
			wantErr: nil,
		},
	}

	manager := NewSafeFileManager()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test directory if needed
			var testDir string
			if tt.name == "valid_temp_file_with_specific_dir" {
				testDir = tu.SafeTempDir(t)
				tt.dir = testDir
			}

			file, err := manager.CreateTempFile(tt.dir, tt.pattern)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				if file != nil {
					file.Close()
					os.Remove(file.Name())
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, file)

				// Verify file was created
				stat, err := file.Stat()
				require.NoError(t, err)
				assert.NotEmpty(t, stat.Name())

				// Verify permissions are secure (0600)
				assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm())

				// Clean up
				fileName := file.Name()
				file.Close()
				os.Remove(fileName)
			}
		})
	}
}

func TestSafeFileManager_WriteToTemp(t *testing.T) {
	manager := NewSafeFileManager()
	tempFile, err := manager.CreateTempFile("", "write_test_*.tmp")
	require.NoError(t, err)
	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	tests := []struct {
		name    string
		data    []byte
		wantN   int
		wantErr error
	}{
		{
			name:    "write_valid_data",
			data:    []byte("test data\n"),
			wantN:   10,
			wantErr: nil,
		},
		{
			name:    "write_empty_data",
			data:    []byte{},
			wantN:   0,
			wantErr: nil,
		},
		{
			name:    "write_binary_data",
			data:    []byte{0x00, 0x01, 0x02, 0xFF},
			wantN:   4,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := manager.WriteToTemp(tempFile, tt.data)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantN, n)
			}
		})
	}
}

func TestSafeFileManager_EnsureDirectory(t *testing.T) {
	tests := []struct {
		name                string
		setupPath           func(t *testing.T) string
		wantErr             error
		skipPermissionCheck bool
	}{
		{
			name: "create_new_directory",
			setupPath: func(t *testing.T) string {
				tempDir := tu.SafeTempDir(t)
				return filepath.Join(tempDir, "new_dir")
			},
			wantErr: nil,
		},
		{
			name: "existing_directory",
			setupPath: func(t *testing.T) string {
				tempDir := tu.SafeTempDir(t)
				newDir := filepath.Join(tempDir, "existing")
				err := os.MkdirAll(newDir, 0o755)
				require.NoError(t, err)
				return newDir
			},
			wantErr:             nil,
			skipPermissionCheck: true, // Existing directory has different permissions
		},
		{
			name: "nested_directory_creation",
			setupPath: func(t *testing.T) string {
				tempDir := tu.SafeTempDir(t)
				return filepath.Join(tempDir, "level1", "level2", "level3")
			},
			wantErr: nil,
		},
		{
			name: "path_on_existing_file",
			setupPath: func(t *testing.T) string {
				tempDir := tu.SafeTempDir(t)
				filePath := filepath.Join(tempDir, "existing_file")
				file, err := os.Create(filePath)
				require.NoError(t, err)
				file.Close()
				return filePath
			},
			wantErr: ErrNotDirectory,
		},
	}

	manager := NewSafeFileManager()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupPath(t)
			err := manager.EnsureDirectory(path)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)

				// Verify directory exists
				stat, err := os.Stat(path)
				require.NoError(t, err)
				assert.True(t, stat.IsDir())

				// Verify permissions (only for newly created directories)
				if !tt.skipPermissionCheck {
					assert.Equal(t, os.FileMode(0o750), stat.Mode().Perm())
				}
			}
		})
	}
}

func TestSafeFileManager_MoveToFinal(t *testing.T) {
	manager := NewSafeFileManager()

	tests := []struct {
		name       string
		setupFiles func(t *testing.T) (tempPath, finalPath string)
		wantErr    error
	}{
		{
			name: "move_existing_temp_file",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := tu.SafeTempDir(t)

				// Create temp file
				tempFile, err := manager.CreateTempFile(tempDir, "move_test_*.tmp")
				require.NoError(t, err)

				// Write some data
				data := []byte("test content for move")
				_, err = manager.WriteToTemp(tempFile, data)
				require.NoError(t, err)
				tempFile.Close()

				finalPath := filepath.Join(tempDir, "final_output.txt")
				return tempFile.Name(), finalPath
			},
			wantErr: nil,
		},
		{
			name: "move_to_existing_file_overwrite",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := tu.SafeTempDir(t)

				// Create temp file
				tempFile, err := manager.CreateTempFile(tempDir, "move_overwrite_*.tmp")
				require.NoError(t, err)
				data := []byte("new content")
				_, err = manager.WriteToTemp(tempFile, data)
				require.NoError(t, err)
				tempFile.Close()

				// Create existing final file
				finalPath := filepath.Join(tempDir, "existing_final.txt")
				err = os.WriteFile(finalPath, []byte("old content"), 0o644)
				require.NoError(t, err)

				return tempFile.Name(), finalPath
			},
			wantErr: nil,
		},
		{
			name: "move_nonexistent_temp_file",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := tu.SafeTempDir(t)
				tempPath := filepath.Join(tempDir, "nonexistent.tmp")
				finalPath := filepath.Join(tempDir, "final.txt")
				return tempPath, finalPath
			},
			wantErr: os.ErrNotExist,
		},
		{
			name: "move_to_directory_instead_of_file",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := tu.SafeTempDir(t)

				// Create temp file
				tempFile, err := manager.CreateTempFile(tempDir, "move_to_dir_*.tmp")
				require.NoError(t, err)
				tempFile.Close()

				// Create directory as "final" destination
				finalPath := filepath.Join(tempDir, "final_dir")
				err = os.MkdirAll(finalPath, 0o755)
				require.NoError(t, err)

				return tempFile.Name(), finalPath
			},
			// The rename fails because the destination already exists as a
			// directory. Matching "directory" in the message would also have been
			// satisfied by the temp path, which carries the test name.
			wantErr: fs.ErrExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempPath, finalPath := tt.setupFiles(t)

			// Store original content if temp file exists
			var originalContent []byte
			if _, err := os.Stat(tempPath); err == nil {
				originalContent, err = os.ReadFile(tempPath)
				require.NoError(t, err)
			}

			const testPerm = os.FileMode(0o600)
			err := manager.MoveToFinal(tempPath, finalPath, testPerm)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)

				// Verify temp file was moved (no longer exists)
				_, err = os.Stat(tempPath)
				assert.True(t, os.IsNotExist(err), "temp file should be removed")

				// Verify final file exists with correct content
				finalContent, err := os.ReadFile(finalPath)
				require.NoError(t, err)
				assert.Equal(t, originalContent, finalContent)

				// Verify final file has the permission specified by the caller
				stat, err := os.Stat(finalPath)
				require.NoError(t, err)
				assert.Equal(t, testPerm, stat.Mode().Perm())
			}
		})
	}
}

func TestSafeFileManager_RemoveTemp(t *testing.T) {
	manager := NewSafeFileManager()

	tests := []struct {
		name      string
		setupPath func(t *testing.T) string
		wantErr   error
	}{
		{
			name: "remove_existing_temp_file",
			setupPath: func(t *testing.T) string {
				tempFile, err := manager.CreateTempFile("", "remove_test_*.tmp")
				require.NoError(t, err)
				tempFile.Close()
				return tempFile.Name()
			},
			wantErr: nil,
		},
		{
			name: "remove_nonexistent_file",
			setupPath: func(t *testing.T) string {
				tempDir := tu.SafeTempDir(t)
				return filepath.Join(tempDir, "nonexistent.tmp")
			},
			wantErr: nil, // RemoveTemp should be idempotent
		},
		{
			name: "remove_directory_instead_of_file",
			setupPath: func(t *testing.T) string {
				tempDir := tu.SafeTempDir(t)
				dirPath := filepath.Join(tempDir, "test_dir")
				err := os.MkdirAll(dirPath, 0o755)
				require.NoError(t, err)
				return dirPath
			},
			wantErr: ErrNotFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupPath(t)
			err := manager.RemoveTemp(path)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)

				// Verify file no longer exists
				_, err = os.Stat(path)
				assert.True(t, os.IsNotExist(err), "file should be removed")
			}
		})
	}
}

func TestSafeFileManager_FileDescriptorLeakagePrevention(t *testing.T) {
	manager := NewSafeFileManager()
	tempDir := tu.SafeTempDir(t)

	// 1. Create target file with potentially vulnerable permissions
	finalPath := filepath.Join(tempDir, "output.txt")
	oldContent := []byte("sensitive old output")
	require.NoError(t, os.WriteFile(finalPath, oldContent, 0o644))

	// 2. Simulate attacker opening the file
	attackerFd, err := os.Open(finalPath)
	require.NoError(t, err, "Attacker should be able to open the file")
	defer attackerFd.Close()

	// Verify attacker can read original content
	attackerContent := make([]byte, len(oldContent))
	n, err := attackerFd.Read(attackerContent)
	require.NoError(t, err, "Attacker should be able to read original content")
	assert.Equal(t, oldContent, attackerContent[:n], "Attacker should see original content")

	// 3. Create temp file with new sensitive content
	tempFile, err := manager.CreateTempFile(tempDir, "safe-*.tmp")
	require.NoError(t, err)
	tempPath := tempFile.Name()

	newContent := []byte("new sensitive output that should not leak")
	_, err = manager.WriteToTemp(tempFile, newContent)
	require.NoError(t, err)
	require.NoError(t, tempFile.Close())

	// 4. Use SafeFileManager to move temp to final
	err = manager.MoveToFinal(tempPath, finalPath, 0o600)
	assert.NoError(t, err, "MoveToFinal should succeed")

	// 5. Verify file was overwritten
	finalContent, err := os.ReadFile(finalPath)
	require.NoError(t, err, "Should be able to read final file")
	assert.Equal(t, newContent, finalContent, "File should contain new content")

	// 6. Critical security check: Old file descriptor should not leak new content
	_, err = attackerFd.Seek(0, 0)
	require.NoError(t, err, "Should be able to seek to beginning")

	attackerNewRead := make([]byte, len(newContent))
	n, readErr := attackerFd.Read(attackerNewRead)

	switch {
	case readErr != nil:
		t.Logf("Attacker's file descriptor became invalid after move: %v", readErr)
	case n == 0:
		t.Logf("Attacker's file descriptor returned no content")
	default:
		attackerReadContent := attackerNewRead[:n]
		assert.False(t, bytes.Equal(attackerReadContent, newContent), "SECURITY ISSUE: Attacker can read new content through old file descriptor")
		t.Logf("Attacker sees different content (safe): %q", attackerReadContent)
	}

	// 7. Verify secure permissions
	stat, err := os.Stat(finalPath)
	require.NoError(t, err, "Should be able to stat final file")
	assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm(), "Final file should have secure permissions")
}

func TestSafeFileManager_Integration(t *testing.T) {
	// Integration test for complete file operation workflow
	manager := NewSafeFileManager()
	tempDir := tu.SafeTempDir(t)
	finalPath := filepath.Join(tempDir, "integration_output.txt")

	// Ensure directory exists
	err := manager.EnsureDirectory(tempDir)
	require.NoError(t, err)

	// Create temp file
	tempFile, err := manager.CreateTempFile(tempDir, "integration_*.tmp")
	require.NoError(t, err)
	tempPath := tempFile.Name()

	// Write data to temp file
	testData := []byte("Integration test data\nLine 2\nLine 3\n")
	n, err := manager.WriteToTemp(tempFile, testData)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)

	// Close temp file before moving
	tempFile.Close()

	// Move to final location
	err = manager.MoveToFinal(tempPath, finalPath, 0o600)
	require.NoError(t, err)

	// Verify final file content
	finalContent, err := os.ReadFile(finalPath)
	require.NoError(t, err)
	assert.Equal(t, testData, finalContent)

	// Verify temp file was removed
	_, err = os.Stat(tempPath)
	assert.True(t, os.IsNotExist(err))

	// Verify final file permissions
	stat, err := os.Stat(finalPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm())
}

// ============================================================================
// Mock-based Unit Tests
// These tests use mock implementations to test SafeFileManager logic
// without depending on the actual file system.
// ============================================================================

func TestSafeFileManager_MoveToFinal_WithMock(t *testing.T) {
	tests := []struct {
		name                 string
		tempPath             string
		finalPath            string
		perm                 os.FileMode
		setupMock            func(*commontestutil.MockFileSystem)
		atomicMoveError      error
		wantErr              error
		wantAtomicMoveCalled bool
	}{
		{
			name:      "successful_move",
			tempPath:  "/tmp/test.tmp",
			finalPath: "/output/final.txt",
			perm:      0o600,
			setupMock: func(mock *commontestutil.MockFileSystem) {
				// Pre-add directory so EnsureDirectory succeeds
				require.NoError(t, mock.AddDir("/output", 0o750))
			},
			atomicMoveError:      nil,
			wantErr:              nil,
			wantAtomicMoveCalled: true,
		},
		{
			name:      "atomic_move_fails",
			tempPath:  "/tmp/test.tmp",
			finalPath: "/output/final.txt",
			perm:      0o600,
			setupMock: func(mock *commontestutil.MockFileSystem) {
				// Pre-add directory so EnsureDirectory succeeds
				require.NoError(t, mock.AddDir("/output", 0o750))
			},
			atomicMoveError: errRenameFailed,
			// Asserting on the injected error also pins that MoveToFinal wraps it
			// rather than replacing it with one of its own.
			wantErr:              errRenameFailed,
			wantAtomicMoveCalled: true,
		},
		{
			name:      "ensure_directory_fails_path_is_file",
			tempPath:  "/tmp/test.tmp",
			finalPath: "/existing_file/final.txt",
			perm:      0o600,
			setupMock: func(mock *commontestutil.MockFileSystem) {
				// Add a file at the path where we expect a directory
				require.NoError(t, mock.AddFile("/existing_file", 0o644, []byte("content")))
			},
			wantErr:              ErrNotDirectory,
			wantAtomicMoveCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock safefileio.FileSystem
			mockSafeFS := safefileiotestutil.NewMockFileSystem()
			mockSafeFS.AtomicMoveFileFunc = func(_, _ string, _ os.FileMode) error {
				return tt.atomicMoveError
			}

			// Set up mock common.FileSystem
			mockCommonFS := commontestutil.NewMockFileSystem()
			if tt.setupMock != nil {
				tt.setupMock(mockCommonFS)
			}

			// Create SafeFileManager with mocks
			manager := &SafeFileManager{safeFS: mockSafeFS, commonFS: mockCommonFS}

			// Execute
			err := manager.MoveToFinal(tt.tempPath, tt.finalPath, tt.perm)

			// Verify error handling
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			// AtomicMoveFile must receive the original (unresolved) paths:
			// OS-managed symlinks in parent directories (e.g. /tmp -> /private/tmp
			// on macOS) are its own to handle, not the caller's to pre-resolve.
			if tt.wantAtomicMoveCalled {
				require.Len(t, mockSafeFS.AtomicMoveFileCalls, 1)
				call := mockSafeFS.AtomicMoveFileCalls[0]
				assert.Equal(t, tt.tempPath, call.SrcPath)
				assert.Equal(t, tt.finalPath, call.DstPath)
				assert.Equal(t, tt.perm, call.RequiredPerm)
			} else {
				assert.Empty(t, mockSafeFS.AtomicMoveFileCalls)
			}
		})
	}
}

func TestSafeFileManager_EnsureDirectory_WithMock(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		setupMock func(*commontestutil.MockFileSystem)
		wantErr   error
	}{
		{
			name: "directory_already_exists",
			path: "/existing/dir",
			setupMock: func(mock *commontestutil.MockFileSystem) {
				require.NoError(t, mock.AddDir("/existing/dir", 0o755))
			},
			wantErr: nil,
		},
		{
			name: "create_new_directory",
			path: "/new/dir",
			setupMock: func(_ *commontestutil.MockFileSystem) {
				// Directory doesn't exist, MkdirAll should be called
			},
			wantErr: nil,
		},
		{
			name: "path_is_file_not_directory",
			path: "/path/to/file",
			setupMock: func(mock *commontestutil.MockFileSystem) {
				require.NoError(t, mock.AddFile("/path/to/file", 0o644, []byte("content")))
			},
			wantErr: ErrNotDirectory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock common.FileSystem
			mockCommonFS := commontestutil.NewMockFileSystem()
			if tt.setupMock != nil {
				tt.setupMock(mockCommonFS)
			}

			// Set up mock safefileio.FileSystem (not used in EnsureDirectory)
			mockSafeFS := safefileiotestutil.NewMockFileSystem()

			// Create SafeFileManager with mocks
			manager := &SafeFileManager{safeFS: mockSafeFS, commonFS: mockCommonFS}

			// Execute
			err := manager.EnsureDirectory(tt.path)

			// Verify
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSafeFileManager_RemoveTemp_WithMock(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		setupMock func(*commontestutil.MockFileSystem)
		wantErr   error
	}{
		{
			name: "file_exists_and_removed",
			path: "/tmp/test.tmp",
			setupMock: func(mock *commontestutil.MockFileSystem) {
				require.NoError(t, mock.AddFile("/tmp/test.tmp", 0o600, []byte("content")))
			},
			wantErr: nil,
		},
		{
			name: "file_does_not_exist_idempotent",
			path: "/tmp/nonexistent.tmp",
			setupMock: func(_ *commontestutil.MockFileSystem) {
				// File doesn't exist
			},
			wantErr: nil, // RemoveTemp should be idempotent
		},
		{
			name: "path_is_directory_error",
			path: "/tmp/dir",
			setupMock: func(mock *commontestutil.MockFileSystem) {
				require.NoError(t, mock.AddDir("/tmp/dir", 0o755))
			},
			wantErr: ErrNotFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock common.FileSystem
			mockCommonFS := commontestutil.NewMockFileSystem()
			if tt.setupMock != nil {
				tt.setupMock(mockCommonFS)
			}

			// Set up mock safefileio.FileSystem (not used in RemoveTemp)
			mockSafeFS := safefileiotestutil.NewMockFileSystem()

			// Create SafeFileManager with mocks
			manager := &SafeFileManager{safeFS: mockSafeFS, commonFS: mockCommonFS}

			// Execute
			err := manager.RemoveTemp(tt.path)

			// Verify
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSafeFileManager_CreateTempFile_WithMock(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		pattern   string
		setupMock func(*commontestutil.MockFileSystem)
		wantErr   error
	}{
		{
			name:    "create_temp_file_in_default_dir",
			dir:     "",
			pattern: "test_*.tmp",
			setupMock: func(_ *commontestutil.MockFileSystem) {
				// MockFileSystem.CreateTemp handles temp file creation
			},
			wantErr: nil,
		},
		{
			name:    "create_temp_file_in_specific_dir",
			dir:     "/custom/dir",
			pattern: "output_*.tmp",
			setupMock: func(mock *commontestutil.MockFileSystem) {
				require.NoError(t, mock.AddDir("/custom/dir", 0o755))
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock common.FileSystem
			mockCommonFS := commontestutil.NewMockFileSystem()
			if tt.setupMock != nil {
				tt.setupMock(mockCommonFS)
			}

			// Set up mock safefileio.FileSystem (not used in CreateTempFile)
			mockSafeFS := safefileiotestutil.NewMockFileSystem()

			// Create SafeFileManager with mocks
			manager := &SafeFileManager{safeFS: mockSafeFS, commonFS: mockCommonFS}

			// Execute
			file, err := manager.CreateTempFile(tt.dir, tt.pattern)

			// Verify
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, file)
				// Clean up
				if file != nil {
					file.Close()
				}
			}
		})
	}
}
