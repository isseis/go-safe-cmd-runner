package elfanalyzer

import (
	"debug/elf"
)

// X86GoWrapperResolver implements GoWrapperResolver for x86_64 binaries.
type X86GoWrapperResolver struct {
	goWrapperBase
	decoder *X86Decoder // Shared decoder instance to avoid repeated allocation
}

// NewX86GoWrapperResolver creates a new X86GoWrapperResolver and loads symbols
// from the given ELF file's .gopclntab section.
//
// Returns an error if symbol loading fails (e.g., missing .gopclntab).
// Even on error, the returned resolver is safe to use; it simply has no
// symbols loaded and FindWrapperCalls will return nil.
func NewX86GoWrapperResolver(elfFile *elf.File) (*X86GoWrapperResolver, error) {
	r := newX86GoWrapperResolver()
	if err := r.loadFromPclntab(elfFile); err != nil {
		return r, err
	}
	r.decoder.SetDataSections(loadX86DataSections(elfFile))
	r.hasSymbols = len(r.symbols) > 0
	return r, nil
}

// loadX86DataSections reads ELF sections that may contain package-level
// variables used as syscall numbers (e.g. syscall.fcntl64Syscall).
func loadX86DataSections(elfFile *elf.File) []x86DataSection {
	sectionNames := []string{".noptrdata", ".rodata", ".data"}
	sections := make([]x86DataSection, 0, len(sectionNames))
	for _, name := range sectionNames {
		sec := elfFile.Section(name)
		if sec == nil {
			continue
		}
		data, err := sec.Data()
		if err != nil || len(data) == 0 {
			continue
		}
		sections = append(sections, x86DataSection{Addr: sec.Addr, Data: data})
	}
	return sections
}

// newX86GoWrapperResolver creates an empty X86GoWrapperResolver without loading symbols.
// This is used internally and by tests that set up symbols manually.
func newX86GoWrapperResolver() *X86GoWrapperResolver {
	return &X86GoWrapperResolver{
		goWrapperBase: goWrapperBase{
			symbols:      make(map[string]SymbolInfo),
			wrapperAddrs: make(map[uint64]GoSyscallWrapper),
		},
		decoder: NewX86Decoder(),
	}
}

// FindWrapperCalls implements GoWrapperResolver.
// Scans the code section for CALL instructions to known Go syscall wrappers,
// then resolves the syscall number from the preceding RAX/EAX register assignments.
//
// Performance Note:
// This function performs linear decoding of the entire code section, unlike
// Pass 1 (findSyscallInstructions) which uses window-based scanning.
// For typical static Go binaries (1-10 MB code section), linear decoding
// completes in approximately 50-200ms, which is acceptable for the record
// command's batch processing use case.
// Future optimization: If performance becomes an issue for very large binaries,
// consider implementing window-based scanning similar to Pass 1, but this adds
// complexity for maintaining CALL instruction context for backward scanning.
// See NFR-4.1.2 for performance requirements.
func (r *X86GoWrapperResolver) FindWrapperCalls(code []byte, baseAddr uint64) ([]WrapperCall, int) {
	return r.findWrapperCalls(code, baseAddr, r.decoder)
}

// GetWrapperAddresses returns all known wrapper function addresses.
// This is primarily useful for testing.
func (r *X86GoWrapperResolver) GetWrapperAddresses() map[uint64]GoSyscallWrapper {
	return r.wrapperAddrs
}

// GetSymbols returns all loaded symbols.
// This is primarily useful for testing.
func (r *X86GoWrapperResolver) GetSymbols() map[string]SymbolInfo {
	return r.symbols
}
