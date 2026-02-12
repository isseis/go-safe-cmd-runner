package elfanalyzer

import (
	"debug/elf"
	"errors"
	"fmt"
	"log/slog"
	"math"
)

// SyscallAnalysisResult represents the result of syscall analysis.
type SyscallAnalysisResult struct {
	// DetectedSyscalls contains all detected syscall events with their numbers.
	// This includes both direct syscall instructions (opcode 0F 05) and
	// indirect syscalls via Go wrapper function calls (e.g., syscall.Syscall).
	DetectedSyscalls []SyscallInfo

	// HasUnknownSyscalls indicates whether any syscall number could not be determined.
	HasUnknownSyscalls bool

	// HighRiskReasons explains why the analysis resulted in high risk, if applicable.
	HighRiskReasons []string

	// Summary provides aggregated information about the analysis.
	Summary SyscallSummary
}

// SyscallInfo represents information about a single detected syscall event.
// An event can be either a direct syscall instruction or an indirect syscall
// via a Go wrapper function call.
type SyscallInfo struct {
	// Number is the syscall number (e.g., 41 for socket on x86_64).
	// -1 indicates the number could not be determined.
	Number int `json:"number"`

	// Name is the human-readable syscall name (e.g., "socket").
	// Empty if the number is unknown or not in the table.
	Name string `json:"name,omitempty"`

	// IsNetwork indicates whether this syscall is network-related.
	IsNetwork bool `json:"is_network"`

	// Location is the virtual address of the syscall instruction
	// (typically located within the .text section).
	Location uint64 `json:"location"`

	// DeterminationMethod describes how the syscall number was determined.
	// See DeterminationMethod* constants for possible values.
	DeterminationMethod string `json:"determination_method"`
}

// SyscallSummary provides aggregated analysis information.
type SyscallSummary struct {
	// HasNetworkSyscalls indicates presence of network-related syscalls.
	HasNetworkSyscalls bool `json:"has_network_syscalls"`

	// IsHighRisk indicates the analysis could not fully determine network capability.
	IsHighRisk bool `json:"is_high_risk"`

	// TotalDetectedEvents is the count of detected syscall events.
	// This includes both direct syscall instructions and indirect syscalls
	// via Go wrapper function calls.
	TotalDetectedEvents int `json:"total_detected_events"`

	// NetworkSyscallCount is the count of network-related syscall events.
	NetworkSyscallCount int `json:"network_syscall_count"`
}

// maxInstructionLength is the maximum instruction length in bytes for x86_64.
const maxInstructionLength = 15

// defaultMaxBackwardScan is the default maximum number of instructions to scan
// backward from a syscall instruction to find the syscall number.
const defaultMaxBackwardScan = 50

// maxValidSyscallNumber is the maximum valid syscall number on x86_64.
// This is a conservative upper bound to filter out invalid immediates.
// Current x86_64 Linux syscalls range from 0-288, but we allow up to 500
// to account for future syscall additions and various kernel configurations.
const maxValidSyscallNumber = 500

// Determination method constants for syscall number extraction.
// These constants describe how the syscall number was determined during analysis.
const (
	// DeterminationMethodImmediate indicates the syscall number was determined
	// from an immediate value (e.g., mov eax, 42).
	DeterminationMethodImmediate = "immediate"

	// DeterminationMethodGoWrapper indicates the syscall number was determined
	// from a Go wrapper function call (e.g., syscall.Syscall).
	DeterminationMethodGoWrapper = "go_wrapper"

	// DeterminationMethodUnknownDecodeFailed indicates the syscall number
	// could not be determined because instruction decoding failed.
	DeterminationMethodUnknownDecodeFailed = "unknown:decode_failed"

	// DeterminationMethodUnknownControlFlowBoundary indicates the syscall number
	// could not be determined because a control flow boundary was encountered.
	DeterminationMethodUnknownControlFlowBoundary = "unknown:control_flow_boundary"

	// DeterminationMethodUnknownIndirectSetting indicates the syscall number
	// could not be determined because it was set indirectly (e.g., from a register or memory).
	DeterminationMethodUnknownIndirectSetting = "unknown:indirect_setting"

	// DeterminationMethodUnknownScanLimitExceeded indicates the syscall number
	// could not be determined because the backward scan limit was exceeded.
	DeterminationMethodUnknownScanLimitExceeded = "unknown:scan_limit_exceeded"

	// DeterminationMethodUnknownInvalidOffset indicates the syscall number
	// could not be determined because the offset was invalid.
	DeterminationMethodUnknownInvalidOffset = "unknown:invalid_offset"
)

// SyscallAnalyzer analyzes ELF binaries for syscall instructions.
//
// Security Note: This analyzer is designed to work with pre-opened *elf.File
// instances. The caller is responsible for opening files securely using
// safefileio.SafeOpenFile() followed by elf.NewFile(). This design ensures
// TOCTOU safety and symlink attack prevention, consistent with the existing
// StandardELFAnalyzer pattern.
type SyscallAnalyzer struct {
	decoder      MachineCodeDecoder
	syscallTable SyscallNumberTable
	goResolver   *GoWrapperResolver

	// maxBackwardScan is the maximum number of instructions to scan backward
	// from a syscall instruction to find the syscall number.
	maxBackwardScan int
}

// NewSyscallAnalyzer creates a new SyscallAnalyzer with default settings.
func NewSyscallAnalyzer() *SyscallAnalyzer {
	return &SyscallAnalyzer{
		decoder:         NewX86Decoder(),
		syscallTable:    NewX86_64SyscallTable(),
		goResolver:      NewGoWrapperResolver(),
		maxBackwardScan: defaultMaxBackwardScan,
	}
}

// NewSyscallAnalyzerWithConfig creates a SyscallAnalyzer with custom configuration.
// If a nil decoder or syscall table is provided, this function falls back to the
// default x86 decoder and x86_64 syscall table to avoid panics during analysis.
// If maxScan is non-positive, it is clamped to defaultMaxBackwardScan to keep
// backward scanning behavior predictable.
func NewSyscallAnalyzerWithConfig(decoder MachineCodeDecoder, table SyscallNumberTable, maxScan int) *SyscallAnalyzer {
	if decoder == nil {
		decoder = NewX86Decoder()
	}
	if table == nil {
		table = NewX86_64SyscallTable()
	}
	if maxScan <= 0 {
		maxScan = defaultMaxBackwardScan
	}
	return &SyscallAnalyzer{
		decoder:         decoder,
		syscallTable:    table,
		goResolver:      NewGoWrapperResolver(),
		maxBackwardScan: maxScan,
	}
}

// AnalyzeSyscallsFromELF analyzes the given ELF file for syscall instructions.
// Returns SyscallAnalysisResult containing all found syscalls and risk assessment.
//
// Note: This method accepts an *elf.File that has already been opened securely.
// The caller is responsible for using safefileio.SafeOpenFile() to prevent
// symlink attacks and TOCTOU race conditions, then wrapping with elf.NewFile().
// See StandardELFAnalyzer.AnalyzeNetworkSymbols() for the recommended pattern.
func (a *SyscallAnalyzer) AnalyzeSyscallsFromELF(elfFile *elf.File) (*SyscallAnalysisResult, error) {
	// Verify architecture
	if elfFile.Machine != elf.EM_X86_64 {
		return nil, &UnsupportedArchitectureError{
			Machine: elfFile.Machine,
		}
	}

	// Load .text section
	textSection := elfFile.Section(".text")
	if textSection == nil {
		return nil, ErrNoTextSection
	}

	code, err := textSection.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read .text section: %w", err)
	}

	// Load symbols for Go wrapper resolution.
	// Always reload symbols for each ELF file to avoid carrying over
	// stale wrapper addresses from a previously analyzed binary.
	if a.goResolver != nil {
		if err := a.goResolver.LoadSymbols(elfFile); err != nil && !errors.Is(err, ErrSymbolLoadingNotImplemented) {
			// Non-fatal: continue without Go wrapper resolution
			// This handles stripped binaries
			slog.Debug("failed to load symbols for Go wrapper resolution",
				slog.String("error", err.Error()))
		}
	}

	// Analyze syscalls
	return a.analyzeSyscallsInCode(code, textSection.Addr)
}

// analyzeSyscallsInCode performs the actual syscall analysis on code bytes.
// This method uses two separate analysis passes:
//  1. Direct syscall instruction analysis (syscall opcode 0F 05)
//  2. Go wrapper call analysis (calls to syscall.Syscall, etc.)
func (a *SyscallAnalyzer) analyzeSyscallsInCode(code []byte, baseAddr uint64) (*SyscallAnalysisResult, error) {
	result := &SyscallAnalysisResult{
		DetectedSyscalls: make([]SyscallInfo, 0),
	}

	// Pass 1: Analyze direct syscall instructions
	syscallLocs := a.findSyscallInstructions(code, baseAddr)
	for _, loc := range syscallLocs {
		info := a.extractSyscallInfo(code, loc, baseAddr)
		result.DetectedSyscalls = append(result.DetectedSyscalls, info)

		if info.Number == -1 {
			result.HasUnknownSyscalls = true
			result.HighRiskReasons = append(result.HighRiskReasons,
				fmt.Sprintf("syscall at 0x%x: number could not be determined (%s)",
					info.Location, info.DeterminationMethod))
		}

		if info.IsNetwork {
			result.Summary.NetworkSyscallCount++
		}
	}

	// Pass 2: Analyze Go wrapper calls (if symbols are available)
	if a.goResolver != nil {
		wrapperCalls := a.goResolver.FindWrapperCalls(code, baseAddr)
		for _, call := range wrapperCalls {
			info := SyscallInfo{
				Number:              call.SyscallNumber,
				Location:            call.CallSiteAddress,
				DeterminationMethod: DeterminationMethodGoWrapper,
			}

			if call.SyscallNumber >= 0 {
				info.Name = a.syscallTable.GetSyscallName(call.SyscallNumber)
				info.IsNetwork = a.syscallTable.IsNetworkSyscall(call.SyscallNumber)
			} else {
				result.HasUnknownSyscalls = true
				result.HighRiskReasons = append(result.HighRiskReasons,
					fmt.Sprintf("go wrapper call at 0x%x: syscall number could not be determined",
						call.CallSiteAddress))
			}

			result.DetectedSyscalls = append(result.DetectedSyscalls, info)

			if info.IsNetwork {
				result.Summary.NetworkSyscallCount++
			}
		}
	}

	// Build summary with consistent field calculation rules:
	// - TotalDetectedEvents: total count of all detected syscall events (Pass 1 + Pass 2)
	// - HasNetworkSyscalls: true if NetworkSyscallCount > 0
	// - IsHighRisk: true if HasUnknownSyscalls (any syscall number could not be determined)
	// - NetworkSyscallCount: incremented during Pass 1 and Pass 2
	// These rules ensure convertSyscallResult() in StandardELFAnalyzer correctly
	// interprets the analysis result for network capability detection.
	result.Summary.TotalDetectedEvents = len(result.DetectedSyscalls)
	result.Summary.HasNetworkSyscalls = result.Summary.NetworkSyscallCount > 0
	result.Summary.IsHighRisk = result.HasUnknownSyscalls

	return result, nil
}

// findSyscallInstructions scans the code for syscall instructions (0F 05).
// This method decodes instructions at proper boundaries to avoid false positives
// from 0F 05 bytes appearing inside other instructions or immediate values.
func (a *SyscallAnalyzer) findSyscallInstructions(code []byte, baseAddr uint64) []uint64 {
	var locations []uint64
	pos := 0

	for pos < len(code) {
		// Validate pos is non-negative before converting to uint64 to prevent overflow.
		if pos < 0 {
			break
		}
		inst, err := a.decoder.Decode(code[pos:], baseAddr+uint64(pos)) // #nosec G115 safe: pos is checked to be non-negative above
		if err != nil {
			// Skip problematic byte and continue
			pos++
			continue
		}
		// Defensively guard against a decoder returning a non-positive length.
		if inst.Len <= 0 {
			pos++
			continue
		}

		// Check if this is a syscall instruction at proper instruction boundary.
		// Verify both the instruction length (2 bytes) and the actual opcode bytes.
		if inst.Len == 2 && pos+1 < len(code) && code[pos] == 0x0F && code[pos+1] == 0x05 {
			locations = append(locations, inst.Offset)
		}

		pos += inst.Len
	}

	return locations
}

// extractSyscallInfo extracts syscall number by backward scanning.
func (a *SyscallAnalyzer) extractSyscallInfo(code []byte, syscallAddr uint64, baseAddr uint64) SyscallInfo {
	info := SyscallInfo{
		Number:   -1,
		Location: syscallAddr,
	}

	// Calculate offset in code.
	// NOTE: syscallAddr and baseAddr are uint64, so we must avoid unsigned
	// underflow and ensure the result fits into an int before converting.
	if syscallAddr < baseAddr {
		info.DeterminationMethod = DeterminationMethodUnknownInvalidOffset
		return info
	}
	delta := syscallAddr - baseAddr
	// The syscall instruction is 2 bytes. We must ensure the offset is valid
	// and there's enough room to read the instruction.
	// A check against math.MaxInt is included to satisfy gosec's requirement
	// for safe uint64 to int conversion, although it's logically redundant
	// since len(code) is an int.
	if delta > uint64(math.MaxInt) || int(delta) > len(code)-2 {
		info.DeterminationMethod = DeterminationMethodUnknownInvalidOffset
		return info
	}
	offset := int(delta)

	// Backward scan to find eax/rax modification
	number, method := a.backwardScanForSyscallNumber(code, baseAddr, offset)
	info.Number = number
	info.DeterminationMethod = method

	if number >= 0 {
		info.Name = a.syscallTable.GetSyscallName(number)
		info.IsNetwork = a.syscallTable.IsNetworkSyscall(number)
	}

	return info
}

// backwardScanForSyscallNumber scans backward from syscall instruction
// to find where eax/rax is set.
// Note: This method only handles direct syscall instructions.
// Go wrapper calls (e.g., Go's syscall wrappers) are handled separately
// via goResolver.FindWrapperCalls.
func (a *SyscallAnalyzer) backwardScanForSyscallNumber(code []byte, baseAddr uint64, syscallOffset int) (int, string) {
	// Performance optimization: Use windowed decoding to avoid re-decoding
	// the entire .text section for each syscall instruction.
	// Window starts from max(0, syscallOffset - maxBackwardScan * maxInstructionLength)
	windowStart := syscallOffset - (a.maxBackwardScan * maxInstructionLength)
	if windowStart < 0 {
		windowStart = 0
	}

	// Build instruction list by forward decoding within the window
	instructions := a.decodeInstructionsInWindow(code, baseAddr, windowStart, syscallOffset)
	if len(instructions) == 0 {
		return -1, DeterminationMethodUnknownDecodeFailed
	}

	// Scan backward through decoded instructions
	scanCount := 0
	for i := len(instructions) - 1; i >= 0 && scanCount < a.maxBackwardScan; i-- {
		inst := instructions[i]
		scanCount++

		// Check for control flow instruction (basic block boundary)
		if a.decoder.IsControlFlowInstruction(inst) {
			return -1, DeterminationMethodUnknownControlFlowBoundary
		}

		// Check if this instruction modifies eax/rax
		if !a.decoder.ModifiesEAXorRAX(inst) {
			continue
		}

		// Check if it's an immediate move
		if isImm, value := a.decoder.IsImmediateMove(inst); isImm {
			// Validate immediate value is a valid syscall number.
			// Reject negative immediates (e.g., 0xffffffff as -1) and out-of-range values.
			// This prevents inconsistency where Number=-1 (unknown sentinel) with
			// DeterminationMethodImmediate could indicate a successful decode of an invalid value.
			if value >= 0 && value <= maxValidSyscallNumber {
				return int(value), DeterminationMethodImmediate
			}
			// Immediate value is out of valid range; treat as indirect setting
			return -1, DeterminationMethodUnknownIndirectSetting
		}

		// Non-immediate modification found (register move, memory load, etc.)
		return -1, DeterminationMethodUnknownIndirectSetting
	}

	// Reached scan limit without finding eax/rax modification
	return -1, DeterminationMethodUnknownScanLimitExceeded
}

// decodeInstructionsInWindow decodes instructions within a specified window [startOffset, endOffset).
// This method provides better performance by avoiding unnecessary decoding of the entire code section.
// For large binaries with many syscall instructions, this reduces total decode overhead significantly.
//
// Parameters:
//   - code: the code section bytes
//   - baseAddr: base virtual address of the code section (used to compute instruction VAs)
//   - startOffset, endOffset: section-relative byte offsets defining the decode window
//
// Instruction boundary handling:
// The startOffset may not align with an instruction boundary (since we calculate it by
// subtracting a fixed byte count from syscallOffset). When decoding fails at startOffset,
// we skip one byte (pos++) and retry. This "resynchronization" approach works because:
//  1. x86_64 instruction encoding is self-synchronizing within a few bytes
//  2. We decode forward toward syscallOffset which IS a known instruction boundary
//  3. Even if initial instructions are mis-decoded, the final instructions before
//     syscallOffset will be correct (they align with the known syscall instruction)
//  4. We only need the last few instructions for backward scan, not the entire window
//
// In practice, resynchronization typically occurs within 1-3 bytes for x86_64 code.
// The worst case (15 bytes of invalid decodes) is rare and doesn't affect correctness
// since we scan backward from the end of the decoded instruction list.
//
// Performance comparison (example: 10MB .text, 100 syscalls):
//   - Old approach: 100 * 5MB avg = ~500MB worth of redundant decoding
//   - Window approach: 100 * (50 instructions * 15 bytes) = ~75KB of focused decoding
func (a *SyscallAnalyzer) decodeInstructionsInWindow(code []byte, baseAddr uint64, startOffset, endOffset int) []DecodedInstruction {
	var instructions []DecodedInstruction
	pos := startOffset

	for pos < endOffset {
		// Slice input to [pos:endOffset] to prevent decoding beyond window boundary.
		// This ensures the decoder cannot consume bytes past endOffset (e.g., the syscall instruction itself).
		// Validate pos is non-negative before converting to uint64 to prevent overflow.
		if pos < 0 {
			break
		}
		inst, err := a.decoder.Decode(code[pos:endOffset], baseAddr+uint64(pos)) // #nosec G115 safe: pos is checked to be non-negative above
		if err != nil {
			// Skip problematic byte and continue
			pos++
			continue
		}
		// Defensively guard against a decoder returning a non-positive length.
		// This prevents an infinite loop if a buggy or mocked MachineCodeDecoder
		// produces an instruction with Len <= 0 while pos < endOffset.
		if inst.Len <= 0 {
			pos++
			continue
		}
		instructions = append(instructions, inst)
		pos += inst.Len
	}

	return instructions
}
