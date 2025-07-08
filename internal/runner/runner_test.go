package runner

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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

	runner := NewRunner(config)
	assert.NotNil(t, runner)
	assert.Equal(t, config, runner.config)
	assert.NotNil(t, runner.executor)
	assert.NotNil(t, runner.envVars)
}

func TestRunner_ExecuteGroup(t *testing.T) {
	// Clean environment for tests
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range originalEnv {
			if eq := strings.Index(env, "="); eq >= 0 {
				os.Setenv(env[:eq], env[eq+1:])
			}
		}
	}()

	// Set only safe environment variables for tests
	os.Clearenv()
	os.Setenv("PATH", "/usr/bin:/bin")
	os.Setenv("HOME", "/home/test")
	os.Setenv("USER", "test")

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
			runner := NewRunner(config)
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
			err := runner.ExecuteGroup(ctx, tt.group)

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
	// Clean environment for tests
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range originalEnv {
			if eq := strings.Index(env, "="); eq >= 0 {
				os.Setenv(env[:eq], env[eq+1:])
			}
		}
	}()

	// Set only safe environment variables for tests
	os.Clearenv()
	os.Setenv("PATH", "/usr/bin:/bin")
	os.Setenv("HOME", "/home/test")
	os.Setenv("USER", "test")

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
	runner := NewRunner(config)
	runner.executor = mockExecutor

	// Setup mock expectations - should be called in priority order
	mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, mock.Anything).Return(&executor.Result{ExitCode: 0, Stdout: "first\n"}, nil)
	mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-2", Cmd: "echo", Args: []string{"second"}, Dir: "/tmp"}, mock.Anything).Return(&executor.Result{ExitCode: 0, Stdout: "second\n"}, nil)

	ctx := context.Background()
	err := runner.ExecuteAll(ctx)

	assert.NoError(t, err)
	mockExecutor.AssertExpectations(t)
}

func TestRunner_ExecuteCommand(t *testing.T) {
	// Clean environment for tests
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range originalEnv {
			if eq := strings.Index(env, "="); eq >= 0 {
				os.Setenv(env[:eq], env[eq+1:])
			}
		}
	}()

	// Set only safe environment variables for tests
	os.Clearenv()
	os.Setenv("PATH", "/usr/bin:/bin")
	os.Setenv("HOME", "/home/test")
	os.Setenv("USER", "test")

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
	runner := NewRunner(config)
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
	runner := &Runner{}
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
			expectedErr: ErrUndefinedVariable,
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
			result, err := runner.resolveVariableReferences(tt.input, envVars)

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
	runner := NewRunner(config)

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
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir: "/tmp",
		},
	}
	runner := NewRunner(config)
	runner.envVars = map[string]string{
		"LOADED_VAR": "from_env_file",
		"PATH":       "/custom/path", // This should override system PATH
	}

	// Use a custom test environment to avoid system environment variable validation issues
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range originalEnv {
			if eq := strings.Index(env, "="); eq >= 0 {
				os.Setenv(env[:eq], env[eq+1:])
			}
		}
	}()

	// Clear environment and set only safe test variables
	os.Clearenv()
	os.Setenv("SAFE_VAR", "safe_value")
	os.Setenv("PATH", "/usr/bin:/bin")

	cmd := runnertypes.Command{
		Env: []string{
			"CMD_VAR=command_value",
			"REFERENCE_VAR=${LOADED_VAR}",
		},
	}

	envVars, err := runner.resolveEnvironmentVars(cmd)
	assert.NoError(t, err)

	// Check that loaded vars are present
	assert.Equal(t, "from_env_file", envVars["LOADED_VAR"])
	assert.Equal(t, "/custom/path", envVars["PATH"])

	// Check that command vars are present
	assert.Equal(t, "command_value", envVars["CMD_VAR"])
	assert.Equal(t, "from_env_file", envVars["REFERENCE_VAR"])
}

func TestRunner_resolveVariableReferences_ComplexCircular(t *testing.T) {
	runner := &Runner{}

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
			_, err := runner.resolveVariableReferences(tt.input, envVars)

			assert.Error(t, err)
			assert.True(t, errors.Is(err, tt.expectedErr), "expected error %v, got %v", tt.expectedErr, err)
		})
	}
}

func TestRunner_SecurityIntegration(t *testing.T) {
	// Clean environment for tests
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, env := range originalEnv {
			if eq := strings.Index(env, "="); eq >= 0 {
				os.Setenv(env[:eq], env[eq+1:])
			}
		}
	}()

	// Set only safe environment variables for tests
	os.Clearenv()
	os.Setenv("PATH", "/usr/bin:/bin")
	os.Setenv("HOME", "/home/test")
	os.Setenv("USER", "test")

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout:  3600,
			WorkDir:  "/tmp",
			LogLevel: "info",
		},
	}

	t.Run("command whitelist validation", func(t *testing.T) {
		runner := NewRunner(config)

		// Test allowed command
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
		_, err := runner.executeCommand(ctx, allowedCmd)
		assert.NoError(t, err)

		// Test disallowed command
		disallowedCmd := runnertypes.Command{
			Name: "test-rm",
			Cmd:  "rm",
			Args: []string{"-rf", "/"},
		}

		_, err = runner.executeCommand(ctx, disallowedCmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "command security validation failed")
	})

	t.Run("environment variable validation", func(t *testing.T) {
		runner := NewRunner(config)

		// Test safe environment variables
		safeEnv := map[string]string{
			"PATH": "/usr/bin:/bin",
			"HOME": "/home/user",
		}

		err := runner.validator.ValidateAllEnvironmentVars(safeEnv)
		assert.NoError(t, err)

		// Test unsafe environment variables
		unsafeEnv := map[string]string{
			"DANGEROUS": "value; rm -rf /",
		}

		err = runner.validator.ValidateAllEnvironmentVars(unsafeEnv)
		assert.Error(t, err)
	})

	t.Run("environment variable sanitization", func(t *testing.T) {
		runner := NewRunner(config)
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
		runner := NewRunner(config)

		// Create a temporary .env file with correct permissions
		tmpDir := t.TempDir()
		envFile := tmpDir + "/.env"

		err := os.WriteFile(envFile, []byte("TEST_VAR=test_value\n"), 0o644)
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
		assert.Contains(t, err.Error(), "security validation failed")
	})

	t.Run("load environment with unsafe values", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir: "/tmp",
			},
		}
		runner := NewRunner(config)

		// Create a temporary .env file with unsafe values
		tmpDir := t.TempDir()
		envFile := tmpDir + "/.env"

		unsafeContent := "DANGEROUS=value; rm -rf /\n"
		err := os.WriteFile(envFile, []byte(unsafeContent), 0o644)
		assert.NoError(t, err)

		// Should fail due to unsafe environment variable value
		err = runner.LoadEnvironment(envFile, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "environment variable security validation failed")
	})
}
