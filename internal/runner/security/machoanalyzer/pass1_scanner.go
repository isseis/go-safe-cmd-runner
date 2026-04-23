package machoanalyzer

import (
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/arm64util"
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

		if addr < textBase || addr >= textBase+uint64(len(code)) {
			results = append(results, common.SyscallInfo{
				Number:              -1,
				Name:                "",
				IsNetwork:           false,
				Location:            addr,
				DeterminationMethod: determinationMethodUnknownIndirect,
				Source:              "",
			})
			continue
		}
		svcOffset := int(addr - textBase) //nolint:gosec // G115: addr-textBase < len(code) which fits in int

		num, ok := arm64util.BackwardScanX16(code, svcOffset)

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
