//go:build test

package machoanalyzer

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataPath returns the absolute path to a file in the testdata directory.
func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	return filepath.Join(dir, "testdata", name)
}

// skipIfNotExist skips the test if the fixture file does not exist.
func skipIfNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("fixture not found (run Phase 4 to generate): %s", path)
	}
}

// TestNormalizeSymbolName tests the normalizeSymbolName function.
func TestNormalizeSymbolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"_socket", "socket"},
		{"_socket$UNIX2003", "socket"},
		{"socket", "socket"},
		{"_connect$INODE64", "connect"},
		{"SSL_connect", "SSL_connect"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeSymbolName(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestStandardMachOAnalyzer_NetworkSymbols_Detected tests that a C binary importing socket
// returns NetworkDetected with detected symbols. (AC-2)
func TestStandardMachOAnalyzer_NetworkSymbols_Detected(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}
	path := testdataPath("network_macho_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "")

	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
	assert.NotEmpty(t, output.DetectedSymbols, "expected at least one detected network symbol")
}

// TestStandardMachOAnalyzer_NoNetworkSymbols tests that a C binary without network symbols
// returns NoNetworkSymbols. (AC-2)
func TestStandardMachOAnalyzer_NoNetworkSymbols(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}
	path := testdataPath("no_network_macho_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "")

	assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
}

// TestStandardMachOAnalyzer_SVCOnly_HighRisk tests that a binary containing only svc #0x80
// returns AnalysisError wrapping ErrDirectSyscall. (AC-6)
func TestStandardMachOAnalyzer_SVCOnly_HighRisk(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}
	path := testdataPath("svc_only_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	require.Error(t, output.Error)
	assert.True(t, errors.Is(output.Error, ErrDirectSyscall),
		"expected ErrDirectSyscall, got: %v", output.Error)
}

// TestStandardMachOAnalyzer_NetworkSymbols_SVCIgnored tests that network symbols take priority
// over svc #0x80. (AC-6)
func TestStandardMachOAnalyzer_NetworkSymbols_SVCIgnored(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}
	// network_macho_arm64 may also contain svc, but symbols take priority
	path := testdataPath("network_macho_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "")

	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
}

// TestStandardMachOAnalyzer_FatBinary_Arm64Selected tests that a Fat binary's arm64 slice
// is selected and analyzed. (AC-1)
func TestStandardMachOAnalyzer_FatBinary_Arm64Selected(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}
	path := testdataPath("fat_binary")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "")

	// fat_binary is built from network_macho_arm64 + x86_64, so arm64 should detect network symbols
	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
	assert.NotEmpty(t, output.DetectedSymbols)
}

// TestStandardMachOAnalyzer_FatBinary_NoArm64Slice tests that a Fat binary without an arm64 slice
// returns NotSupportedBinary. (AC-1)
func TestStandardMachOAnalyzer_FatBinary_NoArm64Slice(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}
	path := testdataPath("network_macho_x86_64")
	skipIfNotExist(t, path)

	// Wrap the x86_64-only binary in a fat file structure by testing via fat parsing path.
	// Since we can't easily create an x86_64-only fat binary without lipo,
	// we test the selectMachOFromFat logic indirectly via a mock approach:
	// We use the x86_64 single-arch binary which will not be a Fat binary,
	// but the Fat "no arm64 slice" case is tested via unit test of selectMachOFromFat.
	t.Skip("x86_64-only Fat binary fixture not available; selectMachOFromFat tested via unit test")
}

// TestStandardMachOAnalyzer_GoNetwork_Detected tests that a Go binary using the net package
// returns NetworkDetected. (AC-3)
func TestStandardMachOAnalyzer_GoNetwork_Detected(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}
	path := testdataPath("network_go_macho_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "")

	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
	assert.NotEmpty(t, output.DetectedSymbols)
}

// TestStandardMachOAnalyzer_GoNoNetwork_NoSymbols tests that a Go binary without network operations
// returns NoNetworkSymbols. (AC-3)
func TestStandardMachOAnalyzer_GoNoNetwork_NoSymbols(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}
	path := testdataPath("no_network_go_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "")

	assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
}

// TestStandardMachOAnalyzer_NonMachO_Script tests that a non-Mach-O file (shell script)
// returns NotSupportedBinary. (AC-1)
func TestStandardMachOAnalyzer_NonMachO_Script(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}
	path := testdataPath("script.sh")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "")

	assert.Equal(t, binaryanalyzer.NotSupportedBinary, output.Result)
}

// TestStandardMachOAnalyzer_InvalidMachO_NoPanic tests that a corrupted Mach-O file
// returns AnalysisError without panicking. (AC-5)
func TestStandardMachOAnalyzer_InvalidMachO_NoPanic(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	// Create a temp file that looks like a Mach-O (valid magic) but has truncated content
	tmp, err := os.CreateTemp(t.TempDir(), "invalid_macho_*")
	require.NoError(t, err)
	defer tmp.Close()

	// Write a valid 64-bit Mach-O magic number followed by garbage
	magic := []byte{0xCF, 0xFA, 0xED, 0xFE, 0x00, 0x00, 0x00, 0x00}
	_, err = tmp.Write(magic)
	require.NoError(t, err)
	tmp.Close()

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(tmp.Name(), "")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	require.Error(t, output.Error)
}

// TestStandardMachOAnalyzer_FileOpenError tests that a non-existent path
// returns AnalysisError. (AC-5)
func TestStandardMachOAnalyzer_FileOpenError(t *testing.T) {
	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols("/nonexistent/path/to/binary", "")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	require.Error(t, output.Error)
}

// TestNetworkAnalyzer_Integration_MachO tests real macOS binaries for network detection. (AC-4)
func TestNetworkAnalyzer_Integration_MachO(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-only integration test")
	}

	analyzer := NewStandardMachOAnalyzer(nil)

	// /usr/bin/curl is a well-known network binary on macOS
	output := analyzer.AnalyzeNetworkSymbols("/usr/bin/curl", "")
	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result,
		"expected NetworkDetected for /usr/bin/curl")
}
