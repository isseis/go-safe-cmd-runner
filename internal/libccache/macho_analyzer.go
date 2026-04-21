package libccache

import (
	"debug/macho"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sort"
)

// maxBackwardScanInstructions is the maximum number of instructions scanned backward from svc.
const maxBackwardScanInstructions = 16

// bsdSyscallClassPrefix is the macOS arm64 BSD syscall class prefix (0x2000000).
const bsdSyscallClassPrefix = 0x2000000

// svcMacOSEncoding is the little-endian uint32 encoding of the "svc #0x80" instruction.
// ARM64 encoding: 0xD4001001.
const svcMacOSEncoding = uint32(0xD4001001)

// machoSymTypeDebugMin is the minimum n_type value for debug/stab entries (N_STAB range).
// Symbols with type >= this value are compiler-generated debug entries and should be excluded.
const machoSymTypeDebugMin = uint8(0x20)

// arm64Imm16HighShift is the bit shift used to place a 16-bit immediate into the upper
// word when constructing a 32-bit value from MOVZ/MOVK LSL #16 instructions.
const arm64Imm16HighShift = 16

// ARM64 instruction pattern constants for control-flow detection.
const (
	// arm64PatternBLRBR matches bits [31:22] for BLR / BR instructions.
	arm64PatternBLRBR = uint32(0b1101011000) // 0x36C
	// arm64PatternRET matches bits [31:22] for RET instructions (bit 22 differs from BLR/BR).
	arm64PatternRET = uint32(0b1101011001) // 0x36D
	// arm64PatternCBZ matches bits [30:25] for CBZ / CBNZ instructions.
	arm64PatternCBZ = uint32(0b011010) // 0x1A
	// arm64PatternTBZ matches bits [30:25] for TBZ / TBNZ instructions.
	arm64PatternTBZ = uint32(0b011011) // 0x1B
	// arm64Field6Mask is a 6-bit mask used to extract a 6-bit field from an instruction.
	arm64Field6Mask = uint32(0x3F)
	// arm64PatternMovzMovkBits28_23 is the [28:23] pattern shared by MOVZ and MOVK.
	arm64PatternMovzMovkBits28_23 = uint32(0b100101) // 0x25
	// arm64BitShiftBLRBR is the right-shift to extract bits [31:22] for BLR/BR/RET.
	arm64BitShiftBLRBR = 22
	// arm64BitShiftCBZTBZ is the right-shift to extract bits [30:25] for CBZ/CBNZ/TBZ/TBNZ.
	arm64BitShiftCBZTBZ = 25
	// arm64BitShiftMovzMovk is the right-shift to extract bits [28:23] for MOVZ/MOVK.
	arm64BitShiftMovzMovk = 23
)

// MachoLibSystemAnalyzer analyzes a libsystem_kernel.dylib Mach-O file and returns
// a list of syscall wrapper functions.
type MachoLibSystemAnalyzer struct{}

// Analyze scans exported functions in machoFile and returns WrapperEntry values
// for functions recognized as syscall wrappers.
// For non-arm64 architectures, logs an info message and returns nil, nil.
// The returned slice is sorted by Number and then by Name.
func (a *MachoLibSystemAnalyzer) Analyze(machoFile *macho.File) ([]WrapperEntry, error) {
	if machoFile.Cpu != macho.CpuArm64 {
		slog.Info("Skipping libsystem_kernel.dylib analysis: not arm64",
			"cpu", fmt.Sprintf("%v", machoFile.Cpu))
		return nil, nil
	}

	// Get the __TEXT,__text section.
	textSection := machoFile.Section("__text")
	if textSection == nil || textSection.Seg != "__TEXT" {
		return nil, nil
	}
	code, err := textSection.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read __TEXT,__text section: %w", err)
	}
	textBase := textSection.Addr // Virtual address base.

	// Enumerate externally defined symbols from LC_SYMTAB.
	if machoFile.Symtab == nil {
		return nil, nil
	}

	// Sort by address to estimate function sizes.
	syms := filterFunctionSymbols(machoFile.Symtab.Syms)
	sort.Slice(syms, func(i, j int) bool {
		return syms[i].Value < syms[j].Value
	})

	textEnd := textBase + uint64(len(code))
	var entries []WrapperEntry

	for i, sym := range syms {
		// Estimate function size because Mach-O symtab has no st_size equivalent.
		// Skip aliases (syms[j].Value == sym.Value) to avoid zero-length bodies.
		var funcEnd uint64
		for j := i + 1; j < len(syms); j++ {
			if syms[j].Value > sym.Value {
				funcEnd = syms[j].Value
				break
			}
		}
		if funcEnd == 0 {
			funcEnd = textEnd
		}
		// Clamp the inferred end to the __TEXT,__text boundary so the last
		// in-range function is still analyzed even if the next symbol is in a
		// later section.
		if funcEnd > textEnd {
			funcEnd = textEnd
		}
		if sym.Value >= funcEnd || sym.Value < textBase {
			continue
		}
		funcSize := funcEnd - sym.Value

		// Real syscall wrappers are small; skip oversized functions.
		if funcSize > MaxWrapperFunctionSize {
			continue
		}

		// Slice out the function bytes.
		startOff := int(sym.Value - textBase) //nolint:gosec // #nosec G115 -- safe: sym.Value >= textBase verified above
		endOff := int(funcEnd - textBase)     //nolint:gosec // #nosec G115 -- safe: funcEnd <= textEnd verified above
		funcCode := code[startOff:endOff]

		// Detect svc #0x80 and resolve the BSD syscall number by scanning backward for x16 setup.
		number, ok := analyzeWrapperFunction(funcCode)
		if !ok {
			continue
		}
		entries = append(entries, WrapperEntry{Name: sym.Name, Number: number})
	}

	// Sort by Number then by Name for deterministic output.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Number != entries[j].Number {
			return entries[i].Number < entries[j].Number
		}
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// filterFunctionSymbols returns all section-defined symbols from Symtab.
// Local symbols are kept because they are needed for accurate function boundary
// detection: a local function between two exported functions would otherwise
// cause the exported function's size to be overestimated.
// macOS Mach-O symbol type flags:
//
//	N_TYPE = 0x0E (type mask)
//	N_SECT = 0x0E (defined in section)
//	N_UNDF = 0x00 (undefined)
func filterFunctionSymbols(syms []macho.Symbol) []macho.Symbol {
	var result []macho.Symbol
	for _, s := range syms {
		// Exclude undefined symbols (imports).
		if s.Sect == 0 {
			continue
		}
		// Exclude debug symbols (N_STAB: type >= machoSymTypeDebugMin).
		if s.Type >= machoSymTypeDebugMin {
			continue
		}
		result = append(result, s)
	}
	return result
}

// analyzeWrapperFunction analyzes funcCode, which contains one function body,
// and returns a single BSD syscall number. It returns (0, false) if the
// function contains no svc or if multiple distinct syscall numbers are found.
func analyzeWrapperFunction(funcCode []byte) (int, bool) {
	var foundNumbers []int
	const instrLen = 4

	for i := 0; i+instrLen <= len(funcCode); i += instrLen {
		word := binary.LittleEndian.Uint32(funcCode[i:])
		if word != svcMacOSEncoding {
			continue
		}
		// Found svc #0x80. Scan backward to find the immediate loaded into x16.
		num, ok := backwardScanX16(funcCode, i)
		if !ok {
			return 0, false
		}
		foundNumbers = append(foundNumbers, num)
	}

	if len(foundNumbers) == 0 {
		return 0, false
	}

	// A valid syscall wrapper calls exactly one syscall; reject functions with mixed numbers.
	first := foundNumbers[0]
	for _, n := range foundNumbers[1:] {
		if n != first {
			return 0, false
		}
	}
	return first, true
}

// backwardScanX16 walks backward from the svc #0x80 instruction at funcCode[svcOffset]
// and looks for an immediate-load sequence into x16. When found, it returns the
// syscall number with the BSD class prefix removed. The scan is limited to
// maxBackwardScanInstructions instructions.
//
// Supported instruction patterns:
//   - MOVZ X16, #imm                           (single-instruction: imm < 0x10000)
//   - MOVZ X16, #hi, LSL #16                   (upper 16 bits only)
//   - MOVZ X16, #hi, LSL #16 + MOVK X16, #lo  (32-bit value sequence)
//
// arm64asm is intentionally not used here. Only a small subset of fixed encodings
// is needed, and direct decoding keeps the dependency surface small.
func backwardScanX16(funcCode []byte, svcOffset int) (int, bool) {
	const instrLen = 4

	// MOVZ X16, #imm, LSL #shift encoding (ARM64):
	//   [31]:   sf=1 (64-bit)
	//   [30:29]: opc=10 (MOVZ)
	//   [28:23]: 100101
	//   [22:21]: hw (shift: 00=0, 01=16, 10=32, 11=48)
	//   [20:5]:  imm16
	//   [4:0]:   Rd=16 (x16)
	//
	// MOVK X16, #imm, LSL #shift encoding (ARM64):
	//   [31]:   sf=1
	//   [30:29]: opc=11 (MOVK)
	//   [28:23]: 100101
	//   [22:21]: hw
	//   [20:5]:  imm16
	//   [4:0]:   Rd=16 (x16)

	const (
		movzX16Base  = uint32(0xD2800010) // MOVZ X16, #0, LSL #0
		movzX16Lsl16 = uint32(0xD2A00010) // MOVZ X16, #0, LSL #16
		movkX16Base  = uint32(0xF2800010) // MOVK X16, #0, LSL #0
		movkX16Lsl16 = uint32(0xF2A00010) // MOVK X16, #0, LSL #16
		imm16Mask    = uint32(0x001FFFE0) // bits[20:5]
		imm16Shift   = 5
	)

	// Scan backward from the instruction immediately before svc.
	startIdx := svcOffset/instrLen - 1
	endIdx := startIdx - maxBackwardScanInstructions
	if endIdx < 0 {
		endIdx = -1
	}

	// Keep partial values so a MOVZ+MOVK sequence can be reconstructed.
	// -1 means "not observed yet".
	x16Lo := -1 // Lower 16 bits recorded from MOVK X16, #imm, LSL #0.
	x16Hi := -1 // Upper 16 bits recorded from MOVK X16, #imm, LSL #16.

	for i := startIdx; i > endIdx; i-- {
		off := i * instrLen
		if off < 0 {
			break
		}
		word := binary.LittleEndian.Uint32(funcCode[off:])

		// MOVZ X16, #imm (LSL #0) terminates the sequence and sets the low bits.
		// Combine it with a previously observed MOVK #hi if present.
		if word&^imm16Mask == movzX16Base {
			lo := int((word & imm16Mask) >> imm16Shift)
			hi := 0
			if x16Hi >= 0 {
				hi = x16Hi
			}
			return stripBSDPrefix(hi | lo), true
		}

		// MOVZ X16, #imm, LSL #16 terminates the sequence and sets the high bits.
		// Combine it with a previously observed MOVK #lo if present.
		if word&^imm16Mask == movzX16Lsl16 {
			hi := int((word&imm16Mask)>>imm16Shift) << arm64Imm16HighShift
			lo := 0
			if x16Lo >= 0 {
				lo = x16Lo
			}
			return stripBSDPrefix(hi | lo), true
		}

		// MOVK X16, #imm (LSL #0): record the low 16 bits and continue scanning.
		if word&^imm16Mask == movkX16Base {
			x16Lo = int((word & imm16Mask) >> imm16Shift)
			continue
		}

		// MOVK X16, #imm, LSL #16: record the high 16 bits and continue scanning.
		if word&^imm16Mask == movkX16Lsl16 {
			x16Hi = int((word&imm16Mask)>>imm16Shift) << arm64Imm16HighShift
			continue
		}

		// Stop when a control-flow instruction is reached.
		if isControlFlowInstruction(word) {
			break
		}

		// Stop when some other instruction writes to x16.
		if writesX16NotMovzMovk(word) {
			break
		}
	}
	return 0, false
}

// stripBSDPrefix removes the macOS BSD syscall class prefix (0x2000000) that ARM64
// wrappers encode in the high bits of x16 before the svc instruction.
func stripBSDPrefix(value int) int {
	if value >= bsdSyscallClassPrefix {
		return value - bsdSyscallClassPrefix
	}
	return value
}

// isControlFlowInstruction reports whether an ARM64 instruction is a control-flow instruction.
// It recognizes B / BL / BLR / BR / RET / CBZ / CBNZ / TBZ / TBNZ.
func isControlFlowInstruction(word uint32) bool {
	// B:  [31:26] = 000101
	// BL: [31:26] = 100101
	if word>>26 == 0b000101 || word>>26 == 0b100101 {
		return true
	}
	// BLR / BR / RET: [31:22] = arm64PatternBLRBR or arm64PatternRET
	if word>>arm64BitShiftBLRBR == arm64PatternBLRBR || word>>arm64BitShiftBLRBR == arm64PatternRET {
		return true
	}
	// CBZ / CBNZ: [30:25] = arm64PatternCBZ
	if (word>>arm64BitShiftCBZTBZ)&arm64Field6Mask == arm64PatternCBZ {
		return true
	}
	// TBZ / TBNZ: [30:25] = arm64PatternTBZ
	if (word>>arm64BitShiftCBZTBZ)&arm64Field6Mask == arm64PatternTBZ {
		return true
	}
	return false
}

// writesX16NotMovzMovk detects 64-bit instructions that write to x16,
// excluding MOVZ and MOVK.
// It is used after MOVZ/MOVK handling inside backwardScanX16.
//
// MOVZ/MOVK share a fixed [28:23] = 100101 pattern.
// When that pattern is present, this helper returns false because the caller
// already handled those instructions.
func writesX16NotMovzMovk(word uint32) bool {
	// MOVZ/MOVK: [28:23] = 100101, with Rd encoded in [4:0].
	// Reaching this point means either MOVZ/MOVK targeting another register
	// or a different encoding, so perform a generic x16-write check.
	bits28_23 := (word >> arm64BitShiftMovzMovk) & arm64Field6Mask
	if bits28_23 == arm64PatternMovzMovkBits28_23 {
		// MOVZ or MOVK encoding for some Rd: already handled by the caller.
		return false
	}
	// 64-bit instruction (sf=1) with Rd=16 ([4:0] = 0b10000).
	return word>>31 == 1 && word&0x1F == 0x10
}
