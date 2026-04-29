//go:build test

package machodylib

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTempLib creates a temp file to simulate a resolved library path.
// Returns the path and a cleanup function.
func createTempLib(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	require.NoError(t, err)
	_ = f.Close()

	return path
}

// resolvePath resolves symlinks and cleans the path, matching tryResolve behavior.
func resolvePath(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)

	return filepath.Clean(resolved)
}

func TestResolve_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	libPath := createTempLib(t, dir, "libfoo.dylib")

	r := NewLibraryResolver(dir)
	resolved, err := r.Resolve(libPath, "/tmp/loader", nil)
	require.NoError(t, err)
	assert.Equal(t, resolvePath(t, libPath), resolved)
}

func TestResolve_AbsolutePath_NotFound(t *testing.T) {
	r := NewLibraryResolver("/tmp")
	_, err := r.Resolve("/nonexistent/libfoo.dylib", "/tmp/loader", nil)

	var notResolved *ErrLibraryNotResolved
	assert.ErrorAs(t, err, &notResolved)
	assert.Equal(t, "/nonexistent/libfoo.dylib", notResolved.InstallName)
}

func TestResolve_ExecutablePath(t *testing.T) {
	dir := t.TempDir()
	libPath := createTempLib(t, dir, "libfoo.dylib")

	r := NewLibraryResolver(dir)
	resolved, err := r.Resolve("@executable_path/libfoo.dylib", "/tmp/loader", nil)
	require.NoError(t, err)
	assert.Equal(t, resolvePath(t, libPath), resolved)
}

func TestResolve_LoaderPath(t *testing.T) {
	dir := t.TempDir()
	libPath := createTempLib(t, dir, "libbar.dylib")

	r := NewLibraryResolver("/other/dir")
	loaderPath := filepath.Join(dir, "binary")
	resolved, err := r.Resolve("@loader_path/libbar.dylib", loaderPath, nil)
	require.NoError(t, err)
	assert.Equal(t, resolvePath(t, libPath), resolved)
}

func TestResolve_Rpath(t *testing.T) {
	dir := t.TempDir()
	libPath := createTempLib(t, dir, "libbaz.dylib")

	r := NewLibraryResolver("/other/dir")
	rpaths := []string{"/nonexistent", dir}
	resolved, err := r.Resolve("@rpath/libbaz.dylib", "/tmp/loader", rpaths)
	require.NoError(t, err)
	assert.Equal(t, resolvePath(t, libPath), resolved)
}

func TestResolve_Rpath_ExecutablePathExpansion(t *testing.T) {
	dir := t.TempDir()
	libPath := createTempLib(t, dir, "libqux.dylib")

	r := NewLibraryResolver(dir)
	// LC_RPATH entry uses @executable_path
	rpaths := []string{"@executable_path"}
	resolved, err := r.Resolve("@rpath/libqux.dylib", "/tmp/loader", rpaths)
	require.NoError(t, err)
	assert.Equal(t, resolvePath(t, libPath), resolved)
}

func TestResolve_Rpath_LoaderPathExpansion(t *testing.T) {
	dir := t.TempDir()
	libPath := createTempLib(t, dir, "libqux2.dylib")

	r := NewLibraryResolver("/other/dir")
	loaderPath := filepath.Join(dir, "binary")
	// LC_RPATH entry uses @loader_path
	rpaths := []string{"@loader_path"}
	resolved, err := r.Resolve("@rpath/libqux2.dylib", loaderPath, rpaths)
	require.NoError(t, err)
	assert.Equal(t, resolvePath(t, libPath), resolved)
}

func TestResolve_DefaultSearchPath(t *testing.T) {
	// Test that resolution fails with appropriate error when lib is not in default paths.
	r := NewLibraryResolver("/tmp")
	_, err := r.Resolve("libnonexistent_xyz.dylib", "/tmp/loader", nil)

	var notResolved *ErrLibraryNotResolved
	require.ErrorAs(t, err, &notResolved)
	assert.Equal(t, "libnonexistent_xyz.dylib", notResolved.InstallName)
	// Should have tried /usr/local/lib/... and /usr/lib/...
	assert.GreaterOrEqual(t, len(notResolved.Tried), 1)
}

func TestResolve_UnknownAtToken(t *testing.T) {
	r := NewLibraryResolver("/tmp")
	_, err := r.Resolve("@unknown_token/libfoo.dylib", "/tmp/loader", nil)

	var unknownErr *ErrUnknownAtToken
	require.ErrorAs(t, err, &unknownErr)
	assert.Equal(t, "@unknown_token/libfoo.dylib", unknownErr.InstallName)
	assert.Equal(t, "@unknown_token", unknownErr.Token)
}

func TestResolve_Rpath_NotFound(t *testing.T) {
	r := NewLibraryResolver("/tmp")
	rpaths := []string{"/nonexistent1", "/nonexistent2"}
	_, err := r.Resolve("@rpath/libmissing.dylib", "/tmp/loader", rpaths)

	var notResolved *ErrLibraryNotResolved
	require.ErrorAs(t, err, &notResolved)
	assert.Equal(t, "@rpath/libmissing.dylib", notResolved.InstallName)
	assert.Len(t, notResolved.Tried, 2)
}
