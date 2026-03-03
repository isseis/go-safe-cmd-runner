package elfanalyzer

import (
	"debug/elf"
)

// ARM64GoWrapperResolver implements GoWrapperResolver for arm64 binaries.
type ARM64GoWrapperResolver struct {
	goWrapperBase
	decoder *ARM64Decoder // Shared decoder instance to avoid repeated allocation
}

// NewARM64GoWrapperResolver creates a new ARM64GoWrapperResolver and loads symbols
// from the given ELF file's .gopclntab section.
//
// Returns an error if symbol loading fails (e.g., missing .gopclntab).
// Even on error, the returned resolver is safe to use; it simply has no
// symbols loaded and FindWrapperCalls will return nil.
func NewARM64GoWrapperResolver(elfFile *elf.File) (*ARM64GoWrapperResolver, error) {
	r := newARM64GoWrapperResolver()
	if err := r.loadFromPclntab(elfFile); err != nil {
		return r, err
	}
	r.hasSymbols = len(r.symbols) > 0
	return r, nil
}

// newARM64GoWrapperResolver creates an empty ARM64GoWrapperResolver without loading symbols.
// This is used internally and by tests that set up symbols manually.
func newARM64GoWrapperResolver() *ARM64GoWrapperResolver {
	return &ARM64GoWrapperResolver{
		goWrapperBase: goWrapperBase{
			symbols:      make(map[string]SymbolInfo),
			wrapperAddrs: make(map[uint64]GoSyscallWrapper),
		},
		decoder: NewARM64Decoder(),
	}
}

// FindWrapperCalls implements GoWrapperResolver.
// Scans the code section for BL instructions targeting known Go syscall wrappers,
// then resolves the syscall number from the preceding X0/W0 register assignments.
// On arm64, all instructions are exactly 4 bytes. On decode failure, the scanner
// advances by 4 bytes (InstructionAlignment) to stay aligned.
func (r *ARM64GoWrapperResolver) FindWrapperCalls(code []byte, baseAddr uint64) ([]WrapperCall, int) {
	return r.findWrapperCalls(code, baseAddr, r.decoder)
}

// GetWrapperAddresses returns all known wrapper function addresses.
// This is primarily useful for testing.
func (r *ARM64GoWrapperResolver) GetWrapperAddresses() map[uint64]GoSyscallWrapper {
	return r.wrapperAddrs
}

// GetSymbols returns all loaded symbols.
// This is primarily useful for testing.
func (r *ARM64GoWrapperResolver) GetSymbols() map[string]SymbolInfo {
	return r.symbols
}
