package security

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.NotEmpty(t, config.AllowedCommands)
	assert.Equal(t, os.FileMode(0o644), config.RequiredFilePermissions)
	assert.NotEmpty(t, config.SensitiveEnvVars)
	assert.Equal(t, 4096, config.MaxPathLength)
}

func TestNewValidator(t *testing.T) {
	t.Run("with config", func(t *testing.T) {
		config := &Config{
			AllowedCommands:         []string{"^echo$"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*"},
			MaxPathLength:           4096,
		}
		validator, err := NewValidator(config)

		assert.NoError(t, err)
		assert.NotNil(t, validator)
		assert.Equal(t, config, validator.config)
		assert.Len(t, validator.allowedCommandRegexps, 1)
		assert.Len(t, validator.sensitiveEnvRegexps, 1)
		assert.GreaterOrEqual(t, len(validator.dangerousEnvRegexps), 1) // Ensure there is at least one dangerous pattern
	})

	t.Run("with nil config", func(t *testing.T) {
		validator, err := NewValidator(nil)

		assert.NoError(t, err)
		assert.NotNil(t, validator)
		assert.NotNil(t, validator.config)
		assert.NotEmpty(t, validator.allowedCommandRegexps)
		assert.NotEmpty(t, validator.sensitiveEnvRegexps)
		assert.NotEmpty(t, validator.dangerousEnvRegexps)
	})

	t.Run("with invalid command pattern", func(t *testing.T) {
		config := &Config{
			AllowedCommands:         []string{"[invalid regex"}, // Invalid regex
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*"},
			MaxPathLength:           4096,
		}
		validator, err := NewValidator(config)

		assert.Error(t, err)
		assert.Nil(t, validator)
		assert.True(t, errors.Is(err, ErrInvalidRegexPattern))
	})

	t.Run("with invalid sensitive env pattern", func(t *testing.T) {
		config := &Config{
			AllowedCommands:         []string{"^echo$"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{"[invalid regex"}, // Invalid regex
			MaxPathLength:           4096,
		}
		validator, err := NewValidator(config)

		assert.Error(t, err)
		assert.Nil(t, validator)
		assert.True(t, errors.Is(err, ErrInvalidRegexPattern))
	})
}

func TestValidator_ValidateFilePermissions(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("empty path", func(t *testing.T) {
		err := validator.ValidateFilePermissions("")

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidPath))
	})

	t.Run("non-existent file", func(t *testing.T) {
		err := validator.ValidateFilePermissions("/non/existent/file")

		assert.Error(t, err)
	})

	t.Run("valid file with correct permissions", func(t *testing.T) {
		// Create a temporary file with correct permissions
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.conf")

		err := os.WriteFile(tmpFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		err = validator.ValidateFilePermissions(tmpFile)
		assert.NoError(t, err)
	})

	t.Run("file with excessive permissions", func(t *testing.T) {
		// Create a temporary file with excessive permissions
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.conf")

		err := os.WriteFile(tmpFile, []byte("test content"), 0o777)
		require.NoError(t, err)

		err = validator.ValidateFilePermissions(tmpFile)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidFilePermissions))
	})

	t.Run("file with dangerous group/other permissions", func(t *testing.T) {
		// Test the security vulnerability case: 0o077 should be rejected even though 0o077 < 0o644
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.conf")

		// Create file with permissions that are numerically less than 0o644 but contain dangerous bits
		err := os.WriteFile(tmpFile, []byte("test content"), 0o077) // ---rwxrwx (dangerous!)
		require.NoError(t, err)

		err = validator.ValidateFilePermissions(tmpFile)
		assert.Error(t, err, "0o077 permissions should be rejected even though 077 < 644")
		assert.True(t, errors.Is(err, ErrInvalidFilePermissions))
		assert.Contains(t, err.Error(), "disallowed bits")
	})

	t.Run("file with only subset of allowed permissions", func(t *testing.T) {
		// Test that files with permissions that are a subset of allowed permissions pass
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.conf")

		// 0o600 is a subset of 0o644 (owner read/write only)
		err := os.WriteFile(tmpFile, []byte("test content"), 0o600)
		require.NoError(t, err)

		err = validator.ValidateFilePermissions(tmpFile)
		assert.NoError(t, err, "0o600 should be allowed as it's a subset of 0o644")
	})

	t.Run("file with exact allowed permissions", func(t *testing.T) {
		// Test that files with exact allowed permissions pass
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.conf")

		// 0o644 should be exactly allowed
		err := os.WriteFile(tmpFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		err = validator.ValidateFilePermissions(tmpFile)
		assert.NoError(t, err, "0o644 should be allowed")
	})

	t.Run("directory instead of file", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := validator.ValidateFilePermissions(tmpDir)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidFilePermissions))
	})

	t.Run("path too long", func(t *testing.T) {
		config := &Config{
			AllowedCommands:         []string{"^echo$"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*"},
			MaxPathLength:           10,
		}
		validator, err := NewValidator(config)
		require.NoError(t, err)

		longPath := "/very/long/path/that/exceeds/limit"
		err = validator.ValidateFilePermissions(longPath)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidPath))
	})
}

func TestValidator_SanitizeEnvironmentVariables(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("nil input", func(t *testing.T) {
		result := validator.SanitizeEnvironmentVariables(nil)

		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("no sensitive variables", func(t *testing.T) {
		envVars := map[string]string{
			"PATH": "/usr/bin",
			"HOME": "/home/user",
			"USER": "testuser",
		}

		result := validator.SanitizeEnvironmentVariables(envVars)

		assert.Equal(t, envVars, result)
	})

	t.Run("with sensitive variables", func(t *testing.T) {
		envVars := map[string]string{
			"PATH":        "/usr/bin",
			"API_KEY":     "secret123",
			"DB_PASSWORD": "password123",
			"AUTH_TOKEN":  "token456",
			"USER":        "testuser",
		}

		result := validator.SanitizeEnvironmentVariables(envVars)

		assert.Equal(t, "/usr/bin", result["PATH"])
		assert.Equal(t, "testuser", result["USER"])
		assert.Equal(t, "[REDACTED]", result["API_KEY"])
		assert.Equal(t, "[REDACTED]", result["DB_PASSWORD"])
		assert.Equal(t, "[REDACTED]", result["AUTH_TOKEN"])
	})
}

func TestValidator_ValidateCommand(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("empty command", func(t *testing.T) {
		err := validator.ValidateCommand("")

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrCommandNotAllowed))
	})

	t.Run("allowed commands", func(t *testing.T) {
		allowedCommands := []string{
			"echo",
			"cat",
			"ls",
			"/bin/bash",
			"/usr/bin/grep",
		}

		for _, cmd := range allowedCommands {
			err := validator.ValidateCommand(cmd)
			assert.NoError(t, err, "command %s should be allowed", cmd)
		}
	})

	t.Run("disallowed commands", func(t *testing.T) {
		disallowedCommands := []string{
			"rm",
			"sudo",
			"su",
			"/tmp/malicious_script",
			"../../../bin/malicious",
		}

		for _, cmd := range disallowedCommands {
			err := validator.ValidateCommand(cmd)
			assert.Error(t, err, "command %s should be disallowed", cmd)
			assert.True(t, errors.Is(err, ErrCommandNotAllowed))
		}
	})
}

func TestValidator_ValidateEnvironmentValue(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("safe values", func(t *testing.T) {
		safeValues := map[string]string{
			"PATH":    "/usr/bin:/bin",
			"HOME":    "/home/user",
			"USER":    "testuser",
			"MESSAGE": "Hello World",
			"NUMBER":  "12345",
		}

		for key, value := range safeValues {
			err := validator.ValidateEnvironmentValue(key, value)
			assert.NoError(t, err, "value %s=%s should be safe", key, value)
		}
	})

	t.Run("unsafe values", func(t *testing.T) {
		unsafeValues := map[string]string{
			"DANGEROUS": "value; rm -rf /",
			"PIPE":      "value | malicious_cmd",
			"AND":       "value && malicious_cmd",
			"OR":        "value || malicious_cmd",
			"SUBST":     "value $(malicious_cmd)",
			"BACKTICK":  "value `malicious_cmd`",
			"REDIRECT":  "value > /etc/passwd",
		}

		for key, value := range unsafeValues {
			err := validator.ValidateEnvironmentValue(key, value)
			assert.Error(t, err, "value %s=%s should be unsafe", key, value)
			assert.True(t, errors.Is(err, ErrUnsafeEnvironmentVar))
		}
	})
}

func TestValidator_ValidateAllEnvironmentVars(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("all safe", func(t *testing.T) {
		envVars := map[string]string{
			"PATH": "/usr/bin:/bin",
			"HOME": "/home/user",
			"USER": "testuser",
		}

		err := validator.ValidateAllEnvironmentVars(envVars)
		assert.NoError(t, err)
	})

	t.Run("contains unsafe", func(t *testing.T) {
		envVars := map[string]string{
			"PATH":      "/usr/bin:/bin",
			"DANGEROUS": "value; rm -rf /",
			"USER":      "testuser",
		}

		err := validator.ValidateAllEnvironmentVars(envVars)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrUnsafeEnvironmentVar))
	})

	t.Run("empty map", func(t *testing.T) {
		err := validator.ValidateAllEnvironmentVars(map[string]string{})
		assert.NoError(t, err)
	})

	t.Run("nil map", func(t *testing.T) {
		err := validator.ValidateAllEnvironmentVars(nil)
		assert.NoError(t, err)
	})
}

func TestValidator_isSensitiveEnvVar(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("sensitive patterns", func(t *testing.T) {
		sensitiveVars := []string{
			"PASSWORD",
			"DB_PASSWORD",
			"SECRET",
			"API_SECRET",
			"TOKEN",
			"AUTH_TOKEN",
			"KEY",
			"PRIVATE_KEY",
			"API",
			"API_ENDPOINT",
		}

		for _, varName := range sensitiveVars {
			result := validator.isSensitiveEnvVar(varName)
			assert.True(t, result, "variable %s should be sensitive", varName)
		}
	})

	t.Run("non-sensitive patterns", func(t *testing.T) {
		nonSensitiveVars := []string{
			"PATH",
			"HOME",
			"USER",
			"SHELL",
			"TERM",
			"LANG",
		}

		for _, varName := range nonSensitiveVars {
			result := validator.isSensitiveEnvVar(varName)
			assert.False(t, result, "variable %s should not be sensitive", varName)
		}
	})
}
