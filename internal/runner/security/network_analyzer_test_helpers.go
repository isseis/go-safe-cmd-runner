//go:build test

package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// NewNetworkAnalyzerWithBinaryAnalyzer creates a new NetworkAnalyzer with a custom BinaryAnalyzer.
// This function is only available in test builds.
func NewNetworkAnalyzerWithBinaryAnalyzer(analyzer elfanalyzer.BinaryAnalyzer) *NetworkAnalyzer {
	return &NetworkAnalyzer{binaryAnalyzer: analyzer}
}
