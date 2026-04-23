//go:build test

package machoanalyzer

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gosDarwin is the GOOS value for macOS, used in darwin-only test guards.
const gosDarwin = "darwin"

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
		t.Skipf("fixture not found (run machoanalyzer-testdata to generate): %s", path)
	}
}

// TestNormalizeSymbolName tests the NormalizeSymbolName function.
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
			got := NormalizeSymbolName(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestStandardMachOAnalyzer_NetworkSymbols_Detected tests that a C binary importing socket
// returns NetworkDetected with detected symbols. (AC-2)
func TestStandardMachOAnalyzer_NetworkSymbols_Detected(t *testing.T) {
	path := testdataPath("network_macho_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
	assert.NotEmpty(t, output.DetectedSymbols, "expected at least one detected network symbol")
}

// TestStandardMachOAnalyzer_NoNetworkSymbols tests that a C binary without network symbols
// returns NoNetworkSymbols. (AC-2)
func TestStandardMachOAnalyzer_NoNetworkSymbols(t *testing.T) {
	path := testdataPath("no_network_macho_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
}

// TestStandardMachOAnalyzer_SVCOnly_NoNetworkSymbols tests that a binary containing only
// svc #0x80 (no network symbols) returns NoNetworkSymbols from AnalyzeNetworkSymbols.
// svc #0x80 risk is evaluated separately via SyscallAnalysis (ScanSyscallInfos).
func TestStandardMachOAnalyzer_SVCOnly_NoNetworkSymbols(t *testing.T) {
	path := testdataPath("svc_only_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
	assert.NoError(t, output.Error)
}

// TestStandardMachOAnalyzer_NetworkSymbols_SVCIgnored tests that network symbols take priority
// over svc #0x80. (AC-6)
func TestStandardMachOAnalyzer_NetworkSymbols_SVCIgnored(t *testing.T) {
	// network_macho_arm64 may also contain svc, but symbols take priority
	path := testdataPath("network_macho_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
}

// TestStandardMachOAnalyzer_FatBinary_Arm64Selected tests that a Fat binary's arm64 slice
// is selected and analyzed. (AC-1)
func TestStandardMachOAnalyzer_FatBinary_Arm64Selected(t *testing.T) {
	path := testdataPath("fat_binary")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

	// fat_binary is built from network_macho_arm64 + x86_64, so arm64 should detect network symbols
	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
	assert.NotEmpty(t, output.DetectedSymbols)
}

// TestStandardMachOAnalyzer_FatBinary_AllSlicesAnalyzed tests that when a Fat binary contains
// a malicious x86_64 slice (with network symbols) and a benign arm64 slice, the analyzer
// detects the threat from the x86_64 slice. This prevents a cross-architecture security bypass
// where an attacker hides a malicious slice behind a clean arm64 slice. (AC-1)
func TestStandardMachOAnalyzer_FatBinary_AllSlicesAnalyzed(t *testing.T) {
	path := testdataPath("fat_network_x86_only")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

	// Even though the arm64 slice has no network symbols, the x86_64 slice does.
	// The analyzer must detect the threat from any slice.
	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
	assert.NotEmpty(t, output.DetectedSymbols)
}

// TestStandardMachOAnalyzer_GoNetwork_Detected tests that a Go binary using the net package
// returns NetworkDetected. (AC-3)
func TestStandardMachOAnalyzer_GoNetwork_Detected(t *testing.T) {
	path := testdataPath("network_go_macho_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
	assert.NotEmpty(t, output.DetectedSymbols)
}

// TestStandardMachOAnalyzer_GoNoNetwork_NoSymbols tests that a Go binary without network operations
// returns NoNetworkSymbols. (AC-3)
func TestStandardMachOAnalyzer_GoNoNetwork_NoSymbols(t *testing.T) {
	path := testdataPath("no_network_go_arm64")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
}

// TestStandardMachOAnalyzer_NonMachO_Script tests that a non-Mach-O file (shell script)
// returns NotSupportedBinary. (AC-1)
func TestStandardMachOAnalyzer_NonMachO_Script(t *testing.T) {
	path := testdataPath("script.sh")
	skipIfNotExist(t, path)

	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

	assert.Equal(t, binaryanalyzer.NotSupportedBinary, output.Result)
}

// TestStandardMachOAnalyzer_InvalidMachO_NoPanic tests that a corrupted Mach-O file
// returns AnalysisError without panicking. (AC-5)
func TestStandardMachOAnalyzer_InvalidMachO_NoPanic(t *testing.T) {
	if runtime.GOOS != gosDarwin {
		// safefileio rejects paths containing symlink components; on macOS the
		// source tree is under a real path but t.TempDir() is under /private/tmp
		// which is accessed via /tmp (a symlink). This test is darwin-only until
		// the temp-file path issue is resolved separately.
		t.Skip("Mach-O tests only run on macOS (temp file path limitation)")
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
	output := analyzer.AnalyzeNetworkSymbols(tmp.Name(), "sha256:dummy")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	require.Error(t, output.Error)
}

// largeFakeFileInfo implements os.FileInfo reporting a size larger than maxFileSize.
type largeFakeFileInfo struct{ os.FileInfo }

func (largeFakeFileInfo) Size() int64 { return maxFileSize + 1 }
func (largeFakeFileInfo) Mode() os.FileMode {
	return 0o644 // regular file
}
func (largeFakeFileInfo) IsDir() bool { return false }

// largeFakeFile implements safefileio.File; only Stat() is exercised before the
// size-check early return, so all other methods are no-ops.
type largeFakeFile struct{}

func (largeFakeFile) Read(_ []byte) (int, error)            { return 0, io.EOF }
func (largeFakeFile) Write(_ []byte) (int, error)           { return 0, nil }
func (largeFakeFile) Seek(_ int64, _ int) (int64, error)    { return 0, nil }
func (largeFakeFile) ReadAt(_ []byte, _ int64) (int, error) { return 0, io.EOF }
func (largeFakeFile) Chmod(_ os.FileMode) error             { return nil }
func (largeFakeFile) Close() error                          { return nil }
func (largeFakeFile) Stat() (os.FileInfo, error)            { return largeFakeFileInfo{}, nil }
func (largeFakeFile) Truncate(_ int64) error                { return nil }

// largeFakeFS implements safefileio.FileSystem, returning largeFakeFile for any path.
type largeFakeFS struct{}

func (largeFakeFS) SafeOpenFile(_ string, _ int, _ os.FileMode) (safefileio.File, error) {
	return largeFakeFile{}, nil
}
func (largeFakeFS) AtomicMoveFile(_, _ string, _ os.FileMode) error      { return nil }
func (largeFakeFS) GetGroupMembership() *groupmembership.GroupMembership { return nil }
func (largeFakeFS) Remove(_ string) error                                { return nil }

// TestStandardMachOAnalyzer_FileTooLarge tests that a file exceeding maxFileSize
// returns AnalysisError wrapping ErrFileTooLarge. (AC-5)
func TestStandardMachOAnalyzer_FileTooLarge(t *testing.T) {
	analyzer := NewStandardMachOAnalyzer(largeFakeFS{})
	output := analyzer.AnalyzeNetworkSymbols("any_path", "sha256:dummy")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	require.Error(t, output.Error)
	assert.True(t, errors.Is(output.Error, ErrFileTooLarge),
		"expected ErrFileTooLarge, got: %v", output.Error)
}

// TestStandardMachOAnalyzer_FileOpenError tests that a non-existent path
// returns AnalysisError. (AC-5)
func TestStandardMachOAnalyzer_FileOpenError(t *testing.T) {
	analyzer := NewStandardMachOAnalyzer(nil)
	output := analyzer.AnalyzeNetworkSymbols("/nonexistent/path/to/binary", "sha256:dummy")

	assert.Equal(t, binaryanalyzer.AnalysisError, output.Result)
	require.Error(t, output.Error)
}

// TestNetworkAnalyzer_Integration_MachO tests real macOS binaries for network detection. (AC-4)
func TestNetworkAnalyzer_Integration_MachO(t *testing.T) {
	if runtime.GOOS != gosDarwin {
		t.Skip("macOS-only integration test")
	}

	analyzer := NewStandardMachOAnalyzer(nil)

	// /usr/bin/curl is a well-known network binary on macOS
	output := analyzer.AnalyzeNetworkSymbols("/usr/bin/curl", "sha256:dummy")
	assert.Equal(t, binaryanalyzer.NetworkDetected, output.Result,
		"expected NetworkDetected for /usr/bin/curl")
}
