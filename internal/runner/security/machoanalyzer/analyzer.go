// Package machoanalyzer implements BinaryAnalyzer for Mach-O binaries (macOS/arm64).
// It uses Go's standard debug/macho package to inspect imported symbols
// and detect network-related function calls.
package machoanalyzer

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// StandardMachOAnalyzer implements elfanalyzer.BinaryAnalyzer for Mach-O binaries.
// It analyzes imported symbols and scans for svc #0x80 instructions (arm64 only).
type StandardMachOAnalyzer struct {
	fs             safefileio.FileSystem
	networkSymbols map[string]elfanalyzer.SymbolCategory
}

// NewStandardMachOAnalyzer creates a new StandardMachOAnalyzer.
// If fs is nil, safefileio.NewFileSystem(safefileio.FileSystemConfig{}) is used as the default.
// networkSymbols is obtained from elfanalyzer.GetNetworkSymbols().
func NewStandardMachOAnalyzer(fs safefileio.FileSystem) *StandardMachOAnalyzer {
	if fs == nil {
		fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	}
	return &StandardMachOAnalyzer{
		fs:             fs,
		networkSymbols: elfanalyzer.GetNetworkSymbols(),
	}
}
