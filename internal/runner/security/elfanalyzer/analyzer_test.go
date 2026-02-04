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
		skipIfMissing  bool
	}{
		{
			name:           "binary with socket symbols",
			filename:       "with_socket.elf",
			expectedResult: NetworkDetected,
			expectSymbols:  true,
			skipIfMissing:  true,
		},
		{
			name:           "binary with curl symbols",
			filename:       "with_curl.elf",
			expectedResult: NetworkDetected,
			expectSymbols:  true,
			skipIfMissing:  true,
		},
		{
			name:           "binary with ssl symbols",
			filename:       "with_ssl.elf",
			expectedResult: NetworkDetected,
			expectSymbols:  true,
			skipIfMissing:  true,
		},
		{
			name:           "binary without network symbols",
			filename:       "no_network.elf",
			expectedResult: NoNetworkSymbols,
			expectSymbols:  false,
			skipIfMissing:  true,
		},
		{
			name:           "static binary",
			filename:       "static.elf",
			expectedResult: StaticBinary,
			expectSymbols:  false,
			skipIfMissing:  true,
		},
		{
			name:           "shell script (non-ELF)",
			filename:       "script.sh",
			expectedResult: NotELFBinary,
			expectSymbols:  false,
			skipIfMissing:  true,
		},
		{
			name:           "corrupted ELF",
			filename:       "corrupted.elf",
			expectedResult: AnalysisError,
			expectSymbols:  false,
			skipIfMissing:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(testdataDir, tt.filename)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if tt.skipIfMissing {
					t.Skipf("test file %s not found", tt.filename)
				}
				t.Fatalf("required test file %s not found", tt.filename)
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

func TestAnalysisOutput_IsNetworkCapable(t *testing.T) {
	tests := []struct {
		result   AnalysisResult
		expected bool
	}{
		{NetworkDetected, true},
		{NoNetworkSymbols, false},
		{NotELFBinary, false},
		{StaticBinary, false},
		{AnalysisError, true}, // Errors are treated as potential network for safety
	}

	for _, tt := range tests {
		t.Run(tt.result.String(), func(t *testing.T) {
			output := AnalysisOutput{Result: tt.result}
			assert.Equal(t, tt.expected, output.IsNetworkCapable())
		})
	}
}

func TestAnalysisOutput_IsIndeterminate(t *testing.T) {
	tests := []struct {
		result   AnalysisResult
		expected bool
	}{
		{NetworkDetected, false},
		{NoNetworkSymbols, false},
		{NotELFBinary, false},
		{StaticBinary, true},
		{AnalysisError, true},
	}

	for _, tt := range tests {
		t.Run(tt.result.String(), func(t *testing.T) {
			output := AnalysisOutput{Result: tt.result}
			assert.Equal(t, tt.expected, output.IsIndeterminate())
		})
	}
}

func TestAnalysisResult_String(t *testing.T) {
	tests := []struct {
		result   AnalysisResult
		expected string
	}{
		{NetworkDetected, "network_detected"},
		{NoNetworkSymbols, "no_network_symbols"},
		{NotELFBinary, "not_elf_binary"},
		{StaticBinary, "static_binary"},
		{AnalysisError, "analysis_error"},
		{AnalysisResult(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.String())
		})
	}
}

func TestIsNetworkSymbol(t *testing.T) {
	tests := []struct {
		symbol   string
		expected bool
		category SymbolCategory
	}{
		{"socket", true, CategorySocket},
		{"connect", true, CategorySocket},
		{"curl_easy_init", true, CategoryHTTP},
		{"SSL_connect", true, CategoryTLS},
		{"getaddrinfo", true, CategoryDNS},
		{"printf", false, ""},
		{"malloc", false, ""},
		{"unknown_function", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			cat, found := IsNetworkSymbol(tt.symbol)
			assert.Equal(t, tt.expected, found)
			if tt.expected {
				assert.Equal(t, tt.category, cat)
			}
		})
	}
}

func TestSymbolCount(t *testing.T) {
	count := SymbolCount()
	// Ensure we have a reasonable number of symbols registered
	assert.Greater(t, count, 30, "expected at least 30 registered symbols")
}

func TestGetNetworkSymbols_ReturnsCopy(t *testing.T) {
	symbols := GetNetworkSymbols()
	originalCount := len(symbols)

	// Modify the returned map
	symbols["test_symbol"] = "test"

	// Verify the original registry is not modified
	assert.Equal(t, originalCount, SymbolCount(),
		"GetNetworkSymbols should return a copy, not the original")
}

func TestIsELFMagic(t *testing.T) {
	tests := []struct {
		name     string
		magic    []byte
		expected bool
	}{
		{"valid ELF magic", []byte{0x7f, 'E', 'L', 'F'}, true},
		{"invalid magic", []byte{0x00, 0x00, 0x00, 0x00}, false},
		{"short input", []byte{0x7f, 'E'}, false},
		{"empty input", []byte{}, false},
		{"shell script shebang", []byte{'#', '!', '/', 'b'}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isELFMagic(tt.magic))
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substrs  []string
		expected bool
	}{
		{"matches first", "no symbol section", []string{"no symbol", "missing"}, true},
		{"matches second", "missing section", []string{"no symbol", "missing"}, true},
		{"no match", "valid section", []string{"no symbol", "missing"}, false},
		{"empty string", "", []string{"no symbol"}, false},
		{"empty substrs", "some text", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, containsAny(tt.s, tt.substrs...))
		})
	}
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
