package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadEnvironment_SystemVariableValidation tests that system environment variables
// are loaded without validation (validation is deferred to execution time)
func TestLoadEnvironment_SystemVariableValidation(t *testing.T) {
	// Create clean test environment to avoid interference from system variables
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	// Create temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	envContent := `SAFE_VAR=safe_value
`
	err := os.WriteFile(envFile, []byte(envContent), 0o644)
	require.NoError(t, err)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"SAFE_VAR", "DANGEROUS_SYSTEM", "PATH", "HOME", "USER"},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	t.Run("Safe system variables should be loaded", func(t *testing.T) {
		// Load environment with safe system variables
		err = runner.LoadEnvironment(envFile, true)
		require.NoError(t, err)

		// Should have safe variables
		assert.Equal(t, "safe_value", runner.envVars["SAFE_VAR"])
		assert.Contains(t, runner.envVars, "PATH")
		assert.Contains(t, runner.envVars, "HOME")
		assert.Contains(t, runner.envVars, "USER")
	})

	t.Run("Dangerous system variables should be loaded (validation deferred)", func(t *testing.T) {
		// Set a dangerous system environment variable
		dangerousCleanup := setupTestEnv(t, map[string]string{
			"DANGEROUS_SYSTEM": "value; rm -rf /",
			"PATH":             "/usr/bin:/bin",
			"HOME":             "/home/test",
			"USER":             "test",
		})
		defer dangerousCleanup()

		// Create new runner for this test
		dangerousRunner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Should succeed in loading (validation deferred to execution time)
		err = dangerousRunner.LoadEnvironment(envFile, true)
		assert.NoError(t, err, "LoadEnvironment should succeed - validation is deferred to execution time")

		// Dangerous variable should be loaded but not validated yet
		assert.Equal(t, "value; rm -rf /", dangerousRunner.envVars["DANGEROUS_SYSTEM"])
	})
}

// TestLoadEnvironment_EnvFileVariableValidation tests that .env file variables
// are loaded without validation (validation is deferred to execution time)
func TestLoadEnvironment_EnvFileVariableValidation(t *testing.T) {
	// Setup clean test environment
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"SAFE_VAR", "DANGEROUS_VAR", "PATH", "HOME", "USER"},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	t.Run("Safe .env file variables should be loaded", func(t *testing.T) {
		// Create .env file with safe variables
		safeEnvFile := filepath.Join(tmpDir, "safe.env")
		safeContent := `SAFE_VAR=safe_value
PATH=/usr/bin:/bin
`
		err := os.WriteFile(safeEnvFile, []byte(safeContent), 0o644)
		require.NoError(t, err)

		err = runner.LoadEnvironment(safeEnvFile, true)
		require.NoError(t, err)

		assert.Equal(t, "safe_value", runner.envVars["SAFE_VAR"])
		assert.Equal(t, "/usr/bin:/bin", runner.envVars["PATH"])
	})

	t.Run("Dangerous .env file variables should be loaded (validation deferred)", func(t *testing.T) {
		// Create .env file with dangerous variable
		dangerousEnvFile := filepath.Join(tmpDir, "dangerous.env")
		dangerousContent := `DANGEROUS_VAR=value; rm -rf /
SAFE_VAR=safe_value
`
		err := os.WriteFile(dangerousEnvFile, []byte(dangerousContent), 0o644)
		require.NoError(t, err)

		// Create new runner for this test
		dangerousRunner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Should succeed in loading (validation deferred to execution time)
		err = dangerousRunner.LoadEnvironment(dangerousEnvFile, true)
		assert.NoError(t, err, "LoadEnvironment should succeed - validation is deferred to execution time")

		// Dangerous variable should be loaded but not validated yet
		assert.Equal(t, "value; rm -rf /", dangerousRunner.envVars["DANGEROUS_VAR"])
		assert.Equal(t, "safe_value", dangerousRunner.envVars["SAFE_VAR"])
	})
}

// TestLoadEnvironment_ValidationPatterns tests that dangerous patterns are loaded but not validated
func TestLoadEnvironment_ValidationPatterns(t *testing.T) {
	// Setup clean test environment
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"TEST_VAR", "PATH", "HOME", "USER"},
		},
	}

	dangerousPatterns := []struct {
		name    string
		value   string
		pattern string
	}{
		{"command injection with semicolon", "value; rm -rf /", "semicolon"},
		{"command injection with &&", "value && rm -rf /", "&&"},
		{"command injection with ||", "value || rm -rf /", "||"},
		{"command injection with pipe", "value | rm -rf /", "pipe"},
		{"command substitution", "value$(rm -rf /)", "command substitution"},
		{"backticks", "value`rm -rf /`", "backticks"},
		{"redirection >", "value > /etc/passwd", "redirection"},
		{"redirection <", "value < /etc/passwd", "redirection"},
		{"rm command", "rm -rf /tmp", "destructive command"},
		{"dd command", "dd if=/dev/zero of=/dev/sda", "destructive command"},
		{"mkfs command", "mkfs.ext4 /dev/sda1", "destructive command"},
		{"exec command", "exec /bin/sh", "code execution"},
		{"eval command", "eval('dangerous')", "code execution"},
	}

	for _, pattern := range dangerousPatterns {
		t.Run("Dangerous pattern loaded: "+pattern.name, func(t *testing.T) {
			// Create .env file with dangerous pattern
			envFile := filepath.Join(tmpDir, "test_"+pattern.name+".env")
			content := "TEST_VAR=" + pattern.value + "\n"
			err := os.WriteFile(envFile, []byte(content), 0o644)
			require.NoError(t, err)

			// Create new runner for each test
			runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
			require.NoError(t, err)

			// Should succeed in loading (validation deferred to execution time)
			err = runner.LoadEnvironment(envFile, true)
			assert.NoError(t, err, "Pattern '%s' should be loaded - validation deferred to execution time", pattern.value)

			// Dangerous variable should be loaded but not validated yet
			assert.Equal(t, pattern.value, runner.envVars["TEST_VAR"])
		})
	}
}

// TestLoadEnvironment_FilePermissionValidation tests file permission validation
func TestLoadEnvironment_FilePermissionValidation(t *testing.T) {
	// Setup clean test environment
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"TEST_VAR", "PATH", "HOME", "USER"},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	t.Run("Correct file permissions should be accepted", func(t *testing.T) {
		// Create .env file with correct permissions
		goodEnvFile := filepath.Join(tmpDir, "good.env")
		content := "TEST_VAR=safe_value\n"
		err := os.WriteFile(goodEnvFile, []byte(content), 0o644)
		require.NoError(t, err)

		err = runner.LoadEnvironment(goodEnvFile, true)
		assert.NoError(t, err)
		assert.Equal(t, "safe_value", runner.envVars["TEST_VAR"])
	})

	t.Run("Excessive file permissions should be rejected", func(t *testing.T) {
		// Create .env file with excessive permissions
		badEnvFile := filepath.Join(tmpDir, "bad.env")
		content := "TEST_VAR=safe_value\n"
		err := os.WriteFile(badEnvFile, []byte(content), 0o777)
		require.NoError(t, err)

		err = runner.LoadEnvironment(badEnvFile, true)
		assert.Error(t, err)
		assert.ErrorIs(t, err, safefileio.ErrInvalidFilePermissions, "Should return file permission error")
	})
}

// TestLoadEnvironment_MixedValidAndInvalidVariables tests loading with a mix of valid and invalid variables
func TestLoadEnvironment_MixedValidAndInvalidVariables(t *testing.T) {
	// Setup clean test environment
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"SAFE_VAR", "DANGEROUS_VAR", "ANOTHER_SAFE_VAR", "PATH", "HOME", "USER"},
		},
	}

	// Create .env file with mix of safe and dangerous variables
	envFile := filepath.Join(tmpDir, "mixed.env")
	content := `SAFE_VAR=safe_value
DANGEROUS_VAR=value; rm -rf /
ANOTHER_SAFE_VAR=another_safe_value
`
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// Should succeed in loading (validation deferred to execution time)
	err = runner.LoadEnvironment(envFile, true)
	assert.NoError(t, err, "LoadEnvironment should succeed - validation is deferred to execution time")

	// All variables should be loaded, including dangerous ones (validation deferred)
	assert.Equal(t, "safe_value", runner.envVars["SAFE_VAR"])
	assert.Equal(t, "value; rm -rf /", runner.envVars["DANGEROUS_VAR"])
	assert.Equal(t, "another_safe_value", runner.envVars["ANOTHER_SAFE_VAR"])
}

// TestExecutionTimeValidation tests that environment variables are validated during command execution
func TestExecutionTimeValidation(t *testing.T) {
	// Setup clean test environment
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()

	// Create .env file with dangerous variable
	envFile := filepath.Join(tmpDir, "dangerous.env")
	content := `DANGEROUS_VAR=value; rm -rf /
`
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"DANGEROUS_VAR", "PATH", "HOME", "USER"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "test-group",
				EnvAllowlist: []string{"DANGEROUS_VAR", "PATH", "HOME", "USER"},
				Commands: []runnertypes.Command{
					{
						Name: "test-command",
						Cmd:  "echo",
						Args: []string{"test"},
						Env:  []string{"DANGEROUS_VAR=value; rm -rf /"}, // Use the dangerous variable with value
					},
				},
			},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// LoadEnvironment should succeed (no validation yet)
	err = runner.LoadEnvironment(envFile, true)
	assert.NoError(t, err, "LoadEnvironment should succeed - validation is deferred")

	// Dangerous variable should be loaded
	assert.Equal(t, "value; rm -rf /", runner.envVars["DANGEROUS_VAR"])

	// Now attempt to execute a command that uses this dangerous variable
	// This should fail during validation at execution time
	ctx := context.Background()
	err = runner.ExecuteGroup(ctx, config.Groups[0])
	assert.Error(t, err, "ExecuteGroup should fail due to dangerous environment variable")
	assert.ErrorIs(t, err, security.ErrUnsafeEnvironmentVar, "Should return unsafe environment variable error")
}

// TestExecutionTimeVariableNameValidation tests that environment variable names are validated during command execution
func TestExecutionTimeVariableNameValidation(t *testing.T) {
	// Setup clean test environment
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()

	// Create .env file with invalid variable name (contains space)
	envFile := filepath.Join(tmpDir, "invalid_name.env")
	content := `INVALID NAME=some_value
`
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"INVALID NAME", "PATH", "HOME", "USER"}, // Allow the invalid name to pass filtering
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "test-group-invalid-name",
				EnvAllowlist: []string{"INVALID NAME", "PATH", "HOME", "USER"},
				Commands: []runnertypes.Command{
					{
						Name: "test-command-invalid-name",
						Cmd:  "echo",
						Args: []string{"test"},
						Env:  []string{"INVALID NAME=some_value"}, // Use the invalid variable name
					},
				},
			},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// LoadEnvironment should succeed (no validation yet, even for invalid names)
	err = runner.LoadEnvironment(envFile, true)
	assert.NoError(t, err, "LoadEnvironment should succeed - validation is deferred")

	// Invalid variable name should be loaded but not validated yet
	assert.Equal(t, "some_value", runner.envVars["INVALID NAME"])

	// Now attempt to execute a command that uses this invalid variable name
	// This should fail during validation at execution time
	ctx := context.Background()
	err = runner.ExecuteGroup(ctx, config.Groups[0])
	assert.Error(t, err, "ExecuteGroup should fail due to invalid environment variable name")
	// The error should be about malformed environment variable (which includes name validation)
	assert.Contains(t, err.Error(), "malformed", "Should return malformed environment variable error")
}

// TestExecutionTimeValidationDemonstration tests that validation occurs during execution, not loading
func TestExecutionTimeValidationDemonstration(t *testing.T) {
	// Setup clean test environment
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	tmpDir := t.TempDir()

	// Create .env file with safe variables
	envFile := filepath.Join(tmpDir, "demonstration.env")
	content := `SAFE_VAR=safe_value
`
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      tmpDir,
			EnvAllowlist: []string{"SAFE_VAR", "DANGEROUS_CMD_VAR", "PATH", "HOME", "USER"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "validation-demo-group",
				EnvAllowlist: []string{"SAFE_VAR", "DANGEROUS_CMD_VAR", "PATH", "HOME", "USER"},
				Commands: []runnertypes.Command{
					{
						Name: "dangerous-env-command",
						Cmd:  "echo",
						Args: []string{"test"},
						Env:  []string{"DANGEROUS_CMD_VAR=command; rm -rf /"}, // Dangerous value in command env
					},
				},
			},
		},
	}

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()), WithRunID("test-run-123"))
	require.NoError(t, err)

	// LoadEnvironment should succeed (validation is deferred)
	err = runner.LoadEnvironment(envFile, true)
	assert.NoError(t, err, "LoadEnvironment should succeed - validation is deferred")

	// Verify the env file variables are loaded
	assert.Equal(t, "safe_value", runner.envVars["SAFE_VAR"])

	// Now attempt to execute - this should fail during environment variable processing/validation
	ctx := context.Background()
	err = runner.ExecuteGroup(ctx, config.Groups[0])

	// Should fail due to dangerous variable in command Env field
	assert.Error(t, err, "ExecuteGroup should fail due to dangerous environment variable in Command.Env")
	assert.ErrorIs(t, err, security.ErrUnsafeEnvironmentVar, "Should return unsafe environment variable error")
	assert.Contains(t, err.Error(), "DANGEROUS_CMD_VAR", "Error should mention the dangerous variable")
}
