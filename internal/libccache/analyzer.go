// Package libccache provides caching and matching of libc syscall wrapper functions.
package libccache

import (
	"debug/elf"
	"errors"
	"fmt"
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// MaxWrapperFunctionSize is the maximum size in bytes for a function to be
// considered a syscall wrapper.
const MaxWrapperFunctionSize = 256

// LibcWrapperAnalyzer analyzes a libc ELF file and returns a list of
// syscall wrapper functions.
type LibcWrapperAnalyzer struct {
	syscallAnalyzer *elfanalyzer.SyscallAnalyzer
}

// NewLibcWrapperAnalyzer creates a new LibcWrapperAnalyzer.
func NewLibcWrapperAnalyzer(syscallAnalyzer *elfanalyzer.SyscallAnalyzer) *LibcWrapperAnalyzer {
	return &LibcWrapperAnalyzer{syscallAnalyzer: syscallAnalyzer}
}

// symTextRange returns the [startOffset, endOffset) range within the code slice
// for the given symbol, validating all constraints. ok=false means skip this symbol.
func symTextRange(sym elf.Symbol, sectionBaseAddr uint64, codeLen int) (start, end int, ok bool) {
	// Exclude local / import / non-function symbols.
	if elf.ST_BIND(sym.Info) == elf.STB_LOCAL {
		return 0, 0, false
	}
	if sym.Section == elf.SHN_UNDEF {
		return 0, 0, false
	}
	if elf.ST_TYPE(sym.Info) != elf.STT_FUNC {
		return 0, 0, false
	}
	// Size filter.
	if sym.Size > MaxWrapperFunctionSize {
		return 0, 0, false
	}
	// Address-to-offset conversion.
	if sym.Value < sectionBaseAddr {
		return 0, 0, false
	}
	startOffset := int(sym.Value - sectionBaseAddr) //nolint:gosec // G115: safe, sym.Value >= sectionBaseAddr
	endOffset := startOffset + int(sym.Size)        //nolint:gosec // G115: sym.Size <= MaxWrapperFunctionSize, fits int
	if endOffset > codeLen {
		return 0, 0, false
	}
	return startOffset, endOffset, true
}

// validateInfos checks that all SyscallInfo entries have DeterminationMethod=="immediate",
// Number>=0, and all share the same Number. Returns (number, true) on success.
func validateInfos(infos []common.SyscallInfo) (int, bool) {
	if len(infos) == 0 {
		return 0, false
	}
	first := infos[0].Number
	for _, info := range infos {
		if info.DeterminationMethod != elfanalyzer.DeterminationMethodImmediate || info.Number < 0 {
			return 0, false
		}
		if info.Number != first {
			return 0, false
		}
	}
	return first, true
}

// Analyze scans the exported functions in libcELFFile and returns WrapperEntry
// values for functions that are recognized syscall wrappers.
// The returned slice is sorted by Number ascending, then Name ascending.
// Returns *elfanalyzer.UnsupportedArchitectureError (detectable via errors.As)
// if the ELF architecture is not supported.
func (a *LibcWrapperAnalyzer) Analyze(libcELFFile *elf.File) ([]WrapperEntry, error) {
	// Step 1: Get .text section data and base address.
	textSection := libcELFFile.Section(".text")
	if textSection == nil {
		return nil, nil
	}
	code, err := textSection.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read .text section: %w", err)
	}
	sectionBaseAddr := textSection.Addr

	// Step 2: Get exported symbols from .dynsym.
	syms, err := libcELFFile.DynamicSymbols()
	if err != nil {
		if errors.Is(err, elf.ErrNoSymbols) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to enumerate export symbols: %w", ErrExportSymbolsFailed)
	}

	var entries []WrapperEntry
	for _, sym := range syms {
		startOffset, endOffset, ok := symTextRange(sym, sectionBaseAddr, len(code))
		if !ok {
			continue
		}

		// Analyze syscalls in the function range.
		infos, err := a.syscallAnalyzer.AnalyzeSyscallsInRange(
			code, sectionBaseAddr, startOffset, endOffset, libcELFFile.Machine,
		)
		if err != nil {
			var archErr *elfanalyzer.UnsupportedArchitectureError
			if errors.As(err, &archErr) {
				return nil, err
			}
			continue
		}

		if number, ok := validateInfos(infos); ok {
			entries = append(entries, WrapperEntry{Name: sym.Name, Number: number})
		}
	}

	// Sort by Number ascending, then Name ascending.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Number != entries[j].Number {
			return entries[i].Number < entries[j].Number
		}
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}
