package elfanalyzer

import (
	"debug/elf"
	"fmt"
	"log/slog"
	"sort"
)

// Constants for backward scan
const (
	// minRecentInstructionsForScan is the minimum number of recent instructions
	// needed to attempt syscall argument resolution.
	minRecentInstructionsForScan = 2

	// maxRecentInstructionsToKeep is the maximum number of recent instructions
	// to keep in memory for backward scanning.
	maxRecentInstructionsToKeep = 12

	// maxBackwardScanSteps is the maximum number of instructions to scan backward
	// when resolving syscall arguments in wrapper calls.
	// Set to maxRecentInstructionsToKeep - 1 so the entire available window is
	// searched (the most recent slot holds the call instruction itself, so at most
	// maxRecentInstructionsToKeep-1 pre-call instructions are available).
	maxBackwardScanSteps = maxRecentInstructionsToKeep - 1
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
//
// Note on pclntab symbol naming: pclntab records the Go function name without
// ABI suffixes (e.g. "internal/runtime/syscall.Syscall6", not ".abi0").
// The ".abi0" suffix only appears in the ELF symbol table (.symtab/.dynsym).
// Since loadFromPclntab reads from pclntab, all entries here must use the
// plain Go function name without ABI suffixes.
var knownSyscallImpls = map[string]struct{}{
	"syscall.rawVforkSyscall":                 {},
	"syscall.rawSyscallNoError":               {},
	"internal/runtime/syscall/linux.Syscall6": {}, // Go 1.22 and earlier / x86_64
	"internal/runtime/syscall.Syscall6":       {}, // Go 1.23+ / arm64 (pclntab name, no .abi0 suffix)
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

// findWrapperCalls is the shared implementation of FindWrapperCalls for all
// GoWrapperResolver implementations. It decodes the entire code section and
// collects calls to known Go syscall wrappers.
func (b *goWrapperBase) findWrapperCalls(code []byte, baseAddr uint64, decoder MachineCodeDecoder) ([]WrapperCall, int) {
	if len(b.wrapperAddrs) == 0 {
		return nil, 0
	}

	var results []WrapperCall
	decodeFailures := 0

	pos := 0
	var recentInstructions []DecodedInstruction

	for pos < len(code) {
		// pos is guaranteed non-negative (starts at 0, only incremented)
		// and less than len(code) (loop condition), so conversion is safe
		inst, err := decoder.Decode(code[pos:], baseAddr+uint64(pos)) //nolint:gosec // G115: pos is validated by loop condition
		if err != nil {
			decodeFailures++
			if decodeFailures <= MaxDecodeFailureLogs {
				slog.Debug("instruction decode failed in go wrapper resolver",
					slog.String("offset", fmt.Sprintf("0x%x", baseAddr+uint64(pos))), //nolint:gosec // G115: pos is validated by loop condition
					slog.String("bytes", fmt.Sprintf("%x", code[pos:min(pos+DecodeFailureLogBytesLen, len(code))])))
			}
			pos += decoder.InstructionAlignment()
			continue
		}

		// Decoder invariant: successful decode must have positive length.
		// If this fails, it indicates a programming bug in the decoder implementation.
		if inst.Len <= 0 {
			panic("decoder returned non-positive instruction length without error")
		}

		// Keep track of recent instructions for backward scanning
		recentInstructions = append(recentInstructions, inst)
		if len(recentInstructions) > maxRecentInstructionsToKeep {
			recentInstructions = recentInstructions[1:]
		}

		// Check if this is a call instruction targeting a known wrapper
		if target, ok := decoder.GetCallTarget(inst, inst.Offset); ok {
			callAddr := baseAddr + uint64(pos) //nolint:gosec // G115: pos is validated by loop condition
			// Skip call instructions that originate from inside a wrapper function body.
			// Wrapper functions (e.g. syscall.Syscall) may themselves call other wrappers
			// or internal helpers; those internal calls do not represent user-level
			// syscall usage and cannot have their syscall number resolved from context.
			if !b.IsInsideWrapper(callAddr) {
				if wrapper, ok := b.wrapperAddrs[target]; ok {
					syscallNum, method := b.resolveSyscallArgument(recentInstructions, decoder)
					results = append(results, WrapperCall{
						CallSiteAddress:     callAddr,
						TargetFunction:      string(wrapper),
						SyscallNumber:       syscallNum,
						Resolved:            syscallNum >= 0,
						DeterminationMethod: method,
					})
				}
			}
		}

		pos += inst.Len
	}

	return results, decodeFailures
}

// resolveSyscallArgument scans recent instructions backward from a call to
// determine the syscall number passed as the first argument.
//
// Returns the syscall number and a DeterminationMethod string describing
// how the result was obtained. On failure, returns -1 and one of the
// DeterminationMethodUnknown* constants indicating the reason.
func (b *goWrapperBase) resolveSyscallArgument(recentInstructions []DecodedInstruction, decoder MachineCodeDecoder) (int, string) {
	if len(recentInstructions) < minRecentInstructionsForScan {
		return -1, DeterminationMethodUnknownDecodeFailed
	}

	// Scan backward through recent instructions (excluding the call itself).
	// Start from the instruction before the call (len-2) and scan up to maxBackwardScanSteps.
	startIdx := len(recentInstructions) - minRecentInstructionsForScan
	scanCount := 0
	for i := startIdx; i >= 0 && scanCount < maxBackwardScanSteps; i-- {
		scanCount++
		inst := recentInstructions[i]

		// Check for immediate move to first argument register.
		if value, ok := decoder.IsImmediateToFirstArgRegister(inst); ok {
			// Validate immediate value is a plausible syscall number.
			// Reject negative immediates and out-of-range values to prevent
			// incorrect marking of wrapper calls as resolved.
			if value >= 0 && value <= maxValidSyscallNumber {
				return int(value), DeterminationMethodGoWrapper
			}
			// Immediate value is out of valid range; treat as indirect setting
			return -1, DeterminationMethodUnknownIndirectSetting
		}

		// Try resolving first argument from a global-data load pattern
		// (e.g., ADRP+LDR on arm64).
		if value, ok := decoder.TryResolveFirstArgFromGlobalLoad(recentInstructions, i); ok {
			if value >= 0 && value <= maxValidSyscallNumber {
				return int(value), DeterminationMethodGoWrapper
			}
			return -1, DeterminationMethodUnknownIndirectSetting
		}

		// Any non-immediate write to the first argument register means
		// the value is set indirectly and cannot be resolved statically.
		if decoder.ModifiesFirstArgRegister(inst) {
			return -1, DeterminationMethodUnknownIndirectSetting
		}

		// Stop at control flow boundary.
		if decoder.IsControlFlowInstruction(inst) {
			return -1, DeterminationMethodUnknownControlFlowBoundary
		}
	}

	// Distinguish between exhausting all available instructions (window exhausted)
	// and hitting the step limit (more instructions may precede the window).
	if scanCount < maxBackwardScanSteps {
		return -1, DeterminationMethodUnknownWindowExhausted
	}
	return -1, DeterminationMethodUnknownScanLimitExceeded
}

// noopGoWrapperResolver is a no-op implementation of GoWrapperResolver.
// It is used as a fallback when GoWrapperResolver initialization fails
// (e.g., missing .gopclntab section in a stripped binary).
type noopGoWrapperResolver struct{}

func newNoopGoWrapperResolver() *noopGoWrapperResolver {
	return &noopGoWrapperResolver{}
}

// HasSymbols returns false, indicating no symbols are available.
func (n *noopGoWrapperResolver) HasSymbols() bool { return false }

// FindWrapperCalls returns nil and 0, performing no analysis.
func (n *noopGoWrapperResolver) FindWrapperCalls(_ []byte, _ uint64) ([]WrapperCall, int) {
	return nil, 0
}

// IsInsideWrapper returns false for all addresses.
func (n *noopGoWrapperResolver) IsInsideWrapper(_ uint64) bool { return false }
