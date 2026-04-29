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
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/libccache"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfUnsupported skips the test if the current environment does not support
// the integration test requirements: Linux OS, amd64 or arm64 architecture, and gcc available.
func skipIfUnsupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("integration test requires Linux")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("syscall analysis supports amd64 and arm64 only")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available")
	}
}

// socketSyscallNumber returns the syscall number for socket(2) on the current architecture.
// x86_64: 41, arm64: 198.
func socketSyscallNumber() int {
	switch runtime.GOARCH {
	case "amd64":
		return 41
	case "arm64":
		return 198
	default:
		return -1
	}
}

// newTestValidator creates a fully wired Validator using a real libccache pipeline.
func newTestValidator(t *testing.T, hashDir string) *filevalidator.Validator {
	t.Helper()

	v, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	v.SetELFDynLibAnalyzer(elfdynlib.NewDynLibAnalyzer(fs))

	syscallAn := elfanalyzer.NewSyscallAnalyzer()
	v.SetSyscallAnalyzer(libccache.NewSyscallAdapter(syscallAn))

	cacheDir := filepath.Join(hashDir, "lib-cache")
	libcAnalyzer := libccache.NewLibcWrapperAnalyzer(syscallAn)
	cacheMgr, err := libccache.NewLibcCacheManager(cacheDir, fs, libcAnalyzer)
	require.NoError(t, err)

	v.SetLibcCache(libccache.NewCacheAdapter(cacheMgr, syscallAn))
	// Keep syscall occurrences in the saved record for integration assertions.
	v.SetIncludeDebugInfo(true)

	return v
}

// TestLibcCache_Integration_SocketSyscallDetected verifies the end-to-end pipeline:
// compile a C program that calls socket(), record it, and verify that the socket
// syscall is detected with source "libc_symbol_import".
// socket is a network syscall (IsNetwork=true) and is reliably detectable across architectures.
//
// Syscall numbers: x86_64=41, arm64=198.
//
// This covers AC-4.
func TestLibcCache_Integration_SocketSyscallDetected(t *testing.T) {
	skipIfUnsupported(t)

	syscallNum := socketSyscallNumber()

	// Compile a minimal C program that calls socket() via libc.
	src := `
#include <sys/socket.h>
int main() {
	int fd = socket(AF_INET, SOCK_STREAM, 0);
	return fd >= 0 ? 0 : 1;
}
`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "test.c")
	binFile := filepath.Join(tmpDir, "test_socket.elf")

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

	// Verify socket syscall is detected.
	var socketInfo *common.SyscallInfo
	for i := range record.SyscallAnalysis.DetectedSyscalls {
		info := &record.SyscallAnalysis.DetectedSyscalls[i]
		if info.Number == syscallNum {
			socketInfo = info
			break
		}
	}
	require.NotNil(t, socketInfo, "socket syscall (number %d) should be detected", syscallNum)
	require.NotEmpty(t, socketInfo.Occurrences, "socket syscall should have at least one occurrence")
	assert.Equal(t, "libc_symbol_import", socketInfo.Occurrences[0].Source, "socket should be detected via libc symbol import")
	assert.Equal(t, uint64(0), socketInfo.Occurrences[0].Location, "libc_symbol_import entries should have Location=0")

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
	skipIfUnsupported(t)

	src := `
#include <sys/socket.h>
int main() {
	int fd = socket(AF_INET, SOCK_STREAM, 0);
	return fd >= 0 ? 0 : 1;
}
`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "test2.c")
	binFile := filepath.Join(tmpDir, "test_socket2.elf")

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
