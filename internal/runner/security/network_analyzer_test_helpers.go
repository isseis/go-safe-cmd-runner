//go:build test

package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// NewNetworkAnalyzerWithELFAnalyzer creates a new NetworkAnalyzer with a custom ELFAnalyzer.
// This function is only available in test builds.
func NewNetworkAnalyzerWithELFAnalyzer(elfAnalyzer elfanalyzer.ELFAnalyzer) *NetworkAnalyzer {
	return &NetworkAnalyzer{elfAnalyzer: elfAnalyzer}
}
