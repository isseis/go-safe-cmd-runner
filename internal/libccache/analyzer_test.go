//go:build test

package libccache

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// elfBuilder builds minimal in-memory ELF64 LE binaries
// suitable for testing LibcWrapperAnalyzer.
type elfBuilder struct {
	machine   elf.Machine
	textCode  []byte   // raw bytes for .text section
	textBase  uint64   // virtual address of .text
	symbols   []elfSym // exported function symbols
	badDynsym bool     // if true, create a .dynsym with truncated/bad data
	noDynsym  bool     // if true, omit .dynsym entirely
	noText    bool     // if true, rename the text section to .data (Section(".text") returns nil)
}

type elfSym struct {
	name   string
	value  uint64
	size   uint64
	global bool // global binding (true) or local (false)
}

const (
	elf64EhdrSize  = 64
	elf64ShdrSize  = 64
	elf64Sym64Size = 24
	stInfoShift    = 4
)

// buildBytes constructs the raw ELF binary bytes.
func (b *elfBuilder) buildBytes(t *testing.T) []byte {
	t.Helper()

	// Build .dynstr (null byte + symbol names)
	dynstr := []byte{0} // index 0 = empty string
	nameOffsets := make([]uint32, len(b.symbols))
	for i, sym := range b.symbols {
		nameOffsets[i] = uint32(len(dynstr)) //nolint:gosec // G115: dynstr length < 64KB in tests
		dynstr = append(dynstr, []byte(sym.name)...)
		dynstr = append(dynstr, 0)
	}

	// Build .dynsym (null symbol + provided symbols)
	var dynsymData []byte
	nullSym := make([]byte, elf64Sym64Size)
	dynsymData = append(dynsymData, nullSym...)
	for i, sym := range b.symbols {
		s := make([]byte, elf64Sym64Size)
		binary.LittleEndian.PutUint32(s[0:4], nameOffsets[i]) // st_name
		binding := elf.STB_LOCAL
		if sym.global {
			binding = elf.STB_GLOBAL
		}
		s[4] = byte(elf.STT_FUNC) | byte(binding<<stInfoShift) // st_info
		s[5] = 0                                               // st_other
		// st_shndx: use section index 1 for defined symbols (text section mapped at idx 1)
		binary.LittleEndian.PutUint16(s[6:8], 1) // st_shndx = .text section (not SHN_UNDEF)
		binary.LittleEndian.PutUint64(s[8:16], sym.value)
		binary.LittleEndian.PutUint64(s[16:24], sym.size)
		dynsymData = append(dynsymData, s...)
	}

	// Build .shstrtab
	shstrtab := []byte{0} // index 0 = empty
	shstrtabOffNull := uint32(0)
	shstrtabOffText := uint32(len(shstrtab))
	// When noText is set, use ".data" instead of ".text" so Section(".text") returns nil.
	textSectionName := ".text"
	if b.noText {
		textSectionName = ".data"
	}
	shstrtab = append(shstrtab, []byte(textSectionName)...)
	shstrtab = append(shstrtab, 0)
	shstrtabOffDynsym := uint32(len(shstrtab))
	shstrtab = append(shstrtab, []byte(".dynsym")...)
	shstrtab = append(shstrtab, 0)
	shstrtabOffDynstr := uint32(len(shstrtab))
	shstrtab = append(shstrtab, []byte(".dynstr")...)
	shstrtab = append(shstrtab, 0)
	shstrtabOffShstrtab := uint32(len(shstrtab))
	shstrtab = append(shstrtab, []byte(".shstrtab")...)
	shstrtab = append(shstrtab, 0)

	// Determine section layout
	// Sections (with .dynsym):
	//   [0] null
	//   [1] .text       (link=0, info=0)
	//   [2] .dynsym     (link=3 -> .dynstr)
	//   [3] .dynstr
	//   [4] .shstrtab
	// Without .dynsym:
	//   [0] null
	//   [1] .text
	//   [2] .shstrtab

	hasDynsym := !b.noDynsym
	var numSections int
	var shstrndx int
	if hasDynsym {
		numSections = 5 // null + text + dynsym + dynstr + shstrtab
		shstrndx = 4
	} else {
		numSections = 3 // null + text + shstrtab
		shstrndx = 2
	}

	// Calculate offsets:
	shdrsOffset := int64(elf64EhdrSize)
	shdrsSize := int64(numSections * elf64ShdrSize)
	textOffset := shdrsOffset + shdrsSize
	var dynsymOffset, dynstrOffset, shstrtabOffset int64
	if hasDynsym {
		dynsymOffset = textOffset + int64(len(b.textCode))
		if b.badDynsym {
			// When badDynsym: write dynsymData but NOT dynstr.
			// .shstrtab immediately follows dynsymData in the file.
			// The .dynstr section header will point beyond file end.
			shstrtabOffset = dynsymOffset + int64(len(dynsymData))
			dynstrOffset = shstrtabOffset + int64(len(shstrtab)) + 1<<20 // beyond file end
		} else {
			dynstrOffset = dynsymOffset + int64(len(dynsymData))
			shstrtabOffset = dynstrOffset + int64(len(dynstr))
		}
	} else {
		shstrtabOffset = textOffset + int64(len(b.textCode))
	}

	buf := &bytes.Buffer{}

	// ELF header
	ehdr := make([]byte, elf64EhdrSize)
	copy(ehdr[0:4], []byte{0x7f, 'E', 'L', 'F'})
	ehdr[4] = byte(elf.ELFCLASS64)
	ehdr[5] = byte(elf.ELFDATA2LSB)
	ehdr[6] = byte(elf.EV_CURRENT)
	ehdr[7] = byte(elf.ELFOSABI_NONE)
	binary.LittleEndian.PutUint16(ehdr[16:18], uint16(elf.ET_DYN))
	binary.LittleEndian.PutUint16(ehdr[18:20], uint16(b.machine))
	binary.LittleEndian.PutUint32(ehdr[20:24], uint32(elf.EV_CURRENT))
	binary.LittleEndian.PutUint64(ehdr[40:48], uint64(shdrsOffset)) //nolint:gosec // G115: offset is positive, safe cast
	binary.LittleEndian.PutUint16(ehdr[52:54], uint16(elf64EhdrSize))
	binary.LittleEndian.PutUint16(ehdr[54:56], 56) // phentsize (no phdrs but needed)
	binary.LittleEndian.PutUint16(ehdr[56:58], 0)  // phnum = 0
	binary.LittleEndian.PutUint16(ehdr[58:60], uint16(elf64ShdrSize))
	binary.LittleEndian.PutUint16(ehdr[60:62], uint16(numSections)) //nolint:gosec // G115: numSections <= 5, safe
	binary.LittleEndian.PutUint16(ehdr[62:64], uint16(shstrndx))    //nolint:gosec // G115: shstrndx <= 4, safe
	buf.Write(ehdr)

	// Helper to write a section header
	writeSectionHdr := func(nameIdx uint32, shType elf.SectionType, flags elf.SectionFlag,
		addr, offset, size uint64, link, info uint32, entSize uint32,
	) {
		sh := make([]byte, elf64ShdrSize)
		binary.LittleEndian.PutUint32(sh[0:4], nameIdx)
		binary.LittleEndian.PutUint32(sh[4:8], uint32(shType))
		binary.LittleEndian.PutUint64(sh[8:16], uint64(flags))
		binary.LittleEndian.PutUint64(sh[16:24], addr)   // sh_addr
		binary.LittleEndian.PutUint64(sh[24:32], offset) // sh_offset
		binary.LittleEndian.PutUint64(sh[32:40], size)   // sh_size
		binary.LittleEndian.PutUint32(sh[40:44], link)
		binary.LittleEndian.PutUint32(sh[44:48], info)
		binary.LittleEndian.PutUint64(sh[48:56], 1) // sh_addralign
		binary.LittleEndian.PutUint32(sh[56:60], entSize)
		buf.Write(sh)
	}

	// [0] null section
	writeSectionHdr(shstrtabOffNull, elf.SHT_NULL, 0, 0, 0, 0, 0, 0, 0)

	// [1] .text section (virtual address = b.textBase)
	writeSectionHdr(shstrtabOffText, elf.SHT_PROGBITS, elf.SHF_ALLOC|elf.SHF_EXECINSTR,
		b.textBase, uint64(textOffset), uint64(len(b.textCode)), 0, 0, 0) //nolint:gosec // G115: offsets are safe int64->uint64 conversions

	if hasDynsym {
		// [2] .dynsym section (link=3 -> .dynstr, info=1 = first global symbol index)
		writeSectionHdr(shstrtabOffDynsym, elf.SHT_DYNSYM, elf.SHF_ALLOC,
			0, uint64(dynsymOffset), uint64(len(dynsymData)), 3, 1, elf64Sym64Size) //nolint:gosec // G115: safe

		// [3] .dynstr section; when badDynsym=true, dynstrOffset is beyond file end
		// so DynamicSymbols() will fail to read the string table.
		writeSectionHdr(shstrtabOffDynstr, elf.SHT_STRTAB, elf.SHF_ALLOC,
			0, uint64(dynstrOffset), uint64(len(dynstr)), 0, 0, 0) //nolint:gosec // G115: safe

		// [4] .shstrtab section
		writeSectionHdr(shstrtabOffShstrtab, elf.SHT_STRTAB, 0,
			0, uint64(shstrtabOffset), uint64(len(shstrtab)), 0, 0, 0) //nolint:gosec // G115: safe
	} else {
		// [2] .shstrtab section (when no .dynsym)
		writeSectionHdr(shstrtabOffShstrtab, elf.SHT_STRTAB, 0,
			0, uint64(shstrtabOffset), uint64(len(shstrtab)), 0, 0, 0) //nolint:gosec // G115: safe
	}

	// Write section data
	buf.Write(b.textCode)
	if hasDynsym {
		// Always write dynsymData (for badDynsym, .dynsym section header still points here).
		// For badDynsym, omit dynstr data so .dynstr section header points beyond file end.
		buf.Write(dynsymData)
		if !b.badDynsym {
			buf.Write(dynstr)
		}
	}
	buf.Write(shstrtab)

	return buf.Bytes()
}

// build constructs the ELF binary and parses it with elf.NewFile.
func (b *elfBuilder) build(t *testing.T) *elf.File {
	t.Helper()
	data := b.buildBytes(t)

	r := bytes.NewReader(data)
	elfFile, err := elf.NewFile(r)
	require.NoError(t, err, "failed to parse in-memory ELF")
	return elfFile
}

// x86_64 machine code helpers
// movEAX encodes: mov $num, %eax (5 bytes)
func movEAX(num int) []byte {
	b := make([]byte, 5)
	b[0] = 0xb8
	binary.LittleEndian.PutUint32(b[1:], uint32(num)) //nolint:gosec // G115: num is a small syscall number, safe
	return b
}

// syscallInsn is the x86_64 SYSCALL instruction (2 bytes)
var syscallInsn = []byte{0x0f, 0x05}

// nopInsn is NOP (1 byte)
var nopInsn = []byte{0x90}

func newAnalyzer() *LibcWrapperAnalyzer {
	return NewLibcWrapperAnalyzer(elfanalyzer.NewSyscallAnalyzer())
}

// TestLibcWrapperAnalyzer_NormalDetection verifies that a small function
// containing a single syscall instruction is detected.
func TestLibcWrapperAnalyzer_NormalDetection(t *testing.T) {
	// Function at base=0x1000 containing: mov $41, %eax; syscall
	funcCode := append(movEAX(41), syscallInsn...)
	base := uint64(0x1000)
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: funcCode,
		textBase: base,
		symbols: []elfSym{
			{name: "socket", value: base, size: uint64(len(funcCode)), global: true}, //nolint:gosec // G115
		},
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	entries, err := newAnalyzer().Analyze(elfFile)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "socket", entries[0].Name)
	assert.Equal(t, 41, entries[0].Number)
}

// TestLibcWrapperAnalyzer_SizeFilter verifies that functions > 256 bytes are excluded.
func TestLibcWrapperAnalyzer_SizeFilter(t *testing.T) {
	// Build a 257-byte function (1 syscall + 255 NOPs)
	big := append(movEAX(41), syscallInsn...)
	for len(big) < 257 {
		big = append(big, nopInsn...)
	}
	base := uint64(0x1000)
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: big,
		textBase: base,
		symbols: []elfSym{
			{name: "big_func", value: base, size: uint64(len(big)), global: true}, //nolint:gosec // G115
		},
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	entries, err := newAnalyzer().Analyze(elfFile)
	require.NoError(t, err)
	assert.Empty(t, entries, "functions > 256 bytes should be excluded")
}

// TestLibcWrapperAnalyzer_MultipleDifferentSyscalls verifies that functions with
// two different syscall numbers are excluded.
func TestLibcWrapperAnalyzer_MultipleDifferentSyscalls(t *testing.T) {
	// mov $41, %eax; syscall; mov $42, %eax; syscall
	funcCode := append(movEAX(41), syscallInsn...)
	funcCode = append(funcCode, movEAX(42)...)
	funcCode = append(funcCode, syscallInsn...)
	base := uint64(0x1000)
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: funcCode,
		textBase: base,
		symbols: []elfSym{
			{name: "multi", value: base, size: uint64(len(funcCode)), global: true}, //nolint:gosec // G115
		},
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	entries, err := newAnalyzer().Analyze(elfFile)
	require.NoError(t, err)
	assert.Empty(t, entries, "functions with multiple different syscall numbers should be excluded")
}

// TestLibcWrapperAnalyzer_SameSyscallMultipleTimes verifies that functions with
// multiple syscall instructions all having the same number are accepted.
func TestLibcWrapperAnalyzer_SameSyscallMultipleTimes(t *testing.T) {
	// mov $41, %eax; syscall; mov $41, %eax; syscall
	funcCode := append(movEAX(41), syscallInsn...)
	funcCode = append(funcCode, movEAX(41)...)
	funcCode = append(funcCode, syscallInsn...)
	base := uint64(0x1000)
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: funcCode,
		textBase: base,
		symbols: []elfSym{
			{name: "socket", value: base, size: uint64(len(funcCode)), global: true}, //nolint:gosec // G115
		},
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	entries, err := newAnalyzer().Analyze(elfFile)
	require.NoError(t, err)
	require.Len(t, entries, 1, "functions with same syscall number repeated should be accepted")
	assert.Equal(t, 41, entries[0].Number)
}

// TestLibcWrapperAnalyzer_NoSyscall verifies that functions without syscall
// instructions are excluded.
func TestLibcWrapperAnalyzer_NoSyscall(t *testing.T) {
	// mov $41, %eax (no syscall)
	funcCode := movEAX(41)
	base := uint64(0x1000)
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: funcCode,
		textBase: base,
		symbols: []elfSym{
			{name: "non_wrapper", value: base, size: uint64(len(funcCode)), global: true}, //nolint:gosec // G115
		},
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	entries, err := newAnalyzer().Analyze(elfFile)
	require.NoError(t, err)
	assert.Empty(t, entries, "functions without syscall instructions should be excluded")
}

// TestLibcWrapperAnalyzer_SortOrder verifies that WrapperEntry results are sorted
// by Number ascending, then Name ascending.
func TestLibcWrapperAnalyzer_SortOrder(t *testing.T) {
	// Build .text with three functions:
	//   - "write"   at 0x1000: mov $1, %eax; syscall   (number=1)
	//   - "read"    at 0x100c: mov $0, %eax; syscall   (number=0)
	//   - "socket"  at 0x1018: mov $41, %eax; syscall  (number=41)
	// Expected sort: read(0), write(1), socket(41)
	funcSize := uint64(len(movEAX(0)) + len(syscallInsn)) //nolint:gosec // G115
	textCode := []byte{}
	offsets := []uint64{0, funcSize, funcSize * 2}

	textCode = append(textCode, movEAX(1)...)
	textCode = append(textCode, syscallInsn...)
	textCode = append(textCode, movEAX(0)...)
	textCode = append(textCode, syscallInsn...)
	textCode = append(textCode, movEAX(41)...)
	textCode = append(textCode, syscallInsn...)

	base := uint64(0x1000)
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: textCode,
		textBase: base,
		symbols: []elfSym{
			{name: "write", value: base + offsets[0], size: funcSize, global: true},
			{name: "read", value: base + offsets[1], size: funcSize, global: true},
			{name: "socket", value: base + offsets[2], size: funcSize, global: true},
		},
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	entries, err := newAnalyzer().Analyze(elfFile)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, 0, entries[0].Number, "first entry should be read (number=0)")
	assert.Equal(t, "read", entries[0].Name)
	assert.Equal(t, 1, entries[1].Number, "second entry should be write (number=1)")
	assert.Equal(t, "write", entries[1].Name)
	assert.Equal(t, 41, entries[2].Number, "third entry should be socket (number=41)")
	assert.Equal(t, "socket", entries[2].Name)
}

// TestLibcWrapperAnalyzer_IndirectSyscallExcluded verifies that functions
// where the syscall number cannot be determined as "immediate" are excluded.
func TestLibcWrapperAnalyzer_IndirectSyscallExcluded(t *testing.T) {
	// mov %ebx, %eax; syscall  (indirect setting, not immediate)
	funcCode := []byte{
		0x89, 0xd8, // mov %ebx, %eax
		0x0f, 0x05, // syscall
	}
	base := uint64(0x1000)
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: funcCode,
		textBase: base,
		symbols: []elfSym{
			{name: "indirect_func", value: base, size: uint64(len(funcCode)), global: true}, //nolint:gosec // G115
		},
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	entries, err := newAnalyzer().Analyze(elfFile)
	require.NoError(t, err)
	assert.Empty(t, entries, "functions with indirect syscall number should be excluded")
}

// TestLibcWrapperAnalyzer_UnsupportedArchitecture verifies that an unsupported
// ELF architecture returns *elfanalyzer.UnsupportedArchitectureError.
func TestLibcWrapperAnalyzer_UnsupportedArchitecture(t *testing.T) {
	// Build ELF with EM_386 (unsupported architecture)
	funcCode := append(movEAX(41), syscallInsn...)
	base := uint64(0x1000)
	eb := &elfBuilder{
		machine:  elf.EM_386, // unsupported
		textCode: funcCode,
		textBase: base,
		symbols: []elfSym{
			{name: "socket", value: base, size: uint64(len(funcCode)), global: true}, //nolint:gosec // G115
		},
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	_, err := newAnalyzer().Analyze(elfFile)
	require.Error(t, err)
	var archErr *elfanalyzer.UnsupportedArchitectureError
	require.ErrorAs(t, err, &archErr, "should return UnsupportedArchitectureError")
	assert.Equal(t, elf.EM_386, archErr.Machine)
}

// TestLibcWrapperAnalyzer_NoSymbols verifies that an ELF without .dynsym
// returns an empty slice with no error.
func TestLibcWrapperAnalyzer_NoSymbols(t *testing.T) {
	funcCode := append(movEAX(41), syscallInsn...)
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: funcCode,
		textBase: 0x1000,
		noDynsym: true, // no .dynsym section
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	entries, err := newAnalyzer().Analyze(elfFile)
	require.NoError(t, err, "ErrNoSymbols should not be returned as error")
	assert.Empty(t, entries)
}

// TestLibcWrapperAnalyzer_DynsymReadError verifies that a DynamicSymbols read
// error (non-ErrNoSymbols) causes ErrExportSymbolsFailed to be returned.
func TestLibcWrapperAnalyzer_DynsymReadError(t *testing.T) {
	funcCode := append(movEAX(41), syscallInsn...)
	eb := &elfBuilder{
		machine:   elf.EM_X86_64,
		textCode:  funcCode,
		textBase:  0x1000,
		badDynsym: true, // .dynsym section offset beyond file end
		symbols: []elfSym{
			{name: "socket", value: 0x1000, size: uint64(len(funcCode)), global: true}, //nolint:gosec // G115
		},
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	_, err := newAnalyzer().Analyze(elfFile)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrExportSymbolsFailed),
		"expected ErrExportSymbolsFailed, got: %v", err)
}

// TestLibcWrapperAnalyzer_NoTextSection verifies that an ELF without a .text
// section returns nil wrappers and no error (treated as empty libc).
func TestLibcWrapperAnalyzer_NoTextSection(t *testing.T) {
	// Use elfBuilder with noText=true: the code section is named ".data" instead
	// of ".text", so elf.File.Section(".text") returns nil.
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: append(movEAX(41), syscallInsn...),
		textBase: 0x1000,
		noText:   true,
	}
	elfFile := eb.build(t)
	defer elfFile.Close()

	// Precondition: Section(".text") must be nil.
	require.Nil(t, elfFile.Section(".text"), "precondition: no .text section")

	entries, analyzeErr := newAnalyzer().Analyze(elfFile)
	require.NoError(t, analyzeErr, "no .text section should not be an error")
	assert.Nil(t, entries, "no .text section should return nil wrappers")
}
