package security

import (
	"errors"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.NotEmpty(t, config.AllowedCommands)
	assert.Equal(t, os.FileMode(0o644), config.RequiredFilePermissions)
	assert.Equal(t, os.FileMode(0o755), config.RequiredDirectoryPermissions)
	assert.NotEmpty(t, config.SensitiveEnvVars)
	assert.Equal(t, 4096, config.MaxPathLength)
}

func TestNewValidator(t *testing.T) {
	t.Run("with config", func(t *testing.T) {
		config := &Config{
			AllowedCommands:              []string{"^echo$"},
			RequiredFilePermissions:      0o644,
			RequiredDirectoryPermissions: 0o755,
			SensitiveEnvVars:             []string{".*PASSWORD.*"},
			MaxPathLength:                4096,
		}
		validator, err := NewValidator(config)

		assert.NoError(t, err)
		assert.NotNil(t, validator)
		assert.Equal(t, config, validator.config)
		assert.Len(t, validator.allowedCommandRegexps, 1)
		assert.Len(t, validator.sensitiveEnvRegexps, 1)
		assert.GreaterOrEqual(t, len(validator.dangerousEnvRegexps), 1)
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
			AllowedCommands:         []string{"[invalid"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{},
			MaxPathLength:           4096,
		}
		validator, err := NewValidator(config)

		assert.Error(t, err)
		assert.Nil(t, validator)
		assert.True(t, errors.Is(err, ErrInvalidRegexPattern))
	})

	t.Run("with invalid sensitive env pattern", func(t *testing.T) {
		config := &Config{
			AllowedCommands:         []string{".*"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{"[invalid"},
			MaxPathLength:           4096,
		}
		validator, err := NewValidator(config)

		assert.Error(t, err)
		assert.Nil(t, validator)
		assert.True(t, errors.Is(err, ErrInvalidRegexPattern))
	})
}

func TestNewValidatorWithFS(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	config := &Config{
		AllowedCommands:         []string{"^echo$"},
		RequiredFilePermissions: 0o644,
		SensitiveEnvVars:        []string{".*PASSWORD.*"},
		MaxPathLength:           4096,
	}
	validator, err := NewValidatorWithFS(config, mockFS)

	assert.NoError(t, err)
	assert.NotNil(t, validator)
	assert.Equal(t, config, validator.config)
	assert.Equal(t, mockFS, validator.fs)
}

func TestValidator_ValidateFilePermissions(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	validator, err := NewValidatorWithFS(DefaultConfig(), mockFS)
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
		// Create a file with correct permissions in mock filesystem
		mockFS.AddFile("/test.conf", 0o644, []byte("test content"))

		err := validator.ValidateFilePermissions("/test.conf")
		assert.NoError(t, err)
	})

	t.Run("file with excessive permissions", func(t *testing.T) {
		// Create a file with excessive permissions in mock filesystem
		mockFS.AddFile("/test-excessive.conf", 0o777, []byte("test content"))

		err := validator.ValidateFilePermissions("/test-excessive.conf")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidFilePermissions))
	})

	t.Run("file with dangerous group/other permissions", func(t *testing.T) {
		// Test the security vulnerability case: 0o077 should be rejected even though 0o077 < 0o644
		mockFS.AddFile("/test-dangerous.conf", 0o077, []byte("test content"))

		err := validator.ValidateFilePermissions("/test-dangerous.conf")
		assert.Error(t, err, "0o077 permissions should be rejected even though 077 < 644")
		assert.True(t, errors.Is(err, ErrInvalidFilePermissions))
		assert.Contains(t, err.Error(), "disallowed bits")
	})

	t.Run("file with only subset of allowed permissions", func(t *testing.T) {
		// Test that files with permissions that are a subset of allowed permissions pass
		mockFS.AddFile("/test-subset.conf", 0o600, []byte("test content"))

		err := validator.ValidateFilePermissions("/test-subset.conf")
		assert.NoError(t, err, "0o600 should be allowed as it's a subset of 0o644")
	})

	t.Run("file with exact allowed permissions", func(t *testing.T) {
		// Test that files with exact allowed permissions pass
		mockFS.AddFile("/test-exact.conf", 0o644, []byte("test content"))

		err := validator.ValidateFilePermissions("/test-exact.conf")
		assert.NoError(t, err, "0o644 should be allowed as it's exactly the allowed permissions")
	})

	t.Run("directory instead of file", func(t *testing.T) {
		// Test that directories are rejected
		mockFS.AddDir("/test-dir", 0o755)

		err := validator.ValidateFilePermissions("/test-dir")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidFilePermissions))
		// Should fail because it's not a regular file
		assert.Contains(t, err.Error(), "is not a regular file")
	})

	t.Run("path too long", func(t *testing.T) {
		// Test with a path that's too long
		mockFS2 := common.NewMockFileSystem()
		validator2, err := NewValidatorWithFS(&Config{
			AllowedCommands:         []string{".*"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{},
			MaxPathLength:           10, // Very short for testing
		}, mockFS2)
		require.NoError(t, err)

		longPath := "/very/long/path/that/exceeds/limit"
		err = validator2.ValidateFilePermissions(longPath)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidPath))
		assert.Contains(t, err.Error(), "path too long")
	})
}

func TestValidator_ValidateDirectoryPermissions(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	validator, err := NewValidatorWithFS(DefaultConfig(), mockFS)
	require.NoError(t, err)

	t.Run("empty path", func(t *testing.T) {
		err := validator.ValidateDirectoryPermissions("")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidPath))
		assert.Contains(t, err.Error(), "empty path")
	})

	t.Run("non-existent directory", func(t *testing.T) {
		err := validator.ValidateDirectoryPermissions("/non/existent/dir")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to stat")
	})

	t.Run("valid directory with correct permissions", func(t *testing.T) {
		// Create a directory with correct permissions in mock filesystem
		mockFS.AddDir("/test-dir", 0o755)

		err := validator.ValidateDirectoryPermissions("/test-dir")
		assert.NoError(t, err)
	})

	t.Run("directory with excessive permissions", func(t *testing.T) {
		// Create a directory with excessive permissions in mock filesystem
		mockFS.AddDir("/test-excessive-dir", 0o777)

		err := validator.ValidateDirectoryPermissions("/test-excessive-dir")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidFilePermissions))
	})

	t.Run("directory with only subset of allowed permissions", func(t *testing.T) {
		// Test that directories with permissions that are a subset of allowed permissions pass
		mockFS.AddDir("/test-subset-dir", 0o700)

		err := validator.ValidateDirectoryPermissions("/test-subset-dir")
		assert.NoError(t, err, "0o700 should be allowed as it's a subset of 0o755")
	})

	t.Run("directory with exact allowed permissions", func(t *testing.T) {
		// Test that directories with exact allowed permissions pass
		mockFS.AddDir("/test-exact-dir", 0o755)

		err := validator.ValidateDirectoryPermissions("/test-exact-dir")
		assert.NoError(t, err, "0o755 should be allowed as it's exactly the allowed permissions")
	})

	t.Run("file instead of directory", func(t *testing.T) {
		// Test that files are rejected
		mockFS.AddFile("/test-file.txt", 0o644, []byte("test content"))

		err := validator.ValidateDirectoryPermissions("/test-file.txt")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidFilePermissions))
		assert.Contains(t, err.Error(), "is not a directory")
	})

	t.Run("path too long", func(t *testing.T) {
		// Test with a path that's too long
		mockFS2 := common.NewMockFileSystem()
		validator2, err := NewValidatorWithFS(&Config{
			AllowedCommands:              []string{".*"},
			RequiredFilePermissions:      0o644,
			RequiredDirectoryPermissions: 0o755,
			SensitiveEnvVars:             []string{},
			MaxPathLength:                10, // Very short for testing
		}, mockFS2)
		require.NoError(t, err)

		longPath := "/very/long/path/that/exceeds/limit"
		err = validator2.ValidateDirectoryPermissions(longPath)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidPath))
		assert.Contains(t, err.Error(), "path too long")
	})
}

func TestValidator_SanitizeEnvironmentVariables(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("nil input", func(t *testing.T) {
		result := validator.SanitizeEnvironmentVariables(nil)
		assert.NotNil(t, result)
		assert.Equal(t, make(map[string]string), result)
	})

	t.Run("no sensitive variables", func(t *testing.T) {
		env := map[string]string{
			"PATH":     "/usr/bin:/bin",
			"HOME":     "/home/user",
			"LANGUAGE": "en_US.UTF-8",
		}
		result := validator.SanitizeEnvironmentVariables(env)
		assert.Equal(t, env, result)
	})

	t.Run("with sensitive variables", func(t *testing.T) {
		env := map[string]string{
			"PATH":         "/usr/bin:/bin",
			"HOME":         "/home/user",
			"API_PASSWORD": "secret123",
			"DB_TOKEN":     "token456",
			"NORMAL_VAR":   "value",
		}
		result := validator.SanitizeEnvironmentVariables(env)

		assert.NotEqual(t, env, result)
		assert.Equal(t, "/usr/bin:/bin", result["PATH"])
		assert.Equal(t, "/home/user", result["HOME"])
		assert.Equal(t, "value", result["NORMAL_VAR"])
		assert.Equal(t, "[REDACTED]", result["API_PASSWORD"])
		assert.Equal(t, "[REDACTED]", result["DB_TOKEN"])
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
			"/bin/echo",
			"/usr/bin/ls",
			"/bin/cat",
			"/usr/bin/grep",
		}

		for _, cmd := range allowedCommands {
			err := validator.ValidateCommand(cmd)
			assert.NoError(t, err, "Command %s should be allowed", cmd)
		}
	})

	t.Run("disallowed commands", func(t *testing.T) {
		disallowedCommands := []string{
			"rm",
			"sudo",
			"../../../bin/sh",
			"evil-command",
		}

		for _, cmd := range disallowedCommands {
			err := validator.ValidateCommand(cmd)
			assert.Error(t, err, "Command %s should not be allowed", cmd)
			assert.True(t, errors.Is(err, ErrCommandNotAllowed))
		}
	})
}

func TestValidator_ValidateEnvironmentValue(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("safe values", func(t *testing.T) {
		safeValues := []string{
			"simple_value",
			"/path/to/file",
			"user@example.com",
			"123456",
			"normal-value_with_underscores",
		}

		for _, value := range safeValues {
			err := validator.ValidateEnvironmentValue("TEST_VAR", value)
			assert.NoError(t, err, "Value %s should be safe", value)
		}
	})

	t.Run("unsafe values", func(t *testing.T) {
		unsafeValues := []string{
			"value; rm -rf /",
			"value | cat /etc/passwd",
			"value && malicious_command",
			"value || backup_command",
			"value $(malicious_command)",
			"value `malicious_command`",
			"value > /tmp/output",
			"value < /etc/passwd",
		}

		for _, value := range unsafeValues {
			err := validator.ValidateEnvironmentValue("TEST_VAR", value)
			assert.Error(t, err, "Value %s should be unsafe", value)
			assert.True(t, errors.Is(err, ErrUnsafeEnvironmentVar))
		}
	})
}

func TestValidator_ValidateAllEnvironmentVars(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("all safe", func(t *testing.T) {
		env := map[string]string{
			"PATH": "/usr/bin:/bin",
			"HOME": "/home/user",
			"USER": "testuser",
		}
		err := validator.ValidateAllEnvironmentVars(env)
		assert.NoError(t, err)
	})

	t.Run("contains unsafe", func(t *testing.T) {
		env := map[string]string{
			"PATH":      "/usr/bin:/bin",
			"HOME":      "/home/user",
			"DANGEROUS": "value; rm -rf /",
		}
		err := validator.ValidateAllEnvironmentVars(env)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrUnsafeEnvironmentVar))
	})

	t.Run("empty map", func(t *testing.T) {
		env := map[string]string{}
		err := validator.ValidateAllEnvironmentVars(env)
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
			"API_PASSWORD",
			"DB_PASSWORD",
			"SECRET",
			"API_SECRET",
			"TOKEN",
			"ACCESS_TOKEN",
			"KEY",
			"API_KEY",
			"PRIVATE_KEY",
		}

		for _, varName := range sensitiveVars {
			assert.True(t, validator.isSensitiveEnvVar(varName), "Variable %s should be sensitive", varName)
		}
	})

	t.Run("non-sensitive patterns", func(t *testing.T) {
		nonSensitiveVars := []string{
			"PATH",
			"HOME",
			"USER",
			"LANG",
			"TMPDIR",
			"PWD",
			"SHELL",
		}

		for _, varName := range nonSensitiveVars {
			assert.False(t, validator.isSensitiveEnvVar(varName), "Variable %s should not be sensitive", varName)
		}
	})
}
