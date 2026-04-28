//go:build integration

package elfanalyzer

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hasNetworkSyscall(arch string, syscalls []SyscallInfo) bool {
	var table interface{ IsNetworkSyscall(int) bool }
	switch arch {
	case "x86_64":
		table = NewX86_64SyscallTable()
	case "arm64":
		table = NewARM64LinuxSyscallTable()
	default:
		return false
	}
	for _, s := range syscalls {
		if table.IsNetworkSyscall(s.Number) {
			return true
		}
	}
	return false
}

func TestSyscallAnalyzer_RealCBinary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF syscall analysis requires Linux")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("syscall analysis only supports x86_64 and arm64")
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
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "test.c")
	binFile := filepath.Join(tmpDir, "test")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	// Compile with static linking; skip if static libc is unavailable (common on arm64)
	cmd := exec.Command("gcc", "-static", "-o", binFile, srcFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("gcc -static failed (static libc may be unavailable): %s", string(output))
	}

	// Open and parse ELF
	elfFile, err := elf.Open(binFile)
	require.NoError(t, err)
	defer elfFile.Close()

	// Analyze syscalls
	analyzer := NewSyscallAnalyzer()
	result, err := analyzer.AnalyzeSyscallsFromELF(elfFile)
	require.NoError(t, err)

	// Verify network syscall detected
	assert.True(t, hasNetworkSyscall(result.Architecture, result.DetectedSyscalls), "socket syscall should be detected as network-related")

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
	if runtime.GOOS != "linux" {
		t.Skip("ELF syscall analysis requires Linux")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("syscall analysis only supports x86_64 and arm64")
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
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "main.go")
	binFile := filepath.Join(tmpDir, "test")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	// Compile as static binary with CGO disabled for the current architecture
	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOARCH="+runtime.GOARCH)
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
	assert.Greater(t, len(result.DetectedSyscalls), 0,
		"Go binary should contain detectable syscall events")

	// Check that Go wrapper calls were detected (Pass 2)
	hasGoWrapper := false
	for _, info := range result.DetectedSyscalls {
		for _, occ := range info.Occurrences {
			if occ.DeterminationMethod == DeterminationMethodGoWrapper {
				hasGoWrapper = true
				break
			}
		}
		if hasGoWrapper {
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
	if runtime.GOOS != "linux" {
		t.Skip("ELF syscall analysis requires Linux")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("syscall analysis only supports x86_64 and arm64")
	}

	// Create a simple Go program that does NOT use networking
	src := `package main

import "fmt"

func main() {
	fmt.Println("hello, world")
}
`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "main.go")
	binFile := filepath.Join(tmpDir, "test")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	// Compile as static binary for the current architecture
	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOARCH="+runtime.GOARCH)
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
	assert.False(t, hasNetworkSyscall(result.Architecture, result.DetectedSyscalls), "hello-world Go binary should not have network syscalls")
}

// TestE2E_RecordToRunnerFallbackChain tests the full pipeline:
// compile Go binary → analyze with SyscallAnalyzer → save to FileAnalysisStore
// → load via fileanalysis.SyscallAnalysisStore → verify correct AnalysisOutput
// via StandardELFAnalyzer.convertSyscallResult.
//
// This verifies AC-8: the fallback chain from record command to runner.
func TestE2E_RecordToRunnerFallbackChain(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF syscall analysis requires Linux")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("syscall analysis only supports x86_64 and arm64")
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
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "main.go")
	binFile := filepath.Join(tmpDir, "test_binary")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOARCH="+runtime.GOARCH)
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
	assert.Equal(t, binaryanalyzer.NetworkDetected, analysisOutput.Result,
		"StandardELFAnalyzer should detect network capability from stored analysis")
	assert.NotEmpty(t, analysisOutput.DetectedSymbols,
		"should have detected network symbols")
}

// TestSyscallAnalyzer_IntegrationARM64_NetworkSyscalls verifies that the ELF
// analyzer correctly detects network syscalls in a pre-compiled arm64 binary.
//
// The test binary is at testdata/arm64_network_program/arm64_network_program.elf.
// To regenerate it:
//
//	cd internal/runner/security/elfanalyzer
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
//	  -o testdata/arm64_network_program/arm64_network_program.elf \
//	  ./testdata/arm64_network_program/
func TestSyscallAnalyzer_IntegrationARM64_NetworkSyscalls(t *testing.T) {
	const binaryPath = "testdata/arm64_network_program/arm64_network_program.elf"

	elfFile, err := elf.Open(binaryPath)
	require.NoError(t, err, "failed to open arm64 test binary: %s", binaryPath)
	defer elfFile.Close()

	// Verify this is actually an arm64 binary
	require.Equal(t, elf.EM_AARCH64, elfFile.Machine,
		"test binary must be arm64 (EM_AARCH64)")

	analyzer := NewSyscallAnalyzer()
	result, err := analyzer.AnalyzeSyscallsFromELF(elfFile)
	require.NoError(t, err)

	// The binary uses net.Dial which resolves to socket(198) on arm64
	assert.True(t, hasNetworkSyscall(result.Architecture, result.DetectedSyscalls), "arm64 binary using net.Dial should have network syscalls detected")

	// Verify socket syscall (arm64 number 198) is among the detected syscalls
	found := false
	for _, info := range result.DetectedSyscalls {
		if info.Name == "socket" && info.Number == 198 {
			found = true
			break
		}
	}
	assert.True(t, found,
		"socket syscall (number 198) should be detected in the arm64 binary")
}

// TestAC1_CgoBinaryNetworkDetection verifies AC-1 (third condition) for arm64:
// After Pass 1 fix (knownSyscallImpls updated) and Pass 2 fix, a CGO binary
// that calls syscall.Socket() directly should return HasNetworkSyscalls: true.
func TestAC1_CgoBinaryNetworkDetection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("this test targets Linux ELF binaries and Linux arm64 syscall numbering")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("this test targets arm64 CGO binary detection")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go compiler not available")
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("C compiler (cc) not available; required for CGO_ENABLED=1")
	}

	src := `package main
import "C"
import "syscall"
func main() {
    fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
    if err == nil { _ = syscall.Close(fd) }
}`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "main.go")
	binaryPath := filepath.Join(tmpDir, "cgo_test")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	cmd := exec.Command("go", "build", "-o", binaryPath, srcFile)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(output))

	t.Run("dynsym_returns_NoNetworkSymbols", func(t *testing.T) {
		// Verify that .dynsym analysis returns NoNetworkSymbols (the blind spot).
		elfFile, err := elf.Open(binaryPath)
		require.NoError(t, err)
		defer elfFile.Close()

		dynsyms, err := elfFile.DynamicSymbols()
		require.NoError(t, err, ".dynsym must be present in a CGO binary")

		networkSyms := map[string]bool{
			"socket": true, "connect": true, "bind": true,
			"sendto": true, "recvfrom": true, "getaddrinfo": true,
		}
		for _, sym := range dynsyms {
			if networkSyms[sym.Name] {
				t.Errorf(".dynsym contains network symbol %q; CGO binary should not have it", sym.Name)
			}
		}
		t.Logf("AnalyzeNetworkSymbols result: no_network_symbols (confirmed: no network symbols in .dynsym)")
	})

	t.Run("syscall_analysis_detects_socket", func(t *testing.T) {
		// AC-1 third condition: after Pass 1 + Pass 2 fixes, HasNetworkSyscalls must be true.
		elfFile, err := elf.Open(binaryPath)
		require.NoError(t, err)
		defer elfFile.Close()

		analyzer := NewSyscallAnalyzer()
		result, err := analyzer.AnalyzeSyscallsFromELF(elfFile)
		require.NoError(t, err)

		t.Logf("SyscallAnalysis architecture: %s", result.Architecture)
		t.Logf("DetectedSyscalls count: %d", len(result.DetectedSyscalls))

		for i, sc := range result.DetectedSyscalls {
			method, location := "", uint64(0)
			if len(sc.Occurrences) > 0 {
				method = sc.Occurrences[0].DeterminationMethod
				location = sc.Occurrences[0].Location
			}
			t.Logf("Syscall[%d]: #%-4d (%-20s) method=%s at 0x%x",
				i, sc.Number, sc.Name, method, location)
		}

		assert.True(t, hasNetworkSyscall(result.Architecture, result.DetectedSyscalls),
			"CGO binary calling syscall.Socket() should have network syscalls detected after fixes")

		found := false
		for _, sc := range result.DetectedSyscalls {
			if sc.Name == "socket" && sc.Number == 198 {
				found = true
				method := ""
				if len(sc.Occurrences) > 0 {
					method = sc.Occurrences[0].DeterminationMethod
				}
				t.Logf("socket(198) detected via method=%s", method)
				break
			}
		}
		assert.True(t, found, "socket syscall (arm64 #198) should be detected")
	})
}

// TestAC1_X86CgoBinaryLowLevelImplExcluded verifies that x86_64 CGO binaries
// do not report unknown:control_flow_boundary from low-level syscall
// implementation bodies where syscall numbers are caller-supplied.
func TestAC1_X86CgoBinaryLowLevelImplExcluded(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("this test targets Linux ELF binaries")
	}
	if runtime.GOARCH != "amd64" {
		t.Skip("this test targets x86_64 CGO binary detection")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go compiler not available")
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("C compiler (cc) not available; required for CGO_ENABLED=1")
	}

	src := `package main
import "C"
import "syscall"

func main() {
	fd, _, errno := syscall.RawSyscall(
		syscall.SYS_SOCKET,
		uintptr(syscall.AF_INET),
		uintptr(syscall.SOCK_STREAM),
		0,
	)
	if errno == 0 {
		_ = syscall.Close(int(fd))
	}
}`
	tmpDir := commontesting.SafeTempDir(t)
	srcFile := filepath.Join(tmpDir, "main.go")
	binaryPath := filepath.Join(tmpDir, "cgo_x86_test")

	require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

	cmd := exec.Command("go", "build", "-o", binaryPath, srcFile)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(output))

	elfFile, err := elf.Open(binaryPath)
	require.NoError(t, err)
	defer elfFile.Close()

	analyzer := NewSyscallAnalyzer()
	result, err := analyzer.AnalyzeSyscallsFromELF(elfFile)
	require.NoError(t, err)

	assert.Equal(t, "x86_64", result.Architecture)
	assert.True(t, hasNetworkSyscall(result.Architecture, result.DetectedSyscalls),
		"x86_64 CGO binary calling syscall.RawSyscall(SYS_SOCKET) should detect network syscall")

	// Regression check: low-level implementation bodies should be excluded from
	// direct-syscall pass, so control-flow-boundary unknowns are not reported.
	for _, sc := range result.DetectedSyscalls {
		for _, occ := range sc.Occurrences {
			assert.NotEqual(t, DeterminationMethodUnknownControlFlowBoundary, occ.DeterminationMethod,
				"unexpected unknown:control_flow_boundary at 0x%x", occ.Location)
		}
	}
}

// TestSyscallAnalyzer_IntegrationARM64_Architecture verifies that the
// Architecture field in the analysis result is set to "arm64".
func TestSyscallAnalyzer_IntegrationARM64_Architecture(t *testing.T) {
	const binaryPath = "testdata/arm64_network_program/arm64_network_program.elf"

	elfFile, err := elf.Open(binaryPath)
	require.NoError(t, err, "failed to open arm64 test binary: %s", binaryPath)
	defer elfFile.Close()

	analyzer := NewSyscallAnalyzer()
	result, err := analyzer.AnalyzeSyscallsFromELF(elfFile)
	require.NoError(t, err)

	assert.Equal(t, "arm64", result.Architecture,
		"analysis result Architecture field should be 'arm64' for an arm64 binary")
}
