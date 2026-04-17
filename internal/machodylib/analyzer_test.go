//go:build test && darwin

package machodylib

import (
	"debug/macho"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

// nativeAndNonNativeCPU returns the native Mach-O CPU type for the running
// architecture and a CPU type that will NOT match, used to build deterministic
// Fat binary fixtures for slice-selection tests.
func nativeAndNonNativeCPU() (native, nonNative macho.Cpu) {
	switch runtime.GOARCH {
	case "arm64":
		return macho.CpuArm64, macho.CpuAmd64
	default: // amd64 and any future architectures
		return macho.CpuAmd64, macho.CpuArm64
	}
}

// buildMinimalMachOSlice returns a 32-byte minimal 64-bit little-endian Mach-O
// header for cpuType with no load commands.
func buildMinimalMachOSlice(cpuType macho.Cpu) []byte {
	buf := make([]byte, 32)                             // mach_header_64: 8 × uint32
	binary.LittleEndian.PutUint32(buf[0:4], 0xFEEDFACF) // MH_MAGIC_64 (LE)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(cpuType))
	binary.LittleEndian.PutUint32(buf[8:12], 0)  // cpusubtype = ALL
	binary.LittleEndian.PutUint32(buf[12:16], 2) // filetype = MH_EXECUTE
	binary.LittleEndian.PutUint32(buf[16:20], 0) // ncmds = 0
	binary.LittleEndian.PutUint32(buf[20:24], 0) // sizeofcmds = 0
	binary.LittleEndian.PutUint32(buf[24:28], 0) // flags = 0
	binary.LittleEndian.PutUint32(buf[28:32], 0) // reserved = 0
	return buf
}

// buildFatBinary returns a minimal Fat Mach-O binary (FAT_MAGIC header followed
// by one minimal Mach-O slice per cpuType).  Each slice has no load commands.
func buildFatBinary(cpuTypes []macho.Cpu) []byte {
	nArch := len(cpuTypes)
	fatHdrSize := 8 + 20*nArch // magic(4)+narch(4) + nArch*fatArch(5×4)
	sliceSize := 32
	buf := make([]byte, fatHdrSize+sliceSize*nArch)

	binary.BigEndian.PutUint32(buf[0:4], 0xCAFEBABE) // FAT_MAGIC
	binary.BigEndian.PutUint32(buf[4:8], uint32(nArch))

	for i, cpu := range cpuTypes {
		archOff := 8 + i*20
		sliceOff := fatHdrSize + i*sliceSize
		binary.BigEndian.PutUint32(buf[archOff:archOff+4], uint32(cpu))           // cputype
		binary.BigEndian.PutUint32(buf[archOff+4:archOff+8], 0)                   // cpusubtype
		binary.BigEndian.PutUint32(buf[archOff+8:archOff+12], uint32(sliceOff))   // offset
		binary.BigEndian.PutUint32(buf[archOff+12:archOff+16], uint32(sliceSize)) // size
		binary.BigEndian.PutUint32(buf[archOff+16:archOff+20], 0)                 // align
		copy(buf[sliceOff:], buildMinimalMachOSlice(cpu))
	}

	return buf
}

// writeFatBinary writes a Fat binary to a test-scoped temp dir and returns the
// resolved absolute path (symlinks expanded for safefileio compatibility).
func writeFatBinary(t *testing.T, name string, cpuTypes []macho.Cpu) string {
	t.Helper()
	tmp := realPath(t, t.TempDir())
	path := filepath.Join(tmp, name)
	require.NoError(t, os.WriteFile(path, buildFatBinary(cpuTypes), 0o600))

	return path
}

// --- Fat binary slice-selection tests ---

// TestOpenMachO_FatBinary_MatchingSlice verifies that openMachO selects the
// native-arch slice from a Fat binary and returns a valid *macho.File whose
// Cpu field matches the native CPU type.
func TestOpenMachO_FatBinary_MatchingSlice(t *testing.T) {
	nativeCPU, _ := nativeAndNonNativeCPU()
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
	_, nonNativeCPU := nativeAndNonNativeCPU()
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
	nativeCPU, _ := nativeAndNonNativeCPU()
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
	_, nonNativeCPU := nativeAndNonNativeCPU()
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
	nativeCPU, _ := nativeAndNonNativeCPU()
	path := writeFatBinary(t, "fat_native.bin", []macho.Cpu{nativeCPU})

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	hasDeps, err := HasDynamicLibDeps(path, fs)
	require.NoError(t, err)
	assert.False(t, hasDeps, "Fat binary with no load commands should report no deps")
}

// TestHasDynamicLibDeps_FatBinary_NoMatchingSlice verifies that HasDynamicLibDeps
// returns (false, nil) for a Fat binary that has no native-arch slice.
func TestHasDynamicLibDeps_FatBinary_NoMatchingSlice(t *testing.T) {
	_, nonNativeCPU := nativeAndNonNativeCPU()
	path := writeFatBinary(t, "fat_non_native.bin", []macho.Cpu{nonNativeCPU})

	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	hasDeps, err := HasDynamicLibDeps(path, fs)
	require.NoError(t, err)
	assert.False(t, hasDeps, "Fat binary with no matching arch should report no deps")
}
