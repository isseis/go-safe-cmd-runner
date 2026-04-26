//go:build test

package machoanalyzer

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/arm64util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSyscallTable is a minimal in-process syscall table for tests.
// It maps a few well-known BSD syscall numbers to names and network flags.
type stubSyscallTable struct {
	entries map[int]struct {
		name      string
		isNetwork bool
	}
}

func newStubSyscallTable() *stubSyscallTable {
	return &stubSyscallTable{
		entries: map[int]struct {
			name      string
			isNetwork bool
		}{
			3:  {name: "read", isNetwork: false},
			49: {name: "munmap", isNetwork: false},
			97: {name: "socket", isNetwork: true},
			98: {name: "connect", isNetwork: true},
		},
	}
}

func (t *stubSyscallTable) GetSyscallName(number int) string {
	if e, ok := t.entries[number]; ok {
		return e.name
	}
	return ""
}

func (t *stubSyscallTable) IsNetworkSyscall(number int) bool {
	if e, ok := t.entries[number]; ok {
		return e.isNetwork
	}
	return false
}

// encodeMovzX16 encodes MOVZ X16, #imm (LSL #0).
// ARM64: sf=1, opc=10, [28:23]=100101, hw=00, imm16, Rd=16
func encodeMovzX16(imm uint32) uint32 {
	return 0xD2800010 | ((imm & 0xFFFF) << 5)
}

// encodeMovzX16Lsl16 encodes MOVZ X16, #imm, LSL #16.
func encodeMovzX16Lsl16(imm uint32) uint32 {
	return 0xD2A00010 | ((imm & 0xFFFF) << 5)
}

// encodeMovkX16 encodes MOVK X16, #imm (LSL #0).
func encodeMovkX16(imm uint32) uint32 {
	return 0xF2800010 | ((imm & 0xFFFF) << 5)
}

// encodeBL encodes a BL instruction with a PC-relative offset (in bytes).
// The offset must be a multiple of 4.
func encodeBL(offsetBytes int32) uint32 {
	imm26 := (offsetBytes / 4) & 0x03FFFFFF
	return 0x94000000 | uint32(imm26)
}

// encodeLDRX16SP encodes LDR X16, [SP, #imm] where imm is a multiple of 8.
// This is used to represent an indirect load that the scanner cannot resolve.
func encodeLDRX16SP(offset uint32) uint32 {
	// STR/LDR X16, [SP, #imm]: [31:30]=11, [29:27]=111, [26]=0, [25:24]=01, imm12=[21:10], Rn=SP(31), Rt=16
	imm12 := (offset / 8) & 0xFFF
	return 0xF9400010 | (imm12 << 10) | (31 << 5)
}

// TestScanSVCWithX16_ImmediateNetworkSyscall verifies that MOVZ X16, #98 + svc
// produces Number=98 (connect), IsNetwork=true, Method="immediate".
func TestScanSVCWithX16_ImmediateNetworkSyscall(t *testing.T) {
	t.Parallel()
	code := buildCodeSlice(encodeMovzX16(98), svcEncoding)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 4 // second instruction

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, 98, results[0].Number)
	assert.Equal(t, "connect", results[0].Name)
	assert.True(t, results[0].IsNetwork)
	assert.Equal(t, determinationMethodImmediate, results[0].Occurrences[0].DeterminationMethod)
	assert.Equal(t, "", results[0].Occurrences[0].Source)
}

// TestScanSVCWithX16_ImmediateNonNetworkSyscall verifies MOVZ X16, #3 + svc
// produces Number=3, IsNetwork=false.
func TestScanSVCWithX16_ImmediateNonNetworkSyscall(t *testing.T) {
	t.Parallel()
	code := buildCodeSlice(encodeMovzX16(3), svcEncoding)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 4

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, 3, results[0].Number)
	assert.Equal(t, "read", results[0].Name)
	assert.False(t, results[0].IsNetwork)
	assert.Equal(t, determinationMethodImmediate, results[0].Occurrences[0].DeterminationMethod)
}

// TestScanSVCWithX16_BSDPrefix32bit verifies a 32-bit value with BSD prefix:
// MOVZ X16, #0x200, LSL#16 + MOVK X16, #0x62 + svc → Number=98.
func TestScanSVCWithX16_BSDPrefix32bit(t *testing.T) {
	t.Parallel()
	// BSD prefix 0x2000000 = 0x200 << 16, syscall 98 = 0x62
	// MOVZ X16, #0x0200, LSL#16 + MOVK X16, #0x0062
	code := buildCodeSlice(
		encodeMovzX16Lsl16(0x0200), // sets X16 = 0x02000000
		encodeMovkX16(0x0062),      // X16 |= 0x62 → 0x02000062
		svcEncoding,
	)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 8 // third instruction

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, 98, results[0].Number, "expected BSD prefix stripped: 0x2000062 - 0x2000000 = 98")
}

// TestScanSVCWithX16_IndirectLoad verifies that LDR X16, [SP, #N] + svc
// produces Number=-1, Method="unknown:indirect_setting".
func TestScanSVCWithX16_IndirectLoad(t *testing.T) {
	t.Parallel()
	code := buildCodeSlice(encodeLDRX16SP(24), svcEncoding)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 4

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, -1, results[0].Number)
	assert.Equal(t, determinationMethodUnknownIndirect, results[0].Occurrences[0].DeterminationMethod)
}

// TestScanSVCWithX16_ControlFlowBoundary verifies that a BL instruction between
// MOVZ X16, #98 and svc causes the backward scan to stop, yielding Number=-1.
func TestScanSVCWithX16_ControlFlowBoundary(t *testing.T) {
	t.Parallel()
	// Layout (ascending addresses):
	//   offset 0: MOVZ X16, #98  — would set x16=98
	//   offset 4: BL target      — control-flow boundary; backward scan stops here
	//   offset 8: svc #0x80      — starting point for backward scan
	//
	// The backward scan from svc (startIdx=1) first sees BL at i=1 and stops,
	// so MOVZ at i=0 is never reached → Number=-1.
	code := buildCodeSlice(
		encodeMovzX16(98), // offset 0: MOVZ X16, #98
		encodeBL(-4),      // offset 4: BL (control-flow; scan stops here)
		svcEncoding,       // offset 8: svc #0x80
	)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 8 // third instruction

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, -1, results[0].Number, "expected scan to stop at BL, producing unknown result")
}

// TestScanSVCWithX16_SVCInsideStubRange verifies that svc inside a known stub
// range is excluded from Pass 1 results.
func TestScanSVCWithX16_SVCInsideStubRange(t *testing.T) {
	t.Parallel()
	code := buildCodeSlice(encodeMovzX16(98), svcEncoding)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 4

	// Mark the entire code range as a stub range.
	stubRanges := []funcRange{{start: textBase, end: textBase + 100}}

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, stubRanges, table)

	assert.Empty(t, results, "svc inside stub range should be excluded from Pass 1")
}

// TestScanSVCWithX16_SVCOutsideStubRange verifies that svc outside stub ranges
// is included in Pass 1 results.
func TestScanSVCWithX16_SVCOutsideStubRange(t *testing.T) {
	t.Parallel()
	code := buildCodeSlice(encodeMovzX16(98), svcEncoding)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 4

	// Stub range does not cover the svc address.
	stubRanges := []funcRange{{start: textBase + 0x1000, end: textBase + 0x2000}}

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, stubRanges, table)

	require.Len(t, results, 1, "svc outside stub range should be included in Pass 1")
	assert.Equal(t, 98, results[0].Number)
}

// TestBuildStubRanges verifies that only known syscall stub names produce ranges.
func TestBuildStubRanges(t *testing.T) {
	t.Parallel()
	funcs := map[string]MachoPclntabFunc{
		"syscall.Syscall":  {Entry: 0x100, End: 0x200},
		"syscall.Syscall6": {Entry: 0x300, End: 0x400},
		"not.a.stub":       {Entry: 0x500, End: 0x600},
	}
	ranges := buildStubRanges(funcs)
	assert.Len(t, ranges, 2, "expected only known stub names to produce ranges")
	// Verify sorted order.
	for i := 1; i < len(ranges); i++ {
		assert.LessOrEqual(t, ranges[i-1].start, ranges[i].start)
	}
}

// TestScanSVCWithX16_OutOfBoundsAddress verifies that an svc address outside the
// text section produces an unknown-syscall entry instead of panicking.
func TestScanSVCWithX16_OutOfBoundsAddress(t *testing.T) {
	t.Parallel()
	code := buildCodeSlice(encodeMovzX16(98), svcEncoding)
	const textBase = uint64(0x100000000)
	table := newStubSyscallTable()

	tests := []struct {
		name    string
		svcAddr uint64
	}{
		{"below textBase", textBase - 4},
		{"beyond end", textBase + uint64(len(code))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			results := scanSVCWithX16([]uint64{tc.svcAddr}, code, textBase, nil, table)
			require.Len(t, results, 1)
			assert.Equal(t, -1, results[0].Number)
			assert.Equal(t, determinationMethodUnknownIndirect, results[0].Occurrences[0].DeterminationMethod)
			assert.Equal(t, tc.svcAddr, results[0].Occurrences[0].Location)
		})
	}
}

// TestArm64BackwardScanX16_NoPrecedingInstruction verifies that a svc with no
// preceding MOVZ/MOVK instructions returns (0, false).
func TestArm64BackwardScanX16_NoPrecedingInstruction(t *testing.T) {
	t.Parallel()
	code := buildCodeSlice(svcEncoding)
	num, ok := arm64util.BackwardScanX16(code, 0)
	assert.False(t, ok)
	assert.Zero(t, num)
}
