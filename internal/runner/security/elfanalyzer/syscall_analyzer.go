package elfanalyzer

import (
	"debug/elf"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// MaxDecodeFailureLogs is the maximum number of individual decode failure
// log messages to emit per analysis. This prevents excessive log output
// for binaries with many decode failures (e.g., binaries containing
// large data sections interleaved with code).
const MaxDecodeFailureLogs = 10

// SyscallAnalysisResult represents the result of syscall analysis.
type SyscallAnalysisResult struct {
	// SyscallAnalysisResultCore contains the common fields shared with
	// fileanalysis.SyscallAnalysisResult. Embedding ensures field-level
	// consistency between packages and enables direct struct copy for
	// type conversion.
	common.SyscallAnalysisResultCore

	// DecodeStats contains statistics about instruction decoding.
	// These are populated during analysis and intended for diagnostic
	// logging by the caller (e.g., record command).
	DecodeStats DecodeStatistics
}

// DecodeStatistics contains instruction decode failure statistics.
// This is used for diagnostic logging, not for risk assessment.
// Decode failures do not affect risk classification (see §8.5 / §9.1.2).
type DecodeStatistics struct {
	// DecodeFailureCount is the total number of instruction decode failures
	// across all passes (Pass 1: findSyscallInstructions,
	// Pass 2: FindWrapperCalls).
	DecodeFailureCount int `json:"-"`

	// TotalBytesAnalyzed is the total number of bytes in the .text section.
	TotalBytesAnalyzed int `json:"-"`
}

// SyscallInfo is an alias for common.SyscallInfo.
// Using a type alias preserves backward compatibility for code that references
// elfanalyzer.SyscallInfo while the canonical definition lives in common.
type SyscallInfo = common.SyscallInfo

// maxInstructionLength is the maximum instruction length in bytes for x86_64.
const maxInstructionLength = 15

// DecodeFailureLogBytesLen is the number of leading bytes to include
// in decode-failure log messages for diagnostic purposes.
const DecodeFailureLogBytesLen = 4

// defaultMaxBackwardScan is the default maximum number of instructions to scan
// backward from a syscall instruction. Applied to both syscall number extraction
// and syscall argument evaluation (e.g., mprotect prot flag).
const defaultMaxBackwardScan = 50

// maxValidSyscallNumber is the maximum valid syscall number on x86_64.
// This is a conservative upper bound to filter out invalid immediates.
// Current x86_64 Linux syscalls range up to 461 (lsm_list_modules, as of the
// syscall table in this repo), but we allow up to 500 to account for future
// syscall additions and various kernel configurations.
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
	// could not be determined because the backward scan step limit was reached
	// before exhausting all decoded instructions in the window.
	DeterminationMethodUnknownScanLimitExceeded = "unknown:scan_limit_exceeded"

	// DeterminationMethodUnknownWindowExhausted indicates the syscall number
	// could not be determined because all decoded instructions in the scan
	// window were examined without finding a register-modifying instruction.
	// Unlike scan_limit_exceeded, the scan consumed the entire available window.
	DeterminationMethodUnknownWindowExhausted = "unknown:window_exhausted"

	// DeterminationMethodUnknownInvalidOffset indicates the syscall number
	// could not be determined because the offset was invalid.
	DeterminationMethodUnknownInvalidOffset = "unknown:invalid_offset"
)

// archConfig holds architecture-specific components for syscall analysis.
type archConfig struct {
	decoder              MachineCodeDecoder
	syscallTable         SyscallNumberTable
	archName             string
	newGoWrapperResolver func(*elf.File) (GoWrapperResolver, error)
}

// SyscallAnalyzer analyzes ELF binaries for syscall instructions.
//
// Security Note: This analyzer is designed to work with pre-opened *elf.File
// instances. The caller is responsible for opening files securely using
// safefileio.SafeOpenFile() followed by elf.NewFile(). This design ensures
// TOCTOU safety and symlink attack prevention, consistent with the existing
// StandardELFAnalyzer pattern.
type SyscallAnalyzer struct {
	// archConfigs maps ELF machine type to architecture-specific components.
	archConfigs map[elf.Machine]*archConfig

	// maxBackwardScan is the maximum number of instructions to scan backward
	// from a syscall instruction to find the syscall number.
	maxBackwardScan int
}

// NewSyscallAnalyzer creates a new SyscallAnalyzer with x86_64 and arm64 support.
func NewSyscallAnalyzer() *SyscallAnalyzer {
	a := &SyscallAnalyzer{
		archConfigs:     make(map[elf.Machine]*archConfig),
		maxBackwardScan: defaultMaxBackwardScan,
	}
	a.archConfigs[elf.EM_X86_64] = &archConfig{
		decoder:      NewX86Decoder(),
		syscallTable: NewX86_64SyscallTable(),
		archName:     "x86_64",
		newGoWrapperResolver: func(f *elf.File) (GoWrapperResolver, error) {
			return NewX86GoWrapperResolver(f)
		},
	}
	a.archConfigs[elf.EM_AARCH64] = &archConfig{
		decoder:      NewARM64Decoder(),
		syscallTable: NewARM64LinuxSyscallTable(),
		archName:     "arm64",
		newGoWrapperResolver: func(f *elf.File) (GoWrapperResolver, error) {
			return NewARM64GoWrapperResolver(f)
		},
	}
	return a
}

// NewSyscallAnalyzerWithConfig creates a SyscallAnalyzer with custom configuration.
// The provided decoder and table are registered for elf.EM_X86_64.
// This is primarily used for testing with mock decoder/table.
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
	a := &SyscallAnalyzer{
		archConfigs:     make(map[elf.Machine]*archConfig),
		maxBackwardScan: maxScan,
	}
	a.archConfigs[elf.EM_X86_64] = &archConfig{
		decoder:      decoder,
		syscallTable: table,
		archName:     "x86_64",
		newGoWrapperResolver: func(f *elf.File) (GoWrapperResolver, error) {
			return NewX86GoWrapperResolver(f)
		},
	}
	return a
}

// AnalyzeSyscallsFromELF analyzes the given ELF file for syscall instructions.
// Returns SyscallAnalysisResult containing all found syscalls and risk assessment.
//
// Note: This method accepts an *elf.File that has already been opened securely.
// The caller is responsible for using safefileio.SafeOpenFile() to prevent
// symlink attacks and TOCTOU race conditions, then wrapping with elf.NewFile().
// See StandardELFAnalyzer.AnalyzeNetworkSymbols() for the recommended pattern.
func (a *SyscallAnalyzer) AnalyzeSyscallsFromELF(elfFile *elf.File) (*SyscallAnalysisResult, error) {
	// Look up arch config for this ELF's machine type.
	cfg, ok := a.archConfigs[elfFile.Machine]
	if !ok {
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

	// Create a fresh GoWrapperResolver for this ELF file.
	// A new instance is created per call to guarantee no stale state
	// carries over between different binaries.
	goResolver, err := cfg.newGoWrapperResolver(elfFile)
	if err != nil {
		// Non-fatal: continue with a no-op resolver.
		// ErrNoPclntab is expected for non-Go or stripped binaries; log at Debug.
		// Other errors (malformed pclntab) are unexpected; log at Warn.
		if errors.Is(err, ErrNoPclntab) {
			slog.Debug("no .gopclntab section, skipping Go wrapper analysis",
				slog.String("arch", cfg.archName))
		} else {
			slog.Warn("GoWrapperResolver init failed, continuing without wrapper analysis",
				slog.String("arch", cfg.archName),
				slog.String("error", err.Error()))
		}
		goResolver = newNoopGoWrapperResolver()
	}

	// Analyze syscalls
	result := a.analyzeSyscallsInCode(code, textSection.Addr, cfg.decoder, cfg.syscallTable, goResolver)
	result.Architecture = cfg.archName
	return result, nil
}

// AnalyzeSyscallsInRange analyzes syscall instructions in code[startOffset:endOffset].
// sectionBaseAddr is the virtual address of the start of code.
// The slice code[startOffset:endOffset] is passed to findSyscallInstructions with
// a shifted base address (sectionBaseAddr + startOffset), so backwardScanForSyscallNumber's
// existing max(windowStart, 0) clamp handles the slice boundary without additional clamping.
// Go wrapper analysis (Pass 2) is not performed.
// Returns *UnsupportedArchitectureError (detectable via errors.As) for unsupported architectures.
func (a *SyscallAnalyzer) AnalyzeSyscallsInRange(
	code []byte,
	sectionBaseAddr uint64,
	startOffset, endOffset int,
	machine elf.Machine,
) ([]common.SyscallInfo, error) {
	cfg, ok := a.archConfigs[machine]
	if !ok {
		return nil, &UnsupportedArchitectureError{Machine: machine}
	}

	subCode := code[startOffset:endOffset]
	subBase := sectionBaseAddr + uint64(startOffset) //nolint:gosec // G115: startOffset is validated by caller (symbol range within ELF section)

	syscallLocs, _ := a.findSyscallInstructions(subCode, subBase, cfg.decoder)
	results := make([]common.SyscallInfo, 0, len(syscallLocs))
	for _, loc := range syscallLocs {
		info := a.extractSyscallInfo(subCode, loc, subBase, cfg.decoder, cfg.syscallTable)
		results = append(results, info)
	}
	return results, nil
}

// GetSyscallTable returns the SyscallNumberTable for the given machine architecture.
// Returns (table, true) for supported architectures, (nil, false) for unsupported ones.
// Used by Validator to obtain the architecture-specific table for ImportSymbolMatcher.
func (a *SyscallAnalyzer) GetSyscallTable(machine elf.Machine) (SyscallNumberTable, bool) {
	cfg, ok := a.archConfigs[machine]
	if !ok {
		return nil, false
	}
	return cfg.syscallTable, true
}

// analyzeSyscallsInCode performs the actual syscall analysis on code bytes.
// This method uses two separate analysis passes:
//  1. Direct syscall instruction analysis (architecture-specific: SYSCALL on x86_64, SVC #0 on arm64)
//  2. Go wrapper call analysis (calls to syscall.Syscall, etc.)
//
// goResolver may be nil if symbol loading failed or was not attempted.
func (a *SyscallAnalyzer) analyzeSyscallsInCode(code []byte, baseAddr uint64, decoder MachineCodeDecoder, table SyscallNumberTable, goResolver GoWrapperResolver) *SyscallAnalysisResult {
	result := &SyscallAnalysisResult{}
	result.DetectedSyscalls = make([]common.SyscallInfo, 0)

	// Pass 1: Analyze direct syscall instructions
	syscallLocs, pass1DecodeFailures := a.findSyscallInstructions(code, baseAddr, decoder)
	result.DecodeStats.DecodeFailureCount += pass1DecodeFailures
	result.DecodeStats.TotalBytesAnalyzed = len(code)
	for _, loc := range syscallLocs {
		// Skip direct syscall instructions that fall inside known wrapper/impl functions.
		// These functions receive the syscall number from their caller and the number
		// cannot be determined statically from the function body.
		if goResolver != nil && goResolver.IsInsideWrapper(loc) {
			continue
		}
		info := a.extractSyscallInfo(code, loc, baseAddr, decoder, table)
		result.DetectedSyscalls = append(result.DetectedSyscalls, info)

		if info.Number == -1 {
			result.AnalysisWarnings = append(result.AnalysisWarnings,
				fmt.Sprintf("syscall at 0x%x: number could not be determined (%s)",
					info.Location, info.DeterminationMethod))
		}
	}

	// Pass 2: Analyze Go wrapper calls (if symbols are available)
	if goResolver != nil {
		wrapperCalls, pass2DecodeFailures := goResolver.FindWrapperCalls(code, baseAddr)
		result.DecodeStats.DecodeFailureCount += pass2DecodeFailures
		for _, call := range wrapperCalls {
			info := common.SyscallInfo{
				Number:              call.SyscallNumber,
				Location:            call.CallSiteAddress,
				DeterminationMethod: call.DeterminationMethod,
			}

			if call.SyscallNumber >= 0 {
				info.Name = table.GetSyscallName(call.SyscallNumber)
				info.IsNetwork = table.IsNetworkSyscall(call.SyscallNumber)
			} else {
				result.AnalysisWarnings = append(result.AnalysisWarnings,
					fmt.Sprintf("go wrapper call at 0x%x: %s",
						call.CallSiteAddress, call.DeterminationMethod))
			}

			result.DetectedSyscalls = append(result.DetectedSyscalls, info)
		}
	}

	evalResults := a.evaluateMprotectFamilyArgs(
		code, baseAddr, decoder, result.DetectedSyscalls,
	)
	for _, eval := range evalResults {
		result.ArgEvalResults = append(result.ArgEvalResults, eval.Result)

		if EvalMprotectRisk([]common.SyscallArgEvalResult{eval.Result}) {
			// Add analysis warning message
			switch eval.Result.Status {
			case common.SyscallArgEvalExecConfirmed:
				result.AnalysisWarnings = append(result.AnalysisWarnings,
					fmt.Sprintf("%s at 0x%x: PROT_EXEC confirmed (%s)",
						eval.Result.SyscallName, eval.Location, eval.Result.Details))
			case common.SyscallArgEvalExecUnknown:
				result.AnalysisWarnings = append(result.AnalysisWarnings,
					fmt.Sprintf("%s at 0x%x: PROT_EXEC could not be ruled out (%s)",
						eval.Result.SyscallName, eval.Location, eval.Result.Details))
			}
		}
	}

	return result
}

// protExecFlag is the PROT_EXEC flag value (0x4) used in mprotect syscall.
// See: https://man7.org/linux/man-pages/man2/mprotect.2.html
const protExecFlag = 0x4

// riskPriority constants for comparing SyscallArgEvalStatus severity.
const (
	riskPriorityExecConfirmed = 2
	riskPriorityExecUnknown   = 1
	riskPriorityExecNotSet    = 0
)

// syscall names for the mprotect family.
const (
	syscallNameMprotect     = "mprotect"
	syscallNamePkeyMprotect = "pkey_mprotect"
)

// MprotectFamilyNames lists the syscall names in the mprotect family.
// Each name is processed independently to produce at most one ArgEvalResult per name.
var MprotectFamilyNames = []string{syscallNameMprotect, syscallNamePkeyMprotect}

type mprotectFamilyEvalResult struct {
	Result   common.SyscallArgEvalResult
	Location uint64
}

// evaluateMprotectFamilyArgs evaluates the prot argument for each syscall in the
// mprotect family (mprotect and pkey_mprotect).
// It returns a slice of evaluation results, where each entry contains the
// highest-risk SyscallArgEvalResult for a detected family member and its
// corresponding syscall instruction address.
// Syscall family members that were not detected are omitted.
func (a *SyscallAnalyzer) evaluateMprotectFamilyArgs(
	code []byte,
	baseAddr uint64,
	decoder MachineCodeDecoder,
	detectedSyscalls []common.SyscallInfo,
) []mprotectFamilyEvalResult {
	var results []mprotectFamilyEvalResult

	for _, syscallName := range MprotectFamilyNames {
		// Collect entries for this syscall name.
		// Only consider entries determined by "immediate" method, as those
		// have confirmed syscall numbers.
		var entries []common.SyscallInfo
		for _, info := range detectedSyscalls {
			if info.Name == syscallName &&
				info.DeterminationMethod == DeterminationMethodImmediate {
				entries = append(entries, info)
			}
		}

		if len(entries) == 0 {
			continue
		}

		// Evaluate each entry and select the highest risk.
		// Priority: exec_confirmed > exec_unknown > exec_not_set
		var bestResult common.SyscallArgEvalResult
		hasBestResult := false
		var bestLocation uint64

		for _, entry := range entries {
			result := a.evalSingleMprotect(code, baseAddr, decoder, entry, syscallName)

			if !hasBestResult || riskPriority(result.Status) > riskPriority(bestResult.Status) {
				bestResult = result
				hasBestResult = true
				bestLocation = entry.Location
			}
		}

		results = append(results, mprotectFamilyEvalResult{
			Result:   bestResult,
			Location: bestLocation,
		})
	}

	return results
}

// evalSingleMprotect evaluates the prot argument of a single mprotect-family entry.
// syscallName is used as the SyscallName field in the returned result (e.g., "mprotect"
// or "pkey_mprotect"). The evaluation logic is identical for all family members since
// prot is always the third argument (rdx on x86_64, x2 on arm64).
func (a *SyscallAnalyzer) evalSingleMprotect(
	code []byte,
	baseAddr uint64,
	decoder MachineCodeDecoder,
	entry common.SyscallInfo,
	syscallName string,
) common.SyscallArgEvalResult {
	offset, ok := validateSyscallOffset(entry.Location, baseAddr, len(code))
	if !ok {
		return common.SyscallArgEvalResult{
			SyscallName: syscallName,
			Status:      common.SyscallArgEvalExecUnknown,
			Details:     "invalid offset",
		}
	}

	value, method := a.backwardScanForRegister(
		code, baseAddr, offset, decoder,
		decoder.ModifiesThirdArgRegister,
		decoder.IsImmediateToThirdArgRegister,
	)

	if method == DeterminationMethodImmediate {
		if value&protExecFlag != 0 {
			return common.SyscallArgEvalResult{
				SyscallName: syscallName,
				Status:      common.SyscallArgEvalExecConfirmed,
				Details:     fmt.Sprintf("prot=0x%x", value),
			}
		}
		return common.SyscallArgEvalResult{
			SyscallName: syscallName,
			Status:      common.SyscallArgEvalExecNotSet,
			Details:     fmt.Sprintf("prot=0x%x", value),
		}
	}

	// Map determination method to exec_unknown details.
	details := unknownMethodDetail(method)

	return common.SyscallArgEvalResult{
		SyscallName: syscallName,
		Status:      common.SyscallArgEvalExecUnknown,
		Details:     details,
	}
}

// riskPriority returns the priority of a SyscallArgEvalStatus.
// Higher value = higher risk.
func riskPriority(status common.SyscallArgEvalStatus) int {
	switch status {
	case common.SyscallArgEvalExecConfirmed:
		return riskPriorityExecConfirmed
	case common.SyscallArgEvalExecUnknown:
		return riskPriorityExecUnknown
	case common.SyscallArgEvalExecNotSet:
		return riskPriorityExecNotSet
	default:
		return -1
	}
}

// unknownMethodDetail converts unknown:* determination methods to
// compact, stable detail strings for ArgEvalResults.
func unknownMethodDetail(method string) string {
	switch method {
	case DeterminationMethodUnknownDecodeFailed:
		return "decode failed"
	case DeterminationMethodUnknownControlFlowBoundary:
		return "control flow boundary"
	case DeterminationMethodUnknownIndirectSetting:
		return "indirect register setting"
	case DeterminationMethodUnknownScanLimitExceeded:
		return "scan limit exceeded"
	case DeterminationMethodUnknownWindowExhausted:
		return "window exhausted"
	default:
		return "unknown reason"
	}
}

// validateSyscallOffset converts an absolute address to a section-relative offset,
// validating that the address is within the code section with at least a small
// margin from the end (a conservative sanity check; the decoder enforces the
// exact per-architecture minimum instruction size separately).
// Returns (offset, true) on success, or (-1, false) if the address is out of bounds.
func validateSyscallOffset(location, baseAddr uint64, codeLen int) (int, bool) {
	if location < baseAddr {
		return -1, false
	}
	delta := location - baseAddr
	if delta > uint64(math.MaxInt) || int(delta) > codeLen-2 {
		return -1, false
	}
	return int(delta), true
}

// maxWindowBytesPerInstruction returns the number of bytes to allocate per
// instruction in the backward scan window.
func maxWindowBytesPerInstruction(decoder MachineCodeDecoder) int {
	return decoder.MaxInstructionLength()
}

// findSyscallInstructions scans the code for syscall instructions.
// Decode failures during instruction-boundary scanning are counted in
// decodeFailureCount and logged up to maxDecodeFailureLogs times via slog.Debug.
func (a *SyscallAnalyzer) findSyscallInstructions(code []byte, baseAddr uint64, decoder MachineCodeDecoder) ([]uint64, int) {
	var locations []uint64
	decodeFailures := 0
	pos := 0

	for pos < len(code) {
		// Validate pos is non-negative before converting to uint64 to prevent overflow.
		if pos < 0 {
			break
		}
		inst, err := decoder.Decode(code[pos:], baseAddr+uint64(pos)) // #nosec G115 safe: pos is checked to be non-negative above
		if err != nil {
			decodeFailures++
			if decodeFailures <= MaxDecodeFailureLogs {
				slog.Debug("instruction decode failed",
					slog.String("offset", fmt.Sprintf("0x%x", baseAddr+uint64(pos))),                                // #nosec G115 safe: pos is checked non-negative above
					slog.String("bytes", fmt.Sprintf("%x", code[pos:min(pos+DecodeFailureLogBytesLen, len(code))]))) //nolint:gosec // G115: pos is validated above
			}
			// Skip by alignment granularity on decode failure
			pos += decoder.InstructionAlignment()
			continue
		}

		// Decoder invariant: successful decode must have positive length.
		// If this fails, it indicates a programming bug in the decoder implementation.
		if inst.Len <= 0 {
			panic("decoder returned non-positive instruction length without error")
		}

		// Check if this is a syscall instruction using the architecture-specific decoder.
		if decoder.IsSyscallInstruction(inst) {
			locations = append(locations, inst.Offset)
		}

		pos += inst.Len
	}

	return locations, decodeFailures
}

// extractSyscallInfo extracts syscall number by backward scanning.
func (a *SyscallAnalyzer) extractSyscallInfo(code []byte, syscallAddr uint64, baseAddr uint64, decoder MachineCodeDecoder, table SyscallNumberTable) common.SyscallInfo {
	info := common.SyscallInfo{
		Number:   -1,
		Location: syscallAddr,
	}

	offset, ok := validateSyscallOffset(syscallAddr, baseAddr, len(code))
	if !ok {
		info.DeterminationMethod = DeterminationMethodUnknownInvalidOffset
		return info
	}

	// Backward scan to find syscall number register modification
	number, method := a.backwardScanForSyscallNumber(code, baseAddr, offset, decoder)
	info.Number = number
	info.DeterminationMethod = method

	if number >= 0 {
		info.Name = table.GetSyscallName(number)
		info.IsNetwork = table.IsNetworkSyscall(number)
	}

	return info
}

// backwardScanForRegister is a generalized backward scan that extracts an
// immediate value from a target register. modifiesReg and isImmediateToReg
// are decoder methods specifying which register to track.
//
// Returns:
//   - value: the immediate value found, or -1 if not found
//   - method: the determination method string describing the result
func (a *SyscallAnalyzer) backwardScanForRegister(
	code []byte,
	baseAddr uint64,
	syscallOffset int,
	decoder MachineCodeDecoder,
	modifiesReg func(DecodedInstruction) bool,
	isImmediateToReg func(DecodedInstruction) (bool, int64),
) (value int64, method string) {
	// Window calculation identical to backwardScanForSyscallNumber
	windowStart := syscallOffset - (a.maxBackwardScan * maxWindowBytesPerInstruction(decoder))
	if windowStart < 0 {
		windowStart = 0
	}

	instructions, _ := a.decodeInstructionsInWindow(
		code, baseAddr, windowStart, syscallOffset, decoder,
	)
	if len(instructions) == 0 {
		return -1, DeterminationMethodUnknownDecodeFailed
	}

	scanCount := 0
	for i := len(instructions) - 1; i >= 0 && scanCount < a.maxBackwardScan; i-- {
		inst := instructions[i]
		scanCount++

		if decoder.IsControlFlowInstruction(inst) {
			return -1, DeterminationMethodUnknownControlFlowBoundary
		}

		if !modifiesReg(inst) {
			continue
		}

		if isImm, val := isImmediateToReg(inst); isImm {
			return val, DeterminationMethodImmediate
		}

		return -1, DeterminationMethodUnknownIndirectSetting
	}

	// Distinguish between exhausting the scan window (all decoded instructions
	// examined) and hitting the step limit (more instructions may exist beyond
	// the window but were not decoded).
	if scanCount < a.maxBackwardScan {
		return -1, DeterminationMethodUnknownWindowExhausted
	}
	return -1, DeterminationMethodUnknownScanLimitExceeded
}

// backwardScanForSyscallNumber scans backward from syscall instruction
// to find where the syscall number register is set.
// Note: This method only handles direct syscall instructions.
// Go wrapper calls (e.g., Go's syscall wrappers) are handled separately
// via goResolver.FindWrapperCalls.
func (a *SyscallAnalyzer) backwardScanForSyscallNumber(code []byte, baseAddr uint64, syscallOffset int, decoder MachineCodeDecoder) (int, string) {
	value, method := a.backwardScanForRegister(
		code, baseAddr, syscallOffset, decoder,
		decoder.ModifiesSyscallNumberRegister,
		decoder.IsImmediateToSyscallNumberRegister,
	)

	if method == DeterminationMethodImmediate {
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

	return int(value), method
}

// decodeInstructionsInWindow decodes instructions within a specified window [startOffset, endOffset).
// This method provides better performance by avoiding unnecessary decoding of the entire code section.
// For large binaries with many syscall instructions, this reduces total decode overhead significantly.
//
// Parameters:
//   - code: the code section bytes
//   - baseAddr: base virtual address of the code section (used to compute instruction VAs)
//   - startOffset, endOffset: section-relative byte offsets defining the decode window
//   - decoder: the architecture-specific decoder to use
//
// Instruction boundary handling:
// The startOffset may not align with an instruction boundary (since we calculate it by
// subtracting a fixed byte count from syscallOffset). When decoding fails at startOffset,
// we skip by InstructionAlignment() bytes and retry. This "resynchronization" approach
// works because:
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
func (a *SyscallAnalyzer) decodeInstructionsInWindow(code []byte, baseAddr uint64, startOffset, endOffset int, decoder MachineCodeDecoder) ([]DecodedInstruction, int) {
	var instructions []DecodedInstruction
	decodeFailures := 0
	pos := startOffset

	for pos < endOffset {
		// Slice input to [pos:endOffset] to prevent decoding beyond window boundary.
		// This ensures the decoder cannot consume bytes past endOffset (e.g., the syscall instruction itself).
		// Validate pos is non-negative before converting to uint64 to prevent overflow.
		if pos < 0 {
			break
		}
		inst, err := decoder.Decode(code[pos:endOffset], baseAddr+uint64(pos)) // #nosec G115 safe: pos is checked to be non-negative above
		if err != nil {
			decodeFailures++
			// Skip by alignment granularity on decode failure
			pos += decoder.InstructionAlignment()
			continue
		}

		// Decoder invariant: successful decode must have positive length.
		// If this fails, it indicates a programming bug in the decoder implementation.
		if inst.Len <= 0 {
			panic("decoder returned non-positive instruction length without error")
		}

		instructions = append(instructions, inst)
		pos += inst.Len
	}

	return instructions, decodeFailures
}
