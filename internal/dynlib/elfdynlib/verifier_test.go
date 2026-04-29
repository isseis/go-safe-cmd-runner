//go:build test

package elfdynlib

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestVerifier creates a DynLibVerifier for testing using the real filesystem.
func newTestVerifier() *DynLibVerifier {
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	return NewDynLibVerifier(fs)
}

// writeFile creates a file at path with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// TestVerify_NilDeps ensures Verify returns nil immediately for nil DynLibDeps.
func TestVerify_NilDeps(t *testing.T) {
	v := newTestVerifier()
	err := v.Verify(nil)
	assert.NoError(t, err)
}

// TestVerify_EmptyLibs ensures Verify returns nil when DynLibDeps is empty.
func TestVerify_EmptyLibs(t *testing.T) {
	v := newTestVerifier()
	err := v.Verify([]fileanalysis.LibEntry{})
	assert.NoError(t, err)
}

// TestVerify_EmptyPath ensures dynlib.ErrEmptyLibraryPath is returned when an entry has an empty Path.
func TestVerify_EmptyPath(t *testing.T) {
	v := newTestVerifier()

	deps := []fileanalysis.LibEntry{
		{
			SOName: "libtest.so",
			Path:   "",
			Hash:   "sha256:aabbccdd",
		},
	}

	err := v.Verify(deps)
	require.Error(t, err)
	var errEmpty *dynlib.ErrEmptyLibraryPath
	assert.ErrorAs(t, err, &errEmpty, "expected dynlib.ErrEmptyLibraryPath")
	assert.Equal(t, "libtest.so", errEmpty.SOName)
}

// TestVerify_HashMatch ensures Verify returns nil when all library hashes match.
func TestVerify_HashMatch(t *testing.T) {
	v := newTestVerifier()
	tmpDir := t.TempDir()

	libPath := filepath.Join(tmpDir, "libfoo.so")
	writeFile(t, libPath, "fake library content for hash test")

	actualHash, err := computeFileHash(safefileio.NewFileSystem(safefileio.FileSystemConfig{}), libPath)
	require.NoError(t, err)

	deps := []fileanalysis.LibEntry{
		{
			SOName: "libfoo.so",
			Path:   libPath,
			Hash:   actualHash,
		},
	}

	err = v.Verify(deps)
	assert.NoError(t, err)
}

// TestVerify_HashMismatch ensures dynlib.ErrLibraryHashMismatch is returned
// when a recorded hash does not match the current file content.
func TestVerify_HashMismatch(t *testing.T) {
	v := newTestVerifier()
	tmpDir := t.TempDir()

	libPath := filepath.Join(tmpDir, "libbar.so")
	writeFile(t, libPath, "original content")

	const wrongHash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

	deps := []fileanalysis.LibEntry{
		{
			SOName: "libbar.so",
			Path:   libPath,
			Hash:   wrongHash,
		},
	}

	err := v.Verify(deps)
	require.Error(t, err)

	var hashErr *dynlib.ErrLibraryHashMismatch
	require.ErrorAs(t, err, &hashErr, "expected dynlib.ErrLibraryHashMismatch")
	assert.Equal(t, "libbar.so", hashErr.SOName)
	assert.Equal(t, libPath, hashErr.Path)
	assert.Equal(t, wrongHash, hashErr.ExpectedHash)
	assert.NotEmpty(t, hashErr.ActualHash)
	assert.NotEqual(t, wrongHash, hashErr.ActualHash)
}

// TestVerify_FileNotFound ensures an error is returned when the library file does not exist.
func TestVerify_FileNotFound(t *testing.T) {
	v := newTestVerifier()

	deps := []fileanalysis.LibEntry{
		{
			SOName: "libmissing.so",
			Path:   "/nonexistent/libmissing.so",
			Hash:   "sha256:abc123",
		},
	}

	err := v.Verify(deps)
	require.Error(t, err)
	// Must NOT be a hash mismatch — file could not be read at all.
	_, ok := errors.AsType[*dynlib.ErrLibraryHashMismatch](err)
	assert.False(t, ok)
}
