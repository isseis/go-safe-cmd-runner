//go:build test

package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
)

// newNetworkAnalyzer creates a NetworkAnalyzer with a custom BinaryAnalyzer and store for testing.
// Pass nil for store to disable cache-based analysis.
// This function is only available in test builds.
func newNetworkAnalyzer(
	analyzer binaryanalyzer.BinaryAnalyzer,
	store fileanalysis.NetworkSymbolStore,
) *NetworkAnalyzer {
	return &NetworkAnalyzer{binaryAnalyzer: analyzer, store: store}
}
