package main

import (
	"github.com/isseis/go-safe-cmd-runner/internal/libccache"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// newLibcCacheAdapter creates a filevalidator.LibcCacheInterface backed by
// LibcCacheManager and SyscallAnalyzer using the shared libccache adapter.
func newLibcCacheAdapter(cacheMgr *libccache.LibcCacheManager, syscallAnalyzer *elfanalyzer.SyscallAnalyzer) *libccache.CacheAdapter {
	return libccache.NewCacheAdapter(cacheMgr, syscallAnalyzer)
}

// newSyscallAnalyzerAdapter creates a filevalidator.SyscallAnalyzerInterface backed by
// SyscallAnalyzer using the shared libccache adapter.
func newSyscallAnalyzerAdapter(analyzer *elfanalyzer.SyscallAnalyzer) *libccache.SyscallAdapter {
	return libccache.NewSyscallAdapter(analyzer)
}
