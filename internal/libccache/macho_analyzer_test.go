//go:build test

package libccache

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Test Mach-O builder ----

// testSym describes a symbol for building a test Mach-O.
type testSym struct {
	Name  string
	Value uint64 // virtual address within __text section
	Sect  uint8  // section number (1 = __text)
	Type  uint8  // e.g., 0x0F for N_SECT | N_EXT
}

const testTextVMAddr = uint64(0x100000000) // typical macOS arm64 __TEXT base

// buildTestMacho constructs a minimal Mach-O 64-bit binary in memory that
// debug/macho.NewFile can parse. It contains a single __TEXT,__text section
// with the supplied code bytes and a symbol table from syms.
//
// Binary layout follows the Apple Mach-O spec:
//   - segment_command_64: fileoff/filesize are uint64 (72 bytes total)
//   - section_64: offset is uint32 (80 bytes total)
func buildTestMacho(t *testing.T, cpu macho.Cpu, textBytes []byte, syms []testSym) *macho.File {
	t.Helper()

	const (
		lcSegment64 = uint32(0x19)       // LC_SEGMENT_64
		lcSymtab    = uint32(0x02)       // LC_SYMTAB
		magic64     = uint32(0xFEEDFACF) // MH_MAGIC_64
		hdrSize     = uint32(32)         // FileHeader64 binary size
		// segment_command_64 body: 72 bytes (fileoff/filesize are uint64)
		segCmdBodySize = uint32(72)
		sect64Size     = uint32(80) // section_64 struct
		symtabCmdSz    = uint32(24) // LC_SYMTAB cmd
		nlist64Size    = uint32(16) // Nlist64 entry
	)

	nsyms := uint32(len(syms))
	// Build string table (index 0 = empty string)
	strtab := []byte{0}
	strOffsets := make([]uint32, nsyms)
	for i, s := range syms {
		strOffsets[i] = uint32(len(strtab))
		strtab = append(strtab, []byte(s.Name)...)
		strtab = append(strtab, 0)
	}
	strtabSize := uint32(len(strtab))

	segCmdTotal := segCmdBodySize + sect64Size // segment header + 1 section header
	sizeofcmds := segCmdTotal + symtabCmdSz
	textOffset := hdrSize + sizeofcmds
	symtabOffset := textOffset + uint32(len(textBytes))
	strtabOffset := symtabOffset + nsyms*nlist64Size

	bo := binary.LittleEndian
	buf := &bytes.Buffer{}

	// 1. FileHeader (32 bytes)
	binary.Write(buf, bo, magic64)     // magic
	binary.Write(buf, bo, uint32(cpu)) // cputype
	binary.Write(buf, bo, uint32(0))   // cpusubtype
	binary.Write(buf, bo, uint32(6))   // filetype MH_DYLIB
	binary.Write(buf, bo, uint32(2))   // ncmds
	binary.Write(buf, bo, sizeofcmds)  // sizeofcmds
	binary.Write(buf, bo, uint32(0))   // flags
	binary.Write(buf, bo, uint32(0))   // reserved (64-bit header)

	// 2. LC_SEGMENT_64 header (72 bytes)
	// segment_command_64: cmd, cmdsize, segname[16], vmaddr, vmsize,
	//                     fileoff, filesize, maxprot, initprot, nsects, flags
	seg := [16]byte{}
	copy(seg[:], "__TEXT")
	binary.Write(buf, bo, lcSegment64)
	binary.Write(buf, bo, segCmdTotal) // cmdsize
	buf.Write(seg[:])
	binary.Write(buf, bo, testTextVMAddr)         // vmaddr (uint64)
	binary.Write(buf, bo, uint64(len(textBytes))) // vmsize (uint64)
	binary.Write(buf, bo, uint64(textOffset))     // fileoff (uint64)
	binary.Write(buf, bo, uint64(len(textBytes))) // filesize (uint64)
	binary.Write(buf, bo, uint32(5))              // maxprot
	binary.Write(buf, bo, uint32(5))              // initprot
	binary.Write(buf, bo, uint32(1))              // nsects = 1
	binary.Write(buf, bo, uint32(0))              // flags

	// 2a. section_64 for __text (80 bytes)
	// section_64: sectname[16], segname[16], addr, size, offset (uint32!),
	//             align, reloff, nreloc, flags, reserved1..3
	sect := [16]byte{}
	copy(sect[:], "__text")
	buf.Write(sect[:])
	buf.Write(seg[:])                             // segname __TEXT
	binary.Write(buf, bo, testTextVMAddr)         // addr (uint64)
	binary.Write(buf, bo, uint64(len(textBytes))) // size (uint64)
	binary.Write(buf, bo, textOffset)             // offset (uint32)
	binary.Write(buf, bo, uint32(2))              // align
	binary.Write(buf, bo, uint32(0))              // reloff
	binary.Write(buf, bo, uint32(0))              // nreloc
	binary.Write(buf, bo, uint32(0x80000400))     // flags PURE_INSTRUCTIONS
	binary.Write(buf, bo, uint32(0))              // reserved1
	binary.Write(buf, bo, uint32(0))              // reserved2
	binary.Write(buf, bo, uint32(0))              // reserved3

	// 3. LC_SYMTAB (24 bytes)
	binary.Write(buf, bo, lcSymtab)
	binary.Write(buf, bo, symtabCmdSz)
	binary.Write(buf, bo, symtabOffset)
	binary.Write(buf, bo, nsyms)
	binary.Write(buf, bo, strtabOffset)
	binary.Write(buf, bo, strtabSize)

	// 4. __text section data
	buf.Write(textBytes)

	// 5. Symbol table (Nlist64: strx 4B, type 1B, sect 1B, desc 2B, value 8B)
	for i, s := range syms {
		binary.Write(buf, bo, strOffsets[i]) // n_strx
		buf.WriteByte(s.Type)                // n_type
		buf.WriteByte(s.Sect)                // n_sect
		binary.Write(buf, bo, uint16(0))     // n_desc
		binary.Write(buf, bo, s.Value)       // n_value
	}

	// 6. String table
	buf.Write(strtab)

	f, err := macho.NewFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err, "buildTestMacho: failed to parse constructed Mach-O (buf size=%d)", buf.Len())
	return f
}

// ARM64 instruction constants for test code generation.
const (
	// svcMacOS is the "svc #0x80" instruction (0xD4001001).
	svcMacOS = uint32(0xD4001001)

	// nopArm64 is the NOP instruction (0xD503201F).
	nopArm64 = uint32(0xD503201F)
)

// movzX16 encodes "movz x16, #imm" (LSL #0).
func movzX16(imm uint32) uint32 {
	return uint32(0xD2800010) | (imm << 5)
}

// movzX16Lsl16 encodes "movz x16, #imm, lsl #16".
func movzX16Lsl16Enc(imm uint32) uint32 {
	return uint32(0xD2A00010) | (imm << 5)
}

// movkX16 encodes "movk x16, #imm" (LSL #0).
func movkX16(imm uint32) uint32 {
	return uint32(0xF2800010) | (imm << 5)
}

// u32LE encodes a uint32 as 4 little-endian bytes.
func u32LE(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// buildInstructions concatenates ARM64 instructions (uint32) into a byte slice.
func buildInstructions(instrs ...uint32) []byte {
	var code []byte
	for _, i := range instrs {
		code = append(code, u32LE(i)...)
	}
	return code
}

// ---- Test cases ----

// TestMachoLibSystemAnalyzer_Analyze_SvcDetection verifies basic svc detection
// with a simple movz x16 / svc sequence.
func TestMachoLibSystemAnalyzer_Analyze_SvcDetection(t *testing.T) {
	// socket = syscall 97 = 0x61
	// Instruction pattern: MOVZ X16, #97; SVC #0x80
	text := buildInstructions(
		movzX16(97), // MOVZ X16, #97
		svcMacOS,    // SVC #0x80
	)

	mf := buildTestMacho(t, macho.CpuArm64, text, []testSym{
		{Name: "socket", Value: testTextVMAddr, Sect: 1, Type: 0x0F},
	})

	analyzer := &MachoLibSystemAnalyzer{}
	entries, err := analyzer.Analyze(mf)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "socket", entries[0].Name)
	assert.Equal(t, 97, entries[0].Number)
}

// TestMachoLibSystemAnalyzer_Analyze_BSDPrefixRemoval verifies that the macOS BSD
// syscall class prefix (0x2000000) encoded in x16 is stripped before recording the number.
func TestMachoLibSystemAnalyzer_Analyze_BSDPrefixRemoval(t *testing.T) {
	// socket with full BSD prefix: MOVZ X16, #0x200, LSL #16 / MOVK X16, #0x61 / SVC
	// 0x2000000 | 97 = 0x2000061
	text := buildInstructions(
		movzX16Lsl16Enc(0x200), // MOVZ X16, #0x200, LSL #16  -> x16 = 0x02000000
		movkX16(0x61),          // MOVK X16, #0x61            -> x16 = 0x02000061
		svcMacOS,               // SVC #0x80
	)

	mf := buildTestMacho(t, macho.CpuArm64, text, []testSym{
		{Name: "socket", Value: testTextVMAddr, Sect: 1, Type: 0x0F},
	})

	analyzer := &MachoLibSystemAnalyzer{}
	entries, err := analyzer.Analyze(mf)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 97, entries[0].Number, "BSD prefix should be stripped")
}

// TestMachoLibSystemAnalyzer_Analyze_MultipleSameSyscall verifies that multiple
// svc instructions with the same syscall number are accepted (not rejected).
func TestMachoLibSystemAnalyzer_Analyze_MultipleSameSyscall(t *testing.T) {
	// Two svc instructions both use syscall 97.
	text := buildInstructions(
		movzX16(97), // MOVZ X16, #97
		svcMacOS,    // SVC #0x80
		movzX16(97), // MOVZ X16, #97
		svcMacOS,    // SVC #0x80 (same number)
	)

	mf := buildTestMacho(t, macho.CpuArm64, text, []testSym{
		{Name: "socket", Value: testTextVMAddr, Sect: 1, Type: 0x0F},
	})

	analyzer := &MachoLibSystemAnalyzer{}
	entries, err := analyzer.Analyze(mf)
	require.NoError(t, err)
	// Same syscall twice => still one entry (same number, same function)
	require.Len(t, entries, 1)
	assert.Equal(t, 97, entries[0].Number)
}

// TestMachoLibSystemAnalyzer_Analyze_MultipleDistinctSyscalls verifies that functions
// with multiple distinct syscall numbers are excluded.
func TestMachoLibSystemAnalyzer_Analyze_MultipleDistinctSyscalls(t *testing.T) {
	// Function has svc for syscall 97 and svc for syscall 98 -> should be skipped.
	text := buildInstructions(
		movzX16(97), // MOVZ X16, #97
		svcMacOS,    // SVC #0x80
		movzX16(98), // MOVZ X16, #98
		svcMacOS,    // SVC #0x80 (different number)
	)

	mf := buildTestMacho(t, macho.CpuArm64, text, []testSym{
		{Name: "mixed", Value: testTextVMAddr, Sect: 1, Type: 0x0F},
	})

	analyzer := &MachoLibSystemAnalyzer{}
	entries, err := analyzer.Analyze(mf)
	require.NoError(t, err)
	assert.Empty(t, entries, "function with distinct syscall numbers should be excluded")
}

// TestMachoLibSystemAnalyzer_Analyze_SizeTooLarge verifies that functions larger
// than MaxWrapperFunctionSize (256 bytes) are excluded.
func TestMachoLibSystemAnalyzer_Analyze_SizeTooLarge(t *testing.T) {
	// Build a function with 257 bytes (> 256 = MaxWrapperFunctionSize).
	// Fill with nop, add svc at the end.
	instrCount := (MaxWrapperFunctionSize / 4) + 1 // 65 instructions = 260 bytes > 256
	instrs := make([]uint32, instrCount)
	for i := range instrs {
		instrs[i] = nopArm64
	}
	// Place movz + svc at the start so the x16 scan finds the syscall.
	instrs[0] = movzX16(97)
	instrs[1] = svcMacOS

	text := buildInstructions(instrs...)

	mf := buildTestMacho(t, macho.CpuArm64, text, []testSym{
		{Name: "toobig", Value: testTextVMAddr, Sect: 1, Type: 0x0F},
	})

	analyzer := &MachoLibSystemAnalyzer{}
	entries, err := analyzer.Analyze(mf)
	require.NoError(t, err)
	assert.Empty(t, entries, "function larger than MaxWrapperFunctionSize should be excluded")
}

// TestMachoLibSystemAnalyzer_Analyze_NonArm64 verifies that non-arm64 inputs
// are skipped with a log message (no error returned).
func TestMachoLibSystemAnalyzer_Analyze_NonArm64(t *testing.T) {
	text := buildInstructions(nopArm64)

	mf := buildTestMacho(t, macho.CpuAmd64, text, nil)

	analyzer := &MachoLibSystemAnalyzer{}
	entries, err := analyzer.Analyze(mf)
	// Non-arm64: should return nil, nil (skip with info log, no error)
	assert.NoError(t, err)
	assert.Nil(t, entries)
}

// TestMachoLibSystemAnalyzer_Analyze_NoSVC verifies that functions without
// svc #0x80 are not included.
func TestMachoLibSystemAnalyzer_Analyze_NoSVC(t *testing.T) {
	text := buildInstructions(nopArm64, nopArm64, nopArm64)

	mf := buildTestMacho(t, macho.CpuArm64, text, []testSym{
		{Name: "nosvc", Value: testTextVMAddr, Sect: 1, Type: 0x0F},
	})

	analyzer := &MachoLibSystemAnalyzer{}
	entries, err := analyzer.Analyze(mf)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestMachoLibSystemAnalyzer_Analyze_LastFunctionClampedToTextEnd verifies that a
// wrapper function occupying the last bytes of __TEXT,__text is still analyzed when
// the next symbol in the symbol table lies beyond the section boundary.
//
// Before the funcEnd clamp fix, funcEnd was set to the next symbol's address
// (> textEnd), the boundary check "funcEnd > textEnd" triggered, and the function
// was silently skipped. After the fix, funcEnd is clamped to textEnd so the last
// in-range function is always analyzed.
func TestMachoLibSystemAnalyzer_Analyze_LastFunctionClampedToTextEnd(t *testing.T) {
	// socket wrapper: 8 bytes occupying the entire __text section.
	text := buildInstructions(
		movzX16(97), // MOVZ X16, #97
		svcMacOS,    // SVC #0x80
	)
	textEnd := testTextVMAddr + uint64(len(text)) // 0x100000008

	// _beyond_text simulates a symbol in a different section (e.g. __DATA) whose
	// address is past the end of __text. Including it causes the loop to set
	// funcEnd = _beyond_text.Value > textEnd for the socket function.
	syms := []testSym{
		{Name: "socket", Value: testTextVMAddr, Sect: 1, Type: 0x0F},
		{Name: "_beyond_text", Value: textEnd + 0x10, Sect: 2, Type: 0x0F},
	}

	mf := buildTestMacho(t, macho.CpuArm64, text, syms)

	analyzer := &MachoLibSystemAnalyzer{}
	entries, err := analyzer.Analyze(mf)
	require.NoError(t, err)
	require.Len(t, entries, 1, "socket must be detected even though the next symbol is beyond textEnd")
	assert.Equal(t, "socket", entries[0].Name)
	assert.Equal(t, 97, entries[0].Number)
}

// TestMachoLibSystemAnalyzer_Analyze_MultipleSymbols verifies that multiple
// wrapper functions are returned and sorted by Number then Name.
func TestMachoLibSystemAnalyzer_Analyze_MultipleSymbols(t *testing.T) {
	// socket (97) at offset 0, connect (98) at offset 8
	socket := buildInstructions(movzX16(97), svcMacOS)  // 8 bytes
	connect := buildInstructions(movzX16(98), svcMacOS) // 8 bytes
	text := make([]byte, 0, len(socket)+len(connect))
	text = append(text, socket...)
	text = append(text, connect...)

	syms := []testSym{
		{Name: "socket", Value: testTextVMAddr + 0, Sect: 1, Type: 0x0F},
		{Name: "connect", Value: testTextVMAddr + 8, Sect: 1, Type: 0x0F},
	}

	mf := buildTestMacho(t, macho.CpuArm64, text, syms)

	analyzer := &MachoLibSystemAnalyzer{}
	entries, err := analyzer.Analyze(mf)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	// Should be sorted by Number
	assert.Equal(t, 97, entries[0].Number)
	assert.Equal(t, "socket", entries[0].Name)
	assert.Equal(t, 98, entries[1].Number)
	assert.Equal(t, "connect", entries[1].Name)
}
