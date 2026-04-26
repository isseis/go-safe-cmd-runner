package elfanalyzer

import (
	"errors"
	"math"

	"golang.org/x/arch/arm64/arm64asm"
)

// arm64InstructionLen is the fixed length of all arm64 instructions in bytes.
const arm64InstructionLen = 4

// ARM64Decoder implements MachineCodeDecoder for arm64.
type ARM64Decoder struct{}

// NewARM64Decoder creates a new ARM64Decoder.
func NewARM64Decoder() *ARM64Decoder { return &ARM64Decoder{} }

var errCodeTooShort = errors.New("code too short for arm64 instruction")

// Decode decodes a single arm64 instruction (always 4 bytes).
// Returns an error if decoding fails or if the code slice is shorter than 4 bytes.
func (d *ARM64Decoder) Decode(code []byte, offset uint64) (DecodedInstruction, error) {
	if len(code) < arm64InstructionLen {
		return DecodedInstruction{}, errCodeTooShort
	}
	inst, err := arm64asm.Decode(code)
	if err != nil {
		return DecodedInstruction{}, err
	}
	return DecodedInstruction{
		Offset: offset,
		Len:    arm64InstructionLen,
		Raw:    code[:arm64InstructionLen],
		arch:   inst,
	}, nil
}

// IsSyscallInstruction returns true if inst is "svc #0".
// arm64 syscalls are invoked with "svc #0" (encoding: 0xD4000001).
func (d *ARM64Decoder) IsSyscallInstruction(inst DecodedInstruction) bool {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok {
		return false
	}
	if a.Op != arm64asm.SVC {
		return false
	}
	// SVC takes one immediate operand; verify it is Imm(0)
	imm, ok := a.Args[0].(arm64asm.Imm)
	return ok && imm.Imm == 0
}

// arm64ReadOnlyFirstOperandOp reports whether op uses its first operand as a
// read-only source (i.e., does not write to it). Such instructions must not
// be treated as register modifications during backward scanning.
// Control-flow ops (B, BL, etc.) are handled separately by IsControlFlowInstruction.
func arm64ReadOnlyFirstOperandOp(op arm64asm.Op) bool {
	switch op {
	case arm64asm.STR, arm64asm.STP,
		arm64asm.CMP, arm64asm.CMN,
		arm64asm.TST, arm64asm.CCMP, arm64asm.CCMN:
		return true
	}
	return false
}

// arm64MatchesReg returns true if arg represents the specified register.
// It handles both arm64asm.Reg and arm64asm.RegSP types: arm64asm encodes the
// destination operand of ORR-immediate instructions as RegSP rather than Reg,
// which causes a simple type assertion to arm64asm.Reg to fail for those cases.
func arm64MatchesReg(arg arm64asm.Arg, reg arm64asm.Reg) bool {
	if r, ok := arg.(arm64asm.Reg); ok {
		return r == reg
	}
	if rSP, ok := arg.(arm64asm.RegSP); ok {
		return arm64asm.Reg(rSP) == reg
	}
	return false
}

// arm64OrrZeroRegImm returns (true, value) if a is "ORR dst, XZR/WZR, #imm"
// and dst matches one of the given registers.
// This encoding is used when a constant cannot be represented as a 16-bit MOVZ
// immediate but fits the ARM64 bitmask-immediate format; it is functionally
// identical to "MOV dst, #imm".
func arm64OrrZeroRegImm(a arm64asm.Inst, regs ...arm64asm.Reg) (bool, int64) {
	if a.Op != arm64asm.ORR {
		return false, 0
	}
	if a.Args[0] == nil || a.Args[1] == nil || a.Args[2] == nil {
		return false, 0
	}
	// Destination must be one of the target registers.
	// ORR-immediate uses arm64asm.RegSP for the destination operand.
	matched := false
	for _, reg := range regs {
		if arm64MatchesReg(a.Args[0], reg) {
			matched = true
			break
		}
	}
	if !matched {
		return false, 0
	}
	// Source must be the zero register (XZR or WZR).
	if !arm64MatchesReg(a.Args[1], arm64asm.XZR) && !arm64MatchesReg(a.Args[1], arm64asm.WZR) {
		return false, 0
	}
	val, ok := arm64ImmValue(a.Args[2])
	return ok, val
}

// ModifiesSyscallNumberRegister returns true if the instruction writes to
// the arm64 syscall number register (W8 or X8).
func (d *ARM64Decoder) ModifiesSyscallNumberRegister(inst DecodedInstruction) bool {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok {
		return false
	}
	if arm64ReadOnlyFirstOperandOp(a.Op) {
		return false
	}
	if a.Args[0] == nil {
		return false
	}
	return arm64MatchesReg(a.Args[0], arm64asm.W8) || arm64MatchesReg(a.Args[0], arm64asm.X8)
}

// IsImmediateToSyscallNumberRegister returns (true, value) if inst sets
// W8 or X8 to a known immediate value.
// Handles two encodings:
//   - MOV W8/X8, #imm  (arm64asm normalises MOVZ to MOV)
//   - ORR W8/X8, WZR/XZR, #imm  (bitmask-immediate; functionally identical to MOV)
func (d *ARM64Decoder) IsImmediateToSyscallNumberRegister(inst DecodedInstruction) (bool, int64) {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok {
		return false, 0
	}
	if a.Op == arm64asm.MOV {
		if a.Args[0] == nil || a.Args[1] == nil {
			return false, 0
		}
		if !arm64MatchesReg(a.Args[0], arm64asm.W8) && !arm64MatchesReg(a.Args[0], arm64asm.X8) {
			return false, 0
		}
		val, ok := arm64ImmValue(a.Args[1])
		return ok, val
	}
	return arm64OrrZeroRegImm(a, arm64asm.W8, arm64asm.X8)
}

// IsControlFlowInstruction returns true if inst changes the instruction pointer.
// arm64 control flow instructions: B, BL, BLR, BR, RET, CBZ, CBNZ, TBZ, TBNZ.
func (d *ARM64Decoder) IsControlFlowInstruction(inst DecodedInstruction) bool {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok {
		return false
	}
	switch a.Op {
	case arm64asm.B, arm64asm.BL, arm64asm.BLR, arm64asm.BR, arm64asm.RET,
		arm64asm.CBZ, arm64asm.CBNZ, arm64asm.TBZ, arm64asm.TBNZ:
		return true
	}
	return false
}

// InstructionAlignment returns 4, reflecting arm64's fixed 4-byte instruction width.
func (d *ARM64Decoder) InstructionAlignment() int { return arm64InstructionLen }

// MaxInstructionLength returns 4, since all arm64 instructions are exactly 4 bytes.
func (d *ARM64Decoder) MaxInstructionLength() int { return arm64InstructionLen }

// GetCallTarget returns the target address of a BL instruction.
// For BL with a PCRel operand, target = instAddr + int64(pcrel).
// arm64 PC points to the current instruction, so Len is not added.
// Returns (addr, true) on success, (0, false) otherwise.
func (d *ARM64Decoder) GetCallTarget(inst DecodedInstruction, instAddr uint64) (uint64, bool) {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok || a.Op != arm64asm.BL {
		return 0, false
	}
	if a.Args[0] == nil {
		return 0, false
	}
	pcrel, ok := a.Args[0].(arm64asm.PCRel)
	if !ok {
		return 0, false
	}
	// Guard against overflow: reject instAddr values that, when converted to
	// int64, overflow (i.e. instAddr > math.MaxInt64).
	if instAddr > math.MaxInt64 {
		return 0, false
	}
	target := int64(instAddr) + int64(pcrel)
	if target < 0 {
		return 0, false
	}
	return uint64(target), true //nolint:gosec // G115: target non-negative validated above
}

// IsImmediateToFirstArgRegister returns (value, true) if inst sets the arm64
// first argument register (X0 or W0) to an immediate.
// arm64 Go ABI uses X0 for the first integer argument.
// Handles two encodings:
//   - MOV X0/W0, #imm  (arm64asm normalises MOVZ to MOV)
//   - ORR X0/W0, XZR/WZR, #imm  (bitmask-immediate; functionally identical to MOV)
func (d *ARM64Decoder) IsImmediateToFirstArgRegister(inst DecodedInstruction) (int64, bool) {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok {
		return 0, false
	}
	if a.Op == arm64asm.MOV {
		if a.Args[0] == nil || a.Args[1] == nil {
			return 0, false
		}
		if !arm64MatchesReg(a.Args[0], arm64asm.X0) && !arm64MatchesReg(a.Args[0], arm64asm.W0) {
			return 0, false
		}
		val, ok := arm64ImmValue(a.Args[1])
		return val, ok
	}
	ok2, val := arm64OrrZeroRegImm(a, arm64asm.X0, arm64asm.W0)
	return val, ok2
}

// ModifiesThirdArgRegister returns true if the instruction writes to
// the arm64 third syscall argument register (W2 or X2).
func (d *ARM64Decoder) ModifiesThirdArgRegister(inst DecodedInstruction) bool {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok {
		return false
	}
	if arm64ReadOnlyFirstOperandOp(a.Op) {
		return false
	}
	if a.Args[0] == nil {
		return false
	}
	return arm64MatchesReg(a.Args[0], arm64asm.W2) || arm64MatchesReg(a.Args[0], arm64asm.X2)
}

// IsImmediateToThirdArgRegister returns (true, value) if inst sets
// W2 or X2 to a known immediate value.
// Handles two encodings:
//   - MOV W2/X2, #imm  (arm64asm normalises MOVZ to MOV)
//   - ORR W2/X2, WZR/XZR, #imm  (bitmask-immediate; functionally identical to MOV)
func (d *ARM64Decoder) IsImmediateToThirdArgRegister(inst DecodedInstruction) (bool, int64) {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok {
		return false, 0
	}
	if a.Op == arm64asm.MOV {
		if a.Args[0] == nil || a.Args[1] == nil {
			return false, 0
		}
		if !arm64MatchesReg(a.Args[0], arm64asm.W2) && !arm64MatchesReg(a.Args[0], arm64asm.X2) {
			return false, 0
		}
		val, ok := arm64ImmValue(a.Args[1])
		return ok, val
	}
	return arm64OrrZeroRegImm(a, arm64asm.W2, arm64asm.X2)
}

// arm64ImmValue extracts an int64 immediate value from an arm64asm.Arg.
// Handles both arm64asm.Imm and arm64asm.Imm64.
// Returns (value, true) on success, (0, false) if arg is not an immediate.
func arm64ImmValue(arg arm64asm.Arg) (int64, bool) {
	switch v := arg.(type) {
	case arm64asm.Imm:
		return int64(v.Imm), true
	case arm64asm.Imm64:
		return int64(v.Imm), true //nolint:gosec // G115: caller validates range via maxValidSyscallNumber before using the value
	}
	return 0, false
}
