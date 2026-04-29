//go:build test && linux

package filevalidator

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildMinimalELFWithDeps writes a minimal x86-64 ELF (ET_DYN) to dir/fileName.
// sonames are recorded as DT_NEEDED entries in the given order.
// If runpath is non-empty it is set as DT_RUNPATH.
// Returns the absolute path of the written file.
//
// This is a local copy of the elfdynlib test helper — kept here to avoid
// cross-package test dependencies.
func buildMinimalELFWithDeps(t *testing.T, dir, fileName string, sonames []string, runpath string) string {
	t.Helper()

	const (
		elfHeaderSize = 64
		phdrSize      = 56
		shdrSize      = 64
		dynEntrySize  = 16
		baseVaddr     = uint64(0x400000)

		elfClass64 = 2
		elfDataLE  = 1
		elfVersion = 1
		etDyn      = 3
		emX8664    = 62
		ptLoad     = 1
		ptDynamic  = 2
		pfRX       = 5
		pfRW       = 6

		dtNull    = 0
		dtNeeded  = 1
		dtStrtab  = 5
		dtStrsz   = 10
		dtRunpath = 29

		shtNull    = 0
		shtStrtab  = 3
		shtDynamic = 6

		shfAlloc  = 2
		shfAllocW = shfAlloc | 1
	)

	numPhdrs := 2

	var dynstrBuf bytes.Buffer
	dynstrBuf.WriteByte(0)

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

	var shstrtabBuf bytes.Buffer
	shstrtabBuf.WriteByte(0)
	dynstrNameIdx := uint32(shstrtabBuf.Len())
	shstrtabBuf.WriteString(".dynstr\x00")
	dynamicNameIdx := uint32(shstrtabBuf.Len())
	shstrtabBuf.WriteString(".dynamic\x00")
	shstrtabNameIdx := uint32(shstrtabBuf.Len())
	shstrtabBuf.WriteString(".shstrtab\x00")
	shstrtabData := shstrtabBuf.Bytes()

	numDynEntries := 3
	if runpath != "" {
		numDynEntries++
	}
	numDynEntries += len(sonames)

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

	out.Write([]byte{
		0x7f, 'E', 'L', 'F', elfClass64, elfDataLE, elfVersion, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
	})
	_ = binary.Write(&out, le, uint16(etDyn))
	_ = binary.Write(&out, le, uint16(emX8664))
	_ = binary.Write(&out, le, uint32(elfVersion))
	_ = binary.Write(&out, le, uint64(0))
	_ = binary.Write(&out, le, uint64(elfHeaderSize))
	_ = binary.Write(&out, le, shOffset)
	_ = binary.Write(&out, le, uint32(0))
	_ = binary.Write(&out, le, uint16(elfHeaderSize))
	_ = binary.Write(&out, le, uint16(phdrSize))
	_ = binary.Write(&out, le, uint16(numPhdrs))
	_ = binary.Write(&out, le, uint16(shdrSize))
	_ = binary.Write(&out, le, uint16(4))
	_ = binary.Write(&out, le, uint16(3))

	_ = binary.Write(&out, le, uint32(ptLoad))
	_ = binary.Write(&out, le, uint32(pfRX))
	_ = binary.Write(&out, le, uint64(0))
	_ = binary.Write(&out, le, baseVaddr)
	_ = binary.Write(&out, le, baseVaddr)
	_ = binary.Write(&out, le, totalSize)
	_ = binary.Write(&out, le, totalSize)
	_ = binary.Write(&out, le, uint64(0x1000))

	_ = binary.Write(&out, le, uint32(ptDynamic))
	_ = binary.Write(&out, le, uint32(pfRW))
	_ = binary.Write(&out, le, dynSectionOffset)
	_ = binary.Write(&out, le, dynVaddr)
	_ = binary.Write(&out, le, dynVaddr)
	_ = binary.Write(&out, le, dynSectionSize)
	_ = binary.Write(&out, le, dynSectionSize)
	_ = binary.Write(&out, le, uint64(8))

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

	out.Write(dynstrData)
	out.Write(shstrtabData)

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
		_ = binary.Write(&out, le, uint32(0))
		_ = binary.Write(&out, le, addralign)
		_ = binary.Write(&out, le, entsize)
	}

	writeShdr(0, shtNull, 0, 0, 0, 0, 0, 0, 0)
	writeShdr(dynstrNameIdx, shtStrtab, uint64(shfAlloc),
		dynstrVaddr, dynstrOffset, uint64(len(dynstrData)), 0, 1, 0)
	writeShdr(dynamicNameIdx, shtDynamic, uint64(shfAllocW),
		dynVaddr, dynSectionOffset, dynSectionSize, 1, 8, uint64(dynEntrySize))
	writeShdr(shstrtabNameIdx, shtStrtab, 0,
		0, shstrtabOffset, uint64(len(shstrtabData)), 0, 1, 0)

	path := filepath.Join(dir, fileName)
	require.NoError(t, os.WriteFile(path, out.Bytes(), 0o644))
	return path
}

// TestDynLibDeps_SortedBySOName verifies that DynLibDeps is sorted by SOName
// (then Path, then Hash) regardless of the order DT_NEEDED entries appear in
// the binary.
func TestDynLibDeps_SortedBySOName(t *testing.T) {
	tmpDir := safeTempDir(t)
	hashDir := filepath.Join(tmpDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	// Create leaf libraries (no deps) in the temp dir.
	buildMinimalELFWithDeps(t, tmpDir, "libz.so.1", nil, "")
	buildMinimalELFWithDeps(t, tmpDir, "libm.so.6", nil, "")
	buildMinimalELFWithDeps(t, tmpDir, "liba.so.1", nil, "")

	// Main ELF references the three libraries in reverse-alphabetical order.
	mainELF := buildMinimalELFWithDeps(t, tmpDir, "main.elf",
		[]string{"libz.so.1", "libm.so.6", "liba.so.1"}, tmpDir)

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	v.SetELFDynLibAnalyzer(elfdynlib.NewDynLibAnalyzer(
		safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
	))

	_, _, err = v.SaveRecord(mainELF, false)
	require.NoError(t, err)

	record, err := v.LoadRecord(mainELF)
	require.NoError(t, err)

	require.Len(t, record.DynLibDeps, 3)
	sonames := make([]string, len(record.DynLibDeps))
	for i, e := range record.DynLibDeps {
		sonames[i] = e.SOName
	}
	assert.Equal(t, []string{"liba.so.1", "libm.so.6", "libz.so.1"}, sonames)
}
