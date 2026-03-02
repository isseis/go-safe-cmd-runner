//go:build test

package elfanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verified arm64 instruction byte sequences (little-endian, 4 bytes each):
//   svc  #0           : 01 00 00 D4
//   mov  w8, #198     : C8 18 80 52  (MOVZ W8, #198)
//   mov  x8, #198     : C8 18 80 D2  (MOVZ X8, #198)
//   mov  w8, #41      : 28 05 80 52  (MOVZ W8, #41 = socket on arm64)
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

func TestARM64Decoder_ModifiesSyscallNumberRegister(t *testing.T) {
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
		// Instructions that do NOT write to W8/X8
		{"svc #0", []byte{0x01, 0x00, 0x00, 0xD4}, false},
		{"mov x0, #41", []byte{0x20, 0x05, 0x80, 0xD2}, false},
		{"nop", []byte{0x1F, 0x20, 0x03, 0xD5}, false},
		{"ret", []byte{0xC0, 0x03, 0x5F, 0xD6}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoder.ModifiesSyscallNumberRegister(inst))
		})
	}
}

func TestARM64Decoder_IsImmediateToSyscallNumberRegister(t *testing.T) {
	decoder := NewARM64Decoder()

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
			gotImm, gotVal := decoder.IsImmediateToSyscallNumberRegister(inst)
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

func TestARM64Decoder_IsImmediateToFirstArgRegister(t *testing.T) {
	decoder := NewARM64Decoder()

	// In arm64 Go ABI, X0 is the first argument register.
	t.Run("mov x0, #41 (first arg register)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x20, 0x05, 0x80, 0xD2}, 0)
		require.NoError(t, err)
		imm, ok := decoder.IsImmediateToFirstArgRegister(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(41), imm)
	})

	t.Run("mov w0, #198 (first arg register, 32-bit)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xC0, 0x18, 0x80, 0x52}, 0)
		require.NoError(t, err)
		imm, ok := decoder.IsImmediateToFirstArgRegister(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(198), imm)
	})

	t.Run("mov w8, #198 (syscall reg, not first arg)", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xC8, 0x18, 0x80, 0x52}, 0)
		require.NoError(t, err)
		_, ok := decoder.IsImmediateToFirstArgRegister(inst)
		assert.False(t, ok)
	})

	t.Run("nop returns false", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x1F, 0x20, 0x03, 0xD5}, 0)
		require.NoError(t, err)
		_, ok := decoder.IsImmediateToFirstArgRegister(inst)
		assert.False(t, ok)
	})
}
