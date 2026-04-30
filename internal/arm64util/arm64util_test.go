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

// encodeBLX0 encodes a BL instruction with a small forward offset.
const blPlaceholder = uint32(0x94000001) // BL +4

// encodeStrSp8 encodes STR xN, [SP, #8].
func encodeStrSp8(regN uint32) uint32 {
	// STR Xt,[Xn,#pimm]: bits[31:22]=0x3E4, imm12=1, Rn=SP(31), Rt=N
	return 0xF9000000 | (1 << 10) | (31 << 5) | regN
}

// encodeStpSp8 encodes STP xN, xM, [SP, #8].
func encodeStpSp8(regN, regM uint32) uint32 {
	// STP Xt1,Xt2,[Xn,#imm]: bits[31:22]=0x2A4, imm7=1, Rt2=M, Rn=SP(31), Rt1=N
	return 0xA9000000 | (1 << 15) | (regM << 10) | (31 << 5) | regN
}

// encodeMovzReg encodes MOVZ xN, #imm (LSL#0, 64-bit).
func encodeMovzReg(regN, imm uint32) uint32 { //nolint:unparam
	return 0xD2800000 | ((imm & 0xFFFF) << 5) | regN
}

// TestBackwardScanStackTrap_STRPattern verifies STR x5,#imm + STR x5,[SP,#8] + BL → syscall number.
func TestBackwardScanStackTrap_STRPattern(t *testing.T) {
	t.Parallel()
	// MOVZ X5, #73; STR X5, [SP, #8]; BL placeholder
	code := buildCodeSliceX0(encodeMovzReg(5, 73), encodeStrSp8(5), blPlaceholder)
	num, ok := BackwardScanStackTrap(code, 8) // BL at offset 8
	require.True(t, ok)
	assert.Equal(t, 73, num)
}

// TestBackwardScanStackTrap_STPPattern verifies MOVZ xN + STP xN,xM,[SP,#8] + BL → syscall number.
func TestBackwardScanStackTrap_STPPattern(t *testing.T) {
	t.Parallel()
	// MOVZ X5, #73; STP X5, X4, [SP, #8]; BL placeholder
	code := buildCodeSliceX0(encodeMovzReg(5, 73), encodeStpSp8(5, 4), blPlaceholder)
	num, ok := BackwardScanStackTrap(code, 8)
	require.True(t, ok)
	assert.Equal(t, 73, num)
}

// TestBackwardScanStackTrap_RegX0 verifies that regN=0 (X0) works correctly.
func TestBackwardScanStackTrap_RegX0(t *testing.T) {
	t.Parallel()
	// MOVZ X0, #197; STR X0, [SP, #8]; BL placeholder
	code := buildCodeSliceX0(encodeMovzX0(197), encodeStrSp8(0), blPlaceholder)
	num, ok := BackwardScanStackTrap(code, 8)
	require.True(t, ok)
	assert.Equal(t, 197, num)
}

// TestBackwardScanStackTrap_NoStore verifies that a BL with no preceding store
// to [SP, #8] returns (0, false).
func TestBackwardScanStackTrap_NoStore(t *testing.T) {
	t.Parallel()
	code := buildCodeSliceX0(encodeMovzReg(5, 73), blPlaceholder)
	_, ok := BackwardScanStackTrap(code, 4)
	assert.False(t, ok)
}

// TestBackwardScanStackTrap_IndirectRegWrite verifies that a non-immediate write
// to the trap register stops the scan.
func TestBackwardScanStackTrap_IndirectRegWrite(t *testing.T) {
	t.Parallel()
	// ADD X5, X1, X2 → non-immediate write to X5
	const addX5X1X2 = uint32(0x8B020025)
	code := buildCodeSliceX0(encodeMovzReg(5, 73), addX5X1X2, encodeStrSp8(5), blPlaceholder)
	_, ok := BackwardScanStackTrap(code, 12) // BL at offset 12
	assert.False(t, ok, "indirect write to trap register must stop the scan")
}

// svcImm80 is the ARM64 encoding of SVC #0x80 (macOS BSD syscall trap).
const svcImm80 = uint32(0xD4001001)

// encodeORRX16 builds an ORR X16, XZR, #imm instruction word from a raw encoding
// (caller supplies the exact 32-bit word; helper only exists for documentation).
// The actual instruction words used in tests are computed from the ARM64 bitmask spec.

// TestBackwardScanX16_ORRX16_Basic verifies that ORR X16, XZR, #1 (0xB24003F0)
// immediately before svc #0x80 resolves to syscall number 1.
func TestBackwardScanX16_ORRX16_Basic(t *testing.T) {
	t.Parallel()
	// ORR X16, XZR, #1 (N=1, immr=0, imms=0 → value=1); SVC #0x80
	const orrX16XZR1 = uint32(0xB24003F0)
	code := buildCodeSliceX0(orrX16XZR1, svcImm80)
	num, ok := BackwardScanX16(code, 4) // SVC at offset 4
	require.True(t, ok)
	assert.Equal(t, 1, num)
}

// TestBackwardScanX16_ORRX16_BSDPrefix verifies that when ORR loads a BSD-prefixed
// value the prefix is stripped before returning.
// Encoding: ORR X16, XZR, #0x3FFFFFF (N=1, immr=0, imms=25 → 26 consecutive ones).
// 0x3FFFFFF >= bsdSyscallClassPrefix(0x2000000), so result = 0x3FFFFFF - 0x2000000 = 0x1FFFFFF.
func TestBackwardScanX16_ORRX16_BSDPrefix(t *testing.T) {
	t.Parallel()
	// ORR X16, XZR, #0x3FFFFFF: N=1, immr=0, imms=25=0x19
	// word = 0xB2000000 | (1<<22) | (0<<16) | (0x19<<10) | (31<<5) | 16
	//      = 0xB2400000 | 0x6400 | 0x3E0 | 0x10 = 0xB24067F0
	const orrX16BSDPrefixed = uint32(0xB24067F0)
	code := buildCodeSliceX0(orrX16BSDPrefixed, svcImm80)
	num, ok := BackwardScanX16(code, 4)
	require.True(t, ok)
	assert.Equal(t, 0x1FFFFFF, num)
}

// TestDecodeORRX16XZR_N0 verifies correct decoding of a N=0 bitmask immediate
// by calling decodeORRX16XZR directly (bypassing the BSD-prefix strip in
// BackwardScanX16, which would change the result for large values).
// Encoding: ORR X16, XZR, #0x0F0F0F0F0F0F0F0F
//
//	N=0, esize=8, S=3 (4 ones), R=0, imms=0x33, immr=0
//	word = 0xB2000000 | (0x33<<10) | (31<<5) | 16 = 0xB200CFF0
func TestDecodeORRX16XZR_N0(t *testing.T) {
	t.Parallel()
	val, ok := decodeORRX16XZR(0xB200CFF0)
	require.True(t, ok)
	assert.Equal(t, int(uint64(0x0F0F0F0F0F0F0F0F)), val)
}

// TestBackwardScanX16_ORRX16_DoesNotStopScan verifies that an ORR X16 instruction
// that used to terminate the scan (via writesX16NotMovzMovk) now resolves correctly
// even when preceded by other instructions.
func TestBackwardScanX16_ORRX16_DoesNotStopScan(t *testing.T) {
	t.Parallel()
	// NOP-like instruction (MOV X3, X3 = ORR X3, XZR, X3 encoded differently)
	// Use a simple ADD X1, X1, #0 as a harmless filler (does not write X16).
	// ADD X1, X1, #0 = 0x91000021
	const nopFiller = uint32(0x91000021)
	const orrX16XZR1 = uint32(0xB24003F0) // ORR X16, XZR, #1
	code := buildCodeSliceX0(orrX16XZR1, nopFiller, svcImm80)
	num, ok := BackwardScanX16(code, 8)
	require.True(t, ok)
	assert.Equal(t, 1, num)
}
