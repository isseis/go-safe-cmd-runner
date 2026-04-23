//go:build test

package machoanalyzer

import "encoding/binary"

// svcEncoding is the little-endian encoding of "svc #0x80" for arm64.
const svcEncoding = uint32(0xD4001001)

// buildCodeSlice assembles a sequence of 32-bit ARM64 instructions into a byte slice.
func buildCodeSlice(instrs ...uint32) []byte {
	buf := make([]byte, len(instrs)*4)
	for i, instr := range instrs {
		binary.LittleEndian.PutUint32(buf[i*4:], instr)
	}
	return buf
}
