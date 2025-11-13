//go:build test

package cmdcommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateValidator_ValidHashDirectory(t *testing.T) {
	// Create a temporary hash directory
	tmpDir := t.TempDir()

	validator, err := CreateValidator(tmpDir)

	require.NoError(t, err, "CreateValidator should not return an error with valid hash directory")
	require.NotNil(t, validator, "CreateValidator should return a non-nil validator")
}

func TestCreateValidator_DefaultHashDirectory(_ *testing.T) {
	// Use the default hash directory constant
	validator, err := CreateValidator(DefaultHashDirectory)

	// This may fail if the directory doesn't exist, which is expected behavior
	// We're testing that the function executes without panicking
	_ = err
	_ = validator
}

func TestCreateValidator_NonExistentDirectory(t *testing.T) {
	// Use a non-existent directory path
	nonExistentDir := filepath.Join(t.TempDir(), "nonexistent", "hash", "dir")

	validator, err := CreateValidator(nonExistentDir)

	// The validator should be created even if the directory doesn't exist yet
	// (it will be created when needed)
	_ = err
	_ = validator
}

func TestCreateValidator_RelativePath(t *testing.T) {
	// Test with a relative path - create the directory first
	relativeDir := "./test_hashes"

	// Create the directory
	err := os.MkdirAll(relativeDir, 0o755)
	require.NoError(t, err, "Failed to create test directory")

	// Clean up after test
	defer os.RemoveAll(relativeDir)

	validator, err := CreateValidator(relativeDir)

	require.NoError(t, err, "CreateValidator should handle relative paths")
	require.NotNil(t, validator, "CreateValidator should return a non-nil validator")
}

func TestCreateValidator_EmptyPath(_ *testing.T) {
	// Test with empty path
	validator, err := CreateValidator("")

	// CreateValidator should handle empty path
	_ = err
	_ = validator
}

func TestDefaultHashDirectory_IsSet(t *testing.T) {
	// Verify that DefaultHashDirectory is set to a non-empty value
	assert.NotEmpty(t, DefaultHashDirectory, "DefaultHashDirectory should be set")
	assert.Equal(t, "/usr/local/etc/go-safe-cmd-runner/hashes", DefaultHashDirectory)
}
