package elfanalyzer

import (
	"debug/elf"
)

// WrapperCall represents a detected call to a known Go syscall wrapper function.
type WrapperCall struct {
	// SyscallNumber is the syscall number passed to the wrapper.
	// -1 indicates the number could not be determined.
	SyscallNumber int

	// CallSiteAddress is the virtual address of the CALL instruction.
	CallSiteAddress uint64

	// WrapperName is the name of the detected wrapper function.
	WrapperName string
}

// GoWrapperResolver resolves Go syscall wrapper function calls.
// It identifies calls to known wrapper functions (e.g., syscall.Syscall,
// syscall.Syscall6) and attempts to determine the syscall number argument.
type GoWrapperResolver struct {
	hasSymbols bool
}

// NewGoWrapperResolver creates a new GoWrapperResolver.
func NewGoWrapperResolver() *GoWrapperResolver {
	return &GoWrapperResolver{}
}

// LoadSymbols loads function symbols from the ELF file.
// This populates the resolver with function address information
// needed to identify wrapper calls.
//
// Currently a stub; full implementation in Phase 3 (pclntab_parser.go + go_wrapper_resolver.go).
func (r *GoWrapperResolver) LoadSymbols(elfFile *elf.File) error {
	_ = elfFile
	return ErrSymbolLoadingNotImplemented
}

// HasSymbols returns true if symbols were successfully loaded.
func (r *GoWrapperResolver) HasSymbols() bool {
	return r.hasSymbols
}

// FindWrapperCalls scans the code for calls to known Go syscall wrappers.
// Returns a list of detected wrapper calls with their syscall numbers.
func (r *GoWrapperResolver) FindWrapperCalls(code []byte, baseAddr uint64) []WrapperCall {
	// Full implementation in Phase 3
	_ = code
	_ = baseAddr
	return nil
}
