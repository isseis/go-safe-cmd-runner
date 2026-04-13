//go:build test

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_TOCTOU_RunnerFailsOnWorldWritableVerifyFilesDir tests that the runner
// exits with an error when verify_files references a file in a world-writable directory.
// This validates AC-M2S-7: runner aborts on TOCTOU permission violations.
func TestE2E_TOCTOU_RunnerFailsOnWorldWritableVerifyFilesDir(t *testing.T) {
	// Create a world-writable directory with a file inside it.
	// The TOCTOU check inspects the parent directory of verify_files entries.
	tmpDir := commontesting.SafeTempDir(t)
	err := os.Chmod(tmpDir, 0o777)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chmod(tmpDir, 0o755)
	})

	// Create a dummy file inside the world-writable directory
	targetFile := filepath.Join(tmpDir, "target.txt")
	err = os.WriteFile(targetFile, []byte("hello"), 0o644)
	require.NoError(t, err)

	// Create a config file that references the file in the world-writable directory
	configDir := commontesting.SafeTempDir(t)
	configFile := filepath.Join(configDir, "config.toml")
	tomlContent := `version = "1.0"

[global]
verify_files = ["` + targetFile + `"]

[[groups]]
name = "test"

[[groups.commands]]
name = "echo"
cmd = "/bin/echo"
args = ["hello"]
`
	err = os.WriteFile(configFile, []byte(tomlContent), 0o644)
	require.NoError(t, err)

	// Run the runner in dry-run mode to skip hash verification
	cmd := exec.Command("go", "run", ".", "-config", configFile, "-dry-run")
	cmd.Dir = "."

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	// The runner should fail because of the TOCTOU violation
	require.Error(t, err, "runner should fail when verify_files dir is world-writable")

	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1")

	// Verify that the output mentions the TOCTOU failure
	combined := stdout.String() + stderr.String()
	assert.True(t,
		strings.Contains(combined, "TOCTOU") || strings.Contains(combined, "permission") || strings.Contains(combined, "file_access_error"),
		"output should mention the permission check failure: %s", combined,
	)
}
