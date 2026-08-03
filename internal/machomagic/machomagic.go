// Package machomagic provides Mach-O magic number detection.
package machomagic

import (
	"debug/macho"
	"encoding/binary"
)

// Len is the length of the Mach-O magic number in bytes.
const Len = 4

// Byte-swapped magic values not exported by debug/macho
// (see <mach-o/loader.h> MH_CIGAM_64 / MH_CIGAM / FAT_CIGAM).
const (
	magicCigam64  = 0xcffaedfe // byte-swapped Magic64
	magicCigam32  = 0xcefaedfe // byte-swapped Magic32
	magicFatCigam = 0xbebafeca // byte-swapped MagicFat
)

// Is reports whether b starts with any recognized Mach-O or Fat binary magic
// value (32-bit, 64-bit, or Fat, in either byte order).
func Is(b []byte) bool {
	if len(b) < Len {
		return false
	}
	m := binary.LittleEndian.Uint32(b[:Len])
	switch m {
	case macho.Magic32, macho.Magic64, macho.MagicFat,
		magicCigam32, magicCigam64, magicFatCigam:
		return true
	}
	return false
}

// IsFat reports whether b starts with a Fat binary magic value (either byte order).
func IsFat(b []byte) bool {
	if len(b) < Len {
		return false
	}
	m := binary.LittleEndian.Uint32(b[:Len])
	return m == macho.MagicFat || m == magicFatCigam
}
