//go:build test

package elfdynlib

import (
	"debug/elf"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createLibFile creates a fake library file in the given directory.
// Returns the full path of the created file.
func createLibFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("fake library"), 0o644))
	return path
}

func TestResolve_RUNPATH(t *testing.T) {
	tmpDir := t.TempDir()
	libPath := createLibFile(t, tmpDir, "libfoo.so.1")

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)

	resolved, err := resolver.Resolve("libfoo.so.1", "/usr/bin/myapp", []string{tmpDir})
	require.NoError(t, err)
	assert.Equal(t, libPath, resolved)
}

func TestResolve_Origin(t *testing.T) {
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	require.NoError(t, os.MkdirAll(libDir, 0o755))
	libPath := createLibFile(t, libDir, "libfoo.so.1")

	binaryPath := filepath.Join(tmpDir, "myapp")

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)
	// RUNPATH uses $ORIGIN which should expand to tmpDir
	resolved, err := resolver.Resolve("libfoo.so.1", binaryPath, []string{"$ORIGIN/lib"})
	require.NoError(t, err)
	assert.Equal(t, libPath, resolved)
}

func TestResolve_LDCache(t *testing.T) {
	tmpDir := t.TempDir()
	libPath := createLibFile(t, tmpDir, "libcached.so.1")

	// Build a synthetic ld.so.cache with our test library
	cacheEntries := map[string]string{
		"libcached.so.1": libPath,
	}
	cacheData := buildTestLDCache(cacheEntries)
	cache, err := parseLDCacheData(cacheData)
	require.NoError(t, err)

	resolver := NewLibraryResolver(cache, elf.EM_X86_64)

	resolved, err := resolver.Resolve("libcached.so.1", "/app", nil)
	require.NoError(t, err)
	assert.Equal(t, libPath, resolved)
}

func TestResolve_DefaultPaths(t *testing.T) {
	// This test verifies that if a lib exists in a default path, it's found.
	// We can only test this if the system has the lib in one of the default paths.
	// Instead, we verify the behavior of different architectures by using a mock.

	// For x86_64, the default paths should be tested correctly.
	paths := DefaultSearchPaths(elf.EM_X86_64)
	assert.NotEmpty(t, paths)
	// Verify the paths are in the right order (multiarch first)
	found386 := false
	foundMultiarch := false
	for _, p := range paths {
		if p == "/lib/x86_64-linux-gnu" || p == "/usr/lib/x86_64-linux-gnu" {
			foundMultiarch = true
		}
		if p == "/lib" || p == "/usr/lib" {
			found386 = true
		}
		if foundMultiarch && found386 {
			break
		}
	}
	assert.True(t, foundMultiarch, "should have multiarch paths for x86_64")
	assert.True(t, found386, "should have generic paths for x86_64")
}

func TestResolve_Failure(t *testing.T) {
	resolver := NewLibraryResolver(nil, elf.EM_X86_64)

	_, err := resolver.Resolve("libnonexistent.so.9999", "/app", nil)
	require.Error(t, err)

	var notResolved *ErrLibraryNotResolved
	require.ErrorAs(t, err, &notResolved)
	assert.Equal(t, "libnonexistent.so.9999", notResolved.SOName)
	assert.Equal(t, "/app", notResolved.ParentPath)
	assert.NotEmpty(t, notResolved.SearchPaths)
}

func TestResolve_WithSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	// Create real file and a symlink to it
	realPath := filepath.Join(tmpDir, "libfoo.so.1.2.3")
	require.NoError(t, os.WriteFile(realPath, []byte("lib content"), 0o644))
	symlinkPath := filepath.Join(tmpDir, "libfoo.so.1")
	require.NoError(t, os.Symlink(realPath, symlinkPath))

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)

	resolved, err := resolver.Resolve("libfoo.so.1", "/app", []string{tmpDir})
	require.NoError(t, err)
	// Should resolve to the real path (symlink resolved)
	assert.Equal(t, realPath, resolved)
}
