//go:build test && darwin

package machodylib

import (
	"encoding/binary"
	"os"
	"path/filepath"
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
