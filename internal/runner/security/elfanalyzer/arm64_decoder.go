package elfanalyzer

import (
	"encoding/binary"
	"errors"
	"math"

	"golang.org/x/arch/arm64/arm64asm"
)

// arm64InstructionLen is the fixed length of all arm64 instructions in bytes.
const arm64InstructionLen = 4

const (
	arm64PageMask                  = 0xfff
	arm64LoadSize32                = 4
	arm64LoadSize64                = 8
	arm64ADRPBacktrackLimit        = 4
	arm64LDRImm12Shift             = 10
	arm64LDRSizeShift              = 30
	arm64LDRImm12Mask       uint32 = 0x0fff
	arm64LDRSizeMask        uint32 = 0x3
	// arm64LDRSizeWord and arm64LDRSizeDWord are the expected values of bits[31:30]
	// in the LDR unsigned-offset encoding for 32-bit (W) and 64-bit (X) registers.
	arm64LDRSizeWord  uint32 = 2 // W registers: byte_size = 1 << 2 = 4
	arm64LDRSizeDWord uint32 = 3 // X registers: byte_size = 1 << 3 = 8
)

// ARM64Decoder implements MachineCodeDecoder for arm64.
type ARM64Decoder struct {
	dataSections []arm64DataSection
}

type arm64DataSection struct {
	Addr uint64
	Data []byte
}

// NewARM64Decoder creates a new ARM64Decoder.
func NewARM64Decoder() *ARM64Decoder { return &ARM64Decoder{} }

// SetDataSections sets readonly data regions used to resolve ADRP+LDR loads.
func (d *ARM64Decoder) SetDataSections(sections []arm64DataSection) {
	d.dataSections = sections
}

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

// ModifiesSyscallReg returns true if the instruction writes to
// the arm64 syscall number register (W8 or X8).
func (d *ARM64Decoder) ModifiesSyscallReg(inst DecodedInstruction) bool {
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

// IsSyscallNumImm returns (true, value) if inst sets
// W8 or X8 to a known immediate value.
// Handles two encodings:
//   - MOV W8/X8, #imm  (arm64asm normalises MOVZ to MOV)
//   - ORR W8/X8, WZR/XZR, #imm  (bitmask-immediate; functionally identical to MOV)
func (d *ARM64Decoder) IsSyscallNumImm(inst DecodedInstruction) (bool, int64) {
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

// IsFirstArgImm returns (true, value) if inst sets the arm64
// first argument register (X0 or W0) to an immediate.
// arm64 Go ABI uses X0 for the first integer argument.
// Handles two encodings:
//   - MOV X0/W0, #imm  (arm64asm normalises MOVZ to MOV)
//   - ORR X0/W0, XZR/WZR, #imm  (bitmask-immediate; functionally identical to MOV)
func (d *ARM64Decoder) IsFirstArgImm(inst DecodedInstruction) (bool, int64) {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok {
		return false, 0
	}
	if a.Op == arm64asm.MOV {
		if a.Args[0] == nil || a.Args[1] == nil {
			return false, 0
		}
		if !arm64MatchesReg(a.Args[0], arm64asm.X0) && !arm64MatchesReg(a.Args[0], arm64asm.W0) {
			return false, 0
		}
		val, ok := arm64ImmValue(a.Args[1])
		return ok, val
	}
	ok2, val := arm64OrrZeroRegImm(a, arm64asm.X0, arm64asm.W0)
	return ok2, val
}

// ModifiesFirstArg returns true if the instruction writes to
// the arm64 first syscall argument register (W0 or X0).
func (d *ARM64Decoder) ModifiesFirstArg(inst DecodedInstruction) bool {
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
	return arm64MatchesReg(a.Args[0], arm64asm.W0) || arm64MatchesReg(a.Args[0], arm64asm.X0)
}

// ResolveFirstArgGlobal resolves X0/W0 value for the pattern:
//
//	ADRP Xn, <page>
//	LDR  X0/W0, [Xn, #offset]
func (d *ARM64Decoder) ResolveFirstArgGlobal(recentInstructions []DecodedInstruction, idx int) (bool, int64) {
	if idx < 0 || idx >= len(recentInstructions) {
		return false, 0
	}
	if len(d.dataSections) == 0 {
		return false, 0
	}

	loadInfo, ok := d.decodeFirstArgGlobalLoad(recentInstructions[idx])
	if !ok {
		return false, 0
	}

	addr, ok := d.resolveADRPBacktrackAddress(recentInstructions, idx, loadInfo.base, loadInfo.offset)
	if !ok {
		return false, 0
	}

	val, ok := d.readResolvedFirstArg(addr, loadInfo.is64Bit)
	return ok, val
}

// ModifiesThirdArg returns true if the instruction writes to
// the arm64 third syscall argument register (W2 or X2).
func (d *ARM64Decoder) ModifiesThirdArg(inst DecodedInstruction) bool {
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

// IsThirdArgImm returns (true, value) if inst sets
// W2 or X2 to a known immediate value.
// Handles two encodings:
//   - MOV W2/X2, #imm  (arm64asm normalises MOVZ to MOV)
//   - ORR W2/X2, WZR/XZR, #imm  (bitmask-immediate; functionally identical to MOV)
func (d *ARM64Decoder) IsThirdArgImm(inst DecodedInstruction) (bool, int64) {
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

func arm64UnsignedOffsetFromEnc(enc uint32, is64Bit bool) (uint64, bool) {
	imm12 := (enc >> arm64LDRImm12Shift) & arm64LDRImm12Mask
	size := (enc >> arm64LDRSizeShift) & arm64LDRSizeMask
	expectedSize := arm64LDRSizeWord
	if is64Bit {
		expectedSize = arm64LDRSizeDWord
	}
	if size != expectedSize {
		return 0, false
	}
	return uint64(imm12) << size, true
}

func (d *ARM64Decoder) readUintAtVA(addr uint64, size int) (uint64, bool) {
	for _, sec := range d.dataSections {
		if addr < sec.Addr {
			continue
		}
		off := addr - sec.Addr
		if off > uint64(len(sec.Data)) {
			continue
		}
		start := int(off) //nolint:gosec // G115: off bounded by section length above
		if size > len(sec.Data)-start {
			continue
		}
		if size == arm64LoadSize64 {
			return binary.LittleEndian.Uint64(sec.Data[start : start+8]), true
		}
		if size == arm64LoadSize32 {
			return uint64(binary.LittleEndian.Uint32(sec.Data[start : start+4])), true
		}
	}
	return 0, false
}

type arm64LoadInfo struct {
	base    arm64asm.RegSP
	offset  uint64
	is64Bit bool
}

func (d *ARM64Decoder) decodeFirstArgGlobalLoad(inst DecodedInstruction) (arm64LoadInfo, bool) {
	a, ok := inst.arch.(arm64asm.Inst)
	if !ok || a.Op != arm64asm.LDR || a.Args[0] == nil || a.Args[1] == nil {
		return arm64LoadInfo{}, false
	}

	isX0 := arm64MatchesReg(a.Args[0], arm64asm.X0)
	isW0 := arm64MatchesReg(a.Args[0], arm64asm.W0)
	if !isX0 && !isW0 {
		return arm64LoadInfo{}, false
	}

	mem, ok := a.Args[1].(arm64asm.MemImmediate)
	if !ok || mem.Mode != arm64asm.AddrOffset {
		return arm64LoadInfo{}, false
	}

	loadEnc := binary.LittleEndian.Uint32(inst.Raw)
	offset, ok := arm64UnsignedOffsetFromEnc(loadEnc, isX0)
	if !ok {
		return arm64LoadInfo{}, false
	}

	info := arm64LoadInfo{base: mem.Base, offset: offset, is64Bit: isX0}
	return info, true
}

func (d *ARM64Decoder) resolveADRPBacktrackAddress(recentInstructions []DecodedInstruction, idx int, base arm64asm.RegSP, offset uint64) (uint64, bool) {
	for j := idx - 1; j >= 0 && idx-j <= arm64ADRPBacktrackLimit; j-- {
		prev := recentInstructions[j]
		if d.IsControlFlowInstruction(prev) {
			return 0, false
		}
		p, ok := prev.arch.(arm64asm.Inst)
		if !ok || p.Op != arm64asm.ADRP || p.Args[0] == nil || p.Args[1] == nil {
			continue
		}
		if !arm64MatchesReg(p.Args[0], arm64asm.Reg(base)) {
			continue
		}
		rel, ok := p.Args[1].(arm64asm.PCRel)
		if !ok {
			continue
		}
		return arm64ResolveADRPAddress(prev.Offset, rel, offset)
	}
	return 0, false
}

func arm64ResolveADRPAddress(instOffset uint64, rel arm64asm.PCRel, offset uint64) (uint64, bool) {
	pageBase := instOffset &^ uint64(arm64PageMask)
	if pageBase > math.MaxInt64 {
		return 0, false
	}
	pageBaseSigned := int64(pageBase)
	relSigned := int64(rel)
	// Check signed overflow: addition overflows when operands share the same sign
	// but the result has the opposite sign.
	if relSigned > 0 && pageBaseSigned > math.MaxInt64-relSigned {
		return 0, false
	}
	if relSigned < 0 && pageBaseSigned < math.MinInt64-relSigned {
		return 0, false
	}
	targetPage := pageBaseSigned + relSigned
	if targetPage < 0 {
		return 0, false
	}
	// Check unsigned overflow before adding the page offset.
	if offset > math.MaxUint64-uint64(targetPage) {
		return 0, false
	}
	return uint64(targetPage) + offset, true
}

func (d *ARM64Decoder) readResolvedFirstArg(addr uint64, is64Bit bool) (int64, bool) {
	if is64Bit {
		v, ok := d.readUintAtVA(addr, arm64LoadSize64)
		if !ok || v > math.MaxInt64 {
			return 0, false
		}
		return int64(v), true
	}

	v, ok := d.readUintAtVA(addr, arm64LoadSize32)
	if !ok {
		return 0, false
	}
	return int64(v), true //nolint:gosec // G115: v is zero-extended from uint32, always fits int64
}
