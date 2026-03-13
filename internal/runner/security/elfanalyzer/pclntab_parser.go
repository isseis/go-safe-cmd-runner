package elfanalyzer

import (
	"debug/elf"
	"debug/gosym"
	"errors"
	"fmt"
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

// ParsePclntab reads the .gopclntab section from an ELF file and extracts
// function information. This works even on stripped binaries because Go
// runtime requires pclntab for stack traces and garbage collection.
//
// For CGO binaries, the .text section contains C runtime startup code before
// the Go runtime functions. This causes pclntab addresses to be offset from
// the actual virtual addresses. ParsePclntab detects and corrects this offset
// by comparing pclntab entries against .symtab entries when available.
func ParsePclntab(elfFile *elf.File) (map[string]PclntabFunc, error) {
	pclntabSection := elfFile.Section(".gopclntab")
	if pclntabSection == nil {
		return nil, ErrNoPclntab
	}

	pclntabData, err := pclntabSection.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read .gopclntab: %w", err)
	}

	// The text start address is required by gosym.NewLineTable.
	// In Go 1.26+, textStart was removed from the pclntab header and must be
	// obtained from the ELF .text section directly.
	var textStart uint64
	textSection := elfFile.Section(".text")
	if textSection != nil {
		textStart = textSection.Addr
	}

	lineTable := gosym.NewLineTable(pclntabData, textStart)
	symTable, err := gosym.NewTable(nil, lineTable)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupportedPclntab, err)
	}

	functions := make(map[string]PclntabFunc, len(symTable.Funcs))
	for i := range symTable.Funcs {
		fn := &symTable.Funcs[i]
		functions[fn.Name] = PclntabFunc{
			Name:  fn.Name,
			Entry: fn.Entry,
			End:   fn.End,
		}
	}

	// CGO binaries may have a constant address offset between pclntab entries
	// and actual virtual addresses because C runtime startup code is inserted
	// at the beginning of the .text section. Detect and apply the correction.
	if offset := detectPclntabOffset(elfFile, functions); offset != 0 {
		for name, fn := range functions {
			functions[name] = PclntabFunc{
				Name:  fn.Name,
				Entry: uint64(int64(fn.Entry) + offset), //nolint:gosec // G115: offset is bounded by binary size, no overflow risk
				End:   uint64(int64(fn.End) + offset),   //nolint:gosec // G115: offset is bounded by binary size, no overflow risk
			}
		}
	}

	return functions, nil
}

// detectPclntabOffset returns the address correction needed for pclntab entries
// in CGO binaries. It compares pclntab function addresses against .symtab entries
// to detect a constant offset introduced by C runtime startup code in .text.
//
// Returns 0 if no correction is needed (non-CGO binaries, stripped binaries,
// or when .symtab is unavailable).
func detectPclntabOffset(elfFile *elf.File, pclntabFuncs map[string]PclntabFunc) int64 {
	syms, err := elfFile.Symbols()
	if err != nil {
		// .symtab absent (stripped binary) — skip correction.
		// Stripped CGO binaries remain IsHighRisk via the fail-safe path.
		return 0
	}

	for _, sym := range syms {
		fn, ok := pclntabFuncs[sym.Name]
		if !ok || sym.Value == 0 || fn.Entry == 0 {
			continue
		}
		// First matching symbol determines the offset.
		return int64(sym.Value) - int64(fn.Entry) //nolint:gosec // G115: addresses are valid ELF virtual addresses
	}
	return 0
}
