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
		name    string
		code    []byte
		wantOp  x86asm.Op
		wantLen int
	}{
		{
			name:    "nop",
			code:    []byte{0x90},
			wantOp:  x86asm.NOP,
			wantLen: 1,
		},
		{
			name:    "syscall",
			code:    []byte{0x0f, 0x05},
			wantOp:  x86asm.SYSCALL,
			wantLen: 2,
		},
		{
			name:    "mov eax immediate",
			code:    []byte{0xb8, 0x29, 0x00, 0x00, 0x00},
			wantOp:  x86asm.MOV,
			wantLen: 5,
		},
		{
			name:    "ret",
			code:    []byte{0xc3},
			wantOp:  x86asm.RET,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0x1000)
			require.NoError(t, err)

			assert.Equal(t, tt.wantOp, inst.Op)
			assert.Equal(t, tt.wantLen, inst.Len)
			assert.Equal(t, uint64(0x1000), inst.Offset)
			assert.Equal(t, tt.code[:tt.wantLen], inst.Raw)
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

func TestX86Decoder_ModifiesEAXorRAX(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoder.ModifiesEAXorRAX(inst))
		})
	}
}

func TestX86Decoder_IsImmediateMove(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := decoder.Decode(tt.code, 0)
			require.NoError(t, err)

			gotImm, gotVal := decoder.IsImmediateMove(inst)
			assert.Equal(t, tt.wantImm, gotImm)
			if tt.wantImm {
				assert.Equal(t, tt.wantVal, gotVal)
			}
		})
	}
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
