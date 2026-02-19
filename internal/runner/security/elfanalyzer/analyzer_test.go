//go:build test

package elfanalyzer

import (
	"debug/elf"
	"os"
	"path/filepath"
	"testing"

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
		expectedResult AnalysisResult
		expectSymbols  bool
	}{
		{
			name:           "binary with socket symbols",
			filename:       "with_socket.elf",
			expectedResult: NetworkDetected,
			expectSymbols:  true,
		},
		{
			name:           "binary with ssl symbols",
			filename:       "with_ssl.elf",
			expectedResult: NetworkDetected,
			expectSymbols:  true,
		},
		{
			name:           "binary without network symbols",
			filename:       "no_network.elf",
			expectedResult: NoNetworkSymbols,
			expectSymbols:  false,
		},
		{
			name:           "static binary",
			filename:       "static.elf",
			expectedResult: StaticBinary,
			expectSymbols:  false,
		},
		{
			name:           "shell script (non-ELF)",
			filename:       "script.sh",
			expectedResult: NotELFBinary,
			expectSymbols:  false,
		},
		{
			name:           "corrupted ELF",
			filename:       "corrupted.elf",
			expectedResult: AnalysisError,
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

			output := analyzer.AnalyzeNetworkSymbols(absPath)
			assert.Equal(t, tt.expectedResult, output.Result,
				"unexpected result for %s", tt.filename)

			if tt.expectSymbols {
				assert.NotEmpty(t, output.DetectedSymbols,
					"expected symbols for %s", tt.filename)
			} else {
				assert.Empty(t, output.DetectedSymbols,
					"unexpected symbols for %s", tt.filename)
			}

			if tt.expectedResult == AnalysisError {
				assert.NotNil(t, output.Error,
					"expected error for %s", tt.filename)
			}
		})
	}
}

func TestStandardELFAnalyzer_NonexistentFile(t *testing.T) {
	analyzer := NewStandardELFAnalyzer(nil, nil)

	output := analyzer.AnalyzeNetworkSymbols("/nonexistent/path/to/binary")

	assert.Equal(t, AnalysisError, output.Result)
	assert.NotNil(t, output.Error)
}

func TestStandardELFAnalyzer_WithCustomSymbols(t *testing.T) {
	// Create analyzer with a minimal custom symbol set
	customSymbols := map[string]SymbolCategory{
		"my_network_func": CategorySocket,
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

	output := analyzer.AnalyzeNetworkSymbols(absPath)
	// with_socket.elf has "socket" and "connect", but our custom set only has "my_network_func"
	assert.Equal(t, NoNetworkSymbols, output.Result,
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
		return nil, ErrHashMismatch
	}
	return m.result, m.err
}

// createStaticELFFile creates a minimal static ELF file for testing.
// The file has no .dynsym section, simulating a statically linked binary.
func createStaticELFFile(t *testing.T, path string) {
	t.Helper()

	// Create a minimal ELF header for x86_64
	// This is a valid ELF header that will parse but has no .dynsym section
	elfHeader := []byte{
		// ELF magic
		0x7f, 'E', 'L', 'F',
		// Class: 64-bit
		0x02,
		// Data: little endian
		0x01,
		// Version
		0x01,
		// OS/ABI: System V
		0x00,
		// ABI version
		0x00,
		// Padding
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Type: Executable
		0x02, 0x00,
		// Machine: x86_64
		0x3e, 0x00,
		// Version
		0x01, 0x00, 0x00, 0x00,
		// Entry point
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Program header offset
		0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Section header offset (0 = none)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Flags
		0x00, 0x00, 0x00, 0x00,
		// ELF header size
		0x40, 0x00,
		// Program header size
		0x38, 0x00,
		// Number of program headers
		0x00, 0x00,
		// Section header size
		0x40, 0x00,
		// Number of section headers
		0x00, 0x00,
		// Section name string table index
		0x00, 0x00,
	}

	err := os.WriteFile(path, elfHeader, 0o644)
	require.NoError(t, err)

	// Verify it can be parsed as ELF
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	_, err = elf.NewFile(f)
	require.NoError(t, err)
}

func TestStandardELFAnalyzer_SyscallLookup_NetworkDetected(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "static.elf")
	createStaticELFFile(t, testFile)

	// Create mock store that returns network syscall result
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
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
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile)

	assert.Equal(t, NetworkDetected, output.Result)
	assert.Len(t, output.DetectedSymbols, 2)
	assert.Equal(t, "socket", output.DetectedSymbols[0].Name)
	assert.Equal(t, "syscall", output.DetectedSymbols[0].Category)
}

func TestStandardELFAnalyzer_SyscallLookup_NoNetwork(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "static.elf")
	createStaticELFFile(t, testFile)

	// Create mock store that returns non-network syscall result
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
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
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile)

	assert.Equal(t, NoNetworkSymbols, output.Result)
	assert.Empty(t, output.DetectedSymbols)
}

func TestStandardELFAnalyzer_SyscallLookup_HighRisk(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "static.elf")
	createStaticELFFile(t, testFile)

	// Create mock store that returns high-risk result (unknown syscalls)
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
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
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile)

	assert.Equal(t, AnalysisError, output.Result)
	assert.NotNil(t, output.Error)
	assert.Contains(t, output.Error.Error(), "high risk")
}

func TestStandardELFAnalyzer_SyscallLookup_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "static.elf")
	createStaticELFFile(t, testFile)

	// Create mock store that returns not found
	mockStore := &mockSyscallAnalysisStore{
		err: ErrRecordNotFound,
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile)

	// Should fallback to StaticBinary when no analysis found
	assert.Equal(t, StaticBinary, output.Result)
}

func TestStandardELFAnalyzer_SyscallLookup_HashMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "static.elf")
	createStaticELFFile(t, testFile)

	// Create mock store that expects a specific hash
	mockStore := &mockSyscallAnalysisStore{
		result: &SyscallAnalysisResult{
			DetectedSyscalls: []SyscallInfo{
				{Number: 41, Name: "socket", IsNetwork: true},
			},
			Summary: SyscallSummary{HasNetworkSyscalls: true},
		},
		expectedHash: "sha256:differenthash", // This won't match the actual file hash
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, mockStore)
	output := analyzer.AnalyzeNetworkSymbols(testFile)

	// Should fallback to StaticBinary when hash doesn't match
	assert.Equal(t, StaticBinary, output.Result)
}

func TestStandardELFAnalyzer_WithoutSyscallStore(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "static.elf")
	createStaticELFFile(t, testFile)

	// Create analyzer without syscall store (nil)
	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil, nil)
	output := analyzer.AnalyzeNetworkSymbols(testFile)

	// Should behave like regular analyzer - return StaticBinary for static ELF
	assert.Equal(t, StaticBinary, output.Result)
}
