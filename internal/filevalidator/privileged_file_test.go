package filevalidator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
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
			// Create a mock privilege manager for testing
			file, err := OpenFileWithPrivileges(filepath, nil) // nil for normal file access test
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
	// This test simulates a permission error scenario
	t.Run("permission denied without privilege manager", func(t *testing.T) {
		// Create a file that we can't access (simulated by using non-existent directory)
		restrictedPath := "/root/restricted_file.txt"
		file, err := OpenFileWithPrivileges(restrictedPath, nil)
		assert.Error(t, err)
		assert.Nil(t, file)
		assert.True(t, errors.Is(err, runnertypes.ErrPrivilegedExecutionNotAvailable))
	})
}

func TestOpenFileWithPrivileges_WithPrivilegeManager(t *testing.T) {
	testFile := createTestFile(t, "test content")

	t.Run("privilege manager not supported", func(t *testing.T) {
		mockPM := &MockPrivilegeManager{supported: false}
		// Simulate permission error by trying to access non-existent path
		file, err := OpenFileWithPrivileges("/root/restricted", mockPM)
		assert.Error(t, err)
		assert.Nil(t, file)
		assert.True(t, errors.Is(err, privilege.ErrPrivilegedExecutionNotSupported))
	})

	t.Run("privilege manager supported but execution fails", func(t *testing.T) {
		mockPM := &MockPrivilegeManager{supported: true, shouldErr: true}
		// Simulate permission error
		file, err := OpenFileWithPrivileges("/root/restricted", mockPM)
		assert.Error(t, err)
		assert.Nil(t, file)
		assert.Contains(t, err.Error(), "failed to open file")
	})

	t.Run("privilege manager supported and execution succeeds", func(t *testing.T) {
		mockPM := &MockPrivilegeManager{supported: true, shouldErr: false}
		file, err := OpenFileWithPrivileges(testFile, mockPM)
		assert.NoError(t, err)
		assert.NotNil(t, file)
		if file != nil {
			file.Close()
		}
	})
}

func TestIsPrivilegeError(t *testing.T) {
	// For privilege-related errors
	privErr := runnertypes.ErrPrivilegedExecutionNotAvailable
	assert.True(t, IsPrivilegeError(privErr))

	// For normal errors
	normalErr := os.ErrNotExist
	assert.False(t, IsPrivilegeError(normalErr))
}
