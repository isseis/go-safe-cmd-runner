package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadEnvironment_GroupBasedFiltering tests that LoadEnvironment stores all variables
// without filtering and that group filtering applies correctly during execution
func TestLoadEnvironment_GroupBasedFiltering(t *testing.T) {
	// Setup clean test environment
	testSystemEnv := map[string]string{
		"SYSTEM_VAR1":   "system_value1",
		"SYSTEM_VAR2":   "system_value2",
		"SYSTEM_COMMON": "system_common_value",
		"PATH":          "/usr/bin:/bin",
	}
	setupTestEnv(t, testSystemEnv)

	// Create temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	envContent := `ENV_FILE_VAR1=env_value1
ENV_FILE_VAR2=env_value2
ENV_FILE_COMMON=env_common_value
`
	err := os.WriteFile(envFile, []byte(envContent), 0o644)
	require.NoError(t, err)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"SYSTEM_VAR1", "ENV_FILE_VAR1", "PATH"}, // Global allowlist
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name: "group1",
				EnvAllowlist: []string{
					"SYSTEM_VAR1", "ENV_FILE_VAR1", "SYSTEM_COMMON", "ENV_FILE_COMMON", "PATH",
				}, // Group1 specific allowlist
			},
			{
				Name: "group2",
				EnvAllowlist: []string{
					"SYSTEM_VAR2", "ENV_FILE_VAR2", "SYSTEM_COMMON", "ENV_FILE_COMMON", "PATH",
				}, // Group2 specific allowlist
			},
			{
				Name: "group3_no_allowlist", // Group without allowlist - should inherit global
			},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// Load environment variables from both system and .env file
	err = runner.LoadEnvironment(envFile, true)
	require.NoError(t, err)

	// Verify that LoadEnvironment stores all variables without filtering
	expectedVars := map[string]string{
		"SYSTEM_VAR1":     "system_value1",
		"SYSTEM_VAR2":     "system_value2",
		"SYSTEM_COMMON":   "system_common_value",
		"ENV_FILE_VAR1":   "env_value1",
		"ENV_FILE_VAR2":   "env_value2",
		"ENV_FILE_COMMON": "env_common_value",
		"PATH":            "/usr/bin:/bin",
	}

	for varName, expectedValue := range expectedVars {
		actualValue, exists := runner.envVars[varName]
		assert.True(t, exists, "Variable %s should be stored in runner.envVars", varName)
		assert.Equal(t, expectedValue, actualValue, "Variable %s should have correct value", varName)
	}

	t.Run("Group1 filtering behavior", func(t *testing.T) {
		// Test group1 environment variable resolution
		cmd := runnertypes.Command{Name: "test-cmd"}
		envVars, err := runner.resolveEnvironmentVars(cmd, &config.Groups[0])
		require.NoError(t, err)

		// Group1 should have its allowed variables
		assert.Equal(t, "system_value1", envVars["SYSTEM_VAR1"])
		assert.Equal(t, "env_value1", envVars["ENV_FILE_VAR1"])
		assert.Equal(t, "system_common_value", envVars["SYSTEM_COMMON"])
		assert.Equal(t, "env_common_value", envVars["ENV_FILE_COMMON"]) // .env overrides system

		// Group1 should NOT have variables not in its allowlist
		assert.NotContains(t, envVars, "SYSTEM_VAR2")
		assert.NotContains(t, envVars, "ENV_FILE_VAR2")

		// Should have PATH for command execution
		assert.Contains(t, envVars, "PATH")
	})

	t.Run("Group2 filtering behavior", func(t *testing.T) {
		// Test group2 environment variable resolution
		cmd := runnertypes.Command{Name: "test-cmd"}
		envVars, err := runner.resolveEnvironmentVars(cmd, &config.Groups[1])
		require.NoError(t, err)

		// Group2 should have its allowed variables
		assert.Equal(t, "system_value2", envVars["SYSTEM_VAR2"])
		assert.Equal(t, "env_value2", envVars["ENV_FILE_VAR2"])
		assert.Equal(t, "system_common_value", envVars["SYSTEM_COMMON"])
		assert.Equal(t, "env_common_value", envVars["ENV_FILE_COMMON"]) // .env overrides system

		// Group2 should NOT have variables not in its allowlist
		assert.NotContains(t, envVars, "SYSTEM_VAR1")
		assert.NotContains(t, envVars, "ENV_FILE_VAR1")

		// Should have PATH for command execution
		assert.Contains(t, envVars, "PATH")
	})

	t.Run("Group3 inherits global allowlist", func(t *testing.T) {
		// Test group3 (no allowlist) environment variable resolution
		cmd := runnertypes.Command{Name: "test-cmd"}
		envVars, err := runner.resolveEnvironmentVars(cmd, &config.Groups[2])
		require.NoError(t, err)

		// Group3 should inherit global allowlist
		assert.Equal(t, "system_value1", envVars["SYSTEM_VAR1"])
		assert.Equal(t, "env_value1", envVars["ENV_FILE_VAR1"])

		// Group3 should NOT have variables not in global allowlist
		assert.NotContains(t, envVars, "SYSTEM_VAR2")
		assert.NotContains(t, envVars, "ENV_FILE_VAR2")
		assert.NotContains(t, envVars, "SYSTEM_COMMON")
		assert.NotContains(t, envVars, "ENV_FILE_COMMON")

		// Should have PATH for command execution
		assert.Contains(t, envVars, "PATH")
	})
}

// TestLoadEnvironment_OverridesBehavior tests that .env file variables override system variables
func TestLoadEnvironment_OverridesBehavior(t *testing.T) {
	// Setup test environment with a variable that will be overridden
	testSystemEnv := map[string]string{
		"OVERRIDE_VAR": "system_value",
		"SYSTEM_ONLY":  "system_only_value",
		"PATH":         "/usr/bin:/bin",
	}
	setupTestEnv(t, testSystemEnv)

	// Create temporary .env file that overrides OVERRIDE_VAR
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	envContent := `OVERRIDE_VAR=env_file_value
ENV_FILE_ONLY=env_file_only_value
`
	err := os.WriteFile(envFile, []byte(envContent), 0o644)
	require.NoError(t, err)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"OVERRIDE_VAR", "SYSTEM_ONLY", "ENV_FILE_ONLY", "PATH"},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// Load environment variables from both system and .env file
	err = runner.LoadEnvironment(envFile, true)
	require.NoError(t, err)

	// Verify override behavior
	assert.Equal(t, "env_file_value", runner.envVars["OVERRIDE_VAR"], ".env file should override system variable")
	assert.Equal(t, "system_only_value", runner.envVars["SYSTEM_ONLY"], "System-only variable should be preserved")
	assert.Equal(t, "env_file_only_value", runner.envVars["ENV_FILE_ONLY"], "Env file-only variable should be loaded")
	assert.Contains(t, runner.envVars, "PATH")
}

// TestLoadEnvironment_NoSystemEnv tests loading only from .env file
func TestLoadEnvironment_NoSystemEnv(t *testing.T) {
	// Setup test environment
	testSystemEnv := map[string]string{
		"SYSTEM_VAR": "system_value",
		"PATH":       "/usr/bin:/bin",
	}
	setupTestEnv(t, testSystemEnv)

	// Create temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	envContent := `ENV_FILE_VAR=env_file_value
`
	err := os.WriteFile(envFile, []byte(envContent), 0o644)
	require.NoError(t, err)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"SYSTEM_VAR", "ENV_FILE_VAR", "PATH"},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// Load environment variables only from .env file (loadSystemEnv = false)
	err = runner.LoadEnvironment(envFile, false)
	require.NoError(t, err)

	// Should only have .env file variables, not system variables
	assert.Equal(t, "env_file_value", runner.envVars["ENV_FILE_VAR"])
	assert.NotContains(t, runner.envVars, "SYSTEM_VAR", "System variables should not be loaded when loadSystemEnv=false")
	assert.NotContains(t, runner.envVars, "PATH", "System PATH should not be loaded when loadSystemEnv=false")
}

// TestLoadEnvironment_EmptyEnvFile tests behavior when .env file is empty or not specified
func TestLoadEnvironment_EmptyEnvFile(t *testing.T) {
	// Setup test environment
	testSystemEnv := map[string]string{
		"SYSTEM_VAR": "system_value",
		"PATH":       "/usr/bin:/bin",
	}
	setupTestEnv(t, testSystemEnv)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      t.TempDir(),
			EnvAllowlist: []string{"SYSTEM_VAR", "PATH"},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// Load environment variables with empty env file path
	err = runner.LoadEnvironment("", true)
	require.NoError(t, err)

	// Should only have system variables
	assert.Equal(t, "system_value", runner.envVars["SYSTEM_VAR"])
	assert.Contains(t, runner.envVars, "PATH")
}
