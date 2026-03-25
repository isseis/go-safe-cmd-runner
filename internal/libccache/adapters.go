package libccache

import (
	"debug/elf"
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
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
func (a *SyscallAdapter) AnalyzeSyscallsFromELF(elfFile *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, error) {
	result, err := a.analyzer.AnalyzeSyscallsFromELF(elfFile)
	if err != nil {
		if archErr, ok := errors.AsType[*elfanalyzer.UnsupportedArchitectureError](err); ok {
			return nil, nil, fmt.Errorf("%w: %v", filevalidator.ErrUnsupportedArch, archErr.Machine)
		}
		return nil, nil, err
	}
	if result == nil {
		return nil, nil, nil
	}
	return result.DetectedSyscalls, result.ArgEvalResults, nil
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
