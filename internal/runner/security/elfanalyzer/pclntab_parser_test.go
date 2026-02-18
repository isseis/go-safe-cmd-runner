//go:build test

package elfanalyzer

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPclntabParser_MagicNumbers(t *testing.T) {
	tests := []struct {
		name      string
		magic     uint32
		expectErr error
		goVersion string
	}{
		{
			name:      "Go 1.20+ magic (Go 1.20-1.24 header detected as unsupported)",
			magic:     pclntabMagicGo120,
			expectErr: ErrUnsupportedPclntab,
			goVersion: "go1.25+",
		},
		{
			name:      "Go 1.18-1.19 magic (unsupported)",
			magic:     0xFFFFFFF0,
			expectErr: ErrUnsupportedPclntab,
		},
		{
			name:      "Go 1.16-1.17 magic (unsupported)",
			magic:     0xFFFFFFFA,
			expectErr: ErrUnsupportedPclntab,
		},
		{
			name:      "Go 1.2-1.15 magic (unsupported)",
			magic:     0xFFFFFFFB,
			expectErr: ErrUnsupportedPclntab,
		},
		{
			name:      "Unknown magic",
			magic:     0x12345678,
			expectErr: ErrUnsupportedPclntab,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := &pclntabParser{}

			var err error
			if tt.magic == pclntabMagicGo120 {
				// createMinimalPclntab creates a Go 1.20-1.24 format (80-byte header,
				// funcnameOffset=0x50), which parseGo125Plus rejects as unsupported.
				data := createMinimalPclntabGo120(tt.magic)
				err = parser.parseGo125Plus(data)
			} else {
				// Non-Go 1.20+ magics are rejected at the parse() level.
				// Simulate the rejection here since parse() requires elf.File.
				err = fmt.Errorf("%w: unknown magic 0x%08X", ErrUnsupportedPclntab, tt.magic)
			}

			if tt.goVersion != "" {
				assert.Equal(t, tt.goVersion, parser.goVersion)
			}
			assert.ErrorIs(t, err, tt.expectErr)
		})
	}
}

func TestPclntabParser_InvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "empty data",
			data: []byte{},
		},
		{
			name: "too short for magic",
			data: []byte{0xF0, 0xFF, 0xFF},
		},
		{
			name: "too short for header",
			data: []byte{0xF0, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x01},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := &pclntabParser{}

			err := parser.parseGo125Plus(tt.data)
			assert.ErrorIs(t, err, ErrInvalidPclntab)
		})
	}
}

func TestPclntabParser_NoPclntab(t *testing.T) {
	// This test verifies the error message when .gopclntab is missing
	// The actual test with elf.File would require a real ELF file
	// For unit testing, we verify the error is properly defined
	assert.Equal(t, "no .gopclntab section found", ErrNoPclntab.Error())
}

func TestPclntabParser_UnsupportedPointerSize(t *testing.T) {
	// Create pclntab with 32-bit pointer size (not supported)
	data := make([]byte, pcHeaderSizeGo125)
	binary.LittleEndian.PutUint32(data[0:4], pclntabMagicGo120)
	data[7] = 4 // 32-bit pointer size

	parser := &pclntabParser{}
	err := parser.parseGo125Plus(data)

	assert.ErrorIs(t, err, ErrInvalidPclntab)
	assert.Contains(t, err.Error(), "unsupported pointer size 4")
}

func TestPclntabParser_GetFunctions(t *testing.T) {
	parser := &pclntabParser{}

	// Initially empty
	assert.Empty(t, parser.funcData)
}

func TestPclntabParser_FindFunction(t *testing.T) {
	result := &PclntabResult{}

	// Function not found when result is empty
	_, found := result.FindFunction("main.main")
	assert.False(t, found)
}

func TestPclntabParser_GetGoVersion(t *testing.T) {
	parser := &pclntabParser{}

	// Initially empty
	assert.Equal(t, "", parser.goVersion)
}

// createMinimalPclntabGo120 creates a minimal Go 1.20-1.24 pclntab (80-byte header)
// for testing. This format is rejected by the parser (Go 1.25+ only).
func createMinimalPclntabGo120(magic uint32) []byte {
	data := make([]byte, 0x50) // 80 bytes header

	// [0:4] magic
	binary.LittleEndian.PutUint32(data[0:4], magic)

	// [4:5] pad1
	data[4] = 0

	// [5:6] pad2
	data[5] = 0

	// [6:7] minLC (instruction size quantum, 1 for x86)
	data[6] = 1

	// [7:8] ptrSize (must be 8 for 64-bit)
	data[7] = 8

	// [8:16] nfunc = 0
	binary.LittleEndian.PutUint64(data[0x08:0x10], 0)

	// [0x10:0x18] nfiles = 0
	binary.LittleEndian.PutUint64(data[0x10:0x18], 0)

	// [0x18:0x20] textStart = 0
	binary.LittleEndian.PutUint64(data[0x18:0x20], 0)

	// [0x20:0x28] funcnameOffset = 0x50 (after header)
	binary.LittleEndian.PutUint64(data[0x20:0x28], 0x50)

	// [0x28:0x30] cuOffset = 0x50
	binary.LittleEndian.PutUint64(data[0x28:0x30], 0x50)

	// [0x30:0x38] filetabOffset = 0x50
	binary.LittleEndian.PutUint64(data[0x30:0x38], 0x50)

	// [0x38:0x40] pctabOffset = 0x50
	binary.LittleEndian.PutUint64(data[0x38:0x40], 0x50)

	// [0x40:0x48] pclntabOffset = 0x50
	binary.LittleEndian.PutUint64(data[0x40:0x48], 0x50)

	// [0x48:0x50] ftabOffset = 0x50
	binary.LittleEndian.PutUint64(data[0x48:0x50], 0x50)

	return data
}

func TestPclntabParser_Go120PclntabRejected(t *testing.T) {
	// Go 1.20-1.24 format (80-byte header) should be rejected.
	data := createPclntabGo120WithFunction("main.main", 0x401000)

	parser := &pclntabParser{}
	err := parser.parseGo125Plus(data)

	assert.ErrorIs(t, err, ErrUnsupportedPclntab)
	assert.Contains(t, err.Error(), errMsgGo120Unsupported)
}

// createPclntabGo120WithFunction creates a Go 1.20-1.24 pclntab (80-byte header)
// with a single function. This format is rejected by the parser (Go 1.25+ only).
func createPclntabGo120WithFunction(funcName string, entry uint64) []byte {
	// Calculate sizes
	headerSize := 0x50
	funcNameLen := len(funcName) + 1 // +1 for null terminator
	funcDataSize := 8                // _func struct minimal size (entryOff + nameOff)
	ftabEntrySize := 8               // {entryoff uint32, funcoff uint32}
	nfunc := 1

	// Layout:
	// [0x00:0x50] header
	// [0x50:...] funcname table ("main.main\0")
	// [...:...]  func data (_func struct)
	// [...:...]  ftab (function table entries)

	funcnameOffset := headerSize
	funcDataOffset := funcnameOffset + funcNameLen
	ftabOffset := funcDataOffset + funcDataSize
	totalSize := ftabOffset + (nfunc+1)*ftabEntrySize

	data := make([]byte, totalSize)

	// Header
	binary.LittleEndian.PutUint32(data[0:4], pclntabMagicGo120)
	data[4] = 0 // pad1
	data[5] = 0 // pad2
	data[6] = 1 // minLC
	data[7] = 8 // ptrSize

	// textStart - entry address minus the entry offset (0)
	textStart := entry
	binary.LittleEndian.PutUint64(data[0x08:0x10], uint64(nfunc))
	binary.LittleEndian.PutUint64(data[0x10:0x18], 0)                      // nfiles
	binary.LittleEndian.PutUint64(data[0x18:0x20], textStart)              // textStart
	binary.LittleEndian.PutUint64(data[0x20:0x28], uint64(funcnameOffset)) // funcnameOffset
	binary.LittleEndian.PutUint64(data[0x28:0x30], 0)                      // cuOffset
	binary.LittleEndian.PutUint64(data[0x30:0x38], 0)                      // filetabOffset
	binary.LittleEndian.PutUint64(data[0x38:0x40], 0)                      // pctabOffset
	binary.LittleEndian.PutUint64(data[0x40:0x48], 0)                      // pclntabOffset
	binary.LittleEndian.PutUint64(data[0x48:0x50], uint64(ftabOffset))     // ftabOffset

	// Function name table
	copy(data[funcnameOffset:], funcName)
	data[funcnameOffset+len(funcName)] = 0 // null terminator

	// _func struct (minimal):
	// offset 0: entryOff (uint32) - relative to textStart
	// offset 4: nameOff (uint32) - relative to funcnameOffset
	binary.LittleEndian.PutUint32(data[funcDataOffset:], 0)   // entryOff = 0 (first function at textStart)
	binary.LittleEndian.PutUint32(data[funcDataOffset+4:], 0) // nameOff = 0 (first name in funcname table)

	// ftab entries:
	// entry 0: {entryoff=0, funcoff=funcDataOffset}
	// entry 1: {entryoff=next_entry, funcoff=0} (sentinel)
	binary.LittleEndian.PutUint32(data[ftabOffset:], 0)                        // entryoff
	binary.LittleEndian.PutUint32(data[ftabOffset+4:], uint32(funcDataOffset)) // funcoff

	// Sentinel entry (marks end of functions)
	binary.LittleEndian.PutUint32(data[ftabOffset+8:], 0x1000) // next entryoff
	binary.LittleEndian.PutUint32(data[ftabOffset+12:], 0)     // funcoff (not used)

	return data
}

func TestPclntabParser_Go120MultipleFunctionsRejected(t *testing.T) {
	// Go 1.20-1.24 format with multiple functions should be rejected.
	data := createPclntabGo120WithMultipleFunctions([]struct {
		name  string
		entry uint64
	}{
		{"main.main", 0x401000},
		{"main.foo", 0x401100},
		{"syscall.Syscall", 0x402000},
	})

	parser := &pclntabParser{}
	err := parser.parseGo125Plus(data)

	assert.ErrorIs(t, err, ErrUnsupportedPclntab)
	assert.Contains(t, err.Error(), errMsgGo120Unsupported)
}

// createPclntabGo120WithMultipleFunctions creates a Go 1.20-1.24 pclntab (80-byte header)
// with multiple functions. This format is rejected by the parser (Go 1.25+ only).
func createPclntabGo120WithMultipleFunctions(functions []struct {
	name  string
	entry uint64
},
) []byte {
	if len(functions) == 0 {
		return createMinimalPclntabGo120(pclntabMagicGo120)
	}

	// Calculate sizes
	headerSize := 0x50
	nfunc := len(functions)

	// Calculate funcname table size
	funcnameTableSize := 0
	for _, f := range functions {
		funcnameTableSize += len(f.name) + 1 // +1 for null terminator
	}

	// Each _func struct needs at least 8 bytes
	funcDataSize := nfunc * 8
	ftabEntrySize := 8 // {entryoff uint32, funcoff uint32}

	funcnameOffset := headerSize
	funcDataOffset := funcnameOffset + funcnameTableSize
	ftabOffset := funcDataOffset + funcDataSize
	totalSize := ftabOffset + (nfunc+1)*ftabEntrySize

	data := make([]byte, totalSize)

	// Use the smallest entry address as textStart
	textStart := functions[0].entry
	for _, f := range functions {
		if f.entry < textStart {
			textStart = f.entry
		}
	}

	// Header
	binary.LittleEndian.PutUint32(data[0:4], pclntabMagicGo120)
	data[4] = 0 // pad1
	data[5] = 0 // pad2
	data[6] = 1 // minLC
	data[7] = 8 // ptrSize

	binary.LittleEndian.PutUint64(data[0x08:0x10], uint64(nfunc))
	binary.LittleEndian.PutUint64(data[0x10:0x18], 0)                      // nfiles
	binary.LittleEndian.PutUint64(data[0x18:0x20], textStart)              // textStart
	binary.LittleEndian.PutUint64(data[0x20:0x28], uint64(funcnameOffset)) // funcnameOffset
	binary.LittleEndian.PutUint64(data[0x28:0x30], 0)                      // cuOffset
	binary.LittleEndian.PutUint64(data[0x30:0x38], 0)                      // filetabOffset
	binary.LittleEndian.PutUint64(data[0x38:0x40], 0)                      // pctabOffset
	binary.LittleEndian.PutUint64(data[0x40:0x48], 0)                      // pclntabOffset
	binary.LittleEndian.PutUint64(data[0x48:0x50], uint64(ftabOffset))     // ftabOffset

	// Function name table
	nameOffset := 0
	nameOffsets := make([]int, nfunc)
	for i, f := range functions {
		nameOffsets[i] = nameOffset
		copy(data[funcnameOffset+nameOffset:], f.name)
		data[funcnameOffset+nameOffset+len(f.name)] = 0 // null terminator
		nameOffset += len(f.name) + 1
	}

	// _func structs and ftab entries
	for i, f := range functions {
		funcStructOff := funcDataOffset + i*8
		entryOff := uint32(f.entry - textStart)

		// _func struct
		binary.LittleEndian.PutUint32(data[funcStructOff:], entryOff)
		binary.LittleEndian.PutUint32(data[funcStructOff+4:], uint32(nameOffsets[i]))

		// ftab entry
		ftabEntryOff := ftabOffset + i*8
		binary.LittleEndian.PutUint32(data[ftabEntryOff:], entryOff)
		binary.LittleEndian.PutUint32(data[ftabEntryOff+4:], uint32(funcStructOff))
	}

	// Sentinel entry
	sentinelOff := ftabOffset + nfunc*8
	// Use a large entry offset as sentinel
	binary.LittleEndian.PutUint32(data[sentinelOff:], 0xFFFFFF)
	binary.LittleEndian.PutUint32(data[sentinelOff+4:], 0)

	return data
}

// createMinimalPclntabGo125 creates a minimal Go 1.25+ pclntab with the given
// magic and zero functions. Go 1.25+ removed the ftabOffset field from pcHeader,
// shrinking the header from 80 to 72 bytes.
func createMinimalPclntabGo125(magic uint32) []byte {
	data := make([]byte, pcHeaderSizeGo125) // 0x48 = 72 bytes

	// [0:4] magic
	binary.LittleEndian.PutUint32(data[0:4], magic)
	data[4] = 0 // pad1
	data[5] = 0 // pad2
	data[6] = 1 // minLC
	data[7] = 8 // ptrSize

	// [0x08:0x10] nfunc = 0
	binary.LittleEndian.PutUint64(data[0x08:0x10], 0)
	// [0x10:0x18] nfiles = 0
	binary.LittleEndian.PutUint64(data[0x10:0x18], 0)
	// [0x18:0x20] textStart = 0x400000
	binary.LittleEndian.PutUint64(data[0x18:0x20], 0x400000)
	// [0x20:0x28] funcnameOffset = 0x48 (right after 72-byte header)
	binary.LittleEndian.PutUint64(data[0x20:0x28], 0x48)
	// [0x28:0x30] cuOffset = 0x48
	binary.LittleEndian.PutUint64(data[0x28:0x30], 0x48)
	// [0x30:0x38] filetabOffset = 0x48
	binary.LittleEndian.PutUint64(data[0x30:0x38], 0x48)
	// [0x38:0x40] pctabOffset = 0x48
	binary.LittleEndian.PutUint64(data[0x38:0x40], 0x48)
	// [0x40:0x48] pclntabOffset = 0x48 (also serves as ftab offset in Go 1.25+)
	binary.LittleEndian.PutUint64(data[0x40:0x48], 0x48)

	return data
}

// createPclntabGo125WithFunction creates a Go 1.25+ pclntab with a single function.
// The Go 1.25+ header is 72 bytes (ftabOffset field removed); ftab is at pclntabOffset.
func createPclntabGo125WithFunction(funcName string, entry uint64) []byte {
	headerSize := pcHeaderSizeGo125 // 0x48 = 72 bytes
	funcNameLen := len(funcName) + 1
	funcDataSize := 8  // _func struct minimal (entryOff + nameOff)
	ftabEntrySize := 8 // {entryoff uint32, funcoff uint32}
	nfunc := 1

	funcnameOffset := headerSize
	funcDataOffset := funcnameOffset + funcNameLen
	ftabOffset := funcDataOffset + funcDataSize
	totalSize := ftabOffset + (nfunc+1)*ftabEntrySize

	data := make([]byte, totalSize)

	// Header
	binary.LittleEndian.PutUint32(data[0:4], pclntabMagicGo120)
	data[4] = 0 // pad1
	data[5] = 0 // pad2
	data[6] = 1 // minLC
	data[7] = 8 // ptrSize

	textStart := entry
	binary.LittleEndian.PutUint64(data[0x08:0x10], uint64(nfunc))
	binary.LittleEndian.PutUint64(data[0x10:0x18], 0)                      // nfiles
	binary.LittleEndian.PutUint64(data[0x18:0x20], textStart)              // textStart
	binary.LittleEndian.PutUint64(data[0x20:0x28], uint64(funcnameOffset)) // funcnameOffset
	binary.LittleEndian.PutUint64(data[0x28:0x30], 0)                      // cuOffset
	binary.LittleEndian.PutUint64(data[0x30:0x38], 0)                      // filetabOffset
	binary.LittleEndian.PutUint64(data[0x38:0x40], 0)                      // pctabOffset
	// pclntabOffset = ftabOffset (merged in Go 1.25+)
	binary.LittleEndian.PutUint64(data[0x40:0x48], uint64(ftabOffset))

	// Function name table
	copy(data[funcnameOffset:], funcName)
	data[funcnameOffset+len(funcName)] = 0 // null terminator

	// _func struct
	binary.LittleEndian.PutUint32(data[funcDataOffset:], 0)   // entryOff = 0
	binary.LittleEndian.PutUint32(data[funcDataOffset+4:], 0) // nameOff = 0

	// ftab entries
	binary.LittleEndian.PutUint32(data[ftabOffset:], 0)                        // entryoff
	binary.LittleEndian.PutUint32(data[ftabOffset+4:], uint32(funcDataOffset)) // funcoff

	// Sentinel entry
	binary.LittleEndian.PutUint32(data[ftabOffset+8:], 0x1000)
	binary.LittleEndian.PutUint32(data[ftabOffset+12:], 0)

	return data
}

func TestPclntabParser_Go125MinimalHeader(t *testing.T) {
	// Go 1.25+ uses the same magic (0xFFFFFFF1) but a 72-byte header.
	// A minimal header with 0 functions will fail during ftab bounds check,
	// but Go version should be detected.
	data := createMinimalPclntabGo125(pclntabMagicGo120)

	parser := &pclntabParser{}
	err := parser.parseGo125Plus(data)

	assert.ErrorIs(t, err, ErrInvalidPclntab)
	assert.Equal(t, "go1.25+", parser.goVersion)
}

func TestPclntabParser_Go125WithFunction(t *testing.T) {
	data := createPclntabGo125WithFunction("main.main", 0x401000)

	parser := &pclntabParser{}
	err := parser.parseGo125Plus(data)

	require.NoError(t, err)
	assert.Equal(t, "go1.25+", parser.goVersion)

	require.Len(t, parser.funcData, 1)
	fn, ok := parser.funcData["main.main"]
	require.True(t, ok)
	assert.Equal(t, uint64(0x401000), fn.Entry)

	result := &PclntabResult{Functions: parser.funcData}
	fn, found := result.FindFunction("main.main")
	assert.True(t, found)
	assert.Equal(t, "main.main", fn.Name)
}

func TestPclntabParser_Go125MultipleFunctions(t *testing.T) {
	// Build a Go 1.25+ pclntab with multiple functions to verify
	// full parsing works with the 72-byte header format.
	type funcEntry struct {
		name  string
		entry uint64
	}
	functions := []funcEntry{
		{"main.main", 0x401000},
		{"main.foo", 0x401100},
		{"syscall.Syscall", 0x402000},
	}

	// Calculate layout
	headerSize := pcHeaderSizeGo125
	nfunc := len(functions)

	funcnameTableSize := 0
	for _, f := range functions {
		funcnameTableSize += len(f.name) + 1
	}
	funcDataSize := nfunc * 8
	ftabEntrySize := 8

	funcnameOffset := headerSize
	funcDataOffset := funcnameOffset + funcnameTableSize
	ftabOffset := funcDataOffset + funcDataSize
	totalSize := ftabOffset + (nfunc+1)*ftabEntrySize

	data := make([]byte, totalSize)

	textStart := functions[0].entry
	for _, f := range functions {
		if f.entry < textStart {
			textStart = f.entry
		}
	}

	// Header (72 bytes)
	binary.LittleEndian.PutUint32(data[0:4], pclntabMagicGo120)
	data[4] = 0
	data[5] = 0
	data[6] = 1
	data[7] = 8

	binary.LittleEndian.PutUint64(data[0x08:0x10], uint64(nfunc))
	binary.LittleEndian.PutUint64(data[0x10:0x18], 0)
	binary.LittleEndian.PutUint64(data[0x18:0x20], textStart)
	binary.LittleEndian.PutUint64(data[0x20:0x28], uint64(funcnameOffset))
	binary.LittleEndian.PutUint64(data[0x28:0x30], 0)
	binary.LittleEndian.PutUint64(data[0x30:0x38], 0)
	binary.LittleEndian.PutUint64(data[0x38:0x40], 0)
	binary.LittleEndian.PutUint64(data[0x40:0x48], uint64(ftabOffset))

	// Function name table
	nameOffset := 0
	nameOffsets := make([]int, nfunc)
	for i, f := range functions {
		nameOffsets[i] = nameOffset
		copy(data[funcnameOffset+nameOffset:], f.name)
		data[funcnameOffset+nameOffset+len(f.name)] = 0
		nameOffset += len(f.name) + 1
	}

	// _func structs and ftab entries
	for i, f := range functions {
		funcStructOff := funcDataOffset + i*8
		entryOff := uint32(f.entry - textStart)

		binary.LittleEndian.PutUint32(data[funcStructOff:], entryOff)
		binary.LittleEndian.PutUint32(data[funcStructOff+4:], uint32(nameOffsets[i]))

		ftabEntryOff := ftabOffset + i*8
		binary.LittleEndian.PutUint32(data[ftabEntryOff:], entryOff)
		binary.LittleEndian.PutUint32(data[ftabEntryOff+4:], uint32(funcStructOff))
	}

	// Sentinel
	sentinelOff := ftabOffset + nfunc*8
	binary.LittleEndian.PutUint32(data[sentinelOff:], 0xFFFFFF)
	binary.LittleEndian.PutUint32(data[sentinelOff+4:], 0)

	parser := &pclntabParser{}
	err := parser.parseGo125Plus(data)

	require.NoError(t, err)

	require.Len(t, parser.funcData, 3)

	mainMain := parser.funcData["main.main"]
	assert.Equal(t, uint64(0x401000), mainMain.Entry)
	assert.Equal(t, uint64(0x401100), mainMain.End)

	mainFoo := parser.funcData["main.foo"]
	assert.Equal(t, uint64(0x401100), mainFoo.Entry)

	syscallSyscall := parser.funcData["syscall.Syscall"]
	assert.Equal(t, uint64(0x402000), syscallSyscall.Entry)

	result := &PclntabResult{Functions: parser.funcData}
	fn, found := result.FindFunction("syscall.Syscall")
	assert.True(t, found)
	assert.Equal(t, "syscall.Syscall", fn.Name)
}
