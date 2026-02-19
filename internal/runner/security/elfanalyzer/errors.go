package elfanalyzer

import (
	"debug/elf"
	"errors"
	"fmt"
)

// Static errors
var (
	// ErrNotELF indicates the file is not an ELF binary.
	// This error is returned when the file cannot be parsed as ELF format.
	ErrNotELF = errors.New("file is not an ELF binary")

	// ErrNotStaticELF indicates the ELF file is dynamically linked, not statically linked.
	// This error is returned when syscall analysis is attempted on a dynamic binary.
	ErrNotStaticELF = errors.New("ELF file is not statically linked")

	// ErrNoTextSection indicates the ELF file has no .text section.
	ErrNoTextSection = errors.New("ELF file has no .text section")

	// ErrNoSymbolTable indicates the ELF file has no symbol table.
	ErrNoSymbolTable = errors.New("ELF file has no symbol table (possibly stripped)")

	// ErrRecordNotFound indicates that no analysis record exists for the file.
	ErrRecordNotFound = errors.New("syscall analysis record not found")

	// ErrHashMismatch indicates that the file content hash does not match the stored record.
	ErrHashMismatch = errors.New("syscall analysis hash mismatch")

	// ErrNoSyscallAnalysis indicates that the record exists but contains no syscall analysis data.
	ErrNoSyscallAnalysis = errors.New("no syscall analysis data in record")
)

// UnsupportedArchitectureError indicates the ELF architecture is not supported.
type UnsupportedArchitectureError struct {
	Machine elf.Machine
}

func (e *UnsupportedArchitectureError) Error() string {
	return fmt.Sprintf("unsupported ELF architecture: %s", e.Machine)
}
