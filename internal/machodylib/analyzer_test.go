//go:build test && darwin

package machodylib

import (
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib"
	machodylibtesting "github.com/isseis/go-safe-cmd-runner/internal/machodylib/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realPath resolves symlinks in the path so that safefileio can accept it on macOS
// (where /var is a symlink to /private/var).
func realPath(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	require.NoError(t, err)
	return resolved
}

// TestAnalyze_NonMachO verifies that Analyze returns (nil, nil, nil) for a
// non-Mach-O file.
func TestAnalyze_NonMachO(t *testing.T) {
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	tmp := realPath(t, t.TempDir())
	notMachO := filepath.Join(tmp, "notmacho.txt")
	require.NoError(t, os.WriteFile(notMachO, []byte("hello world\n"), 0o600))

	libs, warnings, err := a.Analyze(notMachO)
	assert.NoError(t, err)
	assert.Nil(t, libs)
	assert.Nil(t, warnings)
}

// TestAnalyze_MacOSBinary tests Analyze against the real /bin/ls on macOS.
// /bin/ls links against dyld shared cache libraries only; Analyze must parse at
// least one LC_LOAD_DYLIB entry and return without error or warnings.
// Because all deps are shared cache libraries absent from disk, libs is empty.
func TestAnalyze_MacOSBinary(t *testing.T) {
	if _, err := os.Stat("/bin/ls"); err != nil {
		t.Skip("/bin/ls not available")
	}

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	// openMachO must succeed, confirming that extractLoadCommands can parse the
	// binary and return at least one LC_LOAD_DYLIB (e.g., libSystem.B.dylib).
	machoFile, closer, err := a.openMachO("/bin/ls")
	require.NoError(t, err)
	deps, _ := extractLoadCommands(machoFile)
	_ = machoFile.Close()
	_ = closer.Close()
	assert.NotEmpty(t, deps, "/bin/ls should have LC_LOAD_DYLIB entries")

	libs, warnings, err := a.Analyze("/bin/ls")
	require.NoError(t, err)
	assert.Empty(t, warnings)

	// All /bin/ls dependencies are in dyld shared cache and absent from disk,
	// so libs should be nil or empty.
	for _, lib := range libs {
		assert.True(t, strings.HasPrefix(lib.Hash, "sha256:"),
			"expected sha256: prefix in hash for %s", lib.SOName)
		assert.NotEmpty(t, lib.Path)
		assert.NotEmpty(t, lib.SOName)
	}
}

// TestHasDynamicLibDeps_SystemBinary verifies HasDynamicLibDeps for /bin/ls.
// /bin/ls links only against dyld shared cache libraries absent from disk,
// so HasDynamicLibDeps should return false.
func TestHasDynamicLibDeps_SystemBinary(t *testing.T) {
	if _, err := os.Stat("/bin/ls"); err != nil {
		t.Skip("/bin/ls not available")
	}

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	hasDeps, err := HasDynamicLibDeps("/bin/ls", fs)
	require.NoError(t, err)
	assert.False(t, hasDeps, "/bin/ls should have no non-shared-cache dependencies")
}

// TestHasDynamicLibDeps_NonMachO verifies HasDynamicLibDeps for a non-Mach-O file.
func TestHasDynamicLibDeps_NonMachO(t *testing.T) {
	tmp := realPath(t, t.TempDir())
	notMachO := filepath.Join(tmp, "text.txt")
	require.NoError(t, os.WriteFile(notMachO, []byte("not a binary\n"), 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	hasDeps, err := HasDynamicLibDeps(notMachO, fs)
	require.NoError(t, err)
	assert.False(t, hasDeps)
}

// TestExtractDylibName verifies that extractDylibName correctly parses the
// library name from a synthesized LC_LOAD_DYLIB raw bytes.
func TestExtractDylibName(t *testing.T) {
	// Synthesize LC_LOAD_DYLIB raw bytes (little-endian):
	// [0:4]  cmd        = 0x0C (LC_LOAD_DYLIB)
	// [4:8]  cmdsize    = total size
	// [8:12] nameOffset = 24 (fixed header is 6*4=24 bytes)
	// [12:24] timestamp, current_version, compat_version (unused)
	// [24:]  name (null-terminated)
	name := "/usr/lib/libFoo.dylib"
	nameOffset := uint32(24)
	raw := make([]byte, int(nameOffset)+len(name)+1)
	binary.LittleEndian.PutUint32(raw[0:4], 0x0C)
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)))
	binary.LittleEndian.PutUint32(raw[8:12], nameOffset)
	copy(raw[nameOffset:], name)

	result := extractDylibName(raw, binary.LittleEndian)
	assert.Equal(t, name, result)
}

// TestExtractRpathName verifies that extractRpathName correctly parses the
// rpath from a synthesized LC_RPATH raw bytes.
func TestExtractRpathName(t *testing.T) {
	// Synthesize LC_RPATH raw bytes (little-endian):
	// [0:4]  cmd        = 0x8000001C (LC_RPATH)
	// [4:8]  cmdsize    = total size
	// [8:12] pathOffset = 12
	// [12:]  path (null-terminated)
	path := "@executable_path/../Frameworks"
	pathOffset := uint32(12)
	raw := make([]byte, int(pathOffset)+len(path)+1)
	binary.LittleEndian.PutUint32(raw[0:4], 0x8000001C)
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)))
	binary.LittleEndian.PutUint32(raw[8:12], pathOffset)
	copy(raw[12:], path)

	result := extractRpathName(raw, binary.LittleEndian)
	assert.Equal(t, path, result)
}

// TestExtractDylibName_TooShort verifies that extractDylibName returns empty
// string for raw bytes that are too short.
func TestExtractDylibName_TooShort(t *testing.T) {
	result := extractDylibName([]byte{0x01, 0x02}, binary.LittleEndian)
	assert.Equal(t, "", result)
}

// TestExtractRpathName_TooShort verifies that extractRpathName returns empty
// string for raw bytes that are too short.
func TestExtractRpathName_TooShort(t *testing.T) {
	result := extractRpathName([]byte{0x01, 0x02}, binary.LittleEndian)
	assert.Equal(t, "", result)
}

// --- Fat Mach-O test helpers ---

// writeFatBinary writes a Fat binary (with one minimal no-dep slice per CPU type)
// to a test-scoped temp dir and returns the resolved absolute path.
func writeFatBinary(t *testing.T, name string, cpuTypes []macho.Cpu) string {
	t.Helper()
	tmp := realPath(t, t.TempDir())
	path := filepath.Join(tmp, name)
	slices := make([]machodylibtesting.FatSlice, len(cpuTypes))
	for i, cpu := range cpuTypes {
		slices[i] = machodylibtesting.FatSlice{CPU: cpu, Bytes: machodylibtesting.BuildMachOWithDeps(cpu, nil, nil, nil)}
	}
	require.NoError(t, os.WriteFile(path, machodylibtesting.BuildFatBinaryFromSlices(slices), 0o600))
	return path
}

// --- Fat binary slice-selection tests ---

// TestOpenMachO_FatBinary_MatchingSlice verifies that openMachO selects the
// native-arch slice from a Fat binary and returns a valid *macho.File whose
// Cpu field matches the native CPU type.
func TestOpenMachO_FatBinary_MatchingSlice(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	path := writeFatBinary(t, "fat_native.bin", []macho.Cpu{nativeCPU})

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	machoFile, closer, err := a.openMachO(path)
	require.NoError(t, err)
	require.NotNil(t, machoFile)
	require.NotNil(t, closer)
	defer func() { _ = machoFile.Close() }()
	defer func() { _ = closer.Close() }()

	assert.Equal(t, nativeCPU, machoFile.Cpu,
		"opened slice CPU type should match native CPU")
}

// TestOpenMachO_FatBinary_NoMatchingSlice verifies that openMachO returns an
// ErrNoMatchingSlice error when the Fat binary contains no native-arch slice.
func TestOpenMachO_FatBinary_NoMatchingSlice(t *testing.T) {
	nonNativeCPU := machodylibtesting.NonNativeCPU()
	path := writeFatBinary(t, "fat_non_native.bin", []macho.Cpu{nonNativeCPU})

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	_, _, err := a.openMachO(path)
	require.Error(t, err)

	var noSlice *ErrNoMatchingSlice
	require.True(t, errors.As(err, &noSlice),
		"expected ErrNoMatchingSlice, got %T: %v", err, err)
	assert.Equal(t, path, noSlice.BinaryPath)
	assert.Equal(t, runtime.GOARCH, noSlice.GOARCH)
}

// TestAnalyze_FatBinary_MatchingSlice verifies that Analyze returns (nil, nil, nil)
// for a Fat binary with a native-arch slice that has no LC_LOAD_DYLIB entries.
func TestAnalyze_FatBinary_MatchingSlice(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	path := writeFatBinary(t, "fat_native.bin", []macho.Cpu{nativeCPU})

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	libs, warnings, err := a.Analyze(path)
	require.NoError(t, err)
	assert.Nil(t, libs)
	assert.Nil(t, warnings)
}

// TestAnalyze_FatBinary_NoMatchingSlice verifies that Analyze propagates
// ErrNoMatchingSlice (and does NOT swallow it as ErrNotMachO) when the Fat
// binary has no native-arch slice.
func TestAnalyze_FatBinary_NoMatchingSlice(t *testing.T) {
	nonNativeCPU := machodylibtesting.NonNativeCPU()
	path := writeFatBinary(t, "fat_non_native.bin", []macho.Cpu{nonNativeCPU})

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	_, _, err := a.Analyze(path)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotMachO),
		"ErrNoMatchingSlice must not be silently swallowed as ErrNotMachO")

	var noSlice *ErrNoMatchingSlice
	require.True(t, errors.As(err, &noSlice),
		"expected ErrNoMatchingSlice, got %T: %v", err, err)
}

// TestHasDynamicLibDeps_FatBinary_MatchingSlice verifies HasDynamicLibDeps for a
// Fat binary whose native-arch slice has no LC_LOAD_DYLIB entries.
func TestHasDynamicLibDeps_FatBinary_MatchingSlice(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	path := writeFatBinary(t, "fat_native.bin", []macho.Cpu{nativeCPU})

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	hasDeps, err := HasDynamicLibDeps(path, fs)
	require.NoError(t, err)
	assert.False(t, hasDeps, "Fat binary with no load commands should report no deps")
}

// TestHasDynamicLibDeps_FatBinary_NoMatchingSlice verifies that HasDynamicLibDeps
// returns (false, nil) for a Fat binary that has no native-arch slice.
func TestHasDynamicLibDeps_FatBinary_NoMatchingSlice(t *testing.T) {
	nonNativeCPU := machodylibtesting.NonNativeCPU()
	path := writeFatBinary(t, "fat_non_native.bin", []macho.Cpu{nonNativeCPU})

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	hasDeps, err := HasDynamicLibDeps(path, fs)
	require.NoError(t, err)
	assert.False(t, hasDeps, "Fat binary with no matching arch should report no deps")
}

// --- Phase 2 unit tests using synthetic Mach-O files ---

// TestAnalyze_SingleArchMachO_NoDeps verifies that Analyze returns (nil, nil, nil) for a
// single-architecture Mach-O binary that has no LC_LOAD_DYLIB entries.
func TestAnalyze_SingleArchMachO_NoDeps(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	buf := machodylibtesting.BuildMachOWithDeps(nativeCPU, nil, nil, nil)

	tmp := realPath(t, t.TempDir())
	path := filepath.Join(tmp, "nolc.bin")
	require.NoError(t, os.WriteFile(path, buf, 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	libs, warnings, err := a.Analyze(path)
	require.NoError(t, err)
	assert.Nil(t, libs)
	assert.Nil(t, warnings)
}

// TestAnalyze_StrongDepResolutionFailure verifies that Analyze returns an error when
// a LC_LOAD_DYLIB (strong) dependency cannot be resolved.
func TestAnalyze_StrongDepResolutionFailure(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	// Use an absolute path outside dyld shared cache prefixes that does not exist.
	buf := machodylibtesting.BuildMachOWithDeps(nativeCPU,
		[]string{"/nonexistent_dir_xyz_12345/libfoo.dylib"},
		nil, nil)

	tmp := realPath(t, t.TempDir())
	path := filepath.Join(tmp, "test.bin")
	require.NoError(t, os.WriteFile(path, buf, 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	_, _, err := a.Analyze(path)
	require.Error(t, err, "unresolvable strong dep must cause an error")
}

// TestAnalyze_WeakDepSkipped verifies that an unresolvable LC_LOAD_WEAK_DYLIB
// dependency is silently skipped and Analyze returns (nil, nil, nil).
func TestAnalyze_WeakDepSkipped(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	buf := machodylibtesting.BuildMachOWithDeps(nativeCPU,
		nil,
		[]string{"/nonexistent_dir_xyz_12345/libweak.dylib"},
		nil)

	tmp := realPath(t, t.TempDir())
	path := filepath.Join(tmp, "test.bin")
	require.NoError(t, os.WriteFile(path, buf, 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	libs, warnings, err := a.Analyze(path)
	require.NoError(t, err, "unresolvable weak dep must not cause an error")
	assert.Nil(t, libs)
	assert.Nil(t, warnings)
}

// TestAnalyze_UnknownAtToken_Warning verifies that an install name with an unknown
// @ token generates an AnalysisWarning and does not cause Analyze to fail.
func TestAnalyze_UnknownAtToken_Warning(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	// @loaderrpath is an intentional misspelling of @loader_path – an unknown @ token.
	buf := machodylibtesting.BuildMachOWithDeps(nativeCPU,
		[]string{"@loaderrpath/libfoo.dylib"},
		nil, nil)

	tmp := realPath(t, t.TempDir())
	path := filepath.Join(tmp, "test.bin")
	require.NoError(t, os.WriteFile(path, buf, 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	libs, warnings, err := a.Analyze(path)
	require.NoError(t, err, "unknown @ token must not cause an error")
	assert.Nil(t, libs)
	require.Len(t, warnings, 1, "expected exactly one AnalysisWarning")
	assert.Equal(t, "@loaderrpath/libfoo.dylib", warnings[0].InstallName)
}

// TestAnalyze_IndirectDeps verifies that Analyze recursively resolves transitive
// LC_LOAD_DYLIB dependencies (BFS) and includes them all in the returned slice.
func TestAnalyze_IndirectDeps(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	tmp := realPath(t, t.TempDir())

	// lib2.dylib: leaf, no dependencies.
	lib2Path := filepath.Join(tmp, "lib2.dylib")
	require.NoError(t, os.WriteFile(lib2Path,
		machodylibtesting.BuildMachOWithDeps(nativeCPU, nil, nil, nil), 0o600))

	// lib1.dylib: depends on lib2 by absolute path.
	lib1Path := filepath.Join(tmp, "lib1.dylib")
	require.NoError(t, os.WriteFile(lib1Path,
		machodylibtesting.BuildMachOWithDeps(nativeCPU, []string{lib2Path}, nil, nil), 0o600))

	// root.bin: depends on lib1 by absolute path.
	rootPath := filepath.Join(tmp, "root.bin")
	require.NoError(t, os.WriteFile(rootPath,
		machodylibtesting.BuildMachOWithDeps(nativeCPU, []string{lib1Path}, nil, nil), 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	libs, warnings, err := a.Analyze(rootPath)
	require.NoError(t, err)
	assert.Nil(t, warnings)
	require.Len(t, libs, 2, "both lib1 and lib2 must be recorded")

	soNames := make(map[string]bool, len(libs))
	for _, lib := range libs {
		soNames[lib.SOName] = true
		assert.True(t, strings.HasPrefix(lib.Hash, "sha256:"), "hash must have sha256: prefix")
		assert.NotEmpty(t, lib.Path)
	}
	assert.True(t, soNames[lib1Path], "lib1.dylib must be in DynLibDeps")
	assert.True(t, soNames[lib2Path], "lib2.dylib must be in DynLibDeps")
}

// TestAnalyze_IndirectDeps_RpathFromDylib verifies that @rpath entries in a child
// .dylib's own LC_RPATH are used to resolve its dependencies, not the root binary's rpaths.
func TestAnalyze_IndirectDeps_RpathFromDylib(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	tmp := realPath(t, t.TempDir())

	// lib2.dylib lives in a subdirectory; its install name is resolved via @rpath.
	lib2Dir := filepath.Join(tmp, "libdir2")
	require.NoError(t, os.MkdirAll(lib2Dir, 0o700))
	lib2Path := filepath.Join(lib2Dir, "lib2.dylib")
	require.NoError(t, os.WriteFile(lib2Path,
		machodylibtesting.BuildMachOWithDeps(nativeCPU, nil, nil, nil), 0o600))

	// lib1.dylib: references lib2 via @rpath, carries its own LC_RPATH pointing to lib2Dir.
	lib1Path := filepath.Join(tmp, "lib1.dylib")
	require.NoError(t, os.WriteFile(lib1Path,
		machodylibtesting.BuildMachOWithDeps(nativeCPU,
			[]string{"@rpath/lib2.dylib"},
			nil,
			[]string{lib2Dir}), 0o600))

	// root.bin: references lib1 by absolute path (no rpath expansion needed for lib1).
	rootPath := filepath.Join(tmp, "root.bin")
	require.NoError(t, os.WriteFile(rootPath,
		machodylibtesting.BuildMachOWithDeps(nativeCPU, []string{lib1Path}, nil, nil), 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	libs, warnings, err := a.Analyze(rootPath)
	require.NoError(t, err)
	assert.Nil(t, warnings)
	require.Len(t, libs, 2, "lib1 and lib2 must both be recorded")

	soNames := make(map[string]bool, len(libs))
	for _, lib := range libs {
		soNames[lib.SOName] = true
	}
	assert.True(t, soNames[lib1Path], "lib1.dylib must be in DynLibDeps")
	assert.True(t, soNames["@rpath/lib2.dylib"], "@rpath/lib2.dylib must be in DynLibDeps")
}

// TestAnalyze_CircularDeps verifies that circular dependencies (A→B→A) do not
// cause an infinite loop; the visited set prevents redundant processing.
func TestAnalyze_CircularDeps(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	tmp := realPath(t, t.TempDir())

	libAPath := filepath.Join(tmp, "libA.dylib")
	libBPath := filepath.Join(tmp, "libB.dylib")

	// Write libA first; it references libB (which does not yet exist – the bytes
	// embed the path string; the file is resolved later at analysis time).
	require.NoError(t, os.WriteFile(libAPath,
		machodylibtesting.BuildMachOWithDeps(nativeCPU, []string{libBPath}, nil, nil), 0o600))
	require.NoError(t, os.WriteFile(libBPath,
		machodylibtesting.BuildMachOWithDeps(nativeCPU, []string{libAPath}, nil, nil), 0o600))

	rootPath := filepath.Join(tmp, "root.bin")
	require.NoError(t, os.WriteFile(rootPath,
		machodylibtesting.BuildMachOWithDeps(nativeCPU, []string{libAPath}, nil, nil), 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	libs, warnings, err := a.Analyze(rootPath)
	require.NoError(t, err, "circular dependencies must not cause an error or infinite loop")
	assert.Nil(t, warnings)

	soNames := make(map[string]bool, len(libs))
	for _, lib := range libs {
		soNames[lib.SOName] = true
	}
	assert.True(t, soNames[libAPath], "libA must appear in DynLibDeps")
	assert.True(t, soNames[libBPath], "libB must appear in DynLibDeps")
}

// TestAnalyze_RecursionDepthExceeded verifies that Analyze returns an
// ErrRecursionDepthExceeded when the dependency chain exceeds MaxRecursionDepth.
func TestAnalyze_RecursionDepthExceeded(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	tmp := realPath(t, t.TempDir())

	// Build a chain: root → chain[0] → chain[1] → … → chain[MaxRecursionDepth-1] → chain[MaxRecursionDepth].
	// chain[i] is at depth i+1 in the BFS queue.
	// chain[MaxRecursionDepth] reaches depth MaxRecursionDepth+1 > MaxRecursionDepth, triggering the error
	// before any resolve attempt – it does not need to exist on disk.
	chain := make([]string, MaxRecursionDepth+1)
	for i := range chain {
		chain[i] = filepath.Join(tmp, fmt.Sprintf("dep%02d.dylib", i))
	}

	// Write chain[0]..chain[MaxRecursionDepth-1]; each references the next.
	// chain[MaxRecursionDepth-1] references chain[MaxRecursionDepth] (does not need to exist).
	for i := 0; i < MaxRecursionDepth; i++ {
		require.NoError(t, os.WriteFile(chain[i],
			machodylibtesting.BuildMachOWithDeps(nativeCPU, []string{chain[i+1]}, nil, nil), 0o600))
	}

	rootPath := filepath.Join(tmp, "root.bin")
	require.NoError(t, os.WriteFile(rootPath,
		machodylibtesting.BuildMachOWithDeps(nativeCPU, []string{chain[0]}, nil, nil), 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	a := NewMachODynLibAnalyzer(fs)

	_, _, err := a.Analyze(rootPath)
	require.Error(t, err, "chain deeper than MaxRecursionDepth should return an error")

	var depthErr *dynlib.ErrRecursionDepthExceeded
	require.True(t, errors.As(err, &depthErr),
		"expected ErrRecursionDepthExceeded, got %T: %v", err, err)
	assert.Equal(t, MaxRecursionDepth+1, depthErr.Depth)
	assert.Equal(t, MaxRecursionDepth, depthErr.MaxDepth)
}

// TestHasDynamicLibDeps_SingleArch_NonSharedCacheDep verifies that HasDynamicLibDeps
// returns (true, nil) for a single-architecture Mach-O binary with at least one
// LC_LOAD_DYLIB entry pointing to a non-dyld-shared-cache path.
// The referenced library does not need to exist on disk: IsDyldSharedCacheLib
// returns false for /opt/homebrew/lib/, so the function returns true immediately.
func TestHasDynamicLibDeps_SingleArch_NonSharedCacheDep(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	buf := machodylibtesting.BuildMachOWithDeps(nativeCPU,
		[]string{"/opt/homebrew/lib/libfoo.dylib"},
		nil, nil)

	tmp := realPath(t, t.TempDir())
	path := filepath.Join(tmp, "test.bin")
	require.NoError(t, os.WriteFile(path, buf, 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	hasDeps, err := HasDynamicLibDeps(path, fs)
	require.NoError(t, err)
	assert.True(t, hasDeps, "single-arch Mach-O with non-dyld-cache dep should report true")
}

// TestHasDynamicLibDeps_FatBinary_NonSharedCacheDep verifies that HasDynamicLibDeps
// returns (true, nil) for a Fat binary whose native-arch slice has at least one
// LC_LOAD_DYLIB entry pointing to a non-dyld-shared-cache path.
func TestHasDynamicLibDeps_FatBinary_NonSharedCacheDep(t *testing.T) {
	nativeCPU := machodylibtesting.NativeCPU()
	nativeSlice := machodylibtesting.BuildMachOWithDeps(nativeCPU,
		[]string{"/opt/homebrew/lib/libfoo.dylib"},
		nil, nil)
	fatBin := machodylibtesting.BuildFatBinaryFromSlices([]machodylibtesting.FatSlice{{CPU: nativeCPU, Bytes: nativeSlice}})

	tmp := realPath(t, t.TempDir())
	path := filepath.Join(tmp, "fat_test.bin")
	require.NoError(t, os.WriteFile(path, fatBin, 0o600))

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	hasDeps, err := HasDynamicLibDeps(path, fs)
	require.NoError(t, err)
	assert.True(t, hasDeps, "Fat binary with non-dyld-cache dep should report true")
}
