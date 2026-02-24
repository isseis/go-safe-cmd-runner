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

// PclntabResult holds the parsed pclntab data.
type PclntabResult struct {
	GoVersion string
	Functions map[string]PclntabFunc
}

// FindFunction finds a function by name.
func (r *PclntabResult) FindFunction(name string) (PclntabFunc, bool) {
	fn, ok := r.Functions[name]
	return fn, ok
}

// ParsePclntab reads the .gopclntab section from an ELF file and extracts
// function information. This works even on stripped binaries because Go
// runtime requires pclntab for stack traces and garbage collection.
func ParsePclntab(elfFile *elf.File) (*PclntabResult, error) {
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

	return &PclntabResult{
		GoVersion: "go1.2+",
		Functions: functions,
	}, nil
}
