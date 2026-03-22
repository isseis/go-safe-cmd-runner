package elfanalyzer

import (
	"math"

	"golang.org/x/arch/x86/x86asm"
)

const (
	// x86_64BitMode is the bit width for 64-bit mode decoding.
	x86_64BitMode = 64

	// minArgsForImmediateMove is the minimum number of arguments
	// required to check for an immediate move instruction (destination + source).
	minArgsForImmediateMove = 2
)

// X86Decoder implements MachineCodeDecoder for x86_64.
type X86Decoder struct{}

// NewX86Decoder creates a new X86Decoder.
func NewX86Decoder() *X86Decoder {
	return &X86Decoder{}
}

// Decode decodes a single x86_64 instruction.
func (d *X86Decoder) Decode(code []byte, offset uint64) (DecodedInstruction, error) {
	inst, err := x86asm.Decode(code, x86_64BitMode)
	if err != nil {
		return DecodedInstruction{}, err
	}

	return DecodedInstruction{
		Offset: offset,
		Len:    inst.Len,
		Raw:    code[:inst.Len],
		arch:   inst,
	}, nil
}

// IsSyscallInstruction checks if the instruction is a syscall.
func (d *X86Decoder) IsSyscallInstruction(inst DecodedInstruction) bool {
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok {
		return false
	}
	return x86inst.Op == x86asm.SYSCALL
}

// isReadOnlyFirstOperandOp reports whether op uses its first operand as a
// read-only source and does not write to it. Such instructions must not be
// treated as register modifications during backward scanning.
// This covers the most common cases; control-flow ops (CALL, JMP, etc.) are
// handled separately by IsControlFlowInstruction.
func isReadOnlyFirstOperandOp(op x86asm.Op) bool {
	switch op {
	case x86asm.PUSH, x86asm.CMP, x86asm.TEST, x86asm.BT:
		return true
	}
	return false
}

// implicitlyWritesRAXEAX reports whether the instruction unconditionally writes
// to RAX/EAX as an implicit (unlisted) destination, i.e. RAX does not appear
// as the first explicit operand. Callers must handle the two/three-operand
// IMUL forms separately via the first-operand path.
//
// Covered cases:
//   - MUL r/m  — unsigned multiply: rDX:rAX = rAX × operand
//   - IMUL r/m — one-operand signed multiply (same implicit write as MUL);
//     distinguished from multi-operand IMUL by having exactly one non-nil arg
//   - DIV r/m  — unsigned divide: quotient → rAX, remainder → rDX
//   - IDIV r/m — signed divide: same layout as DIV
//   - CPUID    — writes EAX (plus EBX/ECX/EDX); no explicit operands
//
// Not included: CQO/CDQ/CWD — these read rAX and write rDX only.
func implicitlyWritesRAXEAX(x86inst x86asm.Inst) bool {
	switch x86inst.Op {
	case x86asm.MUL, x86asm.DIV, x86asm.IDIV, x86asm.CPUID:
		return true
	case x86asm.IMUL:
		// Only the one-operand form has an implicit rAX destination.
		// Two/three-operand forms carry an explicit destination as args[0]
		// and are already covered by the first-operand register check.
		args := x86inst.Args[:]
		for len(args) > 0 && args[len(args)-1] == nil {
			args = args[:len(args)-1]
		}
		return len(args) == 1
	}
	return false
}

// ModifiesSyscallNumberRegister checks if the instruction modifies eax or rax.
func (d *X86Decoder) ModifiesSyscallNumberRegister(inst DecodedInstruction) bool {
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok {
		return false
	}

	if isReadOnlyFirstOperandOp(x86inst.Op) {
		return false
	}

	// Instructions that implicitly write RAX/EAX without it appearing as the
	// first explicit operand (e.g. MUL, one-operand IMUL, DIV, IDIV, CPUID).
	if implicitlyWritesRAXEAX(x86inst) {
		return true
	}

	// Trim trailing nil arguments
	args := x86inst.Args[:]
	for len(args) > 0 && args[len(args)-1] == nil {
		args = args[:len(args)-1]
	}
	if len(args) == 0 {
		return false
	}

	// Check destination register (first argument for most instructions)
	if arg, ok := args[0].(x86asm.Reg); ok {
		return arg == x86asm.EAX || arg == x86asm.RAX ||
			arg == x86asm.AX || arg == x86asm.AL
	}

	return false
}

// IsImmediateToSyscallNumberRegister checks if the instruction sets eax/rax to a known immediate value.
// This covers two common compiler patterns:
//   - MOV EAX/RAX, <imm>  — direct immediate load
//   - XOR EAX, EAX        — idiom for zeroing EAX (equivalent to MOV EAX, 0)
func (d *X86Decoder) IsImmediateToSyscallNumberRegister(inst DecodedInstruction) (bool, int64) {
	return d.isImmediateToReg(inst, func(reg x86asm.Reg) bool {
		return reg == x86asm.EAX || reg == x86asm.RAX
	})
}

// isImmediateToReg is an internal helper that checks if an instruction sets
// a register (matched by regMatch) to an immediate value.
// Covers MOV <reg>, <imm> and XOR <reg>, <reg> (zeroing idiom).
func (d *X86Decoder) isImmediateToReg(inst DecodedInstruction, regMatch func(x86asm.Reg) bool) (bool, int64) {
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok {
		return false, 0
	}

	// Trim trailing nil arguments
	args := x86inst.Args[:]
	for len(args) > 0 && args[len(args)-1] == nil {
		args = args[:len(args)-1]
	}
	if len(args) < minArgsForImmediateMove {
		return false, 0
	}

	destReg, ok := args[0].(x86asm.Reg)
	if !ok {
		return false, 0
	}
	if !regMatch(destReg) {
		return false, 0
	}

	switch x86inst.Op {
	case x86asm.MOV:
		// MOV EAX/RAX, <imm>
		if src, ok := args[1].(x86asm.Imm); ok {
			return true, int64(src)
		}
	case x86asm.XOR:
		// XOR EAX, EAX (or XOR RAX, RAX) — sets register to 0.
		// Only match when both operands are the same register (self-XOR idiom).
		if srcReg, ok := args[1].(x86asm.Reg); ok && srcReg == destReg {
			return true, 0
		}
	}

	return false, 0
}

// IsControlFlowInstruction checks if the instruction is a control flow instruction.
func (d *X86Decoder) IsControlFlowInstruction(inst DecodedInstruction) bool {
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok {
		return false
	}
	switch x86inst.Op {
	case x86asm.JMP, x86asm.JA, x86asm.JAE, x86asm.JB, x86asm.JBE,
		x86asm.JE, x86asm.JG, x86asm.JGE, x86asm.JL, x86asm.JLE,
		x86asm.JNE, x86asm.JNO, x86asm.JNP, x86asm.JNS, x86asm.JO,
		x86asm.JP, x86asm.JS, x86asm.JCXZ, x86asm.JECXZ, x86asm.JRCXZ,
		x86asm.CALL, x86asm.RET, x86asm.IRET, x86asm.INT,
		x86asm.LOOP, x86asm.LOOPE, x86asm.LOOPNE:
		return true
	}
	return false
}

// InstructionAlignment returns 1 for x86_64 variable-length instructions.
func (d *X86Decoder) InstructionAlignment() int { return 1 }

// MaxInstructionLength returns 15, the architectural maximum for x86_64 instructions.
func (d *X86Decoder) MaxInstructionLength() int { return maxInstructionLength }

// GetCallTarget returns the target address of a CALL instruction.
// Returns (addr, true) if inst is a CALL with a relative (Rel) operand.
// Returns (0, false) otherwise.
func (d *X86Decoder) GetCallTarget(inst DecodedInstruction, instAddr uint64) (uint64, bool) {
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok {
		return 0, false
	}
	if x86inst.Op != x86asm.CALL {
		return 0, false
	}

	// Trim trailing nil arguments
	args := x86inst.Args[:]
	for len(args) > 0 && args[len(args)-1] == nil {
		args = args[:len(args)-1]
	}
	if len(args) == 0 {
		return 0, false
	}

	// Only handle relative calls (x86asm.Rel type)
	rel, ok := args[0].(x86asm.Rel)
	if !ok {
		return 0, false
	}

	// Check: instAddr + inst.Len won't overflow uint64
	if instAddr > math.MaxUint64-uint64(inst.Len) { //nolint:gosec // G115: Len validated non-negative
		return 0, false
	}
	nextPC := instAddr + uint64(inst.Len) //nolint:gosec // G115: overflow checked above

	// Check: nextPC fits in int64 for signed displacement calculation
	if nextPC > uint64(math.MaxInt64) {
		return 0, false
	}

	displacement := int64(nextPC) + int64(rel)
	if displacement < 0 {
		return 0, false
	}

	return uint64(displacement), true
}

// IsImmediateToFirstArgRegister returns (value, true) if the instruction
// sets the first argument register (RAX/EAX for x86_64 Go ABI) to an immediate.
// Returns (0, false) otherwise.
// Note: same as IsImmediateToSyscallNumberRegister for x86_64 (RAX is both syscall
// number register and first argument register in Go's register-based ABI).
func (d *X86Decoder) IsImmediateToFirstArgRegister(inst DecodedInstruction) (int64, bool) {
	ok, val := d.isImmediateToReg(inst, func(reg x86asm.Reg) bool {
		return reg == x86asm.RAX || reg == x86asm.EAX
	})
	return val, ok
}

// ModifiesThirdArgRegister checks if the instruction modifies edx or rdx.
func (d *X86Decoder) ModifiesThirdArgRegister(inst DecodedInstruction) bool {
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok {
		return false
	}

	if isReadOnlyFirstOperandOp(x86inst.Op) {
		return false
	}

	// Trim trailing nil arguments
	args := x86inst.Args[:]
	for len(args) > 0 && args[len(args)-1] == nil {
		args = args[:len(args)-1]
	}
	if len(args) == 0 {
		return false
	}

	// Check destination register (first argument for most instructions)
	if arg, ok := args[0].(x86asm.Reg); ok {
		return arg == x86asm.EDX || arg == x86asm.RDX ||
			arg == x86asm.DX || arg == x86asm.DL
	}

	return false
}

// IsImmediateToThirdArgRegister checks if the instruction sets edx/rdx to a known
// immediate value. Covers MOV EDX/RDX, imm and XOR EDX, EDX (zeroing idiom).
func (d *X86Decoder) IsImmediateToThirdArgRegister(inst DecodedInstruction) (bool, int64) {
	return d.isImmediateToReg(inst, func(reg x86asm.Reg) bool {
		return reg == x86asm.EDX || reg == x86asm.RDX
	})
}
