//go:build test

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
			expectSymbols:  false,
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

// TestCheckDynamicSymbols_HasDynamicLoad verifies that checkDynamicSymbols
// correctly populates DynamicLoadSymbols when dlopen/dlsym/dlvsym appear in the symbol list.
func TestCheckDynamicSymbols_HasDynamicLoad(t *testing.T) {
	analyzer := NewStandardELFAnalyzer(nil, nil)

	tests := []struct {
		name                    string
		symbols                 []elf.Symbol
		wantResult              binaryanalyzer.AnalysisResult
		wantDynamicLoadSymNames []string
	}{
		{
			name: "dlopen only",
			symbols: []elf.Symbol{
				{Name: "dlopen", Section: elf.SHN_UNDEF},
			},
			wantResult:              binaryanalyzer.NoNetworkSymbols,
			wantDynamicLoadSymNames: []string{"dlopen"},
		},
		{
			name: "dlsym only",
			symbols: []elf.Symbol{
				{Name: "dlsym", Section: elf.SHN_UNDEF},
			},
			wantResult:              binaryanalyzer.NoNetworkSymbols,
			wantDynamicLoadSymNames: []string{"dlsym"},
		},
		{
			name: "dlvsym only",
			symbols: []elf.Symbol{
				{Name: "dlvsym", Section: elf.SHN_UNDEF},
			},
			wantResult:              binaryanalyzer.NoNetworkSymbols,
			wantDynamicLoadSymNames: []string{"dlvsym"},
		},
		{
			name: "dlopen and socket (both signals)",
			symbols: []elf.Symbol{
				{Name: "dlopen", Section: elf.SHN_UNDEF},
				{Name: "socket", Section: elf.SHN_UNDEF},
			},
			wantResult:              binaryanalyzer.NetworkDetected,
			wantDynamicLoadSymNames: []string{"dlopen"},
		},
		{
			name: "socket only (no dynamic load)",
			symbols: []elf.Symbol{
				{Name: "socket", Section: elf.SHN_UNDEF},
			},
			wantResult:              binaryanalyzer.NetworkDetected,
			wantDynamicLoadSymNames: nil,
		},
		{
			name: "no relevant symbols",
			symbols: []elf.Symbol{
				{Name: "printf", Section: elf.SHN_UNDEF},
			},
			wantResult:              binaryanalyzer.NoNetworkSymbols,
			wantDynamicLoadSymNames: nil,
		},
		{
			name: "dlopen defined (not imported, SHN_UNDEF=0)",
			symbols: []elf.Symbol{
				// Section != SHN_UNDEF means it's defined, not imported
				{Name: "dlopen", Section: elf.SHN_ABS},
			},
			wantResult:              binaryanalyzer.NoNetworkSymbols,
			wantDynamicLoadSymNames: nil,
		},
		{
			name: "dlsym and dlvsym both present",
			symbols: []elf.Symbol{
				{Name: "dlsym", Section: elf.SHN_UNDEF},
				{Name: "dlvsym", Section: elf.SHN_UNDEF},
			},
			wantResult:              binaryanalyzer.NoNetworkSymbols,
			wantDynamicLoadSymNames: []string{"dlsym", "dlvsym"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := analyzer.checkDynamicSymbols(tt.symbols)
			assert.Equal(t, tt.wantResult, output.Result)
			var gotNames []string
			for _, sym := range output.DynamicLoadSymbols {
				gotNames = append(gotNames, sym.Name)
				// All DynamicLoadSymbols must have category "dynamic_load"
				assert.Equal(t, "dynamic_load", sym.Category,
					"DynamicLoadSymbol %q should have category dynamic_load", sym.Name)
			}
			assert.Equal(t, tt.wantDynamicLoadSymNames, gotNames)
		})
	}
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
				Summary: SyscallSummary{
					HasNetworkSyscalls:  true,
					NetworkSyscallCount: 2,
					TotalDetectedEvents: 2,
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
				Summary: SyscallSummary{
					HasNetworkSyscalls:  false,
					NetworkSyscallCount: 0,
					TotalDetectedEvents: 1,
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
				HasUnknownSyscalls: true,
				HighRiskReasons: []string{
					"syscall at 0x401000: number could not be determined (unknown:indirect_setting)",
				},
				Summary: SyscallSummary{
					HasNetworkSyscalls:  false,
					IsHighRisk:          true,
					TotalDetectedEvents: 1,
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
	// IsHighRisk must win: incomplete analysis makes the result unreliable regardless of
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
				HasUnknownSyscalls: true,
				HighRiskReasons: []string{
					"syscall at 0x401010: number could not be determined (unknown:indirect_setting)",
				},
				Summary: SyscallSummary{
					HasNetworkSyscalls:  true,
					NetworkSyscallCount: 1,
					IsHighRisk:          true,
					TotalDetectedEvents: 2,
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	// IsHighRisk must take precedence over HasNetworkSyscalls
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
				Summary: SyscallSummary{HasNetworkSyscalls: true},
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
