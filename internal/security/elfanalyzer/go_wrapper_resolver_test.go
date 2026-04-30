//go:build test

package elfanalyzer

import (
	"debug/elf"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewX86GoWrapperResolver_NoPclntab(t *testing.T) {
	// An empty elf.File has no .gopclntab section.
	// NewX86GoWrapperResolver should return a usable resolver and ErrNoPclntab.
	resolver, err := NewX86GoWrapperResolver(&elf.File{})

	require.ErrorIs(t, err, ErrNoPclntab)
	assert.NotNil(t, resolver)
	assert.False(t, resolver.HasSymbols())

	// Returned resolver should be safe to call without panic.
	calls, decodeFailures := resolver.FindWrapperCalls([]byte{0x90}, 0)
	assert.Nil(t, calls)
	assert.Equal(t, 0, decodeFailures)
}

func TestNewX86GoWrapperResolver_FindWrapperCalls_NoWrappers(t *testing.T) {
	// A resolver created from an ELF without .gopclntab has no wrappers loaded.
	resolver, err := NewX86GoWrapperResolver(&elf.File{})
	require.ErrorIs(t, err, ErrNoPclntab)

	result, decodeFailures := resolver.FindWrapperCalls([]byte{0x90, 0x90}, 0x401000)
	assert.Nil(t, result)
	assert.Equal(t, 0, decodeFailures)
}

func TestX86GoWrapperResolver_FindWrapperCalls_WithWrapper(t *testing.T) {
	resolver := newX86GoWrapperResolver()

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

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, uint64(0x401005), result[0].CallSiteAddress)
	assert.Equal(t, "syscall.Syscall", result[0].TargetFunction)
	assert.Equal(t, 41, result[0].SyscallNumber) // socket syscall
	assert.True(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodGoWrapper, result[0].DeterminationMethod)
}

func TestX86GoWrapperResolver_FindWrapperCalls_UnresolvedSyscall(t *testing.T) {
	resolver := newX86GoWrapperResolver()

	// Register a wrapper
	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.Syscall"

	// 5 nops + call: fewer than maxBackwardScanSteps(6) instructions before the
	// call, so the scan exhausts the entire window without finding a mov to rax.
	// Expected: window_exhausted (not scan_limit_exceeded).
	baseAddr := uint64(0x401000)
	code := []byte{
		0x90, 0x90, 0x90, 0x90, 0x90, // nop x5
		0xe8, 0xf6, 0x0f, 0x00, 0x00, // call rel32 (target = 0x402000)
	}

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, uint64(0x401005), result[0].CallSiteAddress)
	assert.Equal(t, -1, result[0].SyscallNumber)
	assert.False(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodUnknownWindowExhausted, result[0].DeterminationMethod)
}

func TestX86GoWrapperResolver_FindWrapperCalls_WindowExhausted(t *testing.T) {
	resolver := newX86GoWrapperResolver()

	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.Syscall"

	// 7 nops + call: the entire buffer (7 nops + call = 8 entries, less than
	// maxRecentInstructionsToKeep) is scanned without finding a syscall-number
	// setter. Because the buffer is not full, all available instructions were
	// examined → window_exhausted (not scan_limit_exceeded).
	//
	// CALL is at offset 7 (addr 0x401007), nextPC = 0x40100C.
	// rel32 = 0x402000 - 0x40100C = 0xFF4 → bytes: f4 0f 00 00.
	baseAddr := uint64(0x401000)
	code := []byte{
		0x90, 0x90, 0x90, 0x90, 0x90, 0x90, 0x90, // nop x7
		0xe8, 0xf4, 0x0f, 0x00, 0x00, // call rel32 → 0x402000
	}

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, -1, result[0].SyscallNumber)
	assert.False(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodUnknownWindowExhausted, result[0].DeterminationMethod)
}

func TestX86GoWrapperResolver_FindWrapperCalls_ScanLimitExceeded(t *testing.T) {
	resolver := newX86GoWrapperResolver()

	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.Syscall"

	// maxRecentInstructionsToKeep nops + call: fills the rolling buffer exactly,
	// so the scan exhausts maxBackwardScanSteps steps before reaching the window
	// start. Expected: scan_limit_exceeded (not window_exhausted).
	//
	// CALL is at offset maxRecentInstructionsToKeep (one byte per nop).
	// rel32 = wrapperAddr - (baseAddr + maxRecentInstructionsToKeep + 5).
	baseAddr := uint64(0x401000)
	nopCount := maxRecentInstructionsToKeep
	rel32 := int32(wrapperAddr) - int32(baseAddr+uint64(nopCount)+5) //nolint:gosec // G115: addresses are test constants that fit int32
	code := make([]byte, nopCount+5)
	for i := range nopCount {
		code[i] = 0x90 // nop
	}
	code[nopCount] = 0xe8
	code[nopCount+1] = byte(rel32)
	code[nopCount+2] = byte(rel32 >> 8)
	code[nopCount+3] = byte(rel32 >> 16)
	code[nopCount+4] = byte(rel32 >> 24)

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, -1, result[0].SyscallNumber)
	assert.False(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodUnknownScanLimitExceeded, result[0].DeterminationMethod)
}

func TestX86GoWrapperResolver_ResolveSyscallArgument_ControlFlowBoundary(t *testing.T) {
	resolver := newX86GoWrapperResolver()

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

	syscallNum, method := resolver.resolveSyscallArgument(recentInstructions, resolver.decoder)
	assert.Equal(t, -1, syscallNum) // Should not find syscall number due to control flow boundary
	assert.Equal(t, DeterminationMethodUnknownControlFlowBoundary, method)
}

func TestX86GoWrapperResolver_ResolveSyscallArgument_RAX(t *testing.T) {
	resolver := newX86GoWrapperResolver()

	// Test with mov to RAX (64-bit)
	// 48 c7 c0 29 00 00 00  mov $0x29, %rax
	movCode := []byte{0x48, 0xc7, 0xc0, 0x29, 0x00, 0x00, 0x00}
	callCode := []byte{0xe8, 0x00, 0x00, 0x00, 0x00}

	decoder := NewX86Decoder()
	movInst, _ := decoder.Decode(movCode, 0x401000)
	callInst, _ := decoder.Decode(callCode, 0x401007)

	recentInstructions := []DecodedInstruction{movInst, callInst}

	syscallNum, method := resolver.resolveSyscallArgument(recentInstructions, resolver.decoder)
	assert.Equal(t, 41, syscallNum) // socket syscall
	assert.Equal(t, DeterminationMethodGoWrapper, method)
}

func TestX86GoWrapperResolver_ResolveSyscallArgument_EAX(t *testing.T) {
	resolver := newX86GoWrapperResolver()

	// Test with mov to EAX (32-bit, commonly used by Go compiler)
	// b8 29 00 00 00  mov $0x29, %eax
	movCode := []byte{0xb8, 0x29, 0x00, 0x00, 0x00}
	callCode := []byte{0xe8, 0x00, 0x00, 0x00, 0x00}

	decoder := NewX86Decoder()
	movInst, _ := decoder.Decode(movCode, 0x401000)
	callInst, _ := decoder.Decode(callCode, 0x401005)

	recentInstructions := []DecodedInstruction{movInst, callInst}

	syscallNum, method := resolver.resolveSyscallArgument(recentInstructions, resolver.decoder)
	assert.Equal(t, 41, syscallNum) // socket syscall
	assert.Equal(t, DeterminationMethodGoWrapper, method)
}

func TestX86GoWrapperResolver_ResolveSyscallArgument_OutOfRange(t *testing.T) {
	resolver := newX86GoWrapperResolver()

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

			syscallNum, method := resolver.resolveSyscallArgument(recentInstructions, resolver.decoder)
			assert.Equal(t, tt.expected, syscallNum, tt.reason)
			assert.Equal(t, tt.expectedMethod, method, tt.reason)
		})
	}
}

func TestX86GoWrapperResolver_IsInsideWrapper(t *testing.T) {
	resolver := newX86GoWrapperResolver()

	// Set up three non-overlapping ranges in unsorted order to verify that
	// loadFromPclntab sorting (and hence binary search) works correctly.
	resolver.wrapperRanges = []wrapperRange{
		{start: 0x403000, end: 0x403100}, // range C (added last)
		{start: 0x401000, end: 0x401100}, // range A (added first)
		{start: 0x402000, end: 0x402100}, // range B
	}

	tests := []struct {
		addr     uint64
		expected bool
		label    string
	}{
		{0x401000, true, "start of range A"},
		{0x4010ff, true, "last byte of range A"},
		{0x401100, false, "one past end of range A"},
		{0x402050, true, "middle of range B"},
		{0x403000, true, "start of range C"},
		{0x403100, false, "one past end of range C"},
		{0x400fff, false, "before all ranges"},
		{0x404000, false, "after all ranges"},
		{0x4011ff, false, "gap between A and B"},
	}

	// Sort as loadFromPclntab would.
	sort.Slice(resolver.wrapperRanges, func(i, j int) bool {
		return resolver.wrapperRanges[i].start < resolver.wrapperRanges[j].start
	})

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolver.IsInsideWrapper(tt.addr))
		})
	}
}

func TestX86GoWrapperResolver_GetWrapperAddresses(t *testing.T) {
	resolver := newX86GoWrapperResolver()

	// Initially empty
	addrs := resolver.GetWrapperAddresses()
	assert.Empty(t, addrs)

	// Add a wrapper
	resolver.wrapperAddrs[0x401000] = "syscall.Syscall"

	addrs = resolver.GetWrapperAddresses()
	assert.Len(t, addrs, 1)
	assert.Equal(t, GoSyscallWrapper("syscall.Syscall"), addrs[0x401000])
}

func TestX86GoWrapperResolver_GetSymbols(t *testing.T) {
	resolver := newX86GoWrapperResolver()

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

func TestX86GoWrapperResolver_KnownGoWrappers(t *testing.T) {
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

func TestX86GoWrapperResolver_FindWrapperCalls_MultipleCalls(t *testing.T) {
	resolver := newX86GoWrapperResolver()

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

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

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
