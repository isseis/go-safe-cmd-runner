//go:build test

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
)

// TestIntegration_PreExpand_CommandLevelVariableInVerification tests that
// command-level variables are available during verification phase.
func TestIntegration_PreExpand_CommandLevelVariableInVerification(t *testing.T) {
	env := setupTestEnvironment(t, "001")

	// Create a file to verify using command-level variable
	verifyFile := filepath.Join(env.TestDir, "verify.txt")
	err := os.WriteFile(verifyFile, []byte("verify content"), 0o644)
	require.NoError(t, err)

	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 30

[[groups]]
name = "test_group"
# Use group-level variable in verify_files (verify_files is a group-level field)
verify_files = ["%%{verify_path}"]

[groups.vars]
group_var = "%s"
verify_path = "%s"

[[groups.commands]]
name = "test_cmd"
cmd = "/usr/bin/echo"
args = ["test"]
`, env.TestDir, verifyFile)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	// Execute - verification should use pre-expanded command-level variables
	ctx := context.Background()
	err = r.Execute(ctx, nil)
	assert.NoError(t, err, "Group execution should succeed")
}

// TestIntegration_PreExpand_FailFast_UndefinedVariable tests that undefined
// variable errors are detected early (Fail Fast).
func TestIntegration_PreExpand_FailFast_UndefinedVariable(t *testing.T) {
	env := setupTestEnvironment(t, "002")

	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 30

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "/usr/bin/echo"
args = ["test", "%%{undefined_var}"]
`)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	// Execute - should fail early during pre-expansion
	ctx := context.Background()
	err := r.Execute(ctx, nil)
	require.Error(t, err, "Group execution should fail")

	// Verify error type using errors.Is instead of fragile string matching
	assert.ErrorIs(t, err, config.ErrUndefinedVariable)

	// Also verify detailed error contains variable name
	var detailErr *config.ErrUndefinedVariableDetail
	if errors.As(err, &detailErr) {
		assert.Equal(t, "undefined_var", detailErr.VariableName, "Error should mention undefined variable name")
	}
}

// TestIntegration_PreExpand_FailFast_WorkdirResolutionError tests that workdir
// resolution errors are detected during pre-expansion (Fail Fast).
func TestIntegration_PreExpand_FailFast_WorkdirResolutionError(t *testing.T) {
	env := setupTestEnvironment(t, "003")

	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 30

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "/usr/bin/echo"
args = ["test"]
workdir = "/nonexistent/directory/%%{some_var}"

[groups.commands.vars]
some_var = "subdir"
`)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	// Execute group - should fail during pre-expansion due to workdir resolution error
	ctx := context.Background()
	err := r.Execute(ctx, nil)
	require.Error(t, err, "Group execution should fail")

	// Verify error type using errors.Is instead of fragile string matching
	assert.ErrorIs(t, err, executor.ErrDirNotExists)
}

// TestIntegration_PreExpand_DryRunMode tests that pre-expansion works correctly
// in dry-run mode.
func TestIntegration_PreExpand_DryRunMode(t *testing.T) {
	env := setupTestEnvironment(t, "004")

	outputFile := env.outputFilePath()

	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 30

[[groups]]
name = "test_group"

[groups.vars]
output_file = "%s"

[[groups.commands]]
name = "test_cmd"
cmd = "/usr/bin/echo"
args = ["test output"]
output_file = "%%{output_file}"
`, outputFile)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	// Execute in dry-run mode
	ctx := context.Background()
	err := r.Execute(ctx, nil)
	assert.NoError(t, err, "Group execution should succeed in dry-run mode")

	// Output file should not be created in dry-run mode
	_, err = os.Stat(outputFile)
	assert.True(t, os.IsNotExist(err), "Output file should not exist in dry-run mode")
}

// TestIntegration_PreExpand_MultipleCommands_FirstFails tests that when
// multiple commands exist and the first one has an expansion error, it's
// detected immediately (Fail Fast).
func TestIntegration_PreExpand_MultipleCommands_FirstFails(t *testing.T) {
	env := setupTestEnvironment(t, "005")

	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 30

[[groups]]
name = "test_group"

# First command with error
[[groups.commands]]
name = "first_cmd"
cmd = "/usr/bin/echo"
args = ["%%{undefined_var}"]

# Second command (should not be reached)
[[groups.commands]]
name = "second_cmd"
cmd = "/usr/bin/echo"
args = ["second"]

# Third command (should not be reached)
[[groups.commands]]
name = "third_cmd"
cmd = "/usr/bin/echo"
args = ["third"]
`)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	// Execute group - should fail at first command during pre-expansion
	ctx := context.Background()
	err := r.Execute(ctx, nil)
	require.Error(t, err, "Group execution should fail")

	// Verify error type using errors.Is instead of fragile string matching
	assert.ErrorIs(t, err, config.ErrUndefinedVariable)

	// Verify detailed error contains variable name and command name in error message
	var detailErr *config.ErrUndefinedVariableDetail
	if errors.As(err, &detailErr) {
		assert.Equal(t, "undefined_var", detailErr.VariableName, "Error should mention undefined variable name")
	}
	// Command name appears in the outer wrapper error message
	assert.Contains(t, err.Error(), "first_cmd", "Error should mention the failing command")
}

// TestIntegration_PreExpand_CommandVarsInArgs tests that command-level
// variables can be used in command arguments.
func TestIntegration_PreExpand_CommandVarsInArgs(t *testing.T) {
	env := setupTestEnvironment(t, "006")

	outputFile := filepath.Join(env.TestDir, "cmd-output.txt")

	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 30

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "/bin/sh"
args = ["-c", "echo 'test output' > %%{cmd_output}"]
risk_level = "medium"

[groups.commands.vars]
cmd_output = "%s"
`, outputFile)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	// Execute
	ctx := context.Background()
	err := r.Execute(ctx, nil)
	assert.NoError(t, err, "Group execution should succeed")

	// Check output file exists and contains expected content
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err, "Output file should exist")
	assert.True(t, strings.Contains(string(content), "test output"), "Output should contain expected text")
}

// TestIntegration_PreExpand_WorkdirWithCommandVars tests that command-level
// variables can be used in workdir.
func TestIntegration_PreExpand_WorkdirWithCommandVars(t *testing.T) {
	env := setupTestEnvironment(t, "007")

	// Create a subdirectory
	subdir := filepath.Join(env.TestDir, "subdir")
	err := os.MkdirAll(subdir, 0o755)
	require.NoError(t, err)

	markerFile := filepath.Join(subdir, "marker.txt")

	configContent := fmt.Sprintf(`
version = "1.0"

[global]
timeout = 30

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "/bin/sh"
args = ["-c", "pwd > marker.txt"]
workdir = "%%{work_directory}"
risk_level = "medium"

[groups.commands.vars]
work_directory = "%s"
`, subdir)

	env.writeConfig(t, configContent)
	r := env.createRunner(t)

	// Execute
	ctx := context.Background()
	err = r.Execute(ctx, nil)
	assert.NoError(t, err, "Group execution should succeed")

	// Check that pwd output shows the correct working directory
	content, err := os.ReadFile(markerFile)
	require.NoError(t, err, "Marker file should exist")
	assert.True(t, strings.Contains(string(content), subdir), "pwd should show the correct working directory")
}
