//go:build test

package elfanalyzer

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
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
	header[4] = 2                                                       // ELFCLASS64
	header[5] = 1                                                       // ELFDATA2LSB
	header[6] = 1                                                       // EV_CURRENT
	binary.LittleEndian.PutUint16(header[16:18], 2)                     // ET_EXEC
	binary.LittleEndian.PutUint16(header[18:20], 62)                    // EM_X86_64
	binary.LittleEndian.PutUint32(header[20:24], 1)                     // EV_CURRENT
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
	binary.LittleEndian.PutUint32(sh1[0:4], 1)          // sh_name: ".text" at offset 1
	binary.LittleEndian.PutUint32(sh1[4:8], 1)          // SHT_PROGBITS
	binary.LittleEndian.PutUint64(sh1[8:16], 6)         // SHF_ALLOC|SHF_EXECINSTR
	binary.LittleEndian.PutUint64(sh1[16:24], 0x401000) // sh_addr (textStart)
	buf.Write(sh1[:])

	// Section 2: .gopclntab
	var sh2 [shEntrySize]byte
	binary.LittleEndian.PutUint32(sh2[0:4], 7)                          // sh_name: ".gopclntab" at offset 7
	binary.LittleEndian.PutUint32(sh2[4:8], 1)                          // SHT_PROGBITS
	binary.LittleEndian.PutUint64(sh2[24:32], uint64(pclntabOffset))    // sh_offset
	binary.LittleEndian.PutUint64(sh2[32:40], uint64(len(pclntabData))) // sh_size
	buf.Write(sh2[:])

	// Section 3: .shstrtab
	var sh3 [shEntrySize]byte
	binary.LittleEndian.PutUint32(sh3[0:4], 18)                       // sh_name: ".shstrtab" at offset 18
	binary.LittleEndian.PutUint32(sh3[4:8], 3)                        // SHT_STRTAB
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
	f, err := elf.Open("testdata/no_network.elf")
	require.NoError(t, err)
	defer f.Close()

	_, err = ParsePclntab(f)
	assert.ErrorIs(t, err, ErrNoPclntab)
}

// TestParsePclntab_InvalidData verifies behavior when .gopclntab contains
// data that debug/gosym cannot interpret as valid pclntab.
//
// NOTE: Unlike the previous hand-rolled parser that returned ErrUnsupportedPclntab
// or ErrInvalidPclntab for bad magic/short data, debug/gosym silently accepts
// invalid data and returns an empty function table (no error). This is the
// documented behavior of the standard library package. ParsePclntab therefore
// succeeds but returns a result with no functions when the data is unrecognizable.
func TestParsePclntab_InvalidData(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{
			name: "empty pclntab",
			data: []byte{},
		},
		{
			name: "too short for header",
			data: []byte{0x01, 0x02, 0x03},
		},
		{
			name: "invalid magic bytes",
			data: []byte{0x01, 0x02, 0x03, 0x04, 0x00, 0x00, 0x01, 0x08},
		},
		{
			name: "random garbage",
			data: bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 8),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := openELFWithPclntab(t, tc.data)

			result, err := ParsePclntab(f)

			// debug/gosym does not return errors for unrecognizable data;
			// it returns an empty table. Verify ParsePclntab reflects that.
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Empty(t, result)
		})
	}
}

func TestParsePclntab_ErrorWrapping(t *testing.T) {
	// Verify the exported error values are properly defined for use with errors.Is.
	assert.Equal(t, "no .gopclntab section found", ErrNoPclntab.Error())
	assert.Equal(t, "unsupported pclntab format", ErrUnsupportedPclntab.Error())
	assert.Equal(t, "invalid pclntab structure", ErrInvalidPclntab.Error())
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
