package runner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var errCommandNotFound = errors.New("command not found")

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

// MockFileSystem is a mock implementation of common.FileSystem
type MockFileSystem struct {
	mock.Mock
}

func (m *MockFileSystem) CreateTempDir(prefix string) (string, error) {
	args := m.Called(prefix)
	return args.String(0), args.Error(1)
}

func (m *MockFileSystem) TempDir() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	args := m.Called(path, perm)
	return args.Error(0)
}

func (m *MockFileSystem) RemoveAll(path string) error {
	args := m.Called(path)
	return args.Error(0)
}

func (m *MockFileSystem) Remove(path string) error {
	args := m.Called(path)
	return args.Error(0)
}

func (m *MockFileSystem) Lstat(name string) (os.FileInfo, error) {
	args := m.Called(name)
	return args.Get(0).(os.FileInfo), args.Error(1)
}

func (m *MockFileSystem) Readlink(name string) (string, error) {
	args := m.Called(name)
	return args.String(0), args.Error(1)
}

func (m *MockFileSystem) Stat(name string) (os.FileInfo, error) {
	args := m.Called(name)
	return args.Get(0).(os.FileInfo), args.Error(1)
}

func (m *MockFileSystem) FileExists(path string) (bool, error) {
	args := m.Called(path)
	return args.Bool(0), args.Error(1)
}

func (m *MockFileSystem) IsDir(path string) (bool, error) {
	args := m.Called(path)
	return args.Bool(0), args.Error(1)
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

	t.Run("with multiple options", func(t *testing.T) {
		securityConfig := &security.Config{
			AllowedCommands:         []string{"^echo$"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*"},
			MaxPathLength:           4096,
		}
		customResourceManager := resource.NewManager("/custom/path")

		runner, err := NewRunner(config,
			WithSecurity(securityConfig),
			WithResourceManager(customResourceManager))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
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
				Groups: []runnertypes.CommandGroup{tt.group},
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

func TestRunner_ExecuteGroup_ComplexErrorScenarios(t *testing.T) {
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	t.Run("multiple commands with first failing", func(t *testing.T) {
		group := runnertypes.CommandGroup{
			Name: "test-first-fails",
			Commands: []runnertypes.Command{
				{Name: "cmd-1", Cmd: "false"}, // This fails
				{Name: "cmd-2", Cmd: "echo", Args: []string{"second"}},
				{Name: "cmd-3", Cmd: "echo", Args: []string{"third"}},
			},
		}

		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:  3600,
				WorkDir:  "/tmp",
				LogLevel: "info",
			},
			Groups: []runnertypes.CommandGroup{group},
		}

		mockExecutor := new(MockExecutor)
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.executor = mockExecutor

		// First command fails with non-zero exit code
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "false", Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Subsequent commands should not be executed due to fail-fast behavior
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrCommandFailed))
		mockExecutor.AssertExpectations(t)
	})

	t.Run("multiple commands with middle failing", func(t *testing.T) {
		group := runnertypes.CommandGroup{
			Name: "test-middle-fails",
			Commands: []runnertypes.Command{
				{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}},
				{Name: "cmd-2", Cmd: "false"}, // This fails
				{Name: "cmd-3", Cmd: "echo", Args: []string{"third"}},
			},
		}

		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:  3600,
				WorkDir:  "/tmp",
				LogLevel: "info",
			},
			Groups: []runnertypes.CommandGroup{group},
		}

		mockExecutor := new(MockExecutor)
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.executor = mockExecutor

		// First command succeeds
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second command fails
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-2", Cmd: "false", Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Third command should not be executed due to fail-fast behavior

		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrCommandFailed))
		mockExecutor.AssertExpectations(t)
	})

	t.Run("executor returns error instead of non-zero exit code", func(t *testing.T) {
		group := runnertypes.CommandGroup{
			Name: "test-executor-error",
			Commands: []runnertypes.Command{
				{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}},
				{Name: "cmd-2", Cmd: "invalid-command"}, // This causes executor error
				{Name: "cmd-3", Cmd: "echo", Args: []string{"third"}},
			},
		}

		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:  3600,
				WorkDir:  "/tmp",
				LogLevel: "info",
			},
			Groups: []runnertypes.CommandGroup{group},
		}

		mockExecutor := new(MockExecutor)
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.executor = mockExecutor

		// First command succeeds
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second command returns executor error
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-2", Cmd: "invalid-command", Dir: "/tmp"}, mock.Anything).
			Return((*executor.Result)(nil), errCommandNotFound)

		// Third command should not be executed

		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, errCommandNotFound))
		mockExecutor.AssertExpectations(t)
	})

	t.Run("environment variable access denied causes error", func(t *testing.T) {
		group := runnertypes.CommandGroup{
			Name:         "test-env-error",
			EnvAllowlist: []string{"VALID_VAR"}, // Note: INVALID_VAR is not in allowlist
			Commands: []runnertypes.Command{
				{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}},
				{Name: "cmd-2", Cmd: "echo", Args: []string{"test"}, Env: []string{"INVALID_VAR=${NONEXISTENT_VAR}"}},
			},
		}

		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:      3600,
				WorkDir:      "/tmp",
				LogLevel:     "info",
				EnvAllowlist: []string{"VALID_VAR"},
			},
			Groups: []runnertypes.CommandGroup{group},
		}

		mockExecutor := new(MockExecutor)
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.executor = mockExecutor

		// First command should succeed
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second command should not be executed due to environment variable access denial

		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Should fail due to environment variable access denied
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrVariableAccessDenied), "expected error to wrap ErrVariableAccessDenied")
		mockExecutor.AssertExpectations(t)
	})
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

func TestRunner_ExecuteAll_ComplexErrorScenarios(t *testing.T) {
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	t.Run("first group fails, remaining groups should not execute", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:  3600,
				WorkDir:  "/tmp",
				LogLevel: "info",
			},
			Groups: []runnertypes.CommandGroup{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.Command{
						{Name: "fail-cmd", Cmd: "false"},
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.Command{
						{Name: "success-cmd", Cmd: "echo", Args: []string{"should not execute"}},
					},
				},
				{
					Name:     "group-3",
					Priority: 3,
					Commands: []runnertypes.Command{
						{Name: "another-cmd", Cmd: "echo", Args: []string{"also should not execute"}},
					},
				},
			},
		}

		mockExecutor := new(MockExecutor)
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.executor = mockExecutor

		// Only the first group's command should be called (and fail)
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "fail-cmd", Cmd: "false", Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Remaining groups should not be executed

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrCommandFailed))
		mockExecutor.AssertExpectations(t)
	})

	t.Run("middle group fails, remaining groups should not execute", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:  3600,
				WorkDir:  "/tmp",
				LogLevel: "info",
			},
			Groups: []runnertypes.CommandGroup{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.Command{
						{Name: "success-cmd-1", Cmd: "echo", Args: []string{"first"}},
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.Command{
						{Name: "fail-cmd", Cmd: "false"},
					},
				},
				{
					Name:     "group-3",
					Priority: 3,
					Commands: []runnertypes.Command{
						{Name: "should-not-execute", Cmd: "echo", Args: []string{"third"}},
					},
				},
			},
		}

		mockExecutor := new(MockExecutor)
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.executor = mockExecutor

		// First group should succeed
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "success-cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second group should fail
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "fail-cmd", Cmd: "false", Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Third group should not be executed

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrCommandFailed))
		mockExecutor.AssertExpectations(t)
	})

	t.Run("group with multiple commands, second command fails", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:  3600,
				WorkDir:  "/tmp",
				LogLevel: "info",
			},
			Groups: []runnertypes.CommandGroup{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.Command{
						{Name: "success-cmd-1", Cmd: "echo", Args: []string{"first"}},
						{Name: "fail-cmd", Cmd: "false"},
						{Name: "should-not-execute", Cmd: "echo", Args: []string{"third"}},
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.Command{
						{Name: "group2-cmd", Cmd: "echo", Args: []string{"group2"}},
					},
				},
			},
		}

		mockExecutor := new(MockExecutor)
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.executor = mockExecutor

		// First command in group-1 should succeed
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "success-cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second command in group-1 should fail
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "fail-cmd", Cmd: "false", Dir: "/tmp"}, mock.Anything).
			Return(&executor.Result{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Third command in group-1 and group-2 should not be executed

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrCommandFailed))
		mockExecutor.AssertExpectations(t)
	})

	t.Run("executor error in first group", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:  3600,
				WorkDir:  "/tmp",
				LogLevel: "info",
			},
			Groups: []runnertypes.CommandGroup{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.Command{
						{Name: "executor-error-cmd", Cmd: "nonexistent-command"},
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.Command{
						{Name: "should-not-execute", Cmd: "echo", Args: []string{"second"}},
					},
				},
			},
		}

		mockExecutor := new(MockExecutor)
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.executor = mockExecutor

		// First command should return executor error
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "executor-error-cmd", Cmd: "nonexistent-command", Dir: "/tmp"}, mock.Anything).
			Return((*executor.Result)(nil), errCommandNotFound)

		// Second group should not be executed

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, errCommandNotFound))
		mockExecutor.AssertExpectations(t)
	})

	t.Run("context cancellation during execution", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:  3600,
				WorkDir:  "/tmp",
				LogLevel: "info",
			},
			Groups: []runnertypes.CommandGroup{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.Command{
						{Name: "long-running-cmd", Cmd: "sleep", Args: []string{"10"}},
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.Command{
						{Name: "should-not-execute", Cmd: "echo", Args: []string{"second"}},
					},
				},
			},
		}

		mockExecutor := new(MockExecutor)
		runner, err := NewRunner(config)
		require.NoError(t, err)
		runner.executor = mockExecutor

		// Mock executor should return context.Canceled error
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{Name: "long-running-cmd", Cmd: "sleep", Args: []string{"10"}, Dir: "/tmp"}, mock.Anything).
			Return((*executor.Result)(nil), context.Canceled)

		// Create a context that gets cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
		mockExecutor.AssertExpectations(t)
	})

	t.Run("no groups to execute", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout:  3600,
				WorkDir:  "/tmp",
				LogLevel: "info",
			},
			Groups: []runnertypes.CommandGroup{}, // Empty groups
		}

		runner, err := NewRunner(config)
		require.NoError(t, err)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		// Should succeed with no groups to execute
		assert.NoError(t, err)
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

	// Initialize the environment filter with the test configuration
	envFilter := environment.NewFilter(config)

	runner := &Runner{
		config:    config,
		envFilter: envFilter,
	}
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
			testGroup := &config.Groups[0] // Get reference to the test group
			result, err := runner.resolveVariableReferences(tt.input, envVars, testGroup)

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

	envVars, err := runner.resolveEnvironmentVars(cmd, &config.Groups[0])
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

	// Initialize the environment filter with the test configuration
	envFilter := environment.NewFilter(config)

	runner := &Runner{
		config:    config,
		envFilter: envFilter,
	}

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
			testGroup := &config.Groups[0] // Get reference to the test group
			_, err := runner.resolveVariableReferences(tt.input, envVars, testGroup)

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
			Timeout:      3600,
			WorkDir:      "/tmp",
			LogLevel:     "info",
			EnvAllowlist: []string{"PATH", "TEST_VAR", "DANGEROUS"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name: "test-group",
			},
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
		testGroup := &config.Groups[0] // Get reference to the test group
		_, err = runner.executeCommandInGroup(ctx, allowedCmd, testGroup)
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
		testGroup := &config.Groups[0] // Get reference to the test group
		_, err = runner.executeCommandInGroup(ctx, disallowedCmd, testGroup)
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

		testGroup := &config.Groups[0] // Get reference to the test group
		_, err = runner.executeCommandInGroup(context.Background(), safeCmd, testGroup)
		assert.NoError(t, err)

		// Test with unsafe environment variable value
		unsafeCmd := runnertypes.Command{
			Name: "test-unsafe-env",
			Cmd:  "echo",
			Args: []string{"$DANGEROUS"},
			Dir:  "/tmp",
			Env:  []string{"DANGEROUS=value; rm -rf /"},
		}

		_, err = runner.executeCommandInGroup(context.Background(), unsafeCmd, testGroup)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, security.ErrUnsafeEnvironmentVar), "expected error to wrap security.ErrUnsafeEnvironmentVar")
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
		envFile := filepath.Join(tmpDir, ".env")

		err = os.WriteFile(envFile, []byte("TEST_VAR=test_value\n"), 0o644)
		assert.NoError(t, err)

		// Should succeed with correct permissions
		err = runner.LoadEnvironment(envFile, false)
		assert.NoError(t, err)

		// Create a file with excessive permissions
		badEnvFile := filepath.Join(tmpDir, ".env_bad")
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
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"DANGEROUS"}, // Allow the variable to pass filtering so it can be validated
			},
		}
		runner, err := NewRunner(config)
		require.NoError(t, err)

		// Create a temporary .env file with unsafe values
		tmpDir := t.TempDir()
		envFile := filepath.Join(tmpDir, ".env")

		unsafeContent := "DANGEROUS=value; rm -rf /\n"
		err = os.WriteFile(envFile, []byte(unsafeContent), 0o644)
		assert.NoError(t, err)

		// Should fail due to unsafe environment variable value
		err = runner.LoadEnvironment(envFile, false)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, security.ErrUnsafeEnvironmentVar), "expected error to wrap security.ErrUnsafeEnvironmentVar")
	})
}

// TestCommandGroup_NewFields tests the new fields added to CommandGroup for template replacement
func TestCommandGroup_NewFields(t *testing.T) {
	// Setup test environment
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	tests := []struct {
		name        string
		group       runnertypes.CommandGroup
		expectError bool
		description string
	}{
		{
			name: "TempDir enabled",
			group: runnertypes.CommandGroup{
				Name:    "test-tempdir",
				TempDir: true,
				Commands: []runnertypes.Command{
					{Name: "test", Cmd: "echo", Args: []string{"hello"}},
				},
				EnvAllowlist: []string{"PATH"},
			},
			expectError: false,
			description: "Should create temporary directory and set it as working directory",
		},
		{
			name: "WorkDir specified",
			group: runnertypes.CommandGroup{
				Name:    "test-workdir",
				WorkDir: "/tmp",
				Commands: []runnertypes.Command{
					{Name: "test", Cmd: "echo", Args: []string{"hello"}},
				},
				EnvAllowlist: []string{"PATH"},
			},
			expectError: false,
			description: "Should set working directory from group WorkDir field",
		},
		{
			name: "Cleanup enabled with TempDir",
			group: runnertypes.CommandGroup{
				Name:    "test-cleanup",
				TempDir: true,
				Cleanup: true,
				Commands: []runnertypes.Command{
					{Name: "test", Cmd: "echo", Args: []string{"hello"}},
				},
				EnvAllowlist: []string{"PATH"},
			},
			expectError: false,
			description: "Should create temporary directory with cleanup enabled",
		},
		{
			name: "Command with existing Dir should not be overridden",
			group: runnertypes.CommandGroup{
				Name:    "test-existing-dir",
				WorkDir: "/tmp",
				Commands: []runnertypes.Command{
					{Name: "test", Cmd: "echo", Args: []string{"hello"}, Dir: "/usr"},
				},
				EnvAllowlist: []string{"PATH"},
			},
			expectError: false,
			description: "Commands with existing Dir should not be overridden by group WorkDir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					WorkDir:      "/tmp",
					EnvAllowlist: []string{"PATH"},
				},
				Groups: []runnertypes.CommandGroup{tt.group},
			}

			// Create runner with mock executor to avoid actually executing commands
			mockExecutor := &MockExecutor{}
			mockExecutor.On("Execute", mock.Anything, mock.Anything, mock.Anything).Return(
				&executor.Result{ExitCode: 0, Stdout: "test output", Stderr: ""}, nil)

			runner, err := NewRunner(config, WithExecutor(mockExecutor))
			require.NoError(t, err)

			// Load basic environment
			err = runner.LoadEnvironment("", true)
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, tt.group)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)

				// Verify mock was called
				mockExecutor.AssertExpectations(t)

				// Additional verification based on test case
				switch tt.name {
				case "WorkDir specified", "Command with existing Dir should not be overridden":
					// Verify the command was called with the expected working directory
					calls := mockExecutor.Calls
					require.Len(t, calls, 1)
					cmd, ok := calls[0].Arguments[1].(runnertypes.Command)
					require.True(t, ok, "expected calls[0].Arguments[1] to be of type runnertypes.Command, but it was not")
					if tt.name == "WorkDir specified" {
						assert.Equal(t, "/tmp", cmd.Dir)
					} else {
						assert.Equal(t, "/usr", cmd.Dir) // Should not be overridden
					}
				}
			}
		})
	}
}

// TestCommandGroup_TempDir_Detailed tests TempDir functionality with detailed mock expectations
func TestCommandGroup_TempDir_Detailed(t *testing.T) {
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	t.Run("TempDir creates directory and sets Dir field", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
		}

		group := runnertypes.CommandGroup{
			Name:    "test-tempdir-detailed",
			TempDir: true,
			Commands: []runnertypes.Command{
				{Name: "test", Cmd: "echo", Args: []string{"hello"}},
			},
			EnvAllowlist: []string{"PATH"},
		}

		// Create mock file system
		mockFS := &MockFileSystem{}

		// Set expectation for MkdirAll - resource manager will create temp directory
		mockFS.On("MkdirAll", mock.AnythingOfType("string"), mock.AnythingOfType("fs.FileMode")).Return(nil)
		// Set expectation for RemoveAll - resource manager will clean up temp directory
		mockFS.On("RemoveAll", mock.AnythingOfType("string")).Return(nil)

		// Create resource manager with mock filesystem
		resourceManager := resource.NewManagerWithFS("/tmp", mockFS)

		// Create mock executor
		mockExecutor := &MockExecutor{}

		// Set expectation for Execute - verify that Dir field is properly set
		mockExecutor.On("Execute", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			// Verify that the command's Dir field has been set to a temp directory path
			return cmd.Name == "test" &&
				cmd.Cmd == "echo" &&
				len(cmd.Args) == 1 && cmd.Args[0] == "hello" &&
				cmd.Dir != "" && // Dir should be set
				strings.Contains(cmd.Dir, "/tmp") // Should contain temp directory
		}), mock.Anything).Return(
			&executor.Result{ExitCode: 0, Stdout: "hello\n", Stderr: ""}, nil)

		// Create runner with mocks
		runner, err := NewRunner(config,
			WithExecutor(mockExecutor),
			WithResourceManager(resourceManager))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadEnvironment("", true)
		require.NoError(t, err)

		// Execute the group
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Verify no error occurred
		assert.NoError(t, err)

		// Verify all mock expectations were met
		mockFS.AssertExpectations(t)
		mockExecutor.AssertExpectations(t)

		// Verify that MkdirAll was called (temp directory was created)
		mockFS.AssertCalled(t, "MkdirAll", mock.AnythingOfType("string"), mock.AnythingOfType("fs.FileMode"))
	})

	t.Run("TempDir with cleanup enabled", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
		}

		group := runnertypes.CommandGroup{
			Name:    "test-tempdir-cleanup",
			TempDir: true,
			Cleanup: true,
			Commands: []runnertypes.Command{
				{Name: "test", Cmd: "echo", Args: []string{"hello"}},
			},
			EnvAllowlist: []string{"PATH"},
		}

		// Create mock file system
		mockFS := &MockFileSystem{}

		// Set expectation for MkdirAll - resource manager will create temp directory
		mockFS.On("MkdirAll", mock.AnythingOfType("string"), mock.AnythingOfType("fs.FileMode")).Return(nil)
		// Set expectation for RemoveAll - resource manager will clean up temp directory
		mockFS.On("RemoveAll", mock.AnythingOfType("string")).Return(nil)

		// Create resource manager with mock filesystem
		resourceManager := resource.NewManagerWithFS("/tmp", mockFS)

		// Create mock executor
		mockExecutor := &MockExecutor{}

		// Set expectation for Execute - verify that Dir field is properly set
		mockExecutor.On("Execute", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			// Verify that the command's Dir field has been set to a temp directory path
			return cmd.Name == "test" &&
				cmd.Cmd == "echo" &&
				len(cmd.Args) == 1 && cmd.Args[0] == "hello" &&
				cmd.Dir != "" && // Dir should be set
				strings.Contains(cmd.Dir, "/tmp") // Should contain temp directory
		}), mock.Anything).Return(
			&executor.Result{ExitCode: 0, Stdout: "hello\n", Stderr: ""}, nil)

		// Create runner with mocks
		runner, err := NewRunner(config,
			WithExecutor(mockExecutor),
			WithResourceManager(resourceManager))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadEnvironment("", true)
		require.NoError(t, err)

		// Execute the group
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Verify no error occurred
		assert.NoError(t, err)

		// Verify all mock expectations were met
		mockFS.AssertExpectations(t)
		mockExecutor.AssertExpectations(t)

		// Verify that MkdirAll was called (temp directory was created)
		mockFS.AssertCalled(t, "MkdirAll", mock.AnythingOfType("string"), mock.AnythingOfType("fs.FileMode"))
	})

	t.Run("Command with existing Dir is not overridden by TempDir", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
		}

		group := runnertypes.CommandGroup{
			Name:    "test-existing-dir",
			TempDir: true, // TempDir is enabled
			Commands: []runnertypes.Command{
				{Name: "test", Cmd: "echo", Args: []string{"hello"}, Dir: "/existing/dir"}, // But command already has Dir
			},
			EnvAllowlist: []string{"PATH"},
		}

		// Create mock file system
		mockFS := &MockFileSystem{}

		// Set expectation for MkdirAll - temp directory should still be created
		mockFS.On("MkdirAll", mock.AnythingOfType("string"), mock.AnythingOfType("fs.FileMode")).Return(nil)
		// Set expectation for RemoveAll - resource manager will clean up temp directory
		mockFS.On("RemoveAll", mock.AnythingOfType("string")).Return(nil)

		// Create resource manager with mock filesystem
		resourceManager := resource.NewManagerWithFS("/tmp", mockFS)

		// Create mock executor
		mockExecutor := &MockExecutor{}

		// Set expectation for Execute - verify that existing Dir is preserved
		mockExecutor.On("Execute", mock.Anything, runnertypes.Command{
			Name: "test",
			Cmd:  "echo",
			Args: []string{"hello"},
			Dir:  "/existing/dir", // Should preserve original Dir
		}, mock.Anything).Return(
			&executor.Result{ExitCode: 0, Stdout: "hello\n", Stderr: ""}, nil)

		// Create runner with mocks
		runner, err := NewRunner(config,
			WithExecutor(mockExecutor),
			WithResourceManager(resourceManager))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadEnvironment("", true)
		require.NoError(t, err)

		// Execute the group
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Verify no error occurred
		assert.NoError(t, err)

		// Verify all mock expectations were met
		mockFS.AssertExpectations(t)
		mockExecutor.AssertExpectations(t)
	})
}

// TestRunner_EnvironmentVariablePriority tests the priority hierarchy for environment variables:
// command-specific > global (loaded from system/env file)
func TestRunner_EnvironmentVariablePriority(t *testing.T) {
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      "/tmp",
			EnvAllowlist: []string{"GLOBAL_VAR", "CMD_VAR", "OVERRIDE_VAR", "REFERENCE_VAR"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "test-group",
				EnvAllowlist: []string{"GLOBAL_VAR", "CMD_VAR", "OVERRIDE_VAR", "REFERENCE_VAR"},
			},
		},
	}

	runner, err := NewRunner(config)
	require.NoError(t, err)

	// Set global environment variables (loaded from system/env file)
	runner.envVars = map[string]string{
		"GLOBAL_VAR":    "global_value",
		"OVERRIDE_VAR":  "global_override",
		"REFERENCE_VAR": "global_reference",
	}

	tests := []struct {
		name           string
		commandEnvVars []string // Command-level environment variables
		expectedValues map[string]string
		description    string
	}{
		{
			name:           "global variables only",
			commandEnvVars: nil,
			expectedValues: map[string]string{
				"GLOBAL_VAR":    "global_value",
				"OVERRIDE_VAR":  "global_override",
				"REFERENCE_VAR": "global_reference",
			},
			description: "Global variables should be available when no command variables override them",
		},
		{
			name:           "command variables override global",
			commandEnvVars: []string{"CMD_VAR=command_value", "OVERRIDE_VAR=command_override"},
			expectedValues: map[string]string{
				"GLOBAL_VAR":   "global_value",     // Global variable unchanged
				"CMD_VAR":      "command_value",    // Command-specific variable
				"OVERRIDE_VAR": "command_override", // Command overrides global
			},
			description: "Command environment variables should override global variables",
		},
		{
			name:           "variable references with command priority",
			commandEnvVars: []string{"REFERENCE_VAR=${GLOBAL_VAR}_referenced"},
			expectedValues: map[string]string{
				"GLOBAL_VAR":    "global_value",
				"REFERENCE_VAR": "global_value_referenced", // Should resolve to global variable value
			},
			description: "Variable references should resolve using available variables",
		},
		{
			name:           "command variable references other command variables",
			commandEnvVars: []string{"CMD_VAR=command_value", "REFERENCE_VAR=${CMD_VAR}_referenced"},
			expectedValues: map[string]string{
				"CMD_VAR":       "command_value",
				"REFERENCE_VAR": "command_value_referenced", // Should resolve to command variable value
			},
			description: "Command variables should be able to reference other command variables",
		},
		{
			name:           "command direct value overrides reference",
			commandEnvVars: []string{"REFERENCE_VAR=direct_command_value"},
			expectedValues: map[string]string{
				"REFERENCE_VAR": "direct_command_value", // Command value should override global
			},
			description: "Direct command variables should override global variables",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test command with command-level environment variables
			testCmd := runnertypes.Command{
				Name: "test-env-priority",
				Cmd:  "echo",
				Args: []string{"test"},
			}
			if tt.commandEnvVars != nil {
				testCmd.Env = tt.commandEnvVars
			}

			// Resolve environment variables using the runner
			testGroup := &config.Groups[0]
			resolvedEnv, err := runner.resolveEnvironmentVars(testCmd, testGroup)
			require.NoError(t, err, tt.description)

			// Verify expected values are present with correct priority
			for key, expectedValue := range tt.expectedValues {
				actualValue, exists := resolvedEnv[key]
				assert.True(t, exists, "Environment variable %s should exist in %s", key, tt.name)
				assert.Equal(t, expectedValue, actualValue, "Environment variable %s should have correct value in %s", key, tt.name)
			}
		})
	}
}

// TestRunner_EnvironmentVariablePriority_EdgeCases tests edge cases for environment variable priority
func TestRunner_EnvironmentVariablePriority_EdgeCases(t *testing.T) {
	cleanup := setupSafeTestEnv(t)
	defer cleanup()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      "/tmp",
			EnvAllowlist: []string{"EDGE_VAR", "CIRCULAR_VAR", "UNDEFINED_REF"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "test-group",
				EnvAllowlist: []string{"EDGE_VAR", "CIRCULAR_VAR", "UNDEFINED_REF"},
			},
		},
	}

	runner, err := NewRunner(config)
	require.NoError(t, err)

	// Set global environment variables
	runner.envVars = map[string]string{
		"EDGE_VAR": "global_edge_value",
	}

	t.Run("empty variable values", func(t *testing.T) {
		testGroup := config.Groups[0]

		testCmd := runnertypes.Command{
			Name: "test-empty",
			Cmd:  "echo",
			Args: []string{"test"},
			Env:  []string{"EDGE_VAR="}, // Empty value at command level
		}

		resolvedEnv, err := runner.resolveEnvironmentVars(testCmd, &testGroup)
		require.NoError(t, err)

		// Command value should override global value even if empty
		assert.Equal(t, "", resolvedEnv["EDGE_VAR"])
	})

	t.Run("malformed environment variable format", func(t *testing.T) {
		testGroup := config.Groups[0]

		testCmd := runnertypes.Command{
			Name: "test-malformed",
			Cmd:  "echo",
			Args: []string{"test"},
			Env:  []string{"MALFORMED_VAR"}, // No equals sign
		}

		_, err := runner.resolveEnvironmentVars(testCmd, &testGroup)
		// Should handle malformed environment variables gracefully
		assert.NoError(t, err)
	})

	t.Run("variable reference to undefined variable", func(t *testing.T) {
		testGroup := config.Groups[0]

		testCmd := runnertypes.Command{
			Name: "test-undefined-ref",
			Cmd:  "echo",
			Args: []string{"test"},
			Env:  []string{"UNDEFINED_REF=${NONEXISTENT_VAR}"},
		}

		_, err := runner.resolveEnvironmentVars(testCmd, &testGroup)
		// Should fail when referencing undefined variable
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrVariableAccessDenied))
	})

	t.Run("circular reference in command variables", func(t *testing.T) {
		testGroup := config.Groups[0]

		testCmd := runnertypes.Command{
			Name: "test-circular",
			Cmd:  "echo",
			Args: []string{"test"},
			Env:  []string{"CIRCULAR_VAR=${CIRCULAR_VAR}"},
		}

		_, err := runner.resolveEnvironmentVars(testCmd, &testGroup)
		// Should detect and fail on circular references
		// Note: Current implementation detects this as undefined variable rather than circular reference
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "undefined variable: CIRCULAR_VAR")
	})
}
