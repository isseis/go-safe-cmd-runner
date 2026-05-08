//go:build test

package security

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
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
		_ = NewNetworkAnalyzer("", AnalysisDeps{})
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
		_ = NewNetworkAnalyzer(runtime.GOOS, AnalysisDeps{})
	})
}

// TestSyscallAnalysisHasSVCSignal_Empty verifies that an empty result returns false.
func TestSyscallAnalysisHasSVCSignal_Empty(t *testing.T) {
	assert.False(t, syscallAnalysisHasSVCSignal(&fileanalysis.SyscallAnalysisData{}))
}

// TestSyscallAnalysisHasSVCSignal_WithWarningsOnly verifies that AnalysisWarnings alone
// do not trigger the svc signal (to avoid false positives from ELF analysis).
func TestSyscallAnalysisHasSVCSignal_WithWarningsOnly(t *testing.T) {
	r := &fileanalysis.SyscallAnalysisData{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			AnalysisWarnings: []string{"svc #0x80 detected: syscall number unresolved, direct kernel call bypassing libSystem.dylib"},
		},
	}
	assert.False(t, syscallAnalysisHasSVCSignal(r))
}

// TestSyscallAnalysisHasSVCSignal_WithDeterminationMethod verifies that an unresolved svc
// (Number=-1, DeterminationMethod=="direct_svc_0x80") triggers the high-risk svc signal.
func TestSyscallAnalysisHasSVCSignal_WithDeterminationMethod(t *testing.T) {
	r := &fileanalysis.SyscallAnalysisData{
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
	r := &fileanalysis.SyscallAnalysisData{
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
	r := &fileanalysis.SyscallAnalysisData{
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
// numbers (socket, connect) that match syscallTableForArch's behavior on the current OS
// and architecture.
// On macOS, syscallTableForArch ignores the arch field and always uses MacOSSyscallTable
// (socket=97, connect=98); on Linux it dispatches by GOARCH: x86_64 (socket=41, connect=42)
// or arm64 (socket=198, connect=203).
func platformNetworkSyscallNums() (arch string, socketNum, connectNum int) {
	if runtime.GOOS == "darwin" {
		return "arm64", 97, 98
	}
	if runtime.GOARCH == "arm64" {
		return "arm64", 198, 203
	}
	return "x86_64", 41, 42
}

// platformExecSyscallNums returns the architecture string and exec syscall
// numbers that match syscallTableForArch's behavior on the current OS and
// architecture.
// On macOS, syscallTableForArch ignores the arch field and always uses
// MacOSSyscallTable (execve=59, __mac_execve=380); on Linux it dispatches by
// GOARCH: x86_64 (execve=59, execveat=322) or arm64 (execve=221, execveat=281).
func platformExecSyscallNums() (arch string, secondExecNum int) {
	if runtime.GOOS == "darwin" {
		return "arm64", 380
	}
	if runtime.GOARCH == "arm64" {
		return "arm64", 281
	}
	return "x86_64", 322
}

// TestSyscallAnalysisHasNetworkSignal_ResolvedNetworkSVC verifies that a resolved network svc
// is detected as a network signal based on syscall number lookup.
func TestSyscallAnalysisHasNetworkSignal_ResolvedNetworkSVC(t *testing.T) {
	arch, socketNum, _ := platformNetworkSyscallNums()
	r := &fileanalysis.SyscallAnalysisData{
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
	r := &fileanalysis.SyscallAnalysisData{
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

// svcSyscallData builds a SyscallAnalysisData containing a svc #0x80 signal.
func svcSyscallData() *fileanalysis.SyscallAnalysisData {
	return &fileanalysis.SyscallAnalysisData{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture: "arm64",
			DetectedSyscalls: []common.SyscallInfo{
				{Number: -1, Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "direct_svc_0x80"}}},
			},
		},
	}
}

// TestIsNetworkViaBinaryAnalysis_LoadRecordError verifies that an unexpected
// LoadRecord error is propagated as a non-nil error (fail-open for propagation).
func TestIsNetworkViaBinaryAnalysis_LoadRecordError(t *testing.T) {
	store := &stubRecordStore{err: fmt.Errorf("unexpected I/O error")}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	_, _, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.Error(t, err, "unexpected LoadRecord error must be propagated")
}

// TestIsNetworkViaBinaryAnalysis_SchemaMismatch verifies that a
// SchemaVersionMismatchError from LoadRecord returns (true, true, nil) (fail-closed).
func TestIsNetworkViaBinaryAnalysis_RecordNotFound_FailClosed(t *testing.T) {
	store := &stubRecordStore{err: fileanalysis.ErrRecordNotFound}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "missing analysis record must be treated as high risk (fail-closed)")
	assert.True(t, isHigh, "missing analysis record must be treated as high risk (fail-closed)")
}

func TestIsNetworkViaBinaryAnalysis_ContentHashMismatch(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: "sha256:stale_hash_from_old_record",
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "content hash mismatch should return isNetwork=true (fail-closed)")
	assert.True(t, isHigh, "content hash mismatch should return high risk")
}

func TestIsNetworkViaBinaryAnalysis_SchemaMismatch(t *testing.T) {
	schemaErr := &fileanalysis.SchemaVersionMismatchError{
		Expected: fileanalysis.CurrentSchemaVersion,
		Actual:   fileanalysis.CurrentSchemaVersion - 1,
	}
	store := &stubRecordStore{err: schemaErr}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "SchemaVersionMismatchError should return isNetwork=true (fail-closed)")
	assert.True(t, isHigh, "SchemaVersionMismatchError should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAnalysisFound verifies that a static binary
// (nil SymbolAnalysis) with a svc #0x80 signal returns (true, true).
func TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAnalysisFound(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash:     testContentHash,
		SyscallAnalysis: svcSyscallData(),
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "static binary + svc signal should return true")
	assert.True(t, isHigh, "static binary + svc signal should return high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_NoSVC verifies that a static binary
// with no SymbolAnalysis and no SyscallAnalysis returns (false, false).
func TestIsNetworkViaBinaryAnalysis_StaticBinary_NoSVC(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.False(t, isNet, "static binary + no svc should return false")
	assert.False(t, isHigh, "static binary + no svc should return false")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCAnalysisFound verifies that a binary with
// no network symbols and a svc signal returns (true, true).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_SVCAnalysisFound(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: nil,
		},
		SyscallAnalysis: svcSyscallData(),
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "svc signal should escalate to true even for no network symbols")
	assert.True(t, isHigh, "svc signal should set high risk")
}

// TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_NoSVC verifies that a binary with
// no network symbols and no SyscallAnalysis returns (false, false).
func TestIsNetworkViaBinaryAnalysis_NoNetworkSymbols_NoSVC(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: nil,
		},
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.False(t, isNet, "no network symbols + no svc should return false")
	assert.False(t, isHigh, "no network symbols + no svc should return false")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCAnalysisFound verifies that NetworkDetected
// with a svc signal returns (true, true).
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_SVCAnalysisFound(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []string{"socket"},
		},
		SyscallAnalysis: svcSyscallData(),
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "NetworkDetected + svc should return true")
	assert.True(t, isHigh, "svc signal should escalate isHighRisk to true")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_NoSyscallAnalysis verifies that
// NetworkDetected with nil SyscallAnalysis returns (true, false).
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_NoSyscallAnalysis(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []string{"socket"},
		},
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "NetworkDetected should return true")
	assert.False(t, isHigh, "nil SyscallAnalysis should not escalate isHighRisk")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_NoSVC verifies that NetworkDetected
// with no svc signal (successful load, no direct_svc_0x80) returns (true, false).
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_NoSVC(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []string{"socket"},
		},
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{},
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "NetworkDetected should return true")
	assert.False(t, isHigh, "no svc signal should not escalate isHighRisk")
}

// TestIsNetworkViaBinaryAnalysis_NetworkCategorySymbol verifies that
// at least one network category in DetectedSymbols causes NetworkDetected.
func TestIsNetworkViaBinaryAnalysis_NetworkCategorySymbol(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []string{"read", "socket"},
		},
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "network category symbol should trigger NetworkDetected")
	assert.False(t, isHigh, "no svc signal should keep high risk false")
}

// TestIsNetworkViaBinaryAnalysis_SyscallWrapperOnly verifies that symbols with
// only "syscall_wrapper" category do not trigger NetworkDetected.
func TestIsNetworkViaBinaryAnalysis_SyscallWrapperOnly(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []string{"read", "close"},
		},
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.False(t, isNet, "syscall_wrapper only must not trigger NetworkDetected")
	assert.False(t, isHigh, "syscall_wrapper only must not escalate to high risk")
}

// ---- Section 6.2: syscallAnalysisHasNetworkSignal tests ----

// syscallDataWithNetworkSyscall builds a SyscallAnalysisData containing
// one DetectedSyscall. When hasNetwork is true, uses socket (a network syscall for the
// current OS); when false, uses read (#3, non-network on both Linux and macOS).
func syscallDataWithNetworkSyscall(hasNetwork bool) *fileanalysis.SyscallAnalysisData {
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
	return &fileanalysis.SyscallAnalysisData{
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
	assert.False(t, syscallAnalysisHasNetworkSignal(&fileanalysis.SyscallAnalysisData{}, runtime.GOOS))
}

// TestSyscallAnalysisHasNetworkSignal_NetworkSyscall verifies that a network syscall
// (socket) triggers the network signal.
func TestSyscallAnalysisHasNetworkSignal_NetworkSyscall(t *testing.T) {
	assert.True(t, syscallAnalysisHasNetworkSignal(syscallDataWithNetworkSyscall(true), runtime.GOOS))
}

// TestSyscallAnalysisHasNetworkSignal_NonNetworkSyscall verifies that a non-network syscall
// (read #3) does not trigger the network signal.
func TestSyscallAnalysisHasNetworkSignal_NonNetworkSyscall(t *testing.T) {
	assert.False(t, syscallAnalysisHasNetworkSignal(syscallDataWithNetworkSyscall(false), runtime.GOOS))
}

// TestSyscallAnalysisHasNetworkSignal_MultipleEntries verifies that any network syscall entry
// is sufficient to trigger the signal.
func TestSyscallAnalysisHasNetworkSignal_MultipleEntries(t *testing.T) {
	arch, socketNum, connectNum := platformNetworkSyscallNums()
	result := &fileanalysis.SyscallAnalysisData{
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

// ---- Section 6.2: analyzeBinarySignals syscall-signal flow tests ----

// TestIsNetworkViaBinaryAnalysis_StaticBinary_NetworkSyscall verifies that a static binary
// (nil SymbolAnalysis) with a network syscall in SyscallAnalysis returns (true, false).
// Network syscall detection does not escalate to high risk — only direct_svc_0x80 does.
func TestIsNetworkViaBinaryAnalysis_StaticBinary_NetworkSyscall(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash:     testContentHash,
		SyscallAnalysis: syscallDataWithNetworkSyscall(true),
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "static binary with network syscall should return true")
	assert.False(t, isHigh, "network syscall detection should not escalate to high risk")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_NonNetworkSyscall verifies that a static binary
// with no network syscall and no svc returns (false, false).
func TestIsNetworkViaBinaryAnalysis_StaticBinary_NonNetworkSyscall(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash:     testContentHash,
		SyscallAnalysis: syscallDataWithNetworkSyscall(false),
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.False(t, isNet, "static binary with no network syscall should return false")
	assert.False(t, isHigh, "no network signal should not escalate to high risk")
}

// TestIsNetworkViaBinaryAnalysis_IgnoresDepsForRisk ensures DynLibDeps content
// does not affect risk detection and only top-level analysis fields are used.
func TestIsNetworkViaBinaryAnalysis_IgnoresDepsForRisk(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		DynLibDeps: []fileanalysis.LibEntry{{
			SOName: "libsocket.so",
			Path:   "/tmp/libsocket.so",
			Hash:   "sha256:deadbeef",
		}},
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{},
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				Architecture:     "x86_64",
				DetectedSyscalls: []common.SyscallInfo{{Number: 1, Name: "write"}},
			},
		},
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.False(t, isNet, "DynLibDeps should not be used for network risk judgment")
	assert.False(t, isHigh, "DynLibDeps should not affect high-risk detection")
}

// TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAndNetworkSyscall verifies that a static binary
// with both an unresolved svc #0x80 and a network syscall returns (true, true).
func TestIsNetworkViaBinaryAnalysis_StaticBinary_SVCAndNetworkSyscall(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []common.SyscallInfo{
					{Number: -1, Occurrences: []common.SyscallOccurrence{{DeterminationMethod: "direct_svc_0x80"}}},
					{Number: 97, Name: "socket", Occurrences: []common.SyscallOccurrence{{Source: "libsystem_symbol_import"}}},
				},
			},
		},
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "svc + network syscall should return true")
	assert.True(t, isHigh, "svc #0x80 should escalate to high risk")
}

// TestIsNetworkViaBinaryAnalysis_NetworkDetected_WithNetworkSyscall verifies that when
// SymbolAnalysis detects network and SyscallAnalysis also has a network syscall,
// network is detected (true, false) — syscall detection alone does not escalate to high risk.
func TestIsNetworkViaBinaryAnalysis_NetworkDetected_WithNetworkSyscall(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []string{"socket"},
		},
		SyscallAnalysis: syscallDataWithNetworkSyscall(true),
	}}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, isHigh, err := analyzer.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)

	assert.True(t, isNet, "SymbolAnalysis network should return true")
	assert.False(t, isHigh, "network syscall without svc should not escalate to high risk")
}

// TestSyscallAnalysisHasNetworkSignal_UnknownArch verifies that an unknown architecture
// (mips) causes network detection to be skipped and returns false.
func TestSyscallAnalysisHasNetworkSignal_UnknownArch(t *testing.T) {
	result := &fileanalysis.SyscallAnalysisData{
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
	result := &fileanalysis.SyscallAnalysisData{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			Architecture: "x86_64",
			DetectedSyscalls: []common.SyscallInfo{
				{Number: -1},
			},
		},
	}
	assert.False(t, syscallAnalysisHasNetworkSignal(result, runtime.GOOS))
}

func TestSyscallAnalysisHasExecSignal(t *testing.T) {
	execveNum := platformFirstExecSyscallNum()
	arch, secondExecNum := platformExecSyscallNums()
	tests := []struct {
		name   string
		result *fileanalysis.SyscallAnalysisData
		want   bool
	}{
		{
			name: "nil result",
			want: false,
		},
		{
			name: "empty detected syscalls",
			result: &fileanalysis.SyscallAnalysisData{
				SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{Architecture: arch},
			},
			want: false,
		},
		{
			name: "execve detected",
			result: &fileanalysis.SyscallAnalysisData{
				SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
					Architecture: arch,
					DetectedSyscalls: []common.SyscallInfo{
						{Number: execveNum, Name: "execve"},
					},
				},
			},
			want: true,
		},
		{
			name: "second exec syscall detected",
			result: &fileanalysis.SyscallAnalysisData{
				SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
					Architecture: arch,
					DetectedSyscalls: []common.SyscallInfo{
						{Number: secondExecNum, Name: "execveat_or_mac_execve"},
					},
				},
			},
			want: true,
		},
		{
			name: "network syscall only",
			result: &fileanalysis.SyscallAnalysisData{
				SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
					Architecture: arch,
					DetectedSyscalls: []common.SyscallInfo{
						{Number: platformSocketNum(), Name: "socket"},
					},
				},
			},
			want: false,
		},
		{
			name: "non exec syscall only",
			result: &fileanalysis.SyscallAnalysisData{
				SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
					Architecture: arch,
					DetectedSyscalls: []common.SyscallInfo{
						{Number: 1, Name: "write"},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, syscallAnalysisHasExecSignal(tt.result, runtime.GOOS))
		})
	}
}

func TestSyscallAnalysisHasMprotectExecSignal(t *testing.T) {
	tests := []struct {
		name   string
		result *fileanalysis.SyscallAnalysisData
		want   bool
	}{
		{
			name: "nil result",
			want: false,
		},
		{
			name:   "empty arg eval results",
			result: &fileanalysis.SyscallAnalysisData{},
			want:   false,
		},
		{
			name: "mprotect exec confirmed",
			result: &fileanalysis.SyscallAnalysisData{
				SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
					ArgEvalResults: []common.SyscallArgEvalResult{{
						SyscallName: "mprotect",
						Status:      common.SyscallArgEvalExecConfirmed,
					}},
				},
			},
			want: true,
		},
		{
			name: "pkey_mprotect exec confirmed",
			result: &fileanalysis.SyscallAnalysisData{
				SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
					ArgEvalResults: []common.SyscallArgEvalResult{{
						SyscallName: "pkey_mprotect",
						Status:      common.SyscallArgEvalExecConfirmed,
					}},
				},
			},
			want: true,
		},
		{
			name: "mprotect exec unknown",
			result: &fileanalysis.SyscallAnalysisData{
				SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
					ArgEvalResults: []common.SyscallArgEvalResult{{
						SyscallName: "mprotect",
						Status:      common.SyscallArgEvalExecUnknown,
					}},
				},
			},
			want: true,
		},
		{
			name: "non mprotect family exec confirmed",
			result: &fileanalysis.SyscallAnalysisData{
				SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
					ArgEvalResults: []common.SyscallArgEvalResult{{
						SyscallName: "mmap",
						Status:      common.SyscallArgEvalExecConfirmed,
					}},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, syscallAnalysisHasMprotectExecSignal(tt.result))
		})
	}
}

func TestNetworkAnalyzer_MprotectExecConfirmedIsHighRisk(t *testing.T) {
	store := &stubRecordStore{record: &fileanalysis.Record{
		ContentHash: testContentHash,
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				ArgEvalResults: []common.SyscallArgEvalResult{{
					SyscallName: "mprotect",
					Status:      common.SyscallArgEvalExecConfirmed,
					Details:     "prot=0x5",
				}},
			},
		},
	}}
	a := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNetwork, isHighRisk, err := a.analyzeBinarySignals(testCmdPath, testContentHash)
	require.NoError(t, err)
	assert.False(t, isNetwork, "mprotect PROT_EXEC signal alone should not imply network")
	assert.True(t, isHighRisk, "mprotect PROT_EXEC signal should escalate to high risk")
}

func platformSocketNum() int {
	_, socketNum, _ := platformNetworkSyscallNums()
	return socketNum
}

// platformFirstExecSyscallNum returns the execve syscall number for the current OS/arch.
// macOS and x86_64 Linux use 59; arm64 Linux uses 221.
func platformFirstExecSyscallNum() int {
	if runtime.GOOS == "linux" && runtime.GOARCH == "arm64" {
		return 221
	}
	return 59
}

func TestNetworkAnalyzer_ExecSyscallIsHighRisk(t *testing.T) {
	execveNum := platformFirstExecSyscallNum()
	arch, _ := platformExecSyscallNums()
	_, socketNum, _ := platformNetworkSyscallNums()

	tests := []struct {
		name            string
		detectedSyscall []common.SyscallInfo
		wantNetwork     bool
		wantHighRisk    bool
	}{
		{
			name: "exec syscall only",
			detectedSyscall: []common.SyscallInfo{
				{Number: execveNum, Name: "execve"},
			},
			wantNetwork:  false,
			wantHighRisk: true,
		},
		{
			name: "exec and network syscall",
			detectedSyscall: []common.SyscallInfo{
				{Number: socketNum, Name: "socket"},
				{Number: execveNum, Name: "execve"},
			},
			wantNetwork:  true,
			wantHighRisk: true,
		},
		{
			name: "network syscall only",
			detectedSyscall: []common.SyscallInfo{
				{Number: socketNum, Name: "socket"},
			},
			wantNetwork:  true,
			wantHighRisk: false,
		},
		{
			name: "no exec syscall",
			detectedSyscall: []common.SyscallInfo{
				{Number: 1, Name: "write"},
			},
			wantNetwork:  false,
			wantHighRisk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubRecordStore{record: &fileanalysis.Record{
				ContentHash: testContentHash,
				SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
					SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
						Architecture:     arch,
						DetectedSyscalls: tt.detectedSyscall,
					},
				},
			}}
			a := newNetworkAnalyzerWithStore(runtime.GOOS, store)

			isNetwork, isHighRisk, err := a.analyzeBinarySignals(testCmdPath, testContentHash)
			require.NoError(t, err)
			assert.Equal(t, tt.wantNetwork, isNetwork)
			assert.Equal(t, tt.wantHighRisk, isHighRisk)
		})
	}
}

// TestHandleAnalysisOutput_DefaultIsFullyFailClosed verifies that an unknown
// AnalysisResult value (e.g. a future enum added without updating the switch)
// returns (true, true) — fully fail-closed — regardless of DynamicLoadSymbols.
func TestHandleAnalysisOutput_DefaultIsFullyFailClosed(t *testing.T) {
	output := binaryanalyzer.AnalysisOutput{
		Result:             binaryanalyzer.AnalysisResult(999),
		DynamicLoadSymbols: nil, // no dynamic load symbols — isHighRisk must still be true
	}

	isNetwork, isHighRisk := handleAnalysisOutput(output, testCmdPath)
	assert.True(t, isNetwork, "unknown AnalysisResult must be treated as network (fail-closed)")
	assert.True(t, isHighRisk, "unknown AnalysisResult must be treated as high risk (fail-closed)")
}
