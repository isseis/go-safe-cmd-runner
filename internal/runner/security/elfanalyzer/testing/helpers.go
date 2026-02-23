//go:build test

// Package elfanalyzertesting provides test helpers for the elfanalyzer package.
package elfanalyzertesting

import (
	"debug/elf"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateStaticELFFile creates a minimal static ELF file at the given path for testing.
// The file has no .dynsym section, simulating a statically linked binary.
func CreateStaticELFFile(t *testing.T, path string) {
	t.Helper()

	// Create a minimal ELF header for x86_64
	// This is a valid ELF header that will parse but has no .dynsym section
	elfHeader := []byte{
		// ELF magic
		0x7f, 'E', 'L', 'F',
		// Class: 64-bit
		0x02,
		// Data: little endian
		0x01,
		// Version
		0x01,
		// OS/ABI: System V
		0x00,
		// ABI version
		0x00,
		// Padding
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Type: Executable
		0x02, 0x00,
		// Machine: x86_64
		0x3e, 0x00,
		// Version
		0x01, 0x00, 0x00, 0x00,
		// Entry point
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Program header offset
		0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Section header offset (0 = none)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Flags
		0x00, 0x00, 0x00, 0x00,
		// ELF header size
		0x40, 0x00,
		// Program header size
		0x38, 0x00,
		// Number of program headers
		0x00, 0x00,
		// Section header size
		0x40, 0x00,
		// Number of section headers
		0x00, 0x00,
		// Section name string table index
		0x00, 0x00,
	}

	err := os.WriteFile(path, elfHeader, 0o644) //nolint:gosec // test helper: 0644 is intentional for test files
	require.NoError(t, err)

	// Verify it can be parsed as ELF
	f, err := os.Open(path) //nolint:gosec // test helper: path is provided by the test
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	_, err = elf.NewFile(f)
	require.NoError(t, err)
}
