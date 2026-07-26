//go:build test

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroupAndSortSyscalls(t *testing.T) {
	t.Run("empty slice returns nil", func(t *testing.T) {
		result := GroupAndSortSyscalls([]SyscallInfo{})
		assert.Nil(t, result)
	})

	t.Run("sorts by number ascending", func(t *testing.T) {
		input := []SyscallInfo{
			{Number: 257, Name: "openat"},
			{Number: 41, Name: "socket"},
			{Number: 1, Name: "write"},
			{Number: 42, Name: "connect"},
		}
		result := GroupAndSortSyscalls(input)
		assert.Len(t, result, 4)
		assert.Equal(t, 1, result[0].Number)
		assert.Equal(t, 41, result[1].Number)
		assert.Equal(t, 42, result[2].Number)
		assert.Equal(t, 257, result[3].Number)
	})

	t.Run("unknown number minus one placed last", func(t *testing.T) {
		input := []SyscallInfo{
			{Number: -1, Occurrences: []SyscallOccurrence{{Location: 0x402000, DeterminationMethod: "direct_svc_0x80"}}},
			{Number: 5, Name: "fstat", Occurrences: []SyscallOccurrence{{Location: 0x401000, DeterminationMethod: "immediate"}}},
		}
		result := GroupAndSortSyscalls(input)
		assert.Len(t, result, 2)
		assert.Equal(t, 5, result[0].Number)
		assert.Equal(t, -1, result[1].Number)
	})

	t.Run("merges same number entries and sorts occurrences by location", func(t *testing.T) {
		input := []SyscallInfo{
			{Number: 41, Name: "socket", Occurrences: []SyscallOccurrence{{Location: 0x401020, DeterminationMethod: "immediate"}}},
			{Number: 41, Name: "socket", Occurrences: []SyscallOccurrence{{Location: 0x401000, DeterminationMethod: "go_wrapper"}}},
		}
		result := GroupAndSortSyscalls(input)
		assert.Len(t, result, 1)
		assert.Equal(t, 41, result[0].Number)
		assert.Equal(t, "socket", result[0].Name)
		assert.Len(t, result[0].Occurrences, 2)
		assert.Equal(t, uint64(0x401000), result[0].Occurrences[0].Location)
		assert.Equal(t, uint64(0x401020), result[0].Occurrences[1].Location)
	})

	t.Run("non-empty Name from later entry is preserved when first entry has empty name", func(t *testing.T) {
		input := []SyscallInfo{
			{Number: 41, Name: "", Occurrences: []SyscallOccurrence{{Location: 0x401000, DeterminationMethod: "immediate"}}},
			{Number: 41, Name: "socket", Occurrences: []SyscallOccurrence{{Location: 0, DeterminationMethod: "immediate", Source: "libc_symbol_import"}}},
		}
		result := GroupAndSortSyscalls(input)
		assert.Len(t, result, 1)
		assert.Equal(t, "socket", result[0].Name)
	})
}
