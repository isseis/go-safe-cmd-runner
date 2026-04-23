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

// TestScanGoWrapperCalls_ResolvedNetworkSyscall verifies that a BL targeting a
// known wrapper address preceded by MOVZ X0, #97 is detected as syscall 97 (socket).
func TestScanGoWrapperCalls_ResolvedNetworkSyscall(t *testing.T) {
	t.Parallel()

	const (
		textBase    = uint64(0x100000000)
		wrapperAddr = uint64(0x100001000)
	)

	// Layout:
	//   offset 0: MOVZ X0, #97
	//   offset 4: BL wrapperAddr  (target = textBase + 4 + offset_to_wrapper)
	blOffset := int32(wrapperAddr-textBase) - 4 // relative from BL instruction at offset 4
	code := buildCodeSlice(encodeMovzX0(97), encodeBLTo(blOffset))

	wrapperAddrs := map[uint64]string{wrapperAddr: "syscall.Syscall"}
	table := newStubSyscallTable()

	results := scanGoWrapperCalls(code, textBase, wrapperAddrs, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, 97, results[0].Number)
	assert.Equal(t, "socket", results[0].Name)
	assert.True(t, results[0].IsNetwork)
	assert.Equal(t, determinationMethodGoWrapper, results[0].DeterminationMethod)
	assert.Equal(t, textBase+4, results[0].Location)
}

// TestScanGoWrapperCalls_ResolvedW0 verifies that MOVZ W0, #98 is also resolved.
func TestScanGoWrapperCalls_ResolvedW0(t *testing.T) {
	t.Parallel()

	const (
		textBase    = uint64(0x100000000)
		wrapperAddr = uint64(0x100001000)
	)

	blOffset := int32(wrapperAddr-textBase) - 4
	code := buildCodeSlice(encodeMovzW0(98), encodeBLTo(blOffset))

	wrapperAddrs := map[uint64]string{wrapperAddr: "syscall.Syscall"}
	table := newStubSyscallTable()

	results := scanGoWrapperCalls(code, textBase, wrapperAddrs, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, 98, results[0].Number)
	assert.Equal(t, "connect", results[0].Name)
	assert.True(t, results[0].IsNetwork)
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
	assert.Equal(t, determinationMethodUnknownIndirect, results[0].DeterminationMethod)
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

	blOffset := int32(wrapperAddr-textBase) - 4
	code := buildCodeSlice(encodeMovzX0(97), encodeBLTo(blOffset))

	// The BL at offset 4 (= textBase+4) falls inside the stub range.
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
