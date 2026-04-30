// Package elfanalyzertesting provides test helpers for the elfanalyzer package.
package elfanalyzertesting

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateStaticELFFile creates a minimal static ELF file at the given path for testing.
// The file has no .dynsym section, simulating a statically linked binary.
func CreateStaticELFFile(t *testing.T, path string) {
	t.Helper()

	// Create a minimal ELF header for x86_64
	// This is a valid ELF header that will parse but has no .dynsym section
	elfHeader := []byte{
		// ELF magic
		0x7f, 'E', 'L', 'F',
		// Class: 64-bit
		0x02,
		// Data: little endian
		0x01,
		// Version
		0x01,
		// OS/ABI: System V
		0x00,
		// ABI version
		0x00,
		// Padding
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Type: Executable
		0x02, 0x00,
		// Machine: x86_64
		0x3e, 0x00,
		// Version
		0x01, 0x00, 0x00, 0x00,
		// Entry point
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Program header offset
		0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Section header offset (0 = none)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Flags
		0x00, 0x00, 0x00, 0x00,
		// ELF header size
		0x40, 0x00,
		// Program header size
		0x38, 0x00,
		// Number of program headers
		0x00, 0x00,
		// Section header size
		0x40, 0x00,
		// Number of section headers
		0x00, 0x00,
		// Section name string table index
		0x00, 0x00,
	}

	err := os.WriteFile(path, elfHeader, 0o644) //nolint:gosec // test helper: 0644 is intentional for test files
	require.NoError(t, err)

	// Verify it can be parsed as ELF
	f, err := os.Open(path) //nolint:gosec // test helper: path is provided by the test
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	_, err = elf.NewFile(f)
	require.NoError(t, err)
}

// CreateDynamicELFFile creates a minimal dynamic ELF file at the given path for testing.
// The file has a .dynsym section with no network symbols, simulating a CGO binary
// where Go runtime issues syscalls directly without importing libc's socket symbol.
// The .dynsym section contains only a basic symbol (e.g., "__libc_start_main") so
// that DynamicSymbols() succeeds but CheckDynamicSymbols returns NoNetworkSymbols.
func CreateDynamicELFFile(t *testing.T, path string) {
	t.Helper()

	// We build a minimal ELF64 LE binary with the following sections:
	//   [0] null
	//   [1] .dynsym  (1 symbol: __libc_start_main, undefined - non-network)
	//   [2] .dynstr  (string table for .dynsym)
	//   [3] .shstrtab (section name string table)
	//
	// ELF header: 64 bytes
	// Section headers start after the ELF header.
	// We don't include program headers for simplicity.

	const (
		elfHeaderSize  = 64
		sectionHdrSize = 64
		numSections    = 4
		// elf64SymSize is the size of an Elf64_Sym structure in bytes.
		elf64SymSize = 24
		// stInfoShift is the bit shift for the binding field in Elf64_Sym.st_info.
		stInfoShift = 4
		// phentSize is the e_phentsize value (Elf64_Phdr size) for ELF64.
		phentSize = 56
		// shstrndxDynamic is the section index of .shstrtab in our minimal ELF.
		shstrndxDynamic = 3
		// dynstrSectionIdx is the section index of .dynstr (link field for .dynsym).
		dynstrSectionIdx = 2
		// firstGlobalSymIdx is the info field for .dynsym (index of first global symbol).
		firstGlobalSymIdx = 1
	)

	// .shstrtab: section name string table
	// Index 0: "" (null), 1: ".dynsym", 8: ".dynstr", 16: ".shstrtab"
	shstrtab := []byte("\x00.dynsym\x00.dynstr\x00.shstrtab\x00")
	const (
		shstrtabOffNull     = 0
		shstrtabOffDynsym   = 1  // ".dynsym"
		shstrtabOffDynstr   = 9  // ".dynstr"
		shstrtabOffShstrtab = 17 // ".shstrtab"
	)

	// .dynstr: dynamic symbol string table
	// Index 0: "" (null), 1: "__libc_start_main"
	dynstr := []byte("\x00__libc_start_main\x00")
	const dynstrOffLibcStart = 1

	// .dynsym: one symbol entry (Elf64_Sym = elf64SymSize bytes each)
	// [0] null symbol (required)
	// [1] __libc_start_main (undefined, no network relevance)
	sym0 := make([]byte, elf64SymSize) // null symbol
	sym1 := make([]byte, elf64SymSize)
	binary.LittleEndian.PutUint32(sym1[0:4], dynstrOffLibcStart)     // st_name
	sym1[4] = byte(elf.STT_FUNC) | byte(elf.STB_GLOBAL<<stInfoShift) // st_info
	sym1[5] = 0                                                      // st_other
	binary.LittleEndian.PutUint16(sym1[6:8], uint16(elf.SHN_UNDEF))  // st_shndx (undefined)
	// st_value, st_size = 0
	dynsymData := append(sym0, sym1...) //nolint:gocritic // intentional append to nil

	// Layout: ELF header | section headers | .dynsym | .dynstr | .shstrtab
	sectionHdrsOffset := int64(elfHeaderSize)
	sectionHdrsSize := int64(numSections * sectionHdrSize)
	dynsymOffset := sectionHdrsOffset + sectionHdrsSize
	dynstrOffset := dynsymOffset + int64(len(dynsymData))
	shstrtabOffset := dynstrOffset + int64(len(dynstr))

	buf := &bytes.Buffer{}

	// ELF header (Elf64_Ehdr)
	elfHdr := make([]byte, elfHeaderSize)
	copy(elfHdr[0:4], []byte{0x7f, 'E', 'L', 'F'})
	elfHdr[4] = byte(elf.ELFCLASS64)
	elfHdr[5] = byte(elf.ELFDATA2LSB)
	elfHdr[6] = byte(elf.EV_CURRENT)
	elfHdr[7] = byte(elf.ELFOSABI_NONE)
	binary.LittleEndian.PutUint16(elfHdr[16:18], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(elfHdr[18:20], uint16(elf.EM_X86_64))
	binary.LittleEndian.PutUint32(elfHdr[20:24], uint32(elf.EV_CURRENT))
	// e_entry = 0
	// e_phoff = 0 (no program headers)
	binary.LittleEndian.PutUint64(elfHdr[40:48], uint64(sectionHdrsOffset)) //nolint:gosec // G115: sectionHdrsOffset is a positive layout offset, no overflow risk
	// e_flags = 0
	binary.LittleEndian.PutUint16(elfHdr[52:54], uint16(elfHeaderSize))  // e_ehsize
	binary.LittleEndian.PutUint16(elfHdr[54:56], phentSize)              // e_phentsize (irrelevant, no phdr)
	binary.LittleEndian.PutUint16(elfHdr[56:58], 0)                      // e_phnum
	binary.LittleEndian.PutUint16(elfHdr[58:60], uint16(sectionHdrSize)) // e_shentsize
	binary.LittleEndian.PutUint16(elfHdr[60:62], uint16(numSections))    // e_shnum
	binary.LittleEndian.PutUint16(elfHdr[62:64], shstrndxDynamic)        // e_shstrndx = .shstrtab section index
	buf.Write(elfHdr)

	// Section headers
	writeSectionHdr := func(nameIdx uint32, shType elf.SectionType, flags elf.SectionFlag,
		offset, size, link, info uint64, entSize uint32,
	) {
		sh := make([]byte, sectionHdrSize)
		binary.LittleEndian.PutUint32(sh[0:4], nameIdx)
		binary.LittleEndian.PutUint32(sh[4:8], uint32(shType))
		binary.LittleEndian.PutUint64(sh[8:16], uint64(flags))
		// sh_addr = 0
		binary.LittleEndian.PutUint64(sh[24:32], offset)
		binary.LittleEndian.PutUint64(sh[32:40], size)
		binary.LittleEndian.PutUint32(sh[40:44], uint32(link)) //nolint:gosec // G115: link is a section index, always fits uint32
		binary.LittleEndian.PutUint32(sh[44:48], uint32(info)) //nolint:gosec // G115: info is a symbol index, always fits uint32
		// sh_addralign = 1
		binary.LittleEndian.PutUint64(sh[48:56], 1)
		binary.LittleEndian.PutUint32(sh[56:60], entSize)
		buf.Write(sh)
	}

	// [0] null section
	writeSectionHdr(shstrtabOffNull, elf.SHT_NULL, 0, 0, 0, 0, 0, 0)
	// [1] .dynsym: link=dynstrSectionIdx (.dynstr index), info=firstGlobalSymIdx (first global symbol)
	writeSectionHdr(uint32(shstrtabOffDynsym), elf.SHT_DYNSYM, elf.SHF_ALLOC,
		uint64(dynsymOffset), uint64(len(dynsymData)), dynstrSectionIdx, firstGlobalSymIdx, elf64SymSize) //nolint:gosec // G115: dynsymOffset is a positive layout offset, no overflow risk
	// [2] .dynstr
	writeSectionHdr(uint32(shstrtabOffDynstr), elf.SHT_STRTAB, elf.SHF_ALLOC,
		uint64(dynstrOffset), uint64(len(dynstr)), 0, 0, 0) //nolint:gosec // G115: dynstrOffset is a positive layout offset, no overflow risk
	// [3] .shstrtab
	writeSectionHdr(uint32(shstrtabOffShstrtab), elf.SHT_STRTAB, 0,
		uint64(shstrtabOffset), uint64(len(shstrtab)), 0, 0, 0) //nolint:gosec // G115: shstrtabOffset is a positive layout offset, no overflow risk

	// Section data
	buf.Write(dynsymData)
	buf.Write(dynstr)
	buf.Write(shstrtab)

	err := os.WriteFile(path, buf.Bytes(), 0o644) //nolint:gosec // test helper: 0644 is intentional for test files
	require.NoError(t, err)

	// Verify it can be parsed as ELF with .dynsym
	f, err := os.Open(path) //nolint:gosec // test helper: path is provided by the test
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	elfFile, err := elf.NewFile(f)
	require.NoError(t, err)
	syms, err := elfFile.DynamicSymbols()
	require.NoError(t, err)
	require.NotEmpty(t, syms, "dynamic ELF must have at least one symbol")
}
