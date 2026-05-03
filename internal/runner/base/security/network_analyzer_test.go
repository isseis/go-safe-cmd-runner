//go:build test

package security

import (
	"errors"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/dynamicanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyscallAnalysisHasSVCSignal_Nil verifies that a nil result returns false.
func TestSyscallAnalysisHasSVCSignal_Nil(t *testing.T) {
	assert.False(t, syscallAnalysisHasSVCSignal(nil))
}

func TestConstructors_PanicOnEmptyGOOS(t *testing.T) {
	assert.Panics(t, func() {
		_ = newNetworkAnalyzer("")
	})
	assert.Panics(t, func() {
		_ = newNetworkAnalyzerWithStore("", nil)
	})
	assert.Panics(t, func() {
		_ = NewNetworkAnalyzer("", nil, nil, nil, nil)
	})
}

func TestConstructors_AcceptCurrentGOOS(t *testing.T) {
	assert.NotPanics(t, func() {
		_ = newNetworkAnalyzer(runtime.GOOS)
	})
	assert.NotPanics(t, func() {
		_ = newNetworkAnalyzerWithStore(runtime.GOOS, nil)
	})
	assert.NotPanics(t, func() {
		_ = NewNetworkAnalyzer(runtime.GOOS, nil, nil, nil, nil)
	})
	assert.NotPanics(t, func() {
		_ = NewNetworkAnalyzer(runtime.GOOS, nil, nil, nil, nil)
	})
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
			AnalysisWarnings: []string{"svc #0x80 detected: syscall number unresolved, direct kernel call bypassing libSystem.dylib"},
		},
	}
	assert.False(t, syscallAnalysisHasSVCSignal(r))
}

// TestSyscallAnalysisHasSVCSignal_WithDeterminationMethod verifies that an unresolved svc
// (Number=-1, DeterminationMethod=="direct_svc_0x80") triggers the high-risk svc signal.
func TestSyscallAnalysisHasSVCSignal_WithDeterminationMethod(t *testing.T) {
	r := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []common.SyscallInfo{
				{Number: -1, Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "direct_svc_0x80"}}},
			},
		},
	}
	assert.True(t, syscallAnalysisHasSVCSignal(r))
}

// TestSyscallAnalysisHasSVCSignal_ResolvedNonNetworkSVC verifies that a resolved
// non-network svc (Number != -1) does NOT trigger the high-risk signal.
// After filter removal, resolved svc entries appear in DetectedSyscalls; only
// unresolved ones (Number==-1) are high risk.
func TestSyscallAnalysisHasSVCSignal_ResolvedNonNetworkSVC(t *testing.T) {
	r := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []common.SyscallInfo{
				{Number: 3, Name: "read", Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "direct_svc_0x80"}}},
			},
		},
	}
	assert.False(t, syscallAnalysisHasSVCSignal(r),
		"resolved non-network svc (Number != -1) must not be treated as high risk")
}

// TestSyscallAnalysisHasSVCSignal_ResolvedNetworkSVC verifies that a resolved network svc
// (Number != -1) does NOT trigger the high-risk svc signal.
// Its network nature is handled by syscallAnalysisHasNetworkSignal instead.
func TestSyscallAnalysisHasSVCSignal_ResolvedNetworkSVC(t *testing.T) {
	r := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []common.SyscallInfo{
				{Number: 97, Name: "socket", Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "direct_svc_0x80"}}},
			},
		},
	}
	assert.False(t, syscallAnalysisHasSVCSignal(r),
		"resolved network svc (Number != -1) must not be treated as high-risk svc signal")
}

// platformNetworkSyscallNums returns the architecture string and network syscall
// numbers (socket, connect) that match syscallTableForArch's behavior on the current OS.
// On macOS, syscallTableForArch ignores the arch field and always uses MacOSSyscallTable
// (socket=97, connect=98); on Linux it uses the x86_64 table (socket=41, connect=42).
func platformNetworkSyscallNums() (arch string, socketNum, connectNum int) {
	if runtime.GOOS == "darwin" {
		return "arm64", 97, 98
	}
	return "x86_64", 41, 42
}

// TestSyscallAnalysisHasNetworkSignal_ResolvedNetworkSVC verifies that a resolved network svc
// is detected as a network signal based on syscall number lookup.
func TestSyscallAnalysisHasNetworkSignal_ResolvedNetworkSVC(t *testing.T) {
	arch, socketNum, _ := platformNetworkSyscallNums()
	r := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture: arch,
			DetectedSyscalls: []common.SyscallInfo{
				{Number: socketNum, Name: "socket", Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "direct_svc_0x80"}}},
			},
		},
	}
	assert.True(t, syscallAnalysisHasNetworkSignal(r, runtime.GOOS),
		"resolved network svc (socket on %s/%s) must be detected as network signal", runtime.GOOS, arch)
}

// TestSyscallAnalysisHasNetworkSignal_LegacyFilteredRecord verifies backward compatibility:
// when DetectedSyscalls was filtered by the old FilterSyscallsForStorage logic
// (only network or Number==-1 entries present), the new judgment still produces the same result.
func TestSyscallAnalysisHasNetworkSignal_LegacyFilteredRecord(t *testing.T) {
	arch, socketNum, _ := platformNetworkSyscallNums()
	// Simulate old filtered DetectedSyscalls: only network and unresolved entries kept.
	r := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture: arch,
			DetectedSyscalls: []common.SyscallInfo{
				{Number: socketNum, Name: "socket", Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "lib_cache_match"}}},
				{Number: -1, Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "direct_svc_0x80"}}},
			},
		},
	}
	// Network signal from socket must still be detected.
	assert.True(t, syscallAnalysisHasNetworkSignal(r, runtime.GOOS),
		"legacy filtered record with network entry must still trigger network signal")
	// Unresolved svc (Number==-1) must still trigger high-risk signal.
	assert.True(t, syscallAnalysisHasSVCSignal(r),
		"legacy filtered record with unresolved svc must still trigger high-risk signal")
}

const (
	testCmdPath     = "/usr/bin/curl"
	testContentHash = "sha256:abc123"
)

type mockFileanalysisSyscallStore struct {
	result *fileanalysis.SyscallAnalysisResult
	err    error
}

func (m *mockFileanalysisSyscallStore) LoadSyscallAnalysis(_ string, _ string) (*fileanalysis.SyscallAnalysisResult, error) {
	return m.result, m.err
}

func (m *mockFileanalysisSyscallStore) SaveSyscallAnalysis(_, _ string, _ *fileanalysis.SyscallAnalysisResult) error {
	return nil
}

// svcResult builds a SyscallAnalysisResult containing a svc #0x80 signal.
func svcResult() *fileanalysis.SyscallAnalysisResult {
	return &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture: "arm64",
			DetectedSyscalls: []common.SyscallInfo{
				{Number: -1, Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "direct_svc_0x80"}}},
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

// TestIsNetworkViaBinaryAnalysis_DetectedLibraryNetworkDeps verifies that detected library
// network dependencies reported via KnownNetworkLibDeps are handled correctly.
func TestIsNetworkViaBinaryAnalysis_KnownNetworkLibDeps(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: &fileanalysis.SymbolAnalysisData{
		KnownNetworkLibDeps: []string{"libnetdep.so.1"},
	}}
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "known_network_lib_deps should mark as network-capable")
	assert.False(t, isHigh, "known_network_lib_deps alone should not mark high risk")
}

// networkDetectedData builds a SymbolAnalysisData with network symbols detected.
func networkDetectedData() *fileanalysis.SymbolAnalysisData {
	return &fileanalysis.SymbolAnalysisData{
		DetectedSymbols: []string{"socket"},
	}
}

// syscallWrapperOnlyData builds a SymbolAnalysisData that contains only
// non-network libc/libSystem symbols.
func syscallWrapperOnlyData() *fileanalysis.SymbolAnalysisData {
	return &fileanalysis.SymbolAnalysisData{
		DetectedSymbols: []string{"read", "close"},
	}
}

// TestIsNetworkViaBinaryAnalysis_SymbolAnalysisLoadError verifies that an unexpected
// SymbolAnalysis load error returns AnalysisError (true, true).
func TestIsNetworkViaBinaryAnalysis_SymbolAnalysisLoadError(t *testing.T) {
	symStore := &stubNetworkSymbolStore{err: errors.New("unexpected I/O error")}
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "unexpected SymbolAnalysis error should return true (AnalysisError)")
	assert.True(t, isHigh, "unexpected SymbolAnalysis error should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_SymbolAnalysis_HashMismatch verifies that ErrHashMismatch
// from SymbolAnalysis returns AnalysisError (true, true).
func TestIsNetworkViaBinaryAnalysis_SymbolAnalysis_HashMismatch(t *testing.T) {
	symStore := &stubNetworkSymbolStore{err: fileanalysis.ErrHashMismatch}
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

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
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "SchemaVersionMismatchError should return true (AnalysisError)")
	assert.True(t, isHigh, "SchemaVersionMismatchError should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAnalysisFound verifies that a static binary
// (nil SymbolAnalysis) with a svc #0x80 signal returns true, true.
func TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAnalysisFound(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: nil}
	svcStore := &mockFileanalysisSyscallStore{result: svcResult()}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "static binary + svc signal should return true")
	assert.True(t, isHigh, "static binary + svc signal should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_NoSVC verifies that a static binary
// (nil SymbolAnalysis) with nil SyscallAnalysis returns false, false.
func TestIsNetworkViaBinaryAnalysis_StaticBinary_NoSVC(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: nil}
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.False(t, isNet, "static binary + no svc should return false")
	assert.False(t, isHigh, "static binary + no svc should return false")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCAnalysisFound verifies that a binary with
// NoNetworkSymbols and a svc signal returns true, true (svc signal escalates to high risk).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCAnalysisFound(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	svcStore := &mockFileanalysisSyscallStore{result: svcResult()}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "svc signal should escalate to true even for NoNetworkSymbols")
	assert.True(t, isHigh, "svc signal should set high risk")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCAnalysisNil verifies that a binary with
// NoNetworkSymbols and a nil/empty SyscallAnalysis result (no svc signal) returns false, false.
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCAnalysisNil(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	// LoadSyscallAnalysis returns nil result (no svc signal).
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.False(t, isNet, "NoNetworkSymbols + no svc should return false")
	assert.False(t, isHigh, "NoNetworkSymbols + no svc should return false")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCHashMismatch verifies that
// ErrHashMismatch from SyscallAnalysis returns AnalysisError (true, true).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCHashMismatch(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	svcStore := &mockFileanalysisSyscallStore{err: fileanalysis.ErrHashMismatch}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "SVC ErrHashMismatch should return true (AnalysisError)")
	assert.True(t, isHigh, "SVC ErrHashMismatch should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCNoSyscallAnalysis verifies that
// nil SyscallAnalysis (no syscall data) falls through to SymbolAnalysis decision.
// NoNetworkSymbols + nil SyscallAnalysis → false, false (v15 guarantee: scan was performed).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCNoSyscallAnalysis(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: noNetworkSymbolData()}
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.False(t, isNet, "nil SyscallAnalysis should fall through to NoNetworkSymbols result")
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
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

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
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	assert.Panics(t, func() {
		analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)
	}, "ErrRecordNotFound from SyscallAnalysis must panic (consistency bug)")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCAnalysisFound verifies that NetworkDetected
// with a svc signal returns true, true (isHighRisk escalated).
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCAnalysisFound(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: networkDetectedData()}
	svcStore := &mockFileanalysisSyscallStore{result: svcResult()}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "NetworkDetected + svc should return true")
	assert.True(t, isHigh, "svc signal should escalate isHighRisk to true")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCNoSyscallAnalysis verifies that
// NetworkDetected with nil SyscallAnalysis returns true, false (no isHighRisk escalation).
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCNoSyscallAnalysis(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: networkDetectedData()}
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "NetworkDetected should return true")
	assert.False(t, isHigh, "nil SyscallAnalysis should not escalate isHighRisk")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_NoSVC verifies that NetworkDetected
// with no svc signal (successful load, no direct_svc_0x80) returns true, false.
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_NoSVC(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: networkDetectedData()}
	svcStore := &mockFileanalysisSyscallStore{result: noSVCResult()}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "NetworkDetected should return true")
	assert.False(t, isHigh, "no svc signal should not escalate isHighRisk")
}

// TestIsNetworkViaBinaryAnalysis_NetworkCategorySymbol verifies that
// at least one network category in DetectedSymbols causes NetworkDetected.
func TestIsNetworkViaBinaryAnalysis_NetworkCategorySymbol(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: &fileanalysis.SymbolAnalysisData{
		DetectedSymbols: []string{"read", "socket"},
	}}
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "network category symbol should trigger NetworkDetected")
	assert.False(t, isHigh, "no svc signal should keep high risk false")
}

// TestIsNetworkViaBinaryAnalysis_SyscallWrapperOnly verifies that symbols with
// only "syscall_wrapper" category do not trigger NetworkDetected.
func TestIsNetworkViaBinaryAnalysis_SyscallWrapperOnly(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: syscallWrapperOnlyData()}
	svcStore := &mockFileanalysisSyscallStore{result: nil}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.False(t, isNet, "syscall_wrapper only must not trigger NetworkDetected")
	assert.False(t, isHigh, "syscall_wrapper only must not escalate to high risk")
}

// ---- Section 6.2: syscallAnalysisHasNetworkSignal tests ----

// syscallAnalysisResultWithNetworkSyscall builds a SyscallAnalysisResult containing
// one DetectedSyscall. When hasNetwork is true, uses socket (a network syscall for the
// current OS); when false, uses read (#3, non-network on both Linux and macOS).
func syscallAnalysisResultWithNetworkSyscall(hasNetwork bool) *fileanalysis.SyscallAnalysisResult {
	arch, socketNum, _ := platformNetworkSyscallNums()
	var info common.SyscallInfo
	if hasNetwork {
		info = common.SyscallInfo{
			Number: socketNum,
			Name:   "socket",
			Occurrences: []common.SyscallOccurrence{{
				DeterminationMethod: "lib_cache_match",
				Source:              "libsystem_symbol_import",
			}},
		}
	} else {
		info = common.SyscallInfo{
			Number: 3, // read: non-network on both Linux x86_64 and macOS
			Name:   "read",
			Occurrences: []common.SyscallOccurrence{{
				DeterminationMethod: "lib_cache_match",
				Source:              "libsystem_symbol_import",
			}},
		}
	}
	return &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture:     arch,
			DetectedSyscalls: []common.SyscallInfo{info},
		},
	}
}

// TestSyscallAnalysisHasNetworkSignal_Nil verifies nil returns false.
func TestSyscallAnalysisHasNetworkSignal_Nil(t *testing.T) {
	assert.False(t, syscallAnalysisHasNetworkSignal(nil, runtime.GOOS))
}

// TestSyscallAnalysisHasNetworkSignal_Empty verifies empty result returns false.
func TestSyscallAnalysisHasNetworkSignal_Empty(t *testing.T) {
	assert.False(t, syscallAnalysisHasNetworkSignal(&fileanalysis.SyscallAnalysisResult{}, runtime.GOOS))
}

// TestSyscallAnalysisHasNetworkSignal_NetworkSyscall verifies that a network syscall
// (socket #41 on x86_64) triggers the network signal.
func TestSyscallAnalysisHasNetworkSignal_NetworkSyscall(t *testing.T) {
	assert.True(t, syscallAnalysisHasNetworkSignal(syscallAnalysisResultWithNetworkSyscall(true), runtime.GOOS))
}

// TestSyscallAnalysisHasNetworkSignal_NonNetworkSyscall verifies that a non-network syscall
// (read #3 on x86_64) does not trigger the network signal.
func TestSyscallAnalysisHasNetworkSignal_NonNetworkSyscall(t *testing.T) {
	assert.False(t, syscallAnalysisHasNetworkSignal(syscallAnalysisResultWithNetworkSyscall(false), runtime.GOOS))
}

// TestSyscallAnalysisHasNetworkSignal_MultipleEntries verifies that any network syscall entry
// is sufficient to trigger the signal.
func TestSyscallAnalysisHasNetworkSignal_MultipleEntries(t *testing.T) {
	arch, socketNum, connectNum := platformNetworkSyscallNums()
	result := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture: arch,
			DetectedSyscalls: []common.SyscallInfo{
				{Number: 3}, // read: non-network on both Linux and macOS
				{Number: socketNum, Name: "socket"},
				{Number: connectNum, Name: "connect"},
			},
		},
	}
	assert.True(t, syscallAnalysisHasNetworkSignal(result, runtime.GOOS))
}

// ---- Section 6.2: isNetworkViaBinaryAnalysis syscall-signal flow tests ----

// syscallResultWithNetworkEntry builds a SyscallAnalysisResult with a network syscall entry.
func syscallResultWithNetworkEntry() *fileanalysis.SyscallAnalysisResult {
	return syscallAnalysisResultWithNetworkSyscall(true)
}

// syscallResultWithNonNetworkEntry builds a SyscallAnalysisResult with a non-network syscall entry.
func syscallResultWithNonNetworkEntry() *fileanalysis.SyscallAnalysisResult {
	return syscallAnalysisResultWithNetworkSyscall(false)
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_NetworkSyscall verifies that a static binary
// (nil SymbolAnalysis) with a network syscall in SyscallAnalysis returns true, false.
// Network syscall detection does not escalate to high risk — only direct_svc_0x80 does.
func TestIsNetworkViaBinaryAnalysis_StaticBinary_NetworkSyscall(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: nil}
	svcStore := &mockFileanalysisSyscallStore{result: syscallResultWithNetworkEntry()}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "static binary with network syscall should return true")
	assert.False(t, isHigh, "network syscall detection should not escalate to high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_NonNetworkSyscall verifies that a static binary
// with no network syscall and no svc returns false, false.
func TestIsNetworkViaBinaryAnalysis_StaticBinary_NonNetworkSyscall(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: nil}
	svcStore := &mockFileanalysisSyscallStore{result: syscallResultWithNonNetworkEntry()}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.False(t, isNet, "static binary with no network syscall should return false")
	assert.False(t, isHigh, "no network signal should not escalate to high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAndNetworkSyscall verifies that a static binary
// with both an unresolved svc #0x80 and a network syscall returns true, true (svc escalates).
func TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAndNetworkSyscall(t *testing.T) {
	// Build a result that has both an unresolved direct_svc_0x80 and a network syscall.
	result := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []common.SyscallInfo{
				{Number: -1, Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "direct_svc_0x80"}}},
				{Number: 97, Name: "socket", Occurrences: []common.SyscallOccurrence{{Source: "libsystem_symbol_import"}}},
			},
		},
	}
	symStore := &stubNetworkSymbolStore{data: nil}
	svcStore := &mockFileanalysisSyscallStore{result: result}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "svc + network syscall should return true")
	assert.True(t, isHigh, "svc #0x80 should escalate to high risk")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_WithNetworkSyscall verifies that when
// SymbolAnalysis detects network and SyscallAnalysis also has a network syscall,
// network is detected (true, false) — syscall detection alone does not escalate to high risk.
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_WithNetworkSyscall(t *testing.T) {
	symStore := &stubNetworkSymbolStore{data: networkDetectedData()}
	svcStore := &mockFileanalysisSyscallStore{result: syscallResultWithNetworkEntry()}
	analyzer := NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil)

	isNet, isHigh := analyzer.isNetworkViaBinaryAnalysis(testCmdPath, testContentHash)

	assert.True(t, isNet, "SymbolAnalysis network should return true")
	assert.False(t, isHigh, "network syscall without svc should not escalate to high risk")
}

// TestSyscallAnalysisHasNetworkSignal_UnknownArch verifies that an unknown architecture
// (mips) causes network detection to be skipped and returns false.
func TestSyscallAnalysisHasNetworkSignal_UnknownArch(t *testing.T) {
	result := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture: "mips",
			DetectedSyscalls: []common.SyscallInfo{
				{Number: 41},
			},
		},
	}
	assert.False(t, syscallAnalysisHasNetworkSignal(result, runtime.GOOS))
}

// TestSyscallAnalysisHasNetworkSignal_NegativeNumber verifies that a negative syscall
// number (unresolved SVC) does not trigger the network signal.
func TestSyscallAnalysisHasNetworkSignal_NegativeNumber(t *testing.T) {
	result := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture: "x86_64",
			DetectedSyscalls: []common.SyscallInfo{
				{Number: -1},
			},
		},
	}
	assert.False(t, syscallAnalysisHasNetworkSignal(result, runtime.GOOS))
}

// ----- mock types for checkDynLibDepsNetwork tests -----

type mockDynLibDepsStore struct {
	deps []fileanalysis.LibEntry
	err  error
}

func (m *mockDynLibDepsStore) LoadDynLibDeps(_ string, _ string) ([]fileanalysis.LibEntry, error) {
	return m.deps, m.err
}

type mockDynLibAnalysisStore struct {
	results map[string]*dynamicanalysis.Result
	err     error
}

func (m *mockDynLibAnalysisStore) LoadOrAnalyzeAndStore(libPath, _ string) (*dynamicanalysis.Result, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results[libPath], nil
}

func (m *mockDynLibAnalysisStore) LoadAnalysis(libPath, _ string) (*dynamicanalysis.Result, error) {
	if m.err != nil {
		return nil, m.err
	}
	r, ok := m.results[libPath]
	if !ok {
		return nil, dynamicanalysis.ErrAnalysisNotFound
	}
	return r, nil
}

func makeNetworkAnalyzerWithLibStores(
	depsStore fileanalysis.DynLibDepsStore,
	libStore dynamicanalysis.Store,
) *NetworkAnalyzer {
	return NewNetworkAnalyzer(runtime.GOOS, nil, nil, depsStore, libStore)
}

// TestCheckDynLibDepsNetwork_NetworkSymbol verifies that a library with a detected
// network symbol causes the result to be (isNetwork=true, isHighRisk=false).
func TestCheckDynLibDepsNetwork_NetworkSymbol(t *testing.T) {
	dep := fileanalysis.LibEntry{SOName: "libssl.so.3", Path: "/usr/lib/libssl.so.3", Hash: "sha256:aa"}
	depsStore := &mockDynLibDepsStore{deps: []fileanalysis.LibEntry{dep}}
	libStore := &mockDynLibAnalysisStore{
		results: map[string]*dynamicanalysis.Result{
			"/usr/lib/libssl.so.3": {
				SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
					DetectedSymbols: []string{"connect"},
				},
			},
		},
	}
	a := makeNetworkAnalyzerWithLibStores(depsStore, libStore)
	isNetwork, isHighRisk := a.checkDynLibDepsNetwork(testCmdPath, testContentHash)
	require.True(t, isNetwork)
	assert.False(t, isHighRisk)
}

// TestCheckDynLibDepsNetwork_DynamicLoadSymbols verifies that a library with dlopen/dlsym
// causes isHighRisk=true.
func TestCheckDynLibDepsNetwork_DynamicLoadSymbols(t *testing.T) {
	dep := fileanalysis.LibEntry{SOName: "libplugin.so", Path: "/usr/lib/libplugin.so", Hash: "sha256:bb"}
	depsStore := &mockDynLibDepsStore{deps: []fileanalysis.LibEntry{dep}}
	libStore := &mockDynLibAnalysisStore{
		results: map[string]*dynamicanalysis.Result{
			"/usr/lib/libplugin.so": {
				DynamicLoadSymbols: []string{"dlopen"},
			},
		},
	}
	a := makeNetworkAnalyzerWithLibStores(depsStore, libStore)
	_, isHighRisk := a.checkDynLibDepsNetwork(testCmdPath, testContentHash)
	assert.True(t, isHighRisk)
}

// TestCheckDynLibDepsNetwork_ErrAnalysisNotFound verifies fail-closed behaviour when
// library analysis is missing from the store.
func TestCheckDynLibDepsNetwork_ErrAnalysisNotFound(t *testing.T) {
	dep := fileanalysis.LibEntry{SOName: "libunknown.so", Path: "/usr/lib/libunknown.so", Hash: "sha256:cc"}
	depsStore := &mockDynLibDepsStore{deps: []fileanalysis.LibEntry{dep}}
	libStore := &mockDynLibAnalysisStore{
		// results is nil → LoadAnalysis returns ErrAnalysisNotFound
	}
	a := makeNetworkAnalyzerWithLibStores(depsStore, libStore)
	isNetwork, isHighRisk := a.checkDynLibDepsNetwork(testCmdPath, testContentHash)
	assert.True(t, isNetwork)
	assert.True(t, isHighRisk)
}

// TestCheckDynLibDepsNetwork_VDSOSkipped verifies that VDSO entries are skipped.
func TestCheckDynLibDepsNetwork_VDSOSkipped(t *testing.T) {
	dep := fileanalysis.LibEntry{SOName: "linux-vdso.so.1", Path: "", Hash: ""}
	depsStore := &mockDynLibDepsStore{deps: []fileanalysis.LibEntry{dep}}
	// libStore with error to ensure it is never called.
	libStore := &mockDynLibAnalysisStore{err: errors.New("should not be called")}
	a := makeNetworkAnalyzerWithLibStores(depsStore, libStore)
	isNetwork, isHighRisk := a.checkDynLibDepsNetwork(testCmdPath, testContentHash)
	assert.False(t, isNetwork)
	assert.False(t, isHighRisk)
}

// TestCheckDynLibDepsNetwork_NoDeps verifies that a static binary (no deps) returns
// (false, false).
func TestCheckDynLibDepsNetwork_NoDeps(t *testing.T) {
	depsStore := &mockDynLibDepsStore{deps: nil}
	libStore := &mockDynLibAnalysisStore{}
	a := makeNetworkAnalyzerWithLibStores(depsStore, libStore)
	isNetwork, isHighRisk := a.checkDynLibDepsNetwork(testCmdPath, testContentHash)
	assert.False(t, isNetwork)
	assert.False(t, isHighRisk)
}

// TestCheckDynLibDepsNetwork_DepsLoadError verifies fail-closed when deps store errors.
func TestCheckDynLibDepsNetwork_DepsLoadError(t *testing.T) {
	depsStore := &mockDynLibDepsStore{err: errors.New("disk read failed")}
	libStore := &mockDynLibAnalysisStore{}
	a := makeNetworkAnalyzerWithLibStores(depsStore, libStore)
	isNetwork, isHighRisk := a.checkDynLibDepsNetwork(testCmdPath, testContentHash)
	assert.True(t, isNetwork)
	assert.True(t, isHighRisk)
}
