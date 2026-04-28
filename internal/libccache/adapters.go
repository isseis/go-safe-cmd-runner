package libccache

import (
	"debug/elf"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// CacheAdapter implements filevalidator.LibcCacheInterface by combining
// LibcCacheManager (cache read/write) and SyscallAnalyzer (syscall table
// lookup + import symbol matching).
// It converts elfanalyzer.UnsupportedArchitectureError to filevalidator.ErrUnsupportedArch.
type CacheAdapter struct {
	cacheMgr        *LibcCacheManager
	syscallAnalyzer *elfanalyzer.SyscallAnalyzer
}

// NewCacheAdapter creates a CacheAdapter that implements filevalidator.LibcCacheInterface.
func NewCacheAdapter(cacheMgr *LibcCacheManager, syscallAnalyzer *elfanalyzer.SyscallAnalyzer) *CacheAdapter {
	return &CacheAdapter{cacheMgr: cacheMgr, syscallAnalyzer: syscallAnalyzer}
}

// GetOrCreateSyscalls implements filevalidator.LibcCacheInterface.
func (a *CacheAdapter) GetOrCreateSyscalls(libcPath, libcHash string, importSymbols []string, machine elf.Machine) ([]common.SyscallInfo, error) {
	wrappers, err := a.cacheMgr.GetOrCreate(libcPath, libcHash)
	if err != nil {
		if archErr, ok := errors.AsType[*elfanalyzer.UnsupportedArchitectureError](err); ok {
			return nil, fmt.Errorf("%w: %v", filevalidator.ErrUnsupportedArch, archErr.Machine)
		}
		return nil, err
	}

	syscallTable, ok := a.syscallAnalyzer.GetSyscallTable(machine)
	if !ok {
		return nil, fmt.Errorf("%w: machine %v", filevalidator.ErrUnsupportedArch, machine)
	}

	matcher := NewImportSymbolMatcher(syscallTable)
	return matcher.Match(importSymbols, wrappers), nil
}

// SyscallAdapter implements filevalidator.SyscallAnalyzerInterface.
// It wraps *elfanalyzer.SyscallAnalyzer and converts UnsupportedArchitectureError
// to filevalidator.ErrUnsupportedArch.
type SyscallAdapter struct {
	analyzer *elfanalyzer.SyscallAnalyzer
}

// NewSyscallAdapter creates a SyscallAdapter that implements filevalidator.SyscallAnalyzerInterface.
func NewSyscallAdapter(analyzer *elfanalyzer.SyscallAnalyzer) *SyscallAdapter {
	return &SyscallAdapter{analyzer: analyzer}
}

// AnalyzeSyscallsFromELF implements filevalidator.SyscallAnalyzerInterface.
func (a *SyscallAdapter) AnalyzeSyscallsFromELF(elfFile *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
	result, err := a.analyzer.AnalyzeSyscallsFromELF(elfFile)
	if err != nil {
		if archErr, ok := errors.AsType[*elfanalyzer.UnsupportedArchitectureError](err); ok {
			return nil, nil, nil, fmt.Errorf("%w: %v", filevalidator.ErrUnsupportedArch, archErr.Machine)
		}
		return nil, nil, nil, err
	}
	if result == nil {
		return nil, nil, nil, nil
	}
	return result.DetectedSyscalls, result.ArgEvalResults, result.DeterminationStats, nil
}

// EvaluatePLTCallArgs implements filevalidator.SyscallAnalyzerInterface.
func (a *SyscallAdapter) EvaluatePLTCallArgs(elfFile *elf.File, funcName string) (*common.SyscallArgEvalResult, error) {
	result, err := a.analyzer.EvaluatePLTCallArgs(elfFile, funcName)
	if err != nil {
		if archErr, ok := errors.AsType[*elfanalyzer.UnsupportedArchitectureError](err); ok {
			return nil, fmt.Errorf("%w: %v", filevalidator.ErrUnsupportedArch, archErr.Machine)
		}
		return nil, err
	}
	return result, nil
}

// GetSyscallTable implements filevalidator.SyscallAnalyzerInterface.
func (a *SyscallAdapter) GetSyscallTable(machine elf.Machine) (filevalidator.SyscallNumberTable, bool) {
	return a.analyzer.GetSyscallTable(machine)
}

// MachoLibSystemAdapter implements filevalidator.LibSystemCacheInterface
// by combining MachoLibSystemCacheManager and ImportSymbolMatcher.
//
//nolint:revive // MachoLibSystemAdapter is intentional: callers import as libccache.MachoLibSystemAdapter
type MachoLibSystemAdapter struct {
	cacheMgr     *MachoLibSystemCacheManager
	fs           safefileio.FileSystem
	syscallTable SyscallNumberTable
	resolveFunc  func([]fileanalysis.LibEntry, safefileio.FileSystem, bool) (*machodylib.LibSystemKernelSource, error)
}

// NewMachoLibSystemAdapter creates a new MachoLibSystemAdapter.
func NewMachoLibSystemAdapter(
	cacheMgr *MachoLibSystemCacheManager,
	fs safefileio.FileSystem,
) *MachoLibSystemAdapter {
	return &MachoLibSystemAdapter{
		cacheMgr:     cacheMgr,
		fs:           fs,
		syscallTable: MacOSSyscallTable{},
		resolveFunc:  machodylib.ResolveLibSystemKernel,
	}
}

// GetSyscallInfos resolves libsystem_kernel.dylib source from dynLibDeps,
// gets/creates the wrapper cache, matches importSymbols against the cache,
// and returns detected SyscallInfo entries.
//
// hasLibSystemLoadCmd must be true when the binary has an LC_LOAD_DYLIB entry
// naming a libSystem-family library. On macOS 11+ those libraries live only in
// the dyld shared cache and therefore do not appear in dynLibDeps; setting this
// flag allows the resolver to proceed to dyld cache extraction (Step 3).
//
// When libsystem_kernel.dylib cannot be resolved (no libSystem-family library in
// either dynLibDeps or load commands, dyld cache extraction failed, or the
// resolved dylib has no usable __TEXT/__SYMTAB), the method falls back to
// matching import symbol names against the known network syscall wrapper list.
func (a *MachoLibSystemAdapter) GetSyscallInfos(
	dynLibDeps []fileanalysis.LibEntry,
	importSymbols []string,
	hasLibSystemLoadCmd bool,
) ([]common.SyscallInfo, error) {
	source, err := a.resolveFunc(dynLibDeps, a.fs, hasLibSystemLoadCmd)
	if err != nil {
		return nil, err
	}

	if source == nil {
		reason := classifyLibSystemFallbackReason(dynLibDeps, hasLibSystemLoadCmd)

		if reason == "missing_libsystem_dependency" {
			// The binary has no libSystem dependency at all: skip detection entirely
			// to avoid false positives from symbol-name matching.
			return nil, nil
		}

		// libSystem source not resolved; match import names against known network syscall wrappers.
		slog.Info("libSystem cache unavailable; falling back to symbol-name matching",
			"reason", reason)
		result := a.fallbackNameMatch(importSymbols)
		slog.Info("libSystem fallback matching completed",
			"reason", reason,
			"detected_syscalls", len(result))
		return result, nil
	}

	// Load or create the cache.
	wrappers, err := a.cacheMgr.GetOrCreate(source.Path, source.Hash, source.GetData)
	if err != nil {
		// The resolved file could not be read or parsed (e.g., it is a linker stub
		// rather than a standalone Mach-O); fall back to symbol-name matching.
		slog.Info("libSystem cache unavailable after source resolution; falling back to symbol-name matching",
			"source_path", source.Path, "error", err)
		result := a.fallbackNameMatch(importSymbols)
		slog.Info("libSystem fallback matching completed",
			"reason", "cache_error",
			"detected_syscalls", len(result))
		return result, nil
	}

	// Empty wrapper list means the resolved dylib has no usable __TEXT/__SYMTAB
	// (e.g., a linker stub or non-arm64 binary); fall back to symbol-name matching
	// to avoid false negatives in network detection.
	if len(wrappers) == 0 {
		slog.Info("libSystem cache contains no wrappers; falling back to symbol-name matching",
			"source_path", source.Path)
		result := a.fallbackNameMatch(importSymbols)
		slog.Info("libSystem fallback matching completed",
			"reason", "empty_wrapper_cache",
			"detected_syscalls", len(result))
		return result, nil
	}

	// Match imported symbols against the cache.
	matcher := NewImportSymbolMatcher(a.syscallTable)
	return matcher.MatchWithMethod(importSymbols, wrappers, DeterminationMethodLibCacheMatch), nil
}

// classifyLibSystemFallbackReason returns a string describing why the libSystem cache
// lookup fell back to symbol-name matching.
// If neither DynLibDeps nor the binary's load commands contain a libSystem entry,
// the reason is "missing_libsystem_dependency". Otherwise the resolver already
// attempted filesystem and dyld cache resolution and the reason is
// "dyld_extraction_unavailable".
func classifyLibSystemFallbackReason(dynLibDeps []fileanalysis.LibEntry, hasLibSystemLoadCmd bool) string {
	const (
		umbrellaInstallName = "/usr/lib/libSystem.B.dylib"
		kernelBaseName      = "libsystem_kernel.dylib"
	)

	if hasLibSystemLoadCmd {
		return "dyld_extraction_unavailable"
	}

	for _, entry := range dynLibDeps {
		if entry.SOName == umbrellaInstallName || filepath.Base(entry.SOName) == kernelBaseName {
			return "dyld_extraction_unavailable"
		}
	}
	return "missing_libsystem_dependency"
}

// fallbackNameMatch matches importSymbols against the macOS network-related syscall wrapper list
// and returns the resulting SyscallInfo entries.
func (a *MachoLibSystemAdapter) fallbackNameMatch(importSymbols []string) []common.SyscallInfo {
	// Build a set of imported symbols.
	symSet := make(map[string]bool, len(importSymbols))
	for _, s := range importSymbols {
		symSet[s] = true
	}

	var result []common.SyscallInfo
	for _, name := range networkSyscallWrapperNames {
		if !symSet[name] {
			continue
		}
		// Reverse-lookup the syscall number from the macOS syscall table.
		number := -1
		for num, entry := range macOSSyscallEntries {
			if entry.name == name {
				number = num
				break
			}
		}
		if number < 0 {
			continue
		}
		result = append(result, common.SyscallInfo{
			Number: number,
			Name:   name,
			Occurrences: []common.SyscallOccurrence{
				{
					Location:            0,
					DeterminationMethod: DeterminationMethodSymbolNameMatch,
					Source:              SourceLibsystemSymbolImport,
				},
			},
		})
	}

	// Sort by Number.
	sort.Slice(result, func(i, j int) bool { return result[i].Number < result[j].Number })
	return result
}
