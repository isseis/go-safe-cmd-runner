//go:build integration

package elfanalyzer

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyscallAnalyzer_RealCBinary(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("syscall analysis only supports x86_64")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available")
	}

	// Create test C program that calls socket()
	src := `
#include <sys/socket.h>
int main() {
    socket(AF_INET, SOCK_STREAM, 0);
    return 0;
}
`
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.c")
	binFile := filepath.Join(tmpDir, "test")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	// Compile with static linking
	cmd := exec.Command("gcc", "-static", "-o", binFile, srcFile)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "gcc failed: %s", string(output))

	// Open and parse ELF
	elfFile, err := elf.Open(binFile)
	require.NoError(t, err)
	defer elfFile.Close()

	// Analyze syscalls
	analyzer := NewSyscallAnalyzer()
	result, err := analyzer.AnalyzeSyscallsFromELF(elfFile)
	require.NoError(t, err)

	// Verify network syscall detected
	assert.True(t, result.Summary.HasNetworkSyscalls,
		"socket syscall should be detected as network-related")
	assert.Greater(t, result.Summary.NetworkSyscallCount, 0)

	// Verify socket syscall found
	found := false
	for _, info := range result.DetectedSyscalls {
		if info.Name == "socket" {
			found = true
			break
		}
	}
	assert.True(t, found, "socket syscall should be detected")
}

func TestSyscallAnalyzer_RealGoBinary(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("syscall analysis only supports x86_64")
	}

	// Create test Go program that uses net package (which calls socket via Go wrappers)
	src := `package main

import (
	"net"
	"os"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:1")
	if err != nil {
		os.Exit(0) // Expected to fail, but syscall.Syscall is still in the binary
	}
	conn.Close()
}
`
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	binFile := filepath.Join(tmpDir, "test")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	// Compile as static binary with CGO disabled
	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOARCH=amd64")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(output))

	// Open and parse ELF
	elfFile, err := elf.Open(binFile)
	require.NoError(t, err)
	defer elfFile.Close()

	// Analyze syscalls
	analyzer := NewSyscallAnalyzer()
	result, err := analyzer.AnalyzeSyscallsFromELF(elfFile)
	require.NoError(t, err)

	// Verify the analysis completed successfully
	assert.Greater(t, result.Summary.TotalDetectedEvents, 0,
		"Go binary should contain detectable syscall events")

	// Check that Go wrapper calls were detected (Pass 2)
	hasGoWrapper := false
	for _, info := range result.DetectedSyscalls {
		if info.DeterminationMethod == DeterminationMethodGoWrapper {
			hasGoWrapper = true
			break
		}
	}
	// Note: Go wrapper detection depends on .gopclntab being present and
	// the binary containing calls to known wrappers (syscall.Syscall, etc.).
	// The net package uses these wrappers, so we expect at least some.
	assert.True(t, hasGoWrapper,
		"Go binary using net package should have detectable Go wrapper calls")
}

func TestSyscallAnalyzer_RealGoBinary_NoNetwork(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("syscall analysis only supports x86_64")
	}

	// Create a simple Go program that does NOT use networking
	src := `package main

import "fmt"

func main() {
	fmt.Println("hello, world")
}
`
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	binFile := filepath.Join(tmpDir, "test")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	// Compile as static binary
	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOARCH=amd64")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(output))

	// Open and parse ELF
	elfFile, err := elf.Open(binFile)
	require.NoError(t, err)
	defer elfFile.Close()

	// Analyze syscalls
	analyzer := NewSyscallAnalyzer()
	result, err := analyzer.AnalyzeSyscallsFromELF(elfFile)
	require.NoError(t, err)

	// A simple hello-world should NOT have network syscalls
	assert.False(t, result.Summary.HasNetworkSyscalls,
		"hello-world Go binary should not have network syscalls")
	assert.Equal(t, 0, result.Summary.NetworkSyscallCount)
}

// TestE2E_RecordToRunnerFallbackChain tests the full pipeline:
// compile Go binary → analyze with SyscallAnalyzer → save to FileAnalysisStore
// → load via fileanalysis.SyscallAnalysisStore → verify correct AnalysisOutput
// via StandardELFAnalyzer.convertSyscallResult.
//
// This verifies AC-8: the fallback chain from record command to runner.
func TestE2E_RecordToRunnerFallbackChain(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("syscall analysis only supports x86_64")
	}

	// Step 1: Create and compile a Go program with network syscalls
	src := `package main

import (
	"net"
	"os"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:1")
	if err != nil {
		os.Exit(0)
	}
	conn.Close()
}
`
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "main.go")
	binFile := filepath.Join(tmpDir, "test_binary")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOARCH=amd64")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(output))

	// Step 2: Analyze with SyscallAnalyzer (simulates record command)
	elfFile, err := elf.Open(binFile)
	require.NoError(t, err)

	syscallAnalyzer := NewSyscallAnalyzer()
	analysisResult, err := syscallAnalyzer.AnalyzeSyscallsFromELF(elfFile)
	elfFile.Close()
	require.NoError(t, err)

	// Step 3: Save to FileAnalysisStore (simulates record command's store logic)
	pathGetter := filevalidator.NewHybridHashFilePathGetter()
	store, err := fileanalysis.NewStore(tmpDir, pathGetter)
	require.NoError(t, err)

	// Calculate hash for the binary
	hashAlgo := &filevalidator.SHA256{}
	f, err := os.Open(binFile)
	require.NoError(t, err)
	rawHash, err := hashAlgo.Sum(f)
	f.Close()
	require.NoError(t, err)
	contentHash := "sha256:" + rawHash

	// Convert elfanalyzer result to fileanalysis result (same as record command)
	// Both types embed common.SyscallAnalysisResultCore, enabling direct struct copy.
	faResult := &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: analysisResult.SyscallAnalysisResultCore,
	}

	// Save via SyscallAnalysisStore
	syscallStore := fileanalysis.NewSyscallAnalysisStore(store)
	err = syscallStore.SaveSyscallAnalysis(binFile, contentHash, faResult)
	require.NoError(t, err)

	// Step 4: Load via fileanalysis.SyscallAnalysisStore (simulates runner lookup)
	loadedResult, err := syscallStore.LoadSyscallAnalysis(binFile, contentHash)
	require.NoError(t, err)

	// Convert back to elfanalyzer types for convertSyscallResult.
	// Both types embed common.SyscallAnalysisResultCore, enabling direct struct copy.
	eaResult := &SyscallAnalysisResult{
		SyscallAnalysisResultCore: loadedResult.SyscallAnalysisResultCore,
	}

	// Step 5: Verify StandardELFAnalyzer's convertSyscallResult produces correct output
	stdAnalyzer := NewStandardELFAnalyzer(nil, nil)
	analysisOutput := stdAnalyzer.convertSyscallResult(eaResult)

	// The binary uses net.Dial → socket syscall → should detect network
	assert.Equal(t, NetworkDetected, analysisOutput.Result,
		"StandardELFAnalyzer should detect network capability from stored analysis")
	assert.NotEmpty(t, analysisOutput.DetectedSymbols,
		"should have detected network symbols")
}
