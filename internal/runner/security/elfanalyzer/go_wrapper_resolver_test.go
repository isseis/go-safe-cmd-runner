//go:build test

package elfanalyzer

import (
	"debug/elf"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/arch/x86/x86asm"
)

func TestGoWrapperResolver_NewGoWrapperResolver(t *testing.T) {
	resolver := NewGoWrapperResolver()

	assert.NotNil(t, resolver)
	assert.NotNil(t, resolver.symbols)
	assert.NotNil(t, resolver.wrapperAddrs)
	assert.NotNil(t, resolver.pclntabParser)
	assert.NotNil(t, resolver.decoder)
	assert.False(t, resolver.HasSymbols())
}

func TestGoWrapperResolver_HasSymbols(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Initially no symbols
	assert.False(t, resolver.HasSymbols())

	// Manually add a symbol to simulate loading
	resolver.symbols["main.main"] = SymbolInfo{
		Name:    "main.main",
		Address: 0x401000,
		Size:    100,
	}
	resolver.hasSymbols = true

	assert.True(t, resolver.HasSymbols())
}

func TestGoWrapperResolver_FindWrapperCalls_NoWrappers(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// With no wrappers loaded, should return nil
	result := resolver.FindWrapperCalls([]byte{0x90, 0x90}, 0x401000)
	assert.Nil(t, result)
}

func TestGoWrapperResolver_FindWrapperCalls_WithWrapper(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Manually register a wrapper at a known address
	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.Syscall"

	// Create a code segment with:
	// mov $0x29, %eax  ; socket syscall number (41)
	// call 0x402000    ; call to syscall.Syscall
	//
	// The CALL instruction is relative, so we need to calculate the offset
	// Code layout at 0x401000:
	// 0x401000: b8 29 00 00 00    mov $0x29, %eax (5 bytes)
	// 0x401005: e8 f6 0f 00 00    call 0x402000 (5 bytes, rel32 = 0x402000 - 0x40100a = 0xff6)
	//
	// Wait, the call target calculation: target = instruction_addr + instruction_len + rel32
	// So: 0x402000 = 0x401005 + 5 + rel32
	// rel32 = 0x402000 - 0x40100a = 0xff6

	baseAddr := uint64(0x401000)
	code := []byte{
		0xb8, 0x29, 0x00, 0x00, 0x00, // mov $0x29, %eax
		0xe8, 0xf6, 0x0f, 0x00, 0x00, // call rel32 (target = 0x402000)
	}

	result := resolver.FindWrapperCalls(code, baseAddr)

	require.Len(t, result, 1)
	assert.Equal(t, uint64(0x401005), result[0].CallSiteAddress)
	assert.Equal(t, "syscall.Syscall", result[0].TargetFunction)
	assert.Equal(t, 41, result[0].SyscallNumber) // socket syscall
	assert.True(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodGoWrapper, result[0].DeterminationMethod)
}

func TestGoWrapperResolver_FindWrapperCalls_UnresolvedSyscall(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Register a wrapper
	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.Syscall"

	// Create code with a call but no clear mov to eax/rax before it
	// Just a CALL instruction without any mov to rax
	baseAddr := uint64(0x401000)
	code := []byte{
		0x90, 0x90, 0x90, 0x90, 0x90, // nop x5
		0xe8, 0xf6, 0x0f, 0x00, 0x00, // call rel32 (target = 0x402000)
	}

	result := resolver.FindWrapperCalls(code, baseAddr)

	require.Len(t, result, 1)
	assert.Equal(t, uint64(0x401005), result[0].CallSiteAddress)
	assert.Equal(t, -1, result[0].SyscallNumber)
	assert.False(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodUnknownScanLimitExceeded, result[0].DeterminationMethod)
}

func TestGoWrapperResolver_ResolveSyscallArgument_ControlFlowBoundary(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Create recent instructions with a jump between mov and call
	// The control flow boundary (jmp) should stop backward scanning before reaching mov
	// Instructions order: mov, jmp, call
	// Backward scan from call: first sees jmp (control flow boundary), stops
	decoder := NewX86Decoder()

	movCode := []byte{0xb8, 0x29, 0x00, 0x00, 0x00}  // mov $0x29, %eax
	jmpCode := []byte{0xeb, 0x05}                    // jmp +5 (control flow boundary)
	callCode := []byte{0xe8, 0x00, 0x00, 0x00, 0x00} // call (placeholder)

	movInst, _ := decoder.Decode(movCode, 0x401000)
	jmpInst, _ := decoder.Decode(jmpCode, 0x401005)
	callInst, _ := decoder.Decode(callCode, 0x401007)

	// Instructions: mov, jmp, call
	// Scanning backward from call: jmp is hit first, which is a control flow boundary
	recentInstructions := []DecodedInstruction{movInst, jmpInst, callInst}

	syscallNum, method := resolver.resolveSyscallArgument(recentInstructions)
	assert.Equal(t, -1, syscallNum) // Should not find syscall number due to control flow boundary
	assert.Equal(t, DeterminationMethodUnknownControlFlowBoundary, method)
}

func TestGoWrapperResolver_ResolveSyscallArgument_RAX(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Test with mov to RAX (64-bit)
	// 48 c7 c0 29 00 00 00  mov $0x29, %rax
	movCode := []byte{0x48, 0xc7, 0xc0, 0x29, 0x00, 0x00, 0x00}
	callCode := []byte{0xe8, 0x00, 0x00, 0x00, 0x00}

	decoder := NewX86Decoder()
	movInst, _ := decoder.Decode(movCode, 0x401000)
	callInst, _ := decoder.Decode(callCode, 0x401007)

	recentInstructions := []DecodedInstruction{movInst, callInst}

	syscallNum, method := resolver.resolveSyscallArgument(recentInstructions)
	assert.Equal(t, 41, syscallNum) // socket syscall
	assert.Equal(t, DeterminationMethodGoWrapper, method)
}

func TestGoWrapperResolver_ResolveSyscallArgument_EAX(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Test with mov to EAX (32-bit, commonly used by Go compiler)
	// b8 29 00 00 00  mov $0x29, %eax
	movCode := []byte{0xb8, 0x29, 0x00, 0x00, 0x00}
	callCode := []byte{0xe8, 0x00, 0x00, 0x00, 0x00}

	decoder := NewX86Decoder()
	movInst, _ := decoder.Decode(movCode, 0x401000)
	callInst, _ := decoder.Decode(callCode, 0x401005)

	recentInstructions := []DecodedInstruction{movInst, callInst}

	syscallNum, method := resolver.resolveSyscallArgument(recentInstructions)
	assert.Equal(t, 41, syscallNum) // socket syscall
	assert.Equal(t, DeterminationMethodGoWrapper, method)
}

func TestGoWrapperResolver_ResolveSyscallArgument_OutOfRange(t *testing.T) {
	resolver := NewGoWrapperResolver()

	decoder := NewX86Decoder()

	tests := []struct {
		name           string
		code           []byte
		expected       int
		expectedMethod string
		reason         string
	}{
		{
			name:           "value too large",
			code:           []byte{0xb8, 0xe8, 0x03, 0x00, 0x00}, // mov $1000, %eax (exceeds maxValidSyscallNumber)
			expected:       -1,
			expectedMethod: DeterminationMethodUnknownIndirectSetting,
			reason:         "1000 exceeds max valid syscall number (500)",
		},
		{
			name:           "value at boundary (valid)",
			code:           []byte{0x48, 0xc7, 0xc0, 0xf4, 0x01, 0x00, 0x00}, // mov $500, %rax (at boundary)
			expected:       500,
			expectedMethod: DeterminationMethodGoWrapper,
			reason:         "500 is exactly at max valid syscall number",
		},
		{
			name:           "value zero (valid)",
			code:           []byte{0xb8, 0x00, 0x00, 0x00, 0x00}, // mov $0, %eax
			expected:       0,
			expectedMethod: DeterminationMethodGoWrapper,
			reason:         "0 is a valid syscall number (read)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCode := []byte{0xe8, 0x00, 0x00, 0x00, 0x00}

			movInst, err := decoder.Decode(tt.code, 0x401000)
			require.NoError(t, err)
			callInst, err := decoder.Decode(callCode, 0x401000+uint64(len(tt.code)))
			require.NoError(t, err)

			recentInstructions := []DecodedInstruction{movInst, callInst}

			syscallNum, method := resolver.resolveSyscallArgument(recentInstructions)
			assert.Equal(t, tt.expected, syscallNum, tt.reason)
			assert.Equal(t, tt.expectedMethod, method, tt.reason)
		})
	}
}

func TestGoWrapperResolver_ResolveWrapper_NotACall(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Create a non-CALL instruction
	decoder := NewX86Decoder()
	nopCode := []byte{0x90}
	nopInst, _ := decoder.Decode(nopCode, 0x401000)

	wrapper := resolver.resolveWrapper(nopInst)
	assert.Equal(t, NoWrapper, wrapper)
}

func TestGoWrapperResolver_ResolveWrapper_HighOffsetOverflow(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Construct a DecodedInstruction with Offset > math.MaxInt64.
	// resolveWrapper should bail out rather than silently overflow.
	inst := DecodedInstruction{
		Op:     x86asm.CALL,
		Offset: math.MaxUint64 - 10,
		Len:    5,
		Args:   []x86asm.Arg{x86asm.Rel(1)},
	}

	wrapper := resolver.resolveWrapper(inst)
	assert.Equal(t, NoWrapper, wrapper)
}

func TestGoWrapperResolver_ResolveWrapper_NegativeLen(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Negative instruction length should be rejected by the pre-check.
	inst := DecodedInstruction{
		Op:     x86asm.CALL,
		Offset: 0x1000,
		Len:    -5,
		Args:   []x86asm.Arg{x86asm.Rel(1)},
	}

	wrapper := resolver.resolveWrapper(inst)
	assert.Equal(t, NoWrapper, wrapper)
}

func TestGoWrapperResolver_ResolveWrapper_NegativeDisplacement(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Create a CALL whose rel32 points far backwards such that
	// int64(nextPC) + int64(target) < 0. resolveWrapper should reject it.
	// Choose nextPC = 0x10 (Offset 0x0b with Len 5) and target = -0x100
	inst := DecodedInstruction{
		Op:     x86asm.CALL,
		Offset: 0x0b, // nextPC = 0x0b + 5 = 0x10
		Len:    5,
		Args:   []x86asm.Arg{x86asm.Rel(-0x100)},
	}

	wrapper := resolver.resolveWrapper(inst)
	assert.Equal(t, NoWrapper, wrapper)
}

func TestGoWrapperResolver_ResolveWrapper_UnknownTarget(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Register a wrapper at a different address
	resolver.wrapperAddrs[0x403000] = "syscall.Syscall"

	// Create a CALL to a different address
	decoder := NewX86Decoder()
	callCode := []byte{0xe8, 0xfb, 0x0f, 0x00, 0x00} // call to 0x402000
	callInst, _ := decoder.Decode(callCode, 0x401000)

	wrapper := resolver.resolveWrapper(callInst)
	assert.Equal(t, NoWrapper, wrapper)
}

func TestGoWrapperResolver_GetWrapperAddresses(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Initially empty
	addrs := resolver.GetWrapperAddresses()
	assert.Empty(t, addrs)

	// Add a wrapper
	resolver.wrapperAddrs[0x401000] = "syscall.Syscall"

	addrs = resolver.GetWrapperAddresses()
	assert.Len(t, addrs, 1)
	assert.Equal(t, GoSyscallWrapper("syscall.Syscall"), addrs[0x401000])
}

func TestGoWrapperResolver_GetSymbols(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Initially empty
	symbols := resolver.GetSymbols()
	assert.Empty(t, symbols)

	// Add a symbol
	resolver.symbols["main.main"] = SymbolInfo{
		Name:    "main.main",
		Address: 0x401000,
		Size:    100,
	}

	symbols = resolver.GetSymbols()
	assert.Len(t, symbols, 1)
	assert.Equal(t, uint64(0x401000), symbols["main.main"].Address)
}

func TestGoWrapperResolver_KnownGoWrappers(t *testing.T) {
	// Verify the known wrapper set contains all expected entries
	expectedWrappers := map[GoSyscallWrapper]struct{}{
		"syscall.Syscall":     {},
		"syscall.Syscall6":    {},
		"syscall.RawSyscall":  {},
		"syscall.RawSyscall6": {},
		"runtime.syscall":     {},
		"runtime.syscall6":    {},
	}

	assert.Len(t, knownGoWrappers, len(expectedWrappers))

	for name := range expectedWrappers {
		_, ok := knownGoWrappers[name]
		assert.True(t, ok, "wrapper %q not found in knownGoWrappers", name)
	}
}

func TestGoWrapperResolver_FindWrapperCalls_MultipleCalls(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Register multiple wrappers
	resolver.wrapperAddrs[0x402000] = "syscall.Syscall"
	resolver.wrapperAddrs[0x403000] = "syscall.Syscall6"

	// Create code with multiple wrapper calls
	// First call: mov $0x29, %eax; call 0x402000
	// Second call: mov $0x2a, %eax; call 0x403000
	//
	// Layout at baseAddr 0x401000:
	// 0x401000: b8 29 00 00 00       mov $0x29, %eax (5 bytes)
	// 0x401005: e8 f6 0f 00 00       call rel32 (5 bytes) -> 0x40100a + 0xff6 = 0x402000
	// 0x40100a: b8 2a 00 00 00       mov $0x2a, %eax (5 bytes)
	// 0x40100f: e8 ec 1f 00 00       call rel32 (5 bytes) -> 0x401014 + 0x1fec = 0x403000
	baseAddr := uint64(0x401000)
	code := []byte{
		// First call at 0x401005
		0xb8, 0x29, 0x00, 0x00, 0x00, // mov $0x29, %eax
		0xe8, 0xf6, 0x0f, 0x00, 0x00, // call 0x402000 (rel = 0xff6)
		// Second call at 0x40100f
		0xb8, 0x2a, 0x00, 0x00, 0x00, // mov $0x2a, %eax
		0xe8, 0xec, 0x1f, 0x00, 0x00, // call 0x403000 (rel = 0x1fec)
	}

	result := resolver.FindWrapperCalls(code, baseAddr)

	require.Len(t, result, 2)

	// First call
	assert.Equal(t, uint64(0x401005), result[0].CallSiteAddress)
	assert.Equal(t, "syscall.Syscall", result[0].TargetFunction)
	assert.Equal(t, 41, result[0].SyscallNumber) // socket
	assert.True(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodGoWrapper, result[0].DeterminationMethod)

	// Second call
	assert.Equal(t, uint64(0x40100f), result[1].CallSiteAddress)
	assert.Equal(t, "syscall.Syscall6", result[1].TargetFunction)
	assert.Equal(t, 42, result[1].SyscallNumber) // connect
	assert.True(t, result[1].Resolved)
	assert.Equal(t, DeterminationMethodGoWrapper, result[1].DeterminationMethod)
}

func TestGoWrapperResolver_LoadSymbols_ClearsPriorState(t *testing.T) {
	resolver := NewGoWrapperResolver()

	// Simulate state from a previous LoadSymbols call
	resolver.symbols["old.Function"] = SymbolInfo{
		Name:    "old.Function",
		Address: 0x500000,
		Size:    100,
	}
	resolver.wrapperAddrs[0x500000] = "syscall.Syscall"
	resolver.hasSymbols = true

	// Call LoadSymbols with an ELF file that has no .gopclntab
	// This will return ErrNoPclntab, but the state should still be cleared
	err := resolver.LoadSymbols(&elf.File{})
	require.Error(t, err)

	// Verify that prior state was cleared
	assert.Empty(t, resolver.symbols, "symbols should be cleared on LoadSymbols")
	assert.Empty(t, resolver.wrapperAddrs, "wrapperAddrs should be cleared on LoadSymbols")
	assert.False(t, resolver.hasSymbols, "hasSymbols should be reset on LoadSymbols")
}

func TestGoWrapperResolver_DecodedInstruction_Args(t *testing.T) {
	// Test that DecodedInstruction.Args properly contains x86asm types
	decoder := NewX86Decoder()

	// mov $0x29, %eax
	code := []byte{0xb8, 0x29, 0x00, 0x00, 0x00}
	inst, err := decoder.Decode(code, 0x401000)

	require.NoError(t, err)
	require.Len(t, inst.Args, 2)

	// First arg should be EAX register
	reg, ok := inst.Args[0].(x86asm.Reg)
	assert.True(t, ok)
	assert.Equal(t, x86asm.EAX, reg)

	// Second arg should be an immediate
	imm, ok := inst.Args[1].(x86asm.Imm)
	assert.True(t, ok)
	assert.Equal(t, int64(0x29), int64(imm))
}
