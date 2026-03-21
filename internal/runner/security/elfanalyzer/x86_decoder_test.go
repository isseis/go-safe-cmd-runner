//go:build test

package elfanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestX86Decoder_Decode(t *testing.T) {
	decoder := NewX86Decoder()

	tests := []struct {
		name         string
		code         []byte
		wantLen      int
		wantSyscall  bool
		wantCtrlFlow bool
	}{
		{
			name:    "nop",
			code:    []byte{0x90},
			wantLen: 1,
		},
		{
			name:        "syscall",
			code:        []byte{0x0f, 0x05},
			wantLen:     2,
			wantSyscall: true,
		},
		{
			name:    "mov eax immediate",
			code:    []byte{0xb8, 0x29, 0x00, 0x00, 0x00},
			wantLen: 5,
		},
		{
			name:         "ret",
			code:         []byte{0xc3},
			wantLen:      1,
			wantCtrlFlow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0x1000)
			require.NoError(t, err)

			assert.Equal(t, tt.wantLen, inst.Len)
			assert.Equal(t, uint64(0x1000), inst.Offset)
			assert.Equal(t, tt.code[:tt.wantLen], inst.Raw)
			assert.Equal(t, tt.wantSyscall, decoder.IsSyscallInstruction(inst))
			assert.Equal(t, tt.wantCtrlFlow, decoder.IsControlFlowInstruction(inst))
		})
	}
}

func TestX86Decoder_Decode_Error(t *testing.T) {
	decoder := NewX86Decoder()

	// Empty code should return an error
	_, err := decoder.Decode([]byte{}, 0)
	assert.Error(t, err)
}

func TestX86Decoder_IsSyscallInstruction(t *testing.T) {
	decoder := NewX86Decoder()

	tests := []struct {
		name string
		code []byte
		want bool
	}{
		{
			name: "syscall instruction",
			code: []byte{0x0f, 0x05},
			want: true,
		},
		{
			name: "nop instruction",
			code: []byte{0x90},
			want: false,
		},
		{
			name: "mov instruction",
			code: []byte{0xb8, 0x00, 0x00, 0x00, 0x00},
			want: false,
		},
		{
			name: "ret instruction",
			code: []byte{0xc3},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoder.IsSyscallInstruction(inst))
		})
	}
}

func TestX86Decoder_ModifiesSyscallNumberRegister(t *testing.T) {
	decoder := NewX86Decoder()

	tests := []struct {
		name string
		code []byte
		want bool
	}{
		{
			name: "mov immediate to eax",
			code: []byte{0xb8, 0x29, 0x00, 0x00, 0x00},
			want: true,
		},
		{
			name: "mov ebx to eax",
			code: []byte{0x89, 0xd8},
			want: true,
		},
		{
			name: "mov memory to eax",
			code: []byte{0x8b, 0x04, 0x24},
			want: true,
		},
		{
			name: "mov immediate to ecx",
			code: []byte{0xb9, 0x29, 0x00, 0x00, 0x00},
			want: false,
		},
		{
			name: "mov rsi to rdi",
			// mov %rsi, %rdi
			code: []byte{0x48, 0x89, 0xf7},
			want: false,
		},
		{
			name: "nop has no args",
			code: []byte{0x90},
			want: false,
		},
		{
			// push %rax — reads rax, must not be mistaken for a write
			name: "push rax returns false",
			code: []byte{0x50},
			want: false,
		},
		{
			// cmp $0x5, %eax — compares, sets flags only
			name: "cmp eax imm returns false",
			code: []byte{0x83, 0xf8, 0x05},
			want: false,
		},
		{
			// test %eax, %eax — AND for flags only
			name: "test eax eax returns false",
			code: []byte{0x85, 0xc0},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoder.ModifiesSyscallNumberRegister(inst))
		})
	}
}

func TestX86Decoder_IsImmediateToSyscallNumberRegister(t *testing.T) {
	decoder := NewX86Decoder()

	tests := []struct {
		name    string
		code    []byte
		wantImm bool
		wantVal int64
	}{
		{
			name:    "mov $0x29, %eax",
			code:    []byte{0xb8, 0x29, 0x00, 0x00, 0x00},
			wantImm: true,
			wantVal: 41, // socket syscall
		},
		{
			name:    "mov $0x2a, %eax",
			code:    []byte{0xb8, 0x2a, 0x00, 0x00, 0x00},
			wantImm: true,
			wantVal: 42, // connect syscall
		},
		{
			name:    "mov %ebx, %eax (register move)",
			code:    []byte{0x89, 0xd8},
			wantImm: false,
			wantVal: 0,
		},
		{
			name:    "mov (%rsp), %eax (memory load)",
			code:    []byte{0x8b, 0x04, 0x24},
			wantImm: false,
			wantVal: 0,
		},
		{
			name:    "xor %eax, %eax (self-XOR zeroing idiom)",
			code:    []byte{0x31, 0xc0},
			wantImm: true,
			wantVal: 0, // read syscall number
		},
		{
			name:    "xor %ebx, %eax (different register, not zeroing idiom)",
			code:    []byte{0x31, 0xd8},
			wantImm: false,
			wantVal: 0,
		},
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

func TestX86Decoder_InstructionAlignment(t *testing.T) {
	decoder := NewX86Decoder()
	assert.Equal(t, 1, decoder.InstructionAlignment())
}

func TestX86Decoder_IsControlFlowInstruction(t *testing.T) {
	decoder := NewX86Decoder()

	tests := []struct {
		name string
		code []byte
		want bool
	}{
		{"jmp", []byte{0xeb, 0x00}, true},
		{"call", []byte{0xe8, 0x00, 0x00, 0x00, 0x00}, true},
		{"ret", []byte{0xc3}, true},
		{"mov", []byte{0xb8, 0x00, 0x00, 0x00, 0x00}, false},
		{"nop", []byte{0x90}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoder.IsControlFlowInstruction(inst))
		})
	}
}

func TestX86Decoder_GetCallTarget(t *testing.T) {
	decoder := NewX86Decoder()

	t.Run("valid forward call", func(t *testing.T) {
		// Layout: instAddr=0x401000, Len=5, rel=0xffb => nextPC=0x401005, target=0x402000
		code := []byte{0xe8, 0xfb, 0x0f, 0x00, 0x00} // rel = 0xffb; target = 0x401000+5+0xffb = 0x402000
		inst, err := decoder.Decode(code, 0x401000)
		require.NoError(t, err)
		target, ok := decoder.GetCallTarget(inst, 0x401000)
		assert.True(t, ok)
		assert.Equal(t, uint64(0x402000), target)
	})

	t.Run("non-call instruction returns false", func(t *testing.T) {
		code := []byte{0x90} // nop
		inst, err := decoder.Decode(code, 0x401000)
		require.NoError(t, err)
		_, ok := decoder.GetCallTarget(inst, 0x401000)
		assert.False(t, ok)
	})

	t.Run("overflow address returns false", func(t *testing.T) {
		// Valid CALL bytes, but the instruction address overflows int64
		code := []byte{0xe8, 0x01, 0x00, 0x00, 0x00} // call +1
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		// instAddr > math.MaxInt64 should return false
		_, ok := decoder.GetCallTarget(inst, 0x8000000000000001)
		assert.False(t, ok)
	})

	t.Run("negative displacement returns false", func(t *testing.T) {
		// e8 f6 ff ff ff = call rel32(-10); target = 0 + 5 + (-10) = -5 < 0
		code := []byte{0xe8, 0xf6, 0xff, 0xff, 0xff}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		_, ok := decoder.GetCallTarget(inst, 0)
		assert.False(t, ok)
	})
}

func TestX86Decoder_IsImmediateToFirstArgRegister(t *testing.T) {
	decoder := NewX86Decoder()

	// In Go's register-based ABI for amd64, the first argument register is AX/EAX.
	// This is the same as the syscall number register (RAX/EAX), because Go's
	// syscall wrappers pass the syscall number as the first argument.
	t.Run("mov imm to EAX (first arg register in Go ABI)", func(t *testing.T) {
		// b8 29 00 00 00 = mov $0x29, %eax
		code := []byte{0xb8, 0x29, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		imm, ok := decoder.IsImmediateToFirstArgRegister(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(0x29), imm)
	})

	t.Run("mov imm to RAX (first arg register in Go ABI, 64-bit form)", func(t *testing.T) {
		// 48 c7 c0 29 00 00 00 = mov $0x29, %rax
		code := []byte{0x48, 0xc7, 0xc0, 0x29, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		imm, ok := decoder.IsImmediateToFirstArgRegister(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(0x29), imm)
	})

	t.Run("mov imm to RDI (C ABI first arg, not Go ABI)", func(t *testing.T) {
		// 48 c7 c7 29 00 00 00 = mov $0x29, %rdi
		code := []byte{0x48, 0xc7, 0xc7, 0x29, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		_, ok := decoder.IsImmediateToFirstArgRegister(inst)
		assert.False(t, ok)
	})

	t.Run("non-mov instruction returns false", func(t *testing.T) {
		code := []byte{0x90} // nop
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		_, ok := decoder.IsImmediateToFirstArgRegister(inst)
		assert.False(t, ok)
	})
}

func TestX86Decoder_ModifiesThirdArgRegister(t *testing.T) {
	decoder := NewX86Decoder()

	t.Run("mov imm to EDX returns true", func(t *testing.T) {
		// ba 07 00 00 00 = mov $0x7, %edx
		code := []byte{0xba, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArgRegister(inst))
	})

	t.Run("mov imm to RDX returns true", func(t *testing.T) {
		// 48 c7 c2 07 00 00 00 = mov $0x7, %rdx
		code := []byte{0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArgRegister(inst))
	})

	t.Run("mov reg to RDX returns true", func(t *testing.T) {
		// 48 89 f2 = mov %rsi, %rdx
		code := []byte{0x48, 0x89, 0xf2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArgRegister(inst))
	})

	t.Run("xor EDX EDX returns true", func(t *testing.T) {
		// 31 d2 = xor %edx, %edx
		code := []byte{0x31, 0xd2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArgRegister(inst))
	})

	t.Run("mov imm to EAX returns false (wrong register)", func(t *testing.T) {
		// b8 07 00 00 00 = mov $0x7, %eax
		code := []byte{0xb8, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArgRegister(inst))
	})

	t.Run("nop returns false", func(t *testing.T) {
		code := []byte{0x90}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArgRegister(inst))
	})

	t.Run("push rdx returns false (reads rdx, not a write)", func(t *testing.T) {
		// 52 = push %rdx
		code := []byte{0x52}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArgRegister(inst))
	})

	t.Run("cmp edx imm returns false (sets flags only)", func(t *testing.T) {
		// 83 fa 05 = cmp $0x5, %edx
		code := []byte{0x83, 0xfa, 0x05}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArgRegister(inst))
	})

	t.Run("test edx edx returns false (sets flags only)", func(t *testing.T) {
		// 85 d2 = test %edx, %edx
		code := []byte{0x85, 0xd2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArgRegister(inst))
	})
}

func TestX86Decoder_IsImmediateToThirdArgRegister(t *testing.T) {
	decoder := NewX86Decoder()

	t.Run("mov imm to RDX (64bit) returns value", func(t *testing.T) {
		// 48 c7 c2 07 00 00 00 = mov $0x7, %rdx
		code := []byte{0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, val := decoder.IsImmediateToThirdArgRegister(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(7), val)
	})

	t.Run("mov imm to EDX (32bit) returns value", func(t *testing.T) {
		// ba 04 00 00 00 = mov $0x4, %edx
		code := []byte{0xba, 0x04, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, val := decoder.IsImmediateToThirdArgRegister(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(4), val)
	})

	t.Run("mov imm to RDX PROT_EXEC flag only", func(t *testing.T) {
		// ba 03 00 00 00 = mov $0x3, %edx
		code := []byte{0xba, 0x03, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, val := decoder.IsImmediateToThirdArgRegister(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(3), val)
	})

	t.Run("mov reg to RDX returns false (register move)", func(t *testing.T) {
		// 48 89 f2 = mov %rsi, %rdx
		code := []byte{0x48, 0x89, 0xf2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsImmediateToThirdArgRegister(inst)
		assert.False(t, ok)
	})

	t.Run("xor EDX EDX zeroing idiom returns zero", func(t *testing.T) {
		// 31 d2 = xor %edx, %edx
		code := []byte{0x31, 0xd2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, val := decoder.IsImmediateToThirdArgRegister(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(0), val)
	})

	t.Run("mov imm to EAX returns false (wrong register)", func(t *testing.T) {
		// b8 07 00 00 00 = mov $0x7, %eax
		code := []byte{0xb8, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsImmediateToThirdArgRegister(inst)
		assert.False(t, ok)
	})
}
