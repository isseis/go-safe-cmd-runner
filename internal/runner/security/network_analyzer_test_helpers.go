//go:build test

package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
)

// newNetworkAnalyzer creates a new NetworkAnalyzer.
// The caller must inject the target GOOS.
func newNetworkAnalyzer(goos string) *NetworkAnalyzer {
	return &NetworkAnalyzer{goos: isec.RequireGOOS(goos)}
}

// newNetworkAnalyzerWithStore creates a NetworkAnalyzer with a store for cache-based analysis.
func newNetworkAnalyzerWithStore(goos string, store fileanalysis.NetworkSymbolStore) *NetworkAnalyzer {
	return &NetworkAnalyzer{goos: isec.RequireGOOS(goos), store: store}
}
