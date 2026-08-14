//go:build test

package cmdcommon

import (
	"os"
	"path/filepath"
	"slices"
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

func TestCreateValidator_DefaultHashDirectory(t *testing.T) {
	// Use the default hash directory constant
	validator, err := CreateValidator(DefaultHashDirectory)

	// This may fail if the directory doesn't exist in test/CI environments.
	// We only verify that the function executes and returns appropriate results.
	if err != nil {
		// If there's an error, it should be because the directory doesn't exist
		require.Error(t, err, "CreateValidator should return an error if directory doesn't exist")
		require.Nil(t, validator, "validator should be nil when creation fails")
	} else {
		// If no error, validator should be valid
		require.NotNil(t, validator, "validator should not be nil when creation succeeds")
	}
}

func TestCreateValidator_NonExistentDirectory(t *testing.T) {
	// Use a non-existent directory path inside a writable temp dir
	nonExistentDir := filepath.Join(t.TempDir(), "nonexistent", "hash", "dir")

	validator, err := CreateValidator(nonExistentDir)

	// New() now automatically creates the hash directory, so this should succeed.
	require.NoError(t, err, "CreateValidator should auto-create the directory and not return an error")
	require.NotNil(t, validator, "validator should be non-nil when directory is auto-created")

	// Verify the directory was actually created
	_, statErr := os.Stat(nonExistentDir)
	assert.NoError(t, statErr, "hash directory should have been created automatically")
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

func TestCreateValidator_EmptyPath(t *testing.T) {
	// Empty path should return an error.
	validator, err := CreateValidator("")

	require.Error(t, err)
	require.Nil(t, validator)
}

func TestDefaultHashDirectory_IsSet(t *testing.T) {
	// Verify that DefaultHashDirectory is set to a non-empty value
	assert.NotEmpty(t, DefaultHashDirectory, "DefaultHashDirectory should be set")
	assert.Equal(t, "/usr/local/etc/go-safe-cmd-runner/hashes", DefaultHashDirectory)
}

func TestNewDirectoryPermChecker_ConstructsChecker(t *testing.T) {
	checker, err := NewDirectoryPermChecker()
	require.NoError(t, err, "NewDirectoryPermChecker should succeed")
	require.NotNil(t, checker, "NewDirectoryPermChecker should return a non-nil checker")
}

// TestCreateReadOnlyValidator_DoesNotCreateHashDirectory tests that
// CreateReadOnlyValidator builds successfully for a hash directory that does
// not exist yet, without creating it: the whole parent subtree, compared by
// path and mode, must be identical before and after construction.
func TestCreateReadOnlyValidator_DoesNotCreateHashDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "nested", "hashes")

	before := walkEntries(t, tmpDir)

	validator, err := CreateReadOnlyValidator(hashDir)
	require.NoError(t, err, "CreateReadOnlyValidator should succeed for a missing hash directory")
	require.NotNil(t, validator)

	after := walkEntries(t, tmpDir)
	assert.Equal(t, before, after, "parent subtree must be unchanged by CreateReadOnlyValidator")
}

// TestCreateReadOnlyValidator_ExistingHashDirectoryHasNoDeferredError tests
// that a validator built for an existing hash directory carries no deferred
// hash-directory error.
func TestCreateReadOnlyValidator_ExistingHashDirectoryHasNoDeferredError(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	validator, err := CreateReadOnlyValidator(hashDir)
	require.NoError(t, err)
	require.NotNil(t, validator)

	assert.NoError(t, validator.HashDirError())
}

// walkEntries returns the sorted relative paths, entry types, and modes under
// root, so two runs can be compared to prove no filesystem entry was added or
// had its type or mode changed. The entry type comes from DirEntry.Type so a
// symlink is reported as one even when d.Info would follow it.
func walkEntries(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		entries = append(entries, rel+"/"+d.Type().String()+"/"+info.Mode().String())
		return nil
	})
	require.NoError(t, err, "failed to walk parent subtree")
	slices.Sort(entries)
	return entries
}
