//go:build test

package security

import (
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
)

// newNetworkAnalyzer creates a new NetworkAnalyzer.
// The caller must inject the target GOOS.
func newNetworkAnalyzer(goos string) *NetworkAnalyzer {
	return &NetworkAnalyzer{goos: isec.RequireGOOS(goos)}
}

// newNetworkAnalyzerWithStore creates a NetworkAnalyzer with a RecordStore for record-based analysis.
func newNetworkAnalyzerWithStore(goos string, store RecordStore) *NetworkAnalyzer {
	return &NetworkAnalyzer{goos: isec.RequireGOOS(goos), deps: AnalysisDeps{RecordStore: store}}
}
