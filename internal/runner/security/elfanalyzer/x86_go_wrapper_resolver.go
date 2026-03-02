package elfanalyzer

import (
	"debug/elf"
	"fmt"
	"log/slog"
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
	r.hasSymbols = len(r.symbols) > 0
	return r, nil
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
// Scans the code section for calls to known Go syscall wrappers.
// This is a separate analysis pass from direct syscall instruction scanning.
//
// Parameters:
//   - code: the code section bytes
//   - baseAddr: base address of the code section
//
// Returns:
//   - slice of WrapperCall structs containing call site info and resolved syscall numbers
//   - decodeFailures: the number of instruction decode failures during scanning
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
	if len(r.wrapperAddrs) == 0 {
		return nil, 0
	}

	var results []WrapperCall
	decodeFailures := 0

	// Decode entire code section and find CALL instructions to known wrappers
	// Use the shared decoder instance (r.decoder) to avoid repeated allocation
	pos := 0
	var recentInstructions []DecodedInstruction

	for pos < len(code) {
		// pos is guaranteed non-negative (starts at 0, only incremented)
		// and less than len(code) (loop condition), so conversion is safe
		inst, err := r.decoder.Decode(code[pos:], baseAddr+uint64(pos)) //nolint:gosec // G115: pos is validated by loop condition
		if err != nil {
			decodeFailures++
			if decodeFailures <= MaxDecodeFailureLogs {
				slog.Debug("instruction decode failed in go wrapper resolver",
					slog.String("offset", fmt.Sprintf("0x%x", baseAddr+uint64(pos))), //nolint:gosec // G115: pos is validated by loop condition
					slog.String("bytes", fmt.Sprintf("%x", code[pos:min(pos+DecodeFailureLogBytesLen, len(code))])))
			}
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
		if target, ok := r.decoder.GetCallTarget(inst, inst.Offset); ok {
			callAddr := baseAddr + uint64(pos) //nolint:gosec // G115: pos is validated by loop condition
			// Skip CALL instructions that originate from inside a wrapper function body.
			// Wrapper functions (e.g. syscall.Syscall) may themselves call other wrappers
			// or internal helpers; those internal calls do not represent user-level
			// syscall usage and cannot have their syscall number resolved from context.
			if !r.IsInsideWrapper(callAddr) {
				if wrapper, ok := r.wrapperAddrs[target]; ok {
					// Found a call to a wrapper, try to resolve the syscall number
					syscallNum, method := r.resolveSyscallArgument(recentInstructions)
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
func (r *X86GoWrapperResolver) resolveSyscallArgument(recentInstructions []DecodedInstruction) (int, string) {
	if len(recentInstructions) < minRecentInstructionsForScan {
		return -1, DeterminationMethodUnknownDecodeFailed
	}

	// Scan backward through recent instructions (excluding the CALL itself)
	// Start from the instruction before the CALL (len-2) and scan up to maxBackwardScanSteps
	startIdx := len(recentInstructions) - minRecentInstructionsForScan
	minIdx := len(recentInstructions) - 1 - maxBackwardScanSteps
	for i := startIdx; i >= 0 && i >= minIdx; i-- {
		inst := recentInstructions[i]

		// Stop at control flow boundary
		if r.decoder.IsControlFlowInstruction(inst) {
			return -1, DeterminationMethodUnknownControlFlowBoundary
		}

		// Check for immediate move to first argument register (RAX/EAX for x86_64).
		// Note: Go compiler often generates "mov $N, %eax" (EAX) instead of
		// "mov $N, %rax" (RAX) for syscall numbers, so we must check both.
		if value, ok := r.decoder.IsImmediateToFirstArgRegister(inst); ok {
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

	return -1, DeterminationMethodUnknownScanLimitExceeded
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
