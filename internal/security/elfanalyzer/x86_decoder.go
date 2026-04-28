package elfanalyzer

import (
	"encoding/binary"
	"math"

	"golang.org/x/arch/x86/x86asm"
)

const (
	// decodeMode64 is the bit width for 64-bit mode decoding.
	decodeMode64 = 64

	// minArgsForImmediateMove is the minimum number of arguments
	// required to check for an immediate move instruction (destination + source).
	minArgsForImmediateMove = 2
)

const (
	x86ByteWidth32 = 4 // byte width of a 32-bit (EAX) load
	x86ByteWidth64 = 8 // byte width of a 64-bit (RAX) load
)

// x86LoadOp encodes the byte width and little-endian extraction logic for a
// global memory load. Construct via newX86LoadOp; the zero value is invalid.
type x86LoadOp struct {
	byteWidth int
}

// newX86LoadOp returns the load operation for a MOV EAX/RAX, [mem] instruction.
// EAX maps to a 32-bit zero-extending load; RAX maps to a 64-bit load.
// Returns (zero, false) for any other register.
func newX86LoadOp(reg x86asm.Reg) (x86LoadOp, bool) {
	switch reg {
	case x86asm.EAX:
		return x86LoadOp{byteWidth: x86ByteWidth32}, true
	case x86asm.RAX:
		return x86LoadOp{byteWidth: x86ByteWidth64}, true
	default:
		return x86LoadOp{}, false
	}
}

// readLE reads a little-endian integer of op's width from data[off:] and
// returns it as a signed int64.
func (op x86LoadOp) readLE(data []byte, off int) (int64, bool) {
	switch op.byteWidth {
	case x86ByteWidth32:
		return int64(binary.LittleEndian.Uint32(data[off : off+x86ByteWidth32])), true
	case x86ByteWidth64:
		val := binary.LittleEndian.Uint64(data[off : off+x86ByteWidth64])
		if val > math.MaxInt64 {
			return 0, false
		}
		return int64(val), true
	default:
		panic("unreachable: invalid x86LoadOp")
	}
}

// x86DataSection holds the virtual address and raw bytes of an ELF section
// used by ResolveFirstArgGlobal to read global variable values.
type x86DataSection struct {
	Addr uint64
	Data []byte
}

// X86Decoder implements MachineCodeDecoder for x86_64.
type X86Decoder struct {
	dataSections []x86DataSection
}

// NewX86Decoder creates a new X86Decoder.
func NewX86Decoder() *X86Decoder {
	return &X86Decoder{}
}

// Decode decodes a single x86_64 instruction.
func (d *X86Decoder) Decode(code []byte, offset uint64) (DecodedInstruction, error) {
	inst, err := x86asm.Decode(code, decodeMode64)
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

// isFirstOperandReadOnly reports whether op uses its first operand as a
// read-only source and does not write to it. Such instructions must not be
// treated as register modifications during backward scanning.
// This covers the most common cases; control-flow ops (CALL, JMP, etc.) are
// handled separately by IsControlFlowInstruction.
func isFirstOperandReadOnly(op x86asm.Op) bool {
	switch op {
	case x86asm.PUSH, x86asm.CMP, x86asm.TEST, x86asm.BT:
		return true
	}
	return false
}

// writesRAXImplicitly reports whether the instruction unconditionally writes
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
func writesRAXImplicitly(x86inst x86asm.Inst) bool {
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

type x86RegFamily int

const (
	x86RegFamilyUnknown x86RegFamily = iota
	x86RegFamilyAX
	x86RegFamilyCX
	x86RegFamilyDX
	x86RegFamilyBX
	x86RegFamilySP
	x86RegFamilyBP
	x86RegFamilySI
	x86RegFamilyDI
	x86RegFamilyR8
	x86RegFamilyR9
	x86RegFamilyR10
	x86RegFamilyR11
	x86RegFamilyR12
	x86RegFamilyR13
	x86RegFamilyR14
	x86RegFamilyR15
)

func regFamily(reg x86asm.Reg) x86RegFamily {
	switch reg {
	case x86asm.AL, x86asm.AH, x86asm.AX, x86asm.EAX, x86asm.RAX:
		return x86RegFamilyAX
	case x86asm.CL, x86asm.CH, x86asm.CX, x86asm.ECX, x86asm.RCX:
		return x86RegFamilyCX
	case x86asm.DL, x86asm.DH, x86asm.DX, x86asm.EDX, x86asm.RDX:
		return x86RegFamilyDX
	case x86asm.BL, x86asm.BH, x86asm.BX, x86asm.EBX, x86asm.RBX:
		return x86RegFamilyBX
	case x86asm.SPB, x86asm.SP, x86asm.ESP, x86asm.RSP:
		return x86RegFamilySP
	case x86asm.BPB, x86asm.BP, x86asm.EBP, x86asm.RBP:
		return x86RegFamilyBP
	case x86asm.SIB, x86asm.SI, x86asm.ESI, x86asm.RSI:
		return x86RegFamilySI
	case x86asm.DIB, x86asm.DI, x86asm.EDI, x86asm.RDI:
		return x86RegFamilyDI
	case x86asm.R8B, x86asm.R8W, x86asm.R8L, x86asm.R8:
		return x86RegFamilyR8
	case x86asm.R9B, x86asm.R9W, x86asm.R9L, x86asm.R9:
		return x86RegFamilyR9
	case x86asm.R10B, x86asm.R10W, x86asm.R10L, x86asm.R10:
		return x86RegFamilyR10
	case x86asm.R11B, x86asm.R11W, x86asm.R11L, x86asm.R11:
		return x86RegFamilyR11
	case x86asm.R12B, x86asm.R12W, x86asm.R12L, x86asm.R12:
		return x86RegFamilyR12
	case x86asm.R13B, x86asm.R13W, x86asm.R13L, x86asm.R13:
		return x86RegFamilyR13
	case x86asm.R14B, x86asm.R14W, x86asm.R14L, x86asm.R14:
		return x86RegFamilyR14
	case x86asm.R15B, x86asm.R15W, x86asm.R15L, x86asm.R15:
		return x86RegFamilyR15
	default:
		return x86RegFamilyUnknown
	}
}

func isFullWidthWrite(reg x86asm.Reg) bool {
	switch reg {
	case x86asm.EAX, x86asm.RAX,
		x86asm.ECX, x86asm.RCX,
		x86asm.EDX, x86asm.RDX,
		x86asm.EBX, x86asm.RBX,
		x86asm.ESP, x86asm.RSP,
		x86asm.EBP, x86asm.RBP,
		x86asm.ESI, x86asm.RSI,
		x86asm.EDI, x86asm.RDI,
		x86asm.R8L, x86asm.R8,
		x86asm.R9L, x86asm.R9,
		x86asm.R10L, x86asm.R10,
		x86asm.R11L, x86asm.R11,
		x86asm.R12L, x86asm.R12,
		x86asm.R13L, x86asm.R13,
		x86asm.R14L, x86asm.R14,
		x86asm.R15L, x86asm.R15:
		return true
	}

	return false
}

func sameFamily(a, b x86asm.Reg) bool {
	aFamily := regFamily(a)
	if aFamily == x86RegFamilyUnknown {
		return false
	}
	return aFamily == regFamily(b)
}

// WritesSyscallReg returns true if the instruction writes to the RAX register
// family (AL/AH/AX/EAX/RAX), which is the syscall number register on x86_64.
func (d *X86Decoder) WritesSyscallReg(inst DecodedInstruction) bool {
	return d.WritesRegisterFamily(inst, x86asm.RAX)
}

// WritesRegisterFamily reports whether the instruction writes to the same
// register family as targetReg (e.g. EAX and RAX are in the same family).
func (d *X86Decoder) WritesRegisterFamily(inst DecodedInstruction, targetReg x86asm.Reg) bool {
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok {
		return false
	}

	if isFirstOperandReadOnly(x86inst.Op) {
		return false
	}

	// Instructions that implicitly write RAX/EAX without it appearing as the
	// first explicit operand (e.g. MUL, one-operand IMUL, DIV, IDIV, CPUID).
	if sameFamily(targetReg, x86asm.RAX) && writesRAXImplicitly(x86inst) {
		return true
	}
	// Instructions that implicitly write RDX/EDX (e.g. MUL, DIV, CPUID, CQO/CDQ/CWD).
	if sameFamily(targetReg, x86asm.RDX) && writesRDXImplicitly(x86inst) {
		return true
	}
	// CPUID also implicitly writes EBX and ECX.
	if (sameFamily(targetReg, x86asm.RBX) || sameFamily(targetReg, x86asm.RCX)) && x86inst.Op == x86asm.CPUID {
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
		return sameFamily(arg, targetReg)
	}

	return false
}

// IsSyscallNumImm checks if the instruction sets eax/rax to a known immediate value.
// This covers two common compiler patterns:
//   - MOV EAX/RAX, <imm>  — direct immediate load
//   - XOR EAX, EAX        — idiom for zeroing EAX (equivalent to MOV EAX, 0)
func (d *X86Decoder) IsSyscallNumImm(inst DecodedInstruction) (bool, int64) {
	return d.IsImmToRegisterFamily(inst, x86asm.RAX)
}

// IsImmToRegisterFamily returns (true, value) when the instruction sets
// the target register family from an immediate value or a self-XOR zeroing idiom.
func (d *X86Decoder) IsImmToRegisterFamily(inst DecodedInstruction, targetReg x86asm.Reg) (bool, int64) {
	return d.isImmToReg(inst, func(reg x86asm.Reg) bool {
		return sameFamily(reg, targetReg) && isFullWidthWrite(reg)
	})
}

// CopySourceForRegFamily reports source register when instruction is a
// simple register copy into targetReg family (e.g. MOV EAX, EDX).
func (d *X86Decoder) CopySourceForRegFamily(inst DecodedInstruction, targetReg x86asm.Reg) (x86asm.Reg, bool) {
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok || x86inst.Op != x86asm.MOV {
		return 0, false
	}

	args := x86inst.Args[:]
	for len(args) > 0 && args[len(args)-1] == nil {
		args = args[:len(args)-1]
	}
	if len(args) < minArgsForImmediateMove {
		return 0, false
	}

	destReg, ok := args[0].(x86asm.Reg)
	if !ok || !sameFamily(destReg, targetReg) || !isFullWidthWrite(destReg) {
		return 0, false
	}

	srcReg, ok := args[1].(x86asm.Reg)
	if !ok || !isFullWidthWrite(srcReg) {
		return 0, false
	}

	return srcReg, true
}

// isImmToReg is an internal helper that checks if an instruction sets
// a register (matched by regMatch) to an immediate value.
// Covers MOV <reg>, <imm> and XOR <reg>, <reg> (zeroing idiom).
func (d *X86Decoder) isImmToReg(inst DecodedInstruction, regMatch func(x86asm.Reg) bool) (bool, int64) {
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

// IsFirstArgImm returns (value, true) if the instruction
// sets the first argument register (RAX/EAX for x86_64 Go ABI) to an immediate.
// Returns (0, false) otherwise.
// Note: same as IsSyscallNumImm for x86_64 (RAX is both syscall
// number register and first argument register in Go's register-based ABI).
func (d *X86Decoder) IsFirstArgImm(inst DecodedInstruction) (bool, int64) {
	ok, val := d.isImmToReg(inst, func(reg x86asm.Reg) bool {
		return reg == x86asm.RAX || reg == x86asm.EAX
	})
	return ok, val
}

// ModifiesFirstArg returns true if the instruction writes to the
// first argument register in x86_64 Go ABI (RAX/EAX).
func (d *X86Decoder) ModifiesFirstArg(inst DecodedInstruction) bool {
	return d.WritesSyscallReg(inst)
}

// SetDataSections supplies read-only and read-write ELF data sections used
// by ResolveFirstArgGlobal to read global variable values.
func (d *X86Decoder) SetDataSections(sections []x86DataSection) {
	d.dataSections = sections
}

// readGlobal reads a little-endian integer from the data sections at addr
// using the width and extraction logic encoded in op.
func (d *X86Decoder) readGlobal(addr uint64, op x86LoadOp) (int64, bool) {
	for _, sec := range d.dataSections {
		if addr < sec.Addr {
			continue
		}
		off := addr - sec.Addr
		secLen := uint64(len(sec.Data))                        //nolint:gosec // G115: section sizes are bounded by binary size
		if off > secLen || uint64(op.byteWidth) > secLen-off { //nolint:gosec // G115: byteWidth is 4 or 8
			continue
		}
		return op.readLE(sec.Data, int(off)) //nolint:gosec // G115: off <= secLen-uint64(op), so off fits in int
	}
	return 0, false
}

// ResolveFirstArgGlobal resolves the syscall number when it is loaded into
// RAX/EAX via a RIP-relative memory read, i.e. the pattern:
//
//	MOV RAX, [RIP + disp32]
//
// This occurs in Go's syscall package when the syscall number is stored in a
// package-level variable (e.g. syscall.fcntl64Syscall in forkAndExecInChild1).
// SetDataSections must be called before this method returns useful results.
func (d *X86Decoder) ResolveFirstArgGlobal(recentInstructions []DecodedInstruction, idx int) (bool, int64) {
	if len(d.dataSections) == 0 || idx < 0 || idx >= len(recentInstructions) {
		return false, 0
	}

	inst := recentInstructions[idx]
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok || x86inst.Op != x86asm.MOV {
		return false, 0
	}

	args := x86inst.Args[:]
	for len(args) > 0 && args[len(args)-1] == nil {
		args = args[:len(args)-1]
	}
	if len(args) < minArgsForImmediateMove {
		return false, 0
	}

	destReg, ok := args[0].(x86asm.Reg)
	if !ok || !sameFamily(destReg, x86asm.RAX) || !isFullWidthWrite(destReg) {
		return false, 0
	}
	op, ok := newX86LoadOp(destReg)
	if !ok {
		return false, 0
	}

	mem, ok := args[1].(x86asm.Mem)
	if !ok || mem.Base != x86asm.RIP || mem.Index != 0 || mem.Scale != 0 {
		return false, 0
	}

	target, ok := x86RIPRelAddr(inst.Offset, inst.Len, mem.Disp)
	if !ok {
		return false, 0
	}

	val, ok := d.readGlobal(target, op)
	return ok, val
}

// x86RIPRelAddr computes the effective address for a RIP-relative memory
// operand, guarding against uint64 wraparound and int64 overflow/underflow.
//
// nextPC = instOffset + instLen
// target = nextPC + sign_extend(disp32)
func x86RIPRelAddr(instOffset uint64, instLen int, rawDisp int64) (uint64, bool) {
	// Guard uint64 overflow on nextPC.
	if instOffset > math.MaxUint64-uint64(instLen) { //nolint:gosec // G115: instLen validated non-negative
		return 0, false
	}
	nextPC := instOffset + uint64(instLen) //nolint:gosec // G115: overflow checked above
	// nextPC must fit in int64 for signed displacement arithmetic.
	if nextPC > math.MaxInt64 {
		return 0, false
	}

	// x86asm stores disp32 as zero-extended in int64, not sign-extended.
	// Re-interpret through int32 to recover the correct signed value.
	dispSigned := int64(int32(rawDisp)) //nolint:gosec // G115: intentional int32 reinterpretation for sign extension

	// In Go, signed int64 arithmetic wraps on overflow. Both positive overflow
	// (nextPC near MaxInt64, large positive disp) and negative underflow
	// (disp too negative) produce a negative result, so a single < 0 check
	// catches both cases.
	target := int64(nextPC) + dispSigned
	if target < 0 {
		return 0, false
	}
	return uint64(target), true
}

// writesRDXImplicitly reports whether the instruction unconditionally writes
// to RDX/EDX as an implicit (unlisted) destination.
//
// Covered cases:
//   - MUL r/m  — unsigned multiply: rDX:rAX = rAX × operand; high half → rDX
//   - IMUL r/m — one-operand signed multiply: same layout as MUL;
//     distinguished from multi-operand IMUL by having exactly one non-nil arg
//   - DIV r/m  — unsigned divide: remainder → rDX
//   - IDIV r/m — signed divide: remainder → rDX
//   - CPUID    — writes EDX (along with EAX/EBX/ECX); no explicit operands
//   - CQO      — sign-extends RAX into RDX:RAX; writes RDX
//   - CDQ      — sign-extends EAX into EDX:EAX; writes EDX
//   - CWD      — sign-extends AX into DX:AX; writes DX
func writesRDXImplicitly(x86inst x86asm.Inst) bool {
	switch x86inst.Op {
	case x86asm.MUL, x86asm.DIV, x86asm.IDIV, x86asm.CPUID, x86asm.CQO, x86asm.CDQ, x86asm.CWD:
		return true
	case x86asm.IMUL:
		// Only the one-operand form writes the high half into rDX.
		// Two/three-operand forms write only to the explicit destination register.
		args := x86inst.Args[:]
		for len(args) > 0 && args[len(args)-1] == nil {
			args = args[:len(args)-1]
		}
		return len(args) == 1
	}
	return false
}

// ModifiesThirdArg checks if the instruction modifies edx or rdx.
func (d *X86Decoder) ModifiesThirdArg(inst DecodedInstruction) bool {
	x86inst, ok := inst.arch.(x86asm.Inst)
	if !ok {
		return false
	}

	if isFirstOperandReadOnly(x86inst.Op) {
		return false
	}

	// Instructions that implicitly write RDX/EDX without it appearing as the
	// first explicit operand (e.g. MUL, one-operand IMUL, DIV, IDIV, CQO/CDQ/CWD).
	if writesRDXImplicitly(x86inst) {
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

	// Check destination register (first argument for most instructions).
	// DH is included: a write to the high byte of DX overlaps EDX/RDX.
	if arg, ok := args[0].(x86asm.Reg); ok {
		return arg == x86asm.EDX || arg == x86asm.RDX ||
			arg == x86asm.DX || arg == x86asm.DL || arg == x86asm.DH
	}

	return false
}

// IsThirdArgImm checks if the instruction sets edx/rdx to a known
// immediate value. Covers MOV EDX/RDX, imm and XOR EDX, EDX (zeroing idiom).
func (d *X86Decoder) IsThirdArgImm(inst DecodedInstruction) (bool, int64) {
	return d.isImmToReg(inst, func(reg x86asm.Reg) bool {
		return reg == x86asm.EDX || reg == x86asm.RDX
	})
}
