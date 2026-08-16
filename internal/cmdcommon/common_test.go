//go:build test

package cmdcommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
)

func TestDefaultHashDirectory_IsSet(t *testing.T) {
	// Verify that DefaultHashDirectory is set to a non-empty value
	assert.NotEmpty(t, DefaultHashDirectory, "DefaultHashDirectory should be set")
	assert.Equal(t, "/usr/local/etc/go-safe-cmd-runner/hashes", DefaultHashDirectory)
}

// TestCreateReadOnlyValidator_DoesNotCreateHashDirectory tests that
// CreateReadOnlyValidator builds successfully for a hash directory that does
// not exist yet, without creating it: the whole parent subtree, compared by
// path and mode, must be identical before and after construction.
func TestCreateReadOnlyValidator_DoesNotCreateHashDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "nested", "hashes")

	before := tu.WalkEntries(t, tmpDir)

	validator, err := CreateReadOnlyValidator(hashDir)
	require.NoError(t, err, "CreateReadOnlyValidator should succeed for a missing hash directory")
	require.NotNil(t, validator)

	after := tu.WalkEntries(t, tmpDir)
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

// TestCreateReadOnlyValidator_RelativePath tests that a relative hash directory
// is accepted and resolved against the working directory.
func TestCreateReadOnlyValidator_RelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	const relativeDir = "test_hashes"
	require.NoError(t, os.Mkdir(relativeDir, 0o700))

	validator, err := CreateReadOnlyValidator(relativeDir)

	require.NoError(t, err, "CreateReadOnlyValidator should handle relative paths")
	require.NotNil(t, validator)
	assert.NoError(t, validator.HashDirError())
}

// TestCreateReadOnlyValidator_EmptyPath tests that an empty hash directory is
// reported as an unusable directory rather than being repaired: construction
// succeeds, and the deferred error says the directory does not exist, so verify
// stops before it inspects any file.
func TestCreateReadOnlyValidator_EmptyPath(t *testing.T) {
	validator, err := CreateReadOnlyValidator("")

	require.NoError(t, err)
	require.NotNil(t, validator)
	assert.ErrorIs(t, validator.HashDirError(), filevalidator.ErrHashDirNotExist)
}
