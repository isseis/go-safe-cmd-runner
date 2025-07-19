package runner

import (
	"errors"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadEnvironment_SystemVariableValidation tests that system environment variables
// are validated for security during loading
func TestLoadEnvironment_SystemVariableValidation(t *testing.T) {
	// Create clean test environment to avoid interference from system variables
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	// Create temporary .env file
	tmpDir := t.TempDir()
	envFile := tmpDir + "/.env"
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

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()))
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

	t.Run("Dangerous system variables should cause error", func(t *testing.T) {
		// Set a dangerous system environment variable
		dangerousCleanup := setupTestEnv(t, map[string]string{
			"DANGEROUS_SYSTEM": "value; rm -rf /",
			"PATH":             "/usr/bin:/bin",
			"HOME":             "/home/test",
			"USER":             "test",
		})
		defer dangerousCleanup()

		// Create new runner for this test
		dangerousRunner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()))
		require.NoError(t, err)

		// Should fail due to dangerous system variable
		err = dangerousRunner.LoadEnvironment(envFile, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DANGEROUS_SYSTEM")
		assert.Contains(t, err.Error(), "dangerous pattern")
	})
}

// TestLoadEnvironment_EnvFileVariableValidation tests that .env file variables
// are validated for security during loading
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

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()))
	require.NoError(t, err)

	t.Run("Safe .env file variables should be loaded", func(t *testing.T) {
		// Create .env file with safe variables
		safeEnvFile := tmpDir + "/safe.env"
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

	t.Run("Dangerous .env file variables should cause error", func(t *testing.T) {
		// Create .env file with dangerous variable
		dangerousEnvFile := tmpDir + "/dangerous.env"
		dangerousContent := `DANGEROUS_VAR=value; rm -rf /
SAFE_VAR=safe_value
`
		err := os.WriteFile(dangerousEnvFile, []byte(dangerousContent), 0o644)
		require.NoError(t, err)

		// Create new runner for this test
		dangerousRunner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()))
		require.NoError(t, err)

		// Should fail due to dangerous variable in .env file
		err = dangerousRunner.LoadEnvironment(dangerousEnvFile, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DANGEROUS_VAR")
		assert.Contains(t, err.Error(), "dangerous pattern")
	})
}

// TestLoadEnvironment_ValidationPatterns tests various dangerous patterns
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
		t.Run("Dangerous pattern: "+pattern.name, func(t *testing.T) {
			// Create .env file with dangerous pattern
			envFile := tmpDir + "/test_" + pattern.name + ".env"
			content := "TEST_VAR=" + pattern.value + "\n"
			err := os.WriteFile(envFile, []byte(content), 0o644)
			require.NoError(t, err)

			// Create new runner for each test
			runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()))
			require.NoError(t, err)

			// Should fail due to dangerous pattern
			err = runner.LoadEnvironment(envFile, true)
			assert.Error(t, err, "Pattern '%s' should be detected as dangerous", pattern.value)
			assert.Contains(t, err.Error(), "TEST_VAR")
			assert.Contains(t, err.Error(), "dangerous pattern")
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

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()))
	require.NoError(t, err)

	t.Run("Correct file permissions should be accepted", func(t *testing.T) {
		// Create .env file with correct permissions
		goodEnvFile := tmpDir + "/good.env"
		content := "TEST_VAR=safe_value\n"
		err := os.WriteFile(goodEnvFile, []byte(content), 0o644)
		require.NoError(t, err)

		err = runner.LoadEnvironment(goodEnvFile, true)
		assert.NoError(t, err)
		assert.Equal(t, "safe_value", runner.envVars["TEST_VAR"])
	})

	t.Run("Excessive file permissions should be rejected", func(t *testing.T) {
		// Create .env file with excessive permissions
		badEnvFile := tmpDir + "/bad.env"
		content := "TEST_VAR=safe_value\n"
		err := os.WriteFile(badEnvFile, []byte(content), 0o777)
		require.NoError(t, err)

		err = runner.LoadEnvironment(badEnvFile, true)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, security.ErrInvalidFilePermissions), "Should return file permission error")
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
			EnvAllowlist: []string{"SAFE_VAR", "DANGEROUS_VAR", "PATH", "HOME", "USER"},
		},
	}

	// Create .env file with mix of safe and dangerous variables
	envFile := tmpDir + "/mixed.env"
	content := `SAFE_VAR=safe_value
DANGEROUS_VAR=value; rm -rf /
ANOTHER_SAFE_VAR=another_safe_value
`
	err := os.WriteFile(envFile, []byte(content), 0o644)
	require.NoError(t, err)

	runner, err := NewRunner(config, WithExecutor(executor.NewDefaultExecutor()))
	require.NoError(t, err)

	// Should fail on first dangerous variable encountered
	err = runner.LoadEnvironment(envFile, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DANGEROUS_VAR")
	assert.Contains(t, err.Error(), "dangerous pattern")

	// Should not have loaded any variables due to validation failure
	assert.Empty(t, runner.envVars["SAFE_VAR"], "Safe variables should not be loaded if any variable fails validation")
}
