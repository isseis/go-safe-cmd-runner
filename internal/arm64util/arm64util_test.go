//go:build test

package arm64util

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildCodeSliceX0 assembles a slice of arm64 instructions for X0-scan tests.
func buildCodeSliceX0(instrs ...uint32) []byte {
	buf := make([]byte, len(instrs)*4)
	for i, instr := range instrs {
		binary.LittleEndian.PutUint32(buf[i*4:], instr)
	}
	return buf
}

// encodeMovzX0 encodes MOVZ X0, #imm, LSL #0.
func encodeMovzX0(imm uint32) uint32 { return movzX0Base | ((imm & 0xFFFF) << imm16Shift) }

// encodeMovzX0Lsl16 encodes MOVZ X0, #imm, LSL #16.
func encodeMovzX0Lsl16(imm uint32) uint32 { return movzX0Lsl16 | ((imm & 0xFFFF) << imm16Shift) }

// encodeMovkX0 encodes MOVK X0, #imm, LSL #0.
func encodeMovkX0(imm uint32) uint32 { return movkX0Base | ((imm & 0xFFFF) << imm16Shift) }

// encodeMovzW0 encodes MOVZ W0, #imm.
func encodeMovzW0(imm uint32) uint32 { return movzW0Base | ((imm & 0xFFFF) << imm16Shift) }

// encodeBLX0 encodes a BL instruction with a small forward offset.
const blPlaceholder = uint32(0x94000001) // BL +4

// TestBackwardScanX0_ImmediateX0 verifies MOVZ X0, #97 before BL → returns 97.
func TestBackwardScanX0_ImmediateX0(t *testing.T) {
	t.Parallel()
	code := buildCodeSliceX0(encodeMovzX0(97), blPlaceholder)
	num, ok := BackwardScanX0(code, 4) // BL is at offset 4
	require.True(t, ok)
	assert.Equal(t, 97, num)
}

// TestBackwardScanX0_ImmediateW0 verifies MOVZ W0, #98 before BL → returns 98.
func TestBackwardScanX0_ImmediateW0(t *testing.T) {
	t.Parallel()
	code := buildCodeSliceX0(encodeMovzW0(98), blPlaceholder)
	num, ok := BackwardScanX0(code, 4)
	require.True(t, ok)
	assert.Equal(t, 98, num)
}

// TestBackwardScanX0_MovzLsl16PlusMovk verifies a 32-bit value:
// MOVZ X0, #0x0002, LSL#16 + MOVK X0, #0x0061 → value = 0x00020061 = 131169.
func TestBackwardScanX0_MovzLsl16PlusMovk(t *testing.T) {
	t.Parallel()
	code := buildCodeSliceX0(
		encodeMovzX0Lsl16(0x0002),
		encodeMovkX0(0x0061),
		blPlaceholder,
	)
	num, ok := BackwardScanX0(code, 8)
	require.True(t, ok)
	assert.Equal(t, 0x00020061, num)
}

// TestBackwardScanX0_NoPrecedingInstruction verifies that a BL with no
// preceding X0 load returns (0, false).
func TestBackwardScanX0_NoPrecedingInstruction(t *testing.T) {
	t.Parallel()
	code := buildCodeSliceX0(blPlaceholder)
	num, ok := BackwardScanX0(code, 0)
	assert.False(t, ok)
	assert.Zero(t, num)
}

// TestBackwardScanX0_ControlFlowBoundary verifies that a B/BL between the
// MOVZ X0 and the target BL causes the scan to stop → (0, false).
func TestBackwardScanX0_ControlFlowBoundary(t *testing.T) {
	t.Parallel()
	code := buildCodeSliceX0(
		encodeMovzX0(97),
		blPlaceholder, // acts as a control-flow boundary
		blPlaceholder, // the BL we're scanning back from
	)
	num, ok := BackwardScanX0(code, 8)
	assert.False(t, ok, "scan must stop at BL boundary")
	assert.Zero(t, num)
}

// TestBackwardScanX0_IndirectWriteStopsScal verifies that a non-MOV write to X0
// (e.g., ADD X0, X1, X2 — encoded as a 64-bit instruction with Rd=0) stops the scan.
// ADD X0, X1, X2: sf=1, op=0b0001011_000, Rm=X2(2), Rn=X1(1), Rd=X0(0) → 0x8B020020
func TestBackwardScanX0_IndirectWriteStopsScal(t *testing.T) {
	t.Parallel()
	const addX0X1X2 = uint32(0x8B020020) // ADD X0, X1, X2
	code := buildCodeSliceX0(
		encodeMovzX0(97),
		addX0X1X2, // overwrites X0 non-immediately → scan stops
		blPlaceholder,
	)
	num, ok := BackwardScanX0(code, 8)
	assert.False(t, ok, "indirect write to X0 must stop scan")
	assert.Zero(t, num)
}
