package libccache

import (
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// SyscallNumberTable provides syscall name and network classification by number.
// This interface mirrors elfanalyzer.SyscallNumberTable.
type SyscallNumberTable interface {
	GetSyscallName(number int) string
	IsNetworkSyscall(number int) bool
}

// ImportSymbolMatcher matches import symbols of a target binary against a libc wrapper cache.
type ImportSymbolMatcher struct {
	syscallTable SyscallNumberTable
}

// NewImportSymbolMatcher creates a new ImportSymbolMatcher with the given syscall table.
func NewImportSymbolMatcher(syscallTable SyscallNumberTable) *ImportSymbolMatcher {
	return &ImportSymbolMatcher{syscallTable: syscallTable}
}

// Match returns SyscallInfo entries for each import symbol that maps to a known libc wrapper.
// When multiple wrapper entries share the same syscall Number, the entry whose Name is
// lexicographically smallest is kept (stable dedup).
//
// Invariant: every returned entry has Number >= 0, because WrapperEntry.Number >= 0 is
// enforced by validateInfos at cache-build time and Number is copied directly from the wrapper.
func (m *ImportSymbolMatcher) Match(importSymbols []string, wrappers []WrapperEntry) []common.SyscallInfo {
	// Build a name→WrapperEntry map.
	wrapperMap := make(map[string]WrapperEntry, len(wrappers))
	for _, w := range wrappers {
		wrapperMap[w.Name] = w
	}

	// candidate holds the winning WrapperEntry for each syscall Number seen so far.
	candidate := make(map[int]WrapperEntry)

	for _, sym := range importSymbols {
		w, ok := wrapperMap[sym]
		if !ok {
			continue
		}
		prev, seen := candidate[w.Number]
		if !seen || w.Name < prev.Name {
			candidate[w.Number] = w
		}
	}

	result := make([]common.SyscallInfo, 0, len(candidate))
	for _, w := range candidate {
		info := common.SyscallInfo{
			Number:    w.Number,
			Name:      m.syscallTable.GetSyscallName(w.Number),
			IsNetwork: m.syscallTable.IsNetworkSyscall(w.Number),
			Occurrences: []common.SyscallOccurrence{
				{
					Location:            0,
					DeterminationMethod: elfanalyzer.DeterminationMethodImmediate,
					Source:              SourceLibcSymbolImport,
				},
			},
		}
		if info.Name == "" {
			info.Name = w.Name
		}
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Number < result[j].Number })

	return result
}

// MatchWithMethod is equivalent to Match, but allows the caller to specify
// the DeterminationMethod explicitly.
// It is used by the Mach-O path to set "lib_cache_match" on cache hits and
// "symbol_name_match" on fallback.
func (m *ImportSymbolMatcher) MatchWithMethod(
	importSymbols []string,
	wrappers []WrapperEntry,
	determinationMethod string,
) []common.SyscallInfo {
	// Build a symbol-name to WrapperEntry map.
	wrapperMap := make(map[string]WrapperEntry, len(wrappers))
	for _, w := range wrappers {
		wrapperMap[w.Name] = w
	}

	// Deduplicate by Number, keeping the lexicographically smallest symbol name.
	candidate := make(map[int]WrapperEntry)
	for _, sym := range importSymbols {
		w, ok := wrapperMap[sym]
		if !ok {
			continue
		}
		prev, seen := candidate[w.Number]
		if !seen || w.Name < prev.Name {
			candidate[w.Number] = w
		}
	}

	result := make([]common.SyscallInfo, 0, len(candidate))
	for _, w := range candidate {
		info := common.SyscallInfo{
			Number:    w.Number,
			Name:      m.syscallTable.GetSyscallName(w.Number),
			IsNetwork: m.syscallTable.IsNetworkSyscall(w.Number),
			Occurrences: []common.SyscallOccurrence{
				{
					Location:            0,
					DeterminationMethod: determinationMethod,
					Source:              SourceLibsystemSymbolImport,
				},
			},
		}
		if info.Name == "" {
			info.Name = w.Name
		}
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Number < result[j].Number })

	return result
}
