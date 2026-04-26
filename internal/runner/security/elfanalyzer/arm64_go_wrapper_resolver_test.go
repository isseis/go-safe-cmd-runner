//go:build test

package elfanalyzer

import (
	"debug/elf"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/arch/arm64/arm64asm"
)

func TestNewARM64GoWrapperResolver_NoPclntab(t *testing.T) {
	// An empty elf.File has no .gopclntab section.
	// NewARM64GoWrapperResolver should return a usable resolver and ErrNoPclntab.
	resolver, err := NewARM64GoWrapperResolver(&elf.File{})

	require.ErrorIs(t, err, ErrNoPclntab)
	assert.NotNil(t, resolver)
	assert.False(t, resolver.HasSymbols())

	// Returned resolver should be safe to call without panic.
	calls, decodeFailures := resolver.FindWrapperCalls([]byte{0x1F, 0x20, 0x03, 0xD5}, 0)
	assert.Nil(t, calls)
	assert.Equal(t, 0, decodeFailures)
}

func TestARM64GoWrapperResolver_ImplementsInterface(_ *testing.T) {
	// Verify that ARM64GoWrapperResolver implements the GoWrapperResolver interface.
	var _ GoWrapperResolver = (*ARM64GoWrapperResolver)(nil)
}

// TestARM64GoWrapperResolver_FindWrapperCalls_ImmediateX0 verifies that a
// "mov x0, #syscallNum / bl <wrapper>" pattern resolves the syscall number.
//
// Code layout at baseAddr=0x401000:
//
//	0x401000: C0 18 80 D2  mov x0, #198   (socket syscall number)
//	0x401004: FF 03 00 94  bl 0x402000    (call to wrapper)
//
// BL offset: target (0x402000) - instAddr (0x401004) = 0xFFC
// imm26 = 0xFFC / 4 = 0x3FF
// BL encoding: 0x94000000 | 0x3FF = 0x940003FF → LE: {0xFF, 0x03, 0x00, 0x94}
func TestARM64GoWrapperResolver_FindWrapperCalls_ImmediateX0(t *testing.T) {
	resolver := newARM64GoWrapperResolver()

	// Manually register a wrapper at a known address
	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.Syscall"

	baseAddr := uint64(0x401000)
	code := []byte{
		0xC0, 0x18, 0x80, 0xD2, // mov x0, #198 (socket syscall number = 198)
		0xFF, 0x03, 0x00, 0x94, // bl 0x402000 (offset = wrapperAddr - instAddr = 0xFFC)
	}

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, uint64(0x401004), result[0].CallSiteAddress)
	assert.Equal(t, "syscall.Syscall", result[0].TargetFunction)
	assert.Equal(t, 198, result[0].SyscallNumber) // socket syscall on arm64
	assert.True(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodGoWrapper, result[0].DeterminationMethod)
}

// TestARM64GoWrapperResolver_FindWrapperCalls_ImmediateW0 verifies that a
// "mov w0, #syscallNum / bl <wrapper>" pattern (32-bit W0 register) also resolves
// the syscall number correctly.
//
// Code layout at baseAddr=0x401000:
//
//	0x401000: C0 18 80 52  mov w0, #198   (socket syscall number, W0 = 32-bit view)
//	0x401004: FF 03 00 94  bl 0x402000
func TestARM64GoWrapperResolver_FindWrapperCalls_ImmediateW0(t *testing.T) {
	resolver := newARM64GoWrapperResolver()

	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.Syscall"

	baseAddr := uint64(0x401000)
	code := []byte{
		0xC0, 0x18, 0x80, 0x52, // mov w0, #198 (32-bit W0 view)
		0xFF, 0x03, 0x00, 0x94, // bl 0x402000
	}

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, 198, result[0].SyscallNumber)
	assert.True(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodGoWrapper, result[0].DeterminationMethod)
}

// TestARM64GoWrapperResolver_FindWrapperCalls_Unresolved verifies that a BL without
// a preceding immediate assignment to X0/W0 results in Resolved=false.
// With only 2 nops before the BL (fewer than maxBackwardScanSteps=6), the scan
// exhausts the entire window — expected method is window_exhausted, not scan_limit_exceeded.
//
// Code layout at baseAddr=0x401000:
//
//	0x401000: 1F 20 03 D5  nop
//	0x401004: 1F 20 03 D5  nop
//	0x401008: FE 03 00 94  bl 0x402000   (offset = wrapperAddr - 0x401008 = 0xFF8)
//
// BL offset: 0xFF8, imm26 = 0xFF8/4 = 0x3FE
// BL encoding: 0x94000000 | 0x3FE = 0x940003FE → LE: {0xFE, 0x03, 0x00, 0x94}
func TestARM64GoWrapperResolver_FindWrapperCalls_Unresolved(t *testing.T) {
	resolver := newARM64GoWrapperResolver()

	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.Syscall"

	baseAddr := uint64(0x401000)
	code := []byte{
		0x1F, 0x20, 0x03, 0xD5, // nop
		0x1F, 0x20, 0x03, 0xD5, // nop
		0xFE, 0x03, 0x00, 0x94, // bl 0x402000 (offset = 0xFF8)
	}

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, uint64(0x401008), result[0].CallSiteAddress)
	assert.Equal(t, "syscall.Syscall", result[0].TargetFunction)
	assert.Equal(t, -1, result[0].SyscallNumber)
	assert.False(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodUnknownWindowExhausted, result[0].DeterminationMethod)
}

// TestARM64GoWrapperResolver_FindWrapperCalls_ControlFlowBoundary verifies that
// backward scanning stops at a control flow instruction (e.g., a branch).
//
// Code layout at baseAddr=0x401000:
//
//	0x401000: C0 18 80 D2  mov x0, #198     (before branch - should NOT be used)
//	0x401004: 02 00 00 14  b +8             (unconditional branch - boundary)
//	0x401008: 1F 20 03 D5  nop
//	0x40100C: FD 03 00 94  bl 0x402000      (offset = 0x402000 - 0x40100C = 0xFF4)
//
// BL offset: 0xFF4, imm26 = 0xFF4/4 = 0x3FD
// BL encoding: 0x94000000 | 0x3FD = 0x940003FD → LE: {0xFD, 0x03, 0x00, 0x94}
func TestARM64GoWrapperResolver_FindWrapperCalls_ControlFlowBoundary(t *testing.T) {
	resolver := newARM64GoWrapperResolver()

	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.Syscall"

	baseAddr := uint64(0x401000)
	code := []byte{
		0xC0, 0x18, 0x80, 0xD2, // mov x0, #198  (before control flow - should be blocked)
		0x02, 0x00, 0x00, 0x14, // b +8           (control flow boundary)
		0x1F, 0x20, 0x03, 0xD5, // nop
		0xFD, 0x03, 0x00, 0x94, // bl 0x402000
	}

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, uint64(0x40100C), result[0].CallSiteAddress)
	assert.Equal(t, -1, result[0].SyscallNumber)
	assert.False(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodUnknownControlFlowBoundary, result[0].DeterminationMethod)
}

func TestARM64GoWrapperResolver_FindWrapperCalls_NoWrappers(t *testing.T) {
	// A resolver with no wrappers registered should return nil immediately.
	resolver := newARM64GoWrapperResolver()

	code := []byte{
		0xC0, 0x18, 0x80, 0xD2, // mov x0, #198
		0xFF, 0x03, 0x00, 0x94, // bl 0x402000
	}

	result, decodeFailures := resolver.FindWrapperCalls(code, 0x401000)
	assert.Nil(t, result)
	assert.Equal(t, 0, decodeFailures)
}

func TestARM64GoWrapperResolver_FindWrapperCalls_IndirectBeforeControlFlow(t *testing.T) {
	resolver := newARM64GoWrapperResolver()

	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.RawSyscall6"

	baseAddr := uint64(0x401000)
	code := []byte{
		0x00, 0x00, 0x00, 0x94, // bl . (helper call, control flow)
		0xE0, 0x03, 0x01, 0xAA, // mov x0, x1 (indirect setting)
		0xFE, 0x03, 0x00, 0x94, // bl 0x402000
	}

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, -1, result[0].SyscallNumber)
	assert.False(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodUnknownIndirectSetting, result[0].DeterminationMethod)
}

func TestARM64GoWrapperResolver_FindWrapperCalls_GlobalLoadFromDataSection(t *testing.T) {
	resolver := newARM64GoWrapperResolver()

	wrapperAddr := uint64(0x402000)
	resolver.wrapperAddrs[wrapperAddr] = "syscall.RawSyscall"

	baseAddr := uint64(0x401000)
	code := []byte{
		0x1b, 0x89, 0x19, 0x90, // adrp x27, .+0x33120000
		0x60, 0xd7, 0x42, 0xf9, // ldr x0, [x27,#1448]
		0xFE, 0x03, 0x00, 0x94, // bl 0x402000
	}

	adrpInst, err := resolver.decoder.Decode(code[:4], baseAddr)
	require.NoError(t, err)
	a := adrpInst.arch.(arm64asm.Inst)
	rel, ok := a.Args[1].(arm64asm.PCRel)
	require.True(t, ok)
	loadAddr := uint64(int64(adrpInst.Offset&^uint64(0xfff))+int64(rel)) + 1448

	blob := make([]byte, 16)
	binary.LittleEndian.PutUint64(blob[:8], 25)
	resolver.decoder.SetDataSections([]arm64DataSection{{Addr: loadAddr, Data: blob}})

	result, decodeFailures := resolver.FindWrapperCalls(code, baseAddr)
	assert.Equal(t, 0, decodeFailures)

	require.Len(t, result, 1)
	assert.Equal(t, 25, result[0].SyscallNumber)
	assert.True(t, result[0].Resolved)
	assert.Equal(t, DeterminationMethodGoWrapper, result[0].DeterminationMethod)
}
