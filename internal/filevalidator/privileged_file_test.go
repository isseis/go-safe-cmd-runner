package filevalidator

import (
	"os"
	"path/filepath"
	"testing"

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

func TestIsPrivilegeError(t *testing.T) {
	// For privilege-related errors
	privErr := runnertypes.ErrPrivilegedExecutionNotAvailable
	assert.True(t, IsPrivilegeError(privErr))

	// For normal errors
	normalErr := os.ErrNotExist
	assert.False(t, IsPrivilegeError(normalErr))
}
