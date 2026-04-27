//go:build test

package elfanalyzer

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/arch/arm64/arm64asm"
)

// Verified arm64 instruction byte sequences (little-endian, 4 bytes each):
//   svc  #0           : 01 00 00 D4
//   mov  w8, #198     : C8 18 80 52  (MOVZ W8, #198)
//   mov  x8, #198     : C8 18 80 D2  (MOVZ X8, #198)
//   mov  w8, #41      : 28 05 80 52  (MOVZ W8, #41; used as an arbitrary non-network immediate)
//   mov  x0, #41      : 20 05 80 D2  (MOVZ X0, #41)
//   mov  w0, #198     : C0 18 80 52  (MOVZ W0, #198)
//   bl  +8            : 02 00 00 94  (offset = instAddr + 8; BL with PCRel=8)
//   b   +8            : 02 00 00 14
//   blr  x1           : 20 00 3F D6
//   br   x1           : 20 00 1F D6
//   ret               : C0 03 5F D6  (RET X30)
//   cbz  x0, +8       : 40 00 00 B4
//   cbnz x0, +8       : 40 00 00 B5
//   tbz  w0, #0, +8   : 40 00 00 36
//   tbnz w0, #0, +8   : 40 00 00 37
//   nop               : 1F 20 03 D5
//   add  x8, x0, x1   : 08 00 01 8B

func TestARM64Decoder_Decode(t *testing.T) {
	decoder := NewARM64Decoder()

	t.Run("svc #0 (syscall)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x01, 0x00, 0x00, 0xD4}, 0x1000)
		require.NoError(t, err)
		assert.Equal(t, 4, inst.Len)
		assert.Equal(t, uint64(0x1000), inst.Offset)
		assert.NotNil(t, inst.arch)
		assert.True(t, decoder.IsSyscallInstruction(inst))
	})

	t.Run("mov w8, #198 (set syscall number register)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xC8, 0x18, 0x80, 0x52}, 0x1004)
		require.NoError(t, err)
		assert.Equal(t, 4, inst.Len)
		assert.False(t, decoder.IsSyscallInstruction(inst))
	})

	t.Run("ret (control flow)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xC0, 0x03, 0x5F, 0xD6}, 0x2000)
		require.NoError(t, err)
		assert.Equal(t, 4, inst.Len)
		assert.True(t, decoder.IsControlFlowInstruction(inst))
	})

	t.Run("bl +8 (call)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x02, 0x00, 0x00, 0x94}, 0x1000)
		require.NoError(t, err)
		assert.Equal(t, 4, inst.Len)
		assert.True(t, decoder.IsControlFlowInstruction(inst))
	})

	t.Run("short code slice returns error", func(t *testing.T) {
		_, err := decoder.Decode([]byte{0x01, 0x00, 0x00}, 0)
		assert.Error(t, err)
	})
}

func TestARM64Decoder_IsSyscallInstruction(t *testing.T) {
	decoder := NewARM64Decoder()

	tests := []struct {
		name string
		code []byte
		want bool
	}{
		{"svc #0", []byte{0x01, 0x00, 0x00, 0xD4}, true},
		{"mov w8, #198", []byte{0xC8, 0x18, 0x80, 0x52}, false},
		{"ret", []byte{0xC0, 0x03, 0x5F, 0xD6}, false},
		{"nop", []byte{0x1F, 0x20, 0x03, 0xD5}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoder.IsSyscallInstruction(inst))
		})
	}
}

func TestARM64Decoder_ModifiesSyscallReg(t *testing.T) {
	decoder := NewARM64Decoder()

	tests := []struct {
		name string
		code []byte
		want bool
	}{
		// Instructions that write to W8/X8 (syscall number register)
		{"mov w8, #198", []byte{0xC8, 0x18, 0x80, 0x52}, true},
		{"mov x8, #198", []byte{0xC8, 0x18, 0x80, 0xD2}, true},
		{"add x8, x0, x1", []byte{0x08, 0x00, 0x01, 0x8B}, true},
		// orr x8, xzr, #0x38 — bitmask immediate; arm64asm uses RegSP for the
		// destination, which must be handled alongside the regular Reg type.
		{"orr x8, xzr, #0x38 (bitmask imm, RegSP dest)", []byte{0xe8, 0x0b, 0x7d, 0xb2}, true},
		// Instructions that do NOT write to W8/X8
		{"svc #0", []byte{0x01, 0x00, 0x00, 0xD4}, false},
		{"mov x0, #41", []byte{0x20, 0x05, 0x80, 0xD2}, false},
		{"nop", []byte{0x1F, 0x20, 0x03, 0xD5}, false},
		{"ret", []byte{0xC0, 0x03, 0x5F, 0xD6}, false},
		// Read-only first operand: must not be mistaken for writes to X8
		// str x8, [x0]  — stores x8 to memory, does not modify x8
		{"str x8, [x0]", []byte{0x08, 0x00, 0x00, 0xF9}, false},
		// cmp x8, #5   — SUBS XZR, X8, #5, sets flags only
		{"cmp x8, #5", []byte{0x1F, 0x15, 0x00, 0xF1}, false},
		// tst x8, x1   — ANDS XZR, X8, X1, sets flags only
		{"tst x8, x1", []byte{0x1F, 0x01, 0x01, 0xEA}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoder.ModifiesSyscallReg(inst))
		})
	}
}

func TestARM64Decoder_IsSyscallNumImm(t *testing.T) {
	decoder := NewARM64Decoder()

	// Verified ORR-immediate encodings (little-endian, bitmask immediate):
	//   orr x8, xzr, #0x38  : e8 0b 7d b2  (open syscall, #56)
	//   orr x8, xzr, #0x3f  : e8 17 40 b2  (read syscall, #63)
	//   orr x8, xzr, #0x40  : e8 03 7a b2  (write syscall, #64)
	//   orr x8, xzr, #0x7c  : e8 13 7e b2  (vgetrandom syscall, #124)
	// These bitmask-immediate values cannot be encoded as 16-bit MOVZ immediates,
	// so the assembler emits ORR instead of MOV.
	tests := []struct {
		name    string
		code    []byte
		wantImm bool
		wantVal int64
	}{
		{"mov w8, #198 (socket)", []byte{0xC8, 0x18, 0x80, 0x52}, true, 198},
		{"mov x8, #198", []byte{0xC8, 0x18, 0x80, 0xD2}, true, 198},
		{"mov w8, #41 (connect-like)", []byte{0x28, 0x05, 0x80, 0x52}, true, 41},
		{"mov w8, #63 (read)", []byte{0xE8, 0x07, 0x80, 0x52}, true, 63},
		// ORR x8/w8, xzr/wzr, #imm (bitmask-immediate encoding, same effect as MOV)
		{"orr x8, xzr, #0x38 (open, bitmask imm)", []byte{0xe8, 0x0b, 0x7d, 0xb2}, true, 0x38},
		{"orr x8, xzr, #0x3f (read, bitmask imm)", []byte{0xe8, 0x17, 0x40, 0xb2}, true, 0x3f},
		{"orr x8, xzr, #0x40 (write, bitmask imm)", []byte{0xe8, 0x03, 0x7a, 0xb2}, true, 0x40},
		{"orr x8, xzr, #0x7c (bitmask imm)", []byte{0xe8, 0x13, 0x7e, 0xb2}, true, 0x7c},
		// Non-matching cases
		{"svc #0 (not mov)", []byte{0x01, 0x00, 0x00, 0xD4}, false, 0},
		{"mov x0, #41 (wrong register)", []byte{0x20, 0x05, 0x80, 0xD2}, false, 0},
		{"add x8, x0, x1 (not immediate)", []byte{0x08, 0x00, 0x01, 0x8B}, false, 0},
		{"nop", []byte{0x1F, 0x20, 0x03, 0xD5}, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			gotImm, gotVal := decoder.IsSyscallNumImm(inst)
			assert.Equal(t, tt.wantImm, gotImm)
			if tt.wantImm {
				assert.Equal(t, tt.wantVal, gotVal)
			}
		})
	}
}

func TestARM64Decoder_IsControlFlowInstruction(t *testing.T) {
	decoder := NewARM64Decoder()

	tests := []struct {
		name string
		code []byte
		want bool
	}{
		// Control flow instructions
		{"b +8", []byte{0x02, 0x00, 0x00, 0x14}, true},
		{"bl +8", []byte{0x02, 0x00, 0x00, 0x94}, true},
		{"blr x1", []byte{0x20, 0x00, 0x3F, 0xD6}, true},
		{"br x1", []byte{0x20, 0x00, 0x1F, 0xD6}, true},
		{"ret", []byte{0xC0, 0x03, 0x5F, 0xD6}, true},
		{"cbz x0, +8", []byte{0x40, 0x00, 0x00, 0xB4}, true},
		{"cbnz x0, +8", []byte{0x40, 0x00, 0x00, 0xB5}, true},
		{"tbz w0, #0, +8", []byte{0x40, 0x00, 0x00, 0x36}, true},
		{"tbnz w0, #0, +8", []byte{0x40, 0x00, 0x00, 0x37}, true},
		// Non-control-flow instructions
		{"svc #0", []byte{0x01, 0x00, 0x00, 0xD4}, false},
		{"mov w8, #198", []byte{0xC8, 0x18, 0x80, 0x52}, false},
		{"nop", []byte{0x1F, 0x20, 0x03, 0xD5}, false},
		{"add x8, x0, x1", []byte{0x08, 0x00, 0x01, 0x8B}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoder.IsControlFlowInstruction(inst))
		})
	}
}

func TestARM64Decoder_InstructionAlignment(t *testing.T) {
	decoder := NewARM64Decoder()
	assert.Equal(t, 4, decoder.InstructionAlignment())
}

func TestARM64Decoder_GetCallTarget(t *testing.T) {
	decoder := NewARM64Decoder()

	t.Run("bl +8 at address 0x1000", func(t *testing.T) {
		// bl +8: PCRel = 8, target = instAddr + 8 = 0x1000 + 8 = 0x1008
		inst, err := decoder.Decode([]byte{0x02, 0x00, 0x00, 0x94}, 0x1000)
		require.NoError(t, err)
		target, ok := decoder.GetCallTarget(inst, 0x1000)
		assert.True(t, ok)
		assert.Equal(t, uint64(0x1008), target)
	})

	t.Run("non-BL instruction returns false", func(t *testing.T) {
		// mov w8, #198 is not BL
		inst, err := decoder.Decode([]byte{0xC8, 0x18, 0x80, 0x52}, 0x1000)
		require.NoError(t, err)
		_, ok := decoder.GetCallTarget(inst, 0x1000)
		assert.False(t, ok)
	})

	t.Run("blr (indirect call) returns false", func(t *testing.T) {
		// blr x1 uses a register, not PCRel
		inst, err := decoder.Decode([]byte{0x20, 0x00, 0x3F, 0xD6}, 0x1000)
		require.NoError(t, err)
		_, ok := decoder.GetCallTarget(inst, 0x1000)
		assert.False(t, ok)
	})

	t.Run("overflow instAddr returns false", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x02, 0x00, 0x00, 0x94}, 0)
		require.NoError(t, err)
		_, ok := decoder.GetCallTarget(inst, 0x8000000000000001)
		assert.False(t, ok)
	})
}

func TestARM64Decoder_IsFirstArgImm(t *testing.T) {
	decoder := NewARM64Decoder()

	// In arm64 Go ABI, X0 is the first argument register.
	t.Run("mov x0, #41 (first arg register)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x20, 0x05, 0x80, 0xD2}, 0)
		require.NoError(t, err)
		ok, imm := decoder.IsFirstArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(41), imm)
	})

	t.Run("mov w0, #198 (first arg register, 32-bit)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xC0, 0x18, 0x80, 0x52}, 0)
		require.NoError(t, err)
		ok, imm := decoder.IsFirstArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(198), imm)
	})

	t.Run("mov w8, #198 (syscall reg, not first arg)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xC8, 0x18, 0x80, 0x52}, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsFirstArgImm(inst)
		assert.False(t, ok)
	})

	t.Run("nop returns false", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x1F, 0x20, 0x03, 0xD5}, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsFirstArgImm(inst)
		assert.False(t, ok)
	})

	// orr x0, xzr, #0x38 — bitmask-immediate encoding of x0 := 0x38 (openat syscall number).
	// arm64asm uses RegSP for the destination operand of ORR-immediate instructions.
	// Encoding: e0 0b 7d b2
	t.Run("orr x0, xzr, #0x38 (bitmask imm, openat syscall number)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xe0, 0x0b, 0x7d, 0xb2}, 0)
		require.NoError(t, err)
		ok, imm := decoder.IsFirstArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(0x38), imm)
	})

	t.Run("orr x8, xzr, #0x38 (bitmask imm, wrong register)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xe8, 0x0b, 0x7d, 0xb2}, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsFirstArgImm(inst)
		assert.False(t, ok)
	})
}

func TestARM64Decoder_ModifiesThirdArg(t *testing.T) {
	decoder := NewARM64Decoder()

	// arm64 third syscall argument register is X2 / W2.
	// Verified encodings:
	//   mov x2, #7   : E2 00 80 D2  (MOVZ X2, #7)
	//   mov w2, #3   : 62 00 80 52  (MOVZ W2, #3)
	//   mov x2, x1   : E2 03 01 AA  (ORR X2, XZR, X1 - MOV alias)
	//   mov x8, #7   : E8 00 80 D2  (MOVZ X8, #7)

	t.Run("mov x2, #7 returns true", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xE2, 0x00, 0x80, 0xD2}, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("mov w2, #3 returns true", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x62, 0x00, 0x80, 0x52}, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("mov x2, x1 returns true (register move modifies x2)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xE2, 0x03, 0x01, 0xAA}, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("mov x8, #7 returns false (wrong register)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xE8, 0x00, 0x80, 0xD2}, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("nop returns false", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x1F, 0x20, 0x03, 0xD5}, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	// Read-only first operand: must not be mistaken for writes to X2/W2
	t.Run("str x2, [x0] returns false (stores x2, does not modify it)", func(t *testing.T) {
		// str x2, [x0] — 02 00 00 F9
		inst, err := decoder.Decode([]byte{0x02, 0x00, 0x00, 0xF9}, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("cmp x2, #5 returns false (sets flags only)", func(t *testing.T) {
		// cmp x2, #5 (SUBS XZR, X2, #5) — 5F 14 00 F1
		inst, err := decoder.Decode([]byte{0x5F, 0x14, 0x00, 0xF1}, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("tst x2, x1 returns false (sets flags only)", func(t *testing.T) {
		// tst x2, x1 (ANDS XZR, X2, X1) — 5F 00 01 EA
		inst, err := decoder.Decode([]byte{0x5F, 0x00, 0x01, 0xEA}, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})
}

func TestARM64Decoder_IsThirdArgImm(t *testing.T) {
	decoder := NewARM64Decoder()

	t.Run("mov x2, #7 returns (true, 7)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xE2, 0x00, 0x80, 0xD2}, 0)
		require.NoError(t, err)
		ok, val := decoder.IsThirdArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(7), val)
	})

	t.Run("mov w2, #3 returns (true, 3)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x62, 0x00, 0x80, 0x52}, 0)
		require.NoError(t, err)
		ok, val := decoder.IsThirdArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(3), val)
	})

	t.Run("mov x2, x1 returns false (register move)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xE2, 0x03, 0x01, 0xAA}, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsThirdArgImm(inst)
		assert.False(t, ok)
	})

	t.Run("mov x8, #7 returns false (wrong register)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xE8, 0x00, 0x80, 0xD2}, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsThirdArgImm(inst)
		assert.False(t, ok)
	})
}

func TestARM64Decoder_ModifiesFirstArg(t *testing.T) {
	decoder := NewARM64Decoder()

	t.Run("mov x0, x1 returns true", func(t *testing.T) {
		// mov x0, x1
		inst, err := decoder.Decode([]byte{0xE0, 0x03, 0x01, 0xAA}, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesFirstArg(inst))
	})

	t.Run("mov w0, #198 returns true", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xC0, 0x18, 0x80, 0x52}, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesFirstArg(inst))
	})

	t.Run("mov x8, #198 returns false", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xC8, 0x18, 0x80, 0xD2}, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesFirstArg(inst))
	})

	t.Run("str x0, [x1] returns false", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x20, 0x00, 0x00, 0xF9}, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesFirstArg(inst))
	})
}

func TestARM64Decoder_ResolveFirstArgGlobal(t *testing.T) {
	decoder := NewARM64Decoder()

	adrpCode := []byte{0x1b, 0x89, 0x19, 0x90} // adrp x27, .+0x33120000
	ldrCode := []byte{0x60, 0xd7, 0x42, 0xf9}  // ldr x0, [x27,#1448]
	callCode := []byte{0x00, 0x00, 0x00, 0x94} // bl .

	adrpInst, err := decoder.Decode(adrpCode, 0x401000)
	require.NoError(t, err)
	ldrInst, err := decoder.Decode(ldrCode, 0x401004)
	require.NoError(t, err)
	callInst, err := decoder.Decode(callCode, 0x401008)
	require.NoError(t, err)

	a := adrpInst.arch.(arm64asm.Inst)
	rel, ok := a.Args[1].(arm64asm.PCRel)
	require.True(t, ok)

	pageBase := adrpInst.Offset &^ uint64(0xfff)
	loadAddr := uint64(int64(pageBase)+int64(rel)) + 1448

	blob := make([]byte, 16)
	binary.LittleEndian.PutUint64(blob[:8], 25)
	decoder.SetDataSections([]arm64DataSection{{Addr: loadAddr, Data: blob}})

	insts := []DecodedInstruction{adrpInst, ldrInst, callInst}
	ok, value := decoder.ResolveFirstArgGlobal(insts, 1)
	assert.True(t, ok)
	assert.Equal(t, int64(25), value)

	ok, value = decoder.ResolveFirstArgGlobal(insts, 0)
	assert.False(t, ok)
	assert.Equal(t, int64(0), value)

	t.Run("control flow boundary before adrp returns false", func(t *testing.T) {
		branchInst, err := decoder.Decode([]byte{0x02, 0x00, 0x00, 0x14}, 0x401004)
		require.NoError(t, err)

		instsWithBranch := []DecodedInstruction{adrpInst, branchInst, ldrInst, callInst}
		ok, value := decoder.ResolveFirstArgGlobal(instsWithBranch, 2)
		assert.False(t, ok)
		assert.Equal(t, int64(0), value)
	})
}
