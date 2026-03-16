package main

import (
	"debug/elf"
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/libccache"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// libcCacheAdapter implements filevalidator.LibcCacheInterface by combining
// LibcCacheManager (cache read/write), LibcWrapperAnalyzer (libc analysis),
// and SyscallAnalyzer (syscall table lookup + import symbol matching).
// It also converts elfanalyzer.UnsupportedArchitectureError to filevalidator.ErrUnsupportedArch.
type libcCacheAdapter struct {
	cacheMgr        *libccache.LibcCacheManager
	syscallAnalyzer *elfanalyzer.SyscallAnalyzer
}

// GetOrCreateSyscalls implements filevalidator.LibcCacheInterface.
func (a *libcCacheAdapter) GetOrCreateSyscalls(libcPath, libcHash string, importSymbols []string, machine elf.Machine) ([]common.SyscallInfo, error) {
	wrappers, err := a.cacheMgr.GetOrCreate(libcPath, libcHash)
	if err != nil {
		var archErr *elfanalyzer.UnsupportedArchitectureError
		if errors.As(err, &archErr) {
			return nil, fmt.Errorf("%w: %v", filevalidator.ErrUnsupportedArch, archErr.Machine)
		}
		return nil, err
	}

	syscallTable, ok := a.syscallAnalyzer.GetSyscallTable(machine)
	if !ok {
		return nil, fmt.Errorf("%w: machine %v", filevalidator.ErrUnsupportedArch, machine)
	}

	matcher := libccache.NewImportSymbolMatcher(syscallTable)
	return matcher.Match(importSymbols, wrappers), nil
}

// syscallAnalyzerAdapter implements filevalidator.SyscallAnalyzerInterface.
// It wraps *elfanalyzer.SyscallAnalyzer and converts UnsupportedArchitectureError
// to filevalidator.ErrUnsupportedArch.
type syscallAnalyzerAdapter struct {
	analyzer *elfanalyzer.SyscallAnalyzer
}

// AnalyzeSyscallsFromELF implements filevalidator.SyscallAnalyzerInterface.
func (a *syscallAnalyzerAdapter) AnalyzeSyscallsFromELF(elfFile *elf.File) ([]common.SyscallInfo, error) {
	result, err := a.analyzer.AnalyzeSyscallsFromELF(elfFile)
	if err != nil {
		var archErr *elfanalyzer.UnsupportedArchitectureError
		if errors.As(err, &archErr) {
			return nil, fmt.Errorf("%w: %v", filevalidator.ErrUnsupportedArch, archErr.Machine)
		}
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.DetectedSyscalls, nil
}

// GetSyscallTable implements filevalidator.SyscallAnalyzerInterface.
func (a *syscallAnalyzerAdapter) GetSyscallTable(machine elf.Machine) (filevalidator.SyscallNumberTable, bool) {
	return a.analyzer.GetSyscallTable(machine)
}
