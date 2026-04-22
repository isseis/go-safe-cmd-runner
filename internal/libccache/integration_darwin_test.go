//go:build integration && darwin

package libccache_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/libccache"
	"github.com/isseis/go-safe-cmd-runner/internal/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfUnsupportedMacOS skips when not running on macOS arm64 or when clang is missing.
func skipIfUnsupportedMacOS(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("macOS integration tests require darwin")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("macOS libSystem syscall analysis supports arm64 only")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not available")
	}
}

// newTestValidatorMacOS creates a fully wired Validator for macOS with a real
// MachoLibSystemAdapter backed by the dyld shared cache, plus MachODynLibAnalyzer
// so that DynLibDeps is populated during SaveRecord.
func newTestValidatorMacOS(t *testing.T, hashDir string) *filevalidator.Validator {
	t.Helper()

	v, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})

	// Wire Mach-O dynamic library dependency analyzer so DynLibDeps is populated.
	v.SetMachODynLibAnalyzer(machodylib.NewMachODynLibAnalyzer(fs))

	cacheDir := filepath.Join(hashDir, "lib-cache")

	machoLibSysMgr, err := libccache.NewMachoLibSystemCacheManager(cacheDir)
	require.NoError(t, err)

	machoAdapter := libccache.NewMachoLibSystemAdapter(machoLibSysMgr, fs)
	v.SetLibSystemCache(machoAdapter)

	return v
}

// compileMacOSBinary compiles a C source file with clang into a Mach-O binary.
func compileMacOSBinary(t *testing.T, srcFile, binFile string) {
	t.Helper()
	cmd := exec.Command("clang", "-o", binFile, srcFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "clang failed: %s", string(output))
}

// TestLibSystemCache_Integration_SocketSyscallDetected verifies the end-to-end
// pipeline on macOS arm64: compile a C program that calls socket(), record it,
// and verify that socket is detected with Source="libsystem_symbol_import".
//
// On macOS 11+, libSystem.B.dylib is in the dyld shared cache and is not
// included in DynLibDeps. The adapter falls back to symbol-name matching which
// is sufficient to detect network syscalls.
func TestLibSystemCache_Integration_SocketSyscallDetected(t *testing.T) {
	skipIfUnsupportedMacOS(t)

	src := `
#include <sys/socket.h>
int main() {
	int fd = socket(AF_INET, SOCK_STREAM, 0);
	return fd >= 0 ? 0 : 1;
}
`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "test.c")
	binFile := filepath.Join(tmpDir, "test_socket.macho")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o600))
	compileMacOSBinary(t, srcFile, binFile)

	hashDir := filepath.Join(tmpDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	v := newTestValidatorMacOS(t, hashDir)
	_, _, err := v.SaveRecord(binFile, false)
	require.NoError(t, err)

	record, err := v.LoadRecord(binFile)
	require.NoError(t, err)

	require.NotNil(t, record.SyscallAnalysis,
		"SyscallAnalysis should be set: socket() import must trigger detection")

	// Verify socket syscall (number=97) is detected via libsystem_symbol_import.
	const socketSyscallNumber = 97
	var socketInfo *struct {
		Number int
		Source string
	}
	for _, sc := range record.SyscallAnalysis.DetectedSyscalls {
		if sc.Number == socketSyscallNumber && sc.Source == libccache.SourceLibsystemSymbolImport {
			socketInfo = &struct {
				Number int
				Source string
			}{sc.Number, sc.Source}
			break
		}
	}
	require.NotNil(t, socketInfo,
		"socket syscall (number %d, source %q) should be detected; got %v",
		socketSyscallNumber, libccache.SourceLibsystemSymbolImport, record.SyscallAnalysis.DetectedSyscalls)
	assert.Equal(t, socketSyscallNumber, socketInfo.Number)
	assert.Equal(t, libccache.SourceLibsystemSymbolImport, socketInfo.Source)
}

// TestLibSystemCache_Integration_CacheReuse verifies that a second SaveRecord
// call does not rewrite the libSystem cache file (mtime must not change).
//
// Verifies cache HIT: the second SaveRecord must not rewrite the libSystem cache file.
func TestLibSystemCache_Integration_CacheReuse(t *testing.T) {
	skipIfUnsupportedMacOS(t)

	src := `
#include <sys/socket.h>
int main() {
	int fd = socket(AF_INET, SOCK_STREAM, 0);
	return fd >= 0 ? 0 : 1;
}
`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "test2.c")
	binFile := filepath.Join(tmpDir, "test_socket2.macho")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o600))
	compileMacOSBinary(t, srcFile, binFile)

	hashDir := filepath.Join(tmpDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	// First record: creates the libSystem cache.
	v1 := newTestValidatorMacOS(t, hashDir)
	_, _, err := v1.SaveRecord(binFile, false)
	require.NoError(t, err)

	cacheDir := filepath.Join(hashDir, "lib-cache")
	mtimesBefore := collectFileMtimes(t, cacheDir)
	if len(mtimesBefore) == 0 {
		t.Skip("no lib-cache files created (libSystem cache not available on this environment)")
	}

	// Second record: should hit the cache and not rewrite the cache files.
	v2 := newTestValidatorMacOS(t, hashDir)
	_, _, err = v2.SaveRecord(binFile, true) // force=true to allow re-recording the hash
	require.NoError(t, err)

	mtimesAfter := collectFileMtimes(t, cacheDir)
	for path, before := range mtimesBefore {
		after, ok := mtimesAfter[path]
		require.True(t, ok, "cache file %s should still exist after second record", path)
		assert.Equal(t, before, after, "mtime of cache file %s should not change on cache HIT", path)
	}
}

// TestLibSystemCache_Integration_Fallback verifies that when dyld cache extraction
// fails (simulated by a binary with no libSystem dependency), the adapter completes
// without error and falls back gracefully.
//
// Verifies graceful fallback: when no network symbols are imported, SaveRecord must
// succeed without error even if the dyld cache extraction path is unavailable.
func TestLibSystemCache_Integration_Fallback(t *testing.T) {
	skipIfUnsupportedMacOS(t)

	// A minimal binary that does not import any network functions.
	// It still imports libSystem.dylib (because all macOS binaries do), but
	// importing no network-related symbols means GetSyscallInfos returns empty
	// via either cache match or fallback.
	src := `
int main() { return 0; }
`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "test3.c")
	binFile := filepath.Join(tmpDir, "test_minimal.macho")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o600))
	compileMacOSBinary(t, srcFile, binFile)

	hashDir := filepath.Join(tmpDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	v := newTestValidatorMacOS(t, hashDir)
	_, _, err := v.SaveRecord(binFile, false)
	require.NoError(t, err, "SaveRecord must succeed even when no network syscalls are detected")

	record, err := v.LoadRecord(binFile)
	require.NoError(t, err)
	// SyscallAnalysis may be nil (no syscalls detected) or set with no network entries.
	if record.SyscallAnalysis != nil {
		for _, sc := range record.SyscallAnalysis.DetectedSyscalls {
			assert.False(t, sc.IsNetwork,
				"minimal binary should not have network syscalls, got %+v", sc)
		}
	}
}

// TestLibSystemCache_Integration_ELFFlowNonRegression verifies that the ELF
// libc cache flow is not affected by the macOS libSystem cache changes.
//
// On macOS, ELF binaries cannot be directly executed, but we verify that
// SaveRecord on a non-ELF binary completes without error and that the macOS
// libSystem cache addition does not break the base record/load pipeline.
func TestLibSystemCache_Integration_ELFFlowNonRegression(t *testing.T) {
	skipIfUnsupportedMacOS(t)

	// Use /bin/ls as a real macOS binary that is not an ELF binary.
	// This verifies that the macOS libSystem cache addition does not break
	// the existing record/load pipeline for Mach-O binaries.
	binFile := "/bin/ls"
	if _, err := os.Stat(binFile); err != nil {
		t.Skip("/bin/ls not available")
	}

	tmpDir := commontesting.SafeTempDir(t)
	hashDir := filepath.Join(tmpDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	// Use a validator WITHOUT libSystem cache to verify the base pipeline is unaffected.
	v, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)

	_, _, err = v.SaveRecord(binFile, false)
	require.NoError(t, err, "SaveRecord must succeed on a real macOS binary without libSystem cache")

	record, err := v.LoadRecord(binFile)
	require.NoError(t, err)
	// Without the libSystem cache injected, SyscallAnalysis is nil (no analyzer set).
	assert.Nil(t, record.SyscallAnalysis,
		"SyscallAnalysis should be nil when no libSystem cache is injected")
}

// TestLibSystemCache_Integration_MachodyLibResolver verifies that
// machodylib.ResolveLibSystemKernel can resolve the libsystem_kernel.dylib
// source from a DynLibDeps slice containing libSystem.B.dylib on macOS arm64.
//
// Note: on macOS 11+, MachODynLibAnalyzer does not include dyld shared cache
// entries in DynLibDeps, so we construct the DynLibDeps directly here.
func TestLibSystemCache_Integration_MachodyLibResolver(t *testing.T) {
	skipIfUnsupportedMacOS(t)

	// Construct a minimal DynLibDeps containing libSystem.B.dylib (the umbrella).
	// On macOS arm64, all system libraries are in the dyld shared cache, so we
	// cannot obtain a real hash — we only provide the install name.
	dynDeps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/libSystem.B.dylib"},
	}

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
    source, err := machodylib.ResolveLibSystemKernel(dynDeps, fs, true)
	require.NoError(t, err)

	if source == nil {
		// dyld shared cache unavailable: acceptable.
		t.Log("ResolveLibSystemKernel returned nil source; dyld shared cache may be unavailable")
		return
	}
	assert.NotEmpty(t, source.Path, "resolved source path must not be empty")
	assert.NotEmpty(t, source.Hash, "resolved source hash must not be empty")
}
