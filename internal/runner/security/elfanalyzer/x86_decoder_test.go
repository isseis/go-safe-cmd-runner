//go:build test

package elfanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/arch/x86/x86asm"
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

func TestX86Decoder_WritesSyscallReg(t *testing.T) {
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
		{
			// mul %ecx — implicit rDX:rAX = rAX * ECX; writes EAX implicitly
			name: "mul ecx returns true (implicit RAX write)",
			code: []byte{0xf7, 0xe1},
			want: true,
		},
		{
			// imul %ecx (one-operand) — implicit rDX:rAX = rAX * ECX
			name: "imul ecx one-operand returns true (implicit RAX write)",
			code: []byte{0xf7, 0xe9},
			want: true,
		},
		{
			// imul %eax, %ecx (two-operand, dest=EAX) — explicit first operand caught by normal path
			name: "imul eax ecx two-operand returns true (explicit EAX dest)",
			code: []byte{0x0f, 0xaf, 0xc1},
			want: true,
		},
		{
			// imul %ecx, %ebx (two-operand, dest=ECX, not RAX) — must not fire
			name: "imul ecx ebx two-operand returns false (dest not RAX)",
			code: []byte{0x0f, 0xaf, 0xcb},
			want: false,
		},
		{
			// div %ecx — quotient → EAX, remainder → EDX
			name: "div ecx returns true (implicit RAX write)",
			code: []byte{0xf7, 0xf1},
			want: true,
		},
		{
			// idiv %ecx — signed divide, quotient → EAX
			name: "idiv ecx returns true (implicit RAX write)",
			code: []byte{0xf7, 0xf9},
			want: true,
		},
		{
			// cpuid — writes EAX (and EBX/ECX/EDX)
			name: "cpuid returns true (implicit EAX write)",
			code: []byte{0x0f, 0xa2},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoder.WritesSyscallReg(inst))
		})
	}
}

func TestX86Decoder_IsSyscallNumImm(t *testing.T) {
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

			gotImm, gotVal := decoder.IsSyscallNumImm(inst)
			assert.Equal(t, tt.wantImm, gotImm)
			if tt.wantImm {
				assert.Equal(t, tt.wantVal, gotVal)
			}
		})
	}
}

func TestRegFamily_AdditionalVariants(t *testing.T) {
	tests := []struct {
		name string
		reg  x86asm.Reg
		want x86RegFamily
	}{
		{name: "AH maps to AX family", reg: x86asm.AH, want: x86RegFamilyAX},
		{name: "CH maps to CX family", reg: x86asm.CH, want: x86RegFamilyCX},
		{name: "DH maps to DX family", reg: x86asm.DH, want: x86RegFamilyDX},
		{name: "BH maps to BX family", reg: x86asm.BH, want: x86RegFamilyBX},
		{name: "SPB maps to SP family", reg: x86asm.SPB, want: x86RegFamilySP},
		{name: "BPB maps to BP family", reg: x86asm.BPB, want: x86RegFamilyBP},
		{name: "SIB maps to SI family", reg: x86asm.SIB, want: x86RegFamilySI},
		{name: "DIB maps to DI family", reg: x86asm.DIB, want: x86RegFamilyDI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, regFamily(tt.reg))
		})
	}
}

func TestX86Decoder_IsSyscallNumImm_PartialRegisterWritesIgnored(t *testing.T) {
	decoder := NewX86Decoder()

	tests := []struct {
		name string
		code []byte
	}{
		{
			name: "mov imm8 to AL",
			code: []byte{0xb0, 0x29},
		},
		{
			name: "mov imm16 to AX",
			code: []byte{0x66, 0xb8, 0x29, 0x00},
		},
		{
			name: "xor AX AX",
			code: []byte{0x66, 0x31, 0xc0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)

			ok, _ := decoder.IsSyscallNumImm(inst)
			assert.False(t, ok)
		})
	}
}

func TestX86Decoder_CopySourceForRegFamily_PartialRegisterWritesIgnored(t *testing.T) {
	decoder := NewX86Decoder()

	t.Run("mov AL DL is ignored for RAX family", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x88, 0xd0}, 0)
		require.NoError(t, err)

		_, ok := decoder.CopySourceForRegFamily(inst, x86asm.RAX)
		assert.False(t, ok)
	})

	t.Run("mov EAX EDX is accepted for RAX family", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0x89, 0xd0}, 0)
		require.NoError(t, err)

		src, ok := decoder.CopySourceForRegFamily(inst, x86asm.RAX)
		assert.True(t, ok)
		assert.Equal(t, x86asm.EDX, src)
	})
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

func TestX86Decoder_IsFirstArgImm(t *testing.T) {
	decoder := NewX86Decoder()

	// In Go's register-based ABI for amd64, the first argument register is AX/EAX.
	// This is the same as the syscall number register (RAX/EAX), because Go's
	// syscall wrappers pass the syscall number as the first argument.
	t.Run("mov imm to EAX (first arg register in Go ABI)", func(t *testing.T) {
		// b8 29 00 00 00 = mov $0x29, %eax
		code := []byte{0xb8, 0x29, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, imm := decoder.IsFirstArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(0x29), imm)
	})

	t.Run("mov imm to RAX (first arg register in Go ABI, 64-bit form)", func(t *testing.T) {
		// 48 c7 c0 29 00 00 00 = mov $0x29, %rax
		code := []byte{0x48, 0xc7, 0xc0, 0x29, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, imm := decoder.IsFirstArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(0x29), imm)
	})

	t.Run("mov imm to RDI (C ABI first arg, not Go ABI)", func(t *testing.T) {
		// 48 c7 c7 29 00 00 00 = mov $0x29, %rdi
		code := []byte{0x48, 0xc7, 0xc7, 0x29, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsFirstArgImm(inst)
		assert.False(t, ok)
	})

	t.Run("non-mov instruction returns false", func(t *testing.T) {
		code := []byte{0x90} // nop
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsFirstArgImm(inst)
		assert.False(t, ok)
	})
}

func TestX86Decoder_ModifiesThirdArg(t *testing.T) {
	decoder := NewX86Decoder()

	t.Run("mov imm to EDX returns true", func(t *testing.T) {
		// ba 07 00 00 00 = mov $0x7, %edx
		code := []byte{0xba, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("mov imm to RDX returns true", func(t *testing.T) {
		// 48 c7 c2 07 00 00 00 = mov $0x7, %rdx
		code := []byte{0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("mov reg to RDX returns true", func(t *testing.T) {
		// 48 89 f2 = mov %rsi, %rdx
		code := []byte{0x48, 0x89, 0xf2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("xor EDX EDX returns true", func(t *testing.T) {
		// 31 d2 = xor %edx, %edx
		code := []byte{0x31, 0xd2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("mov imm to EAX returns false (wrong register)", func(t *testing.T) {
		// b8 07 00 00 00 = mov $0x7, %eax
		code := []byte{0xb8, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("nop returns false", func(t *testing.T) {
		code := []byte{0x90}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("push rdx returns false (reads rdx, not a write)", func(t *testing.T) {
		// 52 = push %rdx
		code := []byte{0x52}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("cmp edx imm returns false (sets flags only)", func(t *testing.T) {
		// 83 fa 05 = cmp $0x5, %edx
		code := []byte{0x83, 0xfa, 0x05}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("test edx edx returns false (sets flags only)", func(t *testing.T) {
		// 85 d2 = test %edx, %edx
		code := []byte{0x85, 0xd2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("mul ecx returns true (implicit RDX write)", func(t *testing.T) {
		// f7 e1 = mul %ecx — high half of result → EDX
		code := []byte{0xf7, 0xe1}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("imul ecx one-operand returns true (implicit RDX write)", func(t *testing.T) {
		// f7 e9 = imul %ecx (one-operand) — high half → EDX
		code := []byte{0xf7, 0xe9}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("imul eax ecx two-operand returns false (result only in EAX)", func(t *testing.T) {
		// 0f af c1 = imul %eax, %ecx (two-operand, dest=EAX, not RDX)
		code := []byte{0x0f, 0xaf, 0xc1}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("div ecx returns true (remainder → EDX)", func(t *testing.T) {
		// f7 f1 = div %ecx
		code := []byte{0xf7, 0xf1}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("idiv ecx returns true (remainder → EDX)", func(t *testing.T) {
		// f7 f9 = idiv %ecx
		code := []byte{0xf7, 0xf9}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("cqo returns true (sign-extends RAX into RDX)", func(t *testing.T) {
		// 48 99 = cqo
		code := []byte{0x48, 0x99}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("cdq returns true (sign-extends EAX into EDX)", func(t *testing.T) {
		// 99 = cdq
		code := []byte{0x99}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("cwd returns true (sign-extends AX into DX)", func(t *testing.T) {
		// 66 99 = cwd
		code := []byte{0x66, 0x99}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})

	t.Run("mov imm to DH returns true (overlaps EDX/RDX)", func(t *testing.T) {
		// b6 01 = mov $0x1, %dh
		code := []byte{0xb6, 0x01}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesThirdArg(inst))
	})
}

func TestX86Decoder_IsThirdArgImm(t *testing.T) {
	decoder := NewX86Decoder()

	t.Run("mov imm to RDX (64bit) returns value", func(t *testing.T) {
		// 48 c7 c2 07 00 00 00 = mov $0x7, %rdx
		code := []byte{0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, val := decoder.IsThirdArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(7), val)
	})

	t.Run("mov imm to EDX (32bit) returns value", func(t *testing.T) {
		// ba 04 00 00 00 = mov $0x4, %edx
		code := []byte{0xba, 0x04, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, val := decoder.IsThirdArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(4), val)
	})

	t.Run("mov imm to RDX PROT_READ|PROT_WRITE (no PROT_EXEC)", func(t *testing.T) {
		// ba 03 00 00 00 = mov $0x3, %edx  (PROT_READ|PROT_WRITE = 0x3)
		code := []byte{0xba, 0x03, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, val := decoder.IsThirdArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(3), val)
	})

	t.Run("mov reg to RDX returns false (register move)", func(t *testing.T) {
		// 48 89 f2 = mov %rsi, %rdx
		code := []byte{0x48, 0x89, 0xf2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsThirdArgImm(inst)
		assert.False(t, ok)
	})

	t.Run("xor EDX EDX zeroing idiom returns zero", func(t *testing.T) {
		// 31 d2 = xor %edx, %edx
		code := []byte{0x31, 0xd2}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, val := decoder.IsThirdArgImm(inst)
		assert.True(t, ok)
		assert.Equal(t, int64(0), val)
	})

	t.Run("mov imm to EAX returns false (wrong register)", func(t *testing.T) {
		// b8 07 00 00 00 = mov $0x7, %eax
		code := []byte{0xb8, 0x07, 0x00, 0x00, 0x00}
		inst, err := decoder.Decode(code, 0)
		require.NoError(t, err)
		ok, _ := decoder.IsThirdArgImm(inst)
		assert.False(t, ok)
	})
}

func TestX86Decoder_ModifiesFirstArg(t *testing.T) {
	decoder := NewX86Decoder()

	t.Run("mov imm to EAX returns true", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xb8, 0x2a, 0x00, 0x00, 0x00}, 0)
		require.NoError(t, err)
		assert.True(t, decoder.ModifiesFirstArg(inst))
	})

	t.Run("mov imm to EDX returns false", func(t *testing.T) {
		inst, err := decoder.Decode([]byte{0xba, 0x07, 0x00, 0x00, 0x00}, 0)
		require.NoError(t, err)
		assert.False(t, decoder.ModifiesFirstArg(inst))
	})
}

// decodeOne decodes a single instruction from code at instAddr and returns it.
func decodeOne(t *testing.T, code []byte, instAddr uint64) DecodedInstruction {
	t.Helper()
	d := NewX86Decoder()
	inst, err := d.Decode(code, instAddr)
	require.NoError(t, err)
	return inst
}

func TestX86Decoder_ResolveFirstArgGlobal(t *testing.T) {
	const instAddr = uint64(0x1000)

	// MOV RAX, [RIP+disp32]: 48 8B 05 <disp32-le>  (7 bytes, REX.W)
	// MOV EAX, [RIP+disp32]: 8B 05 <disp32-le>     (6 bytes, no REX.W)
	// MOV RBX, [RIP+disp32]: 48 8B 1D <disp32-le>  (7 bytes, reg=011=RBX)
	// MOV RAX, [RBX]:        48 8B 03               (3 bytes, register indirect)
	// MOV RAX, imm64:        48 B8 <imm64-le>       (10 bytes)

	makeRAXRIPLoad := func(disp int32) []byte {
		b := []byte{0x48, 0x8B, 0x05, 0, 0, 0, 0}
		b[3] = byte(disp)
		b[4] = byte(disp >> 8)
		b[5] = byte(disp >> 16)
		b[6] = byte(disp >> 24)
		return b
	}
	makeEAXRIPLoad := func(disp int32) []byte {
		b := []byte{0x8B, 0x05, 0, 0, 0, 0}
		b[2] = byte(disp)
		b[3] = byte(disp >> 8)
		b[4] = byte(disp >> 16)
		b[5] = byte(disp >> 24)
		return b
	}

	// Helper: build a data section holding val64 at addr.
	makeSec := func(addr uint64, val uint64) x86DataSection {
		data := make([]byte, 8)
		data[0] = byte(val)
		data[1] = byte(val >> 8)
		data[2] = byte(val >> 16)
		data[3] = byte(val >> 24)
		data[4] = byte(val >> 32)
		data[5] = byte(val >> 40)
		data[6] = byte(val >> 48)
		data[7] = byte(val >> 56)
		return x86DataSection{Addr: addr, Data: data}
	}

	t.Run("no data sections", func(t *testing.T) {
		decoder := NewX86Decoder()
		inst := decodeOne(t, makeRAXRIPLoad(4), instAddr)
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 0)
		assert.False(t, ok)
		assert.Equal(t, int64(0), val)
	})

	t.Run("nil instructions", func(t *testing.T) {
		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{makeSec(0x2000, 72)})
		ok, val := decoder.ResolveFirstArgGlobal(nil, 0)
		assert.False(t, ok)
		assert.Equal(t, int64(0), val)
	})

	t.Run("out-of-range index", func(t *testing.T) {
		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{makeSec(0x2000, 72)})
		inst := decodeOne(t, makeRAXRIPLoad(4), instAddr)
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 1)
		assert.False(t, ok)
		assert.Equal(t, int64(0), val)
	})

	t.Run("MOV RAX [RIP+disp] resolved", func(t *testing.T) {
		// MOV RAX, [RIP+4] at 0x1000 → nextPC=0x1007, target=0x100B
		code := makeRAXRIPLoad(4)
		inst := decodeOne(t, code, instAddr)
		targetAddr := instAddr + uint64(inst.Len) + 4 // 0x100B

		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{makeSec(targetAddr, 72)})
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 0)
		require.True(t, ok)
		assert.Equal(t, int64(72), val)
	})

	t.Run("MOV EAX [RIP+disp] resolved", func(t *testing.T) {
		// MOV EAX, [RIP+8] at 0x1000 → nextPC=0x1006, target=0x100E
		code := makeEAXRIPLoad(8)
		inst := decodeOne(t, code, instAddr)
		targetAddr := instAddr + uint64(inst.Len) + 8 // 0x100E

		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{makeSec(targetAddr, 59)}) // SYS_execve
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 0)
		require.True(t, ok)
		assert.Equal(t, int64(59), val)
	})

	t.Run("MOV EAX [RIP+disp] reads 32-bit value only", func(t *testing.T) {
		code := makeEAXRIPLoad(8)
		inst := decodeOne(t, code, instAddr)
		targetAddr := instAddr + uint64(inst.Len) + 8

		data := []byte{59, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{{Addr: targetAddr, Data: data}})
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 0)
		require.True(t, ok)
		assert.Equal(t, int64(59), val)
	})

	t.Run("negative displacement", func(t *testing.T) {
		// MOV RAX, [RIP-8] at 0x1010 → nextPC=0x1017, target=0x100F
		const addr = uint64(0x1010)
		code := makeRAXRIPLoad(-8)
		inst := decodeOne(t, code, addr)
		targetAddr := addr + uint64(inst.Len) - 8 // 0x100F

		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{makeSec(targetAddr, 202)}) // SYS_futex
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 0)
		require.True(t, ok)
		assert.Equal(t, int64(202), val)
	})

	t.Run("address not in data section", func(t *testing.T) {
		code := makeRAXRIPLoad(4)
		inst := decodeOne(t, code, instAddr)

		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{makeSec(0x9000, 72)}) // wrong address
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 0)
		assert.False(t, ok)
		assert.Equal(t, int64(0), val)
	})

	t.Run("non-RIP-relative load MOV RAX [RBX]", func(t *testing.T) {
		// MOV RAX, [RBX]: 48 8B 03
		inst := decodeOne(t, []byte{0x48, 0x8B, 0x03}, instAddr)
		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{makeSec(0x1000, 72)})
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 0)
		assert.False(t, ok)
		assert.Equal(t, int64(0), val)
	})

	t.Run("wrong destination register MOV RBX [RIP+disp]", func(t *testing.T) {
		// MOV RBX, [RIP+disp32]: 48 8B 1D <disp32-le>
		b := []byte{0x48, 0x8B, 0x1D, 0x04, 0x00, 0x00, 0x00}
		inst := decodeOne(t, b, instAddr)
		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{makeSec(instAddr+uint64(inst.Len)+4, 72)})
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 0)
		assert.False(t, ok)
		assert.Equal(t, int64(0), val)
	})

	t.Run("MOV RAX imm is not a global load", func(t *testing.T) {
		// MOV RAX, 72: 48 B8 48 00 00 00 00 00 00 00
		b := []byte{0x48, 0xB8, 72, 0, 0, 0, 0, 0, 0, 0}
		inst := decodeOne(t, b, instAddr)
		decoder := NewX86Decoder()
		decoder.SetDataSections([]x86DataSection{makeSec(0x1000, 72)})
		ok, val := decoder.ResolveFirstArgGlobal([]DecodedInstruction{inst}, 0)
		assert.False(t, ok)
		assert.Equal(t, int64(0), val)
	})
}
