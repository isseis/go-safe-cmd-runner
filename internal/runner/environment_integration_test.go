package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvironmentFilteringIntegration tests environment variable filtering using actual printenv command
func TestEnvironmentFilteringIntegration(t *testing.T) {
	// Create temporary .env file for testing
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	envContent := `ENV_FILE_VAR1=env_value1
ENV_FILE_VAR2=env_value2
ENV_FILE_COMMON=env_common_value
ENV_FILE_GLOBAL=env_global_value
`
	err := os.WriteFile(envFile, []byte(envContent), 0o644)
	require.NoError(t, err)

	// Set system environment variables for testing
	testSystemEnv := map[string]string{
		"SYSTEM_VAR1":   "system_value1",
		"SYSTEM_VAR2":   "system_value2",
		"SYSTEM_COMMON": "system_common_value",
		"SYSTEM_GLOBAL": "system_global_value",
		"PATH":          "/usr/bin:/bin:/usr/local/bin", // Keep PATH for printenv to work
	}
	cleanup := setupTestEnv(t, testSystemEnv)
	defer cleanup()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"SYSTEM_GLOBAL", "ENV_FILE_GLOBAL", "PATH"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name: "group1",
				EnvAllowlist: []string{
					"SYSTEM_VAR1", "ENV_FILE_VAR1", "SYSTEM_COMMON", "ENV_FILE_COMMON", "PATH",
				},
				Commands: []runnertypes.Command{
					{
						Name: "printenv-group1",
						Cmd:  "printenv",
					},
				},
			},
			{
				Name: "group2",
				EnvAllowlist: []string{
					"SYSTEM_VAR2", "ENV_FILE_VAR2", "SYSTEM_COMMON", "ENV_FILE_COMMON", "PATH",
				},
				Commands: []runnertypes.Command{
					{
						Name: "printenv-group2",
						Cmd:  "printenv",
					},
				},
			},
			{
				Name: "group3_no_allowlist", // Group without allowlist - should inherit global
				Commands: []runnertypes.Command{
					{
						Name: "printenv-group3",
						Cmd:  "printenv",
					},
				},
			},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// Load environment variables from both system and .env file
	err = runner.LoadEnvironment(envFile, true)
	require.NoError(t, err)

	t.Run("Group1 filtering behavior", func(t *testing.T) {
		ctx := context.Background()
		result, err := runner.executeCommandInGroup(ctx, config.Groups[0].Commands[0], &config.Groups[0])
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)

		// Parse printenv output
		envVars := parseEnvOutput(result.Stdout)

		// Group1 should have its allowed variables
		assert.Equal(t, "system_value1", envVars["SYSTEM_VAR1"])
		assert.Equal(t, "env_value1", envVars["ENV_FILE_VAR1"])
		assert.Equal(t, "system_common_value", envVars["SYSTEM_COMMON"])
		assert.Equal(t, "env_common_value", envVars["ENV_FILE_COMMON"]) // .env overrides system

		// Group1 should NOT have variables not in its allowlist
		assert.NotContains(t, envVars, "SYSTEM_VAR2")
		assert.NotContains(t, envVars, "ENV_FILE_VAR2")
		assert.NotContains(t, envVars, "SYSTEM_GLOBAL")
		assert.NotContains(t, envVars, "ENV_FILE_GLOBAL")

		// Should have PATH for command execution
		assert.Contains(t, envVars, "PATH")
	})

	t.Run("Group2 filtering behavior", func(t *testing.T) {
		ctx := context.Background()
		result, err := runner.executeCommandInGroup(ctx, config.Groups[1].Commands[0], &config.Groups[1])
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)

		// Parse printenv output
		envVars := parseEnvOutput(result.Stdout)

		// Group2 should have its allowed variables
		assert.Equal(t, "system_value2", envVars["SYSTEM_VAR2"])
		assert.Equal(t, "env_value2", envVars["ENV_FILE_VAR2"])
		assert.Equal(t, "system_common_value", envVars["SYSTEM_COMMON"])
		assert.Equal(t, "env_common_value", envVars["ENV_FILE_COMMON"]) // .env overrides system

		// Group2 should NOT have variables not in its allowlist
		assert.NotContains(t, envVars, "SYSTEM_VAR1")
		assert.NotContains(t, envVars, "ENV_FILE_VAR1")
		assert.NotContains(t, envVars, "SYSTEM_GLOBAL")
		assert.NotContains(t, envVars, "ENV_FILE_GLOBAL")

		// Should have PATH for command execution
		assert.Contains(t, envVars, "PATH")
	})

	t.Run("Group3 inherits global allowlist", func(t *testing.T) {
		ctx := context.Background()
		result, err := runner.executeCommandInGroup(ctx, config.Groups[2].Commands[0], &config.Groups[2])
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)

		// Parse printenv output
		envVars := parseEnvOutput(result.Stdout)

		// Group3 should inherit global allowlist
		assert.Equal(t, "system_global_value", envVars["SYSTEM_GLOBAL"])
		assert.Equal(t, "env_global_value", envVars["ENV_FILE_GLOBAL"])

		// Group3 should NOT have group-specific variables (not in global allowlist)
		assert.NotContains(t, envVars, "SYSTEM_VAR1")
		assert.NotContains(t, envVars, "SYSTEM_VAR2")
		assert.NotContains(t, envVars, "ENV_FILE_VAR1")
		assert.NotContains(t, envVars, "ENV_FILE_VAR2")
		assert.NotContains(t, envVars, "SYSTEM_COMMON")
		assert.NotContains(t, envVars, "ENV_FILE_COMMON")

		// Should have PATH for command execution
		assert.Contains(t, envVars, "PATH")
	})
}

// TestEnvironmentFilteringWithoutGlobalConfig tests the scenario where no global allowlist is defined
func TestEnvironmentFilteringWithoutGlobalConfig(t *testing.T) {
	// Create temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	envContent := `APP_CONFIG=from_env_file
DATABASE_URL=postgres://localhost/test
`
	err := os.WriteFile(envFile, []byte(envContent), 0o644)
	require.NoError(t, err)

	// Set system environment variables
	testSystemEnv := map[string]string{
		"HOME":       "/home/testuser",
		"USER":       "testuser",
		"APP_CONFIG": "from_system", // This should be overridden by .env
		"PATH":       "/usr/bin:/bin",
	}
	cleanup := setupTestEnv(t, testSystemEnv)
	defer cleanup()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir: tmpDir,
			// No global EnvAllowlist defined
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "app-group",
				EnvAllowlist: []string{"APP_CONFIG", "DATABASE_URL", "HOME", "PATH"},
				Commands: []runnertypes.Command{
					{
						Name: "check-env",
						Cmd:  "printenv",
					},
				},
			},
			{
				Name:         "minimal-group",
				EnvAllowlist: []string{"PATH"}, // Only PATH allowed
				Commands: []runnertypes.Command{
					{
						Name: "check-minimal-env",
						Cmd:  "printenv",
					},
				},
			},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// Load environment from both system and .env file
	err = runner.LoadEnvironment(envFile, true)
	require.NoError(t, err)

	t.Run("App group gets only allowed variables", func(t *testing.T) {
		ctx := context.Background()
		result, err := runner.executeCommandInGroup(ctx, config.Groups[0].Commands[0], &config.Groups[0])
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)

		envVars := parseEnvOutput(result.Stdout)

		// Should have allowed variables
		assert.Equal(t, "from_env_file", envVars["APP_CONFIG"]) // .env overrides system
		assert.Equal(t, "postgres://localhost/test", envVars["DATABASE_URL"])
		assert.Equal(t, "/home/testuser", envVars["HOME"])
		assert.Contains(t, envVars, "PATH")

		// Should NOT have non-allowed variables
		assert.NotContains(t, envVars, "USER")
	})

	t.Run("Minimal group gets only PATH", func(t *testing.T) {
		ctx := context.Background()
		result, err := runner.executeCommandInGroup(ctx, config.Groups[1].Commands[0], &config.Groups[1])
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)

		envVars := parseEnvOutput(result.Stdout)

		// Should only have PATH
		assert.Contains(t, envVars, "PATH")

		// Should NOT have any other variables
		assert.NotContains(t, envVars, "APP_CONFIG")
		assert.NotContains(t, envVars, "DATABASE_URL")
		assert.NotContains(t, envVars, "HOME")
		assert.NotContains(t, envVars, "USER")

		// Verify only PATH is present (plus any variables that printenv might add)
		nonPathVars := 0
		for varName := range envVars {
			if varName != "PATH" && !strings.HasPrefix(varName, "_") {
				nonPathVars++
			}
		}
		assert.Equal(t, 0, nonPathVars, "Expected only PATH variable, but found: %v", envVars)
	})
}

// parseEnvOutput parses the output of printenv command into a map
func parseEnvOutput(output string) map[string]string {
	envVars := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Split on first = only
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	return envVars
}
