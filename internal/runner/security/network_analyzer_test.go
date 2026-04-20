//go:build test

package security

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/stretchr/testify/assert"
)

// TestSyscallAnalysisHasSVCSignal_Nil verifies that a nil result returns false.
func TestSyscallAnalysisHasSVCSignal_Nil(t *testing.T) {
	assert.False(t, syscallAnalysisHasSVCSignal(nil))
}

// TestSyscallAnalysisHasSVCSignal_Empty verifies that an empty result returns false.
func TestSyscallAnalysisHasSVCSignal_Empty(t *testing.T) {
	assert.False(t, syscallAnalysisHasSVCSignal(&fileanalysis.SyscallAnalysisResult{}))
}

// TestSyscallAnalysisHasSVCSignal_WithWarningsOnly verifies that AnalysisWarnings alone
// do not trigger the svc signal (to avoid false positives from ELF analysis).
func TestSyscallAnalysisHasSVCSignal_WithWarningsOnly(t *testing.T) {
	r := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			AnalysisWarnings: []string{"svc #0x80 detected: direct syscall bypassing libSystem.dylib"},
		},
	}
	assert.False(t, syscallAnalysisHasSVCSignal(r))
}

// TestSyscallAnalysisHasSVCSignal_WithDeterminationMethod verifies that a DetectedSyscall
// with DeterminationMethod == "direct_svc_0x80" triggers the svc signal.
func TestSyscallAnalysisHasSVCSignal_WithDeterminationMethod(t *testing.T) {
	r := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []common.SyscallInfo{
				{Number: -1, DeterminationMethod: "direct_svc_0x80"},
			},
		},
	}
	assert.True(t, syscallAnalysisHasSVCSignal(r))
}

const (
	testCmdPath     = "/usr/bin/curl"
	testContentHash = "sha256:abc123"
)

// svcResult builds a SyscallAnalysisResult containing a svc #0x80 signal.
func svcResult() *fileanalysis.SyscallAnalysisResult {
	return &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture: "arm64",
			DetectedSyscalls: []common.SyscallInfo{
				{Number: -1, DeterminationMethod: "direct_svc_0x80"},
			},
		},
	}
}

// noSVCResult builds a SyscallAnalysisResult with no svc #0x80 signal.
func noSVCResult() *fileanalysis.SyscallAnalysisResult {
	return &fileanalysis.SyscallAnalysisResult{}
}

// noNetworkSymbolData builds a SymbolAnalysisData with no network symbols.
func noNetworkSymbolData() *fileanalysis.SymbolAnalysisData {
	return &fileanalysis.SymbolAnalysisData{
		DetectedSymbols:     nil,
		KnownNetworkLibDeps: nil,
	}
}

// networkDetectedData builds a SymbolAnalysisData with network symbols detected.
func networkDetectedData() *fileanalysis.SymbolAnalysisData {
	return &fileanalysis.SymbolAnalysisData{
		DetectedSymbols: []fileanalysis.DetectedSymbolEntry{
			{Name: "socket", Category: "socket"},
		},
	}
}

// TestIsNetworkViaBinaryAnalysis_SymbolAnalysisCacheMiss verifies that an unexpected
// SymbolAnalysis load error returns AnalysisError (true, true).
func TestIsNetworkViaBinaryAnalysis_SymbolAnalysisCacheMiss(t *testing.T) {
	symStore := &stubNetworkSymbolStore{err: errors.New("unexpected I/O error")}
	svcStore := &mockFileanalysisSyscallStore{err: fileanalysis.ErrNoSyscallAnalysis}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "unexpected SymbolAnalysis error should return true (AnalysisError)")
	assert.True(t, isHigh, "unexpected SymbolAnalysis error should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_SymbolAnalysis_HashMismatch verifies that ErrHashMismatch
// from SymbolAnalysis returns AnalysisError (true, true).
func TestIsNetworkViaBinaryAnalysis_SymbolAnalysis_HashMismatch(t *testing.T) {
	symStore := &stubNetworkSymbolStore{err: fileanalysis.ErrHashMismatch}
	svcStore := &mockFileanalysisSyscallStore{err: fileanalysis.ErrNoSyscallAnalysis}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "ErrHashMismatch should return true (AnalysisError)")
	assert.True(t, isHigh, "ErrHashMismatch should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_SymbolAnalysis_SchemaMismatch verifies that a
// SchemaVersionMismatchError from SymbolAnalysis returns AnalysisError (true, true).
func TestIsNetworkViaBinaryAnalysis_SymbolAnalysis_SchemaMismatch(t *testing.T) {
	schemaErr := &fileanalysis.SchemaVersionMismatchError{
		Expected: fileanalysis.CurrentSchemaVersion,
		Actual:   fileanalysis.CurrentSchemaVersion - 1,
	}
	symStore := &stubNetworkSymbolStore{err: schemaErr}
	svcStore := &mockFileanalysisSyscallStore{err: fileanalysis.ErrNoSyscallAnalysis}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "SchemaVersionMismatchError should return true (AnalysisError)")
	assert.True(t, isHigh, "SchemaVersionMismatchError should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCCacheHit verifies that a static binary
// (ErrNoNetworkSymbolAnalysis) with a svc #0x80 signal returns true, true.
func TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCCacheHit(t *testing.T) {
	symStore := &stubNetworkSymbolStore{err: fileanalysis.ErrNoNetworkSymbolAnalysis}
	svcStore := &mockFileanalysisSyscallStore{result: svcResult()}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "static binary + svc signal should return true")
	assert.True(t, isHigh, "static binary + svc signal should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCSignalPresent verifies that a static binary
// with SyscallAnalysis loaded successfully and a svc signal returns true, true.
func TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCSignalPresent(t *testing.T) {
	symStore := &stubNetworkSymbolStore{err: fileanalysis.ErrNoNetworkSymbolAnalysis}
	svcStore := &mockFileanalysisSyscallStore{result: svcResult()}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet)
	assert.True(t, isHigh)
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_NoSVC verifies that a static binary
// (ErrNoNetworkSymbolAnalysis) with ErrNoSyscallAnalysis returns false, false.
func TestIsNetworkViaBinaryAnalysis_StaticBinary_NoSVC(t *testing.T) {
	symStore := &stubNetworkSymbolStore{err: fileanalysis.ErrNoNetworkSymbolAnalysis}
	svcStore := &mockFileanalysisSyscallStore{err: fileanalysis.ErrNoSyscallAnalysis}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.False(t, isNet, "static binary + no svc should return false")
	assert.False(t, isHigh, "static binary + no svc should return false")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheHit verifies that a binary with
// NoNetworkSymbols and a svc signal returns true, true (svc signal escalates to high risk).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheHit(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	svcStore := &mockFileanalysisSyscallStore{result: svcResult()}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "svc signal should escalate to true even for NoNetworkSymbols")
	assert.True(t, isHigh, "svc signal should set high risk")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheNil verifies that a binary with
// NoNetworkSymbols and a nil/empty SyscallAnalysis result (no svc signal) returns false, false.
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCCacheNil(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	// LoadSyscallAnalysis returns nil result (no svc signal).
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.False(t, isNet, "NoNetworkSymbols + no svc should return false")
	assert.False(t, isHigh, "NoNetworkSymbols + no svc should return false")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCHashMismatch verifies that
// ErrHashMismatch from SyscallAnalysis returns AnalysisError (true, true).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCHashMismatch(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	svcStore := &mockFileanalysisSyscallStore{err: fileanalysis.ErrHashMismatch}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "SVC ErrHashMismatch should return true (AnalysisError)")
	assert.True(t, isHigh, "SVC ErrHashMismatch should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCNoSyscallAnalysis verifies that
// ErrNoSyscallAnalysis from SyscallAnalysis falls through to SymbolAnalysis decision.
// NoNetworkSymbols + ErrNoSyscallAnalysis → false, false (v15 guarantee: scan was performed).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCNoSyscallAnalysis(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	svcStore := &mockFileanalysisSyscallStore{err: fileanalysis.ErrNoSyscallAnalysis}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.False(t, isNet, "ErrNoSyscallAnalysis should fall through to NoNetworkSymbols result")
	assert.False(t, isHigh)
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCSchemaMismatch verifies that a
// SchemaVersionMismatchError from SyscallAnalysis returns AnalysisError (true, true).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCSchemaMismatch(t *testing.T) {
	schemaErr := &fileanalysis.SchemaVersionMismatchError{
		Expected: fileanalysis.CurrentSchemaVersion,
		Actual:   fileanalysis.CurrentSchemaVersion - 1,
	}
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	svcStore := &mockFileanalysisSyscallStore{err: schemaErr}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "SVC SchemaVersionMismatchError should return AnalysisError")
	assert.True(t, isHigh, "SVC SchemaVersionMismatchError should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCRecordNotFound verifies that
// ErrRecordNotFound from SyscallAnalysis panics (consistency bug: SymbolAnalysis record
// exists but the matching SyscallAnalysis record is missing).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCRecordNotFound(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	svcStore := &mockFileanalysisSyscallStore{err: fileanalysis.ErrRecordNotFound}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	assert.Panics(t, func() {
		analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)
	}, "ErrRecordNotFound from SyscallAnalysis must panic (consistency bug)")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCCacheHit verifies that NetworkDetected
// with a svc signal returns true, true (isHighRisk escalated).
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCCacheHit(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: networkDetectedData()}
	svcStore := &mockFileanalysisSyscallStore{result: svcResult()}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "NetworkDetected + svc should return true")
	assert.True(t, isHigh, "svc signal should escalate isHighRisk to true")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCNoSyscallAnalysis verifies that
// NetworkDetected with ErrNoSyscallAnalysis returns true, false (no isHighRisk escalation).
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCNoSyscallAnalysis(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: networkDetectedData()}
	svcStore := &mockFileanalysisSyscallStore{err: fileanalysis.ErrNoSyscallAnalysis}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "NetworkDetected should return true")
	assert.False(t, isHigh, "ErrNoSyscallAnalysis should not escalate isHighRisk")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_NoSVC verifies that NetworkDetected
// with no svc signal (successful load, no direct_svc_0x80) returns true, false.
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_NoSVC(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: networkDetectedData()}
	svcStore := &mockFileanalysisSyscallStore{result: noSVCResult()}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "NetworkDetected should return true")
	assert.False(t, isHigh, "no svc signal should not escalate isHighRisk")
}

// ---- Section 6.2: syscallAnalysisHasNetworkSignal tests ----

// syscallAnalysisResultWithIsNetwork builds a SyscallAnalysisResult containing
// one DetectedSyscall with IsNetwork set to the given value.
func syscallAnalysisResultWithIsNetwork(isNetwork bool) *fileanalysis.SyscallAnalysisResult {
	return &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []common.SyscallInfo{
				{
					Number:              97,
					Name:                "socket",
					IsNetwork:           isNetwork,
					DeterminationMethod: "lib_cache_match",
					Source:              "libsystem_symbol_import",
				},
			},
		},
	}
}

// TestSyscallAnalysisHasNetworkSignal_Nil verifies nil returns false.
func TestSyscallAnalysisHasNetworkSignal_Nil(t *testing.T) {
	assert.False(t, syscallAnalysisHasNetworkSignal(nil))
}

// TestSyscallAnalysisHasNetworkSignal_Empty verifies empty result returns false.
func TestSyscallAnalysisHasNetworkSignal_Empty(t *testing.T) {
	assert.False(t, syscallAnalysisHasNetworkSignal(&fileanalysis.SyscallAnalysisResult{}))
}

// TestSyscallAnalysisHasNetworkSignal_IsNetworkTrue verifies that a DetectedSyscall
// with IsNetwork==true triggers the network signal.
func TestSyscallAnalysisHasNetworkSignal_IsNetworkTrue(t *testing.T) {
	assert.True(t, syscallAnalysisHasNetworkSignal(syscallAnalysisResultWithIsNetwork(true)))
}

// TestSyscallAnalysisHasNetworkSignal_IsNetworkFalse verifies that a DetectedSyscall
// with IsNetwork==false does not trigger the network signal.
func TestSyscallAnalysisHasNetworkSignal_IsNetworkFalse(t *testing.T) {
	assert.False(t, syscallAnalysisHasNetworkSignal(syscallAnalysisResultWithIsNetwork(false)))
}

// TestSyscallAnalysisHasNetworkSignal_MultipleEntries verifies that any entry with
// IsNetwork==true is sufficient to trigger the signal.
func TestSyscallAnalysisHasNetworkSignal_MultipleEntries(t *testing.T) {
	result := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []common.SyscallInfo{
				{Number: 5, IsNetwork: false},
				{Number: 97, IsNetwork: true, Name: "socket"},
				{Number: 98, IsNetwork: true, Name: "connect"},
			},
		},
	}
	assert.True(t, syscallAnalysisHasNetworkSignal(result))
}

// ---- Section 6.2: isNetworkViaBinaryAnalysis IsNetwork flow tests ----

// syscallResultWithNetworkEntry builds a SyscallAnalysisResult with a network syscall entry.
func syscallResultWithNetworkEntry() *fileanalysis.SyscallAnalysisResult {
	return syscallAnalysisResultWithIsNetwork(true)
}

// syscallResultWithNonNetworkEntry builds a SyscallAnalysisResult with a non-network syscall entry.
func syscallResultWithNonNetworkEntry() *fileanalysis.SyscallAnalysisResult {
	return syscallAnalysisResultWithIsNetwork(false)
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_IsNetworkTrue verifies that a static binary
// (ErrNoNetworkSymbolAnalysis) with IsNetwork==true in SyscallAnalysis returns true, false.
// (IsNetwork path does not escalate to high risk — only direct_svc_0x80 does.)
func TestIsNetworkViaBinaryAnalysis_StaticBinary_IsNetworkTrue(t *testing.T) {
	symStore := &stubNetworkSymbolStore{err: fileanalysis.ErrNoNetworkSymbolAnalysis}
	svcStore := &mockFileanalysisSyscallStore{result: syscallResultWithNetworkEntry()}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "static binary with IsNetwork syscall should return true")
	assert.False(t, isHigh, "IsNetwork path should not escalate to high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_IsNetworkFalse verifies that a static binary
// with no network syscall and no svc returns false, false.
func TestIsNetworkViaBinaryAnalysis_StaticBinary_IsNetworkFalse(t *testing.T) {
	symStore := &stubNetworkSymbolStore{err: fileanalysis.ErrNoNetworkSymbolAnalysis}
	svcStore := &mockFileanalysisSyscallStore{result: syscallResultWithNonNetworkEntry()}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.False(t, isNet, "static binary with no network syscall should return false")
	assert.False(t, isHigh, "no network signal should not escalate to high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAndIsNetwork verifies that a static binary
// with both svc #0x80 and IsNetwork==true returns true, true (svc escalates).
func TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAndIsNetwork(t *testing.T) {
	// Build a result that has both direct_svc_0x80 and IsNetwork==true.
	result := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []common.SyscallInfo{
				{Number: -1, DeterminationMethod: "direct_svc_0x80", IsNetwork: false},
				{Number: 97, Name: "socket", IsNetwork: true, Source: "libsystem_symbol_import"},
			},
		},
	}
	symStore := &stubNetworkSymbolStore{err: fileanalysis.ErrNoNetworkSymbolAnalysis}
	svcStore := &mockFileanalysisSyscallStore{result: result}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "svc + IsNetwork should return true")
	assert.True(t, isHigh, "svc #0x80 should escalate to high risk")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_IsNetworkTrue verifies that when
// SymbolAnalysis detects network but the SyscallAnalysis also has IsNetwork==true,
// network is detected (true, false) — IsNetwork does not add high risk over symbol analysis.
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_IsNetworkTrue(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: networkDetectedData()}
	svcStore := &mockFileanalysisSyscallStore{result: syscallResultWithNetworkEntry()}
	analyzer := newNetworkAnalyzerWithStores(symStore, svcStore)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "SymbolAnalysis network should return true")
	assert.False(t, isHigh, "IsNetwork without svc should not escalate to high risk")
}
