//go:build test

package elfanalyzer

import (
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
