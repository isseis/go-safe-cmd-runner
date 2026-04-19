//go:build test

package libccache

import (
	"bytes"
	"debug/macho"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildMachoBytes builds a minimal Mach-O binary (arm64) and returns its bytes.
// Contains a single socket wrapper (syscall 97).
func buildMachoBytes(t *testing.T) []byte {
	t.Helper()
	text := buildInstructions(movzX16(97), svcMacOS)
	syms := []testSym{
		{Name: "socket", Value: testTextVMAddr, Sect: 1, Type: 0x0F},
	}
	mf := buildTestMacho(t, macho.CpuArm64, text, syms)

	// Re-serialize by reading raw bytes from the built binary.
	// Since buildTestMacho uses bytes.NewReader, we can rebuild directly.
	text2 := buildInstructions(movzX16(97), svcMacOS)
	syms2 := []testSym{
		{Name: "socket", Value: testTextVMAddr, Sect: 1, Type: 0x0F},
	}
	_ = mf // already verified by buildTestMacho

	// Rebuild the raw binary representation directly.
	return buildRawMachoBytes(t, macho.CpuArm64, text2, syms2)
}

// buildRawMachoBytes builds a raw Mach-O binary byte slice (does not call NewFile).
// This is used to produce getData() callbacks for tests.
func buildRawMachoBytes(t *testing.T, cpu macho.Cpu, textBytes []byte, syms []testSym) []byte {
	t.Helper()

	const (
		lcSegment64    = uint32(0x19)
		lcSymtab       = uint32(0x02)
		magic64        = uint32(0xFEEDFACF)
		hdrSize        = uint32(32)
		segCmdBodySize = uint32(72)
		sect64Size     = uint32(80)
		symtabCmdSz    = uint32(24)
		nlist64Size    = uint32(16)
	)

	nsyms := uint32(len(syms))
	strtab := []byte{0}
	strOffsets := make([]uint32, nsyms)
	for i, s := range syms {
		strOffsets[i] = uint32(len(strtab))
		strtab = append(strtab, []byte(s.Name)...)
		strtab = append(strtab, 0)
	}
	strtabSize := uint32(len(strtab))

	segCmdTotal := segCmdBodySize + sect64Size
	sizeofcmds := segCmdTotal + symtabCmdSz
	textOffset := hdrSize + sizeofcmds
	symtabOffset := textOffset + uint32(len(textBytes))
	strtabOffset := symtabOffset + nsyms*nlist64Size

	bo := make([]byte, 0, 512)
	buf := bytes.NewBuffer(bo)

	wr := func(v interface{}) {
		require.NoError(t, writeLEValue(buf, v))
	}

	// FileHeader
	wr(magic64)
	wr(uint32(cpu))
	wr(uint32(0)) // cpusubtype
	wr(uint32(6)) // MH_DYLIB
	wr(uint32(2)) // ncmds
	wr(sizeofcmds)
	wr(uint32(0)) // flags
	wr(uint32(0)) // reserved

	// LC_SEGMENT_64
	seg := [16]byte{}
	copy(seg[:], "__TEXT")
	wr(lcSegment64)
	wr(segCmdTotal)
	buf.Write(seg[:])
	wr(testTextVMAddr)         // vmaddr
	wr(uint64(len(textBytes))) // vmsize
	wr(uint64(textOffset))     // fileoff
	wr(uint64(len(textBytes))) // filesize
	wr(uint32(5))              // maxprot
	wr(uint32(5))              // initprot
	wr(uint32(1))              // nsects
	wr(uint32(0))              // flags

	// section_64
	sect := [16]byte{}
	copy(sect[:], "__text")
	buf.Write(sect[:])
	buf.Write(seg[:])
	wr(testTextVMAddr)
	wr(uint64(len(textBytes)))
	wr(textOffset) // offset (uint32)
	wr(uint32(2))
	wr(uint32(0))
	wr(uint32(0))
	wr(uint32(0x80000400))
	wr(uint32(0))
	wr(uint32(0))
	wr(uint32(0))

	// LC_SYMTAB
	wr(lcSymtab)
	wr(symtabCmdSz)
	wr(symtabOffset)
	wr(nsyms)
	wr(strtabOffset)
	wr(strtabSize)

	// text
	buf.Write(textBytes)

	// symbol table
	for i, s := range syms {
		wr(strOffsets[i])
		buf.WriteByte(s.Type)
		buf.WriteByte(s.Sect)
		wr(uint16(0))
		wr(s.Value)
	}
	buf.Write(strtab)

	return buf.Bytes()
}

// writeLEValue encodes v as little-endian bytes into buf.
func writeLEValue(buf *bytes.Buffer, v interface{}) error {
	writeBytes := func(b []byte) { buf.Write(b) }
	switch x := v.(type) {
	case uint8:
		buf.WriteByte(x)
	case uint16:
		writeBytes([]byte{byte(x), byte(x >> 8)})
	case uint32:
		writeBytes([]byte{byte(x), byte(x >> 8), byte(x >> 16), byte(x >> 24)})
	case uint64:
		writeBytes([]byte{
			byte(x), byte(x >> 8), byte(x >> 16), byte(x >> 24),
			byte(x >> 32), byte(x >> 40), byte(x >> 48), byte(x >> 56),
		})
	default:
		return errors.New("unsupported type")
	}
	return nil
}

// newMachoTestCacheManager creates a MachoLibSystemCacheManager backed by a temp dir.
func newMachoTestCacheManager(t *testing.T) (*MachoLibSystemCacheManager, string) {
	t.Helper()
	cacheDir := filepath.Join(t.TempDir(), "macho-cache")
	mgr, err := NewMachoLibSystemCacheManager(cacheDir)
	require.NoError(t, err)
	return mgr, cacheDir
}

// TestMachoLibSystemCacheManager_CreateAndLoad verifies that the manager creates a cache
// on miss and returns the cached result on the second call.
func TestMachoLibSystemCacheManager_CreateAndLoad(t *testing.T) {
	mgr, cacheDir := newMachoTestCacheManager(t)
	rawBytes := buildMachoBytes(t)
	hash := "sha256:aabbccddeeff"
	const libPath = "/usr/lib/system/libsystem_kernel.dylib"

	callCount := 0
	getData := func() ([]byte, error) {
		callCount++
		return rawBytes, nil
	}

	// First call: cache miss, getData called.
	wrappers1, err := mgr.GetOrCreate(libPath, hash, getData)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
	require.Len(t, wrappers1, 1)
	assert.Equal(t, "socket", wrappers1[0].Name)

	// Cache file must exist.
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	// Second call: cache hit, getData not called.
	wrappers2, err := mgr.GetOrCreate(libPath, hash, getData)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "getData should not be called on cache hit")
	assert.Equal(t, wrappers1, wrappers2)
}

// TestMachoLibSystemCacheManager_InvalidatesOnHashMismatch verifies that a cache file with
// a different hash is invalidated.
func TestMachoLibSystemCacheManager_InvalidatesOnHashMismatch(t *testing.T) {
	mgr, _ := newMachoTestCacheManager(t)
	rawBytes := buildMachoBytes(t)
	const libPath = "/usr/lib/system/libsystem_kernel.dylib"

	callCount := 0
	getData := func() ([]byte, error) {
		callCount++
		return rawBytes, nil
	}

	_, err := mgr.GetOrCreate(libPath, "sha256:aabb", getData)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Second call with different hash: should invalidate and call getData again.
	_, err = mgr.GetOrCreate(libPath, "sha256:ccdd", getData)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "getData should be called again on hash mismatch")
}

// TestMachoLibSystemCacheManager_InvalidatesOnSchemaMismatch verifies that a cache file with
// a wrong SchemaVersion is invalidated.
func TestMachoLibSystemCacheManager_InvalidatesOnSchemaMismatch(t *testing.T) {
	mgr, cacheDir := newMachoTestCacheManager(t)
	rawBytes := buildMachoBytes(t)
	const libPath = "/usr/lib/system/libsystem_kernel.dylib"
	const hash = "sha256:aabbcc"

	// Write a cache file with a wrong schema version.
	badCache := LibcCacheFile{
		SchemaVersion:   LibcCacheSchemaVersion + 1, // wrong version
		LibPath:         libPath,
		LibHash:         hash,
		SyscallWrappers: []WrapperEntry{{Name: "old", Number: 1}},
	}
	enc, err := json.MarshalIndent(badCache, "", "  ")
	require.NoError(t, err)

	enc2 := mgr.pathEnc
	encodedName, err := enc2.Encode(libPath)
	require.NoError(t, err)
	cacheFilePath := filepath.Join(cacheDir, encodedName)
	require.NoError(t, os.WriteFile(cacheFilePath, enc, 0o600))

	callCount := 0
	getData := func() ([]byte, error) {
		callCount++
		return rawBytes, nil
	}

	wrappers, err := mgr.GetOrCreate(libPath, hash, getData)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "getData should be called when schema version is wrong")
	require.Len(t, wrappers, 1)
	assert.Equal(t, "socket", wrappers[0].Name)
}

// TestMachoLibSystemCacheManager_ReparsesBrokenCache verifies that a corrupted cache file
// is ignored and re-analyzed.
func TestMachoLibSystemCacheManager_ReparsesBrokenCache(t *testing.T) {
	mgr, cacheDir := newMachoTestCacheManager(t)
	rawBytes := buildMachoBytes(t)
	const libPath = "/usr/lib/system/libsystem_kernel.dylib"
	const hash = "sha256:aabbcc"

	// Write broken JSON.
	enc2 := mgr.pathEnc
	encodedName, err := enc2.Encode(libPath)
	require.NoError(t, err)
	cacheFilePath := filepath.Join(cacheDir, encodedName)
	require.NoError(t, os.WriteFile(cacheFilePath, []byte("not valid json{{{}"), 0o600))

	callCount := 0
	getData := func() ([]byte, error) {
		callCount++
		return rawBytes, nil
	}

	wrappers, err := mgr.GetOrCreate(libPath, hash, getData)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "getData should be called when cache is broken JSON")
	require.Len(t, wrappers, 1)
}

// TestMachoLibSystemCacheManager_GetDataError verifies that a getData error is propagated.
func TestMachoLibSystemCacheManager_GetDataError(t *testing.T) {
	mgr, _ := newMachoTestCacheManager(t)
	const libPath = "/usr/lib/system/libsystem_kernel.dylib"

	getData := func() ([]byte, error) {
		return nil, errors.New("disk read error")
	}

	_, err := mgr.GetOrCreate(libPath, "sha256:aabb", getData)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLibcFileNotAccessible)
}
