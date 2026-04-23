//go:build test

package machoanalyzer

import (
	"encoding/binary"
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

// svcEncodingPass1 is the little-endian encoding of "svc #0x80" for arm64.
// Named with Pass1 suffix to avoid redeclaration with svc_scanner_test.go.
const svcEncodingPass1 = uint32(0xD4001001)

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

// buildCodeSlice assembles a sequence of 32-bit ARM64 instructions into a byte slice.
func buildCodeSlice(instrs ...uint32) []byte {
	buf := make([]byte, len(instrs)*4)
	for i, instr := range instrs {
		binary.LittleEndian.PutUint32(buf[i*4:], instr)
	}
	return buf
}

// TestScanSVCWithX16_ImmediateNetworkSyscall verifies that MOVZ X16, #98 + svc
// produces Number=98 (connect), IsNetwork=true, Method="immediate".
func TestScanSVCWithX16_ImmediateNetworkSyscall(t *testing.T) {
	t.Parallel()
	// BSD syscall 98 = connect (network), 0x2000000+98 = 0x2000062
	// MOVZ X16, #98: 0xD2800010 | (98 << 5) = 0xD2800010 | 0xC40 = 0xD2800C50
	// Wait, imm=98 dec = 0x62, (0x62 << 5) = 0xC40
	// 0xD2800010 | 0xC40 = 0xD2800C50... let me recalculate
	// encodeMovzX16(98) = 0xD2800010 | ((98 & 0xFFFF) << 5) = 0xD2800010 | (98*32) = 0xD2800010 | 0xC40
	// = 0xD2800C50... hmm, that gives Rd=0x10 | (imm<<5) part?
	// Actually the encoding is:
	//   [31]:    sf=1
	//   [30:29]: opc=10 (MOVZ)
	//   [28:23]: 100101 = 0x25
	//   [22:21]: hw=00 (LSL #0)
	//   [20:5]:  imm16
	//   [4:0]:   Rd=16=0x10
	// So base = 0xD2800010 = 1101 0010 1000 0000 0000 0000 0001 0000
	// imm16 goes to bits [20:5], so shift by 5
	// For imm=98: (98 << 5) = 3136 = 0xC40
	// base | (98<<5) = 0xD2800010 | 0xC40 = 0xD2800C50... wait that shifts into bit 11
	// Actually imm16Mask = 0x001FFFE0 (bits [20:5]), imm16Shift = 5
	// encodeMovzX16(98) = 0xD2800010 | ((98 & 0xFFFF) << 5)
	// = 0xD2800010 | 0xC40 = 0xD2800C50
	// Let me verify: 0xD2800C50 & ^imm16Mask = 0xD2800C50 & 0xFFE0001F
	//   = 0xD2800010... wait: 0xD2800C50 = 1101 0010 1000 0000 0000 1100 0101 0000
	//   ^imm16Mask = ~0x001FFFE0 = 0xFFE0001F
	//   0xD2800C50 & 0xFFE0001F = 1101 0010 1000 0000 0000 0000 0001 0000 = 0xD2800010 ✓
	// And (0xD2800C50 & imm16Mask) >> 5 = (0xC40) >> 5 = 0x62 = 98 ✓

	code := buildCodeSlice(encodeMovzX16(98), svcEncodingPass1)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 4 // second instruction

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, 98, results[0].Number)
	assert.Equal(t, "connect", results[0].Name)
	assert.True(t, results[0].IsNetwork)
	assert.Equal(t, determinationMethodImmediate, results[0].DeterminationMethod)
	assert.Equal(t, "", results[0].Source)
}

// TestScanSVCWithX16_ImmediateNonNetworkSyscall verifies MOVZ X16, #3 + svc
// produces Number=3, IsNetwork=false.
func TestScanSVCWithX16_ImmediateNonNetworkSyscall(t *testing.T) {
	t.Parallel()
	code := buildCodeSlice(encodeMovzX16(3), svcEncodingPass1)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 4

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, 3, results[0].Number)
	assert.Equal(t, "read", results[0].Name)
	assert.False(t, results[0].IsNetwork)
	assert.Equal(t, determinationMethodImmediate, results[0].DeterminationMethod)
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
		svcEncodingPass1,
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
	code := buildCodeSlice(encodeLDRX16SP(24), svcEncodingPass1)
	const textBase = uint64(0x100000000)
	svcAddr := textBase + 4

	table := newStubSyscallTable()
	results := scanSVCWithX16([]uint64{svcAddr}, code, textBase, nil, table)

	require.Len(t, results, 1)
	assert.Equal(t, -1, results[0].Number)
	assert.Equal(t, determinationMethodUnknownIndirect, results[0].DeterminationMethod)
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
		svcEncodingPass1,  // offset 8: svc #0x80
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
	code := buildCodeSlice(encodeMovzX16(98), svcEncodingPass1)
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
	code := buildCodeSlice(encodeMovzX16(98), svcEncodingPass1)
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
	code := buildCodeSlice(encodeMovzX16(98), svcEncodingPass1)
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
			assert.Equal(t, determinationMethodUnknownIndirect, results[0].DeterminationMethod)
			assert.Equal(t, tc.svcAddr, results[0].Location)
		})
	}
}

// TestArm64BackwardScanX16_NoPrecedingInstruction verifies that a svc with no
// preceding MOVZ/MOVK instructions returns (0, false).
func TestArm64BackwardScanX16_NoPrecedingInstruction(t *testing.T) {
	t.Parallel()
	// Just the svc instruction at offset 0.
	code := buildCodeSlice(svcEncodingPass1)
	num, ok := arm64util.BackwardScanX16(code, 0)
	assert.False(t, ok)
	assert.Zero(t, num)
}
