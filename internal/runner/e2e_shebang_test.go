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
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// Simulate a missing interpreter record by deleting the interpreter's hash file.
	interpPath, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)
	interpHashPath, err := validator.GetHashFilePath(common.ResolvedPath(interpPath))
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
