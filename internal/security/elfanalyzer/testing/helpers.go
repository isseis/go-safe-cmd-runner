//go:build test

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

// SymbolSpec defines a symbol to include in a test ELF binary.
type SymbolSpec struct {
	Name string
}

// CreateDynamicELFFile creates a minimal dynamic ELF file for testing.
// The file has a .dynsym section with no network symbols, simulating a CGO binary
// where Go runtime issues syscalls directly without importing libc's socket symbol.
// The .dynsym section contains only a basic symbol ("__libc_start_main") so
// that DynamicSymbols() succeeds but CheckDynamicSymbols returns NoNetworkSymbols.
func CreateDynamicELFFile(t *testing.T, path string) {
	t.Helper()
	CreateELFWithSymbols(t, path, []SymbolSpec{{Name: "__libc_start_main"}})
}

// CreateELFWithSymbols creates a minimal dynamic ELF64 LE file at the given path.
// All symbols are SHN_UNDEF with no VERNEED section (musl-style binary).
// The generated ELF has the following sections:
//
//	[0] null
//	[1] .dynsym  (null symbol + one entry per SymbolSpec)
//	[2] .dynstr  (string table for .dynsym)
//	[3] .shstrtab (section name string table)
func CreateELFWithSymbols(t *testing.T, path string, symbols []SymbolSpec) {
	t.Helper()

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
		// shstrndx is the section index of .shstrtab.
		shstrndx = 3
		// dynstrIdx is the section index of .dynstr (link field for .dynsym).
		dynstrIdx = 2
	)

	// .shstrtab: section name string table
	// offsets: 0="", 1=".dynsym", 9=".dynstr", 17=".shstrtab"
	shstrtab := []byte("\x00.dynsym\x00.dynstr\x00.shstrtab\x00")
	const (
		shOffNull     = 0
		shOffDynsym   = 1
		shOffDynstr   = 9
		shOffShstrtab = 17
	)

	// .dynstr: "\x00" + name1 + "\x00" + name2 + "\x00" + ...
	dynstr := []byte{0}
	nameOffsets := make([]int, len(symbols))
	for i, s := range symbols {
		nameOffsets[i] = len(dynstr)
		dynstr = append(dynstr, []byte(s.Name)...)
		dynstr = append(dynstr, 0)
	}

	// .dynsym: null symbol + one entry per SymbolSpec
	dynsymData := make([]byte, (1+len(symbols))*elf64SymSize)
	for i := range symbols {
		off := (1 + i) * elf64SymSize
		sym := dynsymData[off : off+elf64SymSize]
		binary.LittleEndian.PutUint32(sym[0:4], uint32(nameOffsets[i])) //nolint:gosec // nameOffsets[i] is a dynstr byte offset, always fits uint32
		sym[4] = byte(elf.STT_FUNC) | byte(elf.STB_GLOBAL<<stInfoShift)
		binary.LittleEndian.PutUint16(sym[6:8], uint16(elf.SHN_UNDEF))
	}

	// Layout: ELF header | section headers | .dynsym | .dynstr | .shstrtab
	shdrsOffset := int64(elfHeaderSize)
	shdrsSize := int64(numSections * sectionHdrSize)
	dynsymOffset := shdrsOffset + shdrsSize
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
	binary.LittleEndian.PutUint64(elfHdr[40:48], uint64(shdrsOffset))    //nolint:gosec // G115: shdrsOffset is a positive layout offset, no overflow risk
	binary.LittleEndian.PutUint16(elfHdr[52:54], uint16(elfHeaderSize))  // e_ehsize
	binary.LittleEndian.PutUint16(elfHdr[54:56], phentSize)              // e_phentsize (irrelevant, no phdr)
	binary.LittleEndian.PutUint16(elfHdr[56:58], 0)                      // e_phnum
	binary.LittleEndian.PutUint16(elfHdr[58:60], uint16(sectionHdrSize)) // e_shentsize
	binary.LittleEndian.PutUint16(elfHdr[60:62], uint16(numSections))    // e_shnum
	binary.LittleEndian.PutUint16(elfHdr[62:64], shstrndx)               // e_shstrndx
	buf.Write(elfHdr)

	// Section header helper
	writeSHdr := func(nameIdx uint32, shType elf.SectionType, flags elf.SectionFlag,
		offset, size, link, info uint64, entSize uint32,
	) {
		sh := make([]byte, sectionHdrSize)
		binary.LittleEndian.PutUint32(sh[0:4], nameIdx)
		binary.LittleEndian.PutUint32(sh[4:8], uint32(shType))
		binary.LittleEndian.PutUint64(sh[8:16], uint64(flags))
		binary.LittleEndian.PutUint64(sh[24:32], offset)
		binary.LittleEndian.PutUint64(sh[32:40], size)
		binary.LittleEndian.PutUint32(sh[40:44], uint32(link)) //nolint:gosec // G115: link is a section index, always fits uint32
		binary.LittleEndian.PutUint32(sh[44:48], uint32(info)) //nolint:gosec // G115: info is a symbol index, always fits uint32
		binary.LittleEndian.PutUint64(sh[48:56], 1)            // sh_addralign = 1
		binary.LittleEndian.PutUint32(sh[56:60], entSize)
		buf.Write(sh)
	}

	writeSHdr(shOffNull, elf.SHT_NULL, 0, 0, 0, 0, 0, 0)
	writeSHdr(uint32(shOffDynsym), elf.SHT_DYNSYM, elf.SHF_ALLOC,
		uint64(dynsymOffset), uint64(len(dynsymData)), dynstrIdx, 1, elf64SymSize) //nolint:gosec // G115: offsets are positive layout values, no overflow risk
	writeSHdr(uint32(shOffDynstr), elf.SHT_STRTAB, elf.SHF_ALLOC,
		uint64(dynstrOffset), uint64(len(dynstr)), 0, 0, 0) //nolint:gosec // G115: dynstrOffset is a positive layout offset, no overflow risk
	writeSHdr(uint32(shOffShstrtab), elf.SHT_STRTAB, 0,
		uint64(shstrtabOffset), uint64(len(shstrtab)), 0, 0, 0) //nolint:gosec // G115: shstrtabOffset is a positive layout offset, no overflow risk

	buf.Write(dynsymData)
	buf.Write(dynstr)
	buf.Write(shstrtab)

	err := os.WriteFile(path, buf.Bytes(), 0o644) //nolint:gosec // test helper: 0644 is intentional for test files
	require.NoError(t, err)

	// Verify the generated ELF file can be parsed and DynamicSymbols() succeeds.
	// This catches malformed ELF generation early in the test helper rather than
	// in later analyzer logic, making test failures easier to debug.
	f, err := os.Open(path) //nolint:gosec // test helper: path is provided by the test
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	file, err := elf.NewFile(f)
	require.NoError(t, err)
	_, err = file.DynamicSymbols()
	require.NoError(t, err, "generated ELF must have valid .dynsym section")
}
