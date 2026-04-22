package machoanalyzer

import (
	"encoding/binary"
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// knownMachoSyscallImpls is the set of known Go syscall stub function names
// whose bodies contain direct svc #0x80 with caller-supplied syscall numbers.
// These stubs are excluded from Pass 1 and their call sites are analyzed by Pass 2.
var knownMachoSyscallImpls = map[string]struct{}{
	"syscall.Syscall":                   {},
	"syscall.Syscall6":                  {},
	"syscall.RawSyscall":                {},
	"syscall.RawSyscall6":               {},
	"internal/runtime/syscall.Syscall6": {},
}

// DeterminationMethod constants for Mach-O syscall analysis.
// Values match the corresponding elfanalyzer constants for cross-architecture consistency.
const (
	determinationMethodImmediate       = "immediate"                // elfanalyzer.DeterminationMethodImmediate
	determinationMethodGoWrapper       = "go_wrapper"               // elfanalyzer.DeterminationMethodGoWrapper
	determinationMethodUnknownIndirect = "unknown:indirect_setting" // elfanalyzer.DeterminationMethodUnknownIndirectSetting
)

// syscallNumberTable provides syscall name and network-risk classification by
// BSD syscall number.
// Structurally identical to libccache.SyscallNumberTable and
// filevalidator.SyscallNumberTable; defined here to avoid an import cycle:
//
//	machoanalyzer → libccache → filevalidator → machoanalyzer
type syscallNumberTable interface {
	GetSyscallName(number int) string
	IsNetworkSyscall(number int) bool
}

// buildStubRanges builds a sorted slice of address ranges for known Go syscall
// stub functions from the pclntab function map. The ranges are used in Pass 1
// to exclude svc #0x80 instructions inside those stubs.
func buildStubRanges(funcs map[string]MachoPclntabFunc) []funcRange {
	var ranges []funcRange
	for name, fn := range funcs {
		if _, ok := knownMachoSyscallImpls[name]; ok {
			if fn.End > fn.Entry {
				ranges = append(ranges, funcRange{start: fn.Entry, end: fn.End})
			}
		}
	}
	// Sort by start address for binary search in isInsideRange.
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start < ranges[j].start
	})
	return ranges
}

// scanSVCWithX16 performs Pass 1 analysis: scans svc #0x80 addresses, skips
// those inside known Go syscall stub address ranges, and resolves the X16
// syscall number via backward scan.
//
//   - svcAddrs: virtual addresses of svc #0x80 instructions (from collectSVCAddresses)
//   - code:     raw bytes of __TEXT,__text section
//   - textBase: virtual address of the section start
//   - stubRanges: address ranges of known Go syscall stub functions (from pclntab)
//   - table: macOS BSD syscall table for name/network lookups
//
// Returns one SyscallInfo per svc #0x80 that was NOT excluded.
func scanSVCWithX16(
	svcAddrs []uint64,
	code []byte,
	textBase uint64, //nolint:unparam // textBase will vary in production use
	stubRanges []funcRange,
	table syscallNumberTable,
) []common.SyscallInfo {
	var results []common.SyscallInfo

	for _, addr := range svcAddrs {
		// Skip svc instructions inside known Go stub ranges (handled by Pass 2).
		if isInsideRange(addr, stubRanges) {
			continue
		}

		if addr < textBase {
			continue
		}
		svcOffset := int(addr - textBase) //nolint:gosec // G115: addr >= textBase verified above

		num, ok := arm64BackwardScanX16(code, svcOffset)

		var info common.SyscallInfo
		if ok {
			info = common.SyscallInfo{
				Number:              num,
				Name:                table.GetSyscallName(num),
				IsNetwork:           table.IsNetworkSyscall(num),
				Location:            addr,
				DeterminationMethod: determinationMethodImmediate,
				Source:              "", // Mach-O direct svc entries have empty Source (same as ELF)
			}
		} else {
			info = common.SyscallInfo{
				Number:              -1,
				Name:                "",
				IsNetwork:           false,
				Location:            addr,
				DeterminationMethod: determinationMethodUnknownIndirect,
				Source:              "",
			}
		}
		results = append(results, info)
	}

	return results
}

// arm64BackwardScanX16 mirrors libccache.BackwardScanX16.
// Duplicated here to avoid an import cycle:
//
//	machoanalyzer → libccache → filevalidator → machoanalyzer
//
// When libccache.BackwardScanX16 is updated, this function must be updated too.
//
// arm64BackwardScanX16 walks backward from the svc #0x80 instruction at
// code[svcOffset] and looks for an immediate-load sequence into x16.
// When found, it returns the syscall number with the BSD class prefix removed.
// The scan is limited to maxX16BackwardScan instructions.
func arm64BackwardScanX16(code []byte, svcOffset int) (int, bool) {
	const (
		instrLen  = 4
		maxScan   = 16
		bsdPrefix = 0x2000000

		movzX16Base  = uint32(0xD2800010) // MOVZ X16, #0, LSL #0
		movzX16Lsl16 = uint32(0xD2A00010) // MOVZ X16, #0, LSL #16
		movkX16Base  = uint32(0xF2800010) // MOVK X16, #0, LSL #0
		movkX16Lsl16 = uint32(0xF2A00010) // MOVK X16, #0, LSL #16
		imm16Mask    = uint32(0x001FFFE0) // bits[20:5]
		imm16Shift   = 5
		imm16HiShift = 16 // for LSL#16 variants

	)

	startIdx := svcOffset/instrLen - 1
	endIdx := startIdx - maxScan
	if endIdx < 0 {
		endIdx = -1
	}

	x16Lo := -1
	x16Hi := -1

	for i := startIdx; i > endIdx; i-- {
		off := i * instrLen
		if off < 0 {
			break
		}
		word := binary.LittleEndian.Uint32(code[off:])

		if word&^imm16Mask == movzX16Base {
			lo := int((word & imm16Mask) >> imm16Shift)
			hi := 0
			if x16Hi >= 0 {
				hi = x16Hi
			}
			v := hi | lo
			if v >= bsdPrefix {
				v -= bsdPrefix
			}
			return v, true
		}

		if word&^imm16Mask == movzX16Lsl16 {
			hi := int((word&imm16Mask)>>imm16Shift) << imm16HiShift
			lo := 0
			if x16Lo >= 0 {
				lo = x16Lo
			}
			v := hi | lo
			if v >= bsdPrefix {
				v -= bsdPrefix
			}
			return v, true
		}

		if word&^imm16Mask == movkX16Base {
			x16Lo = int((word & imm16Mask) >> imm16Shift)
			continue
		}

		if word&^imm16Mask == movkX16Lsl16 {
			x16Hi = int((word&imm16Mask)>>imm16Shift) << imm16HiShift
			continue
		}

		if arm64IsControlFlowInstr(word) {
			break
		}

		// Stop when another instruction writes to x16.
		if arm64WritesX16NotMovzMovk(word) {
			break
		}
	}
	return 0, false
}

// arm64IsControlFlowInstr mirrors libccache.IsControlFlowInstruction.
// See the note on arm64BackwardScanX16 about the duplication rationale.
func arm64IsControlFlowInstr(word uint32) bool {
	const (
		blBranchShift   = 26
		blBranchB       = uint32(0b000101)
		blBranchBL      = uint32(0b100101)
		brRegShift      = 22
		brRegPatternBLR = uint32(0b1101011000)
		brRegPatternBR  = uint32(0b1101011001)
		cbBranchShift   = 25
		cbBranchMask    = uint32(0x3F)
		cbzCbnzPattern  = uint32(0b011010)
		tbzTbnzPattern  = uint32(0b011011)
	)
	// B, BL: [31:26] = 000101 or 100101
	if word>>blBranchShift == blBranchB || word>>blBranchShift == blBranchBL {
		return true
	}
	// BLR, BR, RET: [31:22] = 1101011000 or 1101011001
	hi10 := word >> brRegShift
	if hi10 == brRegPatternBLR || hi10 == brRegPatternBR {
		return true
	}
	// CBZ, CBNZ: [30:25] = 011010
	hi6 := (word >> cbBranchShift) & cbBranchMask
	if hi6 == cbzCbnzPattern {
		return true
	}
	// TBZ, TBNZ: [30:25] = 011011
	if hi6 == tbzTbnzPattern {
		return true
	}
	return false
}

// arm64WritesX16NotMovzMovk detects 64-bit instructions that write to x16,
// excluding MOVZ and MOVK. Mirrors libccache.writesX16NotMovzMovk.
func arm64WritesX16NotMovzMovk(word uint32) bool {
	const (
		patMovzMovk   = uint32(0b100101)
		shiftMovzMovk = 23
		mask6         = uint32(0x3F)
	)
	if (word>>shiftMovzMovk)&mask6 == patMovzMovk {
		return false
	}
	return word>>31 == 1 && word&0x1F == 0x10
}
