//go:build test

package filevalidator

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
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

// buildX86_64MachOBinary builds a minimal x86_64 Mach-O binary in memory.
// textBytes is placed in __TEXT,__text at virtual address 0x100000000.
func buildX86_64MachOBinary(t *testing.T, textBytes []byte) []byte {
	t.Helper()

	const (
		headerSize   = 32
		segCmdSize   = 72
		sectSize     = 80
		textOffset   = headerSize + segCmdSize + sectSize
		lcSegment64  = uint32(0x19)
		mhExecute    = uint32(0x2)
		cpuX86_64    = uint32(0x01000007)
		cpuSubX86All = uint32(3)
		vmBase       = uint64(0x100000000)
	)

	sectDataSize := uint32(len(textBytes))

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
	pu32(0xFEEDFACF)
	pu32(cpuX86_64)
	pu32(cpuSubX86All)
	pu32(mhExecute)
	pu32(1)
	pu32(uint32(segCmdSize + sectSize))
	pu32(0)
	pu32(0)

	// segment_command_64 (72 bytes)
	pu32(lcSegment64)
	pu32(uint32(segCmdSize + sectSize))
	pad16("__TEXT")
	pu64(vmBase)
	pu64(0x1000)
	pu64(uint64(textOffset))
	pu64(uint64(sectDataSize))
	pu32(7)
	pu32(5)
	pu32(1)
	pu32(0)

	// section_64 (80 bytes)
	pad16("__text")
	pad16("__TEXT")
	pu64(vmBase)
	pu64(uint64(sectDataSize))
	pu32(uint32(textOffset))
	pu32(2)
	pu32(0)
	pu32(0)
	pu32(0)
	pu32(0)
	pu32(0)
	pu32(0)

	buf.Write(textBytes)

	require.Equal(t, textOffset+int(sectDataSize), buf.Len())
	return buf.Bytes()
}

// buildFatMachOBinary combines multiple Mach-O arch binaries into a Fat/universal
// binary. cputype and cpusubtype are read directly from each arch's header bytes.
func buildFatMachOBinary(t *testing.T, arches [][]byte) []byte {
	t.Helper()
	require.NotEmpty(t, arches)

	nArch := len(arches)
	headerSize := 8 + nArch*20

	// Calculate 4-byte-aligned offsets for each arch slice.
	offsets := make([]int, nArch)
	offset := headerSize
	for i, arch := range arches {
		offset = (offset + 3) &^ 3
		offsets[i] = offset
		offset += len(arch)
	}

	buf := make([]byte, offset)

	// Fat header is big-endian.
	binary.BigEndian.PutUint32(buf[0:], 0xCAFEBABE)
	binary.BigEndian.PutUint32(buf[4:], uint32(nArch))

	pos := 8
	for i, arch := range arches {
		require.GreaterOrEqual(t, len(arch), 12, "arch %d too short", i)
		// cputype/cpusubtype are at bytes [4:12] of the Mach-O header (little-endian).
		cputype := binary.LittleEndian.Uint32(arch[4:8])
		cpusubtype := binary.LittleEndian.Uint32(arch[8:12])

		binary.BigEndian.PutUint32(buf[pos:], cputype)
		binary.BigEndian.PutUint32(buf[pos+4:], cpusubtype)
		binary.BigEndian.PutUint32(buf[pos+8:], uint32(offsets[i]))
		binary.BigEndian.PutUint32(buf[pos+12:], uint32(len(arch)))
		binary.BigEndian.PutUint32(buf[pos+16:], 2) // align: 4-byte
		pos += 20

		copy(buf[offsets[i]:], arch)
	}

	return buf
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
	v.SetIncludeDebugInfo(true)

	_, _, recErr := v.SaveRecord(binPath, false)
	require.NoError(t, recErr)

	record, loadErr := v.LoadRecord(binPath)
	require.NoError(t, loadErr)
	return record
}

// TestNativeMachoCPU verifies that nativeMachoCPU returns a non-error for the
// current build architecture, confirming the switch covers it.
func TestNativeMachoCPU(t *testing.T) {
	cpu, err := nativeMachoCPU()
	require.NoError(t, err, "current GOARCH must be a recognised macOS architecture")
	assert.NotZero(t, cpu)
}

// TestNativeOrArm64Slice verifies slice selection logic without depending on
// runtime.GOARCH: the fat binary has arm64 and x86_64 slices; selecting by
// CpuArm64 returns the arm64 slice, by CpuAmd64 returns the x86_64 slice,
// and by an unknown CPU falls back to arm64.
func TestNativeOrArm64Slice(t *testing.T) {
	// Build a minimal fat binary (arm64 + x86_64) in a temp file.
	arm64Bytes := buildArm64MachOBinary(t, []uint32{nopEncodingU32})
	x86Bytes := buildX86_64MachOBinary(t, []byte{0x90}) // NOP

	fatBytes := buildFatMachOBinary(t, [][]byte{arm64Bytes, x86Bytes})
	tmpFile := writeTempBinary(t, t.TempDir(), "fat.bin", fatBytes)

	f, err := os.Open(tmpFile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	fat, err := macho.NewFatFile(f)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fat.Close() })

	t.Run("arm64_selected_for_CpuArm64", func(t *testing.T) {
		slice := nativeOrArm64Slice(fat, macho.CpuArm64)
		require.NotNil(t, slice)
		assert.Equal(t, macho.CpuArm64, slice.Cpu)
	})

	t.Run("x86_64_selected_for_CpuAmd64", func(t *testing.T) {
		slice := nativeOrArm64Slice(fat, macho.CpuAmd64)
		require.NotNil(t, slice)
		assert.Equal(t, macho.CpuAmd64, slice.Cpu)
	})

	t.Run("fallback_to_arm64_for_unknown_cpu", func(t *testing.T) {
		// Use a CPU type absent from the fat binary; arm64 must be returned.
		slice := nativeOrArm64Slice(fat, macho.Cpu(0xFFFF))
		require.NotNil(t, slice)
		assert.Equal(t, macho.CpuArm64, slice.Cpu)
	})

	t.Run("nil_when_no_matching_or_arm64_slice", func(t *testing.T) {
		// Build a fat binary with only x86_64 and no arm64 slice.
		fatX86Only := buildFatMachOBinary(t, [][]byte{x86Bytes})
		tmpX86 := writeTempBinary(t, t.TempDir(), "fat_x86_only.bin", fatX86Only)
		fx, err := os.Open(tmpX86)
		require.NoError(t, err)
		t.Cleanup(func() { _ = fx.Close() })
		fatX86, err := macho.NewFatFile(fx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = fatX86.Close() })

		slice := nativeOrArm64Slice(fatX86, macho.Cpu(0xFFFF))
		assert.Nil(t, slice)
	})
}

// TestBuildMachoSyscallAnalysisData_SVCOnly is a unit test for the buildMachoSyscallData helper
// when only svc entries are present. It verifies that the returned SyscallAnalysisData has the
// correct fields for the svc-only case.
func TestBuildMachoSyscallAnalysisData_SVCOnly(t *testing.T) {
	addrs := []uint64{0x100000004, 0x10000000C}
	svcEntries := buildTestSVCInfos(addrs)
	result := buildMachoSyscallData(svcEntries, nil, "arm64", true)

	require.NotNil(t, result)
	assert.Equal(t, "arm64", result.Architecture)
	require.Len(t, result.AnalysisWarnings, 1)
	assert.Contains(t, result.AnalysisWarnings[0], "svc #0x80")
	require.Len(t, result.DetectedSyscalls, 1)

	sc := result.DetectedSyscalls[0]
	assert.Equal(t, -1, sc.Number)
	require.Len(t, sc.Occurrences, 2)
	// Occurrences sorted by Location ascending.
	assert.Equal(t, addrs[0], sc.Occurrences[0].Location)
	assert.Equal(t, "direct_svc_0x80", sc.Occurrences[0].DeterminationMethod)
	assert.Equal(t, "direct_svc_0x80", sc.Occurrences[0].Source)
	assert.Equal(t, addrs[1], sc.Occurrences[1].Location)
	assert.Equal(t, "direct_svc_0x80", sc.Occurrences[1].DeterminationMethod)
	assert.Equal(t, "direct_svc_0x80", sc.Occurrences[1].Source)
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
	assert.Equal(t, "direct_svc_0x80", record.SyscallAnalysis.DetectedSyscalls[0].Occurrences[0].DeterminationMethod)
	assert.Equal(t, uint64(0x100000000), record.SyscallAnalysis.DetectedSyscalls[0].Occurrences[0].Location)
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

// TestUpdateAnalysisRecord_MachoSVCDetected_BinaryAnalyzerNil verifies that the
// Mach-O svc scan does not depend on SymbolAnalysis being enabled.
func TestUpdateAnalysisRecord_MachoSVCDetected_BinaryAnalyzerNil(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	binPath := writeTempBinary(t, tempDir, "target.bin", buildArm64MachOBinary(t, []uint32{svcEncodingU32}))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	v.SetIncludeDebugInfo(true)

	_, _, recErr := v.SaveRecord(binPath, false)
	require.NoError(t, recErr)

	record, loadErr := v.LoadRecord(binPath)
	require.NoError(t, loadErr)
	require.NotNil(t, record.SyscallAnalysis, "SyscallAnalysis must be set when svc #0x80 is found")
	require.Len(t, record.SyscallAnalysis.DetectedSyscalls, 1)
	assert.Equal(t, "direct_svc_0x80", record.SyscallAnalysis.DetectedSyscalls[0].Occurrences[0].DeterminationMethod)
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

	// Plain text file: neither ELF nor Mach-O. analyzeELFSyscalls sets nil.
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

// TestBuildSVCSyscallEntries_CommonSyscallInfo verifies that buildSVCInfos produces
// common.SyscallInfo entries with the expected field values from the spec.
func TestBuildSVCSyscallEntries_CommonSyscallInfo(t *testing.T) {
	addrs := []uint64{0x100000000}
	entries := buildTestSVCInfos(addrs)

	require.Len(t, entries, 1)

	sc := entries[0]
	// Verify the type is common.SyscallInfo (zero-value assignment as type check).
	_ = common.SyscallInfo{}
	assert.Equal(t, -1, sc.Number, "Number must be -1 (undetermined)")
	require.Len(t, sc.Occurrences, 1)
	assert.Equal(t, "direct_svc_0x80", sc.Occurrences[0].DeterminationMethod)
	assert.Equal(t, "direct_svc_0x80", sc.Occurrences[0].Source)
	assert.Empty(t, sc.Name)
}

// ---- stubLibSystemCache for Section 5.2 tests ----

// stubLibSystemCache is a test double for LibSystemCacheInterface.
type stubLibSystemCache struct {
	// infos is returned by GetSyscallInfos when err is nil.
	infos []common.SyscallInfo
	// err is returned by GetSyscallInfos when non-nil.
	err error
}

func buildTestSVCInfos(addrs []uint64) []common.SyscallInfo {
	if len(addrs) == 0 {
		return nil
	}
	infos := make([]common.SyscallInfo, len(addrs))
	for i, addr := range addrs {
		infos[i] = common.SyscallInfo{
			Number: -1,
			Occurrences: []common.SyscallOccurrence{{
				Location:            addr,
				DeterminationMethod: common.DeterminationMethodDirectSVC0x80,
				Source:              common.DeterminationMethodDirectSVC0x80,
			}},
		}
	}
	return infos
}

func (s *stubLibSystemCache) GetSyscallInfos(
	_ []fileanalysis.LibEntry,
	_ []string,
	_ bool,
) ([]common.SyscallInfo, error) {
	return s.infos, s.err
}

// recordMachOWithLibSystem records a synthetic Mach-O and injects the libSystem cache stub.
func recordMachOWithLibSystem(
	t *testing.T,
	binData []byte,
	stub *stubBinaryAnalyzer,
	libsys LibSystemCacheInterface,
) (*fileanalysis.Record, error) {
	t.Helper()
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	binPath := writeTempBinary(t, tempDir, "target.bin", binData)

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	if stub != nil {
		v.SetBinaryAnalyzer(stub)
	}
	v.SetIncludeDebugInfo(true)
	if libsys != nil {
		v.SetLibSystemCache(libsys)
	}

	_, _, recErr := v.SaveRecord(binPath, false)
	if recErr != nil {
		return nil, recErr
	}

	record, loadErr := v.LoadRecord(binPath)
	require.NoError(t, loadErr)
	return record, nil
}

// TestUpdateAnalysisRecord_LibSystemImportOnly verifies that when libSystem
// returns entries and no svc is found, SyscallAnalysis is populated with
// Source=libsystem_symbol_import and Location=0.
func TestUpdateAnalysisRecord_LibSystemImportOnly(t *testing.T) {
	libsysEntries := []common.SyscallInfo{
		{
			Number: 97,
			Name:   "socket",
			Occurrences: []common.SyscallOccurrence{{
				Location:            0,
				DeterminationMethod: "lib_cache_match",
				Source:              "libsystem_symbol_import",
			}},
		},
	}
	stub := &stubLibSystemCache{infos: libsysEntries}

	v, err := New(&SHA256{}, safeTempDir(t))
	require.NoError(t, err)
	v.SetLibSystemCache(stub)

	// Build a record with DynLibDeps so analyzeLibSystem is not skipped.
	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "/usr/lib/libSystem.B.dylib"},
		},
	}

	// Write a minimal arm64 Mach-O as the analysis target (no svc).
	tempDir := safeTempDir(t)
	binPath := writeTempBinary(t, tempDir, "target.bin", buildArm64MachOBinary(t, []uint32{nopEncodingU32}))

	libsys, _, err := v.analyzeLibSystem(record, binPath)
	require.NoError(t, err)
	require.Len(t, libsys, 1)

	sc := libsys[0]
	assert.Equal(t, 97, sc.Number)
	assert.Equal(t, "socket", sc.Name)
	assert.Equal(t, uint64(0), sc.Occurrences[0].Location, "libSystem entries must have Location=0")
	assert.Equal(t, "libsystem_symbol_import", sc.Occurrences[0].Source)
}

// TestUpdateAnalysisRecord_SVCAndLibSystemMerged verifies that buildMachoSyscallData
// merges svc and libSystem entries correctly, sorted by Number.
func TestUpdateAnalysisRecord_SVCAndLibSystemMerged(t *testing.T) {
	svcEntries := buildTestSVCInfos([]uint64{0x100000000})
	libsysEntries := []common.SyscallInfo{
		{
			Number: 98,
			Name:   "connect",
			Occurrences: []common.SyscallOccurrence{{
				Location:            0,
				DeterminationMethod: "lib_cache_match",
				Source:              "libsystem_symbol_import",
			}},
		},
	}

	result := buildMachoSyscallData(svcEntries, libsysEntries, "arm64", true)
	require.NotNil(t, result)

	// svc entry (Number=-1) + libSystem entry (Number=98) = 2 entries.
	require.Len(t, result.DetectedSyscalls, 2)

	// libSystem entry sorts first (Number=98); svc entry (Number=-1) sorts last.
	first := result.DetectedSyscalls[0]
	assert.Equal(t, 98, first.Number)
	assert.Equal(t, "libsystem_symbol_import", first.Occurrences[0].Source)

	second := result.DetectedSyscalls[1]
	assert.Equal(t, -1, second.Number)
	assert.Equal(t, "direct_svc_0x80", second.Occurrences[0].Source)

	// Warning must be set because svc was found.
	require.Len(t, result.AnalysisWarnings, 1)
	assert.Contains(t, result.AnalysisWarnings[0], "svc #0x80")
}

func TestBuildMachoSyscallData_DebugInfo(t *testing.T) {
	svcEntries := []common.SyscallInfo{
		{
			Number: -1,
			Occurrences: []common.SyscallOccurrence{{
				Location:            0x100000000,
				DeterminationMethod: common.DeterminationMethodDirectSVC0x80,
				Source:              common.DeterminationMethodDirectSVC0x80,
			}},
		},
	}

	withoutDebug := buildMachoSyscallData(svcEntries, nil, "arm64", false)
	require.NotNil(t, withoutDebug)
	require.Len(t, withoutDebug.DetectedSyscalls, 1)
	assert.Nil(t, withoutDebug.DetectedSyscalls[0].Occurrences)
	require.Len(t, withoutDebug.AnalysisWarnings, 1)
	assert.Contains(t, withoutDebug.AnalysisWarnings[0], "svc #0x80")

	withDebug := buildMachoSyscallData(svcEntries, nil, "arm64", true)
	require.NotNil(t, withDebug)
	require.Len(t, withDebug.DetectedSyscalls, 1)
	require.Len(t, withDebug.DetectedSyscalls[0].Occurrences, 1)
	assert.Equal(t, uint64(0x100000000), withDebug.DetectedSyscalls[0].Occurrences[0].Location)
}

// TestUpdateAnalysisRecord_LibSystemError verifies that when the libSystem cache
// returns an error and DynLibDeps is populated, analyzeLibSystem propagates it.
func TestUpdateAnalysisRecord_LibSystemError(t *testing.T) {
	stubErr := errors.New("libSystem cache read failure")
	stub := &stubLibSystemCache{err: stubErr}

	// Build a minimal arm64 Mach-O so machoImportSymbols succeeds (returns empty list).
	// analyzeLibSystem skips when DynLibDeps is empty, so inject a mock record directly.
	v, err := New(&SHA256{}, safeTempDir(t))
	require.NoError(t, err)
	v.SetLibSystemCache(stub)

	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "/usr/lib/libSystem.B.dylib"},
		},
	}

	// Write a minimal Mach-O so machoImportSymbols has a file to open.
	tempDir := safeTempDir(t)
	binPath := writeTempBinary(t, tempDir, "target.bin", buildArm64MachOBinary(t, []uint32{nopEncodingU32}))

	_, _, libsysErr := v.analyzeLibSystem(record, binPath)
	// The Mach-O has no imports (no symbol table), so GetSyscallInfos is called with empty list.
	// The stub returns the injected error.
	require.Error(t, libsysErr, "analyzeLibSystem must propagate the cache error")
}

// TestUpdateAnalysisRecord_LibSystemNilCache verifies that when no libSystem cache
// is injected, analyzeLibSystem returns nil and SyscallAnalysis is nil
// (assuming no svc is found either).
func TestUpdateAnalysisRecord_LibSystemNilCache(t *testing.T) {
	binData := buildArm64MachOBinary(t, []uint32{nopEncodingU32})

	record, err := recordMachOWithLibSystem(t, binData, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, record.SyscallAnalysis, "SyscallAnalysis must be nil when no cache and no svc")
}

// TestMergeMachoSyscallInfos_BothNil verifies that merging two nil slices returns nil.
func TestMergeMachoSyscallInfos_BothNil(t *testing.T) {
	result := mergeMachoSyscallInfos(nil, nil)
	assert.Nil(t, result)
}

// TestMergeMachoSyscallInfos_SVCOnly verifies that svc-only merge returns svc entries.
func TestMergeMachoSyscallInfos_SVCOnly(t *testing.T) {
	svcEntries := []common.SyscallInfo{
		{
			Number:      -1,
			Occurrences: []common.SyscallOccurrence{{Location: 0x100000000}},
		},
	}
	result := mergeMachoSyscallInfos(svcEntries, nil)
	require.Len(t, result, 1)
	assert.Equal(t, -1, result[0].Number)
}

// TestMergeMachoSyscallInfos_LibSysOnly verifies that libSystem-only merge returns entries.
func TestMergeMachoSyscallInfos_LibSysOnly(t *testing.T) {
	libsysEntries := []common.SyscallInfo{
		{Number: 97, Occurrences: []common.SyscallOccurrence{{Source: "libsystem_symbol_import"}}},
	}
	result := mergeMachoSyscallInfos(nil, libsysEntries)
	require.Len(t, result, 1)
	assert.Equal(t, 97, result[0].Number)
}

// TestMergeMachoSyscallInfos_SameNumberSortsByLocationThenSource verifies that
// entries sharing the same Number are ordered by (Location, Source) so that
// JSON output is deterministic across runs.
//
// The primary case is Number=-1 (unresolved svc #0x80), where multiple svc
// instructions can appear at different addresses in the same binary.
func TestMergeMachoSyscallInfos_SameNumberSortsByLocationThenSource(t *testing.T) {
	svcEntries := []common.SyscallInfo{
		{Number: -1, Occurrences: []common.SyscallOccurrence{{Location: 0x100000020, Source: "a_source"}}},
		{Number: -1, Occurrences: []common.SyscallOccurrence{{Location: 0x100000010, Source: "a_source"}}},
		{Number: -1, Occurrences: []common.SyscallOccurrence{{Location: 0x100000010, Source: "b_source"}}},
	}
	result := mergeMachoSyscallInfos(svcEntries, nil)
	// All three entries have the same Number, so they are merged into one group.
	require.Len(t, result, 1)
	require.Len(t, result[0].Occurrences, 3)
	// Occurrences sorted by Location ascending; stable sort preserves input order for equal Location.
	assert.Equal(t, uint64(0x100000010), result[0].Occurrences[0].Location)
	assert.Equal(t, uint64(0x100000010), result[0].Occurrences[1].Location)
	assert.Equal(t, uint64(0x100000020), result[0].Occurrences[2].Location)
	assert.Equal(t, "a_source", result[0].Occurrences[0].Source)
	assert.Equal(t, "b_source", result[0].Occurrences[1].Source)
}

// TestMergeMachoSyscallInfos_MixedNumbersSortedFirst verifies that entries with
// different Number values are sorted by Number before Location or Source are
// considered.
func TestMergeMachoSyscallInfos_MixedNumbersSortedFirst(t *testing.T) {
	svcEntries := []common.SyscallInfo{
		{Number: -1, Occurrences: []common.SyscallOccurrence{{Location: 0x100000000}}},
		{Number: 97, Occurrences: []common.SyscallOccurrence{{Location: 0x100000020}}},
		{Number: 98, Occurrences: []common.SyscallOccurrence{{Location: 0x100000010}}},
	}
	result := mergeMachoSyscallInfos(svcEntries, nil)
	require.Len(t, result, 3)
	assert.Equal(t, 97, result[0].Number)
	assert.Equal(t, 98, result[1].Number)
	assert.Equal(t, -1, result[2].Number, "Number=-1 sorts last")
}

// TestBuildMachoSyscallAnalysisData_WarningOnlyWhenSVC verifies that
// AnalysisWarnings is populated only when there are unresolved svc entries
// (Number=-1, DeterminationMethod="direct_svc_0x80"), and that resolved
// non-network svc entries produce no warning but are retained in DetectedSyscalls.
func TestBuildMachoSyscallAnalysisData_WarningOnlyWhenSVC(t *testing.T) {
	// libSystem entry that is non-network: must be retained (no filtering).
	libsysEntries := []common.SyscallInfo{
		{Number: 97, Occurrences: []common.SyscallOccurrence{{Source: "libsystem_symbol_import"}}},
	}

	// No svc entries: no warning, libsys entry is retained.
	result := buildMachoSyscallData(nil, libsysEntries, "arm64", true)
	assert.Empty(t, result.AnalysisWarnings, "no warning when no svc entries")
	require.Len(t, result.DetectedSyscalls, 1, "non-network libsys entry must be retained (no filtering)")
	assert.Equal(t, 97, result.DetectedSyscalls[0].Number)

	// Unresolved svc entry (Number=-1, DeterminationMethod=direct_svc_0x80): warning present.
	unresolvedSVCEntries := []common.SyscallInfo{
		{
			Number: -1,
			Occurrences: []common.SyscallOccurrence{{
				Source:              common.DeterminationMethodDirectSVC0x80,
				DeterminationMethod: common.DeterminationMethodDirectSVC0x80,
			}},
		},
	}
	result = buildMachoSyscallData(unresolvedSVCEntries, libsysEntries, "arm64", true)
	assert.Len(t, result.AnalysisWarnings, 1, "warning expected for unresolved svc entry")
	require.Len(t, result.DetectedSyscalls, 2, "both svc and libsys entries must be retained")

	// Resolved non-network svc entries (e.g., munmap=73): no warning, entries retained.
	resolvedNonNetworkSVCEntries := []common.SyscallInfo{
		{
			Number: 73, // munmap — non-network
			Occurrences: []common.SyscallOccurrence{{
				Source:              common.DeterminationMethodDirectSVC0x80,
				DeterminationMethod: common.DeterminationMethodDirectSVC0x80,
			}},
		},
	}
	result = buildMachoSyscallData(resolvedNonNetworkSVCEntries, libsysEntries, "arm64", true)
	assert.Empty(t, result.AnalysisWarnings, "no warning when all svc entries resolved (Number != -1)")
	require.Len(t, result.DetectedSyscalls, 2, "resolved non-network svc entries must be retained")
	numbers := []int{result.DetectedSyscalls[0].Number, result.DetectedSyscalls[1].Number}
	assert.Contains(t, numbers, 73)
	assert.Contains(t, numbers, 97)

	// Resolved network svc entry (socket=97): no warning, entry retained in DetectedSyscalls.
	resolvedNetworkSVCEntries := []common.SyscallInfo{
		{
			Number: 97, // socket — network
			Name:   "socket",
			Occurrences: []common.SyscallOccurrence{{
				Source:              common.DeterminationMethodDirectSVC0x80,
				DeterminationMethod: common.DeterminationMethodDirectSVC0x80,
			}},
		},
	}
	result = buildMachoSyscallData(resolvedNetworkSVCEntries, nil, "arm64", true)
	assert.Empty(t, result.AnalysisWarnings, "no warning for resolved network svc entry")
	require.Len(t, result.DetectedSyscalls, 1, "resolved network svc entry must be retained")
	assert.Equal(t, 97, result.DetectedSyscalls[0].Number)

	// Mixed: resolved network svc + unresolved svc: warning present, both retained.
	mixedEntries := []common.SyscallInfo{
		{Number: 97, Occurrences: []common.SyscallOccurrence{{DeterminationMethod: common.DeterminationMethodDirectSVC0x80, Source: common.DeterminationMethodDirectSVC0x80}}},
		{Number: -1, Occurrences: []common.SyscallOccurrence{{DeterminationMethod: common.DeterminationMethodDirectSVC0x80, Source: common.DeterminationMethodDirectSVC0x80}}},
	}
	result = buildMachoSyscallData(mixedEntries, nil, "arm64", true)
	assert.Len(t, result.AnalysisWarnings, 1, "warning expected when any unresolved svc entry exists")
	assert.Len(t, result.DetectedSyscalls, 2, "all entries must be retained")
}
