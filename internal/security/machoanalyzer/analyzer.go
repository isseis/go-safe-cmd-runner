// Package machoanalyzer implements BinaryAnalyzer for Mach-O binaries.
// It supports single-arch Mach-O files (any architecture) and Fat binaries
// (all slices are analyzed). Symbol-based network detection works for all
// architectures; the svc #0x80 direct-syscall scan is arm64-only.
package machoanalyzer

import (
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
)

// StandardMachOAnalyzer implements binaryanalyzer.BinaryAnalyzer for Mach-O binaries.
// For Fat binaries every slice is analyzed; a threat in any slice is reported.
// Imported-symbol detection works for all architectures; the svc #0x80 scan
// is arm64-only (skipped silently for other architectures).
type StandardMachOAnalyzer struct {
	fs             safefileio.FileSystem
	networkSymbols map[string]binaryanalyzer.SymbolCategory
}

// NewStandardMachOAnalyzer creates a new StandardMachOAnalyzer.
// If fs is nil, safefileio.NewFileSystem(safefileio.FileSystemConfig{}) is used as the default.
// networkSymbols is obtained from binaryanalyzer.GetNetworkSymbols().
func NewStandardMachOAnalyzer(fs safefileio.FileSystem) *StandardMachOAnalyzer {
	if fs == nil {
		fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	}
	return &StandardMachOAnalyzer{
		fs:             fs,
		networkSymbols: binaryanalyzer.GetNetworkSymbols(),
	}
}
