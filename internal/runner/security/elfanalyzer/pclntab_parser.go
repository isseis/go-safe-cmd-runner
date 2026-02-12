package elfanalyzer

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// pclntab magic numbers for different Go versions
const (
	pclntabMagicGo12  = 0xFFFFFFFB // Go 1.2 - 1.15
	pclntabMagicGo116 = 0xFFFFFFFA // Go 1.16 - 1.17
	pclntabMagicGo118 = 0xFFFFFFF0 // Go 1.18 - 1.19
	pclntabMagicGo120 = 0xFFFFFFF1 // Go 1.20+
)

// pclntab header constants
const (
	pclntabMinMagicSize  = 8    // Minimum bytes needed to read magic
	pclntabMinHeaderSize = 16   // Minimum bytes needed for basic header
	pclntab64PtrSize     = 8    // Expected pointer size for 64-bit binaries
	pcHeaderSizeGo125    = 0x48 // pcHeader size for Go 1.25+ 64-bit (72 bytes, ftabOffset removed)
	pcHeaderSizeFull     = 0x50 // pcHeader size for Go 1.16-1.24 64-bit (80 bytes)
)

// pcHeader field offsets (Go 1.16+, 64-bit)
const (
	pcHeaderOffsetNfunc       = 0x08
	pcHeaderOffsetTextStart   = 0x18
	pcHeaderOffsetFuncnameOff = 0x20
	pcHeaderOffsetPclnOff     = 0x40 // pclntabOffset; in Go 1.25+ also serves as ftabOffset
	pcHeaderOffsetFtab        = 0x48 // ftabOffset; Go 1.20-1.24 only, removed in Go 1.25+
)

// functab entry field offsets
const (
	ftabEntryOffsetFuncOff = 4 // Offset to funcoff within functab entry (uint32)
)

// _func struct field offsets
const (
	funcStructOffsetNameOff = 4 // Offset to nameoff within _func struct
)

// Errors
var (
	ErrNoPclntab          = errors.New("no .gopclntab section found")
	ErrUnsupportedPclntab = errors.New("unsupported pclntab format")
	ErrInvalidPclntab     = errors.New("invalid pclntab structure")
)

// PclntabFunc represents a function entry in pclntab.
type PclntabFunc struct {
	Name  string
	Entry uint64 // Function entry address
	End   uint64 // Function end address (if available)
}

// PclntabParser parses Go's pclntab to extract function information.
// Only 64-bit binaries (ptrSize == 8) are supported (x86_64 target).
type PclntabParser struct {
	ptrSize   int    // Must be 8 for x86_64
	goVersion string // Detected Go version range
	funcData  []PclntabFunc
}

// NewPclntabParser creates a new PclntabParser.
func NewPclntabParser() *PclntabParser {
	return &PclntabParser{
		funcData: make([]PclntabFunc, 0),
	}
}

// Parse reads the .gopclntab section and extracts function information.
// This works even on stripped binaries because Go runtime requires pclntab
// for stack traces and garbage collection.
func (p *PclntabParser) Parse(elfFile *elf.File) error {
	// Reset state to ensure failed parse doesn't leave stale data
	p.funcData = make([]PclntabFunc, 0)
	p.goVersion = ""
	p.ptrSize = 0

	// Find .gopclntab section
	section := elfFile.Section(".gopclntab")
	if section == nil {
		return ErrNoPclntab
	}

	data, err := section.Data()
	if err != nil {
		return fmt.Errorf("failed to read .gopclntab: %w", err)
	}

	if len(data) < pclntabMinMagicSize {
		return ErrInvalidPclntab
	}

	// Read magic number (first 4 bytes, little-endian)
	magic := binary.LittleEndian.Uint32(data[0:4])

	switch magic {
	case pclntabMagicGo118, pclntabMagicGo120:
		// Go 1.18+ format - supported
		return p.parseGo118Plus(data)
	case pclntabMagicGo116:
		// Go 1.16-1.17 format - supported with limitations
		return p.parseGo116(data)
	case pclntabMagicGo12:
		// Go 1.2-1.15 format - legacy, limited support
		return p.parseGo12(data)
	default:
		return fmt.Errorf("%w: unknown magic 0x%08X", ErrUnsupportedPclntab, magic)
	}
}

// parseGo118Plus parses pclntab for Go 1.18 and later.
// Reference: https://go.dev/src/runtime/symtab.go
func (p *PclntabParser) parseGo118Plus(data []byte) error {
	if len(data) < pclntabMinHeaderSize {
		return ErrInvalidPclntab
	}

	// Header layout for Go 1.18+:
	// [0:4]   magic
	// [4:5]   padding (0)
	// [5:6]   padding (0)
	// [6:7]   instruction size quantum (1 for x86, 4 for ARM)
	// [7:8]   pointer size (must be 8 for x86_64)
	// [8:16]  nfunc (number of functions) - uint64 for 64-bit

	p.ptrSize = int(data[7])
	if p.ptrSize != pclntab64PtrSize {
		return fmt.Errorf("%w: unsupported pointer size %d (only 64-bit supported)", ErrInvalidPclntab, p.ptrSize)
	}

	p.goVersion = "go1.18+"

	// Parse function table
	// The structure varies by Go version, but function entries contain:
	// - entry PC (function start address)
	// - offset to function name in string table
	return p.parseFuncTable(data)
}

// parseGo116 parses pclntab for Go 1.16-1.17.
func (p *PclntabParser) parseGo116(data []byte) error {
	if len(data) < pclntabMinHeaderSize {
		return ErrInvalidPclntab
	}

	p.ptrSize = int(data[7])
	if p.ptrSize != pclntab64PtrSize {
		return fmt.Errorf("%w: unsupported pointer size %d (only 64-bit supported)", ErrInvalidPclntab, p.ptrSize)
	}

	p.goVersion = "go1.16-1.17"
	return p.parseFuncTable(data)
}

// parseGo12 parses pclntab for Go 1.2-1.15 (legacy format).
func (p *PclntabParser) parseGo12(data []byte) error {
	if len(data) < pclntabMinMagicSize {
		return ErrInvalidPclntab
	}

	// Go 1.2-1.15 header:
	// [0:4]   magic
	// [4:5]   padding
	// [5:6]   padding
	// [6:7]   instruction size quantum
	// [7:8]   pointer size (must be 8 for x86_64)

	p.ptrSize = int(data[7])
	if p.ptrSize != pclntab64PtrSize {
		return fmt.Errorf("%w: unsupported pointer size %d (only 64-bit supported)", ErrInvalidPclntab, p.ptrSize)
	}

	p.goVersion = "go1.2-1.15"
	return p.parseFuncTable(data)
}

// parseFuncTable extracts function entries from the pclntab.
// This implementation targets Go 1.18+ pclntab layout (pcHeader + functab) on x86_64.
// Legacy formats (Go 1.2-1.17) are best-effort and may return ErrInvalidPclntab.
//
// Note: This implementation only supports 64-bit binaries (ptrSize == 8).
// 32-bit binaries are not supported as the target architecture is x86_64 only.
func (p *PclntabParser) parseFuncTable(data []byte) error {
	// pcHeader layout (Go 1.16+, 64-bit)
	// offset 0x00: magic (uint32)
	// offset 0x04: pad1 (byte)
	// offset 0x05: pad2 (byte)
	// offset 0x06: minLC (byte)
	// offset 0x07: ptrSize (byte)
	// offset 0x08: nfunc (uint64)
	// offset 0x10: nfiles (uint64)
	// offset 0x18: textStart (uint64)
	// offset 0x20: funcnameOffset (uint64)
	// offset 0x28: cuOffset (uint64)
	// offset 0x30: filetabOffset (uint64)
	// offset 0x38: pctabOffset (uint64)
	// offset 0x40: pclntabOffset (uint64)
	// offset 0x48: ftabOffset (uint64)  -- Go 1.20-1.24 only; removed in Go 1.25+
	//
	// Go 1.20-1.24 header size: 0x50 (80 bytes)
	// Go 1.25+     header size: 0x48 (72 bytes)
	//
	// Note: ptrSize validation is skipped here as it's already performed in
	// parseGo118Plus, parseGo116, and parseGo12 before calling parseFuncTable.

	if len(data) < pcHeaderSizeGo125 {
		return ErrInvalidPclntab
	}

	nfunc, textStart, funcNameOff, ftabOff, err := p.readHeaderFields(data)
	if err != nil {
		return err
	}

	return p.extractFunctions(data, nfunc, textStart, funcNameOff, ftabOff)
}

// readHeaderFields reads the key fields from the pcHeader.
// Supports both Go 1.20-1.24 (80-byte header with ftabOffset) and
// Go 1.25+ (72-byte header where ftab is at pclntabOffset).
func (p *PclntabParser) readHeaderFields(data []byte) (nfunc, textStart, funcNameOff, ftabOff uint64, err error) {
	readUint64 := func(off int) (uint64, error) {
		if off < 0 || off+pclntab64PtrSize > len(data) {
			return 0, ErrInvalidPclntab
		}
		return binary.LittleEndian.Uint64(data[off : off+pclntab64PtrSize]), nil
	}

	nfunc, err = readUint64(pcHeaderOffsetNfunc)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	textStart, err = readUint64(pcHeaderOffsetTextStart)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	funcNameOff, err = readUint64(pcHeaderOffsetFuncnameOff)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	pclnOff, err := readUint64(pcHeaderOffsetPclnOff)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	// Detect Go 1.25+ vs Go 1.20-1.24 header format.
	// In Go 1.25+, the ftabOffset field was removed from pcHeader (72 bytes).
	// The ftab is located at the same offset as the pclntab.
	//
	// Detection: In Go 1.20-1.24, the header is 80 bytes (0x50), so the first
	// table (funcname) begins at offset >= 0x50. In Go 1.25+, the header is
	// 72 bytes (0x48), so tables can start at offset 0x48. If funcNameOff < 0x50,
	// the header must be the shorter Go 1.25+ format.
	if funcNameOff < pcHeaderSizeFull {
		// Go 1.25+: ftab is at pclntabOffset (merged with pclntab)
		ftabOff = pclnOff
	} else {
		// Go 1.20-1.24: ftab offset is stored as a separate field at offset 0x48
		ftabOff, err = readUint64(pcHeaderOffsetFtab)
		if err != nil {
			return 0, 0, 0, 0, err
		}
	}

	return nfunc, textStart, funcNameOff, ftabOff, nil
}

// extractFunctions extracts function entries from the functab.
func (p *PclntabParser) extractFunctions(data []byte, nfunc, textStart, funcNameOff, ftabOff uint64) error {
	// Validate uint64 values fit in int before conversion to prevent overflow.
	if nfunc > uint64(math.MaxInt) || ftabOff > uint64(math.MaxInt) ||
		funcNameOff > uint64(math.MaxInt) {
		return ErrInvalidPclntab
	}

	// Function table: nfunc+1 entries, each entry is {entryoff uint32, funcoff uint32}
	// entry address = textStart + entryoff
	// funcoff points to _func struct; nameoff is at +4 in _func
	const entrySize = pclntab64PtrSize // 8 bytes: {entryoff uint32, funcoff uint32}
	nfuncInt := int(nfunc)
	ftabStart := int(ftabOff)
	// Check for overflow in ftabBytes calculation:
	// (nfuncInt + 1) * entrySize must fit in int.
	// nfuncInt is guaranteed non-negative because nfunc (uint64) was checked
	// against math.MaxInt above.
	if nfuncInt > (math.MaxInt/entrySize)-1 {
		return ErrInvalidPclntab
	}
	ftabBytes := (nfuncInt + 1) * entrySize
	// Check if ftabStart + ftabBytes would overflow or exceed data length.
	// This single check handles both overflow and bounds validation safely.
	if ftabStart > len(data)-ftabBytes {
		return ErrInvalidPclntab
	}

	funcs := make([]PclntabFunc, 0, nfuncInt)
	funcNameOffInt := int(funcNameOff)

	for i := range nfuncInt {
		fn, err := p.extractSingleFunction(data, ftabStart, funcNameOffInt, i, entrySize, nfuncInt, textStart)
		if err != nil {
			return err
		}
		funcs = append(funcs, fn)
	}

	p.funcData = funcs
	return nil
}

// extractSingleFunction extracts a single function entry from the functab.
func (p *PclntabParser) extractSingleFunction(data []byte, ftabStart, funcNameOffInt, idx, entrySize, nfuncInt int, textStart uint64) (PclntabFunc, error) {
	readUint32 := func(b []byte, off int) (uint32, error) {
		const uint32Size = 4 // Size of uint32 in bytes
		if off < 0 || off+uint32Size > len(b) {
			return 0, ErrInvalidPclntab
		}
		return binary.LittleEndian.Uint32(b[off : off+uint32Size]), nil
	}

	entryOff, err := readUint32(data, ftabStart+idx*entrySize)
	if err != nil {
		return PclntabFunc{}, err
	}
	funcOff, err := readUint32(data, ftabStart+idx*entrySize+ftabEntryOffsetFuncOff)
	if err != nil {
		return PclntabFunc{}, err
	}

	entry := uint64(entryOff) + textStart
	// funcOff is uint32, safe to convert to int on 64-bit systems
	funcDataOff := int(funcOff)
	if funcDataOff+funcStructOffsetNameOff > len(data) {
		return PclntabFunc{}, ErrInvalidPclntab
	}
	nameOff32, err := readUint32(data, funcDataOff+funcStructOffsetNameOff)
	if err != nil {
		return PclntabFunc{}, err
	}

	// Check for overflow in nameStart calculation
	// Both funcNameOffInt and nameOff32 are non-negative, check sum doesn't overflow
	if int(nameOff32) > math.MaxInt-funcNameOffInt {
		return PclntabFunc{}, ErrInvalidPclntab
	}
	nameStart := funcNameOffInt + int(nameOff32)
	if nameStart >= len(data) {
		return PclntabFunc{}, ErrInvalidPclntab
	}

	// Read null-terminated function name
	nameEnd := nameStart
	for nameEnd < len(data) && data[nameEnd] != 0x00 {
		nameEnd++
	}
	if nameEnd == len(data) {
		return PclntabFunc{}, ErrInvalidPclntab
	}
	name := string(data[nameStart:nameEnd])

	// Determine end address from next function entry (if available)
	end := uint64(0)
	if idx+1 < nfuncInt {
		nextEntryOff, err := readUint32(data, ftabStart+(idx+1)*entrySize)
		if err != nil {
			return PclntabFunc{}, err
		}
		end = uint64(nextEntryOff) + textStart
	}

	return PclntabFunc{
		Name:  name,
		Entry: entry,
		End:   end,
	}, nil
}

// GetFunctions returns all parsed function information.
func (p *PclntabParser) GetFunctions() []PclntabFunc {
	return p.funcData
}

// FindFunction finds a function by name.
func (p *PclntabParser) FindFunction(name string) (PclntabFunc, bool) {
	for _, f := range p.funcData {
		if f.Name == name {
			return f, true
		}
	}
	return PclntabFunc{}, false
}

// GetGoVersion returns the detected Go version range.
func (p *PclntabParser) GetGoVersion() string {
	return p.goVersion
}
