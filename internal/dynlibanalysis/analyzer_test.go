//go:build test

package dynlibanalysis

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ELF binary test helpers -----------------------------------------------

// buildTestELFWithDeps writes a minimal ELF64 LE (x86-64, ET_DYN) file to dir
// with the given soname entries in DT_NEEDED and an optional RPATH.
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
func buildTestELFWithDeps(t *testing.T, dir, fileName string, sonames []string, rpath string) string {
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
		dtNull   = 0
		dtNeeded = 1
		dtStrtab = 5
		dtStrsz  = 10
		dtRpath  = 15

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

	rpathIdx := uint64(0)
	if rpath != "" {
		rpathIdx = uint64(dynstrBuf.Len())
		dynstrBuf.WriteString(rpath)
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

	// Number of dynamic entries: DT_STRTAB + DT_STRSZ + DT_NULL + optional RPATH + sonames.
	numDynEntries := 3 // DT_STRTAB, DT_STRSZ, DT_NULL
	if rpath != "" {
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
	if rpath != "" {
		writeDyn(dtRpath, rpathIdx)
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
	staticELF := "../../runner/security/elfanalyzer/testdata/static.elf"
	if _, err := os.Stat(staticELF); os.IsNotExist(err) {
		t.Skip("static.elf testdata not found")
	}

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
	require.NotNil(t, result, "dynamic ELF should return DynLibDepsData")
	assert.NotEmpty(t, result.Libs, "dynamic ELF should have at least one library entry")
	assert.False(t, result.RecordedAt.IsZero(), "RecordedAt should be set")
}

// TestAnalyze_LibEntryFields verifies that all fields of LibEntry are populated.
func TestAnalyze_LibEntryFields(t *testing.T) {
	binary := resolveTestBinary(t, "/usr/bin/true", "/bin/true")

	a := newTestAnalyzer(t)
	result, err := a.Analyze(binary)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Libs)

	for _, lib := range result.Libs {
		assert.NotEmpty(t, lib.SOName, "SOName should not be empty")
		assert.NotEmpty(t, lib.Path, "Path should not be empty")
		assert.NotEmpty(t, lib.Hash, "Hash should not be empty")
		assert.True(t, filepath.IsAbs(lib.Path), "Path should be absolute")
		assert.Contains(t, lib.Hash, "sha256:", "Hash should have sha256 prefix")
	}
}

// TestAnalyze_ParentPath verifies that direct dependencies have the binary as
// their ParentPath.
func TestAnalyze_ParentPath(t *testing.T) {
	binaryPath := resolveTestBinary(t, "/usr/bin/true", "/bin/true")

	a := newTestAnalyzer(t)
	result, err := a.Analyze(binaryPath)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Libs)

	for _, lib := range result.Libs {
		if lib.ParentPath == binaryPath {
			return
		}
	}
	t.Errorf("no library entry has ParentPath=%q", binaryPath)
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

	assert.Len(t, result.Libs, 2, "transitive dependency should be recorded")

	sonames := make([]string, 0, len(result.Libs))
	for _, lib := range result.Libs {
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
		tmpDir, // RPATH points to tmpDir; no matching file there
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

	// Expected entries (one per unique (resolvedPath, parentPath) pair):
	//   1. lib_a.so.1  parent=main_circ   (direct dependency)
	//   2. lib_b.so.1  parent=lib_a.so.1  (lib_a's dependency)
	//   3. lib_a.so.1  parent=lib_b.so.1  (circular back-reference, new parent)
	// Entry 3 is required so the verifier can re-resolve the (lib_b → lib_a) edge.
	// ELF traversal of lib_a is performed only once (traversed set prevents re-traversal).
	assert.Len(t, result.Libs, 3, "circular deps: 3 unique (path, parent) pairs expected")

	// No physical file should be traversed more than once (no child duplication).
	sonamePaths := make(map[string]int)
	for _, lib := range result.Libs {
		sonamePaths[lib.Path]++
	}
	// lib_a.so.1 appears under two different parents, lib_b.so.1 under one.
	assert.Equal(t, 2, sonamePaths[result.Libs[0].Path], "lib_a should appear under 2 parents")
	assert.Equal(t, 1, sonamePaths[result.Libs[1].Path], "lib_b should appear under 1 parent")
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

	var depthErr *ErrRecursionDepthExceeded
	assert.ErrorAs(t, err, &depthErr, "error should be ErrRecursionDepthExceeded")
}

// TestAnalyze_SharedLibDifferentContexts verifies that when the same physical
// library (libshared.so) is reachable via two parents that carry different
// OwnRPATH entries, and libshared.so itself has no RPATH so it resolves its
// child via the inherited RPATH chain, both sets of grandchild dependencies are
// recorded.
//
// Dependency graph and RPATH inheritance:
//
//	main  (RPATH=dirMain)
//	├── libA.so  (in dirMain, RPATH=dirA:dirShared)
//	│   └── libshared.so  → dirShared/libshared.so  (no RPATH)
//	│       └── libgrand.so  → dirA/libgrand.so
//	│           (inherited chain: dirMain, dirA, dirShared — dirA wins over dirShared)
//	└── libB.so  (in dirMain, RPATH=dirB:dirShared)
//	    └── libshared.so  → dirShared/libshared.so  (same physical file)
//	        └── libgrand.so  → dirB/libgrand.so
//	            (inherited chain: dirMain, dirB, dirShared — dirB wins over dirShared)
//
// Key: dirMain does NOT contain libgrand.so, so the first match in the
// inherited chain differs between the two contexts (dirA vs dirB).
//
// A traversed key of resolvedPath alone cuts off libshared.so after the first
// (libA) context, missing dirB/libgrand.so. The fix keys traversal on
// (resolvedPath, rpathFingerprint) to detect the distinct contexts.
func TestAnalyze_SharedLibDifferentContexts(t *testing.T) {
	tmpDir := t.TempDir()
	dirMain := filepath.Join(tmpDir, "dirMain")
	dirA := filepath.Join(tmpDir, "dirA")
	dirB := filepath.Join(tmpDir, "dirB")
	dirShared := filepath.Join(tmpDir, "dirShared")
	for _, d := range []string{dirMain, dirA, dirB, dirShared} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}

	// libgrand.so exists only in dirA and dirB, NOT in dirMain or dirShared.
	// This ensures the inherited RPATH order (dirA-first vs dirB-first) determines
	// which copy is found.
	buildTestELFWithDeps(t, dirA, "libgrand.so", nil, "")
	buildTestELFWithDeps(t, dirB, "libgrand.so", nil, "")

	// libshared.so has no RPATH; libgrand.so is resolved via the inherited chain.
	buildTestELFWithDeps(t, dirShared, "libshared.so", []string{"libgrand.so"}, "")

	// libA.so: OwnRPATH = dirA:dirShared — libgrand.so found in dirA first.
	buildTestELFWithDeps(t, dirMain, "libA.so", []string{"libshared.so"}, dirA+":"+dirShared)
	// libB.so: OwnRPATH = dirB:dirShared — libgrand.so found in dirB first.
	buildTestELFWithDeps(t, dirMain, "libB.so", []string{"libshared.so"}, dirB+":"+dirShared)

	// main has RPATH=dirMain to locate libA.so and libB.so.
	// dirMain does NOT contain libgrand.so, so the inherited dirMain entry is
	// a no-op for libgrand.so resolution, and dirA/dirB in each library's
	// OwnRPATH determine the winning path.
	mainBin := buildTestELFWithDeps(t, tmpDir, "main_shared_ctx",
		[]string{"libA.so", "libB.so"}, dirMain)

	a := newTestAnalyzer(t)
	result, err := a.Analyze(mainBin)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Collect all (soname, path) pairs recorded.
	type libKey struct{ soname, path string }
	recordedLibs := make(map[libKey]bool)
	for _, lib := range result.Libs {
		recordedLibs[libKey{lib.SOName, lib.Path}] = true
	}

	grandA := filepath.Join(dirA, "libgrand.so")
	grandB := filepath.Join(dirB, "libgrand.so")

	// Both grandchild paths must be recorded.
	// Without the fix, libshared.so's children are expanded only under libA's
	// inherited RPATH chain, so dirB/libgrand.so is never enqueued.
	assert.True(t, recordedLibs[libKey{"libgrand.so", grandA}],
		"grandchild via libA context (%s) should be recorded", grandA)
	assert.True(t, recordedLibs[libKey{"libgrand.so", grandB}],
		"grandchild via libB context (%s) should be recorded", grandB)
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
