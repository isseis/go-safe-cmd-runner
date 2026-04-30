//go:build test

package security

import (
	"runtime"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
)

// newNetworkAnalyzer creates a NetworkAnalyzer with a store for testing.
// Pass nil for store to disable cache-based analysis.
// This function is only available in test builds.
func newNetworkAnalyzer(
	store fileanalysis.NetworkSymbolStore,
) *NetworkAnalyzer {
	return NewNetworkAnalyzerWithStore(runtime.GOOS, store)
}
