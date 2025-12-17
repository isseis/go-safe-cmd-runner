//go:build test

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDryRunJSONOutput_WithDebugInfo tests that JSON dry-run output includes debug information
func TestDryRunJSONOutput_WithDebugInfo(t *testing.T) {
	// Build the runner binary
	runnerBinary := buildRunnerBinary(t)
	defer os.Remove(runnerBinary)

	// Get test config path
	configPath := filepath.Join("testdata", "dry_run_debug_test.toml")

	// Set required environment variables for the test
	env := []string{
		"DB_HOST=localhost",
		"API_KEY=secret123",
		"PATH=/usr/bin:/bin",
		"HOME=/home/test",
		"USER=testuser",
	}

	// Run dry-run with JSON format and full detail level
	output, err := runDryRun(t, runnerBinary, configPath, "json", "full", false, env)
	require.NoError(t, err, "dry-run execution should succeed")

	// Parse JSON output
	var result resource.DryRunResult
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "JSON output should be valid")

	// Verify structure
	require.NotEmpty(t, result.ResourceAnalyses, "should have resource analyses")

	// Find group analyses
	var inheritGroupAnalysis *resource.ResourceAnalysis
	var explicitGroupAnalysis *resource.ResourceAnalysis
	for i := range result.ResourceAnalyses {
		if result.ResourceAnalyses[i].Type == resource.ResourceTypeGroup {
			switch result.ResourceAnalyses[i].Target {
			case "test_group_inherit":
				inheritGroupAnalysis = &result.ResourceAnalyses[i]
			case "test_group_explicit":
				explicitGroupAnalysis = &result.ResourceAnalyses[i]
			}
		}
	}

	// Verify inherit group analysis
	require.NotNil(t, inheritGroupAnalysis, "should have inherit group analysis")
	require.NotNil(t, inheritGroupAnalysis.DebugInfo, "inherit group should have debug info")
	require.NotNil(t, inheritGroupAnalysis.DebugInfo.InheritanceAnalysis, "inherit group should have inheritance analysis")

	// Verify inheritance analysis fields for inherit group
	ia := inheritGroupAnalysis.DebugInfo.InheritanceAnalysis
	assert.Equal(t, []string{"DB_HOST=DB_HOST", "API_KEY=API_KEY"}, ia.GlobalEnvImport, "should have global env_import")
	assert.Equal(t, []string{"PATH", "HOME", "USER", "DB_HOST", "API_KEY"}, ia.GlobalAllowlist, "should have global env_allowed")
	assert.Equal(t, []string{}, ia.GroupEnvImport, "inherit group should have empty group env_import")
	assert.Equal(t, []string{}, ia.GroupAllowlist, "inherit group should have empty group env_allowed")
	assert.Equal(t, runnertypes.InheritanceModeInherit, ia.InheritanceMode, "should be inherit mode")

	// Verify difference fields for inherit group (full detail level)
	assert.NotNil(t, ia.InheritedVariables, "should have inherited variables")
	assert.Contains(t, ia.InheritedVariables, "PATH", "should inherit PATH")
	assert.Contains(t, ia.InheritedVariables, "HOME", "should inherit HOME")
	assert.Contains(t, ia.InheritedVariables, "USER", "should inherit USER")
	assert.Contains(t, ia.InheritedVariables, "DB_HOST", "should inherit DB_HOST")
	assert.Contains(t, ia.InheritedVariables, "API_KEY", "should inherit API_KEY")

	// Verify explicit group analysis
	require.NotNil(t, explicitGroupAnalysis, "should have explicit group analysis")
	require.NotNil(t, explicitGroupAnalysis.DebugInfo, "explicit group should have debug info")
	require.NotNil(t, explicitGroupAnalysis.DebugInfo.InheritanceAnalysis, "explicit group should have inheritance analysis")

	// Verify inheritance analysis fields for explicit group
	iaExplicit := explicitGroupAnalysis.DebugInfo.InheritanceAnalysis
	assert.Equal(t, []string{"DB_HOST=DB_HOST", "API_KEY=API_KEY"}, iaExplicit.GlobalEnvImport, "should have global env_import")
	assert.Equal(t, []string{"PATH", "HOME", "USER", "DB_HOST", "API_KEY"}, iaExplicit.GlobalAllowlist, "should have global env_allowed")
	assert.Equal(t, []string{"db_host=DB_HOST"}, iaExplicit.GroupEnvImport, "explicit group should have group env_import")
	assert.Equal(t, []string{"PATH", "DB_HOST"}, iaExplicit.GroupAllowlist, "explicit group should have group env_allowed")
	assert.Equal(t, runnertypes.InheritanceModeExplicit, iaExplicit.InheritanceMode, "should be explicit mode")

	// Verify difference fields for explicit group
	assert.NotNil(t, iaExplicit.RemovedAllowlistVariables, "should have removed allowlist variables")
	assert.Contains(t, iaExplicit.RemovedAllowlistVariables, "HOME", "should show HOME as removed")
	assert.Contains(t, iaExplicit.RemovedAllowlistVariables, "USER", "should show USER as removed")
	assert.Contains(t, iaExplicit.RemovedAllowlistVariables, "API_KEY", "should show API_KEY as removed")

	assert.NotNil(t, iaExplicit.UnavailableEnvImportVariables, "should have unavailable env_import variables")
	assert.Contains(t, iaExplicit.UnavailableEnvImportVariables, "API_KEY", "should show API_KEY as unavailable")

	// Find command analyses
	var commandAnalyses []*resource.ResourceAnalysis
	for i := range result.ResourceAnalyses {
		if result.ResourceAnalyses[i].Type == resource.ResourceTypeCommand {
			commandAnalyses = append(commandAnalyses, &result.ResourceAnalyses[i])
		}
	}

	// Verify at least one command has debug info with final environment
	require.NotEmpty(t, commandAnalyses, "should have command analyses")
	foundFinalEnv := false
	for _, ca := range commandAnalyses {
		if ca.DebugInfo != nil && ca.DebugInfo.FinalEnvironment != nil {
			foundFinalEnv = true
			// Verify final environment structure
			assert.NotEmpty(t, ca.DebugInfo.FinalEnvironment.Variables, "should have environment variables")

			// Check that variables have required fields
			for name, envVar := range ca.DebugInfo.FinalEnvironment.Variables {
				assert.NotEmpty(t, envVar.Source, "variable %s should have source", name)
				assert.Contains(t, []string{"system", "env_import", "vars", "command"}, envVar.Source,
					"variable %s should have valid source", name)
			}
		}
	}
	assert.True(t, foundFinalEnv, "at least one command should have final environment debug info")
}

// TestDryRunJSONOutput_DetailLevels tests that different detail levels produce appropriate output
func TestDryRunJSONOutput_DetailLevels(t *testing.T) {
	// Build the runner binary
	runnerBinary := buildRunnerBinary(t)
	defer os.Remove(runnerBinary)

	// Get test config path
	configPath := filepath.Join("testdata", "dry_run_debug_test.toml")

	// Set required environment variables
	env := []string{
		"DB_HOST=localhost",
		"API_KEY=secret123",
		"PATH=/usr/bin:/bin",
		"HOME=/home/test",
		"USER=testuser",
	}

	tests := []struct {
		name             string
		detailLevel      string
		expectDebugInfo  bool
		expectDiffFields bool
		expectFinalEnv   bool
	}{
		{
			name:             "summary - no debug info",
			detailLevel:      "summary",
			expectDebugInfo:  false,
			expectDiffFields: false,
			expectFinalEnv:   false,
		},
		{
			name:             "detailed - basic info only",
			detailLevel:      "detailed",
			expectDebugInfo:  true,
			expectDiffFields: false,
			expectFinalEnv:   false,
		},
		{
			name:             "full - all info",
			detailLevel:      "full",
			expectDebugInfo:  true,
			expectDiffFields: true,
			expectFinalEnv:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run dry-run with specified detail level
			output, err := runDryRun(t, runnerBinary, configPath, "json", tt.detailLevel, false, env)
			require.NoError(t, err, "dry-run execution should succeed")

			// Parse JSON output
			var result resource.DryRunResult
			err = json.Unmarshal([]byte(output), &result)
			require.NoError(t, err, "JSON output should be valid")

			// Check group analysis
			groupAnalysis := findResourceAnalysisByTypeAndTarget(result, resource.ResourceTypeGroup, "test_group_inherit")
			require.NotNil(t, groupAnalysis, "should have group analysis")

			if tt.expectDebugInfo {
				assert.NotNil(t, groupAnalysis.DebugInfo, "should have debug info")
				assert.NotNil(t, groupAnalysis.DebugInfo.InheritanceAnalysis, "should have inheritance analysis")

				if tt.expectDiffFields {
					// For full level, difference fields should be present
					ia := groupAnalysis.DebugInfo.InheritanceAnalysis
					assert.NotNil(t, ia.InheritedVariables, "should have inherited variables")
					assert.NotEmpty(t, ia.InheritedVariables, "inherited variables should not be empty for inherit mode")
				} else {
					// For detailed level, difference fields should be nil or empty
					ia := groupAnalysis.DebugInfo.InheritanceAnalysis
					if ia.InheritedVariables != nil {
						assert.Empty(t, ia.InheritedVariables, "inherited variables should be empty for detailed level")
					}
				}
			} else if groupAnalysis.DebugInfo != nil {
				// For summary level, DebugInfo may exist but should be empty (no InheritanceAnalysis or FinalEnvironment)
				assert.Nil(t, groupAnalysis.DebugInfo.InheritanceAnalysis, "should not have inheritance analysis for summary level")
				assert.Nil(t, groupAnalysis.DebugInfo.FinalEnvironment, "should not have final environment for summary level")
			}

			// Check command analysis
			commandAnalyses := findAllResourceAnalysesByType(result, resource.ResourceTypeCommand)
			require.NotEmpty(t, commandAnalyses, "should have command analyses")

			if tt.expectFinalEnv {
				// At least one command should have final environment
				foundFinalEnv := false
				for _, ca := range commandAnalyses {
					if ca.DebugInfo != nil && ca.DebugInfo.FinalEnvironment != nil {
						foundFinalEnv = true
						break
					}
				}
				assert.True(t, foundFinalEnv, "at least one command should have final environment for full detail level")
			} else {
				// No command should have final environment
				for _, ca := range commandAnalyses {
					if ca.DebugInfo != nil {
						assert.Nil(t, ca.DebugInfo.FinalEnvironment, "should not have final environment for %s detail level", tt.detailLevel)
					}
				}
			}
		})
	}
}

// TestDryRunTextOutput_Unchanged tests that text output format is unchanged after modifications
func TestDryRunTextOutput_Unchanged(t *testing.T) {
	// Build the runner binary
	runnerBinary := buildRunnerBinary(t)
	defer os.Remove(runnerBinary)

	// Get test config path
	configPath := filepath.Join("testdata", "dry_run_debug_test.toml")

	// Set required environment variables
	env := []string{
		"DB_HOST=localhost",
		"API_KEY=secret123",
		"PATH=/usr/bin:/bin",
		"HOME=/home/test",
		"USER=testuser",
	}

	// Run dry-run with text format and full detail level
	output, err := runDryRun(t, runnerBinary, configPath, "text", "full", false, env)
	require.NoError(t, err, "dry-run execution should succeed")

	// Verify text output contains expected sections
	assert.Contains(t, output, "===== Variable Expansion Debug Information =====",
		"should have debug information header")
	assert.Contains(t, output, "from_env Inheritance Analysis",
		"should have inheritance analysis section")
	assert.Contains(t, output, "[Global Level]",
		"should have global level section")
	assert.Contains(t, output, "[Group:",
		"should have group section")
	// Note: The text format shows "----- Allowlist Inheritance -----" instead of "[Inheritance Mode]"
	assert.Contains(t, output, "----- Allowlist Inheritance -----",
		"should have allowlist inheritance section")
	assert.Contains(t, output, "----- Final Process Environment -----",
		"should have final environment section")

	// Verify group-specific sections
	assert.Contains(t, output, "test_group_inherit",
		"should show inherit group name")
	assert.Contains(t, output, "test_group_explicit",
		"should show explicit group name")
}

// TestDryRunSensitiveMasking tests that sensitive information is properly masked
func TestDryRunSensitiveMasking(t *testing.T) {
	// Build the runner binary
	runnerBinary := buildRunnerBinary(t)
	defer os.Remove(runnerBinary)

	// Get test config path
	configPath := filepath.Join("testdata", "dry_run_debug_test.toml")

	// Set required environment variables (including sensitive ones)
	env := []string{
		"DB_HOST=localhost",
		"API_KEY=secret123",
		"PASSWORD=supersecret",
		"TOKEN=authtoken",
		"PATH=/usr/bin:/bin",
		"HOME=/home/test",
		"USER=testuser",
	}

	tests := []struct {
		name          string
		showSensitive bool
		shouldMask    bool
	}{
		{
			name:          "with masking (default)",
			showSensitive: false,
			shouldMask:    true,
		},
		{
			name:          "without masking (show-sensitive)",
			showSensitive: true,
			shouldMask:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run dry-run with JSON format and full detail level
			output, err := runDryRun(t, runnerBinary, configPath, "json", "full", tt.showSensitive, env)
			require.NoError(t, err, "dry-run execution should succeed")

			// Parse JSON output
			var result resource.DryRunResult
			err = json.Unmarshal([]byte(output), &result)
			require.NoError(t, err, "JSON output should be valid")

			// Find command with final environment
			var cmdWithEnv *resource.ResourceAnalysis
			for i := range result.ResourceAnalyses {
				if result.ResourceAnalyses[i].Type == resource.ResourceTypeCommand &&
					result.ResourceAnalyses[i].DebugInfo != nil &&
					result.ResourceAnalyses[i].DebugInfo.FinalEnvironment != nil {
					cmdWithEnv = &result.ResourceAnalyses[i]
					break
				}
			}

			require.NotNil(t, cmdWithEnv, "should have command with final environment")
			require.NotNil(t, cmdWithEnv.DebugInfo.FinalEnvironment, "should have final environment")

			// Check for sensitive variables
			sensitiveVars := []string{"API_KEY", "PASSWORD", "TOKEN"}
			for _, varName := range sensitiveVars {
				if envVar, exists := cmdWithEnv.DebugInfo.FinalEnvironment.Variables[varName]; exists {
					if tt.shouldMask {
						assert.True(t, envVar.Masked, "variable %s should be masked", varName)
						assert.Empty(t, envVar.Value, "masked variable %s should have empty value", varName)
					} else {
						assert.False(t, envVar.Masked, "variable %s should not be masked with --show-sensitive", varName)
						assert.NotEmpty(t, envVar.Value, "variable %s should have value with --show-sensitive", varName)
					}
				}
			}
		})
	}
}

// Helper functions

// buildRunnerBinary builds the runner binary for testing
func buildRunnerBinary(t *testing.T) string {
	t.Helper()

	// Create temporary directory for binary
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "runner")

	// Build the binary
	cmd := exec.Command("go", "build", "-tags", "test", "-o", binaryPath, ".")
	cmd.Dir = "." // Current directory (cmd/runner)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "failed to build runner binary: %s", stderr.String())

	return binaryPath
}

// runDryRun executes the runner binary in dry-run mode and returns the output
func runDryRun(t *testing.T, binaryPath, configPath, format, detailLevel string, showSensitive bool, env []string) (string, error) {
	t.Helper()

	// Prepare arguments
	args := []string{
		"--config", configPath,
		"--dry-run",
		"--dry-run-format", format,
		"--dry-run-detail", detailLevel,
	}

	if showSensitive {
		args = append(args, "--show-sensitive")
	}

	// Execute the command
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("command failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// findResourceAnalysisByTypeAndTarget finds a resource analysis by type and target
func findResourceAnalysisByTypeAndTarget(result resource.DryRunResult, resType resource.ResourceType, target string) *resource.ResourceAnalysis {
	for i := range result.ResourceAnalyses {
		if result.ResourceAnalyses[i].Type == resType && result.ResourceAnalyses[i].Target == target {
			return &result.ResourceAnalyses[i]
		}
	}
	return nil
}

// findAllResourceAnalysesByType finds all resource analyses of a given type
func findAllResourceAnalysesByType(result resource.DryRunResult, resType resource.ResourceType) []*resource.ResourceAnalysis {
	var analyses []*resource.ResourceAnalysis
	for i := range result.ResourceAnalyses {
		if result.ResourceAnalyses[i].Type == resType {
			analyses = append(analyses, &result.ResourceAnalyses[i])
		}
	}
	return analyses
}

// TestJSONValidStructure tests that JSON output is well-formed and can be parsed by standard tools
func TestJSONValidStructure(t *testing.T) {
	// Build the runner binary
	runnerBinary := buildRunnerBinary(t)
	defer os.Remove(runnerBinary)

	// Get test config path
	configPath := filepath.Join("testdata", "dry_run_debug_test.toml")

	// Set required environment variables
	env := []string{
		"DB_HOST=localhost",
		"API_KEY=secret123",
		"PATH=/usr/bin:/bin",
		"HOME=/home/test",
		"USER=testuser",
	}

	// Run dry-run with JSON format
	output, err := runDryRun(t, runnerBinary, configPath, "json", "full", false, env)
	require.NoError(t, err, "dry-run execution should succeed")

	// Test 1: Valid JSON structure
	var jsonData any
	err = json.Unmarshal([]byte(output), &jsonData)
	require.NoError(t, err, "output should be valid JSON")

	// Test 2: Can be parsed into specific struct
	var result resource.DryRunResult
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "output should match DryRunResult structure")

	// Test 3: Round-trip test (marshal and unmarshal)
	remarshaled, err := json.Marshal(result)
	require.NoError(t, err, "should be able to marshal result")

	var result2 resource.DryRunResult
	err = json.Unmarshal(remarshaled, &result2)
	require.NoError(t, err, "should be able to unmarshal remarshaled data")

	// Test 4: Verify omitempty works correctly (no null fields for optional data)
	outputStr := output
	assert.NotContains(t, outputStr, `:null`, "should not contain null values (omitempty should work)")

	// Test 5: Test with jq-like processing (check for specific fields)
	// This simulates what users would do with jq
	assert.True(t, strings.Contains(outputStr, `"resource_analyses"`), "should have resource_analyses field")
	assert.True(t, strings.Contains(outputStr, `"debug_info"`), "should have debug_info field in some analyses")
	assert.True(t, strings.Contains(outputStr, `"inheritance_analysis"`), "should have inheritance_analysis field")
}
