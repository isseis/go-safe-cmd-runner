//go:build test

package security

import (
	"slices"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
)

// newNetworkAnalyzerWithStore creates a NetworkAnalyzer with a RecordStore for record-based analysis.
func newNetworkAnalyzerWithStore(goos string, store RecordStore) *NetworkAnalyzer {
	return &NetworkAnalyzer{goos: isec.RequireGOOS(goos), deps: AnalysisDeps{RecordStore: store}}
}

// legacyBoolsFromResult maps a BinaryAnalysisResult back to the (isNetwork,
// isHighRisk) booleans that the binary-signal flow tests assert on. Uncertain
// maps to (true, true), mirroring the pre-refactor fail-closed behavior so the
// signal-flow tests keep their shape; the Uncertain-vs-HighRisk distinction is
// covered by the dedicated Classify tests.
func legacyBoolsFromResult(res risktypes.BinaryAnalysisResult) (isNetwork, isHighRisk bool) {
	switch res.Class {
	case risktypes.BinaryAnalysisUncertain:
		return true, true
	case risktypes.BinaryAnalysisHighRisk:
		return slices.Contains(res.ReasonCodes, risktypes.ReasonBinaryAnalysisNetwork), true
	case risktypes.BinaryAnalysisNetwork:
		return true, false
	default: // BinaryAnalysisClean
		return false, false
	}
}

// profileNetwork reports whether the command's resolved profile classifies this
// invocation as a network operation. This is the profile-matching half of the
// former IsNetworkOperation; binary-analysis and argument-only detection for
// unprofiled commands are no longer part of network classification.
func profileNetwork(cmdName string, args []string) bool {
	profile, found := ResolveProfile(cmdName)
	if !found {
		return false
	}
	return ProfileNetworkApplies(profile, args)
}

// analyzeBinarySignals is a test-only adapter over Classify that returns the
// legacy (isNetwork, isHighRisk) booleans, so the binary-signal flow tests can
// exercise the classification without being rewritten case by case.
func (a *NetworkAnalyzer) analyzeBinarySignals(cmdPath, contentHash string) (isNetwork, isHighRisk bool, err error) {
	res, err := a.Classify(cmdPath, contentHash)
	if err != nil {
		return false, false, err
	}
	isNetwork, isHighRisk = legacyBoolsFromResult(res)
	return isNetwork, isHighRisk, nil
}
