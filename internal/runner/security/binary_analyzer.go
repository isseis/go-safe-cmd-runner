package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/machoanalyzer"
)

// gosDarwin is the GOOS value for macOS.
const gosDarwin = "darwin"

func requireGOOS(goos string) string {
	if goos == "" {
		panic("goos must not be empty")
	}
	return goos
}

// NewBinaryAnalyzer creates a BinaryAnalyzer appropriate for the specified OS.
// On macOS, returns StandardMachOAnalyzer; on Linux and other platforms, returns StandardELFAnalyzer.
func NewBinaryAnalyzer(goos string) binaryanalyzer.BinaryAnalyzer {
	switch requireGOOS(goos) {
	case gosDarwin:
		return machoanalyzer.NewStandardMachOAnalyzer(nil)
	default: // "linux", etc.
		return elfanalyzer.NewStandardELFAnalyzer(nil, nil)
	}
}
