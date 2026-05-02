//go:build test && linux

package elfanalyzer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	elfanalyzertesting "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandardELFAnalyzer_AnalyzeNetworkSymbols(t *testing.T) {
	// Skip if test fixtures don't exist
	testdataDir := "testdata"
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata directory not found, skipping ELF analysis tests")
	}

	analyzer := NewStandardELFAnalyzer(nil)

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
			expectSymbols:  true,
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
	analyzer := NewStandardELFAnalyzer(nil)

	output := analyzer.AnalyzeNetworkSymbols("/nonexistent/path/to/binary", "sha256:dummy")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	assert.NotNil(t, output.Error)
}

// TestHasDynamicLoad_ELF verifies that a binary importing dlopen is detected
// with non-empty DynamicLoadSymbols, independently of network symbol detection.
func TestHasDynamicLoad_ELF(t *testing.T) {
	analyzer := NewStandardELFAnalyzer(nil)

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

// TestStandardELFAnalyzer_LibcSymbolFiltering verifies that only libc-derived symbols
// are recorded in DetectedSymbols, and that each symbol carries the correct category.
//   - socket() imported from libc appears with category "socket"
//   - non-network libc symbols (e.g. __libc_start_main) appear with category "syscall_wrapper"
//   - symbols from non-libc libraries (e.g. SSL_CTX_new from libssl) are not recorded
func TestStandardELFAnalyzer_LibcSymbolFiltering(t *testing.T) {
	testdataDir := "testdata"
	analyzer := NewStandardELFAnalyzer(nil)

	t.Run("libc network symbol has socket category", func(t *testing.T) {
		path := filepath.Join(testdataDir, "with_socket.elf")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Skip("with_socket.elf not found")
		}
		absPath, err := filepath.Abs(path)
		require.NoError(t, err)

		output := analyzer.AnalyzeNetworkSymbols(absPath, "sha256:dummy")
		require.Equal(t, binaryanalyzer.NetworkDetected, output.Result)

		found := false
		for _, sym := range output.DetectedSymbols {
			if sym.Name == "socket" {
				assert.Equal(t, "socket", sym.Category,
					`libc symbol "socket" should have category "socket"`)
				found = true
			}
		}
		assert.True(t, found, `"socket" should be present in DetectedSymbols`)
	})

	t.Run("non-network libc symbols have syscall_wrapper category", func(t *testing.T) {
		path := filepath.Join(testdataDir, "with_socket.elf")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Skip("with_socket.elf not found")
		}
		absPath, err := filepath.Abs(path)
		require.NoError(t, err)

		output := analyzer.AnalyzeNetworkSymbols(absPath, "sha256:dummy")
		require.Equal(t, binaryanalyzer.NetworkDetected, output.Result)

		// Every symbol that is not a network category must be "syscall_wrapper".
		for _, sym := range output.DetectedSymbols {
			if !binaryanalyzer.IsNetworkCategory(sym.Category) {
				assert.Equal(t, "syscall_wrapper", sym.Category,
					`non-network libc symbol %q should have category "syscall_wrapper"`, sym.Name)
			}
		}
		// At least one "syscall_wrapper" symbol must be present (e.g. __libc_start_main).
		hasSyscallWrapper := false
		for _, sym := range output.DetectedSymbols {
			if sym.Category == "syscall_wrapper" {
				hasSyscallWrapper = true
				break
			}
		}
		assert.True(t, hasSyscallWrapper,
			`expected at least one "syscall_wrapper" symbol from libc (e.g. __libc_start_main)`)
	})

	t.Run("non-libc network symbols recorded with correct category", func(t *testing.T) {
		path := filepath.Join(testdataDir, "with_ssl.elf")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Skip("with_ssl.elf not found")
		}
		absPath, err := filepath.Abs(path)
		require.NoError(t, err)

		output := analyzer.AnalyzeNetworkSymbols(absPath, "sha256:dummy")
		// After task 0109, Step 1 records networkSymbols matches regardless of Library.
		// SSL_CTX_new is in networkSymbols (tls category) and must appear in DetectedSymbols.
		foundSSL := false
		for _, sym := range output.DetectedSymbols {
			if sym.Name == "SSL_CTX_new" {
				assert.Equal(t, "tls", sym.Category,
					`SSL_CTX_new should have category "tls"`)
				foundSSL = true
			}
		}
		assert.True(t, foundSSL, `SSL_CTX_new should now appear in DetectedSymbols`)
	})
}

func TestStandardELFAnalyzer_WithCustomSymbols(t *testing.T) {
	// Create analyzer with a minimal custom symbol set
	customSymbols := map[string]binaryanalyzer.SymbolCategory{
		"my_network_func": binaryanalyzer.CategorySocket,
	}
	analyzer := NewStandardELFAnalyzer(nil)
	analyzer.networkSymbols = customSymbols

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
				Architecture: "x86_64",
				DetectedSyscalls: []SyscallInfo{
					{
						Number: 41, // socket
						Name:   "socket",
						Occurrences: []common.SyscallOccurrence{
							{Location: 0x401000},
						},
					},
					{
						Number: 42, // connect
						Name:   "connect",
						Occurrences: []common.SyscallOccurrence{
							{Location: 0x401010},
						},
					},
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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
						Number: 1, // write
						Name:   "write",
						Occurrences: []common.SyscallOccurrence{
							{Location: 0x401000},
						},
					},
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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
						Number: -1,
						Occurrences: []common.SyscallOccurrence{
							{Location: 0x401000, DeterminationMethod: "unknown:indirect_setting"},
						},
					},
				},
				AnalysisWarnings: []string{
					"syscall at 0x401000: number could not be determined (unknown:indirect_setting)",
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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
						Number: 41, // socket
						Name:   "socket",
						Occurrences: []common.SyscallOccurrence{
							{Location: 0x401000},
						},
					},
					{
						Number: -1,
						Occurrences: []common.SyscallOccurrence{
							{Location: 0x401010, DeterminationMethod: "unknown:indirect_setting"},
						},
					},
				},
				AnalysisWarnings: []string{
					"syscall at 0x401010: number could not be determined (unknown:indirect_setting)",
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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
					{Number: 41, Name: "socket"},
				},
			},
		},
		expectedHash: "sha256:differenthash", // This won't match the actual file hash
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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
	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, nil)
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
				Architecture: "x86_64",
				DetectedSyscalls: []SyscallInfo{
					{Number: 41, Name: "socket", Occurrences: []common.SyscallOccurrence{{Location: 0x401000}}},
				},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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
			analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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
					{Number: -1, Occurrences: []common.SyscallOccurrence{{Location: 0x401000, DeterminationMethod: "unknown:indirect_setting"}}},
				},
				AnalysisWarnings: []string{"syscall at 0x401000: number could not be determined (unknown:indirect_setting)"},
			},
		},
	}

	analyzer := NewStandardELFAnalyzerWithSyscallStore(nil, mockStore)
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
	analyzer := NewStandardELFAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(testFile, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
}

// TestCheckDynamicSymbols_NameBasedFilter verifies that checkDynamicSymbols applies
// name-based detection for VERNEED-absent binaries (musl-style) and correctly handles
// mixed-library VERNEED-absent binaries.
func TestCheckDynamicSymbols_NameBasedFilter(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analyzer := NewStandardELFAnalyzer(nil)

	t.Run("no-VERNEED binary importing socket yields NetworkDetected with socket category", func(t *testing.T) {
		path := filepath.Join(tmpDir, "socket_only.elf")
		elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
			{Name: "socket"},
		})

		output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

		require.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
		require.NotEmpty(t, output.DetectedSymbols)
		found := false
		for _, sym := range output.DetectedSymbols {
			if sym.Name == "socket" {
				assert.Equal(t, "socket", sym.Category)
				found = true
			}
		}
		assert.True(t, found, `"socket" must be in DetectedSymbols`)
	})

	t.Run("no-VERNEED binary importing SSL_CTX_new yields NetworkDetected with tls category", func(t *testing.T) {
		path := filepath.Join(tmpDir, "ssl_only.elf")
		elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
			{Name: "SSL_CTX_new"},
		})

		output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

		require.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
		found := false
		for _, sym := range output.DetectedSymbols {
			if sym.Name == "SSL_CTX_new" {
				assert.Equal(t, "tls", sym.Category)
				found = true
			}
		}
		assert.True(t, found, `"SSL_CTX_new" must be in DetectedSymbols`)
	})

	t.Run("no-VERNEED binary importing only non-network symbols yields NoNetworkSymbols", func(t *testing.T) {
		path := filepath.Join(tmpDir, "read_only.elf")
		elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
			{Name: "read"},
		})

		output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

		assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
		assert.Empty(t, output.DetectedSymbols)
	})

	t.Run("no-VERNEED binary with mixed symbols records only networkSymbols matches", func(t *testing.T) {
		path := filepath.Join(tmpDir, "mixed_symbols.elf")
		elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
			{Name: "socket"},
			{Name: "SSL_CTX_new"},
			{Name: "pthread_create"}, // not in networkSymbols, must not be recorded
		})

		output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

		require.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
		names := make(map[string]string)
		for _, sym := range output.DetectedSymbols {
			names[sym.Name] = sym.Category
		}
		assert.Equal(t, "socket", names["socket"])
		assert.Equal(t, "tls", names["SSL_CTX_new"])
		assert.NotContains(t, names, "pthread_create",
			"pthread_create is not in networkSymbols and must not appear in DetectedSymbols")
	})

	t.Run("dlopen in no-VERNEED binary appears in DynamicLoadSymbols", func(t *testing.T) {
		path := filepath.Join(tmpDir, "dlopen_socket.elf")
		elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
			{Name: "dlopen"},
			{Name: "socket"},
		})

		output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

		found := false
		for _, sym := range output.DynamicLoadSymbols {
			if sym.Name == "dlopen" {
				assert.Equal(t, "dynamic_load", sym.Category)
				found = true
			}
		}
		assert.True(t, found, `"dlopen" must appear in DynamicLoadSymbols`)
	})
}
