package logging

import (
	"os"
	"path/filepath"
	"testing"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSafeFileOpener_Success(t *testing.T) {
	opener := NewSafeFileOpener()

	require.NotNil(t, opener)
	assert.NotNil(t, opener.fs)
}

func TestOpenFile_Success(t *testing.T) {
	tempDir := tu.SafeTempDir(t)
	opener := NewSafeFileOpener()

	tests := []struct {
		name     string
		filepath string
		flag     int
		perm     os.FileMode
	}{
		{
			name:     "create new file",
			filepath: filepath.Join(tempDir, "test1.log"),
			flag:     os.O_CREATE | os.O_WRONLY | os.O_TRUNC,
			perm:     0o600,
		},
		{
			name:     "create file in subdirectory",
			filepath: filepath.Join(tempDir, "subdir", "test2.log"),
			flag:     os.O_CREATE | os.O_WRONLY | os.O_TRUNC,
			perm:     0o600,
		},
		{
			name:     "create file with different permissions",
			filepath: filepath.Join(tempDir, "test3.log"),
			flag:     os.O_CREATE | os.O_WRONLY | os.O_TRUNC,
			perm:     0o644,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := opener.OpenFile(tt.filepath, tt.flag, tt.perm)
			require.NoError(t, err)
			defer file.Close()

			assert.NotNil(t, file)

			// Verify file was created
			_, err = os.Stat(tt.filepath)
			assert.NoError(t, err)

			// Write some data to verify the file is writable
			_, err = file.Write([]byte("test data\n"))
			assert.NoError(t, err)
		})
	}
}

func TestOpenFile_PermissionDenied(t *testing.T) {
	// Skip if running as root (no permission errors)
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir := tu.SafeTempDir(t)
	readOnlyDir := filepath.Join(tempDir, "readonly")

	// Create a read-only directory
	err := os.Mkdir(readOnlyDir, 0o444)
	require.NoError(t, err)
	defer os.Chmod(readOnlyDir, 0o755) // Restore permissions for cleanup

	opener := NewSafeFileOpener()
	filepath := filepath.Join(readOnlyDir, "test.log")

	file, err := opener.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)

	if err == nil {
		if file != nil {
			file.Close()
		}
		assert.Error(t, err, "OpenFile() expected error for read-only directory")
	}
}

func TestOpenFile_SymlinkAttack(t *testing.T) {
	tempDir := tu.SafeTempDir(t)
	opener := NewSafeFileOpener()

	// Create a target file
	targetFile := filepath.Join(tempDir, "target.txt")
	err := os.WriteFile(targetFile, []byte("original"), 0o644)
	require.NoError(t, err)

	// Create a symlink
	symlinkPath := filepath.Join(tempDir, "symlink.log")
	err = os.Symlink(targetFile, symlinkPath)
	require.NoError(t, err)

	// Try to open the symlink - should be rejected by safefileio
	file, err := opener.OpenFile(symlinkPath, os.O_WRONLY, 0o600)

	if err == nil {
		if file != nil {
			file.Close()
		}
		assert.Error(t, err, "OpenFile() should reject symlinks")
	}
}

func TestValidateLogDir_Valid(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T) string
		wantErr   bool
	}{
		{
			name:      "existing writable directory",
			setupFunc: tu.SafeTempDir,
			wantErr:   false,
		},
		{
			name: "non-existing directory that can be created",
			setupFunc: func(t *testing.T) string {
				return filepath.Join(tu.SafeTempDir(t), "newdir")
			},
			wantErr: false,
		},
		{
			name: "nested directory that can be created",
			setupFunc: func(t *testing.T) string {
				return filepath.Join(tu.SafeTempDir(t), "a", "b", "c")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setupFunc(t)
			err := ValidateLogDir(dir)

			assert.Equal(t, tt.wantErr, err != nil)

			// Verify directory was created
			if err == nil {
				_, err := os.Stat(dir)
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateLogDir_NotExist(t *testing.T) {
	// This test is actually covered by TestValidateLogDir_Valid
	// because ValidateLogDir creates the directory if it doesn't exist

	// Test case: directory that cannot be created due to invalid path
	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{
			name:    "empty directory path",
			dir:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogDir(tt.dir)

			assert.Equal(t, tt.wantErr, err != nil)

			if err != nil && err != ErrEmptyLogDirectory {
				t.Logf("ValidateLogDir() error = %v", err)
			}
		})
	}
}

func TestValidateLogDir_NotWritable(t *testing.T) {
	// Skip if running as root (no permission errors)
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir := tu.SafeTempDir(t)
	readOnlyDir := filepath.Join(tempDir, "readonly")

	// Create a read-only directory
	err := os.Mkdir(readOnlyDir, 0o444)
	require.NoError(t, err)
	defer os.Chmod(readOnlyDir, 0o755) // Restore permissions for cleanup

	err = ValidateLogDir(readOnlyDir)

	assert.Error(t, err, "ValidateLogDir() expected error for read-only directory")
}
