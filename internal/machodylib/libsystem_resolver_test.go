//go:build test

package machodylib

import (
	"debug/macho"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildMachOWithReexport builds a minimal arm64 Mach-O binary that contains a single
// LC_REEXPORT_DYLIB (0x1F) load command pointing to reexportName.
// The format mirrors LC_LOAD_DYLIB, so buildDylibLoadCmd logic applies directly.
func buildMachOWithReexport(reexportName string) []byte {
	const (
		machOMagic64      = uint32(0xFEEDFACF)
		machOHeaderSize64 = 32
		dylibCmdHdrSize   = 24
		mhDylib           = uint32(6) // MH_DYLIB for an umbrella library
		lcReexportDylib   = uint32(0x1F)
		align4Mask        = 3
		cpuArm64          = uint32(macho.CpuArm64)
	)

	alignTo4 := func(n int) int {
		return (n + align4Mask) &^ align4Mask
	}

	// Build LC_REEXPORT_DYLIB load command for reexportName.
	totalCmdSize := alignTo4(dylibCmdHdrSize + len(reexportName) + 1)
	cmd := make([]byte, totalCmdSize)
	binary.LittleEndian.PutUint32(cmd[0:4], lcReexportDylib)
	binary.LittleEndian.PutUint32(cmd[4:8], uint32(totalCmdSize)) //nolint:gosec
	binary.LittleEndian.PutUint32(cmd[8:12], dylibCmdHdrSize)     // name_offset
	copy(cmd[dylibCmdHdrSize:], reexportName)

	hdr := make([]byte, machOHeaderSize64)
	binary.LittleEndian.PutUint32(hdr[0:4], machOMagic64)
	binary.LittleEndian.PutUint32(hdr[4:8], cpuArm64)
	binary.LittleEndian.PutUint32(hdr[8:12], 0) // cpusubtype
	binary.LittleEndian.PutUint32(hdr[12:16], mhDylib)
	binary.LittleEndian.PutUint32(hdr[16:20], 1)                    // ncmds
	binary.LittleEndian.PutUint32(hdr[20:24], uint32(totalCmdSize)) //nolint:gosec
	binary.LittleEndian.PutUint32(hdr[24:28], 0)                    // flags
	binary.LittleEndian.PutUint32(hdr[28:32], 0)                    // reserved

	out := make([]byte, machOHeaderSize64+totalCmdSize)
	copy(out, hdr)
	copy(out[machOHeaderSize64:], cmd)
	return out
}

// writeTempFile writes data to a file in dir with the given name.
func writeTempFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// evalReal resolves symlinks in path so safefileio.SafeOpenFile accepts it on
// macOS (where /var is a symlink to /private/var).
func evalReal(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return p
}

// TestResolveLibSystemKernel_DirectKernel verifies that a DynLibDeps entry
// pointing directly to libsystem_kernel.dylib is used as the highest-priority source.
func TestResolveLibSystemKernel_DirectKernel(t *testing.T) {
	dir := evalReal(t, t.TempDir())
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})

	kernelPath := writeTempFile(t, dir, "libsystem_kernel.dylib", []byte("dummy"))

	dynLibDeps := []fileanalysis.LibEntry{
		{
			SOName: "/usr/lib/system/libsystem_kernel.dylib",
			Path:   kernelPath,
			Hash:   "sha256:aabbcc",
		},
	}

	src, err := ResolveLibSystemKernel(dynLibDeps, fs, false)
	require.NoError(t, err)
	require.NotNil(t, src)
	assert.Equal(t, kernelPath, src.Path)
	assert.True(t, strings.HasPrefix(src.Hash, "sha256:"), "expected sha256 prefix, got %q", src.Hash)
	assert.NotNil(t, src.GetData)
	data, err := src.GetData()
	require.NoError(t, err)
	assert.Equal(t, []byte("dummy"), data)
}

// TestResolveLibSystemKernel_UmbrellaReexport verifies that when only
// libSystem.B.dylib is in DynLibDeps, libsystem_kernel.dylib is found by
// traversing its LC_REEXPORT_DYLIB entries.
func TestResolveLibSystemKernel_UmbrellaReexport(t *testing.T) {
	dir := evalReal(t, t.TempDir())
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})

	// Write a fake libsystem_kernel.dylib so the stat check passes.
	kernelPath := writeTempFile(t, dir, "libsystem_kernel.dylib", []byte("kernel_dummy"))

	// Build a minimal umbrella Mach-O that re-exports kernelPath.
	umbrellaBytes := buildMachOWithReexport(kernelPath)
	umbrellaPath := writeTempFile(t, dir, "libSystem.B.dylib", umbrellaBytes)

	dynLibDeps := []fileanalysis.LibEntry{
		{
			SOName: "/usr/lib/libSystem.B.dylib",
			Path:   umbrellaPath,
			Hash:   "sha256:umbrella",
		},
	}

	src, err := ResolveLibSystemKernel(dynLibDeps, fs, false)
	require.NoError(t, err)
	require.NotNil(t, src)
	assert.Equal(t, kernelPath, src.Path)
	assert.True(t, strings.HasPrefix(src.Hash, "sha256:"), "expected sha256 prefix, got %q", src.Hash)
}

// TestResolveLibSystemKernel_NoLibSystem verifies that when there is no
// libSystem-family entry in DynLibDeps, ResolveLibSystemKernel returns nil, nil.
func TestResolveLibSystemKernel_NoLibSystem(t *testing.T) {
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/libssl.1.1.dylib", Path: "/usr/lib/libssl.1.1.dylib"},
	}

	src, err := ResolveLibSystemKernel(dynLibDeps, fs, false)
	require.NoError(t, err)
	assert.Nil(t, src)
}

// TestResolveLibSystemKernel_EmptyDeps verifies that an empty DynLibDeps list
// returns nil, nil.
func TestResolveLibSystemKernel_EmptyDeps(t *testing.T) {
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	src, err := ResolveLibSystemKernel(nil, fs, false)
	require.NoError(t, err)
	assert.Nil(t, src)
}

// TestFindLibSystemCandidates_KernelDirect verifies that a direct
// libsystem_kernel.dylib entry is classified as Kernel.
func TestFindLibSystemCandidates_KernelDirect(t *testing.T) {
	deps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/system/libsystem_kernel.dylib", Path: "/path/to/libsystem_kernel.dylib"},
	}
	c := findLibSystemCandidates(deps)
	assert.True(t, c.HasLibSystem)
	require.NotNil(t, c.Kernel)
	assert.Equal(t, "/usr/lib/system/libsystem_kernel.dylib", c.Kernel.SOName)
	assert.Nil(t, c.Umbrella)
}

// TestFindLibSystemCandidates_UmbrellaOnly verifies that libSystem.B.dylib is
// classified as Umbrella when no direct kernel entry is present.
func TestFindLibSystemCandidates_UmbrellaOnly(t *testing.T) {
	deps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/libSystem.B.dylib", Path: "/path/to/libSystem.B.dylib"},
	}
	c := findLibSystemCandidates(deps)
	assert.True(t, c.HasLibSystem)
	assert.Nil(t, c.Kernel)
	require.NotNil(t, c.Umbrella)
	assert.Equal(t, "/usr/lib/libSystem.B.dylib", c.Umbrella.SOName)
}

// TestFindLibSystemCandidates_BothPresent verifies that when both are present,
// Kernel takes the direct entry and Umbrella also refers to the umbrella entry.
func TestFindLibSystemCandidates_BothPresent(t *testing.T) {
	deps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/libSystem.B.dylib", Path: "/path/to/libSystem.B.dylib"},
		{SOName: "/usr/lib/system/libsystem_kernel.dylib", Path: "/path/to/libsystem_kernel.dylib"},
	}
	c := findLibSystemCandidates(deps)
	assert.True(t, c.HasLibSystem)
	require.NotNil(t, c.Kernel)
	require.NotNil(t, c.Umbrella)
}

// TestFindKernelInUmbrellaReexports_Found verifies that a Mach-O umbrella
// binary with LC_REEXPORT_DYLIB pointing to a file on disk is resolved correctly.
func TestFindKernelInUmbrellaReexports_Found(t *testing.T) {
	dir := evalReal(t, t.TempDir())
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})

	kernelPath := writeTempFile(t, dir, "libsystem_kernel.dylib", []byte("kernel"))
	umbrellaBytes := buildMachOWithReexport(kernelPath)
	umbrellaPath := writeTempFile(t, dir, "libSystem.B.dylib", umbrellaBytes)

	result, err := findKernelInUmbrellaReexports(umbrellaPath, fs)
	require.NoError(t, err)
	assert.Equal(t, kernelPath, result)
}

// TestFindKernelInUmbrellaReexports_NotFound verifies that when LC_REEXPORT_DYLIB
// points to a non-existent path, findKernelInUmbrellaReexports returns empty string.
func TestFindKernelInUmbrellaReexports_NotFound(t *testing.T) {
	dir := evalReal(t, t.TempDir())
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})

	// LC_REEXPORT_DYLIB points to a path that does not exist on disk.
	umbrellaBytes := buildMachOWithReexport(filepath.Join(dir, "does_not_exist.dylib"))
	umbrellaPath := writeTempFile(t, dir, "libSystem.B.dylib", umbrellaBytes)

	result, err := findKernelInUmbrellaReexports(umbrellaPath, fs)
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestFindKernelInUmbrellaReexports_SkipsWellKnownStubPath verifies that when
// LC_REEXPORT_DYLIB points to the well-known stub install name
// (/usr/lib/system/libsystem_kernel.dylib), findKernelInUmbrellaReexports returns ""
// regardless of whether the path exists on disk, so that the caller can proceed to
// dyld shared cache extraction (priority 3).
func TestFindKernelInUmbrellaReexports_SkipsWellKnownStubPath(t *testing.T) {
	dir := evalReal(t, t.TempDir())
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})

	// The umbrella re-exports the canonical well-known install name.
	// Even though the path may not exist on this test machine, the function must
	// return "" because it is filtered by the stub-path guard.
	umbrellaBytes := buildMachOWithReexport(libsystemKernelInstallName)
	umbrellaPath := writeTempFile(t, dir, "libSystem.B.dylib", umbrellaBytes)

	result, err := findKernelInUmbrellaReexports(umbrellaPath, fs)
	require.NoError(t, err)
	assert.Empty(t, result, "well-known stub install-name should be skipped in priority-2 resolution")
}
