package runner

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/template"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// setupTestEnv sets up a clean test environment and returns a cleanup function.
// The cleanup function restores the original environment when called.
func setupTestEnv(t *testing.T, envVars map[string]string) func() {
	t.Helper()
	originalEnv := os.Environ()

	// Clear the environment
	os.Clearenv()

	// Set up the test environment variables
	for key, value := range envVars {
		err := os.Setenv(key, value)
		require.NoError(t, err, "failed to set environment variable %s", key)
	}

	// Return a cleanup function that restores the original environment
	return func() {
		os.Clearenv()
		for _, env := range originalEnv {
			if eq := strings.Index(env, "="); eq >= 0 {
				os.Setenv(env[:eq], env[eq+1:])
			}
		}
	}
}

// setupSafeTestEnv sets up a minimal safe environment for tests and returns a cleanup function.
// This is useful for security-related tests where we want to ensure a clean, minimal environment.
func setupSafeTestEnv(t *testing.T) func() {
	t.Helper()
	safeEnv := map[string]string{
		"PATH": "/usr/bin:/bin",
		"HOME": "/home/test",
		"USER": "test",
	}
	return setupTestEnv(t, safeEnv)
}

var ErrExecutionFailed = errors.New("execution failed")

// MockExecutor is a mock implementation of CommandExecutor
type MockExecutor struct {
	mock.Mock
}

func (m *MockExecutor) Execute(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*executor.Result, error) {
	args := m.Called(ctx, cmd, envVars)
	return args.Get(0).(*executor.Result), args.Error(1)
}

func (m *MockExecutor) Validate(cmd runnertypes.Command) error {
	args := m.Called(cmd)
	return args.Error(0)
}

func TestNewRunner(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout:  3600,
			WorkDir:  "/tmp",
			LogLevel: "info",
		},
	}

	t.Run("default configuration", func(t *testing.T) {
		runner, err := NewRunner(config)
		require.NoError(t, err, "NewRunner should not return an error with valid config")
		assert.NotNil(t, runner)
		assert.Equal(t, config, runner.config)
		assert.NotNil(t, runner.executor)
		assert.NotNil(t, runner.envVars)
		assert.NotNil(t, runner.validator)
		assert.NotNil(t, runner.templateEngine)
		assert.NotNil(t, runner.resourceManager)
	})

	t.Run("with custom security config", func(t *testing.T) {
		securityConfig := &security.Config{
			AllowedCommands:         []string{"^echo$", "^cat$"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*", ".*TOKEN.*"},
			MaxPathLength:           4096,
		}

		runner, err := NewRunner(config, WithSecurity(securityConfig))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
		assert.Equal(t, config, runner.config)
		assert.NotNil(t, runner.validator)
	})

	t.Run("with custom template engine", func(t *testing.T) {
		customEngine := template.NewEngine()
		runner, err := NewRunner(config, WithTemplateEngine(customEngine))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
		assert.Equal(t, customEngine, runner.templateEngine)
	})

	t.Run("with multiple options", func(t *testing.T) {
		securityConfig := &security.Config{
			AllowedCommands:         []string{"^echo$"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*"},
			MaxPathLength:           4096,
		}
		customEngine := template.NewEngine()
		customResourceManager := resource.NewManager("/custom/path")

		runner, err := NewRunner(config,
			WithSecurity(securityConfig),
			WithTemplateEngine(customEngine),
			WithResourceManager(customResourceManager))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
		assert.Equal(t, customEngine, runner.templateEngine)
		assert.Equal(t, customResourceManager, runner.resourceManager)
	})

	t.Run("with invalid security config", func(t *testing.T) {
		invalidSecurityConfig := &security.Config{
			AllowedCommands:         []string{"[invalid regex"}, // Invalid regex
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*"},
			MaxPathLength:           4096,
		}

		runner, err := NewRunner(config, WithSecurity(invalidSecurityConfig))
		assert.Error(t, err)
		assert.Nil(t, runner)
		assert.True(t, errors.Is(err, security.ErrInvalidRegexPattern))
	})
}

func TestNewRunnerWithSecurity(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout:  3600,
			WorkDir:  "/tmp",
			LogLevel: "info",
		},
	}

	t.Run("with valid security config", func(t *testing.T) {
		securityConfig := &security.Config{
			AllowedCommands:         []string{"^echo$", "^cat$"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*", ".*TOKEN.*"},
			MaxPathLength:           4096,
		}

		runner, err := NewRunner(config, WithSecurity(securityConfig))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
		assert.Equal(t, config, runner.config)
		assert.NotNil(t, runner.executor)
		assert.NotNil(t, runner.envVars)
		assert.NotNil(t, runner.validator)
		assert.NotNil(t, runner.templateEngine)
		assert.NotNil(t, runner.resourceManager)
	})

	t.Run("with invalid security config", func(t *testing.T) {
		invalidSecurityConfig := &security.Config{
			AllowedCommands:         []string{"[invalid regex"}, // Invalid regex
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*"},
			MaxPathLength:           4096,
		}

		runner, err := NewRunner(config, WithSecurity(invalidSecurityConfig))
		assert.Error(t, err)
		assert.Nil(t, runner)
		assert.True(t, errors.Is(err, security.ErrInvalidRegexPattern))
	})

	t.Run("with nil security config", func(t *testing.T) {
		runner, err := NewRunner(config, WithSecurity(nil))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
	})
}

func TestRunner_ExecuteGroup(t *testing.T) {
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	tests := []struct {
		name        string
		group       runnertypes.CommandGroup
		mockResults []*executor.Result
		mockErrors  []error
		expectedErr error
	}{
		{
			name: "successful execution",
			group: runnertypes.CommandGroup{
				Name:        "test-group",
				Description: "Test group",
				Commands: []runnertypes.Command{
					{
						Name: "test-cmd-1",
						Cmd:  "echo",
						Args: []string{"hello"},
					},
					{
						Name: "test-cmd-2",
						Cmd:  "echo",
						Args: []string{"world"},
					},
				},
			},
			mockResults: []*executor.Result{
				{ExitCode: 0, Stdout: "hello\n", Stderr: ""},
				{ExitCode: 0, Stdout: "world\n", Stderr: ""},
			},
			mockErrors:  []error{nil, nil},
			expectedErr: nil,
		},
		{
			name: "command execution error",
			group: runnertypes.CommandGroup{
				Name: "test-group",
				Commands: []runnertypes.Command{
					{
						Name: "test-cmd-1",
						Cmd:  "echo",
						Args: []string{"hello"},
					},
				},
			},
			mockResults: []*executor.Result{nil},
			mockErrors:  []error{ErrExecutionFailed},
			expectedErr: ErrExecutionFailed,
		},
		{
			name: "command exit code error",
			group: runnertypes.CommandGroup{
				Name: "test-group",
				Commands: []runnertypes.Command{
					{
						Name: "test-cmd-1",
						Cmd:  "false",
					},
				},
			},
			mockResults: []*executor.Result{{ExitCode: 1, Stdout: "", Stderr: ""}},
			mockErrors:  []error{nil},
			expectedErr: ErrCommandFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					Timeout:  3600,
					WorkDir:  "/tmp",
					LogLevel: "info",
				},
			}

			mockExecutor := new(MockExecutor)
			runner, err := NewRunner(config)
			require.NoError(t, err, "NewRunner should not return an error with valid config")
			runner.executor = mockExecutor

			// Setup mock expectations
			for i, cmd := range tt.group.Commands {
				// Create expected command with WorkDir set
				expectedCmd := cmd
				if expectedCmd.Dir == "" {
					expectedCmd.Dir = config.Global.WorkDir
				}
				mockExecutor.On("Execute", mock.Anything, expectedCmd, mock.Anything).Return(tt.mockResults[i], tt.mockErrors[i])
			}

			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, tt.group)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedErr), "expected error %v, got %v", tt.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}

			mockExecutor.AssertExpectations(t)
		})
	}
}

func TestRunner_ExecuteAll(t *testing.T) {
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout:  3600,
			WorkDir:  "/tmp",
			LogLevel: "info",
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:     "group-2",
				Priority: 2,
				Commands: []runnertypes.Command{
					{Name: "cmd-2", Cmd: "echo", Args: []string{"second"}},
				},
			},
			{
				Name:     "group-1",
				Priority: 1,
				Commands: []runnertypes.Command{
					{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}},
				},
			},
		},
	}

	mockExecutor := new(MockExecutor)
	runner, err := NewRunner(config)
	require.NoError(t, err)
	runner.executor = mockExecutor

	// Setup mock expectations - should be called in priority order
	mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, mock.Anything).Return(&executor.Result{ExitCode: 0, Stdout: "first\n"}, nil)
	mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-2", Cmd: "echo", Args: []string{"second"}, Dir: "/tmp"}, mock.Anything).Return(&executor.Result{ExitCode: 0, Stdout: "second\n"}, nil)

	ctx := context.Background()
	err = runner.ExecuteAll(ctx)

	assert.NoError(t, err)
	mockExecutor.AssertExpectations(t)
}

func TestRunner_ExecuteCommand(t *testing.T) {
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout:  3600,
			WorkDir:  "/tmp",
			LogLevel: "info",
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name: "test-group",
				Commands: []runnertypes.Command{
					{Name: "test-cmd", Cmd: "echo", Args: []string{"hello"}},
				},
			},
		},
	}

	mockExecutor := new(MockExecutor)
	runner, err := NewRunner(config)
	require.NoError(t, err)
	runner.executor = mockExecutor

	t.Run("existing command", func(t *testing.T) {
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "test-cmd", Cmd: "echo", Args: []string{"hello"}, Dir: "/tmp"}, mock.Anything).Return(&executor.Result{ExitCode: 0, Stdout: "hello\n"}, nil)

		ctx := context.Background()
		err := runner.ExecuteCommand(ctx, "test-cmd")

		assert.NoError(t, err)
		mockExecutor.AssertExpectations(t)
	})

	t.Run("non-existing command", func(t *testing.T) {
		ctx := context.Background()
		err := runner.ExecuteCommand(ctx, "non-existing-cmd")

		assert.Error(t, err)
		if !errors.Is(err, ErrCommandNotFound) {
			t.Errorf("expected %v, got %v", ErrCommandNotFound, err)
		}
	})
}

func TestRunner_resolveVariableReferences(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"HOME", "USER", "PATH", "GREETING", "CIRCULAR"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "test-group",
				EnvAllowlist: []string{"HOME", "USER", "PATH", "GREETING", "CIRCULAR"},
			},
		},
	}
	runner := &Runner{config: config}
	envVars := map[string]string{
		"HOME":     "/home/user",
		"USER":     "testuser",
		"PATH":     "/usr/bin:/bin",
		"GREETING": "Hello",
		"CIRCULAR": "${CIRCULAR}", // Circular reference to itself
	}

	tests := []struct {
		name        string
		input       string
		expected    string
		expectedErr error
	}{
		{
			name:        "simple variable",
			input:       "${HOME}",
			expected:    "/home/user",
			expectedErr: nil,
		},
		{
			name:        "variable in text",
			input:       "Welcome ${USER}!",
			expected:    "Welcome testuser!",
			expectedErr: nil,
		},
		{
			name:        "multiple variables",
			input:       "${GREETING} ${USER}",
			expected:    "Hello testuser",
			expectedErr: nil,
		},
		{
			name:        "no variables",
			input:       "plain text",
			expected:    "plain text",
			expectedErr: nil,
		},
		{
			name:        "undefined variable",
			input:       "${UNDEFINED_VAR}",
			expectedErr: ErrVariableAccessDenied,
		},
		{
			name:        "unclosed variable",
			input:       "${UNCLOSED",
			expectedErr: ErrUnclosedVariableRef,
		},
		{
			name:        "circular reference",
			input:       "${CIRCULAR}",
			expectedErr: ErrCircularReference,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := runner.resolveVariableReferences(tt.input, envVars, "test-group")

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedErr), "expected error %v, got %v", tt.expectedErr, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestRunner_createCommandContext(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout: 10,
		},
	}
	runner, err := NewRunner(config)
	require.NoError(t, err)

	t.Run("use global timeout", func(t *testing.T) {
		cmd := runnertypes.Command{Name: "test-cmd"}
		ctx := context.Background()

		cmdCtx, cancel := runner.createCommandContext(ctx, cmd)
		defer cancel()

		deadline, ok := cmdCtx.Deadline()
		assert.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(10*time.Second), deadline, 100*time.Millisecond)
	})

	t.Run("use command-specific timeout", func(t *testing.T) {
		cmd := runnertypes.Command{Name: "test-cmd", Timeout: 5}
		ctx := context.Background()

		cmdCtx, cancel := runner.createCommandContext(ctx, cmd)
		defer cancel()

		deadline, ok := cmdCtx.Deadline()
		assert.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(5*time.Second), deadline, 100*time.Millisecond)
	})
}

func TestRunner_resolveEnvironmentVars(t *testing.T) {
	// Setup a custom test environment with specific variables
	testEnv := map[string]string{
		"SAFE_VAR": "safe_value",
		"PATH":     "/usr/bin:/bin",
	}
	cleanup := setupTestEnv(t, testEnv)
	defer cleanup()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      "/tmp",
			EnvAllowlist: []string{"SAFE_VAR", "PATH", "LOADED_VAR", "CMD_VAR", "REFERENCE_VAR"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "test-group",
				EnvAllowlist: []string{"SAFE_VAR", "PATH", "LOADED_VAR", "CMD_VAR", "REFERENCE_VAR"},
			},
		},
	}

	runner, err := NewRunner(config)
	require.NoError(t, err)
	runner.envVars = map[string]string{
		"LOADED_VAR": "from_env_file",
		"PATH":       "/custom/path", // This should override system PATH
	}

	cmd := runnertypes.Command{
		Env: []string{
			"CMD_VAR=command_value",
			"REFERENCE_VAR=${LOADED_VAR}",
		},
	}

	envVars, err := runner.resolveEnvironmentVars(cmd, "test-group")
	assert.NoError(t, err)

	// Check that loaded vars are present
	assert.Equal(t, "from_env_file", envVars["LOADED_VAR"])
	assert.Equal(t, "/custom/path", envVars["PATH"])

	// Check that command vars are present
	assert.Equal(t, "command_value", envVars["CMD_VAR"])
	assert.Equal(t, "from_env_file", envVars["REFERENCE_VAR"])
}

func TestRunner_resolveVariableReferences_ComplexCircular(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"VAR1", "VAR2", "VAR3"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "test-group",
				EnvAllowlist: []string{"VAR1", "VAR2", "VAR3"},
			},
		},
	}
	runner := &Runner{config: config}

	// Test complex circular dependencies: VAR1 -> VAR2 -> VAR1
	envVars := map[string]string{
		"VAR1": "${VAR2}",
		"VAR2": "${VAR1}",
		"VAR3": "prefix-${VAR1}-suffix",
	}

	tests := []struct {
		name        string
		input       string
		expectedErr error
	}{
		{
			name:        "direct circular VAR1",
			input:       "${VAR1}",
			expectedErr: ErrCircularReference,
		},
		{
			name:        "direct circular VAR2",
			input:       "${VAR2}",
			expectedErr: ErrCircularReference,
		},
		{
			name:        "indirect circular through VAR3",
			input:       "${VAR3}",
			expectedErr: ErrCircularReference,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runner.resolveVariableReferences(tt.input, envVars, "test-group")

			assert.Error(t, err)
			assert.True(t, errors.Is(err, tt.expectedErr), "expected error %v, got %v", tt.expectedErr, err)
		})
	}
}

func TestRunner_SecurityIntegration(t *testing.T) {
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout:  3600,
			WorkDir:  "/tmp",
			LogLevel: "info",
		},
	}

	t.Run("allowed command execution should succeed", func(t *testing.T) {
		runner, err := NewRunner(config)
		require.NoError(t, err)

		allowedCmd := runnertypes.Command{
			Name: "test-echo",
			Cmd:  "echo",
			Args: []string{"hello"},
			Dir:  "/tmp",
		}

		mockExecutor := new(MockExecutor)
		runner.executor = mockExecutor
		mockExecutor.On("Execute", mock.Anything, allowedCmd, mock.Anything).Return(&executor.Result{ExitCode: 0}, nil)

		ctx := context.Background()
		_, err = runner.executeCommand(ctx, allowedCmd)
		assert.NoError(t, err)
	})

	t.Run("disallowed command execution should fail", func(t *testing.T) {
		// Test disallowed command - need verification manager for command validation
		verificationManager, err := verification.NewManager(t.TempDir())
		require.NoError(t, err)

		runner, err := NewRunner(config, WithVerificationManager(verificationManager))
		require.NoError(t, err)

		disallowedCmd := runnertypes.Command{
			Name: "test-xsession",
			Cmd:  "/etc/X11/Xsession",
			Args: []string{"failsafe"},
		}

		ctx := context.Background()
		_, err = runner.executeCommand(ctx, disallowedCmd)
		assert.Error(t, err)
		t.Logf("Actual error: %v", err)
		t.Logf("Error type: %T", err)
		assert.True(t, errors.Is(err, security.ErrCommandNotAllowed), "expected error to wrap security.ErrCommandNotAllowed")
	})

	t.Run("command execution with environment variables", func(t *testing.T) {
		runner, err := NewRunner(config)
		require.NoError(t, err)
		mockExecutor := new(MockExecutor)
		runner.executor = mockExecutor

		// Test with safe environment variables
		safeCmd := runnertypes.Command{
			Name: "test-env",
			Cmd:  "echo",
			Args: []string{"$TEST_VAR"},
			Dir:  "/tmp",
			Env:  []string{"TEST_VAR=safe-value", "PATH=/usr/bin:/bin"},
		}

		mockExecutor.On("Execute", mock.Anything, safeCmd, mock.Anything).
			Return(&executor.Result{ExitCode: 0}, nil)

		_, err = runner.executeCommand(context.Background(), safeCmd)
		assert.NoError(t, err)

		// Test with unsafe environment variable value
		unsafeCmd := runnertypes.Command{
			Name: "test-unsafe-env",
			Cmd:  "echo",
			Args: []string{"$DANGEROUS"},
			Dir:  "/tmp",
			Env:  []string{"DANGEROUS=value; rm -rf /"},
		}

		_, err = runner.executeCommand(context.Background(), unsafeCmd)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, security.ErrUnsafeEnvironmentVar), "expected error to wrap security.ErrUnsafeEnvironmentVar")
	})

	t.Run("environment variable sanitization", func(t *testing.T) {
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.envVars = map[string]string{
			"PATH":        "/usr/bin:/bin",
			"API_KEY":     "secret123",
			"DB_PASSWORD": "password456",
		}

		sanitized := runner.GetSanitizedEnvironmentVars()

		assert.Equal(t, "/usr/bin:/bin", sanitized["PATH"])
		assert.Equal(t, "[REDACTED]", sanitized["API_KEY"])
		assert.Equal(t, "[REDACTED]", sanitized["DB_PASSWORD"])
	})
}

func TestRunner_LoadEnvironmentWithSecurity(t *testing.T) {
	t.Run("load environment with file permission validation", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir: "/tmp",
			},
		}
		runner, err := NewRunner(config)
		require.NoError(t, err)

		// Create a temporary .env file with correct permissions
		tmpDir := t.TempDir()
		envFile := tmpDir + "/.env"

		err = os.WriteFile(envFile, []byte("TEST_VAR=test_value\n"), 0o644)
		assert.NoError(t, err)

		// Should succeed with correct permissions
		err = runner.LoadEnvironment(envFile, false)
		assert.NoError(t, err)

		// Create a file with excessive permissions
		badEnvFile := tmpDir + "/.env_bad"
		err = os.WriteFile(badEnvFile, []byte("TEST_VAR=test_value\n"), 0o777)
		assert.NoError(t, err)

		// Should fail with excessive permissions
		err = runner.LoadEnvironment(badEnvFile, false)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, security.ErrInvalidFilePermissions), "expected error to wrap security.ErrInvalidFilePermissions")
	})

	t.Run("load environment with unsafe values", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir: "/tmp",
			},
		}
		runner, err := NewRunner(config)
		require.NoError(t, err)

		// Create a temporary .env file with unsafe values
		tmpDir := t.TempDir()
		envFile := tmpDir + "/.env"

		unsafeContent := "DANGEROUS=value; rm -rf /\n"
		err = os.WriteFile(envFile, []byte(unsafeContent), 0o644)
		assert.NoError(t, err)

		// Should fail due to unsafe environment variable value
		err = runner.LoadEnvironment(envFile, false)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, security.ErrUnsafeEnvironmentVar), "expected error to wrap security.ErrUnsafeEnvironmentVar")
	})
}
