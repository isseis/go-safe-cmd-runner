//go:build test && darwin

package filevalidator

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildMachOBinWithAbsoluteDepForTest builds a minimal 64-bit little-endian
// Mach-O binary (native architecture) with one LC_LOAD_DYLIB entry pointing
// to depPath.  Used to produce synthetic test binaries without a C compiler.
func buildMachOBinWithAbsoluteDepForTest(depPath string) []byte {
	// dylib_command layout:
	//   cmd(4) + cmdsize(4) + name_offset(4) + timestamp(4) + cur_ver(4) + compat_ver(4) = 24 bytes header
	// name follows at offset 24.
	const dylibHdrSize = 24
	totalLC := ((dylibHdrSize + len(depPath) + 1 + 3) &^ 3)

	buf := make([]byte, 32+totalLC)
	// mach_header_64
	binary.LittleEndian.PutUint32(buf[0:4], 0xFEEDFACF)        // MH_MAGIC_64
	binary.LittleEndian.PutUint32(buf[4:8], 0x0100000C)        // CPU_TYPE_ARM64 (arm64)
	binary.LittleEndian.PutUint32(buf[8:12], 0)                // cpusubtype
	binary.LittleEndian.PutUint32(buf[12:16], 2)               // MH_EXECUTE
	binary.LittleEndian.PutUint32(buf[16:20], 1)               // ncmds = 1
	binary.LittleEndian.PutUint32(buf[20:24], uint32(totalLC)) // sizeofcmds
	binary.LittleEndian.PutUint32(buf[24:28], 0)               // flags
	binary.LittleEndian.PutUint32(buf[28:32], 0)               // reserved
	// LC_LOAD_DYLIB at offset 32
	lc := buf[32:]
	binary.LittleEndian.PutUint32(lc[0:4], 0x0C) // LC_LOAD_DYLIB
	binary.LittleEndian.PutUint32(lc[4:8], uint32(totalLC))
	binary.LittleEndian.PutUint32(lc[8:12], dylibHdrSize) // name_offset
	copy(lc[dylibHdrSize:], depPath)
	return buf
}

// TestRecord_Force_MachO_UpdatesDynLibDeps verifies that SaveRecord with force=true
// re-runs Mach-O dynlib analysis and updates DynLibDeps with the new hash.
// This covers Phase 3 completion criterion: record --force で Mach-O の DynLibDeps が更新されること.
func TestRecord_Force_MachO_UpdatesDynLibDeps(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	// Create a minimal dylib file on disk (content is the binary's hash source).
	libPath := filepath.Join(tempDir, "libfoo.dylib")
	require.NoError(t, os.WriteFile(libPath, []byte("initial dylib content"), 0o600))

	// Create a synthetic Mach-O binary referencing libfoo.dylib.
	binPath := filepath.Join(tempDir, "testbin")
	require.NoError(t, os.WriteFile(binPath,
		buildMachOBinWithAbsoluteDepForTest(libPath), 0o700))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	v.SetMachODynLibAnalyzer(machodylib.NewMachODynLibAnalyzer(
		safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
	))

	// First record: DynLibDeps should capture libfoo.dylib with its initial hash.
	_, _, err = v.SaveRecord(binPath, false)
	require.NoError(t, err, "initial SaveRecord should succeed")

	rec1, err := v.LoadRecord(binPath)
	require.NoError(t, err)
	require.NotEmpty(t, rec1.DynLibDeps, "DynLibDeps must be populated after first record")
	hash1 := rec1.DynLibDeps[0].Hash

	// Modify the dylib to change its content hash.
	require.NoError(t, os.WriteFile(libPath, []byte("modified dylib content"), 0o600))

	// Force re-record: DynLibDeps must be updated with the new hash.
	_, _, err = v.SaveRecord(binPath, true)
	require.NoError(t, err, "force re-record should succeed")

	rec2, err := v.LoadRecord(binPath)
	require.NoError(t, err)
	require.NotEmpty(t, rec2.DynLibDeps, "DynLibDeps must still be present after force re-record")
	hash2 := rec2.DynLibDeps[0].Hash

	assert.NotEqual(t, hash1, hash2, "DynLibDeps hash must change after the dylib was modified")
}
