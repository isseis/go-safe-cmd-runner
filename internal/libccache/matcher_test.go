//go:build test

package libccache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSyscallTable is a minimal SyscallNumberTable for use in matcher tests.
type stubSyscallTable struct {
	names    map[int]string
	networks map[int]bool
}

func (s *stubSyscallTable) GetSyscallName(number int) string {
	return s.names[number]
}

func (s *stubSyscallTable) IsNetworkSyscall(number int) bool {
	return s.networks[number]
}

func newStubTable() *stubSyscallTable {
	return &stubSyscallTable{
		names: map[int]string{
			1:   "write",
			41:  "socket",
			231: "exit_group",
		},
		networks: map[int]bool{
			41: true,
		},
	}
}

func newMatcher() *ImportSymbolMatcher {
	return NewImportSymbolMatcher(newStubTable())
}

// TestImportSymbolMatcher_MatchFound verifies that a matching import symbol produces a SyscallInfo.
func TestImportSymbolMatcher_MatchFound(t *testing.T) {
	m := newMatcher()
	wrappers := []WrapperEntry{
		{Name: "write", Number: 1},
	}
	result := m.Match([]string{"write"}, wrappers)
	require.Len(t, result, 1)
	assert.Equal(t, 1, result[0].Number)
}

// TestImportSymbolMatcher_MatchNotFound verifies that unrecognized import symbols are ignored.
func TestImportSymbolMatcher_MatchNotFound(t *testing.T) {
	m := newMatcher()
	wrappers := []WrapperEntry{
		{Name: "write", Number: 1},
	}
	result := m.Match([]string{"unknown_symbol"}, wrappers)
	assert.Empty(t, result)
}

// TestImportSymbolMatcher_SourceIsLibcSymbolImport verifies the Source field.
func TestImportSymbolMatcher_SourceIsLibcSymbolImport(t *testing.T) {
	m := newMatcher()
	wrappers := []WrapperEntry{{Name: "write", Number: 1}}
	result := m.Match([]string{"write"}, wrappers)
	require.Len(t, result, 1)
	assert.Equal(t, SourceLibcSymbolImport, result[0].Occurrences[0].Source)
}

// TestImportSymbolMatcher_LocationIsZero verifies that Location is always 0.
func TestImportSymbolMatcher_LocationIsZero(t *testing.T) {
	m := newMatcher()
	wrappers := []WrapperEntry{{Name: "write", Number: 1}}
	result := m.Match([]string{"write"}, wrappers)
	require.Len(t, result, 1)
	assert.Equal(t, uint64(0), result[0].Occurrences[0].Location)
}

// TestImportSymbolMatcher_DeterminationMethodIsImmediate verifies DeterminationMethod.
func TestImportSymbolMatcher_DeterminationMethodIsImmediate(t *testing.T) {
	m := newMatcher()
	wrappers := []WrapperEntry{{Name: "write", Number: 1}}
	result := m.Match([]string{"write"}, wrappers)
	require.Len(t, result, 1)
	assert.Equal(t, "immediate", result[0].Occurrences[0].DeterminationMethod)
}

// TestImportSymbolMatcher_NoDuplicateNumbers verifies that wrappers with the same Number
// produce only one SyscallInfo entry.
func TestImportSymbolMatcher_NoDuplicateNumbers(t *testing.T) {
	m := newMatcher()
	wrappers := []WrapperEntry{
		{Name: "open", Number: 2},
		{Name: "openat", Number: 2},
	}
	result := m.Match([]string{"open", "openat"}, wrappers)
	assert.Len(t, result, 1)
}

// TestImportSymbolMatcher_DedupPicksLexicographicallySmallestName verifies that when multiple
// import symbols map to the same Number, the one with the lexicographically smallest Name wins.
func TestImportSymbolMatcher_DedupPicksLexicographicallySmallestName(t *testing.T) {
	// Use a table that has no entry for number 2, so Name will fall back to the wrapper name.
	table := &stubSyscallTable{names: map[int]string{}, networks: map[int]bool{}}
	m := NewImportSymbolMatcher(table)
	wrappers := []WrapperEntry{
		{Name: "open", Number: 2},
		{Name: "creat", Number: 2},
		{Name: "openat", Number: 2},
	}
	result := m.Match([]string{"open", "creat", "openat"}, wrappers)
	require.Len(t, result, 1)
	// "creat" < "open" < "openat" lexicographically.
	assert.Equal(t, "creat", result[0].Name)
}

// TestImportSymbolMatcher_MatchedEntryHasCorrectNumber verifies that a matched wrapper
// entry carries the expected syscall number.
func TestImportSymbolMatcher_MatchedEntryHasCorrectNumber(t *testing.T) {
	m := newMatcher()
	wrappers := []WrapperEntry{{Name: "socket", Number: 41}}
	result := m.Match([]string{"socket"}, wrappers)
	require.Len(t, result, 1)
	assert.Equal(t, 41, result[0].Number)
}

// TestImportSymbolMatcher_NumberIsNonNegative verifies the invariant that all returned entries
// have Number >= 0. convertSyscallResult in the runner relies on Number == -1 to detect
// unknown syscalls; libc_symbol_import entries must never produce Number == -1.
func TestImportSymbolMatcher_NumberIsNonNegative(t *testing.T) {
	m := newMatcher()
	wrappers := []WrapperEntry{
		{Name: "write", Number: 1},
		{Name: "socket", Number: 41},
		{Name: "exit_group", Number: 231},
	}
	result := m.Match([]string{"write", "socket", "exit_group"}, wrappers)
	for _, info := range result {
		assert.GreaterOrEqual(t, info.Number, 0,
			"libc_symbol_import entry %q must have Number >= 0", info.Name)
	}
}

// TestImportSymbolMatcher_MultipleSymbols verifies that multiple distinct symbols are all matched
// and that the result is sorted by Number ascending.
func TestImportSymbolMatcher_MultipleSymbols(t *testing.T) {
	m := newMatcher()
	wrappers := []WrapperEntry{
		{Name: "write", Number: 1},
		{Name: "socket", Number: 41},
		{Name: "exit_group", Number: 231},
	}
	// Feed symbols in reverse order to confirm sorting is applied regardless of input order.
	result := m.Match([]string{"exit_group", "socket", "write"}, wrappers)
	require.Len(t, result, 3)
	assert.Equal(t, 1, result[0].Number)
	assert.Equal(t, 41, result[1].Number)
	assert.Equal(t, 231, result[2].Number)
}
