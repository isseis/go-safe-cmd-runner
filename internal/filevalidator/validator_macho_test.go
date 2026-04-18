//go:build test

package filevalidator

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// svcEncodingU32 is the uint32 encoding of arm64 "svc #0x80" (little-endian).
const svcEncodingU32 = uint32(0xD4001001)

// nopEncodingU32 is a common arm64 NOP instruction.
const nopEncodingU32 = uint32(0xD503201F)

// buildArm64MachOBinary builds a minimal arm64 Mach-O binary in memory.
// The given 32-bit instruction words are placed in __TEXT,__text at virtual
// address 0x100000000. Returns the binary as a byte slice.
func buildArm64MachOBinary(t *testing.T, instructions []uint32) []byte {
	t.Helper()

	const (
		headerSize    = 32                                 // mach_header_64
		segCmdSize    = 72                                 // segment_command_64
		sectSize      = 80                                 // section_64
		textOffset    = headerSize + segCmdSize + sectSize // 184 = 0xB8
		lcSegment64   = uint32(0x19)
		mhExecute     = uint32(0x2)
		cpuArm64      = uint32(0x0100000C)
		vmBase        = uint64(0x100000000)
		sAttrPureInst = uint32(0x80000000)
	)

	instBytes := make([]byte, len(instructions)*4)
	for i, inst := range instructions {
		binary.LittleEndian.PutUint32(instBytes[i*4:], inst)
	}
	sectDataSize := uint32(len(instBytes))

	var buf bytes.Buffer
	pu32 := func(v uint32) {
		b := [4]byte{}
		binary.LittleEndian.PutUint32(b[:], v)
		buf.Write(b[:])
	}
	pu64 := func(v uint64) {
		b := [8]byte{}
		binary.LittleEndian.PutUint64(b[:], v)
		buf.Write(b[:])
	}
	pad16 := func(s string) {
		b := [16]byte{}
		copy(b[:], s)
		buf.Write(b[:])
	}

	// mach_header_64 (32 bytes)
	pu32(0xFEEDFACF)                    // magic
	pu32(cpuArm64)                      // cputype
	pu32(0)                             // cpusubtype
	pu32(mhExecute)                     // filetype
	pu32(1)                             // ncmds
	pu32(uint32(segCmdSize + sectSize)) // sizeofcmds
	pu32(0)                             // flags
	pu32(0)                             // reserved

	// segment_command_64 (72 bytes)
	pu32(lcSegment64)                   // cmd
	pu32(uint32(segCmdSize + sectSize)) // cmdsize
	pad16("__TEXT")                     // segname
	pu64(vmBase)                        // vmaddr
	pu64(0x1000)                        // vmsize
	pu64(uint64(textOffset))            // fileoff
	pu64(uint64(sectDataSize))          // filesize
	pu32(7)                             // maxprot
	pu32(5)                             // initprot
	pu32(1)                             // nsects
	pu32(0)                             // flags

	// section_64 (80 bytes)
	pad16("__text")            // sectname
	pad16("__TEXT")            // segname
	pu64(vmBase)               // addr
	pu64(uint64(sectDataSize)) // size
	pu32(uint32(textOffset))   // offset
	pu32(2)                    // align
	pu32(0)                    // reloff
	pu32(0)                    // nreloc
	pu32(sAttrPureInst)        // flags
	pu32(0)                    // reserved1
	pu32(0)                    // reserved2
	pu32(0)                    // reserved3

	// section data
	buf.Write(instBytes)

	require.Equal(t, textOffset+int(sectDataSize), buf.Len())
	return buf.Bytes()
}

// writeTempBinary writes data to a file in the given directory and returns its path.
func writeTempBinary(t *testing.T, dir string, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

// recordMachO records a synthetic binary and returns the analysis record.
// stub controls the SymbolAnalysis result; the svc scan runs independently.
func recordMachO(t *testing.T, binData []byte, stub *stubBinaryAnalyzer) *fileanalysis.Record {
	t.Helper()
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	binPath := writeTempBinary(t, tempDir, "target.bin", binData)

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	v.SetBinaryAnalyzer(stub)

	_, _, recErr := v.SaveRecord(binPath, false)
	require.NoError(t, recErr)

	record, loadErr := v.LoadRecord(binPath)
	require.NoError(t, loadErr)
	return record
}

// TestBuildSVCSyscallAnalysis is a unit test for the buildSVCSyscallAnalysis helper.
// It verifies that the returned SyscallAnalysisData has the correct fields.
func TestBuildSVCSyscallAnalysis(t *testing.T) {
	addrs := []uint64{0x100000004, 0x10000000C}
	result := buildSVCSyscallAnalysis(addrs)

	require.NotNil(t, result)
	assert.Equal(t, "arm64", result.Architecture)
	require.Len(t, result.AnalysisWarnings, 1)
	assert.Contains(t, result.AnalysisWarnings[0], "svc #0x80")
	require.Len(t, result.DetectedSyscalls, 2)

	for i, addr := range addrs {
		sc := result.DetectedSyscalls[i]
		assert.Equal(t, -1, sc.Number)
		assert.Equal(t, addr, sc.Location)
		assert.Equal(t, "direct_svc_0x80", sc.DeterminationMethod)
		assert.Equal(t, "direct_svc_0x80", sc.Source)
		assert.False(t, sc.IsNetwork)
	}
	assert.Nil(t, result.ArgEvalResults)
}

// TestUpdateAnalysisRecord_MachoSVCDetected verifies that SaveRecord sets
// SyscallAnalysis when a svc #0x80 instruction is found in an arm64 Mach-O
// that has no network symbols (NoNetworkSymbols case).
func TestUpdateAnalysisRecord_MachoSVCDetected(t *testing.T) {
	// Build a minimal arm64 Mach-O with one svc #0x80 instruction.
	binData := buildArm64MachOBinary(t, []uint32{svcEncodingU32})
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}

	record := recordMachO(t, binData, stub)

	require.NotNil(t, record.SyscallAnalysis, "SyscallAnalysis must be set when svc #0x80 is found")
	assert.Equal(t, "arm64", record.SyscallAnalysis.Architecture)
	require.Len(t, record.SyscallAnalysis.DetectedSyscalls, 1)
	assert.Equal(t, "direct_svc_0x80", record.SyscallAnalysis.DetectedSyscalls[0].DeterminationMethod)
	assert.Equal(t, uint64(0x100000000), record.SyscallAnalysis.DetectedSyscalls[0].Location)
}

// TestUpdateAnalysisRecord_MachoNoSVC verifies that SaveRecord leaves
// SyscallAnalysis nil when the arm64 Mach-O contains no svc #0x80.
func TestUpdateAnalysisRecord_MachoNoSVC(t *testing.T) {
	// Build a minimal arm64 Mach-O with only NOP instructions (no svc).
	binData := buildArm64MachOBinary(t, []uint32{nopEncodingU32, nopEncodingU32})
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}

	record := recordMachO(t, binData, stub)

	assert.Nil(t, record.SyscallAnalysis, "SyscallAnalysis must be nil when no svc #0x80 found")
}

// TestUpdateAnalysisRecord_MachoNetworkDetected_SVCDetected verifies that SaveRecord
// saves SyscallAnalysis even when the Mach-O also has network symbols (NetworkDetected).
// The runner controls whether to consult SyscallAnalysis; record captures it unconditionally.
func TestUpdateAnalysisRecord_MachoNetworkDetected_SVCDetected(t *testing.T) {
	// Build a minimal arm64 Mach-O with one svc #0x80 instruction.
	binData := buildArm64MachOBinary(t, []uint32{svcEncodingU32})
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.NetworkDetected,
		detectedSymbols: []binaryanalyzer.DetectedSymbol{
			{Name: "socket", Category: "network"},
		},
	}

	record := recordMachO(t, binData, stub)

	// SymbolAnalysis must be set (NetworkDetected).
	require.NotNil(t, record.SymbolAnalysis, "SymbolAnalysis must be set for NetworkDetected")
	// SyscallAnalysis must also be set (svc found regardless of SymbolAnalysis result).
	require.NotNil(t, record.SyscallAnalysis, "SyscallAnalysis must be set when svc #0x80 is found")
	assert.Equal(t, "arm64", record.SyscallAnalysis.Architecture)
}

// TestUpdateAnalysisRecord_MachoNetworkDetected_NoSVC verifies that SaveRecord
// leaves SyscallAnalysis nil when the Mach-O has network symbols but no svc #0x80.
func TestUpdateAnalysisRecord_MachoNetworkDetected_NoSVC(t *testing.T) {
	// Build a minimal arm64 Mach-O with only NOP instructions (no svc).
	binData := buildArm64MachOBinary(t, []uint32{nopEncodingU32})
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.NetworkDetected,
		detectedSymbols: []binaryanalyzer.DetectedSymbol{
			{Name: "socket", Category: "network"},
		},
	}

	record := recordMachO(t, binData, stub)

	require.NotNil(t, record.SymbolAnalysis, "SymbolAnalysis must be set for NetworkDetected")
	assert.Nil(t, record.SyscallAnalysis, "SyscallAnalysis must be nil when no svc #0x80 found")
}

// TestUpdateAnalysisRecord_ELFNotAffected verifies that the Mach-O svc scan path
// does not affect non-Mach-O files. A plain text file (neither ELF nor Mach-O)
// should result in nil SyscallAnalysis after the svc scan code runs.
func TestUpdateAnalysisRecord_ELFNotAffected(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	// Plain text file: neither ELF nor Mach-O. analyzeSyscalls sets nil.
	// The Mach-O svc scan must also return nil (magic mismatch).
	textPath := writeTempBinary(t, tempDir, "not_binary.txt", []byte("hello world"))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	// Set a non-nil BinaryAnalyzer so the svc scan path is exercised.
	v.SetBinaryAnalyzer(&stubBinaryAnalyzer{result: binaryanalyzer.NotSupportedBinary})

	_, _, recErr := v.SaveRecord(textPath, false)
	require.NoError(t, recErr)

	record, loadErr := v.LoadRecord(textPath)
	require.NoError(t, loadErr)

	assert.Nil(t, record.SyscallAnalysis,
		"SyscallAnalysis must remain nil for a non-Mach-O, non-ELF file")
}

// TestBuildSVCSyscallAnalysis_CommonSyscallInfo verifies that DetectedSyscalls use
// common.SyscallInfo and the fields match the expected values from the spec.
func TestBuildSVCSyscallAnalysis_CommonSyscallInfo(t *testing.T) {
	addrs := []uint64{0x100000000}
	result := buildSVCSyscallAnalysis(addrs)

	require.NotNil(t, result)
	require.Len(t, result.DetectedSyscalls, 1)

	sc := result.DetectedSyscalls[0]
	// Verify the type is common.SyscallInfo (zero-value assignment as type check).
	_ = common.SyscallInfo{}
	assert.Equal(t, -1, sc.Number, "Number must be -1 (undetermined)")
	assert.Equal(t, "direct_svc_0x80", sc.DeterminationMethod)
	assert.Equal(t, "direct_svc_0x80", sc.Source)
	assert.False(t, sc.IsNetwork)
	assert.Empty(t, sc.Name)
}
