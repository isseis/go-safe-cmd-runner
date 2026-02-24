package elfanalyzer

import (
	"golang.org/x/arch/x86/x86asm"
)

const (
	// x86_64BitMode is the bit width for 64-bit mode decoding.
	x86_64BitMode = 64

	// minArgsForImmediateMove is the minimum number of arguments
	// required to check for an immediate move instruction (destination + source).
	minArgsForImmediateMove = 2
)

// DecodedInstruction represents a decoded x86_64 instruction.
type DecodedInstruction struct {
	// Offset is the instruction's virtual address (e.g., section base VA plus
	// section-relative offset) corresponding to the first byte of this
	// instruction.
	Offset uint64

	// Len is the instruction length in bytes.
	Len int

	// Op is the instruction opcode (e.g., MOV, SYSCALL).
	Op x86asm.Op

	// Args are the instruction arguments.
	Args []x86asm.Arg

	// Raw contains the raw instruction bytes.
	Raw []byte
}

// MachineCodeDecoder defines the interface for decoding machine code.
type MachineCodeDecoder interface {
	// Decode decodes a single instruction at the given offset.
	Decode(code []byte, offset uint64) (DecodedInstruction, error)

	// IsSyscallInstruction returns true if the instruction is a syscall.
	IsSyscallInstruction(inst DecodedInstruction) bool

	// ModifiesEAXorRAX returns true if the instruction modifies eax or rax.
	ModifiesEAXorRAX(inst DecodedInstruction) bool

	// IsImmediateMove returns (true, value) if the instruction moves an immediate to eax/rax.
	IsImmediateMove(inst DecodedInstruction) (bool, int64)

	// IsControlFlowInstruction returns true if the instruction is a control flow instruction.
	IsControlFlowInstruction(inst DecodedInstruction) bool
}

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

	// Trim trailing nil arguments (x86asm.Arg is an interface, unused slots are nil)
	args := inst.Args[:]
	for len(args) > 0 && args[len(args)-1] == nil {
		args = args[:len(args)-1]
	}

	decoded := DecodedInstruction{
		Offset: offset,
		Len:    inst.Len,
		Op:     inst.Op,
		Args:   args,
		Raw:    code[:inst.Len],
	}

	return decoded, nil
}

// IsSyscallInstruction checks if the instruction is a syscall.
func (d *X86Decoder) IsSyscallInstruction(inst DecodedInstruction) bool {
	return inst.Op == x86asm.SYSCALL
}

// ModifiesEAXorRAX checks if the instruction modifies eax or rax.
func (d *X86Decoder) ModifiesEAXorRAX(inst DecodedInstruction) bool {
	if len(inst.Args) == 0 {
		return false
	}

	// Check destination register (first argument for most instructions)
	if arg, ok := inst.Args[0].(x86asm.Reg); ok {
		return arg == x86asm.EAX || arg == x86asm.RAX ||
			arg == x86asm.AX || arg == x86asm.AL
	}

	return false
}

// IsImmediateMove checks if the instruction sets eax/rax to a known immediate value.
// This covers two common compiler patterns:
//   - MOV EAX/RAX, <imm>  — direct immediate load
//   - XOR EAX, EAX        — idiom for zeroing EAX (equivalent to MOV EAX, 0)
func (d *X86Decoder) IsImmediateMove(inst DecodedInstruction) (bool, int64) {
	if len(inst.Args) < minArgsForImmediateMove {
		return false, 0
	}

	destReg, ok := inst.Args[0].(x86asm.Reg)
	if !ok {
		return false, 0
	}
	if destReg != x86asm.EAX && destReg != x86asm.RAX {
		return false, 0
	}

	switch inst.Op {
	case x86asm.MOV:
		// MOV EAX/RAX, <imm>
		if src, ok := inst.Args[1].(x86asm.Imm); ok {
			return true, int64(src)
		}
	case x86asm.XOR:
		// XOR EAX, EAX (or XOR RAX, RAX) — sets register to 0.
		// Only match when both operands are the same register (self-XOR idiom).
		if srcReg, ok := inst.Args[1].(x86asm.Reg); ok && srcReg == destReg {
			return true, 0
		}
	}

	return false, 0
}

// IsControlFlowInstruction checks if the instruction is a control flow instruction.
func (d *X86Decoder) IsControlFlowInstruction(inst DecodedInstruction) bool {
	switch inst.Op {
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
