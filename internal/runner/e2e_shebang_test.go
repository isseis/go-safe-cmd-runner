//go:build test

// Package runner contains integration tests for shebang interpreter verification.
//
// These tests cover the full record → verification pipeline using real files,
// complementing the mock-based unit tests in group_executor_test.go and the
// mock-based shebang unit tests in internal/verification/manager_shebang_test.go.
//
// Run with: go test -tags test -v ./internal/runner -run TestIntegration_Shebang

package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildNetworkInterpreterBinary(t *testing.T, dir string) string {
	t.Helper()

	if runtime.GOOS != "linux" {
		t.Skipf("buildNetworkInterpreterBinary requires Linux (got %s)", runtime.GOOS)
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("buildNetworkInterpreterBinary requires cc (install build-essential)")
	}

	srcPath := filepath.Join(dir, "net_interp.c")
	binPath := filepath.Join(dir, "net-interpreter")

	src := `#include <dlfcn.h>
#include <sys/socket.h>
#include <unistd.h>

int main(int argc, char** argv) {
	void* handle = dlopen("libc.so.6", RTLD_LAZY);
	if (handle != NULL) {
		(void)dlsym(handle, "socket");
		dlclose(handle);
	}
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd >= 0) {
        close(fd);
    }
    (void)argc;
    (void)argv;
    return 0;
}
`

	require.NoError(t, os.WriteFile(srcPath, []byte(src), 0o644))

	cmd := exec.Command("cc", "-O0", "-o", binPath, srcPath, "-ldl")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to compile network interpreter: %s", string(out))

	require.NoError(t, os.Chmod(binPath, 0o755))
	return binPath
}

// TestIntegration_ShebangVerification_DirectForm tests the full record → verification
// pipeline for a script with a direct-form shebang (e.g. #!/bin/sh).
//
// Prerequisite: /bin/sh must exist (standard on all Linux systems).
func TestIntegration_ShebangVerification_DirectForm(t *testing.T) {
	hashDir := commontesting.SafeTempDir(t)
	scriptDir := commontesting.SafeTempDir(t)

	// Step 1 (record phase): create a shebang script and save its hash record.
	scriptPath := commontesting.WriteExecutableFile(t, scriptDir, "deploy.sh", []byte("#!/bin/sh\necho hello\n"))
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(scriptPath, false)
	require.NoError(t, err)

	// Step 2 (runner phase): create a real verification manager and verify.
	manager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	err = manager.VerifyCommandShebangInterpreter(scriptPath, map[string]string{"PATH": "/usr/bin:/bin"})
	assert.NoError(t, err)
}

// TestIntegration_ShebangVerification_EnvForm tests the full record → verification
// pipeline for a script with an env-form shebang (e.g. #!/usr/bin/env sh).
//
// Prerequisite: /usr/bin/env and sh must exist (standard on all Linux systems).
// "sh" is used instead of "python3" to avoid a dependency on python3 in CI.
func TestIntegration_ShebangVerification_EnvForm(t *testing.T) {
	hashDir := commontesting.SafeTempDir(t)
	scriptDir := commontesting.SafeTempDir(t)

	// Pin PATH for both phases so shebang.Parse (record) and verifyEnvPathResolution
	// (verify) resolve "sh" from the same directories.
	t.Setenv("PATH", "/usr/bin:/bin")

	// Step 1 (record phase): create a shebang script and save its hash record.
	// SaveRecord also records /usr/bin/env and the resolved sh binary automatically.
	scriptPath := commontesting.WriteExecutableFile(t, scriptDir, "process.sh", []byte("#!/usr/bin/env sh\necho hello\n"))
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(scriptPath, false)
	require.NoError(t, err)

	// Step 2 (runner phase): create a real verification manager and verify.
	manager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	err = manager.VerifyCommandShebangInterpreter(scriptPath, map[string]string{"PATH": "/usr/bin:/bin"})
	assert.NoError(t, err)
}

// TestIntegration_ShebangVerification_InterpreterRecordMissing verifies that
// the runner phase detects a missing interpreter hash record and returns
// ErrInterpreterRecordNotFound.
//
// Prerequisite: /bin/sh must exist (standard on all Linux systems).
func TestIntegration_ShebangVerification_InterpreterRecordMissing(t *testing.T) {
	hashDir := commontesting.SafeTempDir(t)
	scriptDir := commontesting.SafeTempDir(t)

	// Step 1 (record phase): record the script (which also records the interpreter).
	scriptPath := commontesting.WriteExecutableFile(t, scriptDir, "deploy.sh", []byte("#!/bin/sh\necho hello\n"))
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(scriptPath, false)
	require.NoError(t, err)

	// Simulate a missing interpreter record by removing the embedded dep hash and
	// deleting the compatibility interpreter record.
	interpPath, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)
	scriptRecord, err := validator.LoadRecord(scriptPath)
	require.NoError(t, err)
	filteredDeps := make([]fileanalysis.LibEntry, 0, len(scriptRecord.DynLibDeps))
	for _, dep := range scriptRecord.DynLibDeps {
		if dep.Path == interpPath {
			continue
		}
		filteredDeps = append(filteredDeps, dep)
	}
	scriptRecord.DynLibDeps = filteredDeps
	store := validator.Store()
	require.NotNil(t, store)
	scriptResolvedPath, err := common.NewResolvedPath(scriptPath)
	require.NoError(t, err)
	require.NoError(t, store.Save(scriptResolvedPath, scriptRecord))

	resolvedInterpPath, err := common.NewResolvedPath(interpPath)
	require.NoError(t, err)
	interpHashPath, err := validator.HashFilePath(resolvedInterpPath)
	require.NoError(t, err)
	require.NoError(t, os.Remove(interpHashPath))

	// Step 2 (runner phase): verification should detect the missing interpreter record.
	manager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	err = manager.VerifyCommandShebangInterpreter(scriptPath, map[string]string{"PATH": "/usr/bin:/bin"})
	require.Error(t, err)
	var notFound *verification.ErrInterpreterRecordNotFound
	assert.ErrorAs(t, err, &notFound, "expected ErrInterpreterRecordNotFound, got: %v", err)
	assert.Equal(t, interpPath, notFound.Path)
}

// TestIntegration_ShebangChainRunnerExecution verifies an end-to-end runner
// execution path for a shebang script using real hash records and verification.
func TestIntegration_ShebangChainRunnerExecution(t *testing.T) {
	hashDir := commontesting.SafeTempDir(t)
	scriptDir := commontesting.SafeTempDir(t)

	// Build a local interpreter binary that references socket(2).
	// This makes the risk signal deterministic in CI and local runs.
	interpPath := buildNetworkInterpreterBinary(t, scriptDir)

	// Step 1 (record phase): create script and record it.
	// SaveRecord stores both the script record and shebang interpreter records.
	scriptContent := "#!" + interpPath + "\n--version\n"
	scriptPath := commontesting.WriteExecutableFile(t, scriptDir, "network-tool.sh", []byte(scriptContent))
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(scriptPath, false)
	require.NoError(t, err)

	// Step 2 (runner phase): execute using a real runner + verification manager.
	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	configSpec := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: commontesting.Int32Ptr(30),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:       "shebang-chain-risk-group",
				CmdAllowed: []string{scriptPath},
				Commands: []runnertypes.CommandSpec{
					{
						Name:      "shebang-network-script",
						Cmd:       scriptPath,
						RiskLevel: runnertypes.RiskLevelHighPtr,
					},
				},
			},
		},
	}

	r, err := NewRunner(
		configSpec,
		WithRunID("test-run-shebang-chain-risk"),
		WithVerificationManager(verificationManager),
	)
	require.NoError(t, err)
	require.NoError(t, r.LoadSystemEnvironment())

	err = r.Execute(context.Background(), nil)
	assert.NoError(t, err)
}

// TestIntegration_ShebangChainRiskRejectsLowRisk verifies the strict rejection
// path: shebang-chain interpreter analysis elevates effective risk and a command
// with low risk_level is denied.
func TestIntegration_ShebangChainRiskRejectsLowRisk(t *testing.T) {
	hashDir := commontesting.SafeTempDir(t)
	scriptDir := commontesting.SafeTempDir(t)

	interpPath := buildNetworkInterpreterBinary(t, scriptDir)

	scriptContent := "#!" + interpPath + "\n--version\n"
	scriptPath := commontesting.WriteExecutableFile(t, scriptDir, "network-tool-reject.sh", []byte(scriptContent))
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(scriptPath, false)
	require.NoError(t, err)

	// Inject explicit high-risk signals into the interpreter record to make
	// rejection deterministic regardless of toolchain/linker symbol variance.
	interpResolved, err := common.NewResolvedPath(interpPath)
	require.NoError(t, err)
	store := validator.Store()
	require.NotNil(t, store)
	interpRecord, err := store.Load(interpResolved)
	require.NoError(t, err)
	interpRecord.SymbolAnalysis = &fileanalysis.SymbolAnalysisData{
		DetectedSymbols:    []string{"socket"},
		DynamicLoadSymbols: []string{"dlopen"},
	}
	require.NoError(t, store.Save(interpResolved, interpRecord))

	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	configSpec := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: commontesting.Int32Ptr(30),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:       "shebang-chain-reject-group",
				CmdAllowed: []string{scriptPath},
				Commands: []runnertypes.CommandSpec{
					{
						Name:      "shebang-network-script-reject",
						Cmd:       scriptPath,
						RiskLevel: runnertypes.RiskLevelLowPtr,
					},
				},
			},
		},
	}

	r, err := NewRunner(
		configSpec,
		WithRunID("test-run-shebang-chain-reject"),
		WithVerificationManager(verificationManager),
	)
	require.NoError(t, err)
	require.NoError(t, r.LoadSystemEnvironment())

	err = r.Execute(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, runnertypes.ErrCommandSecurityViolation)
	assert.Contains(t, err.Error(), "effective risk")
}
