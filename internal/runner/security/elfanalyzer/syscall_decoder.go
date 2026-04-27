package elfanalyzer

// DecodedInstruction represents a decoded machine instruction.
// The arch field stores architecture-specific decoded data and is only
// accessed by the corresponding decoder implementation via type assertion.
// External consumers (SyscallAnalyzer, GoWrapperResolver) must not access
// the arch field directly; they use MachineCodeDecoder interface methods or
// decoder-specific helper methods instead.
type DecodedInstruction struct {
	// Offset is the instruction's virtual address (e.g., section base VA plus
	// section-relative offset) corresponding to the first byte of this
	// instruction.
	Offset uint64

	// Len is the instruction length in bytes.
	Len int

	// Raw contains the raw instruction bytes.
	Raw []byte

	// arch stores architecture-specific decoded instruction.
	// X86Decoder stores x86asm.Inst; ARM64Decoder stores arm64asm.Inst.
	arch any
}

// MachineCodeDecoder defines the interface for decoding machine code.
// Implementations exist for x86_64 (X86Decoder) and arm64 (ARM64Decoder).
type MachineCodeDecoder interface {
	// Decode decodes a single instruction at the given offset from code.
	// On failure, returns a zero-value DecodedInstruction and an error.
	Decode(code []byte, offset uint64) (DecodedInstruction, error)

	// IsSyscallInstruction returns true if the instruction is a syscall.
	// x86_64: SYSCALL opcode (0F 05)
	// arm64:  SVC #0 (D4000001)
	IsSyscallInstruction(inst DecodedInstruction) bool

	// WritesSyscallReg returns true if the instruction writes
	// to the architecture's syscall number register.
	// x86_64: any write to any subregister of RAX (AL/AH/AX/EAX/RAX)
	// arm64:  w8 or x8
	WritesSyscallReg(inst DecodedInstruction) bool

	// IsSyscallNumImm returns (true, value) if the
	// instruction sets the syscall number register to a known immediate.
	// x86_64: MOV EAX/RAX, imm  or  XOR EAX, EAX (zeroing idiom)
	// arm64:  MOV W8/X8, #imm  (arm64asm normalizes MOVZ to MOV)
	//         ORR W8/X8, WZR/XZR, #imm  (bitmask-immediate encoding)
	IsSyscallNumImm(inst DecodedInstruction) (bool, int64)

	// IsControlFlowInstruction returns true if the instruction changes the
	// instruction pointer in a way that may skip over the syscall number setup.
	// x86_64: JMP*, CALL, RET, IRET, INT, LOOP*, Jcc, JCXZ*
	// arm64:  B, BL, BLR, BR, RET, CBZ, CBNZ, TBZ, TBNZ
	IsControlFlowInstruction(inst DecodedInstruction) bool

	// InstructionAlignment returns the number of bytes to skip when a decode
	// failure occurs, and the granularity for instruction boundaries.
	// x86_64: 1 (variable-length instructions, byte-by-byte recovery)
	// arm64:  4 (fixed-length 4-byte instructions)
	InstructionAlignment() int

	// MaxInstructionLength returns the maximum possible byte length of a single
	// instruction for this architecture. Used to size the backward-scan window.
	// x86_64: 15 (the architectural maximum for x86_64 instructions)
	// arm64:  4  (all instructions are exactly 4 bytes)
	MaxInstructionLength() int

	// GetCallTarget returns the target address of a call instruction (CALL on
	// x86_64, BL on arm64). instAddr is the virtual address of inst.
	// Returns (addr, true) on success, (0, false) otherwise.
	GetCallTarget(inst DecodedInstruction, instAddr uint64) (uint64, bool)

	// IsFirstArgImm returns (true, value) if the instruction
	// sets the first integer argument register to a known immediate.
	// x86_64: MOV EAX/RAX, imm  (RAX is the first argument register in Go ABI)
	// arm64:  MOV W0/X0, #imm   (X0 is the first argument register in Go ABI)
	//         ORR X0/W0, XZR/WZR, #imm  (bitmask-immediate encoding)
	// Returns (false, 0) otherwise.
	IsFirstArgImm(inst DecodedInstruction) (bool, int64)

	// ModifiesFirstArg returns true if the instruction writes to the
	// first integer argument register.
	// x86_64: eax/rax (same as syscall number register in Go ABI)
	// arm64:  w0 or x0
	ModifiesFirstArg(inst DecodedInstruction) bool

	// ResolveFirstArgGlobal tries to resolve the first argument value
	// when it is loaded from memory, using nearby context in recentInstructions.
	// idx is the index of the candidate load instruction in recentInstructions.
	// Returns (true, value) if resolved, (false, 0) otherwise.
	ResolveFirstArgGlobal(recentInstructions []DecodedInstruction, idx int) (bool, int64)

	// ModifiesThirdArg returns true if the instruction writes to the
	// third syscall argument register.
	// x86_64: edx/rdx (any write including dl, dx, edx/rdx)
	// arm64:  w2 or x2
	ModifiesThirdArg(inst DecodedInstruction) bool

	// IsThirdArgImm returns (true, value) if the instruction
	// sets the third argument register to a known immediate.
	// x86_64: MOV EDX/RDX, imm  or  XOR EDX, EDX (zeroing idiom)
	// arm64:  MOV W2/X2, #imm
	//         ORR W2/X2, WZR/XZR, #imm  (bitmask-immediate encoding)
	IsThirdArgImm(inst DecodedInstruction) (bool, int64)
}
