package machoanalyzer

import (
	"encoding/binary"

	"github.com/isseis/go-safe-cmd-runner/internal/arm64util"
	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// arm64 branch instruction constants for BL and B decoding.
const (
	arm64BLOpcShift  = 26
	arm64BLOpcode    = uint32(0b100101) // bits[31:26] of BL
	arm64BOpcode     = uint32(0b000101) // bits[31:26] of B (unconditional, no link)
	arm64BLImmMask   = uint32(0x03ffffff)
	arm64BLSignShift = uint32(6) // 32 - 26; also used for B imm26 sign-extension
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

// isKnownWrapper reports whether target is a known Go syscall wrapper address,
// handling single-instruction stub trampolines of the form "B wrapperAddr".
// Go linkers sometimes emit a one-instruction trampoline at a near address that
// simply branches to the actual wrapper; we resolve one level of such stubs.
func isKnownWrapper(code []byte, textBase, target uint64, wrapperAddrs map[uint64]string) bool {
	if _, ok := wrapperAddrs[target]; ok {
		return true
	}
	// Attempt one-level stub resolution: if the instruction at target is a
	// plain B (not BL), follow it and check whether the branch destination is
	// a known wrapper.
	if target < textBase {
		return false
	}
	off := target - textBase
	if off+arm64InstrLen > uint64(len(code)) { //nolint:gosec // G115: off bounded by section size
		return false
	}
	word := binary.LittleEndian.Uint32(code[off:])
	if word>>arm64BLOpcShift != arm64BOpcode {
		return false
	}
	imm26 := int32(word&arm64BLImmMask<<arm64BLSignShift) >> int(arm64BLSignShift) //nolint:gosec // G115: masked to 26 bits
	stubTarget := uint64(int64(target) + int64(imm26)*arm64InstrLen)               //nolint:gosec // G115: target is a Mach-O VA
	_, ok := wrapperAddrs[stubTarget]
	return ok
}

// scanGoWrapperCalls performs Pass 2: scans the __TEXT,__text section for BL
// instructions targeting known Go syscall stub addresses, then resolves the
// syscall number from the preceding trap argument write to [SP, #8].
//
// syscall.Syscall/RawSyscall et al. use the old stack-based calling convention
// (NOSPLIT assembly stubs): the caller stores the trap number at SP+8 (trap+0(FP))
// before the BL, not in a register via the register ABI. BackwardScanStackTrap
// detects the STR/STP to [SP, #8] and then backward-scans the source register
// for an immediate-load sequence.
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

		if !isKnownWrapper(code, textBase, target, wrapperAddrs) {
			continue
		}

		// Skip BL instructions that originate from inside a stub body — those are
		// internal calls within the Go runtime, not user-level syscall requests.
		if isInsideRange(instrAddr, stubRanges) {
			continue
		}

		num, resolved := arm64util.BackwardScanStackTrap(code, offset)

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
