//go:build test && darwin

package machodylib

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseSegmentsAndSymtab tests the load command parsing function with crafted byte slices.
func TestParseSegmentsAndSymtab(t *testing.T) {
	tests := []struct {
		name            string
		lcData          []byte
		wantTextSeg     bool
		wantLinkeditSeg bool
		wantSymtab      bool
	}{
		{
			name:            "empty",
			lcData:          []byte{},
			wantTextSeg:     false,
			wantLinkeditSeg: false,
			wantSymtab:      false,
		},
		{
			name: "minimal LC_SEGMENT_64 __TEXT",
			lcData: func() []byte {
				var buf bytes.Buffer
				// cmd=0x19 (LC_SEGMENT_64), cmdsize=72
				binary.Write(&buf, binary.LittleEndian, uint32(lcSegment64))
				binary.Write(&buf, binary.LittleEndian, uint32(72))
				// segname = "__TEXT\x00..."
				buf.WriteString("__TEXT")
				buf.Write(make([]byte, 10)) // pad to 16 bytes
				// vmaddr, vmsize, fileoff, filesize, maxprot, initprot, nsects, flags
				binary.Write(&buf, binary.LittleEndian, uint64(0x100000000))
				binary.Write(&buf, binary.LittleEndian, uint64(0x10000))
				binary.Write(&buf, binary.LittleEndian, uint64(0))
				binary.Write(&buf, binary.LittleEndian, uint64(0x10000))
				binary.Write(&buf, binary.LittleEndian, uint32(7))
				binary.Write(&buf, binary.LittleEndian, uint32(5))
				binary.Write(&buf, binary.LittleEndian, uint32(0))
				binary.Write(&buf, binary.LittleEndian, uint32(0))
				return buf.Bytes()
			}(),
			wantTextSeg:     true,
			wantLinkeditSeg: false,
			wantSymtab:      false,
		},
		{
			name: "LC_SEGMENT_64 __LINKEDIT",
			lcData: func() []byte {
				var buf bytes.Buffer
				binary.Write(&buf, binary.LittleEndian, uint32(lcSegment64))
				binary.Write(&buf, binary.LittleEndian, uint32(72))
				// segname = "__LINKEDIT\x00..."
				buf.WriteString("__LINKEDIT")
				buf.Write(make([]byte, 6)) // pad to 16 bytes
				binary.Write(&buf, binary.LittleEndian, uint64(0x10010000))
				binary.Write(&buf, binary.LittleEndian, uint64(0x1000))
				binary.Write(&buf, binary.LittleEndian, uint64(0x10000))
				binary.Write(&buf, binary.LittleEndian, uint64(0x1000))
				binary.Write(&buf, binary.LittleEndian, uint32(1))
				binary.Write(&buf, binary.LittleEndian, uint32(1))
				binary.Write(&buf, binary.LittleEndian, uint32(0))
				binary.Write(&buf, binary.LittleEndian, uint32(0))
				return buf.Bytes()
			}(),
			wantTextSeg:     false,
			wantLinkeditSeg: true,
			wantSymtab:      false,
		},
		{
			name: "LC_SYMTAB",
			lcData: func() []byte {
				var buf bytes.Buffer
				binary.Write(&buf, binary.LittleEndian, uint32(lcSymtab))
				binary.Write(&buf, binary.LittleEndian, uint32(24))
				binary.Write(&buf, binary.LittleEndian, uint32(0x1000)) // symoff
				binary.Write(&buf, binary.LittleEndian, uint32(100))    // nsyms
				binary.Write(&buf, binary.LittleEndian, uint32(0x2000)) // stroff
				binary.Write(&buf, binary.LittleEndian, uint32(0x500))  // strsize
				return buf.Bytes()
			}(),
			wantTextSeg:     false,
			wantLinkeditSeg: false,
			wantSymtab:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			textSeg, linkeditSeg, symtab := parseSegmentsAndSymtab(tt.lcData, binary.LittleEndian)
			assert.Equal(t, tt.wantTextSeg, textSeg != nil, "text segment presence mismatch")
			assert.Equal(t, tt.wantLinkeditSeg, linkeditSeg != nil, "linkedit segment presence mismatch")
			assert.Equal(t, tt.wantSymtab, symtab != nil, "symtab presence mismatch")
		})
	}
}

// TestPatchLoadCommands verifies that __TEXT fileoff and __LINKEDIT are correctly patched.
func TestPatchLoadCommands(t *testing.T) {
	tests := []struct {
		name                 string
		lcData               []byte
		newTextFileOff       uint64
		oldTextFileOff       uint64
		newLinkeditFileOff   uint64
		newLinkeditFileSize  uint64
		newSymoff            uint32
		newNsyms             uint32
		newStroff            uint32
		newStrsize           uint32
		checkTextFileOff     bool
		checkLinkeditFileOff bool
		checkSymoff          bool
	}{
		{
			name: "patch __TEXT fileoff in LC_SEGMENT_64",
			lcData: func() []byte {
				var buf bytes.Buffer
				binary.Write(&buf, binary.LittleEndian, uint32(lcSegment64))
				binary.Write(&buf, binary.LittleEndian, uint32(72))
				// segname
				buf.WriteString("__TEXT")
				buf.Write(make([]byte, 10))
				// vmaddr, vmsize, fileoff (offset 40 in LC)
				binary.Write(&buf, binary.LittleEndian, uint64(0x100000000))
				binary.Write(&buf, binary.LittleEndian, uint64(0x10000))
				binary.Write(&buf, binary.LittleEndian, uint64(0)) // old fileoff
				// filesize, maxprot, initprot, nsects, flags
				binary.Write(&buf, binary.LittleEndian, uint64(0x10000))
				binary.Write(&buf, binary.LittleEndian, uint32(7))
				binary.Write(&buf, binary.LittleEndian, uint32(5))
				binary.Write(&buf, binary.LittleEndian, uint32(0))
				binary.Write(&buf, binary.LittleEndian, uint32(0))
				return buf.Bytes()
			}(),
			newTextFileOff:      0x1000,
			oldTextFileOff:      0,
			newLinkeditFileOff:  0x11000,
			newLinkeditFileSize: 0x1000,
			checkTextFileOff:    true,
		},
		{
			name: "patch __LINKEDIT in LC_SEGMENT_64",
			lcData: func() []byte {
				var buf bytes.Buffer
				binary.Write(&buf, binary.LittleEndian, uint32(lcSegment64))
				binary.Write(&buf, binary.LittleEndian, uint32(72))
				// segname
				buf.WriteString("__LINKEDIT")
				buf.Write(make([]byte, 6))
				// vmaddr, vmsize, fileoff
				binary.Write(&buf, binary.LittleEndian, uint64(0x10010000))
				binary.Write(&buf, binary.LittleEndian, uint64(0x1000))
				binary.Write(&buf, binary.LittleEndian, uint64(0x10000))
				// filesize, maxprot, initprot, nsects, flags
				binary.Write(&buf, binary.LittleEndian, uint64(0x1000))
				binary.Write(&buf, binary.LittleEndian, uint32(1))
				binary.Write(&buf, binary.LittleEndian, uint32(1))
				binary.Write(&buf, binary.LittleEndian, uint32(0))
				binary.Write(&buf, binary.LittleEndian, uint32(0))
				return buf.Bytes()
			}(),
			newLinkeditFileOff:   0x20000,
			newLinkeditFileSize:  0x2000,
			checkLinkeditFileOff: true,
		},
		{
			name: "patch LC_SYMTAB fields",
			lcData: func() []byte {
				var buf bytes.Buffer
				binary.Write(&buf, binary.LittleEndian, uint32(lcSymtab))
				binary.Write(&buf, binary.LittleEndian, uint32(24))
				binary.Write(&buf, binary.LittleEndian, uint32(0x1000)) // symoff
				binary.Write(&buf, binary.LittleEndian, uint32(100))    // nsyms
				binary.Write(&buf, binary.LittleEndian, uint32(0x2000)) // stroff
				binary.Write(&buf, binary.LittleEndian, uint32(0x500))  // strsize
				return buf.Bytes()
			}(),
			newSymoff:   0x5000,
			newNsyms:    200,
			newStroff:   0x6000,
			newStrsize:  0x800,
			checkSymoff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lcCopy := make([]byte, len(tt.lcData))
			copy(lcCopy, tt.lcData)

			patchLoadCommands(
				lcCopy,
				tt.newTextFileOff, tt.oldTextFileOff,
				tt.newLinkeditFileOff, tt.newLinkeditFileSize,
				tt.newSymoff, tt.newNsyms, tt.newStroff, tt.newStrsize,
			)

			if tt.checkTextFileOff {
				// Check that fileoff at offset 40 in the LC_SEGMENT_64 was patched
				fileoff := binary.LittleEndian.Uint64(lcCopy[40:48])
				assert.Equal(t, tt.newTextFileOff, fileoff, "text fileoff not patched correctly")
			}

			if tt.checkLinkeditFileOff {
				// Check that fileoff at offset 40 in the LC_SEGMENT_64 was patched
				fileoff := binary.LittleEndian.Uint64(lcCopy[40:48])
				assert.Equal(t, tt.newLinkeditFileOff, fileoff, "linkedit fileoff not patched correctly")
			}

			if tt.checkSymoff {
				// Check that symoff at offset 8 in LC_SYMTAB was patched
				symoff := binary.LittleEndian.Uint32(lcCopy[8:12])
				assert.Equal(t, tt.newSymoff, symoff, "symoff not patched correctly")
			}
		})
	}
}

// TestBuildCompactSymtab_EdgeCases tests error handling and edge cases.
func TestBuildCompactSymtab_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		nsyms   uint32
		wantErr bool
	}{
		{
			name:    "zero symbols",
			nsyms:   0,
			wantErr: false, // should return minimal valid result
		},
		{
			name:    "reasonable symbol count",
			nsyms:   100,
			wantErr: false, // would fail only with I/O error on missing file
		},
		{
			name:    "exceeds limit",
			nsyms:   1<<18 + 1, // 262145, exceeds 1<<17
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a non-existent path to trigger I/O error for non-zero symbols.
			linkeditPath := "/nonexistent/dyld_cache_linkedit"
			compactSyms, compactStrtab, err := buildCompactSymtab(
				linkeditPath,
				0, // symFileOff
				tt.nsyms,
				0, // strFileOff
			)

			switch {
			case tt.wantErr:
				assert.Error(t, err, "expected error for nsyms=%d", tt.nsyms)
				assert.Nil(t, compactSyms)
				assert.Nil(t, compactStrtab)
			case tt.nsyms == 0:
				// Zero symbols should return empty but valid result.
				assert.NoError(t, err)
				assert.NotNil(t, compactSyms)
				assert.NotNil(t, compactStrtab)
			default:
				// Non-zero symbols with non-existent file will fail on I/O, which is expected.
				assert.Error(t, err)
			}
		})
	}
}

// TestReconstructMachO_BasicStructure verifies that reconstructMachO produces valid Mach-O header.
func TestReconstructMachO_BasicStructure(t *testing.T) {
	hdr := &machoHeader{
		magic:      0xFEEDFACF,
		cputype:    0x0100000C,
		cpusubtype: 0x00000000,
		filetype:   6, // MH_DYLIB
		ncmds:      2,
		sizeofcmds: 144,
		flags:      0x00210085,
		reserved:   0,
		byteOrder:  binary.LittleEndian,
	}

	lcData := make([]byte, 144)
	textSeg := &segInfo{
		name:     "__TEXT",
		vmaddr:   0x100000000,
		vmsize:   0x10000,
		fileOff:  0,
		fileSize: 0x10000,
	}
	textData := make([]byte, 100)
	compactSyms := make([]byte, 16)
	compactStrtab := []byte{0}

	machO := reconstructMachO(hdr, lcData, textSeg, textData, compactSyms, compactStrtab)

	require.NotNil(t, machO)
	assert.Greater(t, len(machO), machoHeaderSize, "reconstructed Mach-O too small")

	// Verify magic is preserved.
	magic := binary.LittleEndian.Uint32(machO[0:4])
	assert.Equal(t, uint32(0xFEEDFACF), magic, "magic not preserved")
}
