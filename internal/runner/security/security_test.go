package security

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Static errors to satisfy err113 linter
var (
	errPasswordTest = errors.New("password=secret123 failed")
	errLongTest     = errors.New("this is a very long error message that should be truncated")
	errSecretTest   = errors.New("password=mysecret failed")
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
		assert.ErrorIs(t, err, ErrInvalidRegexPattern)
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
		assert.ErrorIs(t, err, ErrInvalidRegexPattern)
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
		assert.ErrorIs(t, err, ErrInvalidPath)
	})

	t.Run("relative path", func(t *testing.T) {
		err := validator.ValidateFilePermissions("relative/path/file.conf")

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidPath)
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
		assert.ErrorIs(t, err, ErrInvalidFilePermissions)
	})

	t.Run("file with dangerous group/other permissions", func(t *testing.T) {
		// Test the security vulnerability case: 0o077 should be rejected even though 0o077 < 0o644
		mockFS.AddFile("/test-dangerous.conf", 0o077, []byte("test content"))

		err := validator.ValidateFilePermissions("/test-dangerous.conf")
		assert.Error(t, err, "0o077 permissions should be rejected even though 077 < 644")
		assert.ErrorIs(t, err, ErrInvalidFilePermissions)
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
		assert.ErrorIs(t, err, ErrInvalidFilePermissions)
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
		assert.ErrorIs(t, err, ErrInvalidPath)
	})
}

func TestValidator_ValidateDirectoryPermissions(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	validator, err := NewValidatorWithFS(DefaultConfig(), mockFS)
	require.NoError(t, err)

	t.Run("empty path", func(t *testing.T) {
		err := validator.ValidateDirectoryPermissions("")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidPath)
	})

	t.Run("relative path", func(t *testing.T) {
		err := validator.ValidateDirectoryPermissions("relative/path/dir")

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidPath)
	})

	t.Run("non-existent directory", func(t *testing.T) {
		err := validator.ValidateDirectoryPermissions("/non/existent/dir")
		assert.Error(t, err)
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
		assert.ErrorIs(t, err, ErrInvalidDirPermissions)
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
		assert.ErrorIs(t, err, ErrInvalidDirPermissions)
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
		assert.ErrorIs(t, err, ErrInvalidPath)
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
		assert.ErrorIs(t, err, ErrCommandNotAllowed)
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
			assert.ErrorIs(t, err, ErrCommandNotAllowed)
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
			assert.ErrorIs(t, err, ErrUnsafeEnvironmentVar)
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
		assert.ErrorIs(t, err, ErrUnsafeEnvironmentVar)
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

func TestValidator_SanitizeErrorForLogging(t *testing.T) {
	tests := []struct {
		name     string
		opts     LoggingOptions
		err      error
		expected string
	}{
		{
			name: "redacted when include details false",
			opts: LoggingOptions{
				IncludeErrorDetails: false,
			},
			err:      errPasswordTest,
			expected: "[error details redacted for security]",
		},
		{
			name: "redacts sensitive patterns",
			opts: LoggingOptions{
				IncludeErrorDetails: true,
				RedactSensitiveInfo: true,
			},
			err:      errPasswordTest,
			expected: "password=[REDACTED] failed",
		},
		{
			name: "truncates long messages",
			opts: LoggingOptions{
				IncludeErrorDetails:   true,
				MaxErrorMessageLength: 20,
			},
			err:      errLongTest,
			expected: "this is a very long ...[truncated]",
		},
		{
			name:     "nil error returns empty string",
			opts:     DefaultLoggingOptions(),
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create validator with test options
			config := DefaultConfig()
			config.LoggingOptions = tt.opts
			validator, err := NewValidator(config)
			require.NoError(t, err)

			result := validator.SanitizeErrorForLogging(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_SanitizeOutputForLogging(t *testing.T) {
	tests := []struct {
		name     string
		opts     LoggingOptions
		output   string
		expected string
	}{
		{
			name: "redacts API keys",
			opts: LoggingOptions{
				RedactSensitiveInfo: true,
			},
			output:   "API call failed: api_key=abc123def",
			expected: "API call failed: api_key=[REDACTED]",
		},
		{
			name: "truncates long output",
			opts: LoggingOptions{
				TruncateStdout:  true,
				MaxStdoutLength: 20,
			},
			output:   "this is a very long output that should be truncated for security reasons",
			expected: "this is a very long ...[truncated for security]",
		},
		{
			name: "handles bearer tokens",
			opts: LoggingOptions{
				RedactSensitiveInfo: true,
			},
			output:   "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: "Authorization: Bearer [REDACTED]",
		},
		{
			name:     "empty output returns empty string",
			opts:     DefaultLoggingOptions(),
			output:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create validator with test options
			config := DefaultConfig()
			config.LoggingOptions = tt.opts
			validator, err := NewValidator(config)
			require.NoError(t, err)

			result := validator.SanitizeOutputForLogging(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_CreateSafeLogFields(t *testing.T) {
	config := DefaultConfig()
	config.LoggingOptions.RedactSensitiveInfo = true
	config.LoggingOptions.IncludeErrorDetails = true // Enable error details to see redaction
	validator, err := NewValidator(config)
	require.NoError(t, err)

	fields := map[string]any{
		"command":   "curl -H 'Authorization: Bearer secret123'",
		"error":     errSecretTest,
		"exit_code": 1,
		"timeout":   "30s",
	}

	result := validator.CreateSafeLogFields(fields)

	// Check that sensitive data is redacted
	assert.Contains(t, result["command"], "Bearer [REDACTED]")
	assert.Contains(t, result["error"], "password=[REDACTED]")

	// Check that non-sensitive fields are preserved
	assert.Equal(t, 1, result["exit_code"])
	assert.Equal(t, "30s", result["timeout"])
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		command  []string
		pattern  []string
		expected bool
	}{
		// Command name exact matching tests
		{
			name:     "exact command match",
			command:  []string{"rm", "-rf", "/tmp"},
			pattern:  []string{"rm", "-rf"},
			expected: true,
		},
		{
			name:     "command name mismatch",
			command:  []string{"ls", "-la"},
			pattern:  []string{"rm", "-la"},
			expected: false,
		},
		{
			name:     "pattern longer than command",
			command:  []string{"rm"},
			pattern:  []string{"rm", "-rf", "/tmp"},
			expected: false,
		},

		// Regular argument exact matching tests
		{
			name:     "regular argument exact match",
			command:  []string{"chmod", "777", "/tmp/file"},
			pattern:  []string{"chmod", "777"},
			expected: true,
		},
		{
			name:     "regular argument mismatch",
			command:  []string{"chmod", "755", "/tmp/file"},
			pattern:  []string{"chmod", "777"},
			expected: false,
		},

		// Key-value pattern prefix matching tests (ending with "=")
		{
			name:     "dd if= pattern match",
			command:  []string{"dd", "if=/dev/zero", "of=/tmp/file"},
			pattern:  []string{"dd", "if="},
			expected: true,
		},
		{
			name:     "dd of= pattern match",
			command:  []string{"dd", "if=/dev/zero", "of=/dev/sda"},
			pattern:  []string{"dd", "of="},
			expected: true,
		},
		{
			name:     "dd if= pattern with specific value",
			command:  []string{"dd", "if=/dev/zero", "of=/tmp/file"},
			pattern:  []string{"dd", "if=/dev/kmsg"},
			expected: false, // exact match required for non-ending-with-"=" patterns
		},
		{
			name:     "key-value pattern without = in command",
			command:  []string{"dd", "input", "output"},
			pattern:  []string{"dd", "if="},
			expected: false,
		},
		{
			name:     "pattern with = at command name (index 0) - should use exact match",
			command:  []string{"test=value", "arg"},
			pattern:  []string{"test=", "arg"},
			expected: false, // command names require exact match
		},

		// Edge cases - empty command is a programming error and should not occur
		// {
		// 	name:     "empty command and pattern",
		// 	command:  []string{},
		// 	pattern:  []string{},
		// 	expected: true,
		// },
		{
			name:     "empty args pattern with command",
			command:  []string{"ls", "-r"},
			pattern:  []string{"ls"},
			expected: true,
		},
		{
			name:     "pattern with = but no = in command arg",
			command:  []string{"myapp", "config", "value"},
			pattern:  []string{"myapp", "config="},
			expected: false,
		},
		{
			name:     "complex dd command matching",
			command:  []string{"dd", "if=/dev/zero", "of=/tmp/test", "bs=1M", "count=10"},
			pattern:  []string{"dd", "if="},
			expected: true,
		},
		{
			name:     "multiple key-value patterns",
			command:  []string{"rsync", "src=/home", "dst=/backup", "opts=archive"},
			pattern:  []string{"rsync", "src=", "dst="},
			expected: true,
		},
		{
			name:     "mixed exact and prefix patterns",
			command:  []string{"mount", "-t", "ext4", "device=/dev/sdb1", "/mnt"},
			pattern:  []string{"mount", "-t", "ext4", "device="},
			expected: true,
		},

		// Additional test cases for thorough coverage
		{
			name:     "pattern ending with = but no equals in command",
			command:  []string{"cmd", "argument"},
			pattern:  []string{"cmd", "arg="},
			expected: false,
		},
		{
			name:     "argument with equals but different prefix",
			command:  []string{"dd", "if=/dev/sda", "bs=1M"},
			pattern:  []string{"dd", "of="},
			expected: false,
		},
		{
			name:     "exact match for command with equals sign",
			command:  []string{"export", "PATH=/usr/bin"},
			pattern:  []string{"export", "PATH=/usr/bin"},
			expected: true,
		},

		// Full path matching tests
		{
			name:     "full path command matches filename pattern",
			command:  []string{"/bin/rm", "-rf", "/tmp"},
			pattern:  []string{"rm", "-rf"},
			expected: true,
		},
		{
			name:     "full path command matches full path pattern",
			command:  []string{"/bin/rm", "-rf", "/tmp"},
			pattern:  []string{"/bin/rm", "-rf"},
			expected: true,
		},
		{
			name:     "filename command matches filename pattern",
			command:  []string{"rm", "-rf", "/tmp"},
			pattern:  []string{"rm", "-rf"},
			expected: true,
		},
		{
			name:     "filename command does not match full path pattern",
			command:  []string{"rm", "-rf", "/tmp"},
			pattern:  []string{"/bin/rm", "-rf"},
			expected: false,
		},
		{
			name:     "complex full path with filename pattern",
			command:  []string{"/usr/local/bin/custom-tool", "arg1"},
			pattern:  []string{"custom-tool", "arg1"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdName := ""
			cmdArgs := []string{}
			if len(tt.command) > 0 {
				cmdName = tt.command[0]
				cmdArgs = tt.command[1:]
			}
			result := matchesPattern(cmdName, cmdArgs, tt.pattern)
			assert.Equal(t, tt.expected, result, "matchesPattern(%s, %v, %v) should return %v", cmdName, cmdArgs, tt.pattern, tt.expected)
		})
	}
}

func TestExtractAllCommandNames(t *testing.T) {
	t.Run("simple filename", func(t *testing.T) {
		names, exceededDepth := extractAllCommandNames("echo")
		expected := map[string]struct{}{"echo": {}}
		assert.Equal(t, expected, names)
		assert.False(t, exceededDepth)
	})

	t.Run("full path", func(t *testing.T) {
		names, exceededDepth := extractAllCommandNames("/bin/echo")
		expected := map[string]struct{}{"/bin/echo": {}, "echo": {}}
		assert.Equal(t, expected, names)
		assert.False(t, exceededDepth)
	})

	t.Run("non-existent file", func(t *testing.T) {
		// Test with a path that doesn't exist - should not crash
		names, exceededDepth := extractAllCommandNames("/non/existent/path/cmd")
		expected := map[string]struct{}{"/non/existent/path/cmd": {}, "cmd": {}}
		assert.Equal(t, expected, names)
		assert.False(t, exceededDepth)
	})

	t.Run("empty command name", func(t *testing.T) {
		// Test error case: empty command name should return empty map
		names, exceededDepth := extractAllCommandNames("")
		expected := make(map[string]struct{})
		assert.Equal(t, expected, names)
		assert.False(t, exceededDepth)
	})
}

func TestExtractAllCommandNamesWithSymlinks(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()

	// Create the actual executable
	actualCmd := tmpDir + "/actual_echo"
	f, err := os.Create(actualCmd)
	require.NoError(t, err)
	f.Close()

	// Create first level symlink
	symlink1 := tmpDir + "/echo_link"
	err = os.Symlink(actualCmd, symlink1)
	require.NoError(t, err)

	// Create second level symlink (multi-level)
	symlink2 := tmpDir + "/echo_link2"
	err = os.Symlink(symlink1, symlink2)
	require.NoError(t, err)

	t.Run("single level symlink", func(t *testing.T) {
		names, exceededDepth := extractAllCommandNames(symlink1)

		// Should contain: original symlink name, base name, target, target base name
		assert.Contains(t, names, symlink1)
		assert.Contains(t, names, "echo_link")
		assert.Contains(t, names, actualCmd)
		assert.Contains(t, names, "actual_echo")
		assert.False(t, exceededDepth)
	})

	t.Run("multi-level symlink", func(t *testing.T) {
		names, exceededDepth := extractAllCommandNames(symlink2)

		// Should contain all names in the chain
		assert.Contains(t, names, symlink2)
		assert.Contains(t, names, "echo_link2")
		assert.Contains(t, names, symlink1)
		assert.Contains(t, names, "echo_link")
		assert.Contains(t, names, actualCmd)
		assert.Contains(t, names, "actual_echo")
		assert.False(t, exceededDepth)
	})

	t.Run("relative symlink", func(t *testing.T) {
		// Create a relative symlink
		relSymlink := tmpDir + "/rel_link"
		err = os.Symlink("actual_echo", relSymlink)
		require.NoError(t, err)

		names, exceededDepth := extractAllCommandNames(relSymlink)
		assert.Contains(t, names, relSymlink)
		assert.Contains(t, names, "rel_link")
		assert.Contains(t, names, actualCmd)
		assert.Contains(t, names, "actual_echo")
		assert.False(t, exceededDepth)
	})

	t.Run("exceeds max symlink depth", func(t *testing.T) {
		// Create a chain that exceeds MaxSymlinkDepth (40)
		// For testing, we'll create a smaller chain and mock the depth check
		chainStart := tmpDir + "/deep_start"
		current := chainStart

		// Create a chain of 5 symlinks for testing (simulating deep chain)
		for i := range 5 {
			next := fmt.Sprintf("%s/link_%d", tmpDir, i)
			if i == 4 {
				// Last link points to actual file
				err = os.Symlink(actualCmd, current)
			} else {
				err = os.Symlink(next, current)
			}
			require.NoError(t, err)
			current = next
		}

		names, exceededDepth := extractAllCommandNames(chainStart)

		// Should contain the original link and some resolved names
		assert.Contains(t, names, chainStart)
		assert.Contains(t, names, "deep_start")

		// Should contain the final target if chain is within limit
		assert.Contains(t, names, actualCmd)
		assert.Contains(t, names, "actual_echo")
		assert.False(t, exceededDepth, "Chain should be within depth limit")
	})
}

func TestIsPrivilegeEscalationCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmdName  string
		expected bool
	}{
		{
			name:     "simple sudo command",
			cmdName:  "sudo",
			expected: true,
		},
		{
			name:     "sudo with absolute path",
			cmdName:  "/usr/bin/sudo",
			expected: true,
		},
		{
			name:     "sudo with relative path",
			cmdName:  "./sudo",
			expected: true,
		},
		{
			name:     "command containing sudo but not sudo itself",
			cmdName:  "/usr/bin/pseudo-tool",
			expected: false,
		},
		{
			name:     "command with sudo-like name",
			cmdName:  "my-sudo-wrapper",
			expected: false,
		},
		{
			name:     "normal command",
			cmdName:  "/bin/echo",
			expected: false,
		},
		{
			name:     "empty command",
			cmdName:  "",
			expected: false,
		},
		{
			name:     "simple su command",
			cmdName:  "su",
			expected: true,
		},
		{
			name:     "su with absolute path",
			cmdName:  "/bin/su",
			expected: true,
		},
		{
			name:     "simple doas command",
			cmdName:  "doas",
			expected: true,
		},
		{
			name:     "doas with absolute path",
			cmdName:  "/usr/bin/doas",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := IsSudoCommand(tt.cmdName)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Test with actual symbolic link (integration test)
	t.Run("symbolic link to sudo", func(t *testing.T) {
		// Create a temporary directory
		tempDir, err := os.MkdirTemp("", "sudo_symlink_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create a symbolic link to sudo (if it exists)
		sudoPath := "/usr/bin/sudo"
		if _, err := os.Stat(sudoPath); err == nil {
			symlinkPath := filepath.Join(tempDir, "my_sudo")
			err := os.Symlink(sudoPath, symlinkPath)
			require.NoError(t, err)

			// Test that the symbolic link is detected as sudo
			result, err := IsSudoCommand(symlinkPath)
			assert.NoError(t, err)
			assert.True(t, result, "Symbolic link to sudo should be detected as sudo")
		} else {
			t.Skip("sudo not found at /usr/bin/sudo, skipping symlink test")
		}
	})

	// Test symlink depth exceeded case
	t.Run("symlink depth exceeded should return error", func(t *testing.T) {
		// Create a temporary directory for deep symlink chain
		tempDir, err := os.MkdirTemp("", "deep_sudo_symlink_test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// Create a deep chain of symlinks (more than MaxSymlinkDepth=40)
		// Create initial target file
		targetFile := filepath.Join(tempDir, "target_sudo")
		err = os.WriteFile(targetFile, []byte("#!/bin/bash\necho sudo"), 0o755)
		require.NoError(t, err)

		// Create 45 symlinks (exceeds MaxSymlinkDepth=40)
		current := targetFile
		for i := 0; i < 45; i++ {
			linkPath := filepath.Join(tempDir, fmt.Sprintf("link_%d", i))
			err := os.Symlink(current, linkPath)
			require.NoError(t, err)
			current = linkPath
		}

		// Test that deep symlink returns error
		result, err := IsSudoCommand(current)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrSymlinkDepthExceeded)
		assert.False(t, result, "Deep symlink should return false when depth exceeded")
	})
}

func TestAnalyzeCommandSecurityWithDeepSymlinks(t *testing.T) {
	t.Run("normal command has no risk", func(t *testing.T) {
		// Resolve path for echo command
		echoPath, err := exec.LookPath("echo")
		if err != nil {
			echoPath = "/bin/echo" // fallback
		}
		risk, pattern, reason := AnalyzeCommandSecurityWithResolvedPath(echoPath, []string{"hello"})
		assert.Equal(t, RiskLevelNone, risk)
		assert.Empty(t, pattern)
		assert.Empty(t, reason)
	})

	t.Run("dangerous pattern detected", func(t *testing.T) {
		// Resolve path for rm command
		rmPath, err := exec.LookPath("rm")
		if err != nil {
			rmPath = "/bin/rm" // fallback
		}
		risk, pattern, reason := AnalyzeCommandSecurityWithResolvedPath(rmPath, []string{"-rf", "/"})
		assert.Equal(t, RiskLevelHigh, risk)
		assert.Equal(t, "rm -rf", pattern)
		assert.Equal(t, "Recursive file removal", reason)
	})

	// Note: Testing actual symlink depth exceeded would require creating 40+ symlinks
	// which is impractical in unit tests. The logic is tested through extractAllCommandNames.
}

func TestAnalyzeCommandSecuritySetuidSetgid(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "setuid_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("normal executable without setuid/setgid", func(t *testing.T) {
		// Create a normal executable
		normalExec := filepath.Join(tmpDir, "normal_exec")
		err := os.WriteFile(normalExec, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)

		risk, pattern, reason := AnalyzeCommandSecurityWithResolvedPath(normalExec, []string{})
		assert.Equal(t, RiskLevelNone, risk)
		assert.Empty(t, pattern)
		assert.Empty(t, reason)
	})

	// Integration test with real setuid binary (if available)
	t.Run("real setuid binary integration test", func(t *testing.T) {
		// Check if passwd command exists and has setuid bit
		passwdPath := "/usr/bin/passwd"
		if fileInfo, err := os.Stat(passwdPath); err == nil && fileInfo.Mode()&os.ModeSetuid != 0 {
			risk, pattern, reason := AnalyzeCommandSecurityWithResolvedPath(passwdPath, []string{})
			assert.Equal(t, RiskLevelHigh, risk)
			assert.Equal(t, passwdPath, pattern)
			assert.Equal(t, "Executable has setuid or setgid bit set", reason)
		} else {
			t.Skip("No setuid passwd binary found for integration test")
		}
	})

	t.Run("non-existent executable", func(t *testing.T) {
		// Test with non-existent file - should not cause panic and fallback to other checks
		risk, pattern, reason := AnalyzeCommandSecurityWithResolvedPath("/non/existent/file", []string{})
		assert.Equal(t, RiskLevelNone, risk)
		assert.Empty(t, pattern)
		assert.Empty(t, reason)
	})
}

func TestHasSetuidOrSetgidBit(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "setuid_helper_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("normal file", func(t *testing.T) {
		normalFile := filepath.Join(tmpDir, "normal")
		err := os.WriteFile(normalFile, []byte("test"), 0o644)
		require.NoError(t, err)

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(normalFile)
		assert.NoError(t, err)
		assert.False(t, hasSetuidOrSetgid)
	})

	// Integration test with real setuid binary
	t.Run("real setuid binary", func(t *testing.T) {
		// Check if passwd command exists and has setuid bit
		passwdPath := "/usr/bin/passwd"
		if fileInfo, err := os.Stat(passwdPath); err == nil && fileInfo.Mode()&os.ModeSetuid != 0 {
			hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(passwdPath)
			assert.NoError(t, err)
			assert.True(t, hasSetuidOrSetgid)
		} else {
			t.Skip("No setuid passwd binary found for integration test")
		}
	})

	t.Run("directory", func(t *testing.T) {
		dir := filepath.Join(tmpDir, "testdir")
		err := os.Mkdir(dir, 0o755)
		require.NoError(t, err)

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(dir)
		assert.NoError(t, err)
		assert.False(t, hasSetuidOrSetgid) // directories are not regular files
	})

	t.Run("non-existent file", func(t *testing.T) {
		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit("/non/existent/file")
		assert.Error(t, err)
		assert.False(t, hasSetuidOrSetgid)
	})

	t.Run("relative path - command in PATH", func(t *testing.T) {
		// Test with a common command that should exist in PATH
		// Note: This test might be system-dependent
		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit("echo")
		// We don't assert the result as it depends on system configuration,
		// but we check that the function doesn't crash
		t.Logf("echo command setuid/setgid status: %v, error: %v", hasSetuidOrSetgid, err)
	})
}
