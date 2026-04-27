//go:build test

package elfanalyzer

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildELF64WithPclntab constructs a minimal valid ELF64 binary in memory
// containing a single .gopclntab section with the given data.
// The resulting bytes can be opened with elf.NewFile for unit testing.
func buildELF64WithPclntab(pclntabData []byte) []byte {
	var buf bytes.Buffer

	// shstrtab layout: \0 .text\0 .gopclntab\0 .shstrtab\0
	// offsets:          0  1      7            18
	shstrtab := []byte("\x00.text\x00.gopclntab\x00.shstrtab\x00")

	const elfHeaderSize = 64
	const shEntrySize = 64
	const numSections = 4 // null, .text, .gopclntab, .shstrtab

	pclntabOffset := elfHeaderSize
	shstrtabOffset := pclntabOffset + len(pclntabData)

	shOffset := shstrtabOffset + len(shstrtab)
	if shOffset%8 != 0 {
		shOffset += 8 - (shOffset % 8)
	}

	// ELF header
	var header [elfHeaderSize]byte
	copy(header[0:4], []byte{0x7f, 'E', 'L', 'F'})
	header[4] = byte(elf.ELFCLASS64)
	header[5] = byte(elf.ELFDATA2LSB)
	header[6] = byte(elf.EV_CURRENT)
	binary.LittleEndian.PutUint16(header[16:18], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(header[18:20], uint16(elf.EM_X86_64))
	binary.LittleEndian.PutUint32(header[20:24], uint32(elf.EV_CURRENT))
	binary.LittleEndian.PutUint64(header[40:48], uint64(shOffset))      // e_shoff
	binary.LittleEndian.PutUint16(header[52:54], uint16(elfHeaderSize)) // e_ehsize
	binary.LittleEndian.PutUint16(header[58:60], uint16(shEntrySize))   // e_shentsize
	binary.LittleEndian.PutUint16(header[60:62], uint16(numSections))   // e_shnum
	binary.LittleEndian.PutUint16(header[62:64], uint16(numSections-1)) // e_shstrndx

	buf.Write(header[:])
	buf.Write(pclntabData)
	buf.Write(shstrtab)

	for buf.Len() < shOffset {
		buf.WriteByte(0)
	}

	// Section 0: null
	var sh0 [shEntrySize]byte
	buf.Write(sh0[:])

	// Section 1: .text (empty, provides textStart address)
	var sh1 [shEntrySize]byte
	binary.LittleEndian.PutUint32(sh1[0:4], 1)                                        // sh_name: ".text" at offset 1
	binary.LittleEndian.PutUint32(sh1[4:8], uint32(elf.SHT_PROGBITS))                 //nolint:gosec
	binary.LittleEndian.PutUint64(sh1[8:16], uint64(elf.SHF_ALLOC|elf.SHF_EXECINSTR)) //nolint:gosec
	binary.LittleEndian.PutUint64(sh1[16:24], 0x401000)                               // sh_addr (textStart)
	buf.Write(sh1[:])

	// Section 2: .gopclntab
	var sh2 [shEntrySize]byte
	binary.LittleEndian.PutUint32(sh2[0:4], 7)                          // sh_name: ".gopclntab" at offset 7
	binary.LittleEndian.PutUint32(sh2[4:8], uint32(elf.SHT_PROGBITS))   //nolint:gosec
	binary.LittleEndian.PutUint64(sh2[24:32], uint64(pclntabOffset))    // sh_offset
	binary.LittleEndian.PutUint64(sh2[32:40], uint64(len(pclntabData))) // sh_size
	buf.Write(sh2[:])

	// Section 3: .shstrtab
	var sh3 [shEntrySize]byte
	binary.LittleEndian.PutUint32(sh3[0:4], 18)                       // sh_name: ".shstrtab" at offset 18
	binary.LittleEndian.PutUint32(sh3[4:8], uint32(elf.SHT_STRTAB))   //nolint:gosec
	binary.LittleEndian.PutUint64(sh3[24:32], uint64(shstrtabOffset)) // sh_offset
	binary.LittleEndian.PutUint64(sh3[32:40], uint64(len(shstrtab)))  // sh_size
	buf.Write(sh3[:])

	return buf.Bytes()
}

// openELFWithPclntab creates an in-memory ELF with .gopclntab containing
// pclntabData and returns an *elf.File for testing.
func openELFWithPclntab(t *testing.T, pclntabData []byte) *elf.File {
	t.Helper()
	data := buildELF64WithPclntab(pclntabData)
	f, err := elf.NewFile(bytes.NewReader(data))
	require.NoError(t, err)
	return f
}

func TestParsePclntab_NoPclntabSection(t *testing.T) {
	// ELF without .gopclntab returns ErrNoPclntab.
	// no_network.elf is a C binary with no .gopclntab section.
	// On macOS, this file is not generated (gcc produces Mach-O, not ELF).
	const testFile = "testdata/no_network.elf"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Skipping: no_network.elf not available (not generated on macOS)")
	}
	f, err := elf.Open(testFile)
	require.NoError(t, err)
	defer f.Close()

	_, err = parsePclntabAllFuncs(f)
	assert.ErrorIs(t, err, ErrNoPclntab)
}

// TestParsePclntab_InvalidData verifies behavior when .gopclntab contains
// data that cannot be parsed as valid pclntab.
//
// After checkPclntabVersion was added, magic validation now happens before
// gosym is invoked:
//   - Data shorter than 4 bytes → ErrInvalidPclntab
//   - Data with magic != 0xfffffff1 → ErrUnsupportedPclntabVersion
func TestParsePclntab_InvalidData(t *testing.T) {
	cases := []struct {
		name      string
		data      []byte
		expectErr error
	}{
		{
			name:      "empty pclntab",
			data:      []byte{},
			expectErr: ErrInvalidPclntab,
		},
		{
			name:      "too short for header",
			data:      []byte{0x01, 0x02, 0x03},
			expectErr: ErrInvalidPclntab,
		},
		{
			name:      "invalid magic bytes",
			data:      []byte{0x01, 0x02, 0x03, 0x04, 0x00, 0x00, 0x01, 0x08},
			expectErr: ErrUnsupportedPclntabVersion,
		},
		{
			name:      "random garbage",
			data:      bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 8),
			expectErr: ErrUnsupportedPclntabVersion,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := openELFWithPclntab(t, tc.data)

			_, err := parsePclntabAllFuncs(f)

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.expectErr)
		})
	}
}

func TestParsePclntab_ErrorWrapping(t *testing.T) {
	// Verify the exported error values are properly defined for use with errors.Is.
	assert.Equal(t, "no .gopclntab section found", ErrNoPclntab.Error())
	assert.Equal(t, "unsupported pclntab format", ErrUnsupportedPclntab.Error())
	assert.Equal(t, "invalid pclntab structure", ErrInvalidPclntab.Error())
	assert.Equal(t, "unsupported pclntab version: only magic 0xfffffff1 (Go 1.20+) is supported",
		ErrUnsupportedPclntabVersion.Error())
}

func TestPclntabParser_NoPclntab(t *testing.T) {
	// This test verifies the error message when .gopclntab is missing
	assert.Equal(t, "no .gopclntab section found", ErrNoPclntab.Error())
}

func TestPclntabResult_Lookup(t *testing.T) {
	result := map[string]PclntabFunc{
		"main.main": {Name: "main.main", Entry: 0x401000, End: 0x401100},
	}

	fn, found := result["main.main"]
	assert.True(t, found)
	assert.Equal(t, "main.main", fn.Name)
	assert.Equal(t, uint64(0x401000), fn.Entry)
	assert.Equal(t, uint64(0x401100), fn.End)

	_, found = result["nonexistent"]
	assert.False(t, found)
}

// TestDetectPclntabOffset_NonCGO verifies that detectPclntabOffset returns 0
// for a non-CGO binary (CALL cross-reference does not reach minVotes).
func TestDetectPclntabOffset_NonCGO(t *testing.T) {
	const testFile = "testdata/arm64_network_program/arm64_network_program.elf"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("arm64 test binary not available")
	}
	f, err := elf.Open(testFile)
	require.NoError(t, err)
	defer f.Close()

	// Parse raw pclntab entries (no CGO correction for non-CGO binaries).
	funcs, err := parsePclntabFuncsRaw(f)
	if err != nil {
		// If the binary uses a different magic (e.g. Go 1.18-1.19), skip.
		if errors.Is(err, ErrUnsupportedPclntabVersion) {
			t.Skip("test binary uses unsupported pclntab version")
		}
		require.NoError(t, err)
	}
	require.NotEmpty(t, funcs)

	// For non-CGO binaries, CALL targets already match pclntab entries without
	// any adjustment, so detectPclntabOffset must return 0.
	offset := detectPclntabOffset(f, funcs)
	assert.Equal(t, int64(0), offset, "non-CGO binary → offset must be 0")
}

// --- checkPclntabVersion unit tests ---

func TestCheckPclntabVersion_Go120magic(t *testing.T) {
	// magic = 0xfffffff1 (Go 1.20–1.26) should return nil.
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 0xfffffff1)
	err := checkPclntabVersion(data, binary.LittleEndian)
	assert.NoError(t, err)
}

func TestCheckPclntabVersion_Go118magic(t *testing.T) {
	// magic = 0xfffffff0 (Go 1.18–1.19) should return ErrUnsupportedPclntabVersion.
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 0xfffffff0)
	err := checkPclntabVersion(data, binary.LittleEndian)
	assert.ErrorIs(t, err, ErrUnsupportedPclntabVersion)
}

func TestCheckPclntabVersion_Go116magic(t *testing.T) {
	// magic = 0xfffffffa (Go 1.16–1.17) should return ErrUnsupportedPclntabVersion.
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 0xfffffffa)
	err := checkPclntabVersion(data, binary.LittleEndian)
	assert.ErrorIs(t, err, ErrUnsupportedPclntabVersion)
}

func TestCheckPclntabVersion_Go12magic(t *testing.T) {
	// magic = 0xfffffffb (Go 1.2–1.15) should return ErrUnsupportedPclntabVersion.
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 0xfffffffb)
	err := checkPclntabVersion(data, binary.LittleEndian)
	assert.ErrorIs(t, err, ErrUnsupportedPclntabVersion)
}

func TestCheckPclntabVersion_TooShort(t *testing.T) {
	// Data shorter than 4 bytes should return ErrInvalidPclntab.
	data := []byte{0xff, 0xff, 0xff}
	err := checkPclntabVersion(data, binary.LittleEndian)
	assert.ErrorIs(t, err, ErrInvalidPclntab)
}

func TestCheckPclntabVersion_BigEndian(t *testing.T) {
	// BigEndian encoding of 0xfffffff1 → bytes [0xff, 0xff, 0xff, 0xf1].
	data := make([]byte, 8)
	binary.BigEndian.PutUint32(data[0:4], 0xfffffff1)
	err := checkPclntabVersion(data, binary.BigEndian)
	assert.NoError(t, err)
}

// --- detectOffsetByCallTargets unit tests ---

// buildELF64WithTextAndPclntab constructs a minimal ELF64 with .text and .gopclntab sections.
// textData contains raw machine code for .text, textAddr is the load address of .text,
// and pclntabData is placed in .gopclntab.
func buildELF64WithTextAndPclntab(textData []byte, textAddr uint64, pclntabData []byte, machine elf.Machine) []byte {
	var buf bytes.Buffer

	// shstrtab layout: \0 .text\0 .gopclntab\0 .shstrtab\0
	shstrtab := []byte("\x00.text\x00.gopclntab\x00.shstrtab\x00")

	const elfHeaderSize = 64
	const shEntrySize = 64
	const numSections = 4 // null, .text, .gopclntab, .shstrtab

	textOffset := elfHeaderSize
	pclntabOffset := textOffset + len(textData)
	shstrtabOffset := pclntabOffset + len(pclntabData)

	shOffset := shstrtabOffset + len(shstrtab)
	if shOffset%8 != 0 {
		shOffset += 8 - (shOffset % 8)
	}

	// ELF header
	var header [elfHeaderSize]byte
	copy(header[0:4], []byte{0x7f, 'E', 'L', 'F'})
	header[4] = byte(elf.ELFCLASS64)
	header[5] = byte(elf.ELFDATA2LSB)
	header[6] = byte(elf.EV_CURRENT)
	binary.LittleEndian.PutUint16(header[16:18], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(header[18:20], uint16(machine)) //nolint:gosec
	binary.LittleEndian.PutUint32(header[20:24], uint32(elf.EV_CURRENT))
	binary.LittleEndian.PutUint64(header[40:48], uint64(shOffset))      // e_shoff
	binary.LittleEndian.PutUint16(header[52:54], uint16(elfHeaderSize)) // e_ehsize
	binary.LittleEndian.PutUint16(header[58:60], uint16(shEntrySize))   // e_shentsize
	binary.LittleEndian.PutUint16(header[60:62], uint16(numSections))   // e_shnum
	binary.LittleEndian.PutUint16(header[62:64], uint16(numSections-1)) // e_shstrndx

	buf.Write(header[:])
	buf.Write(textData)
	buf.Write(pclntabData)
	buf.Write(shstrtab)

	for buf.Len() < shOffset {
		buf.WriteByte(0)
	}

	// Section 0: null
	var sh0 [shEntrySize]byte
	buf.Write(sh0[:])

	// Section 1: .text
	var sh1 [shEntrySize]byte
	binary.LittleEndian.PutUint32(sh1[0:4], 1)                                        // sh_name: ".text"
	binary.LittleEndian.PutUint32(sh1[4:8], uint32(elf.SHT_PROGBITS))                 //nolint:gosec
	binary.LittleEndian.PutUint64(sh1[8:16], uint64(elf.SHF_ALLOC|elf.SHF_EXECINSTR)) //nolint:gosec
	binary.LittleEndian.PutUint64(sh1[16:24], textAddr)                               // sh_addr
	binary.LittleEndian.PutUint64(sh1[24:32], uint64(textOffset))                     // sh_offset
	binary.LittleEndian.PutUint64(sh1[32:40], uint64(len(textData)))                  // sh_size
	buf.Write(sh1[:])

	// Section 2: .gopclntab
	var sh2 [shEntrySize]byte
	binary.LittleEndian.PutUint32(sh2[0:4], 7)                          // sh_name: ".gopclntab"
	binary.LittleEndian.PutUint32(sh2[4:8], uint32(elf.SHT_PROGBITS))   //nolint:gosec
	binary.LittleEndian.PutUint64(sh2[24:32], uint64(pclntabOffset))    // sh_offset
	binary.LittleEndian.PutUint64(sh2[32:40], uint64(len(pclntabData))) // sh_size
	buf.Write(sh2[:])

	// Section 3: .shstrtab
	var sh3 [shEntrySize]byte
	binary.LittleEndian.PutUint32(sh3[0:4], 18)                       // sh_name: ".shstrtab"
	binary.LittleEndian.PutUint32(sh3[4:8], uint32(elf.SHT_STRTAB))   //nolint:gosec
	binary.LittleEndian.PutUint64(sh3[24:32], uint64(shstrtabOffset)) // sh_offset
	binary.LittleEndian.PutUint64(sh3[32:40], uint64(len(shstrtab)))  // sh_size
	buf.Write(sh3[:])

	return buf.Bytes()
}

// encodeX86Call encodes an x86_64 CALL rel32 instruction into buf[0:5].
// from is the VA of the CALL instruction, to is the target VA.
func encodeX86Call(buf []byte, from, to uint64) {
	rel := int32(int64(to) - int64(from) - 5) //nolint:gosec // G115: address difference is bounded
	buf[0] = 0xe8
	binary.LittleEndian.PutUint32(buf[1:5], uint32(rel)) //nolint:gosec
}

// encodeArm64BL encodes an arm64 BL instruction into buf[0:4].
// from is the VA of the BL instruction, to is the target VA.
func encodeArm64BL(buf []byte, from, to uint64) {
	imm26 := (int64(to) - int64(from)) / 4                   //nolint:gosec // G115: address difference is bounded
	instr := uint32(0b100101<<26) | uint32(imm26&0x03ffffff) //nolint:gosec
	binary.LittleEndian.PutUint32(buf, instr)
}

// TestDetectOffsetByCallTargets_WithOffset_x86 verifies that x86_64 CALL
// instructions whose targets match pclntab entries shifted by 0x100 are
// detected and the offset 0x100 is returned.
//
// Uses dense entry spacing (0x60) matching real Go binaries.
// With window exact-match, each CALL accumulates votes for multiple entries in the
// window, but only the correct offset (0x100) receives votes from all numCalls.
func TestDetectOffsetByCallTargets_WithOffset_x86(t *testing.T) {
	const offsetVal = int64(0x100) // simulated C startup size
	const textAddr = uint64(0x401000)
	const numCalls = 10
	// Use real-binary-like dense spacing. Window exact-match correctly identifies
	// the most frequent diff even when multiple entries fall in each window.
	const entrySpacing = uint64(0x60)

	// pclntab entries are at textAddr (uncorrected, as gosym would return them).
	pclntabFuncs := make(map[string]PclntabFunc, numCalls)
	for i := range numCalls {
		entry := textAddr + uint64(i)*entrySpacing //nolint:gosec
		pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)] = PclntabFunc{
			Name: fmt.Sprintf("pkg.Func%d", i), Entry: entry, End: entry + 0x50,
		}
	}

	// Build .text: each CALL targets (pclntab_entry + offset), simulating a
	// CGO binary where actual function VAs = pclntab_entry + C_startup_size.
	textData := make([]byte, numCalls*10) // 5 bytes per CALL + 5 bytes padding
	for i := range numCalls {
		callSiteVA := textAddr + uint64(i*10)                                            //nolint:gosec
		targetVA := pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)].Entry + uint64(offsetVal) //nolint:gosec
		pos := i * 10
		encodeX86Call(textData[pos:pos+5], callSiteVA, targetVA)
	}

	elfData := buildELF64WithTextAndPclntab(textData, textAddr, []byte{}, elf.EM_X86_64)
	f, err := elf.NewFile(bytes.NewReader(elfData))
	require.NoError(t, err)
	defer f.Close()

	got := detectOffsetByCallTargets(f, pclntabFuncs)
	assert.Equal(t, offsetVal, got, "x86_64: expected offset 0x%x, got 0x%x", offsetVal, got)
}

// TestDetectOffsetByCallTargets_WithOffset_arm64 verifies that arm64 BL
// instructions whose targets match pclntab entries shifted by 0x100 are
// detected and the offset 0x100 is returned.
//
// Uses dense entry spacing (0x60) matching real Go binaries.
func TestDetectOffsetByCallTargets_WithOffset_arm64(t *testing.T) {
	const offsetVal = int64(0x100)
	const textAddr = uint64(0x10000)
	const numCalls = 10
	const entrySpacing = uint64(0x60)

	pclntabFuncs := make(map[string]PclntabFunc, numCalls)
	for i := range numCalls {
		entry := textAddr + uint64(i)*entrySpacing //nolint:gosec
		pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)] = PclntabFunc{
			Name: fmt.Sprintf("pkg.Func%d", i), Entry: entry, End: entry + 0x50,
		}
	}

	// Build .text: place numCalls BL instructions (4 bytes each).
	textData := make([]byte, numCalls*4)
	for i := range numCalls {
		callSiteVA := textAddr + uint64(i*4)                                             //nolint:gosec
		targetVA := pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)].Entry + uint64(offsetVal) //nolint:gosec
		encodeArm64BL(textData[i*4:i*4+4], callSiteVA, targetVA)
	}

	elfData := buildELF64WithTextAndPclntab(textData, textAddr, []byte{}, elf.EM_AARCH64)
	f, err := elf.NewFile(bytes.NewReader(elfData))
	require.NoError(t, err)
	defer f.Close()

	got := detectOffsetByCallTargets(f, pclntabFuncs)
	assert.Equal(t, offsetVal, got, "arm64: expected offset 0x%x, got 0x%x", offsetVal, got)
}

// TestDetectOffsetByCallTargets_NoOffset verifies that when CALL targets
// match pclntab entries exactly (offset = 0), detectOffsetByCallTargets
// returns 0 as the best diff.
//
// With window exact-match, diff=0 accumulates the most votes (each CALL target
// matches its own entry exactly). The caller (detectPclntabOffset) then rejects
// bestDiff==0 via its offset<=0 guard, treating this as a non-CGO binary.
func TestDetectOffsetByCallTargets_NoOffset(t *testing.T) {
	const textAddr = uint64(0x401000)
	const numCalls = 5

	pclntabFuncs := make(map[string]PclntabFunc, numCalls)
	for i := range numCalls {
		entry := textAddr + uint64(i)*0x100 //nolint:gosec
		pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)] = PclntabFunc{
			Name: fmt.Sprintf("pkg.Func%d", i), Entry: entry, End: entry + 0x80,
		}
	}

	// CALL targets equal pclntab entries (diff = 0 for all).
	textData := make([]byte, numCalls*10)
	for i := range numCalls {
		callSiteVA := textAddr + uint64(i*10) //nolint:gosec
		targetVA := pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)].Entry
		pos := i * 10
		encodeX86Call(textData[pos:pos+5], callSiteVA, targetVA)
	}

	elfData := buildELF64WithTextAndPclntab(textData, textAddr, []byte{}, elf.EM_X86_64)
	f, err := elf.NewFile(bytes.NewReader(elfData))
	require.NoError(t, err)
	defer f.Close()

	// diff = 0 accumulates the most votes → detectOffsetByCallTargets returns 0
	got := detectOffsetByCallTargets(f, pclntabFuncs)
	assert.Equal(t, int64(0), got, "no offset → must return 0")
}

// TestDetectOffsetByCallTargets_InsufficientVotes verifies that when fewer
// than minVotes CALL instructions match, the function returns 0.
//
// With window exact-match, entrySpacing=0x1000 ensures each window [T-0x2000, T]
// contains at most 2 entries, so each diff gets at most 2 votes < minVotes=3.
func TestDetectOffsetByCallTargets_InsufficientVotes(t *testing.T) {
	const offsetVal = int64(0x200)
	const textAddr = uint64(0x401000)
	const numCalls = 2 // below minVotes=3

	// entrySpacing=0x1000: window [T-maxOffset, T] = [T-0x2000, T] contains at most
	// 2 entries (spacing 0x1000, window 0x2000), so max votes per diff = 2 < minVotes=3.
	pclntabFuncs := make(map[string]PclntabFunc, numCalls)
	for i := range numCalls {
		entry := textAddr + uint64(i)*0x1000 //nolint:gosec
		pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)] = PclntabFunc{
			Name: fmt.Sprintf("pkg.Func%d", i), Entry: entry, End: entry + 0x80,
		}
	}

	textData := make([]byte, numCalls*10)
	for i := range numCalls {
		callSiteVA := textAddr + uint64(i*10)                                            //nolint:gosec
		targetVA := pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)].Entry + uint64(offsetVal) //nolint:gosec
		pos := i * 10
		encodeX86Call(textData[pos:pos+5], callSiteVA, targetVA)
	}

	elfData := buildELF64WithTextAndPclntab(textData, textAddr, []byte{}, elf.EM_X86_64)
	f, err := elf.NewFile(bytes.NewReader(elfData))
	require.NoError(t, err)
	defer f.Close()

	got := detectOffsetByCallTargets(f, pclntabFuncs)
	assert.Equal(t, int64(0), got, "insufficient votes → must return 0")
}

// TestDetectOffsetByCallTargets_TiedVotes verifies that when two diff values share
// the top vote count, detectOffsetByCallTargets returns 0 (ambiguous, non-deterministic
// otherwise because Go map iteration is randomized).
func TestDetectOffsetByCallTargets_TiedVotes(t *testing.T) {
	const textAddr = uint64(0x401000)

	// Two pclntab entries spaced 0x40 apart.
	// Two CALL instructions: each targets a different entry with the same offset 0x100.
	// But the second CALL also matches the first entry with offset 0x140 (= 0x100 + 0x40).
	// With window exact-match both 0x100 and 0x140 get 2 votes each → tie → return 0.
	//
	// entry0 = 0x401000, entry1 = 0x401040
	// CALL0 → entry0 + 0x100 = 0x401100: diffs = {0x100: entry0→1, 0x40: entry1→... wait}
	//
	// Let's be explicit:
	//   CALL0 target = 0x401100: window [0x401100-0x2000, 0x401100] includes both entries
	//     diff from entry0 (0x401000) = 0x100  → diffCounts[0x100]++
	//     diff from entry1 (0x401040) = 0xc0   → diffCounts[0xc0]++
	//   CALL1 target = 0x401140: window includes both entries
	//     diff from entry0 (0x401000) = 0x140  → diffCounts[0x140]++
	//     diff from entry1 (0x401040) = 0x100  → diffCounts[0x100]++
	//
	// Result: diffCounts[0x100] = 2, diffCounts[0xc0] = 1, diffCounts[0x140] = 1 → 0x100 wins uniquely.
	// That's not a tie. Let's construct a real tie instead:
	//
	// Use 3 entries at spacing 0x100, and 3 CALLs each targeting entry_i + 0x80.
	//   entry0=0x401000, entry1=0x401100, entry2=0x401200
	//   CALL0 target=0x401080: diff entry0=0x80, diff entry1=-0x80 (negative, skip since target<entry1)
	//     Actually target(0x401080) < entry1(0x401100), so only entry0 in window → 0x80 gets 1 vote.
	//
	// Simpler: use 2 CALLs to non-overlapping entries but each produces the same two diffs.
	//   entry0=0x401000, entry1=0x401050
	//   CALL0 target=0x401100: diff0=0x100, diff1=0xb0
	//   CALL1 target=0x401150: diff0=0x150, diff1=0x100
	//   → diffCounts[0x100]=2 (winner), no tie.
	//
	// Construct actual tie: same number of votes for two distinct diffs.
	//   entry0=0x401000, entry1=0x401100
	//   CALL0 target=0x401100: diff entry0=0x100 → diffCounts[0x100]++
	//                           entry1=0x0   → diffCounts[0x0]++
	//   CALL1 target=0x401200: diff entry0=0x200, diff entry1=0x100 → diffCounts[0x200]++, diffCounts[0x100]++
	//   CALL2 target=0x401300: diff entry0=0x300, diff entry1=0x200 → diffCounts[0x300]++, diffCounts[0x200]++
	//   → 0x100=2, 0x200=2, 0x300=1, 0x0=1 → TIE between 0x100 and 0x200 → return 0.
	const entrySpacing = uint64(0x100)
	entry0 := textAddr
	entry1 := textAddr + entrySpacing

	pclntabFuncs := map[string]PclntabFunc{
		"pkg.Func0": {Name: "pkg.Func0", Entry: entry0, End: entry0 + 0x80},
		"pkg.Func1": {Name: "pkg.Func1", Entry: entry1, End: entry1 + 0x80},
	}

	// 3 CALLs: targets at entry0+0x100, entry0+0x200, entry0+0x300
	type callSpec struct{ site, target uint64 }
	calls := []callSpec{
		{textAddr + 0x000, entry0 + 0x100}, // CALL0: diffs {0x100, 0x0}
		{textAddr + 0x010, entry0 + 0x200}, // CALL1: diffs {0x200, 0x100}
		{textAddr + 0x020, entry0 + 0x300}, // CALL2: diffs {0x300, 0x200}
	}
	textData := make([]byte, 0x100)
	for _, c := range calls {
		off := int(c.site - textAddr)
		encodeX86Call(textData[off:off+5], c.site, c.target)
	}

	elfData := buildELF64WithTextAndPclntab(textData, textAddr, []byte{}, elf.EM_X86_64)
	f, err := elf.NewFile(bytes.NewReader(elfData))
	require.NoError(t, err)
	defer f.Close()

	// diffCounts[0x100]=2, diffCounts[0x200]=2 → tie → must return 0.
	got := detectOffsetByCallTargets(f, pclntabFuncs)
	assert.Equal(t, int64(0), got, "tied top vote count → must return 0")
}

// TestDetectOffsetByCallTargets_NoText verifies that the function returns 0
// when the ELF has no .text data (empty section returns no CALL instructions).
func TestDetectOffsetByCallTargets_NoText(t *testing.T) {
	// buildELF64WithPclntab creates an ELF with empty .text data.
	f := openELFWithPclntab(t, []byte{})
	defer f.Close()

	pclntabFuncs := map[string]PclntabFunc{
		"main.main": {Name: "main.main", Entry: 0x401000, End: 0x401100},
	}
	// .text section has addr=0x401000 but size=0 → no CALL data → 0 votes → return 0.
	got := detectOffsetByCallTargets(f, pclntabFuncs)
	assert.Equal(t, int64(0), got, ".text section with no data → must return 0")
}

// --- collectWindowDiffs unit tests ---

// TestCollectWindowDiffs_DenseEntries verifies that collectWindowDiffs records
// differences for all entries within [target - maxOffset, target] and excludes
// entries outside the window.
func TestCollectWindowDiffs_DenseEntries(t *testing.T) {
	target := uint64(0x402200)
	sortedEntries := []uint64{
		0x400000, // outside window: target - 0x400000 = 0x2200 > maxOffset
		0x402100, // inside window: diff = 0x100
		0x402140, // inside window: diff = 0xc0
		0x402180, // inside window: diff = 0x80
		0x4021c0, // inside window: diff = 0x40
		0x402200, // inside window: diff = 0x0
	}

	diffCounts := make(map[int64]int)
	collectWindowDiffs(target, sortedEntries, diffCounts)

	// Entries outside the window must not be recorded.
	assert.Len(t, diffCounts, 5, "only window-internal entries should be recorded")
	assert.Equal(t, 1, diffCounts[0x100], "diff 0x100")
	assert.Equal(t, 1, diffCounts[0xc0], "diff 0xc0")
	assert.Equal(t, 1, diffCounts[0x80], "diff 0x80")
	assert.Equal(t, 1, diffCounts[0x40], "diff 0x40")
	assert.Equal(t, 1, diffCounts[0x0], "diff 0x0")
	_, hasOutside := diffCounts[0x2200]
	assert.False(t, hasOutside, "diff for outside-window entry 0x400000 must not be recorded")
}

// TestCollectWindowDiffs_MaxOffsetBoundary verifies that collectWindowDiffs
// correctly handles boundary cases at maxOffset.
//
// Case A: offset = maxOffset-1 → entry is inside window (diff recorded)
// Case B: offset = maxOffset   → entry is exactly at window boundary (diff recorded)
// Case C: offset = maxOffset+1 → entry is outside window (diff not recorded)
func TestCollectWindowDiffs_MaxOffsetBoundary(t *testing.T) {
	textAddr := uint64(0x401000)
	entry := textAddr // pclntab entry E = 0x401000

	cases := []struct {
		name        string
		offset      int64
		expectFound bool
	}{
		{
			name:        "maxOffset-1: inside window",
			offset:      maxOffset - 1,
			expectFound: true,
		},
		{
			name:        "maxOffset: boundary (included)",
			offset:      maxOffset,
			expectFound: true,
		},
		{
			name:        "maxOffset+1: outside window",
			offset:      maxOffset + 1,
			expectFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := uint64(int64(entry) + tc.offset) //nolint:gosec
			diffCounts := make(map[int64]int)
			collectWindowDiffs(target, []uint64{entry}, diffCounts)

			_, found := diffCounts[tc.offset]
			if tc.expectFound {
				assert.True(t, found, "offset %d should be in window", tc.offset)
				assert.Equal(t, 1, diffCounts[tc.offset])
			} else {
				assert.False(t, found, "offset %d should be outside window", tc.offset)
			}
		})
	}
}

// TestDetectOffsetByCallTargets_DenseLayout_x86 verifies offset detection
// with dense function layout (entrySpacing=0x40) matching p10 real binaries.
func TestDetectOffsetByCallTargets_DenseLayout_x86(t *testing.T) {
	const offsetVal = int64(0x100)
	const textAddr = uint64(0x401000)
	const numCalls = 20
	const entrySpacing = uint64(0x40) // 64 bytes — real binary p10 value

	pclntabFuncs := make(map[string]PclntabFunc, numCalls)
	for i := range numCalls {
		entry := textAddr + uint64(i)*entrySpacing //nolint:gosec
		pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)] = PclntabFunc{
			Name: fmt.Sprintf("pkg.Func%d", i), Entry: entry, End: entry + 0x30,
		}
	}

	textData := make([]byte, numCalls*10)
	for i := range numCalls {
		callSiteVA := textAddr + uint64(i*10)                                            //nolint:gosec
		targetVA := pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)].Entry + uint64(offsetVal) //nolint:gosec
		pos := i * 10
		encodeX86Call(textData[pos:pos+5], callSiteVA, targetVA)
	}

	elfData := buildELF64WithTextAndPclntab(textData, textAddr, []byte{}, elf.EM_X86_64)
	f, err := elf.NewFile(bytes.NewReader(elfData))
	require.NoError(t, err)
	defer f.Close()

	got := detectOffsetByCallTargets(f, pclntabFuncs)
	assert.Equal(t, offsetVal, got, "dense layout x86: expected offset 0x%x, got 0x%x", offsetVal, got)
}

// TestDetectOffsetByCallTargets_OffsetAtMaxBoundary_x86 verifies end-to-end
// behavior of detectOffsetByCallTargets near the maxOffset boundary.
//
// Case 1 & 2: offset <= maxOffset → entry falls within [T - maxOffset, T], votes accumulate.
// Case 3: offset = maxOffset+1 → single entry is outside window, no votes → returns 0.
//
// Case 3 uses a single pclntab entry and a single CALL to ensure no other entry
// accidentally falls into the window (which would produce a different diff value and
// accumulate votes, defeating the "not detected" expectation).
func TestDetectOffsetByCallTargets_OffsetAtMaxBoundary_x86(t *testing.T) {
	const textAddr = uint64(0x401000)

	// Case 1 & 2: multiple entries with dense spacing to accumulate votes.
	casesDetected := []struct {
		name          string
		offsetVal     int64
		numCalls      int
		entrySpacing  uint64
		textSizeExtra uint64 // extra bytes to ensure detectPclntabOffset's offset<=textSize guard passes
	}{
		{
			name:          "maxOffset-1: detected",
			offsetVal:     maxOffset - 1,
			numCalls:      10,
			entrySpacing:  0x40,
			textSizeExtra: 0x100,
		},
		{
			name:          "maxOffset: detected",
			offsetVal:     maxOffset,
			numCalls:      10,
			entrySpacing:  0x40,
			textSizeExtra: 0x100,
		},
	}

	for _, tc := range casesDetected {
		t.Run(tc.name, func(t *testing.T) {
			pclntabFuncs := make(map[string]PclntabFunc, tc.numCalls)
			for i := range tc.numCalls {
				entry := textAddr + uint64(i)*tc.entrySpacing //nolint:gosec
				pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)] = PclntabFunc{
					Name: fmt.Sprintf("pkg.Func%d", i), Entry: entry, End: entry + 0x30,
				}
			}

			textData := make([]byte, tc.numCalls*10)
			for i := range tc.numCalls {
				callSiteVA := textAddr + uint64(i*10)                                               //nolint:gosec
				targetVA := pclntabFuncs[fmt.Sprintf("pkg.Func%d", i)].Entry + uint64(tc.offsetVal) //nolint:gosec
				pos := i * 10
				encodeX86Call(textData[pos:pos+5], callSiteVA, targetVA)
			}

			requiredSize := uint64(tc.offsetVal) + tc.textSizeExtra //nolint:gosec
			if uint64(len(textData)) < requiredSize {
				padding := make([]byte, requiredSize-uint64(len(textData)))
				textData = append(textData, padding...)
			}

			elfData := buildELF64WithTextAndPclntab(textData, textAddr, []byte{}, elf.EM_X86_64)
			f, err := elf.NewFile(bytes.NewReader(elfData))
			require.NoError(t, err)
			defer f.Close()

			got := detectOffsetByCallTargets(f, pclntabFuncs)
			assert.Equal(t, tc.offsetVal, got, "%s: expected offset 0x%x", tc.name, tc.offsetVal)
		})
	}

	// Case 3: maxOffset+1 with a single entry.
	// When offset = maxOffset+1, T - entry = maxOffset+1 > maxOffset, so entry is outside
	// the window [T - maxOffset, T]. No votes accumulate → detectOffsetByCallTargets returns 0.
	// Single entry + single CALL ensures no other entry accidentally enters the window.
	t.Run("maxOffset+1: not detected (single entry)", func(t *testing.T) {
		const offsetVal = maxOffset + 1
		entry := textAddr
		pclntabFuncs := map[string]PclntabFunc{
			"pkg.Func0": {Name: "pkg.Func0", Entry: entry, End: entry + 0x30},
		}

		// Single CALL targeting entry + (maxOffset+1): no entry in window.
		textData := make([]byte, 10)
		callSiteVA := textAddr
		targetVA := entry + uint64(offsetVal) //nolint:gosec
		encodeX86Call(textData[0:5], callSiteVA, targetVA)

		elfData := buildELF64WithTextAndPclntab(textData, textAddr, []byte{}, elf.EM_X86_64)
		f, err := elf.NewFile(bytes.NewReader(elfData))
		require.NoError(t, err)
		defer f.Close()

		got := detectOffsetByCallTargets(f, pclntabFuncs)
		assert.Equal(t, int64(0), got, "maxOffset+1: entry outside window → must return 0")
	})
}

// --- integration: real CGO binary tests ---

// TestParsePclntab_RealCGOBinary_NotStripped and TestParsePclntab_RealCGOBinary_Stripped
// are in pclntab_parser_integration_test.go (build tag: integration).
