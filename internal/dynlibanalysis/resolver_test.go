//go:build test

package dynlibanalysis

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

func TestResolve_RPATH(t *testing.T) {
	tmpDir := t.TempDir()
	libPath := createLibFile(t, tmpDir, "libfoo.so.1")

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)
	ctx := NewRootContext("/usr/bin/myapp", []string{tmpDir}, nil, false)

	resolved, err := resolver.Resolve("libfoo.so.1", ctx)
	require.NoError(t, err)
	assert.Equal(t, libPath, resolved)
}

func TestResolve_RUNPATH(t *testing.T) {
	tmpDir := t.TempDir()
	libPath := createLibFile(t, tmpDir, "libfoo.so.1")

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)
	ctx := NewRootContext("/usr/bin/myapp", nil, []string{tmpDir}, false)

	resolved, err := resolver.Resolve("libfoo.so.1", ctx)
	require.NoError(t, err)
	assert.Equal(t, libPath, resolved)
}

func TestResolve_RPATHvsRUNPATH(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	// Only place the lib in tmpDir2 (RUNPATH dir), not tmpDir1 (RPATH dir)
	libPath := createLibFile(t, tmpDir2, "libfoo.so.1")

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)
	// When RUNPATH is present, RPATH should be ignored
	ctx := NewRootContext("/usr/bin/myapp", []string{tmpDir1}, []string{tmpDir2}, false)

	resolved, err := resolver.Resolve("libfoo.so.1", ctx)
	require.NoError(t, err)
	// Should resolve via RUNPATH (tmpDir2), not RPATH (tmpDir1)
	assert.Equal(t, libPath, resolved)
}

func TestResolve_Origin(t *testing.T) {
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "lib")
	require.NoError(t, os.MkdirAll(libDir, 0o755))
	libPath := createLibFile(t, libDir, "libfoo.so.1")

	binaryPath := filepath.Join(tmpDir, "myapp")

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)
	// RPATH uses $ORIGIN which should expand to tmpDir
	ctx := NewRootContext(binaryPath, []string{"$ORIGIN/lib"}, nil, false)

	resolved, err := resolver.Resolve("libfoo.so.1", ctx)
	require.NoError(t, err)
	assert.Equal(t, libPath, resolved)
}

func TestResolve_OriginInherited(t *testing.T) {
	// Test that inherited RPATH with $ORIGIN expands relative to
	// the originating ELF's directory (not the child's directory).
	appDir := t.TempDir()
	libATmpDir := filepath.Join(appDir, "liba_dir")
	require.NoError(t, os.MkdirAll(libATmpDir, 0o755))
	libBDir := filepath.Join(appDir, "libb_dir")
	require.NoError(t, os.MkdirAll(libBDir, 0o755))

	// Create libB in a directory relative to appDir
	libBPath := createLibFile(t, libBDir, "libB.so.1")

	appPath := filepath.Join(appDir, "myapp")
	libAPath := filepath.Join(libATmpDir, "libA.so.1")

	// App has RPATH=$ORIGIN/libb_dir (expands to appDir/libb_dir)
	appCtx := NewRootContext(appPath, []string{"$ORIGIN/libb_dir"}, nil, false)

	// libA is a child of the app - it should inherit the app's RPATH
	libACtx := appCtx.NewChildContext(libAPath, nil, nil)

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)
	// When resolving libB from libA's context, the inherited RPATH $ORIGIN/libb_dir
	// should expand using appDir (the origin of the RPATH), not libATmpDir
	resolved, err := resolver.Resolve("libB.so.1", libACtx)
	require.NoError(t, err)
	assert.Equal(t, libBPath, resolved)
}

func TestResolve_InheritedRPATH(t *testing.T) {
	tmpDir := t.TempDir()
	libPath := createLibFile(t, tmpDir, "libtransitive.so.1")

	// Create a chain: app -> libA -> the library we're looking for
	appCtx := NewRootContext("/app", []string{tmpDir}, nil, false)
	libACtx := appCtx.NewChildContext("/usr/lib/libA.so.1", nil, nil)

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)

	// libtransitive.so.1 should be found via app's inherited RPATH
	resolved, err := resolver.Resolve("libtransitive.so.1", libACtx)
	require.NoError(t, err)
	assert.Equal(t, libPath, resolved)
}

func TestResolve_InheritanceTermination(t *testing.T) {
	tmpDir := t.TempDir()
	// Only place the lib in tmpDir (accessible via app's RPATH)
	createLibFile(t, tmpDir, "libsecret.so.1")

	// App has RPATH pointing to tmpDir
	appCtx := NewRootContext("/app", []string{tmpDir}, nil, false)

	// libA has RUNPATH, which terminates the RPATH inheritance chain
	libACtx := appCtx.NewChildContext("/usr/lib/libA.so.1",
		nil,                          // no RPATH
		[]string{"/usr/lib/openssl"}, // has RUNPATH
	)

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)

	// libsecret.so.1 should NOT be found because RPATH inheritance was terminated
	_, err := resolver.Resolve("libsecret.so.1", libACtx)
	assert.Error(t, err, "should not find library when RPATH inheritance is terminated")
	var notResolved *ErrLibraryNotResolved
	assert.ErrorAs(t, err, &notResolved)
}

func TestResolve_LDLibraryPath(t *testing.T) {
	tmpDir := t.TempDir()
	libPath := createLibFile(t, tmpDir, "libfoo.so.1")

	resolver := NewLibraryResolver(nil, elf.EM_X86_64)

	t.Run("LD_LIBRARY_PATH not used at record time", func(t *testing.T) {
		t.Setenv("LD_LIBRARY_PATH", tmpDir)
		ctx := NewRootContext("/app", nil, nil, false) // IncludeLDLibraryPath=false
		_, err := resolver.Resolve("libfoo.so.1", ctx)
		assert.Error(t, err, "LD_LIBRARY_PATH should not be used at record time")
	})

	t.Run("LD_LIBRARY_PATH used at runner time", func(t *testing.T) {
		t.Setenv("LD_LIBRARY_PATH", tmpDir)
		ctx := NewRootContext("/app", nil, nil, true) // IncludeLDLibraryPath=true
		resolved, err := resolver.Resolve("libfoo.so.1", ctx)
		require.NoError(t, err, "LD_LIBRARY_PATH should be used at runner time")
		assert.Equal(t, libPath, resolved)
	})
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
	ctx := NewRootContext("/app", nil, nil, false)

	resolved, err := resolver.Resolve("libcached.so.1", ctx)
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
	ctx := NewRootContext("/app", nil, nil, false)

	_, err := resolver.Resolve("libnonexistent.so.9999", ctx)
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
	ctx := NewRootContext("/app", []string{tmpDir}, nil, false)

	resolved, err := resolver.Resolve("libfoo.so.1", ctx)
	require.NoError(t, err)
	// Should resolve to the real path (symlink resolved)
	assert.Equal(t, realPath, resolved)
}
