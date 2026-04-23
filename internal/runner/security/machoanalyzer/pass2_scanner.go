package machoanalyzer

import (
	"encoding/binary"

	"github.com/isseis/go-safe-cmd-runner/internal/arm64util"
	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// arm64BL constants for detecting and decoding BL instructions.
const (
	arm64BLOpcShift  = 26
	arm64BLOpcode    = uint32(0b100101) // bits[31:26] of BL
	arm64BLImmMask   = uint32(0x03ffffff)
	arm64BLSignShift = uint32(6) // 32 - 26
	arm64InstrLen    = 4
)

// buildWrapperAddrs builds a map from known Go syscall stub entry addresses to
// their function names. Used by Pass 2 to detect BL calls into those stubs from
// user code.
func buildWrapperAddrs(funcs map[string]MachoPclntabFunc) map[uint64]string {
	addrs := make(map[uint64]string, len(knownMachoSyscallImpls))
	for name, fn := range funcs {
		if _, ok := knownMachoSyscallImpls[name]; ok {
			addrs[fn.Entry] = name
		}
	}
	return addrs
}

// getBLTarget returns the target virtual address of a BL instruction.
// instrAddr is the virtual address of the BL instruction itself.
// Returns (target, true) on success, (0, false) if not a BL or if the target
// address overflows.
func getBLTarget(word uint32, instrAddr uint64) (uint64, bool) {
	if word>>arm64BLOpcShift != arm64BLOpcode {
		return 0, false
	}
	imm26Raw := word & arm64BLImmMask
	imm26 := int32(imm26Raw<<arm64BLSignShift) >> int(arm64BLSignShift) //nolint:gosec // G115: imm26Raw is masked to 26 bits
	base := int64(instrAddr)                                            //nolint:gosec // G115: instrAddr is a Mach-O VA, fits in int64
	target := base + int64(imm26)*arm64InstrLen
	if target < 0 {
		return 0, false
	}
	return uint64(target), true //nolint:gosec // G115: target non-negative checked above
}

// scanGoWrapperCalls performs Pass 2: scans the __TEXT,__text section for BL
// instructions targeting known Go syscall stub addresses, then resolves the
// syscall number from the preceding X0 register assignment.
//
//   - code: raw bytes of __TEXT,__text section
//   - textBase: virtual address of the section start
//   - wrapperAddrs: map from wrapper entry address to wrapper name (from buildWrapperAddrs)
//   - stubRanges: address ranges of known stubs; BL instructions inside these
//     ranges are skipped (they are internal calls, not user-level syscall requests)
//   - table: macOS BSD syscall table for name/network lookups
//
// Returns one SyscallInfo per BL call to a known wrapper that was NOT excluded.
func scanGoWrapperCalls(
	code []byte,
	textBase uint64,
	wrapperAddrs map[uint64]string,
	stubRanges []funcRange,
	table SyscallNumberTable,
) []common.SyscallInfo {
	if len(wrapperAddrs) == 0 {
		return nil
	}

	var results []common.SyscallInfo

	for offset := 0; offset+arm64InstrLen <= len(code); offset += arm64InstrLen {
		word := binary.LittleEndian.Uint32(code[offset:])
		instrAddr := textBase + uint64(offset) //nolint:gosec // G115: offset bounded by len(code)

		target, ok := getBLTarget(word, instrAddr)
		if !ok {
			continue
		}

		if _, isWrapper := wrapperAddrs[target]; !isWrapper {
			continue
		}

		// Skip BL instructions that originate from inside a stub body — those are
		// internal calls within the Go runtime, not user-level syscall requests.
		if isInsideRange(instrAddr, stubRanges) {
			continue
		}

		num, resolved := arm64util.BackwardScanX0(code, offset)

		var info common.SyscallInfo
		if resolved {
			info = common.SyscallInfo{
				Number:              num,
				Name:                table.GetSyscallName(num),
				IsNetwork:           table.IsNetworkSyscall(num),
				Location:            instrAddr,
				DeterminationMethod: determinationMethodGoWrapper,
			}
		} else {
			info = common.SyscallInfo{
				Number:              -1,
				Location:            instrAddr,
				DeterminationMethod: determinationMethodUnknownIndirect,
			}
		}
		results = append(results, info)
	}

	return results
}
