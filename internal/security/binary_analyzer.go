package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/security/machoanalyzer"
)

// NewBinaryAnalyzer creates a BinaryAnalyzer appropriate for the specified OS.
// On macOS, returns StandardMachOAnalyzer; on Linux and other platforms, returns StandardELFAnalyzer.
func NewBinaryAnalyzer(goos string) binaryanalyzer.BinaryAnalyzer {
	switch RequireGOOS(goos) {
	case GosDarwin:
		return machoanalyzer.NewStandardMachOAnalyzer(nil)
	default: // "linux", etc.
		return elfanalyzer.NewStandardELFAnalyzer(nil)
	}
}
