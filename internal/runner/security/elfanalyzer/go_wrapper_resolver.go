package elfanalyzer

import (
	"debug/elf"
	"sort"
)

// Constants for backward scan
const (
	// minRecentInstructionsForScan is the minimum number of recent instructions
	// needed to attempt syscall argument resolution.
	minRecentInstructionsForScan = 2

	// maxBackwardScanSteps is the maximum number of instructions to scan backward
	// when resolving syscall arguments in wrapper calls.
	maxBackwardScanSteps = 6

	// maxRecentInstructionsToKeep is the maximum number of recent instructions
	// to keep in memory for backward scanning.
	maxRecentInstructionsToKeep = 10
)

// GoSyscallWrapper represents the name of a known Go syscall wrapper function.
// The zero value (NoWrapper) means no wrapper was found.
type GoSyscallWrapper string

// NoWrapper is the zero value of GoSyscallWrapper, indicating no wrapper was found.
const NoWrapper GoSyscallWrapper = ""

// knownGoWrappers is a set of known Go syscall wrapper function names for O(1) lookup.
// These are the public-facing wrapper functions that callers use to make syscalls.
// When a CALL to one of these is found in user code, the syscall number is resolved
// from the argument setup preceding the call.
var knownGoWrappers = map[GoSyscallWrapper]struct{}{
	"syscall.Syscall":     {},
	"syscall.Syscall6":    {},
	"syscall.RawSyscall":  {},
	"syscall.RawSyscall6": {},
	"runtime.syscall":     {},
	"runtime.syscall6":    {},
}

// knownSyscallImpls is a set of low-level syscall implementation function names
// whose bodies contain direct SYSCALL instructions with caller-supplied numbers.
// The syscall number cannot be determined statically from these functions' bodies,
// so both direct SYSCALL instructions and CALL-to-wrapper instructions within
// these functions are excluded from analysis.
var knownSyscallImpls = map[string]struct{}{
	"syscall.rawVforkSyscall":                 {},
	"syscall.rawSyscallNoError":               {},
	"internal/runtime/syscall/linux.Syscall6": {},
}

// SymbolInfo represents information about a symbol in the ELF file.
type SymbolInfo struct {
	Name    string
	Address uint64
	Size    uint64
}

// WrapperCall represents a call to a Go syscall wrapper function.
type WrapperCall struct {
	// CallSiteAddress is the address of the CALL instruction.
	CallSiteAddress uint64

	// TargetFunction is the name of the wrapper function being called.
	TargetFunction string

	// SyscallNumber is the resolved syscall number, or -1 if unresolved.
	SyscallNumber int

	// Resolved indicates whether the syscall number was successfully determined.
	Resolved bool

	// DeterminationMethod describes how the syscall number was determined,
	// or the reason it could not be determined.
	// See DeterminationMethod* constants in syscall_analyzer.go.
	DeterminationMethod string
}

// wrapperRange represents the address range [start, end) of a wrapper function.
type wrapperRange struct {
	start uint64
	end   uint64
}

// GoWrapperResolver analyzes indirect syscalls through Go syscall wrapper functions.
type GoWrapperResolver interface {
	// HasSymbols returns true if symbol information is available.
	HasSymbols() bool

	// FindWrapperCalls scans code for calls to known Go syscall wrapper functions.
	// Returns the list of detected wrapper calls and the number of decode failures.
	FindWrapperCalls(code []byte, baseAddr uint64) ([]WrapperCall, int)

	// IsInsideWrapper returns true if addr is within a known syscall wrapper
	// function body. Used to avoid recursive analysis.
	IsInsideWrapper(addr uint64) bool
}

// goWrapperBase holds symbol information shared by all GoWrapperResolver
// implementations. Concrete resolvers embed this struct.
type goWrapperBase struct {
	// symbols maps function name to SymbolInfo (address, size).
	symbols map[string]SymbolInfo

	// wrapperAddrs maps start address to GoSyscallWrapper name.
	wrapperAddrs map[uint64]GoSyscallWrapper

	// wrapperRanges is sorted by start address for binary search.
	wrapperRanges []wrapperRange

	// hasSymbols reports whether symbol loading succeeded.
	hasSymbols bool
}

// HasSymbols implements GoWrapperResolver.
func (b *goWrapperBase) HasSymbols() bool { return b.hasSymbols }

// IsInsideWrapper implements GoWrapperResolver.
// Returns true if addr falls within any known wrapper function's body.
// This is used to skip CALL instructions emitted from within a wrapper itself
// (e.g. syscall.Syscall calling an internal helper), which would otherwise be
// reported as unresolved wrapper calls and inflate the high-risk count.
//
// wrapperRanges is kept sorted by start address so this can use binary search
// (O(log n)) instead of a linear scan (O(n)).
func (b *goWrapperBase) IsInsideWrapper(addr uint64) bool {
	// Find the last range whose start <= addr.
	i := sort.Search(len(b.wrapperRanges), func(i int) bool {
		return b.wrapperRanges[i].start > addr
	}) - 1
	if i < 0 {
		return false
	}
	return addr < b.wrapperRanges[i].end
}

// loadFromPclntab parses the .gopclntab section and populates symbols,
// wrapperAddrs, and wrapperRanges.
func (b *goWrapperBase) loadFromPclntab(elfFile *elf.File) error {
	functions, err := ParsePclntab(elfFile)
	if err != nil {
		return err
	}

	for name, fn := range functions {
		// Calculate size, guarding against missing/zero End to avoid underflow
		size := uint64(0)
		if fn.End > fn.Entry {
			size = fn.End - fn.Entry
		}

		b.symbols[name] = SymbolInfo{
			Name:    name,
			Address: fn.Entry,
			Size:    size,
		}

		// Check if this is a known Go wrapper (exact match).
		// Go standard library syscall wrappers use stable, unqualified symbol names
		// (e.g. "syscall.Syscall") in pclntab, so exact match is sufficient.
		wrapper := GoSyscallWrapper(name)
		if _, ok := knownGoWrappers[wrapper]; ok {
			b.wrapperAddrs[fn.Entry] = wrapper
			if fn.End > fn.Entry {
				b.wrapperRanges = append(b.wrapperRanges, wrapperRange{
					start: fn.Entry,
					end:   fn.End,
				})
			}
		}

		// Also track low-level syscall implementation functions.
		// Their bodies contain direct SYSCALL instructions with caller-supplied numbers
		// and must be excluded from both Pass 1 and Pass 2 analysis.
		if _, ok := knownSyscallImpls[name]; ok {
			if fn.End > fn.Entry {
				b.wrapperRanges = append(b.wrapperRanges, wrapperRange{
					start: fn.Entry,
					end:   fn.End,
				})
			}
		}
	}

	// Sort wrapperRanges by start address so IsInsideWrapper can use binary search.
	sort.Slice(b.wrapperRanges, func(i, j int) bool {
		return b.wrapperRanges[i].start < b.wrapperRanges[j].start
	})

	return nil
}

// NewGoWrapperResolver creates a new X86GoWrapperResolver and loads symbols
// from the given ELF file's .gopclntab section.
//
// Deprecated: Use NewX86GoWrapperResolver directly.
// This function exists for backward compatibility.
func NewGoWrapperResolver(elfFile *elf.File) (*X86GoWrapperResolver, error) {
	return NewX86GoWrapperResolver(elfFile)
}
