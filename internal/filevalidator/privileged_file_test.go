package filevalidator

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			setup: func(_ *testing.T) string {
				return "/tmp/non_existent_file"
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filepath := tt.setup(t)
			if filepath != "/tmp/non_existent_file" {
				defer os.Remove(filepath)
			}

			file, err := OpenFileWithPrivileges(filepath)
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

func TestNeedsPrivileges(t *testing.T) {
	// 通常のファイル（権限不要）
	testFile := createTestFile(t, "test content")
	defer os.Remove(testFile)

	result := needsPrivileges(testFile)
	assert.False(t, result, "normal file should not need privileges")

	// 存在しないファイル
	result = needsPrivileges("/tmp/non_existent_file")
	assert.False(t, result, "non-existent file should not need privileges")
}

func TestIsPrivilegeError(t *testing.T) {
	// PrivilegeError の場合
	privErr := &PrivilegeError{
		Operation: "escalate",
		UID:       0,
		Cause:     os.ErrPermission,
	}
	assert.True(t, IsPrivilegeError(privErr))

	// 通常のエラーの場合
	normalErr := os.ErrNotExist
	assert.False(t, IsPrivilegeError(normalErr))
}

// テストヘルパー関数
func createTestFile(t *testing.T, content string) string {
	tmpFile, err := os.CreateTemp("", "test_file_*.txt")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}
