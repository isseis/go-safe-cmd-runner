// Package elfmagic provides ELF magic number detection.
package elfmagic

const magicStr = "\x7fELF"

// Len is the length of the ELF magic number in bytes.
const Len = len(magicStr)

// Is reports whether b starts with the ELF magic number (\x7fELF).
func Is(b []byte) bool {
	return len(b) >= Len && b[0] == 0x7f && b[1] == 'E' && b[2] == 'L' && b[3] == 'F'
}
