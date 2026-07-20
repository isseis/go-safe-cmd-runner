// Package elfmagic provides ELF magic number detection.
package elfmagic

import "bytes"

const magicStr = "\x7fELF"

var magic = []byte(magicStr)

// Len is the length of the ELF magic number in bytes.
const Len = len(magicStr)

// Is reports whether b starts with the ELF magic number (\x7fELF).
func Is(b []byte) bool {
	if len(b) < Len {
		return false
	}
	return bytes.Equal(b[:Len], magic)
}
