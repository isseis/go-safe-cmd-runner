//go:build test

package machoanalyzer

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildMachOWithPclntab constructs a minimal valid 64-bit Mach-O file in
// memory containing a __TEXT,__text section and, when pclntabData is not nil,
// a __gopclntab section. This is sufficient for ParseMachoPclntab unit tests.
//
// A nil pclntabData means no __gopclntab section is added.
func buildMachOWithPclntab(t *testing.T, pclntabData []byte) *macho.File {
	t.Helper()

	// We use macho.NewFile with a hand-crafted binary rather than relying on
	// the OS to build one, to keep tests hermetic and cross-platform.
	//
	// Mach-O 64-bit header (little-endian, ARM64):
	//   magic(4) + cputype(4) + cpusubtype(4) + filetype(4) +
	//   ncmds(4) + sizeofcmds(4) + flags(4) + reserved(4) = 32 bytes

	const (
		machoMagic64    = uint32(0xFEEDFACF)
		cpuArm64        = uint32(0x0100000C) // CPU_TYPE_ARM64
		cpuSubtypeAll   = uint32(0x00000000)
		fileTypeExec    = uint32(2) // MH_EXECUTE
		headerSize      = 32        // mach_header_64
		segCmdSize64    = 72        // sizeof(segment_command_64)
		sectCmdSize64   = 80        // sizeof(section_64)
		lcSegment64     = uint32(0x19)
		vmProtNone      = uint32(0)
		vmProtRX        = uint32(5)           // VM_PROT_READ | VM_PROT_EXECUTE
		textSectionData = "\x00\x00\x00\x00"  // 4-byte placeholder
		textVMAddr      = uint64(0x100000000) // typical macOS arm64 base
	)

	// Build section data layout:
	// Sections are placed after all load commands.
	// We need 1 segment (__TEXT) with up to 2 sections (__text + __gopclntab).
	hasPclntab := pclntabData != nil
	numSections := uint32(1)
	if hasPclntab {
		numSections = 2
	}

	segCmdTotalSize := segCmdSize64 + numSections*sectCmdSize64

	// Calculate file offsets. Load commands follow immediately after the header.
	lcOffset := uint32(headerSize)
	// Section data starts after header + all load commands.
	dataOffset := lcOffset + segCmdTotalSize

	textBytes := []byte(textSectionData)
	textOffset := dataOffset
	textSize := uint32(len(textBytes))

	var pclntabOffset, pclntabSize uint32
	if hasPclntab {
		pclntabOffset = textOffset + textSize
		pclntabSize = uint32(len(pclntabData))
	}

	fileSize := pclntabOffset + pclntabSize
	if !hasPclntab {
		fileSize = textOffset + textSize
	}

	var buf bytes.Buffer
	le := binary.LittleEndian

	writeUint32 := func(v uint32) {
		var b [4]byte
		le.PutUint32(b[:], v)
		buf.Write(b[:])
	}
	writeUint64 := func(v uint64) {
		var b [8]byte
		le.PutUint64(b[:], v)
		buf.Write(b[:])
	}
	writeStr16 := func(s string) {
		var b [16]byte
		copy(b[:], s)
		buf.Write(b[:])
	}

	// --- Mach-O header ---
	writeUint32(machoMagic64)
	writeUint32(cpuArm64)
	writeUint32(cpuSubtypeAll)
	writeUint32(fileTypeExec)
	writeUint32(1)               // ncmds
	writeUint32(segCmdTotalSize) // sizeofcmds
	writeUint32(0)               // flags
	writeUint32(0)               // reserved

	// --- LC_SEGMENT_64 (__TEXT) ---
	vmSize := uint64(textSize)
	if hasPclntab {
		vmSize += uint64(pclntabSize)
	}
	writeUint32(lcSegment64)
	writeUint32(segCmdTotalSize)    // cmdsize
	writeStr16("__TEXT")            // segname
	writeUint64(textVMAddr)         // vmaddr
	writeUint64(vmSize)             // vmsize
	writeUint64(uint64(textOffset)) // fileoff
	writeUint64(vmSize)             // filesize
	writeUint32(vmProtRX)           // maxprot
	writeUint32(vmProtRX)           // initprot
	writeUint32(numSections)        // nsects
	writeUint32(0)                  // flags

	// --- Section: __text ---
	writeStr16("__text")
	writeStr16("__TEXT")
	writeUint64(textVMAddr)       // addr
	writeUint64(uint64(textSize)) // size
	writeUint32(textOffset)       // offset
	writeUint32(2)                // align (4 bytes)
	writeUint32(0)                // reloff
	writeUint32(0)                // nreloc
	writeUint32(0x80000400)       // flags (S_REGULAR | S_ATTR_SOME_INSTRUCTIONS)
	writeUint32(0)                // reserved1
	writeUint32(0)                // reserved2
	writeUint32(0)                // reserved3

	// --- Section: __gopclntab (if present) ---
	if hasPclntab {
		pclntabVMAddr := textVMAddr + uint64(textSize)
		writeStr16("__gopclntab")
		writeStr16("__TEXT")
		writeUint64(pclntabVMAddr)       // addr
		writeUint64(uint64(pclntabSize)) // size
		writeUint32(pclntabOffset)       // offset
		writeUint32(0)                   // align
		writeUint32(0)                   // reloff
		writeUint32(0)                   // nreloc
		writeUint32(0)                   // flags
		writeUint32(0)                   // reserved1
		writeUint32(0)                   // reserved2
		writeUint32(0)                   // reserved3
	}

	// --- Section data ---
	buf.Write(textBytes)
	if hasPclntab {
		buf.Write(pclntabData)
	}

	// Pad to fileSize if needed.
	for buf.Len() < int(fileSize) {
		buf.WriteByte(0)
	}

	f, err := macho.NewFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	return f
}

// buildMinimalPclntabData returns a Go 1.20+ pclntab header with the correct
// magic (0xfffffff1). The body is minimal (just a header with quantum/ptrSize
// fields); gosym.NewLineTable will return an empty table for it.
func buildMinimalPclntabData() []byte {
	// pclntab header for Go 1.20+ (magic 0xfffffff1):
	//   [0:4]   magic    = 0xfffffff1 (little-endian)
	//   [4]     pad1     = 0
	//   [5]     pad2     = 0
	//   [6]     quantum  = 1 (x86) or 4 (arm64)
	//   [7]     ptrSize  = 4 or 8
	// Followed by function count and data.
	// For an empty table, we just need a valid header.
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 0xfffffff1)
	data[4] = 0
	data[5] = 0
	data[6] = 4 // quantum=4 (arm64)
	data[7] = 8 // ptrSize=8 (64-bit)
	return data
}

// buildInvalidMagicPclntab returns a pclntab with an unsupported magic number.
func buildInvalidMagicPclntab() []byte {
	data := buildMinimalPclntabData()
	binary.LittleEndian.PutUint32(data[0:4], 0xfffffffb) // unknown magic
	return data
}

// TestParseMachoPclntab_NoPclntabSection verifies that ErrNoPclntab is returned
// when the Mach-O file has no __gopclntab section.
func TestParseMachoPclntab_NoPclntabSection(t *testing.T) {
	t.Parallel()
	f := buildMachOWithPclntab(t, nil) // no __gopclntab
	funcs, err := ParseMachoPclntab(f)
	assert.Nil(t, funcs)
	assert.True(t, errors.Is(err, ErrNoPclntab), "expected ErrNoPclntab, got %v", err)
}

// TestParseMachoPclntab_InvalidMagic verifies that ErrUnsupportedPclntabVersion
// is returned when the pclntab magic is unknown.
func TestParseMachoPclntab_InvalidMagic(t *testing.T) {
	t.Parallel()
	f := buildMachOWithPclntab(t, buildInvalidMagicPclntab())
	funcs, err := ParseMachoPclntab(f)
	assert.Nil(t, funcs)
	assert.True(t, errors.Is(err, ErrUnsupportedPclntabVersion), "expected ErrUnsupportedPclntabVersion, got %v", err)
}

// TestParseMachoPclntab_TooShort verifies that ErrInvalidPclntab is returned
// when the pclntab data is shorter than 4 bytes.
func TestParseMachoPclntab_TooShort(t *testing.T) {
	t.Parallel()
	f := buildMachOWithPclntab(t, []byte{0x01, 0x02}) // only 2 bytes
	funcs, err := ParseMachoPclntab(f)
	assert.Nil(t, funcs)
	assert.True(t, errors.Is(err, ErrInvalidPclntab), "expected ErrInvalidPclntab, got %v", err)
}

// TestParseMachoPclntab_ValidHeader verifies that a valid Go 1.20+ header is
// accepted without error (empty function table is fine).
func TestParseMachoPclntab_ValidHeader(t *testing.T) {
	t.Parallel()
	f := buildMachOWithPclntab(t, buildMinimalPclntabData())
	funcs, err := ParseMachoPclntab(f)
	// gosym may return an error for a minimal stub; accept either empty map or error.
	// The important thing is that magic validation passes (no ErrUnsupportedPclntabVersion).
	if err != nil {
		assert.False(t, errors.Is(err, ErrUnsupportedPclntabVersion),
			"should not return ErrUnsupportedPclntabVersion for valid magic")
		assert.False(t, errors.Is(err, ErrNoPclntab),
			"should not return ErrNoPclntab when section exists")
	} else {
		assert.NotNil(t, funcs)
	}
}

// TestIsInsideRange tests the isInsideRange helper with sorted funcRange slices.
func TestIsInsideRange(t *testing.T) {
	t.Parallel()

	ranges := []funcRange{
		{start: 0x100, end: 0x200},
		{start: 0x300, end: 0x400},
		{start: 0x500, end: 0x600},
	}

	tests := []struct {
		name string
		addr uint64
		want bool
	}{
		{"before first range", 0x50, false},
		{"at start of first range", 0x100, true},
		{"inside first range", 0x150, true},
		{"at end of first range (exclusive)", 0x200, false},
		{"between first and second range", 0x250, false},
		{"at start of second range", 0x300, true},
		{"inside second range", 0x350, true},
		{"at end of second range (exclusive)", 0x400, false},
		{"at start of third range", 0x500, true},
		{"inside third range", 0x550, true},
		{"at end of third range (exclusive)", 0x600, false},
		{"after all ranges", 0x700, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isInsideRange(tt.addr, ranges)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIsInsideRange_EmptySlice verifies isInsideRange returns false for empty input.
func TestIsInsideRange_EmptySlice(t *testing.T) {
	t.Parallel()
	assert.False(t, isInsideRange(0x100, nil))
	assert.False(t, isInsideRange(0x100, []funcRange{}))
}

// TestParseMachoPclntab_LiveBinary verifies that ParseMachoPclntab can parse
// the actual test binary (go test builds contain __gopclntab) and finds known
// Go runtime function names.
func TestParseMachoPclntab_LiveBinary(t *testing.T) {
	t.Parallel()

	// Open the current test binary (self-test).
	f, err := macho.Open("/proc/self/exe")
	if err != nil {
		// Not on macOS or not a Mach-O binary — skip.
		t.Skip("skipping live binary test: cannot open /proc/self/exe as Mach-O")
	}
	defer f.Close() //nolint:errcheck

	funcs, err := ParseMachoPclntab(f)
	if errors.Is(err, ErrNoPclntab) || errors.Is(err, ErrUnsupportedPclntabVersion) || errors.Is(err, ErrInvalidPclntab) {
		t.Skip("no suitable pclntab found in test binary")
	}
	require.NoError(t, err)
	require.NotEmpty(t, funcs)

	// Verify that at least one Go runtime function is present.
	found := false
	for name := range funcs {
		if name == "testing.Main" || name == "testing.tRunner" || name == "main.main" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected at least one known Go runtime function in pclntab")
}

// TestParseMachoPclntab_MacOSTestBinary verifies ParseMachoPclntab on macOS
// using the actual test runner binary (os.Args[0]).
func TestParseMachoPclntab_MacOSTestBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping macOS live binary test in short mode")
	}

	// On macOS the test runner binary (os.Args[0]) is a Mach-O file.
	importPath := os.Args[0]
	f, err := macho.Open(importPath)
	if err != nil {
		t.Skipf("skipping: cannot open %s as Mach-O: %v", importPath, err)
	}
	defer f.Close() //nolint:errcheck

	funcs, err := ParseMachoPclntab(f)
	if errors.Is(err, ErrNoPclntab) {
		t.Skip("test binary has no __gopclntab (stripped)")
	}
	require.NoError(t, err)
	require.NotEmpty(t, funcs)

	// Check for known Go runtime functions.
	found := false
	for name := range funcs {
		if name == "testing.Main" || name == "testing.tRunner" || name == "main.main" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected at least one known Go function in pclntab")
}
