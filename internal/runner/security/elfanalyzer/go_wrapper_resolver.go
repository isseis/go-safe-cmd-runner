package elfanalyzer

import (
	"debug/elf"
	"math"

	"golang.org/x/arch/x86/x86asm"
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
var knownGoWrappers = map[GoSyscallWrapper]struct{}{
	"syscall.Syscall":     {},
	"syscall.Syscall6":    {},
	"syscall.RawSyscall":  {},
	"syscall.RawSyscall6": {},
	"runtime.syscall":     {},
	"runtime.syscall6":    {},
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

// GoWrapperResolver resolves Go syscall wrapper calls to determine syscall numbers.
type GoWrapperResolver struct {
	symbols      map[string]SymbolInfo
	wrapperAddrs map[uint64]GoSyscallWrapper
	hasSymbols   bool
	decoder      *X86Decoder // Shared decoder instance to avoid repeated allocation
}

// NewGoWrapperResolver creates a new GoWrapperResolver and loads symbols
// from the given ELF file's .gopclntab section.
//
// Returns an error if symbol loading fails (e.g., missing .gopclntab).
// Even on error, the returned resolver is safe to use; it simply has no
// symbols loaded and FindWrapperCalls will return nil.
func NewGoWrapperResolver(elfFile *elf.File) (*GoWrapperResolver, error) {
	r := newGoWrapperResolver()
	if err := r.loadFromPclntab(elfFile); err != nil {
		return r, err
	}
	r.hasSymbols = len(r.symbols) > 0
	return r, nil
}

// newGoWrapperResolver creates an empty GoWrapperResolver without loading symbols.
// This is used internally and by tests that set up symbols manually.
func newGoWrapperResolver() *GoWrapperResolver {
	return &GoWrapperResolver{
		symbols:      make(map[string]SymbolInfo),
		wrapperAddrs: make(map[uint64]GoSyscallWrapper),
		decoder:      NewX86Decoder(),
	}
}

// loadFromPclntab loads symbols from the .gopclntab section.
func (r *GoWrapperResolver) loadFromPclntab(elfFile *elf.File) error {
	result, err := ParsePclntab(elfFile)
	if err != nil {
		return err
	}

	for name, fn := range result.Functions {
		// Calculate size, guarding against missing/zero End to avoid underflow
		size := uint64(0)
		if fn.End > fn.Entry {
			size = fn.End - fn.Entry
		}

		r.symbols[name] = SymbolInfo{
			Name:    name,
			Address: fn.Entry,
			Size:    size,
		}

		// Check if this is a known Go wrapper (exact match).
		// Go standard library syscall wrappers use stable, unqualified symbol names
		// (e.g. "syscall.Syscall") in pclntab, so exact match is sufficient.
		wrapper := GoSyscallWrapper(name)
		if _, ok := knownGoWrappers[wrapper]; ok {
			r.wrapperAddrs[fn.Entry] = wrapper
		}
	}

	return nil
}

// HasSymbols returns true if symbols were successfully loaded.
func (r *GoWrapperResolver) HasSymbols() bool {
	return r.hasSymbols
}

// FindWrapperCalls scans the code section for calls to known Go syscall wrappers.
// This is a separate analysis pass from direct syscall instruction scanning.
//
// Parameters:
//   - code: the code section bytes
//   - baseAddr: base address of the code section
//
// Returns:
//   - slice of WrapperCall structs containing call site info and resolved syscall numbers
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
func (r *GoWrapperResolver) FindWrapperCalls(code []byte, baseAddr uint64) []WrapperCall {
	if len(r.wrapperAddrs) == 0 {
		return nil
	}

	var results []WrapperCall

	// Decode entire code section and find CALL instructions to known wrappers
	// Use the shared decoder instance (r.decoder) to avoid repeated allocation
	pos := 0
	var recentInstructions []DecodedInstruction

	for pos < len(code) {
		// pos is guaranteed non-negative (starts at 0, only incremented)
		// and less than len(code) (loop condition), so conversion is safe
		inst, err := r.decoder.Decode(code[pos:], baseAddr+uint64(pos)) //nolint:gosec // G115: pos is validated by loop condition
		if err != nil {
			pos++
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

		// Check if this is a CALL to a known wrapper
		if inst.Op == x86asm.CALL {
			if wrapper := r.resolveWrapper(inst); wrapper != NoWrapper {
				// Found a call to a wrapper, try to resolve the syscall number
				syscallNum, method := r.resolveSyscallArgument(recentInstructions)
				// pos is validated same as above
				results = append(results, WrapperCall{
					CallSiteAddress:     baseAddr + uint64(pos), //nolint:gosec // G115: pos is validated by loop condition
					TargetFunction:      string(wrapper),
					SyscallNumber:       syscallNum,
					Resolved:            syscallNum >= 0,
					DeterminationMethod: method,
				})
			}
		}

		pos += inst.Len
	}

	return results
}

// resolveSyscallArgument analyzes instructions before a wrapper call
// to determine the syscall number argument.
//
// Returns the syscall number and a DeterminationMethod string describing
// how the result was obtained. On failure, returns -1 and one of the
// DeterminationMethodUnknown* constants indicating the reason.
//
// LIMITATION: This implementation ONLY supports Go 1.17+ register-based ABI
// where the first argument (syscall number) is passed in RAX.
// This is a known limited specification:
//   - Go 1.16 and earlier use stack-based ABI (not supported)
//   - Compiler optimizations or unusual wrapper patterns may place the
//     syscall number in a different register or via memory indirection
//   - Calls where the syscall number is not resolved are reported as
//     unknown (SyscallNumber = -1), triggering High Risk classification
//
// The target Go version should be fixed and validated with acceptance
// tests using real Go binaries compiled with the specific Go toolchain.
func (r *GoWrapperResolver) resolveSyscallArgument(recentInstructions []DecodedInstruction) (int, string) {
	if len(recentInstructions) < minRecentInstructionsForScan {
		return -1, DeterminationMethodUnknownDecodeFailed
	}

	// Scan backward through recent instructions (excluding the CALL itself)
	// Use the shared decoder instance (r.decoder) to avoid repeated allocation
	// Start from the instruction before the CALL (len-2) and scan up to maxBackwardScanSteps
	startIdx := len(recentInstructions) - minRecentInstructionsForScan
	minIdx := len(recentInstructions) - 1 - maxBackwardScanSteps
	for i := startIdx; i >= 0 && i >= minIdx; i-- {
		inst := recentInstructions[i]

		// Stop at control flow boundary
		if r.decoder.IsControlFlowInstruction(inst) {
			return -1, DeterminationMethodUnknownControlFlowBoundary
		}

		// Check for immediate move to target register
		// Note: Go compiler often generates "mov $N, %eax" (x86asm.EAX) instead of
		// "mov $N, %rax" (x86asm.RAX) for syscall numbers, so we must check both.
		if isImm, value := r.decoder.IsImmediateMove(inst); isImm {
			if reg, ok := inst.Args[0].(x86asm.Reg); ok && (reg == x86asm.RAX || reg == x86asm.EAX) {
				// Validate immediate value is a plausible syscall number.
				// Reject negative immediates and out-of-range values to prevent
				// incorrect marking of wrapper calls as resolved.
				if value >= 0 && value <= maxValidSyscallNumber {
					return int(value), DeterminationMethodGoWrapper
				}
				// Immediate value is out of valid range; treat as indirect setting
				return -1, DeterminationMethodUnknownIndirectSetting
			}
		}
	}

	return -1, DeterminationMethodUnknownScanLimitExceeded
}

// resolveWrapper checks if the instruction is a CALL to a known wrapper
// and returns the wrapper name if found, or NoWrapper otherwise.
func (r *GoWrapperResolver) resolveWrapper(inst DecodedInstruction) GoSyscallWrapper {
	if inst.Op != x86asm.CALL {
		return NoWrapper
	}

	// Extract call target
	if len(inst.Args) == 0 {
		return NoWrapper
	}

	// For direct calls, check if target is a known wrapper
	// Only handle relative calls (x86asm.Rel type)
	target, ok := inst.Args[0].(x86asm.Rel)
	if !ok {
		return NoWrapper
	}

	// Relative call - calculate absolute address.
	// nextPC is the address of the instruction following the CALL.
	// target (x86asm.Rel / int32) is the signed displacement from nextPC.
	//
	// Defense-in-depth overflow prevention:
	// 1. Guard against overflow in nextPC calculation
	// 2. Guard against negative displacement result (invalid address)
	// 3. Ensure final address is in valid user-space range
	//
	// In practice x86_64 user-space addresses are always < 2^47 (canonical),
	// so these are defensive checks rather than reachable code paths.

	// Check: inst.Offset + inst.Len won't overflow uint64
	// inst.Len is typically ≤15 for x86-64, so this is extremely unlikely
	if inst.Offset > math.MaxUint64-uint64(inst.Len) { //nolint:gosec // G115: Len validated non-negative
		return NoWrapper
	}
	nextPC := inst.Offset + uint64(inst.Len) //nolint:gosec // G115: Overflow checked above

	// Check: nextPC fits in int64 for signed displacement calculation
	if nextPC > uint64(math.MaxInt64) {
		return NoWrapper
	}

	// Check: signed displacement doesn't result in negative address
	// target is x86asm.Rel (int32), so int64 conversion is safe
	displacement := int64(nextPC) + int64(target)
	if displacement < 0 {
		return NoWrapper
	}

	targetAddr := uint64(displacement)
	return r.wrapperAddrs[targetAddr]
}

// GetWrapperAddresses returns all known wrapper function addresses.
// This is primarily useful for testing.
func (r *GoWrapperResolver) GetWrapperAddresses() map[uint64]GoSyscallWrapper {
	return r.wrapperAddrs
}

// GetSymbols returns all loaded symbols.
// This is primarily useful for testing.
func (r *GoWrapperResolver) GetSymbols() map[string]SymbolInfo {
	return r.symbols
}
