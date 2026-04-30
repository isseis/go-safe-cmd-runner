//go:build test

package machoanalyzer

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nopEncoding is a common arm64 NOP instruction.
const nopEncoding = uint32(0xD503201F)

// osWrapFS is a safefileio.FileSystem that opens files using os.Open,
// used to read testdata fixtures without the security constraints of osFS.
type osWrapFS struct{}

func (osWrapFS) SafeOpenFile(name string, _ int, _ os.FileMode) (safefileio.File, error) {
	return os.Open(name)
}
func (osWrapFS) AtomicMoveFile(_, _ string, _ os.FileMode) error      { return nil }
func (osWrapFS) GetGroupMembership() *groupmembership.GroupMembership { return nil }
func (osWrapFS) Remove(_ string) error                                { return nil }

// buildArm64MachO builds a minimal arm64 Mach-O binary in memory.
// The given instructions (as 32-bit little-endian words) are placed in
// the __TEXT,__text section starting at virtual address 0x100000000.
func buildArm64MachO(t *testing.T, instructions []uint32) []byte {
	t.Helper()

	const (
		headerSize    = 32                                 // mach_header_64
		segCmdSize    = 72                                 // segment_command_64 (without sections)
		sectSize      = 80                                 // section_64
		textOffset    = headerSize + segCmdSize + sectSize // 184 = 0xB8
		lcSegment64   = 0x19
		mhExecute     = 0x2
		cpuArm64      = 0x0100000C
		vmBase        = uint64(0x100000000)
		sAttrPureInst = uint32(0x80000000)
	)

	sizeofcmds := uint32(segCmdSize + sectSize)
	instBytes := make([]byte, len(instructions)*4)
	for i, inst := range instructions {
		binary.LittleEndian.PutUint32(instBytes[i*4:], inst)
	}
	sectDataSize := uint32(len(instBytes))

	var buf bytes.Buffer
	writeU32 := func(v uint32) {
		b := [4]byte{}
		binary.LittleEndian.PutUint32(b[:], v)
		buf.Write(b[:])
	}
	writeU64 := func(v uint64) {
		b := [8]byte{}
		binary.LittleEndian.PutUint64(b[:], v)
		buf.Write(b[:])
	}
	writePad16 := func(s string) {
		b := [16]byte{}
		copy(b[:], s)
		buf.Write(b[:])
	}

	// mach_header_64 (32 bytes)
	writeU32(0xFEEDFACF) // magic
	writeU32(cpuArm64)   // cputype
	writeU32(0)          // cpusubtype
	writeU32(mhExecute)  // filetype
	writeU32(1)          // ncmds
	writeU32(sizeofcmds) // sizeofcmds
	writeU32(0)          // flags
	writeU32(0)          // reserved

	// segment_command_64 (72 bytes)
	writeU32(lcSegment64)                   // cmd
	writeU32(uint32(segCmdSize + sectSize)) // cmdsize
	writePad16("__TEXT")                    // segname
	writeU64(vmBase)                        // vmaddr
	writeU64(0x1000)                        // vmsize
	writeU64(uint64(textOffset))            // fileoff
	writeU64(uint64(sectDataSize))          // filesize
	writeU32(7)                             // maxprot
	writeU32(5)                             // initprot
	writeU32(1)                             // nsects
	writeU32(0)                             // flags

	// section_64 (80 bytes)
	writePad16("__text")           // sectname
	writePad16("__TEXT")           // segname
	writeU64(vmBase)               // addr
	writeU64(uint64(sectDataSize)) // size
	writeU32(uint32(textOffset))   // offset
	writeU32(2)                    // align
	writeU32(0)                    // reloff
	writeU32(0)                    // nreloc
	writeU32(sAttrPureInst)        // flags
	writeU32(0)                    // reserved1
	writeU32(0)                    // reserved2
	writeU32(0)                    // reserved3

	// section data
	buf.Write(instBytes)

	require.Equal(t, textOffset+int(sectDataSize), buf.Len())
	return buf.Bytes()
}

// parseMachOFromBytes parses a Mach-O binary from a byte slice using macho.NewFile.
func parseMachOFromBytes(t *testing.T, data []byte) *macho.File {
	t.Helper()
	f, err := macho.NewFile(bytes.NewReader(data))
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	return f
}

// TestCollectSVCAddresses_Arm64WithSVC verifies that collectSVCAddresses returns
// the virtual address of a single svc #0x80 instruction in an arm64 Mach-O.
func TestCollectSVCAddresses_Arm64WithSVC(t *testing.T) {
	path := testdataPath("svc_only_arm64")
	skipIfNotExist(t, path)

	f, err := macho.Open(path)
	require.NoError(t, err)
	defer f.Close()

	addrs, err := collectSVCAddresses(f)
	require.NoError(t, err)
	assert.NotEmpty(t, addrs, "expected at least one svc #0x80 address")
	for _, addr := range addrs {
		assert.NotZero(t, addr)
	}
}

// TestCollectSVCAddresses_Arm64NoSVC verifies that collectSVCAddresses returns nil
// for an arm64 Mach-O that contains no svc #0x80 instruction.
func TestCollectSVCAddresses_Arm64NoSVC(t *testing.T) {
	path := testdataPath("no_network_macho_arm64")
	skipIfNotExist(t, path)

	f, err := macho.Open(path)
	require.NoError(t, err)
	defer f.Close()

	addrs, err := collectSVCAddresses(f)
	require.NoError(t, err)
	assert.Nil(t, addrs, "expected nil for no-svc binary")
}

// TestCollectSVCAddresses_NonArm64 verifies that collectSVCAddresses returns nil
// for a non-arm64 Mach-O (x86_64), regardless of instruction content.
func TestCollectSVCAddresses_NonArm64(t *testing.T) {
	path := testdataPath("network_macho_x86_64")
	skipIfNotExist(t, path)

	f, err := macho.Open(path)
	require.NoError(t, err)
	defer f.Close()

	addrs, err := collectSVCAddresses(f)
	require.NoError(t, err)
	assert.Nil(t, addrs, "expected nil for x86_64 binary")
}

// TestCollectSVCAddresses_MultipleSVC verifies that collectSVCAddresses returns
// all virtual addresses when multiple svc #0x80 instructions are present.
func TestCollectSVCAddresses_MultipleSVC(t *testing.T) {
	// Build a minimal arm64 Mach-O with 3 instructions: nop, svc, nop, svc, nop, svc
	instructions := []uint32{
		nopEncoding, // offset 0  (addr 0x100000000) — no svc
		svcEncoding, // offset 4  (addr 0x100000004) — svc #1
		nopEncoding, // offset 8  (addr 0x100000008) — no svc
		svcEncoding, // offset 12 (addr 0x10000000C) — svc #2
		nopEncoding, // offset 16 (addr 0x100000010) — no svc
		svcEncoding, // offset 20 (addr 0x100000014) — svc #3
	}
	data := buildArm64MachO(t, instructions)
	f := parseMachOFromBytes(t, data)

	addrs, err := collectSVCAddresses(f)
	require.NoError(t, err)
	require.Len(t, addrs, 3, "expected exactly 3 svc addresses")
	assert.Equal(t, uint64(0x100000004), addrs[0])
	assert.Equal(t, uint64(0x10000000C), addrs[1])
	assert.Equal(t, uint64(0x100000014), addrs[2])
}

// TestScanSVCAddrs_NotMacho verifies that ScanSVCAddrs
// returns nil, nil for a non-Mach-O file (e.g., a shell script).
func TestScanSVCAddrs_NotMacho(t *testing.T) {
	path := testdataPath("script.sh")
	skipIfNotExist(t, path)

	addrs, err := ScanSVCAddrs(path, osWrapFS{})
	require.NoError(t, err)
	assert.Nil(t, addrs, "expected nil for non-Mach-O file")
}

// TestScanSVCAddrs_FatBinary verifies that ScanSVCAddrs
// processes a Fat binary, scanning only arm64 slices for svc #0x80.
func TestScanSVCAddrs_FatBinary(t *testing.T) {
	path := testdataPath("fat_binary")
	skipIfNotExist(t, path)

	// fat_binary contains arm64 and x86_64 slices; both lack svc #0x80.
	// Verify: no error, returns nil (no svc in arm64 slice).
	addrs, err := ScanSVCAddrs(path, osWrapFS{})
	require.NoError(t, err)
	assert.Nil(t, addrs, "expected nil: arm64 slice in fat_binary has no svc #0x80")
}

// TestCollectSVCAddresses_HasOrNot verifies collectSVCAddresses result shape
// for binaries with and without svc #0x80 instructions.
func TestCollectSVCAddresses_HasOrNot(t *testing.T) {
	t.Run("with_svc", func(t *testing.T) {
		path := testdataPath("svc_only_arm64")
		skipIfNotExist(t, path)

		f, err := macho.Open(path)
		require.NoError(t, err)
		defer f.Close()

		addrs, err := collectSVCAddresses(f)
		require.NoError(t, err)
		require.NotEmpty(t, addrs)
	})

	t.Run("without_svc", func(t *testing.T) {
		path := testdataPath("no_network_macho_arm64")
		skipIfNotExist(t, path)

		f, err := macho.Open(path)
		require.NoError(t, err)
		defer f.Close()

		addrs, err := collectSVCAddresses(f)
		require.NoError(t, err)
		assert.Empty(t, addrs, "expected no svc addresses for no-svc binary")
	})
}
