package filevalidator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	privtesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	safefileiotesting "github.com/isseis/go-safe-cmd-runner/internal/safefileio/testutil"
	"github.com/stretchr/testify/assert"
)

func TestOpenFileWithPrivileges(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		expectError bool
	}{
		{
			name: "normal file access",
			setup: func(t *testing.T) string {
				return createTestFile(t, "test content")
			},
			expectError: false,
		},
		{
			name: "non-existent file",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "non_existent_file")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filepath := tt.setup(t)
			// Create a PrivilegedFileValidator for testing
			pfv := NewPrivilegedFileValidator(nil) // nil uses default FileSystem
			file, err := pfv.OpenFileWithPrivileges(filepath, nil)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, file)
				if file != nil {
					file.Close()
				}
			}
		})
	}
}

func TestOpenFileWithPrivileges_PermissionError(t *testing.T) {
	// This test simulates a permission error scenario using a mock FileSystem
	t.Run("permission denied without privilege manager", func(t *testing.T) {
		// Create a mock FileSystem that returns a permission error
		mockFS := &safefileiotesting.MockFileSystem{
			SafeOpenFileFunc: func(_ string, _ int, _ os.FileMode) (safefileio.File, error) {
				return nil, os.ErrPermission
			},
		}

		pfv := NewPrivilegedFileValidator(mockFS)
		file, err := pfv.OpenFileWithPrivileges("/some/restricted/file", nil)
		assert.Error(t, err)
		assert.Nil(t, file)
		assert.ErrorIs(t, err, runnertypes.ErrPrivilegedExecutionNotAvailable)
	})
}

func TestOpenFileWithPrivileges_WithPrivilegeManager(t *testing.T) {
	testFile := createTestFile(t, "test content")

	t.Run("privilege manager not supported", func(t *testing.T) {
		// Create a mock FileSystem that returns a permission error
		mockFS := &safefileiotesting.MockFileSystem{
			SafeOpenFileFunc: func(_ string, _ int, _ os.FileMode) (safefileio.File, error) {
				return nil, os.ErrPermission
			},
		}

		mockPM := privtesting.NewMockPrivilegeManager(false)
		pfv := NewPrivilegedFileValidator(mockFS)
		file, err := pfv.OpenFileWithPrivileges("/some/restricted/file", mockPM)
		assert.Error(t, err)
		assert.Nil(t, file)
		assert.ErrorIs(t, err, privilege.ErrPrivilegedExecutionNotSupported)
	})

	t.Run("privilege manager supported but execution fails", func(t *testing.T) {
		// Create a mock FileSystem that returns a permission error
		mockFS := &safefileiotesting.MockFileSystem{
			SafeOpenFileFunc: func(_ string, _ int, _ os.FileMode) (safefileio.File, error) {
				return nil, os.ErrPermission
			},
		}

		mockPM := privtesting.NewFailingMockPrivilegeManager(true)
		pfv := NewPrivilegedFileValidator(mockFS)
		file, err := pfv.OpenFileWithPrivileges("/some/restricted/file", mockPM)
		assert.Error(t, err)
		assert.Nil(t, file)
		assert.ErrorIs(t, err, privtesting.ErrMockPrivilegeElevationFailed)
	})

	t.Run("privilege manager supported and execution succeeds", func(t *testing.T) {
		mockPM := privtesting.NewMockPrivilegeManager(true)
		pfv := NewPrivilegedFileValidator(nil) // Use default FileSystem
		file, err := pfv.OpenFileWithPrivileges(testFile, mockPM)
		assert.NoError(t, err)
		assert.NotNil(t, file)
		if file != nil {
			file.Close()
		}
	})
}
