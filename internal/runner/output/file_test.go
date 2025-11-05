package output

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeFileManager_CreateTempFile(t *testing.T) {
	tests := []struct {
		name       string
		dir        string
		pattern    string
		wantErr    bool
		errMessage string
	}{
		{
			name:    "valid_temp_file_creation",
			dir:     "", // Will use default temp dir
			pattern: "output_*.tmp",
			wantErr: false,
		},
		{
			name:    "valid_temp_file_with_specific_dir",
			pattern: "test_*.tmp",
			wantErr: false,
		},
	}

	manager := NewSafeFileManager()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test directory if needed
			var testDir string
			if tt.name == "valid_temp_file_with_specific_dir" {
				testDir = t.TempDir()
				tt.dir = testDir
			}

			file, err := manager.CreateTempFile(tt.dir, tt.pattern)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
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
		name       string
		data       []byte
		wantN      int
		wantErr    bool
		errMessage string
	}{
		{
			name:    "write_valid_data",
			data:    []byte("test data\n"),
			wantN:   10,
			wantErr: false,
		},
		{
			name:    "write_empty_data",
			data:    []byte{},
			wantN:   0,
			wantErr: false,
		},
		{
			name:    "write_binary_data",
			data:    []byte{0x00, 0x01, 0x02, 0xFF},
			wantN:   4,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := manager.WriteToTemp(tempFile, tt.data)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
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
		wantErr             bool
		errMessage          string
		skipPermissionCheck bool
	}{
		{
			name: "create_new_directory",
			setupPath: func(t *testing.T) string {
				tempDir := t.TempDir()
				return filepath.Join(tempDir, "new_dir")
			},
			wantErr: false,
		},
		{
			name: "existing_directory",
			setupPath: func(t *testing.T) string {
				tempDir := t.TempDir()
				newDir := filepath.Join(tempDir, "existing")
				err := os.MkdirAll(newDir, 0o755)
				require.NoError(t, err)
				return newDir
			},
			wantErr:             false,
			skipPermissionCheck: true, // Existing directory has different permissions
		},
		{
			name: "nested_directory_creation",
			setupPath: func(t *testing.T) string {
				tempDir := t.TempDir()
				return filepath.Join(tempDir, "level1", "level2", "level3")
			},
			wantErr: false,
		},
		{
			name: "path_on_existing_file",
			setupPath: func(t *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "existing_file")
				file, err := os.Create(filePath)
				require.NoError(t, err)
				file.Close()
				return filePath
			},
			wantErr:    true,
			errMessage: "not a directory",
		},
	}

	manager := NewSafeFileManager()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupPath(t)
			err := manager.EnsureDirectory(path)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
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
		wantErr    bool
		errMessage string
	}{
		{
			name: "move_existing_temp_file",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()

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
			wantErr: false,
		},
		{
			name: "move_to_existing_file_overwrite",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()

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
			wantErr: false,
		},
		{
			name: "move_nonexistent_temp_file",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				tempPath := filepath.Join(tempDir, "nonexistent.tmp")
				finalPath := filepath.Join(tempDir, "final.txt")
				return tempPath, finalPath
			},
			wantErr:    true,
			errMessage: "no such file",
		},
		{
			name: "move_to_directory_instead_of_file",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()

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
			wantErr:    true,
			errMessage: "directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempPath, finalPath := tt.setupFiles(t)

			// Store original content if temp file exists
			var originalContent []byte
			if _, err := os.Stat(tempPath); err == nil {
				originalContent, _ = os.ReadFile(tempPath)
			}

			err := manager.MoveToFinal(tempPath, finalPath)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
			} else {
				assert.NoError(t, err)

				// Verify temp file was moved (no longer exists)
				_, err = os.Stat(tempPath)
				assert.True(t, os.IsNotExist(err), "temp file should be removed")

				// Verify final file exists with correct content
				finalContent, err := os.ReadFile(finalPath)
				require.NoError(t, err)
				assert.Equal(t, originalContent, finalContent)

				// Verify final file has secure permissions
				// Note: SafeAtomicMoveFile enforces 0600 permissions for maximum security
				stat, err := os.Stat(finalPath)
				require.NoError(t, err)
				actualPerm := stat.Mode().Perm()
				// SafeAtomicMoveFile enforces 0600 for write operations for security
				expectedPerm := os.FileMode(0o600)
				assert.Equal(t, expectedPerm, actualPerm)
			}
		})
	}
}

func TestSafeFileManager_RemoveTemp(t *testing.T) {
	manager := NewSafeFileManager()

	tests := []struct {
		name       string
		setupPath  func(t *testing.T) string
		wantErr    bool
		errMessage string
	}{
		{
			name: "remove_existing_temp_file",
			setupPath: func(t *testing.T) string {
				tempFile, err := manager.CreateTempFile("", "remove_test_*.tmp")
				require.NoError(t, err)
				tempFile.Close()
				return tempFile.Name()
			},
			wantErr: false,
		},
		{
			name: "remove_nonexistent_file",
			setupPath: func(t *testing.T) string {
				tempDir := t.TempDir()
				return filepath.Join(tempDir, "nonexistent.tmp")
			},
			wantErr: false, // RemoveTemp should be idempotent
		},
		{
			name: "remove_directory_instead_of_file",
			setupPath: func(t *testing.T) string {
				tempDir := t.TempDir()
				dirPath := filepath.Join(tempDir, "test_dir")
				err := os.MkdirAll(dirPath, 0o755)
				require.NoError(t, err)
				return dirPath
			},
			wantErr:    true,
			errMessage: "directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupPath(t)
			err := manager.RemoveTemp(path)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
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
	tempDir := t.TempDir()

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
	err = manager.MoveToFinal(tempPath, finalPath)
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
	tempDir := t.TempDir()
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
	err = manager.MoveToFinal(tempPath, finalPath)
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
	// SafeAtomicMoveFile enforces 0600 for write operations for security
	assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm())
}
