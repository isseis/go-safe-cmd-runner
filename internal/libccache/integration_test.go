//go:build integration

package libccache_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlibanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/libccache"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestValidator creates a fully wired Validator using a real libccache pipeline.
func newTestValidator(t *testing.T, hashDir string) *filevalidator.Validator {
	t.Helper()

	v, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	v.SetDynLibAnalyzer(dynlibanalysis.NewDynLibAnalyzer(fs))

	syscallAn := elfanalyzer.NewSyscallAnalyzer()
	v.SetSyscallAnalyzer(libccache.NewSyscallAdapter(syscallAn))

	cacheDir := filepath.Join(hashDir, "lib-cache")
	libcAnalyzer := libccache.NewLibcWrapperAnalyzer(syscallAn)
	cacheMgr, err := libccache.NewLibcCacheManager(cacheDir, fs, libcAnalyzer)
	require.NoError(t, err)

	v.SetLibcCache(libccache.NewCacheAdapter(cacheMgr, syscallAn))

	return v
}

// TestLibcCache_Integration_MkdirSyscallDetected verifies the end-to-end pipeline:
// compile a C program that calls mkdir(), record it, and verify that syscall 83
// (mkdir on x86_64) is detected with source "libc_symbol_import".
//
// This covers AC-4.
func TestLibcCache_Integration_MkdirSyscallDetected(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("syscall analysis only supports x86_64")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available")
	}

	// Compile a minimal C program that calls mkdir() via libc.
	const mkdirSyscallNumber = 83
	src := `
#include <sys/stat.h>
int main() { mkdir("/tmp/test_libccache", 0755); return 0; }
`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "test.c")
	binFile := filepath.Join(tmpDir, "test_mkdir.elf")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	cmd := exec.Command("gcc", "-o", binFile, srcFile) // dynamic link (default)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "gcc failed: %s", string(output))

	hashDir := filepath.Join(tmpDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o750))

	v := newTestValidator(t, hashDir)
	_, _, err = v.SaveRecord(binFile, false)
	require.NoError(t, err)

	record, err := v.LoadRecord(binFile)
	require.NoError(t, err)
	require.NotNil(t, record.SyscallAnalysis, "SyscallAnalysis should be set for an ELF binary")

	// Verify mkdir syscall (number 83) is detected.
	var mkdirInfo *common.SyscallInfo
	for i := range record.SyscallAnalysis.DetectedSyscalls {
		info := &record.SyscallAnalysis.DetectedSyscalls[i]
		if info.Number == mkdirSyscallNumber {
			mkdirInfo = info
			break
		}
	}
	require.NotNil(t, mkdirInfo, "mkdir syscall (number %d) should be detected", mkdirSyscallNumber)
	assert.Equal(t, "libc_symbol_import", mkdirInfo.Source, "mkdir should be detected via libc symbol import")
	assert.Equal(t, uint64(0), mkdirInfo.Location, "libc_symbol_import entries should have Location=0")

	// Verify the lib-cache directory was created.
	cacheDir := filepath.Join(hashDir, "lib-cache")
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err, "lib-cache directory should exist after record")
	assert.NotEmpty(t, entries, "lib-cache directory should contain at least one cache file")
}

// TestLibcCache_Integration_CacheReuse verifies that a second SaveRecord call
// does not overwrite the libc cache file (mtime must not change).
//
// This covers AC-3 (cache HIT).
func TestLibcCache_Integration_CacheReuse(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("syscall analysis only supports x86_64")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available")
	}

	src := `
#include <sys/stat.h>
int main() { mkdir("/tmp/test_libccache2", 0755); return 0; }
`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "test2.c")
	binFile := filepath.Join(tmpDir, "test_mkdir2.elf")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	cmd := exec.Command("gcc", "-o", binFile, srcFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "gcc failed: %s", string(output))

	hashDir := filepath.Join(tmpDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o750))

	// First record: creates the libc cache.
	v1 := newTestValidator(t, hashDir)
	_, _, err = v1.SaveRecord(binFile, false)
	require.NoError(t, err)

	// Collect mtime of all cache files after first record.
	cacheDir := filepath.Join(hashDir, "lib-cache")
	mtimesBefore := collectFileMtimes(t, cacheDir)
	require.NotEmpty(t, mtimesBefore, "lib-cache should have files after first record")

	// Second record: should hit the cache and not rewrite the cache files.
	v2 := newTestValidator(t, hashDir)
	_, _, err = v2.SaveRecord(binFile, true) // force=true to overwrite the hash record
	require.NoError(t, err)

	mtimesAfter := collectFileMtimes(t, cacheDir)
	for path, before := range mtimesBefore {
		after, ok := mtimesAfter[path]
		require.True(t, ok, "cache file %s should still exist after second record", path)
		assert.Equal(t, before, after, "mtime of cache file %s should not change on cache HIT", path)
	}
}

// collectFileMtimes returns a map of filename → mtime (nanoseconds) for all
// regular files in dir (non-recursive).
func collectFileMtimes(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	result := make(map[string]int64, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		require.NoError(t, err)
		result[e.Name()] = info.ModTime().UnixNano()
	}
	return result
}
