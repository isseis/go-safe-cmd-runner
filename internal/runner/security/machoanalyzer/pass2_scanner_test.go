//go:build test

package machoanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encodeMovzX0 encodes MOVZ X0, #imm, LSL #0.
func encodeMovzX0(imm uint32) uint32 { //nolint:unparam
	return 0xD2800000 | ((imm & 0xFFFF) << 5)
}

// encodeMovzW0 encodes MOVZ W0, #imm.
func encodeMovzW0(imm uint32) uint32 {
	return 0x52800000 | ((imm & 0xFFFF) << 5)
}

// encodeBLTo encodes a BL instruction that targets the given offset (in bytes)
// relative to the BL instruction itself. The offset must be a multiple of 4.
func encodeBLTo(offsetBytes int32) uint32 {
	imm26 := (offsetBytes / 4) & 0x03FFFFFF
	return 0x94000000 | uint32(imm26)
}

// encodeBTo encodes an unconditional B (no link) instruction targeting the
// given offset (in bytes) relative to the B instruction itself.
func encodeBTo(offsetBytes int32) uint32 {
	imm26 := (offsetBytes / 4) & 0x03FFFFFF
	return 0x14000000 | uint32(imm26)
}

// encodeStrSp8 encodes STR xN, [SP, #8] — stores register N to the trap
// argument slot used by Go's old stack ABI.
func encodeStrSp8(regN uint32) uint32 {
	// STR Xt,[Xn,#pimm]: bits[31:22]=0x3E4, imm12=1 (=8/8), Rn=SP(31), Rt=N
	return 0xF9000000 | (1 << 10) | (31 << 5) | regN
}

// encodeStpSp8 encodes STP xN, xM, [SP, #8] — stores register N (trap) and M
// (a1) to the first two argument slots used by Go's old stack ABI.
func encodeStpSp8(regN, regM uint32) uint32 {
	// STP Xt1,Xt2,[Xn,#imm]: bits[31:22]=0x2A4, imm7=1, Rt2=M, Rn=SP(31), Rt1=N
	return 0xA9000000 | (1 << 15) | (regM << 10) | (31 << 5) | regN
}

// encodeMovzReg encodes MOVZ xN, #imm, LSL#0 (64-bit) for any register N.
func encodeMovzReg(regN, imm uint32) uint32 { //nolint:unparam
	return 0xD2800000 | ((imm & 0xFFFF) << 5) | regN
}

// TestScanGoWrapperCalls_ResolvedNetworkSyscall verifies that a BL targeting a
// known wrapper address preceded by the Go old-stack-ABI store pattern
// (MOVZ x5, #97; STR x5, [SP, #8]) is detected as syscall 97 (socket).
func TestScanGoWrapperCalls_ResolvedNetworkSyscall(t *testing.T) {
	t.Parallel()

	const (
		textBase    = uint64(0x100000000)
		wrapperAddr = uint64(0x100001000)
	)

	// Layout:
	//   offset 0: MOVZ X5, #97
	//   offset 4: STR X5, [SP, #8]   (trap argument slot — old stack ABI)
	//   offset 8: BL wrapperAddr
	blOffset := int32(wrapperAddr-textBase) - 8 // relative from BL at offset 8
	code := buildCodeSlice(encodeMovzReg(5, 97), encodeStrSp8(5), encodeBLTo(blOffset))

	wrapperAddrs := map[uint64]string{wrapperAddr: "syscall.Syscall"}
	table := newStubSyscallTable()

	results := scanGoWrapperCalls(code, textBase, wrapperAddrs, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, 97, results[0].Number)
	assert.Equal(t, "socket", results[0].Name)
	assert.Equal(t, determinationMethodGoWrapper, results[0].Occurrences[0].DeterminationMethod)
	assert.Equal(t, textBase+8, results[0].Occurrences[0].Location)
}

// TestScanGoWrapperCalls_ResolvedW0 verifies that MOVZ W0 (32-bit) stored via
// STR X0,[SP,#8] is also resolved correctly.
func TestScanGoWrapperCalls_ResolvedW0(t *testing.T) {
	t.Parallel()

	const (
		textBase    = uint64(0x100000000)
		wrapperAddr = uint64(0x100001000)
	)

	// MOVZ W0, #98; STR X0, [SP, #8]; BL wrapperAddr
	blOffset := int32(wrapperAddr-textBase) - 8
	code := buildCodeSlice(encodeMovzW0(98), encodeStrSp8(0), encodeBLTo(blOffset))

	wrapperAddrs := map[uint64]string{wrapperAddr: "syscall.Syscall"}
	table := newStubSyscallTable()

	results := scanGoWrapperCalls(code, textBase, wrapperAddrs, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, 98, results[0].Number)
	assert.Equal(t, "connect", results[0].Name)
}

// TestScanGoWrapperCalls_UnresolvedX0 verifies that a BL to a wrapper without
// a preceding X0 load produces Number=-1 and method="unknown:indirect_setting".
func TestScanGoWrapperCalls_UnresolvedX0(t *testing.T) {
	t.Parallel()

	const (
		textBase    = uint64(0x100000000)
		wrapperAddr = uint64(0x100001000)
	)

	blOffset := int32(wrapperAddr - textBase)
	code := buildCodeSlice(encodeBLTo(blOffset))

	wrapperAddrs := map[uint64]string{wrapperAddr: "syscall.Syscall"}
	table := newStubSyscallTable()

	results := scanGoWrapperCalls(code, textBase, wrapperAddrs, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, -1, results[0].Number)
	assert.Equal(t, determinationMethodUnknownIndirect, results[0].Occurrences[0].DeterminationMethod)
}

// TestScanGoWrapperCalls_UnknownTarget verifies that a BL to a non-wrapper
// address is not reported.
func TestScanGoWrapperCalls_UnknownTarget(t *testing.T) {
	t.Parallel()

	const textBase = uint64(0x100000000)
	// BL targets offset +8, but that address is not in wrapperAddrs.
	code := buildCodeSlice(encodeMovzX0(97), encodeBLTo(4))

	wrapperAddrs := map[uint64]string{0x100001000: "syscall.Syscall"}
	results := scanGoWrapperCalls(code, textBase, wrapperAddrs, nil, newStubSyscallTable())

	assert.Empty(t, results)
}

// TestScanGoWrapperCalls_InsideStubRange verifies that a BL originating inside
// a known stub range is excluded.
func TestScanGoWrapperCalls_InsideStubRange(t *testing.T) {
	t.Parallel()

	const (
		textBase    = uint64(0x100000000)
		wrapperAddr = uint64(0x100001000)
	)

	// MOVZ X5, #97; STR X5, [SP, #8]; BL wrapperAddr
	// BL is at offset 8 (= textBase+8), which falls inside the stub range below.
	blOffset := int32(wrapperAddr-textBase) - 8
	code := buildCodeSlice(encodeMovzReg(5, 97), encodeStrSp8(5), encodeBLTo(blOffset))

	stubRanges := []funcRange{{start: textBase, end: textBase + 100}}
	wrapperAddrs := map[uint64]string{wrapperAddr: "syscall.Syscall"}

	results := scanGoWrapperCalls(code, textBase, wrapperAddrs, stubRanges, newStubSyscallTable())

	assert.Empty(t, results, "BL inside stub range must be excluded")
}

// TestScanGoWrapperCalls_EmptyWrapperAddrs verifies that an empty wrapperAddrs
// map results in no entries (fast path).
func TestScanGoWrapperCalls_EmptyWrapperAddrs(t *testing.T) {
	t.Parallel()

	code := buildCodeSlice(encodeMovzX0(97), encodeBLTo(4))
	results := scanGoWrapperCalls(code, 0x100000000, nil, nil, newStubSyscallTable())
	assert.Nil(t, results)
}

// TestBuildWrapperAddrs verifies that only known syscall stub names are included.
func TestBuildWrapperAddrs(t *testing.T) {
	t.Parallel()

	funcs := map[string]MachoPclntabFunc{
		"syscall.Syscall":  {Entry: 0x100, End: 0x200},
		"syscall.Syscall6": {Entry: 0x300, End: 0x400},
		"main.notAWrapper": {Entry: 0x500, End: 0x600},
	}
	addrs := buildWrapperAddrs(funcs)

	assert.Len(t, addrs, 2)
	assert.Contains(t, addrs, uint64(0x100))
	assert.Contains(t, addrs, uint64(0x300))
	assert.NotContains(t, addrs, uint64(0x500))
}

// TestGetBLTarget_ValidBL verifies BL target computation.
func TestGetBLTarget_ValidBL(t *testing.T) {
	t.Parallel()

	// BL +8 from address 0x100000000: target = 0x100000000 + 8 = 0x100000008
	word := encodeBLTo(8)
	target, ok := getBLTarget(word, 0x100000000)
	require.True(t, ok)
	assert.Equal(t, uint64(0x100000008), target)
}

// TestGetBLTarget_NotBL verifies that a non-BL instruction returns (0, false).
func TestGetBLTarget_NotBL(t *testing.T) {
	t.Parallel()

	const nop = uint32(0xD503201F)
	_, ok := getBLTarget(nop, 0x100000000)
	assert.False(t, ok)
}

// TestScanGoWrapperCalls_StackABI_STPPattern verifies that STP xN,xM,[SP,#8]
// (the two-argument store form used by syscall.Syscall) is also resolved.
func TestScanGoWrapperCalls_StackABI_STPPattern(t *testing.T) {
	t.Parallel()

	const (
		textBase    = uint64(0x100000000)
		wrapperAddr = uint64(0x100001000)
	)

	// Mirrors the Go compiler output for syscall.Syscall(socket, ...):
	//   MOVZ X5, #97
	//   STP  X5, X4, [SP, #8]   (trap at SP+8, a1 at SP+16)
	//   BL   wrapperAddr
	blOffset := int32(wrapperAddr-textBase) - 8
	code := buildCodeSlice(encodeMovzReg(5, 97), encodeStpSp8(5, 4), encodeBLTo(blOffset))

	wrapperAddrs := map[uint64]string{wrapperAddr: "syscall.Syscall"}
	results := scanGoWrapperCalls(code, textBase, wrapperAddrs, nil, newStubSyscallTable())

	require.Len(t, results, 1)
	assert.Equal(t, 97, results[0].Number)
	assert.Equal(t, determinationMethodGoWrapper, results[0].Occurrences[0].DeterminationMethod)
}

// TestScanGoWrapperCalls_StubTrampoline verifies that a BL targeting a single-
// instruction trampoline stub (B wrapperAddr) is resolved as a wrapper call.
// The Go linker sometimes emits near-branch stubs for far calls.
func TestScanGoWrapperCalls_StubTrampoline(t *testing.T) {
	t.Parallel()

	const (
		textBase    = uint64(0x100000000)
		wrapperAddr = uint64(0x100002000) // actual wrapper
		stubAddr    = uint64(0x100001000) // trampoline: B wrapperAddr
	)

	// Layout in __text (relative to textBase):
	//   offset 0x000: MOVZ X5, #97
	//   offset 0x004: STR  X5, [SP, #8]
	//   offset 0x008: BL   stubAddr        (0x1000 bytes forward)
	//   ...
	//   offset 0x1000: B   wrapperAddr     (0x1000 bytes forward = actual wrapper)
	//   offset 0x2000: <wrapper body>      (just a NOP for this test)

	totalLen := int(wrapperAddr-textBase) + 4 // include first word of wrapper
	code := make([]byte, totalLen)

	// Encode call site at offset 0
	putInstr := func(off int, w uint32) {
		code[off] = byte(w)
		code[off+1] = byte(w >> 8)
		code[off+2] = byte(w >> 16)
		code[off+3] = byte(w >> 24)
	}
	putInstr(0, encodeMovzReg(5, 97))
	putInstr(4, encodeStrSp8(5))
	blToStub := int32(stubAddr-textBase) - 8 // BL at offset 8, targeting stubAddr
	putInstr(8, encodeBLTo(blToStub))

	// Encode stub trampoline at stubAddr-textBase = 0x1000
	stubOff := int(stubAddr - textBase)
	bToWrapper := int32(wrapperAddr - stubAddr) // B at stubAddr, targeting wrapperAddr
	putInstr(stubOff, encodeBTo(bToWrapper))

	// wrapperAddrs only contains the actual wrapper, not the stub
	wrapperAddrs := map[uint64]string{wrapperAddr: "syscall.Syscall"}
	results := scanGoWrapperCalls(code, textBase, wrapperAddrs, nil, newStubSyscallTable())

	require.Len(t, results, 1)
	assert.Equal(t, 97, results[0].Number)
	assert.Equal(t, determinationMethodGoWrapper, results[0].Occurrences[0].DeterminationMethod)
	assert.Equal(t, textBase+8, results[0].Occurrences[0].Location)
}

// TestIsKnownWrapper_DirectAndStub verifies isKnownWrapper for both direct
// wrapper addresses and single-instruction stub trampolines.
func TestIsKnownWrapper_DirectAndStub(t *testing.T) {
	t.Parallel()

	const (
		textBase    = uint64(0x100000000)
		wrapperAddr = uint64(0x100002000)
		stubAddr    = uint64(0x100001000)
	)

	// Build a minimal code slice with a stub at stubAddr.
	totalLen := int(wrapperAddr-textBase) + 4
	code := make([]byte, totalLen)
	stubOff := int(stubAddr - textBase)
	bToWrapper := int32(wrapperAddr - stubAddr)
	w := encodeBTo(bToWrapper)
	code[stubOff] = byte(w)
	code[stubOff+1] = byte(w >> 8)
	code[stubOff+2] = byte(w >> 16)
	code[stubOff+3] = byte(w >> 24)

	wrapperAddrs := map[uint64]string{wrapperAddr: "syscall.Syscall"}

	assert.True(t, isKnownWrapper(code, textBase, wrapperAddr, wrapperAddrs), "direct wrapper must match")
	assert.True(t, isKnownWrapper(code, textBase, stubAddr, wrapperAddrs), "stub trampoline must match")
	assert.False(t, isKnownWrapper(code, textBase, textBase+100, wrapperAddrs), "unknown address must not match")
}
