// Package arm64util provides shared ARM64 instruction decoding utilities.
// Extracted to break the import cycle: machoanalyzer → libccache → filevalidator → machoanalyzer.
package arm64util

import (
	"encoding/binary"
	"math/bits"
)

const (
	instrLen                = 4
	maxBackwardScanInstr    = 16
	bsdSyscallClassPrefix   = 0x2000000
	imm16HighShift          = 16
	movzX16Base             = uint32(0xD2800010) // MOVZ X16, #0, LSL #0
	movzX16Lsl16            = uint32(0xD2A00010) // MOVZ X16, #0, LSL #16
	movkX16Base             = uint32(0xF2800010) // MOVK X16, #0, LSL #0
	movkX16Lsl16            = uint32(0xF2A00010) // MOVK X16, #0, LSL #16
	movzX0Base              = uint32(0xD2800000) // MOVZ X0, #0, LSL #0
	movzX0Lsl16             = uint32(0xD2A00000) // MOVZ X0, #0, LSL #16
	movkX0Base              = uint32(0xF2800000) // MOVK X0, #0, LSL #0
	movkX0Lsl16             = uint32(0xF2A00000) // MOVK X0, #0, LSL #16
	movzW0Base              = uint32(0x52800000) // MOVZ W0, #0, LSL #0
	movkW0Base              = uint32(0x72800000) // MOVK W0, #0, LSL #0
	imm16Mask               = uint32(0x001FFFE0) // bits[20:5]
	imm16Shift              = 5
	patternBLRBR            = uint32(0b1101011000) // bits[31:22] for BLR/BR
	patternRET              = uint32(0b1101011001) // bits[31:22] for RET
	patternCBZ              = uint32(0b011010)     // bits[30:25] for CBZ/CBNZ
	patternTBZ              = uint32(0b011011)     // bits[30:25] for TBZ/TBNZ
	field6Mask              = uint32(0x3F)
	patternMovWideBits28_23 = uint32(0b100101) // [28:23] shared by MOVN/MOVZ/MOVK
	bitShiftBLRBR           = 22
	bitShiftCBZTBZ          = 25
	bitShiftMovWide         = 23
	bitShiftOpc             = 29
	opcMask                 = uint32(0x3)
	opcMOVZ                 = uint32(0b10) // opc[30:29]=10 → MOVZ
	opcMOVK                 = uint32(0b11) // opc[30:29]=11 → MOVK
	arm64BitsPerWord        = uint32(64)   // bit width of a 64-bit ARM64 register
	orrNBitPos              = uint32(22)   // bit position of N in ORR/logical immediate
	orrImmrBitPos           = uint32(16)   // bit position of immr in logical immediate (bits[21:16])
	orrImmsbitPos           = uint32(10)   // bit position of imms in logical immediate (bits[15:10])
	bitmaskImmFieldWidth    = uint32(6)    // bit width of immr/imms fields in bitmask immediate
)

// BackwardScanX16 walks backward from the svc #0x80 instruction at code[svcOffset]
// and looks for an immediate-load sequence into x16. When found, it returns the syscall
// number with the BSD class prefix removed. The scan is limited to maxBackwardScanInstr.
func BackwardScanX16(code []byte, svcOffset int) (int, bool) {
	startIdx := svcOffset/instrLen - 1
	endIdx := startIdx - maxBackwardScanInstr
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
			return stripBSDPrefix(hi | lo), true
		}

		if word&^imm16Mask == movzX16Lsl16 {
			hi := int((word&imm16Mask)>>imm16Shift) << imm16HighShift
			lo := 0
			if x16Lo >= 0 {
				lo = x16Lo
			}
			return stripBSDPrefix(hi | lo), true
		}

		if word&^imm16Mask == movkX16Base {
			x16Lo = int((word & imm16Mask) >> imm16Shift)
			continue
		}

		if word&^imm16Mask == movkX16Lsl16 {
			x16Hi = int((word&imm16Mask)>>imm16Shift) << imm16HighShift
			continue
		}

		// ORR X16, XZR, #imm (bitmask immediate form of MOV X16, #imm)
		if val, ok := decodeORRX16XZR(word); ok {
			return stripBSDPrefix(val), true
		}

		if isControlFlowInstruction(word) {
			break
		}

		if writesX16NotMovzMovk(word) {
			break
		}
	}
	return 0, false
}

// BackwardScanStackTrap walks backward from the BL instruction at code[blOffset]
// looking for a Go old-stack-ABI trap argument write: STR/STP xN, [SP, #8] (the
// trap argument slot), followed by an immediate-load sequence into xN.
//
// This is the correct scan for syscall.Syscall/RawSyscall et al., which are
// NOSPLIT assembly stubs using the old stack-based calling convention: the caller
// stores trap+0(FP) = [SP+8] before the BL, not in a register passed via ABI.
func BackwardScanStackTrap(code []byte, blOffset int) (int, bool) {
	startIdx := blOffset/instrLen - 1
	endIdx := startIdx - maxBackwardScanInstr
	if endIdx < 0 {
		endIdx = -1
	}
	for i := startIdx; i > endIdx; i-- {
		off := i * instrLen
		if off < 0 || off+instrLen > len(code) {
			break
		}
		word := binary.LittleEndian.Uint32(code[off:])
		if isControlFlowInstruction(word) {
			break
		}
		trapReg, found := trapStoreReg(word)
		if !found {
			continue
		}
		return backwardScanRegImm(code, off, trapReg)
	}
	return 0, false
}

// trapStoreReg checks whether word stores a register to [SP, #8] (the trap
// argument slot in Go's old stack ABI). Returns (regN, true) where regN is
// the register that holds the syscall number.
//
// Go old stack ABI frame for syscall.Syscall: SP+0=return addr, SP+8=trap,
// SP+16=a1, ... The compiler emits STR xN,[SP,#8] or STP xN,xM,[SP,#8]
// to set up the trap argument before BL.
func trapStoreReg(word uint32) (uint32, bool) {
	const (
		strUnsignedOffset64 = uint32(0x3E4) // STR Xt,[Xn,#pimm] bits[31:22]
		stpSignedOffset64   = uint32(0x2A4) // STP Xt1,Xt2,[Xn,#imm] bits[31:22]
		strImm12Shift       = 10            // bit position of imm12 in STR unsigned-offset
		stpImm7Shift        = 15            // bit position of imm7 in STP signed-offset
		loadStoreRnShift    = 5             // bit position of Rn in load/store instructions
		loadStoreRdMask     = uint32(0x1F)  // 5-bit mask for Rt/Rd in load/store
		loadStoreImm12Mask  = uint32(0xFFF) // 12-bit mask for imm12
		loadStoreImm7Mask   = uint32(0x7F)  // 7-bit mask for imm7
		trapSlotImm         = uint32(1)     // imm=1 → offset=8 bytes (trap slot)
		regSP               = uint32(31)
	)
	// STR xN, [SP, #8]: imm12=1 (=8/8), Rn=SP
	if word>>22 == strUnsignedOffset64 &&
		(word>>strImm12Shift)&loadStoreImm12Mask == trapSlotImm &&
		(word>>loadStoreRnShift)&loadStoreRdMask == regSP {
		return word & loadStoreRdMask, true
	}
	// STP xN, xM, [SP, #8]: imm7=1 (=8/8), Rn=SP; xN(Rt1) stored at SP+8 = trap slot
	if word>>22 == stpSignedOffset64 &&
		(word>>stpImm7Shift)&loadStoreImm7Mask == trapSlotImm &&
		(word>>loadStoreRnShift)&loadStoreRdMask == regSP {
		return word & loadStoreRdMask, true
	}
	return 0, false
}

// backwardScanRegImm scans backward from code[fromOffset] (exclusive) looking
// for an immediate-load sequence into register regN (MOVZ/MOVK patterns).
// The scan is limited to maxBackwardScanInstr instructions.
func backwardScanRegImm(code []byte, fromOffset int, regN uint32) (int, bool) {
	startIdx := fromOffset/instrLen - 1
	endIdx := startIdx - maxBackwardScanInstr
	if endIdx < 0 {
		endIdx = -1
	}
	immLo := -1
	immHi := -1
	for i := startIdx; i > endIdx; i-- {
		off := i * instrLen
		if off < 0 || off+instrLen > len(code) {
			break
		}
		word := binary.LittleEndian.Uint32(code[off:])
		// MOVZ xN/wN, #imm, LSL#0 — terminal: assemble hi|lo and return
		if word&^imm16Mask == movzX0Base|regN || word&^imm16Mask == movzW0Base|regN {
			lo := int((word & imm16Mask) >> imm16Shift)
			hi := 0
			if immHi >= 0 {
				hi = immHi
			}
			return hi | lo, true
		}
		// MOVZ xN, #imm, LSL#16 — terminal
		if word&^imm16Mask == movzX0Lsl16|regN {
			hi := int((word&imm16Mask)>>imm16Shift) << imm16HighShift
			lo := 0
			if immLo >= 0 {
				lo = immLo
			}
			return hi | lo, true
		}
		// MOVK xN/wN, #imm, LSL#0 — accumulate low half
		if word&^imm16Mask == movkX0Base|regN || word&^imm16Mask == movkW0Base|regN {
			immLo = int((word & imm16Mask) >> imm16Shift)
			continue
		}
		// MOVK xN, #imm, LSL#16 — accumulate high half
		if word&^imm16Mask == movkX0Lsl16|regN {
			immHi = int((word&imm16Mask)>>imm16Shift) << imm16HighShift
			continue
		}
		if isControlFlowInstruction(word) {
			break
		}
		if writesRegNotMovzMovk(word, regN) {
			break
		}
	}
	return 0, false
}

// writesRegNotMovzMovk reports whether word is an instruction that writes to
// register regN, excluding MOVZ and MOVK. Used as a conservative stop signal
// during backward immediate-load scanning.
func writesRegNotMovzMovk(word, regN uint32) bool {
	if (word>>bitShiftMovWide)&field6Mask == patternMovWideBits28_23 {
		opc := (word >> bitShiftOpc) & opcMask
		if opc == opcMOVZ || opc == opcMOVK {
			return false
		}
	}
	return word&0x1F == regN
}

func stripBSDPrefix(v int) int {
	if v >= bsdSyscallClassPrefix {
		return v - bsdSyscallClassPrefix
	}
	return v
}

// isControlFlowInstruction reports whether word is a B/BL/BLR/BR/RET/CBZ/CBNZ/TBZ/TBNZ instruction.
func isControlFlowInstruction(word uint32) bool {
	if word>>26 == 0b000101 || word>>26 == 0b100101 {
		return true
	}
	if word>>bitShiftBLRBR == patternBLRBR || word>>bitShiftBLRBR == patternRET {
		return true
	}
	if (word>>bitShiftCBZTBZ)&field6Mask == patternCBZ {
		return true
	}
	if (word>>bitShiftCBZTBZ)&field6Mask == patternTBZ {
		return true
	}
	return false
}

// decodeORRX16XZR decodes an ORR X16, XZR, #imm instruction (bitmask immediate
// form of MOV X16, #imm, as sometimes emitted by Go 1.25+ compilers) and returns
// the immediate value. Returns (0, false) if word is not that instruction or if
// the bitmask encoding is undefined.
func decodeORRX16XZR(word uint32) (int, bool) {
	// ORR (immediate) 64-bit: sf=1, opc=01, [28:23]=100100, Rn=XZR(31), Rd=X16(16)
	// Fixed-bit mask covers bits[31:23] and bits[9:0] (Rn and Rd).
	const orrX16XZRMask = uint32(0xFF8003FF)
	const orrX16XZRFixed = uint32(0xB20003F0)
	if word&orrX16XZRMask != orrX16XZRFixed {
		return 0, false
	}

	N := (word >> orrNBitPos) & 1
	immr := (word >> orrImmrBitPos) & field6Mask
	imms := (word >> orrImmsbitPos) & field6Mask

	// len = HighestSetBit(N:NOT(imms)) — ARMv8 Reference Manual bitmask decode
	nNotImms := (N << bitmaskImmFieldWidth) | ((^imms) & field6Mask)
	lenBits := bits.Len32(nNotImms) - 1
	if lenBits < 1 {
		return 0, false // undefined encoding
	}

	esize := uint32(1) << lenBits
	levels := esize - 1
	S := imms & levels
	R := immr & levels

	// All-ones within an element is undefined in ARM64 bitmask immediate.
	if S == levels {
		return 0, false
	}

	// Build S+1 consecutive ones (S < levels < esize ≤ 64, so S+1 ≤ 63 — no overflow).
	welem := (uint64(1) << (S + 1)) - 1

	// Rotate right by R within esize bits.
	if R > 0 {
		if esize == arm64BitsPerWord {
			welem = bits.RotateLeft64(welem, -int(R))
		} else {
			mask := (uint64(1) << esize) - 1
			welem = ((welem >> R) | (welem << (esize - R))) & mask
		}
	}

	// Replicate the esize-bit pattern to fill 64 bits.
	var result uint64
	if esize == arm64BitsPerWord {
		result = welem
	} else {
		for i := uint32(0); i < arm64BitsPerWord/esize; i++ {
			result |= welem << (i * esize)
		}
	}

	return int(result), true
}

// writesX16NotMovzMovk reports whether word is a 64-bit instruction that writes to x16,
// excluding MOVZ and MOVK (but not MOVN, which also writes its destination).
func writesX16NotMovzMovk(word uint32) bool {
	if (word>>bitShiftMovWide)&field6Mask == patternMovWideBits28_23 {
		opc := (word >> bitShiftOpc) & opcMask
		if opc == opcMOVZ || opc == opcMOVK {
			return false
		}
	}
	return word>>31 == 1 && word&0x1F == 0x10
}
