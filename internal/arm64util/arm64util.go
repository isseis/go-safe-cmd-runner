// Package arm64util provides shared ARM64 instruction decoding utilities.
// Extracted to break the import cycle: machoanalyzer → libccache → filevalidator → machoanalyzer.
package arm64util

import "encoding/binary"

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
	regEncodingX0           = uint32(0x00)         // arm64 register encoding for X0/W0
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

		if isControlFlowInstruction(word) {
			break
		}

		if writesX16NotMovzMovk(word) {
			break
		}
	}
	return 0, false
}

// BackwardScanX0 walks backward from the BL instruction at code[blOffset]
// and looks for an immediate-load sequence into X0 or W0 (first argument register
// on arm64). When found, it returns the loaded value. The scan is limited to
// maxBackwardScanInstr instructions.
func BackwardScanX0(code []byte, blOffset int) (int, bool) {
	startIdx := blOffset/instrLen - 1
	endIdx := startIdx - maxBackwardScanInstr
	if endIdx < 0 {
		endIdx = -1
	}

	x0Lo := -1
	x0Hi := -1

	for i := startIdx; i > endIdx; i-- {
		off := i * instrLen
		if off < 0 {
			break
		}
		if off+instrLen > len(code) {
			break
		}
		word := binary.LittleEndian.Uint32(code[off:])

		if word&^imm16Mask == movzX0Base {
			lo := int((word & imm16Mask) >> imm16Shift)
			hi := 0
			if x0Hi >= 0 {
				hi = x0Hi
			}
			return hi | lo, true
		}

		if word&^imm16Mask == movzX0Lsl16 {
			hi := int((word&imm16Mask)>>imm16Shift) << imm16HighShift
			lo := 0
			if x0Lo >= 0 {
				lo = x0Lo
			}
			return hi | lo, true
		}

		if word&^imm16Mask == movzW0Base {
			return int((word & imm16Mask) >> imm16Shift), true
		}

		if word&^imm16Mask == movkX0Base {
			x0Lo = int((word & imm16Mask) >> imm16Shift)
			continue
		}

		if word&^imm16Mask == movkX0Lsl16 {
			x0Hi = int((word&imm16Mask)>>imm16Shift) << imm16HighShift
			continue
		}

		if word&^imm16Mask == movkW0Base {
			x0Lo = int((word & imm16Mask) >> imm16Shift)
			continue
		}

		if isControlFlowInstruction(word) {
			break
		}

		if writesX0NotMovzMovk(word) {
			break
		}
	}
	return 0, false
}

// writesX0NotMovzMovk reports whether word is an instruction that writes to
// X0 or W0, excluding MOVZ and MOVK (which are handled by BackwardScanX0).
// Used as a conservative stop signal during backward scanning.
func writesX0NotMovzMovk(word uint32) bool {
	if (word>>bitShiftMovWide)&field6Mask == patternMovWideBits28_23 {
		opc := (word >> bitShiftOpc) & opcMask
		if opc == opcMOVZ || opc == opcMOVK {
			return false
		}
	}
	return word&0x1F == regEncodingX0
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
