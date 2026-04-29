//go:build test && linux

package elfdynlib

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ELF binary test helpers -----------------------------------------------

// buildTestELFWithDeps writes a minimal ELF64 LE (x86-64, ET_DYN) file to dir
// with the given soname entries in DT_NEEDED and an optional RUNPATH.
// The file name is fileName. Returns the absolute path.
//
// Go's debug/elf.DynString requires SHT_DYNAMIC section header.
// This builder provides proper section headers alongside program headers.
//
// Layout:
//   - ELF header (64 B)
//   - 2 program headers: PT_LOAD and PT_DYNAMIC (56 B each)
//   - .dynamic section (numDynEntries x 16 B)
//   - .dynstr (dynamic string table)
//   - .shstrtab (section name string table)
//   - Section header table (4 x 64 B)
func buildTestELFWithDeps(t *testing.T, dir, fileName string, sonames []string, runpath string) string {
	t.Helper()

	// ---- constants --------------------------------------------------------
	const (
		elfHeaderSize = 64
		phdrSize      = 56 // ELF64 program header
		shdrSize      = 64 // ELF64 section header
		dynEntrySize  = 16 // size of a Dyn64 entry
		baseVaddr     = uint64(0x400000)

		// ELF header constants
		elfClass64 = 2
		elfDataLE  = 1
		elfVersion = 1
		etDyn      = 3  // ET_DYN
		emX8664    = 62 // EM_X86_64
		ptLoad     = 1
		ptDynamic  = 2
		pfRX       = 5 // PF_R|PF_X
		pfRW       = 6 // PF_R|PF_W

		// Dynamic tag constants
		dtNull    = 0
		dtNeeded  = 1
		dtStrtab  = 5
		dtStrsz   = 10
		dtRunpath = 29

		// Section type constants
		shtNull    = 0
		shtStrtab  = 3
		shtDynamic = 6

		// Section flags
		shfAlloc  = 2
		shfAllocW = shfAlloc | 1 // SHF_ALLOC|SHF_WRITE
	)

	numPhdrs := 2 // PT_LOAD + PT_DYNAMIC

	// Build .dynstr: index 0 must be "\x00".
	var dynstrBuf bytes.Buffer
	dynstrBuf.WriteByte(0) // offset 0 = ""

	runpathIdx := uint64(0)
	if runpath != "" {
		runpathIdx = uint64(dynstrBuf.Len())
		dynstrBuf.WriteString(runpath)
		dynstrBuf.WriteByte(0)
	}

	soIdx := make([]uint64, len(sonames))
	for i, so := range sonames {
		soIdx[i] = uint64(dynstrBuf.Len())
		dynstrBuf.WriteString(so)
		dynstrBuf.WriteByte(0)
	}
	dynstrData := dynstrBuf.Bytes()

	// Build .shstrtab (section name string table).
	var shstrtabBuf bytes.Buffer
	shstrtabBuf.WriteByte(0) // offset 0 = ""
	dynstrNameIdx := uint32(shstrtabBuf.Len())
	shstrtabBuf.WriteString(".dynstr\x00")
	dynamicNameIdx := uint32(shstrtabBuf.Len())
	shstrtabBuf.WriteString(".dynamic\x00")
	shstrtabNameIdx := uint32(shstrtabBuf.Len())
	shstrtabBuf.WriteString(".shstrtab\x00")
	shstrtabData := shstrtabBuf.Bytes()

	// Number of dynamic entries: DT_STRTAB + DT_STRSZ + DT_NULL + optional RUNPATH + sonames.
	numDynEntries := 3 // DT_STRTAB, DT_STRSZ, DT_NULL
	if runpath != "" {
		numDynEntries++
	}
	numDynEntries += len(sonames)

	// File layout offsets.
	dynSectionOffset := uint64(elfHeaderSize + phdrSize*numPhdrs)
	dynSectionSize := uint64(numDynEntries * dynEntrySize)
	dynstrOffset := dynSectionOffset + dynSectionSize
	shstrtabOffset := dynstrOffset + uint64(len(dynstrData))
	shOffset := shstrtabOffset + uint64(len(shstrtabData))
	totalSize := shOffset + uint64(shdrSize)*4

	dynVaddr := baseVaddr + dynSectionOffset
	dynstrVaddr := baseVaddr + dynstrOffset

	le := binary.LittleEndian
	var out bytes.Buffer

	// ---- ELF header (64 bytes) ----
	out.Write([]byte{
		0x7f, 'E', 'L', 'F', elfClass64, elfDataLE, elfVersion, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
	})
	_ = binary.Write(&out, le, uint16(etDyn))
	_ = binary.Write(&out, le, uint16(emX8664))
	_ = binary.Write(&out, le, uint32(elfVersion))
	_ = binary.Write(&out, le, uint64(0))             // e_entry
	_ = binary.Write(&out, le, uint64(elfHeaderSize)) // e_phoff
	_ = binary.Write(&out, le, shOffset)              // e_shoff
	_ = binary.Write(&out, le, uint32(0))             // e_flags
	_ = binary.Write(&out, le, uint16(elfHeaderSize)) // e_ehsize
	_ = binary.Write(&out, le, uint16(phdrSize))      // e_phentsize
	_ = binary.Write(&out, le, uint16(numPhdrs))      // e_phnum
	_ = binary.Write(&out, le, uint16(shdrSize))      // e_shentsize
	_ = binary.Write(&out, le, uint16(4))             // e_shnum
	_ = binary.Write(&out, le, uint16(3))             // e_shstrndx = .shstrtab

	// ---- Program header: PT_LOAD covering everything ----
	_ = binary.Write(&out, le, uint32(ptLoad))
	_ = binary.Write(&out, le, uint32(pfRX))
	_ = binary.Write(&out, le, uint64(0))      // p_offset
	_ = binary.Write(&out, le, baseVaddr)      // p_vaddr
	_ = binary.Write(&out, le, baseVaddr)      // p_paddr
	_ = binary.Write(&out, le, totalSize)      // p_filesz
	_ = binary.Write(&out, le, totalSize)      // p_memsz
	_ = binary.Write(&out, le, uint64(0x1000)) // p_align

	// ---- Program header: PT_DYNAMIC ----
	_ = binary.Write(&out, le, uint32(ptDynamic))
	_ = binary.Write(&out, le, uint32(pfRW))
	_ = binary.Write(&out, le, dynSectionOffset) // p_offset
	_ = binary.Write(&out, le, dynVaddr)         // p_vaddr
	_ = binary.Write(&out, le, dynVaddr)         // p_paddr
	_ = binary.Write(&out, le, dynSectionSize)   // p_filesz
	_ = binary.Write(&out, le, dynSectionSize)   // p_memsz
	_ = binary.Write(&out, le, uint64(8))        // p_align

	// ---- .dynamic section data ----
	writeDyn := func(tag int64, val uint64) {
		_ = binary.Write(&out, le, tag)
		_ = binary.Write(&out, le, val)
	}
	writeDyn(dtStrtab, dynstrVaddr)
	writeDyn(dtStrsz, uint64(len(dynstrData)))
	if runpath != "" {
		writeDyn(dtRunpath, runpathIdx)
	}
	for _, idx := range soIdx {
		writeDyn(dtNeeded, idx)
	}
	writeDyn(dtNull, 0)

	// ---- .dynstr ----
	out.Write(dynstrData)

	// ---- .shstrtab ----
	out.Write(shstrtabData)

	// ---- Section headers (4 x 64 bytes) ----
	// link: sh_link (section index of associated section, e.g. for SHT_DYNAMIC -> .dynstr)
	// info is always 0 for our sections; hardcoded below to keep the signature simple.
	writeShdr := func(nameIdx, shType uint32, flags, addr, offset, size uint64,
		link uint32, addralign, entsize uint64,
	) {
		_ = binary.Write(&out, le, nameIdx)
		_ = binary.Write(&out, le, shType)
		_ = binary.Write(&out, le, flags)
		_ = binary.Write(&out, le, addr)
		_ = binary.Write(&out, le, offset)
		_ = binary.Write(&out, le, size)
		_ = binary.Write(&out, le, link)
		_ = binary.Write(&out, le, uint32(0)) // sh_info = 0
		_ = binary.Write(&out, le, addralign)
		_ = binary.Write(&out, le, entsize)
	}

	// Section 0: null (required)
	writeShdr(0, shtNull, 0, 0, 0, 0, 0, 0, 0)

	// Section 1: .dynstr (SHT_STRTAB)
	writeShdr(dynstrNameIdx, shtStrtab, uint64(shfAlloc),
		dynstrVaddr, dynstrOffset, uint64(len(dynstrData)), 0, 1, 0)

	// Section 2: .dynamic (SHT_DYNAMIC), sh_link=1 -> .dynstr
	writeShdr(dynamicNameIdx, shtDynamic, uint64(shfAllocW),
		dynVaddr, dynSectionOffset, dynSectionSize, 1, 8, uint64(dynEntrySize))

	// Section 3: .shstrtab (SHT_STRTAB)
	writeShdr(shstrtabNameIdx, shtStrtab, 0,
		0, shstrtabOffset, uint64(len(shstrtabData)), 0, 1, 0)

	path := filepath.Join(dir, fileName)
	require.NoError(t, os.WriteFile(path, out.Bytes(), 0o644))
	return path
}

// ---- Helper to create a DynLibAnalyzer for tests ----------------------------

func newTestAnalyzer(t *testing.T) *DynLibAnalyzer {
	t.Helper()
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	// nil cache: avoids dependency on /etc/ld.so.cache in unit tests.
	return &DynLibAnalyzer{fs: fs, cache: nil}
}

// ---- Tests ------------------------------------------------------------------

// TestAnalyze_NonELF verifies that Analyze returns nil for non-ELF files.
func TestAnalyze_NonELF(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "script.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho hello\n"), 0o755))

	a := newTestAnalyzer(t)
	result, err := a.Analyze(path)
	require.NoError(t, err)
	assert.Nil(t, result, "non-ELF file should return nil")
}

// TestAnalyze_StaticELF verifies that Analyze returns nil for a static ELF
// (no DT_NEEDED entries).
func TestAnalyze_StaticELF(t *testing.T) {
	tmpDir := t.TempDir()
	// sonames=nil produces an ELF with no DT_NEEDED entries.
	staticELF := buildTestELFWithDeps(t, tmpDir, "static.elf", nil, "")

	a := newTestAnalyzer(t)
	result, err := a.Analyze(staticELF)
	require.NoError(t, err)
	assert.Nil(t, result, "static ELF with no DT_NEEDED should return nil")
}

// TestAnalyze_DynamicELF verifies that Analyze returns non-nil for a native
// dynamically linked binary.
func TestAnalyze_DynamicELF(t *testing.T) {
	binary := resolveTestBinary(t, "/usr/bin/true", "/bin/true")

	a := newTestAnalyzer(t)
	result, err := a.Analyze(binary)
	require.NoError(t, err)
	assert.NotEmpty(t, result, "dynamic ELF should have at least one library entry")
}

// TestAnalyze_LibEntryFields verifies that all fields of LibEntry are populated.
func TestAnalyze_LibEntryFields(t *testing.T) {
	binary := resolveTestBinary(t, "/usr/bin/true", "/bin/true")

	a := newTestAnalyzer(t)
	result, err := a.Analyze(binary)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	for _, lib := range result {
		assert.NotEmpty(t, lib.SOName, "SOName should not be empty")
		assert.NotEmpty(t, lib.Path, "Path should not be empty")
		assert.NotEmpty(t, lib.Hash, "Hash should not be empty")
		assert.True(t, filepath.IsAbs(lib.Path), "Path should be absolute")
		assert.Contains(t, lib.Hash, "sha256:", "Hash should have sha256 prefix")
	}
}

// TestAnalyze_DirectDeps verifies that direct dependencies of the binary are recorded.
func TestAnalyze_DirectDeps(t *testing.T) {
	binaryPath := resolveTestBinary(t, "/usr/bin/true", "/bin/true")

	a := newTestAnalyzer(t)
	result, err := a.Analyze(binaryPath)
	require.NoError(t, err)
	require.NotEmpty(t, result, "dynamic binary should have at least one library")

	// Each entry must have a non-empty SOName, absolute Path, and sha256 hash.
	for _, lib := range result {
		assert.NotEmpty(t, lib.SOName)
		assert.True(t, filepath.IsAbs(lib.Path))
		assert.Contains(t, lib.Hash, "sha256:")
	}
}

// TestAnalyze_TransitiveDeps verifies that transitive (indirect) dependencies
// are resolved when using the minimal ELF builder.
func TestAnalyze_TransitiveDeps(t *testing.T) {
	tmpDir := t.TempDir()

	// leaf.so has no dependencies.
	buildTestELFWithDeps(t, tmpDir, "leaf.so.1", nil, tmpDir)

	// mid.so depends on leaf.so.
	buildTestELFWithDeps(t, tmpDir, "mid.so.1", []string{"leaf.so.1"}, tmpDir)

	// main binary depends on mid.so.
	mainBin := buildTestELFWithDeps(t, tmpDir, "main_trans",
		[]string{"mid.so.1"}, tmpDir)

	a := newTestAnalyzer(t)
	result, err := a.Analyze(mainBin)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, result, 2, "transitive dependency should be recorded")

	sonames := make([]string, 0, len(result))
	for _, lib := range result {
		sonames = append(sonames, lib.SOName)
	}
	assert.Contains(t, sonames, "mid.so.1")
	assert.Contains(t, sonames, "leaf.so.1")
}

// TestAnalyze_ResolutionFailure verifies that Analyze returns
// ErrLibraryNotResolved when a required library cannot be found.
func TestAnalyze_ResolutionFailure(t *testing.T) {
	tmpDir := t.TempDir()
	elfPath := buildTestELFWithDeps(t, tmpDir, "test.so",
		[]string{"libnonexistent12345678.so.99"},
		tmpDir, // RUNPATH points to tmpDir; no matching file there
	)

	a := newTestAnalyzer(t)
	_, err := a.Analyze(elfPath)
	require.Error(t, err, "should return error when library cannot be resolved")

	var notResolved *ErrLibraryNotResolved
	assert.ErrorAs(t, err, &notResolved, "error should be ErrLibraryNotResolved")
}

// TestAnalyze_CircularDeps verifies that circular dependencies do not cause an
// infinite loop; each library should appear exactly once.
func TestAnalyze_CircularDeps(t *testing.T) {
	tmpDir := t.TempDir()

	// lib_a.so <-> lib_b.so (circular).
	buildTestELFWithDeps(t, tmpDir, "lib_a.so.1", []string{"lib_b.so.1"}, tmpDir)
	buildTestELFWithDeps(t, tmpDir, "lib_b.so.1", []string{"lib_a.so.1"}, tmpDir)

	mainBin := buildTestELFWithDeps(t, tmpDir, "main_circ",
		[]string{"lib_a.so.1"}, tmpDir)

	a := newTestAnalyzer(t)
	result, err := a.Analyze(mainBin)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Expected entries (one per unique resolvedPath):
	//   1. lib_a.so.1
	//   2. lib_b.so.1
	// Each physical file appears exactly once regardless of how many parents reference it.
	assert.Len(t, result, 2, "circular deps: 2 unique paths expected")

	sonames := make([]string, 0, len(result))
	for _, lib := range result {
		sonames = append(sonames, lib.SOName)
	}
	assert.Contains(t, sonames, "lib_a.so.1")
	assert.Contains(t, sonames, "lib_b.so.1")
}

// TestAnalyze_MaxDepth verifies that Analyze returns ErrRecursionDepthExceeded
// when the dependency chain exceeds MaxRecursionDepth.
func TestAnalyze_MaxDepth(t *testing.T) {
	tmpDir := t.TempDir()

	// Build a linear chain of depth MaxRecursionDepth + 2.
	chainLen := MaxRecursionDepth + 1

	leafName := "lib_leaf.so.1"
	buildTestELFWithDeps(t, tmpDir, leafName, nil, tmpDir)

	prevName := leafName
	for i := chainLen - 1; i >= 0; i-- {
		name := "lib_chain_" + string(rune('a'+i%26)) + ".so.1"
		buildTestELFWithDeps(t, tmpDir, name, []string{prevName}, tmpDir)
		prevName = name
	}

	mainBin := buildTestELFWithDeps(t, tmpDir, "main_deep", []string{prevName}, tmpDir)

	a := newTestAnalyzer(t)
	_, err := a.Analyze(mainBin)
	require.Error(t, err, "deep recursion should return ErrRecursionDepthExceeded")

	var depthErr *dynlib.ErrRecursionDepthExceeded
	assert.ErrorAs(t, err, &depthErr, "error should be ErrRecursionDepthExceeded")
}

// TestAnalyze_DTRPATHReturnsError verifies that Analyze returns
// ErrDTRPATHNotSupported when the binary itself contains DT_RPATH.
func TestAnalyze_DTRPATHReturnsError(t *testing.T) {
	tmpDir := t.TempDir()

	// Build an ELF with DT_RPATH by temporarily writing the tag directly.
	// We reuse buildTestELFWithDeps with a non-empty path to embed as RUNPATH,
	// then patch the tag byte from DT_RUNPATH (29) to DT_RPATH (15).
	elfPath := buildTestELFWithDeps(t, tmpDir, "main_rpath",
		[]string{"libfoo.so.1"}, tmpDir)

	// Patch DT_RUNPATH tag (29 = 0x1d) to DT_RPATH (15 = 0x0f) in the file.
	data, err := os.ReadFile(elfPath)
	require.NoError(t, err)
	patched := false
	for i := 0; i+8 <= len(data); i += 8 {
		tag := int64(data[i]) | int64(data[i+1])<<8 | int64(data[i+2])<<16 | int64(data[i+3])<<24 |
			int64(data[i+4])<<32 | int64(data[i+5])<<40 | int64(data[i+6])<<48 | int64(data[i+7])<<56
		if tag == 29 { // DT_RUNPATH
			data[i] = 15 // DT_RPATH
			patched = true
			break
		}
	}
	require.True(t, patched, "test ELF should contain DT_RUNPATH to patch")
	require.NoError(t, os.WriteFile(elfPath, data, 0o644))

	a := newTestAnalyzer(t)
	_, err = a.Analyze(elfPath)
	require.Error(t, err)

	var rpathErr *ErrDTRPATHNotSupported
	assert.ErrorAs(t, err, &rpathErr, "error should be ErrDTRPATHNotSupported")
	assert.Equal(t, elfPath, rpathErr.Path)
	assert.NotEmpty(t, rpathErr.RPATH)
}

// TestAnalyze_DTRPATHInDependencyReturnsError verifies that ErrDTRPATHNotSupported
// is returned when a transitive dependency (not the root binary) contains DT_RPATH.
func TestAnalyze_DTRPATHInDependencyReturnsError(t *testing.T) {
	tmpDir := t.TempDir()

	// Build libfoo.so with DT_RUNPATH, then patch to DT_RPATH.
	libPath := buildTestELFWithDeps(t, tmpDir, "libfoo.so.1", nil, tmpDir)
	data, err := os.ReadFile(libPath)
	require.NoError(t, err)
	for i := 0; i+8 <= len(data); i += 8 {
		tag := int64(data[i]) | int64(data[i+1])<<8 | int64(data[i+2])<<16 | int64(data[i+3])<<24 |
			int64(data[i+4])<<32 | int64(data[i+5])<<40 | int64(data[i+6])<<48 | int64(data[i+7])<<56
		if tag == 29 {
			data[i] = 15
			break
		}
	}
	require.NoError(t, os.WriteFile(libPath, data, 0o644))

	// main binary has no RPATH, depends on libfoo.so.1 via RUNPATH.
	mainBin := buildTestELFWithDeps(t, tmpDir, "main_dep_rpath",
		[]string{"libfoo.so.1"}, tmpDir)

	a := newTestAnalyzer(t)
	_, err = a.Analyze(mainBin)
	require.Error(t, err)

	var rpathErr *ErrDTRPATHNotSupported
	assert.ErrorAs(t, err, &rpathErr, "error should be ErrDTRPATHNotSupported")
	assert.Equal(t, libPath, rpathErr.Path)
}

// resolveTestBinary returns the first existing candidate path after resolving
// symlinks. Skips the test if none are found.
func resolveTestBinary(t *testing.T, candidates ...string) string {
	t.Helper()
	for _, c := range candidates {
		resolved, err := filepath.EvalSymlinks(c)
		if err == nil {
			if _, statErr := os.Stat(resolved); statErr == nil {
				return resolved
			}
		}
	}
	t.Skipf("none of the test binaries found: %v", candidates)
	return ""
}
