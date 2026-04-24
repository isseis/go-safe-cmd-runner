//go:build test && linux

package elfanalyzer

import (
	"debug/elf"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	elfanalyzertesting "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandardELFAnalyzer_AnalyzeNetworkSymbols(t *testing.T) {
	// Skip if test fixtures don't exist
	testdataDir := "testdata"
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata directory not found, skipping ELF analysis tests")
	}

	analyzer := NewStandardELFAnalyzer(nil, nil)

	tests := []struct {
		name           string
		filename       string
		expectedResult binaryanalyzer.AnalysisResult
		expectSymbols  bool
	}{
		{
			name:           "binary with socket symbols",
			filename:       "with_socket.elf",
			expectedResult: binaryanalyzer.NetworkDetected,
			expectSymbols:  true,
		},
		{
			name:           "binary with ssl symbols",
			filename:       "with_ssl.elf",
			expectedResult: binaryanalyzer.NetworkDetected,
			expectSymbols:  true,
		},
		{
			name:           "binary without network symbols",
			filename:       "no_network.elf",
			expectedResult: binaryanalyzer.NoNetworkSymbols,
			expectSymbols:  true, // libc symbols are recorded with "syscall_wrapper" category
		},
		{
			name:           "static binary",
			filename:       "static.elf",
			expectedResult: binaryanalyzer.StaticBinary,
			expectSymbols:  false,
		},
		{
			name:           "shell script (non-ELF)",
			filename:       "script.sh",
			expectedResult: binaryanalyzer.NotSupportedBinary,
			expectSymbols:  false,
		},
		{
			name:           "corrupted ELF",
			filename:       "corrupted.elf",
			expectedResult: binaryanalyzer.AnalysisError,
			expectSymbols:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(testdataDir, tt.filename)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("test file %s not found", tt.filename)
			}

			absPath, err := filepath.Abs(path)
			require.NoError(t, err)

			output := analyzer.AnalyzeNetworkSymbols(absPath, "sha256:dummy")
			assert.Equal(t, tt.expectedResult, output.Result,
				"unexpected result for %s", tt.filename)

			if tt.expectSymbols {
				assert.NotEmpty(t, output.DetectedSymbols,
					"expected symbols for %s", tt.filename)
			} else {
				assert.Empty(t, output.DetectedSymbols,
					"unexpected symbols for %s", tt.filename)
			}

			if tt.expectedResult == binaryanalyzer.AnalysisError {
				assert.NotNil(t, output.Error,
					"expected error for %s", tt.filename)
			}
		})
	}
}

func TestStandardELFAnalyzer_NonexistentFile(t *testing.T) {
	analyzer := NewStandardELFAnalyzer(nil, nil)

	output := analyzer.AnalyzeNetworkSymbols("/nonexistent/path/to/binary", "sha256:dummy")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	assert.NotNil(t, output.Error)
}

// TestHasDynamicLoad_ELF verifies that a binary importing dlopen is detected
// with non-empty DynamicLoadSymbols, independently of network symbol detection.
func TestHasDynamicLoad_ELF(t *testing.T) {
	analyzer := NewStandardELFAnalyzer(nil, nil)

	// Use a real system binary that is known to import dlopen.
	// python3 uses dlopen for extension loading on Linux.
	// Resolve symlinks: safefileio rejects symlinks, so we need the canonical path.
	candidates := []string{
		"/usr/bin/python3",
		"/usr/bin/python3.11",
		"/usr/bin/python3.12",
		"/usr/bin/python3.10",
	}
	var binaryPath string
	for _, c := range candidates {
		resolved, err := filepath.EvalSymlinks(c)
		if err == nil {
			binaryPath = resolved
			break
		}
	}
	if binaryPath == "" {
		t.Skip("no python3 binary found; skipping HasDynamicLoad_ELF test")
	}

	output := analyzer.AnalyzeNetworkSymbols(binaryPath, "sha256:dummy")
	// python3 uses dlopen for extension loading.
	assert.True(t, len(output.DynamicLoadSymbols) > 0,
		"python3 is expected to import dlopen/dlsym, got no DynamicLoadSymbols")
}

// TestBuildDetectedSymbols verifies the libc-filter logic and DynamicLoadSymbols
// collection in buildDetectedSymbols. AC-1, AC-2.
func TestBuildDetectedSymbols(t *testing.T) {
	networkSymbols := binaryanalyzer.GetNetworkSymbols()

	tests := []struct {
		name                    string
		symbols                 []elf.Symbol
		hasVERNEED              bool
		libcInNeeded            bool
		wantDetectedNames       []string
		wantDetectedCategories  []string
		wantDynamicLoadSymNames []string
	}{
		{
			// VERNEED path: socket from libc.so.6 → detected with "socket" category (AC-1)
			name: "VERNEED: socket from libc detected",
			symbols: []elf.Symbol{
				{Name: "socket", Section: elf.SHN_UNDEF, Library: "libc.so.6"},
			},
			hasVERNEED:             true,
			wantDetectedNames:      []string{"socket"},
			wantDetectedCategories: []string{"socket"},
		},
		{
			// VERNEED path: read from libc → syscall_wrapper category (AC-1)
			name: "VERNEED: read from libc is syscall_wrapper",
			symbols: []elf.Symbol{
				{Name: "read", Section: elf.SHN_UNDEF, Library: "libc.so.6"},
			},
			hasVERNEED:             true,
			wantDetectedNames:      []string{"read"},
			wantDetectedCategories: []string{"syscall_wrapper"},
		},
		{
			// VERNEED path: socket + read from libc (AC-1)
			name: "VERNEED: socket and read from libc",
			symbols: []elf.Symbol{
				{Name: "socket", Section: elf.SHN_UNDEF, Library: "libc.so.6"},
				{Name: "read", Section: elf.SHN_UNDEF, Library: "libc.so.6"},
			},
			hasVERNEED:             true,
			wantDetectedNames:      []string{"socket", "read"},
			wantDetectedCategories: []string{"socket", "syscall_wrapper"},
		},
		{
			// VERNEED path: non-libc library symbol is not recorded (AC-2)
			name: "VERNEED: non-libc symbol is not detected",
			symbols: []elf.Symbol{
				{Name: "socket", Section: elf.SHN_UNDEF, Library: "libfoo.so.1"},
			},
			hasVERNEED:        true,
			wantDetectedNames: nil,
		},
		{
			// VERNEED path: musl libc is recognized (AC-1 variant)
			name: "VERNEED: musl libc socket is detected",
			symbols: []elf.Symbol{
				{Name: "socket", Section: elf.SHN_UNDEF, Library: "libc.musl-x86_64.so.1"},
			},
			hasVERNEED:             true,
			wantDetectedNames:      []string{"socket"},
			wantDetectedCategories: []string{"socket"},
		},
		{
			// Fallback path: no VERNEED, libc in DT_NEEDED, STT_FUNC → detected
			name: "fallback: STT_FUNC socket detected when libc in DT_NEEDED",
			symbols: []elf.Symbol{
				{
					Name:    "socket",
					Section: elf.SHN_UNDEF,
					Info:    uint8(elf.STT_FUNC) | uint8(elf.STB_GLOBAL<<4),
				},
			},
			libcInNeeded:           true,
			wantDetectedNames:      []string{"socket"},
			wantDetectedCategories: []string{"socket"},
		},
		{
			// Fallback path: no VERNEED, no libc in DT_NEEDED → socket not detected (AC-2 variant)
			name: "fallback: socket not detected when libc not in DT_NEEDED",
			symbols: []elf.Symbol{
				{
					Name:    "socket",
					Section: elf.SHN_UNDEF,
					Info:    uint8(elf.STT_FUNC) | uint8(elf.STB_GLOBAL<<4),
				},
			},
			libcInNeeded:      false,
			wantDetectedNames: nil,
		},
		{
			// dlopen is always collected independently of libc filter
			name: "dlopen is collected regardless of libc filter",
			symbols: []elf.Symbol{
				{Name: "dlopen", Section: elf.SHN_UNDEF},
			},
			hasVERNEED:              false,
			libcInNeeded:            false,
			wantDetectedNames:       nil,
			wantDynamicLoadSymNames: []string{"dlopen"},
		},
		{
			// dlopen + socket from libc: both signals captured independently
			name: "VERNEED: dlopen and socket from libc",
			symbols: []elf.Symbol{
				{Name: "dlopen", Section: elf.SHN_UNDEF, Library: "libdl.so.2"},
				{Name: "socket", Section: elf.SHN_UNDEF, Library: "libc.so.6"},
			},
			hasVERNEED:              true,
			wantDetectedNames:       []string{"socket"},
			wantDetectedCategories:  []string{"socket"},
			wantDynamicLoadSymNames: []string{"dlopen"},
		},
		{
			// Defined symbols (SHN_ABS etc.) are skipped
			name: "defined dlopen is not collected",
			symbols: []elf.Symbol{
				{Name: "dlopen", Section: elf.SHN_ABS},
			},
			wantDynamicLoadSymNames: nil,
		},
		{
			// dlsym and dlvsym
			name: "dlsym and dlvsym both collected",
			symbols: []elf.Symbol{
				{Name: "dlsym", Section: elf.SHN_UNDEF},
				{Name: "dlvsym", Section: elf.SHN_UNDEF},
			},
			wantDynamicLoadSymNames: []string{"dlsym", "dlvsym"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected, dynamicLoadSyms := buildDetectedSymbols(
				tt.symbols, tt.hasVERNEED, tt.libcInNeeded, networkSymbols,
			)

			var gotNames []string
			for _, sym := range detected {
				gotNames = append(gotNames, sym.Name)
			}
			assert.Equal(t, tt.wantDetectedNames, gotNames, "detected symbol names")

			if len(tt.wantDetectedCategories) > 0 {
				var gotCats []string
				for _, sym := range detected {
					gotCats = append(gotCats, sym.Category)
				}
				assert.Equal(t, tt.wantDetectedCategories, gotCats, "detected symbol categories")
			}

			var gotDLNames []string
			for _, sym := range dynamicLoadSyms {
				gotDLNames = append(gotDLNames, sym.Name)
				assert.Equal(t, "dynamic_load", sym.Category,
					"DynamicLoadSymbol %q should have category dynamic_load", sym.Name)
			}
			assert.Equal(t, tt.wantDynamicLoadSymNames, gotDLNames, "dynamic load symbol names")
		})
	}
}

// TestAnalyzeNetworkSymbols_DetectsLibcSocketAndSyscallWrapper verifies AC-1:
// socket and read from libc appear in DetectedSymbols with correct categories.
// This test requires testdata/with_socket.elf; skipped if not present.
func TestAnalyzeNetworkSymbols_DetectsLibcSocketAndSyscallWrapper(t *testing.T) {
	testdataDir := "testdata"
	path := filepath.Join(testdataDir, "with_socket.elf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("testdata/with_socket.elf not found; generate per testdata/README.md")
	}

	absPath, err := filepath.Abs(path)
	require.NoError(t, err)

	analyzer := NewStandardELFAnalyzer(nil, nil)
	output := analyzer.AnalyzeNetworkSymbols(absPath, "sha256:dummy")
	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)

	// socket must be detected with "socket" category
	socketFound := false
	for _, sym := range output.DetectedSymbols {
		if sym.Name == "socket" {
			assert.Equal(t, "socket", sym.Category, "socket should have category 'socket'")
			socketFound = true
		}
		// All libc symbols that are not network-related should be "syscall_wrapper"
		if sym.Name != "socket" && sym.Name != "connect" && sym.Name != "bind" &&
			sym.Name != "listen" && sym.Name != "accept" && sym.Name != "getaddrinfo" {
			assert.Equal(t, "syscall_wrapper", sym.Category,
				"non-network symbol %q should be 'syscall_wrapper'", sym.Name)
		}
	}
	assert.True(t, socketFound, "socket should appear in DetectedSymbols")
}

// TestAnalyzeNetworkSymbols_IgnoresNonLibcSymbols verifies AC-2:
// symbols from libraries other than libc are not recorded.
// This test uses buildDetectedSymbols directly to avoid fixture dependency.
func TestAnalyzeNetworkSymbols_IgnoresNonLibcSymbols(t *testing.T) {
	networkSymbols := binaryanalyzer.GetNetworkSymbols()

	// Simulate an ELF that imports only from libfoo (not libc)
	symbols := []elf.Symbol{
		{Name: "socket", Section: elf.SHN_UNDEF, Library: "libfoo.so.1"},
		{Name: "connect", Section: elf.SHN_UNDEF, Library: "libfoo.so.1"},
		{Name: "read", Section: elf.SHN_UNDEF, Library: "libfoo.so.1"},
	}

	detected, _ := buildDetectedSymbols(symbols, true, false, networkSymbols)
	assert.Empty(t, detected, "symbols from non-libc library should not be recorded")
}

func TestStandardELFAnalyzer_WithCustomSymbols(t *testing.T) {
	// Create analyzer with a minimal custom symbol set
	customSymbols := map[string]binaryanalyzer.SymbolCategory{
		"my_network_func": binaryanalyzer.CategorySocket,
	}
	analyzer := NewStandardELFAnalyzerWithSymbols(nil, nil, customSymbols)

	// Test with a real binary that has network symbols not in our custom set
	testdataDir := "testdata"
	path := filepath.Join(testdataDir, "with_socket.elf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("test file with_socket.elf not found")
	}

	absPath, err := filepath.Abs(path)
	require.NoError(t, err)

	output := analyzer.AnalyzeNetworkSymbols(absPath, "sha256:dummy")
	// with_socket.elf has "socket" and "connect", but our custom set only has "my_network_func"
	assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result,
		"custom symbols should not match standard socket symbols")
}

// mockSyscallAnalysisStore is a mock implementation of SyscallAnalysisStore for testing.
type mockSyscallAnalysisStore struct {
	result *SyscallAnalysisResult
	err    error
	// expectedHash is used to verify hash matching behavior.
	// When set, returns ErrHashMismatch if the provided hash does not match.
	expectedHash string
}

func (m *mockSyscallAnalysisStore) LoadSyscallAnalysis(_ string, expectedHash string) (*SyscallAnalysisResult, error) {
	// If expectedHash is set, only return result when hash matches
	if m.expectedHash != "" && m.expectedHash != expectedHash {
		return nil, fileanalysis.ErrHashMismatch
	}
	return m.result, m.err
}

func TestStandardELFAnalyzer_SyscallLookup_NetworkDetected(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "static.elf")
	elfanalyzertesting.CreateStaticELFFile(t, testFile)

	// Create mock store that returns network syscall result
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []SyscallInfo{
					{
						Number:    41, // socket
						Name:      "socket",
						IsNetwork: true,
						Location:  0x401000,
					},
					{
						Number:    42, // connect
						Name:      "connect",
						IsNetwork: true,
						Location:  0x401010,
					},
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
	assert.Len(t, output.DetectedSymbols, 2)
	assert.Equal(t, "socket", output.DetectedSymbols[0].Name)
	assert.Equal(t, "syscall", output.DetectedSymbols[0].Category)
}

func TestStandardELFAnalyzer_SyscallLookup_NoNetwork(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "static.elf")
	elfanalyzertesting.CreateStaticELFFile(t, testFile)

	// Create mock store that returns non-network syscall result
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []SyscallInfo{
					{
						Number:    1, // write
						Name:      "write",
						IsNetwork: false,
						Location:  0x401000,
					},
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
	assert.Empty(t, output.DetectedSymbols)
}

func TestStandardELFAnalyzer_SyscallLookup_HighRisk(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "static.elf")
	elfanalyzertesting.CreateStaticELFFile(t, testFile)

	// Create mock store that returns high-risk result (unknown syscalls)
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []SyscallInfo{
					{
						Number:              -1,
						DeterminationMethod: "unknown:indirect_setting",
						Location:            0x401000,
					},
				},
				AnalysisWarnings: []string{
					"syscall at 0x401000: number could not be determined (unknown:indirect_setting)",
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	assert.NotNil(t, output.Error)
	assert.Contains(t, output.Error.Error(), "high risk")
}

func TestStandardELFAnalyzer_SyscallLookup_HighRiskTakesPrecedenceOverNetwork(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "static.elf")
	elfanalyzertesting.CreateStaticELFFile(t, testFile)

	// Create mock store that returns both network syscalls and high-risk (unknown syscalls).
	// Risk must win: incomplete analysis makes the result unreliable regardless of
	// what network activity was detected.
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []SyscallInfo{
					{
						Number:    41, // socket
						Name:      "socket",
						IsNetwork: true,
						Location:  0x401000,
					},
					{
						Number:              -1,
						DeterminationMethod: "unknown:indirect_setting",
						Location:            0x401010,
					},
				},
				AnalysisWarnings: []string{
					"syscall at 0x401010: number could not be determined (unknown:indirect_setting)",
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	// Risk must take precedence over HasNetworkSyscalls
	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	assert.NotNil(t, output.Error)
	assert.ErrorIs(t, output.Error, ErrSyscallAnalysisHighRisk)
}

func TestStandardELFAnalyzer_SyscallLookup_NotFound(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "static.elf")
	elfanalyzertesting.CreateStaticELFFile(t, testFile)

	// Create mock store that returns not found
	mockStore := &mockSyscallAnalysisStore{
		err: fileanalysis.ErrRecordNotFound,
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	// Should fallback to StaticBinary when no analysis found
	assert.Equal(t, binaryanalyzer.StaticBinary, output.Result)
}

func TestStandardELFAnalyzer_SyscallLookup_HashMismatch(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "static.elf")
	elfanalyzertesting.CreateStaticELFFile(t, testFile)

	// Create mock store that expects a specific hash
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []SyscallInfo{
					{Number: 41, Name: "socket", IsNetwork: true},
				},
			},
		},
		expectedHash: "sha256:differenthash", // This won't match the actual file hash
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	// Hash mismatch means the binary has changed since record time: treat as AnalysisError.
	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	assert.ErrorIs(t, output.Error, ErrSyscallHashMismatch)
}

func TestStandardELFAnalyzer_WithoutSyscallStore(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "static.elf")
	elfanalyzertesting.CreateStaticELFFile(t, testFile)

	// Create analyzer without syscall store (nil)
	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, nil)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	// Should behave like regular analyzer - return StaticBinary for static ELF
	assert.Equal(t, binaryanalyzer.StaticBinary, output.Result)
}

// SyscallAnalysis fallback tests for dynamic ELF binaries.
// These tests cover the case where .dynsym returns NoNetworkSymbols and the
// analyzer falls back to the syscall analysis store.

// TestDynamicELF_SyscallFallback_NetworkDetected verifies that
// when .dynsym returns NoNetworkSymbols but SyscallAnalysis records HasNetworkSyscalls=true,
// AnalyzeNetworkSymbols returns NetworkDetected.
func TestDynamicELF_SyscallFallback_NetworkDetected(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "dynamic.elf")
	elfanalyzertesting.CreateDynamicELFFile(t, testFile)

	// Store returns HasNetworkSyscalls=true (simulates CGO binary with socket syscall)
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []SyscallInfo{
					{Number: 41, Name: "socket", IsNetwork: true, Location: 0x401000},
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
	require.Len(t, output.DetectedSymbols, 1)
	assert.Equal(t, "socket", output.DetectedSymbols[0].Name)
	assert.Equal(t, "syscall", output.DetectedSymbols[0].Category)
}

// TestDynamicELF_SyscallFallback_NotRecorded verifies that
// when .dynsym returns NoNetworkSymbols and SyscallAnalysis is not recorded
// (ErrRecordNotFound or (nil, nil)), AnalyzeNetworkSymbols returns NoNetworkSymbols.
func TestDynamicELF_SyscallFallback_NotRecorded(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "dynamic.elf")
	elfanalyzertesting.CreateDynamicELFFile(t, testFile)

	tests := []struct {
		name   string
		result *SyscallAnalysisResult
		err    error
	}{
		{"ErrRecordNotFound", nil, fileanalysis.ErrRecordNotFound},
		{"nil result (analyzed, none detected)", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockSyscallAnalysisStore{result: tt.result, err: tt.err}
			analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
			output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

			// Should remain NoNetworkSymbols (dynsym result) when no store entry
			assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
		})
	}
}

// TestDynamicELF_SyscallFallback_HashMismatch verifies that
// when .dynsym returns NoNetworkSymbols but SyscallAnalysis returns ErrHashMismatch
// (binary changed since record), AnalyzeNetworkSymbols returns AnalysisError.
func TestDynamicELF_SyscallFallback_HashMismatch(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "dynamic.elf")
	elfanalyzertesting.CreateDynamicELFFile(t, testFile)

	mockStore := &mockSyscallAnalysisStore{
		result:       &SyscallAnalysisResult{},
		expectedHash: "sha256:differenthash", // won't match "sha256:dummy"
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	assert.ErrorIs(t, output.Error, ErrSyscallHashMismatch)
}

// TestDynamicELF_SyscallFallback_HighRisk verifies that
// when .dynsym returns NoNetworkSymbols but SyscallAnalysis returns AnalysisError,
// AnalyzeNetworkSymbols returns AnalysisError.
func TestDynamicELF_SyscallFallback_HighRisk(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "dynamic.elf")
	elfanalyzertesting.CreateDynamicELFFile(t, testFile)

	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []SyscallInfo{
					{Number: -1, DeterminationMethod: "unknown:indirect_setting", Location: 0x401000},
				},
				AnalysisWarnings: []string{"syscall at 0x401000: number could not be determined (unknown:indirect_setting)"},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	assert.ErrorIs(t, output.Error, ErrSyscallAnalysisHighRisk)
}

// TestDynamicELF_WithoutSyscallStore verifies that
// when syscallStore is nil, dynamic ELF with NoNetworkSymbols in .dynsym
// returns NoNetworkSymbols (no fallback attempted).
func TestDynamicELF_WithoutSyscallStore(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	testFile := filepath.Join(tmpDir, "dynamic.elf")
	elfanalyzertesting.CreateDynamicELFFile(t, testFile)

	// No syscall store: fallback disabled
	analyzer := NewStandardELFAnalyzer(nil, nil)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
}
