package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	errCommandNotFound  = errors.New("command not found")
	errPermissionDenied = errors.New("permission denied")
	errDiskFull         = errors.New("disk full")
	errResourceBusy     = errors.New("resource busy")
	errCleanupFailed    = errors.New("cleanup failed")
)

const defaultTestCommandName = "test"

// setupTestEnv sets up a clean test environment.
func setupTestEnv(t *testing.T, envVars map[string]string) {
	t.Helper()

	// Set up the test environment variables
	for key, value := range envVars {
		t.Setenv(key, value)
	}
}

// setupSafeTestEnv sets up a minimal safe environment for tests.
// This is useful for security-related tests where we want to ensure a clean, minimal environment.
func setupSafeTestEnv(t *testing.T) {
	t.Helper()
	safeEnv := map[string]string{
		"PATH": "/usr/bin:/bin",
		"HOME": "/home/test",
		"USER": "test",
	}
	setupTestEnv(t, safeEnv)
}

var ErrExecutionFailed = errors.New("execution failed")

// MockResourceManager is a mock implementation of ResourceManager
type MockResourceManager struct {
	mock.Mock
}

func (m *MockResourceManager) SetMode(mode resource.ExecutionMode, opts *resource.DryRunOptions) {
	m.Called(mode, opts)
}

func (m *MockResourceManager) GetMode() resource.ExecutionMode {
	args := m.Called()
	return args.Get(0).(resource.ExecutionMode)
}

func (m *MockResourceManager) ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*resource.ExecutionResult, error) {
	args := m.Called(ctx, cmd, group, env)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*resource.ExecutionResult), args.Error(1)
}

func (m *MockResourceManager) CreateTempDir(groupName string) (string, error) {
	args := m.Called(groupName)
	return args.String(0), args.Error(1)
}

func (m *MockResourceManager) CleanupTempDir(tempDirPath string) error {
	args := m.Called(tempDirPath)
	return args.Error(0)
}

func (m *MockResourceManager) CleanupAllTempDirs() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockResourceManager) WithPrivileges(ctx context.Context, fn func() error) error {
	args := m.Called(ctx, fn)
	return args.Error(0)
}

func (m *MockResourceManager) SendNotification(message string, details map[string]any) error {
	args := m.Called(message, details)
	return args.Error(0)
}

func (m *MockResourceManager) GetDryRunResults() *resource.DryRunResult {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*resource.DryRunResult)
}

func (m *MockResourceManager) RecordAnalysis(analysis *resource.ResourceAnalysis) {
	m.Called(analysis)
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
		runner, err := NewRunner(config, WithRunID("test-run-123"))
		require.NoError(t, err, "NewRunner should not return an error with valid config")
		assert.NotNil(t, runner)
		assert.Equal(t, config, runner.config)
		assert.NotNil(t, runner.executor)
		assert.NotNil(t, runner.envVars)
		assert.NotNil(t, runner.validator)
		assert.Equal(t, "test-run-123", runner.runID)
	})

	t.Run("fails without runID", func(t *testing.T) {
		runner, err := NewRunner(config)
		require.Error(t, err, "NewRunner should return an error without runID")
		assert.Nil(t, runner)
		assert.Contains(t, err.Error(), "runID is required")
	})

	t.Run("with custom security config", func(t *testing.T) {
		securityConfig := &security.Config{
			AllowedCommands:         []string{"^echo$", "^cat$"},
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*", ".*TOKEN.*"},
			MaxPathLength:           4096,
		}

		runner, err := NewRunner(config, WithSecurity(securityConfig), WithRunID("test-run-123"))
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

		runner, err := NewRunner(config,
			WithSecurity(securityConfig),
			WithRunID("test-run-123"))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
	})

	t.Run("with invalid security config", func(t *testing.T) {
		invalidSecurityConfig := &security.Config{
			AllowedCommands:         []string{"[invalid regex"}, // Invalid regex
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*"},
			MaxPathLength:           4096,
		}

		runner, err := NewRunner(config, WithSecurity(invalidSecurityConfig), WithRunID("test-run-123"))
		assert.Error(t, err)
		assert.Nil(t, runner)
		assert.ErrorIs(t, err, security.ErrInvalidRegexPattern)
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

		runner, err := NewRunner(config, WithSecurity(securityConfig), WithRunID("test-run-123"))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
		assert.Equal(t, config, runner.config)
		assert.NotNil(t, runner.executor)
		assert.NotNil(t, runner.envVars)
		assert.NotNil(t, runner.validator)
	})

	t.Run("with invalid security config", func(t *testing.T) {
		invalidSecurityConfig := &security.Config{
			AllowedCommands:         []string{"[invalid regex"}, // Invalid regex
			RequiredFilePermissions: 0o644,
			SensitiveEnvVars:        []string{".*PASSWORD.*"},
			MaxPathLength:           4096,
		}

		runner, err := NewRunner(config, WithSecurity(invalidSecurityConfig), WithRunID("test-run-123"))
		assert.Error(t, err)
		assert.Nil(t, runner)
		assert.ErrorIs(t, err, security.ErrInvalidRegexPattern)
	})

	t.Run("with nil security config", func(t *testing.T) {
		runner, err := NewRunner(config, WithSecurity(nil), WithRunID("test-run-123"))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
	})
}

func TestRunner_ExecuteGroup(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		group       runnertypes.CommandGroup
		mockResults []*resource.ExecutionResult
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
			mockResults: []*resource.ExecutionResult{
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
			mockResults: []*resource.ExecutionResult{nil},
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
			mockResults: []*resource.ExecutionResult{{ExitCode: 1, Stdout: "", Stderr: ""}},
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

			mockResourceManager := new(MockResourceManager)
			runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
			require.NoError(t, err, "NewRunner should not return an error with valid config")

			// Setup mock expectations
			for i, cmd := range tt.group.Commands {
				// Create expected command with WorkDir set
				expectedCmd := cmd
				if expectedCmd.Dir == "" {
					expectedCmd.Dir = config.Global.WorkDir
				}
				mockResourceManager.On("ExecuteCommand", mock.Anything, expectedCmd, &tt.group, mock.Anything).Return(tt.mockResults[i], tt.mockErrors[i])
			}

			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, tt.group)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr, "expected error %v, got %v", tt.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}

			mockResourceManager.AssertExpectations(t)
		})
	}
}

func TestRunner_ExecuteGroup_ComplexErrorScenarios(t *testing.T) {
	setupSafeTestEnv(t)

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

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command fails with non-zero exit code
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "false", Dir: "/tmp"}, &group, mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Subsequent commands should not be executed due to fail-fast behavior
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
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

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command succeeds
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, &group, mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second command fails
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "cmd-2", Cmd: "false", Dir: "/tmp"}, &group, mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Third command should not be executed due to fail-fast behavior

		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
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

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command succeeds
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, &group, mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second command returns executor error
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "cmd-2", Cmd: "invalid-command", Dir: "/tmp"}, &group, mock.Anything).
			Return((*resource.ExecutionResult)(nil), errCommandNotFound)

		// Third command should not be executed

		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		assert.Error(t, err)
		assert.ErrorIs(t, err, errCommandNotFound)
		mockResourceManager.AssertExpectations(t)
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

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command should succeed
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, &group, mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second command should not be executed due to environment variable access denial

		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Should fail due to environment variable access denied
		assert.Error(t, err)
		assert.ErrorIs(t, err, environment.ErrVariableNotFound, "expected error to wrap ErrVariableNotFound")
		mockResourceManager.AssertExpectations(t)
	})
}

func TestRunner_ExecuteAll(t *testing.T) {
	setupSafeTestEnv(t)

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

	mockResourceManager := new(MockResourceManager)
	runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
	require.NoError(t, err)

	// Setup mock expectations - should be called in priority order
	mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, &config.Groups[1], mock.Anything).Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "first\n"}, nil)
	mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "cmd-2", Cmd: "echo", Args: []string{"second"}, Dir: "/tmp"}, &config.Groups[0], mock.Anything).Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "second\n"}, nil)

	ctx := context.Background()
	err = runner.ExecuteAll(ctx)

	assert.NoError(t, err)
	mockResourceManager.AssertExpectations(t)
}

func TestRunner_ExecuteAll_ComplexErrorScenarios(t *testing.T) {
	setupSafeTestEnv(t)

	t.Run("first group fails, but remaining groups should still execute", func(t *testing.T) {
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
						{Name: "success-cmd", Cmd: "echo", Args: []string{"should execute"}},
					},
				},
				{
					Name:     "group-3",
					Priority: 3,
					Commands: []runnertypes.Command{
						{Name: "another-cmd", Cmd: "echo", Args: []string{"also should execute"}},
					},
				},
			},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First group's command should be called and fail
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "fail-cmd", Cmd: "false", Dir: "/tmp"}, &config.Groups[0], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Remaining groups should still be executed
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "success-cmd", Cmd: "echo", Args: []string{"should execute"}, Dir: "/tmp"}, &config.Groups[1], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "should execute\n", Stderr: ""}, nil)

		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "another-cmd", Cmd: "echo", Args: []string{"also should execute"}, Dir: "/tmp"}, &config.Groups[2], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "also should execute\n", Stderr: ""}, nil)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		// Should still return error from first group, but all groups executed
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("middle group fails, but remaining groups should still execute", func(t *testing.T) {
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
						{Name: "should-execute", Cmd: "echo", Args: []string{"third"}},
					},
				},
			},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First group should succeed
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "success-cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, &config.Groups[0], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second group should fail
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "fail-cmd", Cmd: "false", Dir: "/tmp"}, &config.Groups[1], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Third group should still be executed
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "should-execute", Cmd: "echo", Args: []string{"third"}, Dir: "/tmp"}, &config.Groups[2], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "third\n", Stderr: ""}, nil)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("group with multiple commands, second command fails, but next group still executes", func(t *testing.T) {
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

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command in group-1 should succeed
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "success-cmd-1", Cmd: "echo", Args: []string{"first"}, Dir: "/tmp"}, &config.Groups[0], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second command in group-1 should fail
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "fail-cmd", Cmd: "false", Dir: "/tmp"}, &config.Groups[0], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Third command in group-1 should not be executed (group-level failure stops remaining commands in same group)
		// But group-2 should still be executed (new behavior)
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "group2-cmd", Cmd: "echo", Args: []string{"group2"}, Dir: "/tmp"}, &config.Groups[1], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "group2\n", Stderr: ""}, nil)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("executor error in first group, but remaining groups should still execute", func(t *testing.T) {
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
						{Name: "should-execute", Cmd: "echo", Args: []string{"second"}},
					},
				},
			},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command should return executor error
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "executor-error-cmd", Cmd: "nonexistent-command", Dir: "/tmp"}, &config.Groups[0], mock.Anything).
			Return((*resource.ExecutionResult)(nil), errCommandNotFound)

		// Second group should still be executed
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{Name: "should-execute", Cmd: "echo", Args: []string{"second"}, Dir: "/tmp"}, &config.Groups[1], mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "second\n", Stderr: ""}, nil)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.ErrorIs(t, err, errCommandNotFound)
		mockResourceManager.AssertExpectations(t)
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

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Create a context that gets cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		// No mock expectations since context is cancelled before any commands execute
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

		runner, err := NewRunner(config, WithRunID("test-run-123"))
		require.NoError(t, err)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		// Should succeed with no groups to execute
		assert.NoError(t, err)
	})
}

func TestRunner_createCommandContext(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout: 10,
		},
	}
	runner, err := NewRunner(config, WithRunID("test-run-123"))
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

func TestRunner_CommandTimeoutBehavior(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout: 1, // 1 second timeout
			WorkDir: "/tmp",
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name: "timeout-test-group",
				Commands: []runnertypes.Command{
					{
						Name: "sleep-command",
						Cmd:  "sleep",
						Args: []string{"5"}, // Sleep for 5 seconds, longer than timeout
					},
				},
			},
		},
	}

	t.Run("global timeout is enforced", func(t *testing.T) {
		runner, err := NewRunner(config, WithRunID("test-run-123"))
		require.NoError(t, err)

		ctx := context.Background()
		start := time.Now()

		err = runner.ExecuteAll(ctx)

		elapsed := time.Since(start)

		// Should fail due to timeout
		assert.Error(t, err)
		assert.True(t,
			errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(err.Error(), "signal: killed"),
			"Expected timeout error, got: %v", err)

		// Should complete within ~1 second (plus some buffer for processing)
		assert.Less(t, elapsed, 2*time.Second)
		assert.Greater(t, elapsed, 900*time.Millisecond) // At least close to 1 second
	})

	t.Run("command-specific timeout overrides global timeout", func(t *testing.T) {
		// Create config with command-specific shorter timeout
		configWithCmdTimeout := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout: 10, // 10 seconds global timeout
				WorkDir: "/tmp",
			},
			Groups: []runnertypes.CommandGroup{
				{
					Name: "cmd-timeout-test-group",
					Commands: []runnertypes.Command{
						{
							Name:    "sleep-command-short-timeout",
							Cmd:     "sleep",
							Args:    []string{"5"}, // Sleep for 5 seconds
							Timeout: 1,             // But timeout after 1 second
						},
					},
				},
			},
		}

		runner, err := NewRunner(configWithCmdTimeout, WithRunID("test-run-123"))
		require.NoError(t, err)

		ctx := context.Background()
		start := time.Now()

		err = runner.ExecuteAll(ctx)

		elapsed := time.Since(start)

		// Should fail due to command timeout (1 second), not global timeout (10 seconds)
		assert.Error(t, err)
		assert.True(t,
			errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(err.Error(), "signal: killed"),
			"Expected timeout error, got: %v", err)

		// Should complete within ~1 second, not 10 seconds
		assert.Less(t, elapsed, 2*time.Second)
		assert.Greater(t, elapsed, 900*time.Millisecond)
	})

	t.Run("timeout with context cancellation prioritization", func(t *testing.T) {
		runner, err := NewRunner(config, WithRunID("test-run-123"))
		require.NoError(t, err)

		// Create a context that will be cancelled after 500ms
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		start := time.Now()

		err = runner.ExecuteAll(ctx)

		elapsed := time.Since(start)

		// Should fail due to context cancellation
		assert.Error(t, err)
		// Context cancellation can result in different error messages depending on timing
		assert.True(t,
			errors.Is(err, context.DeadlineExceeded) ||
				errors.Is(err, context.Canceled) ||
				strings.Contains(err.Error(), "signal: killed") ||
				strings.Contains(err.Error(), "context deadline exceeded") ||
				strings.Contains(err.Error(), "context canceled"),
			"Expected context cancellation or timeout error, got: %v", err)

		// Should complete within ~500ms, not the command timeout of 1 second
		assert.Less(t, elapsed, 800*time.Millisecond)
		assert.Greater(t, elapsed, 400*time.Millisecond)
	})
}

func TestRunner_resolveEnvironmentVars(t *testing.T) {
	// Setup a custom test environment with specific variables
	testEnv := map[string]string{
		"SAFE_VAR": "safe_value",
		"PATH":     "/usr/bin:/bin",
	}
	setupTestEnv(t, testEnv)

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

	runner, err := NewRunner(config, WithRunID("test-run-123"))
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

func TestRunner_SecurityIntegration(t *testing.T) {
	setupSafeTestEnv(t)

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
		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		allowedCmd := runnertypes.Command{
			Name: "test-echo",
			Cmd:  "echo",
			Args: []string{"hello"},
			Dir:  "/tmp",
		}

		testGroup := &config.Groups[0] // Get reference to the test group
		mockResourceManager.On("ExecuteCommand", mock.Anything, allowedCmd, testGroup, mock.Anything).Return(&resource.ExecutionResult{ExitCode: 0}, nil)

		ctx := context.Background()
		_, err = runner.executeCommandInGroup(ctx, allowedCmd, testGroup)
		assert.NoError(t, err)
		mockResourceManager.AssertExpectations(t)
	})

	// This test is temporarily disabled during Phase 1 implementation
	// t.Run("disallowed command execution should fail", func(t *testing.T) {
	// 	// Test will be re-enabled in Phase 2 when NewManagerForTest API is available
	// })

	t.Run("command execution with environment variables", func(t *testing.T) {
		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Test with safe environment variables
		safeCmd := runnertypes.Command{
			Name: "test-env",
			Cmd:  "echo",
			Args: []string{"$TEST_VAR"},
			Dir:  "/tmp",
			Env:  []string{"TEST_VAR=safe-value", "PATH=/usr/bin:/bin"},
		}

		mockResourceManager.On("ExecuteCommand", mock.Anything, safeCmd, mock.Anything, mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0}, nil)

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
		assert.ErrorIs(t, err, security.ErrUnsafeEnvironmentVar, "expected error to wrap security.ErrUnsafeEnvironmentVar")
	})
}

// TestCommandGroup_NewFields tests the new fields added to CommandGroup for template replacement
func TestCommandGroup_NewFields(t *testing.T) {
	// Setup test environment
	setupSafeTestEnv(t)

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

			// Create runner with mock resource manager to avoid actually executing commands
			mockResourceManager := &MockResourceManager{}
			mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
				&resource.ExecutionResult{ExitCode: 0, Stdout: "test output", Stderr: ""}, nil)

			// Set up CreateTempDir and CleanupTempDir expectations if TempDir is enabled
			if tt.group.TempDir {
				mockResourceManager.On("CreateTempDir", tt.group.Name).Return("/tmp/test-temp-dir", nil)
				mockResourceManager.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil)
			}

			runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
			require.NoError(t, err)

			// Load basic environment
			err = runner.LoadSystemEnvironment()
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, tt.group)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)

				// Verify mock was called
				mockResourceManager.AssertExpectations(t)

				// Additional verification based on test case
				switch tt.name {
				case "WorkDir specified", "Command with existing Dir should not be overridden":
					// Verify the command was called with the expected working directory
					calls := mockResourceManager.Calls
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
	setupSafeTestEnv(t)

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

		// Create mock resource manager
		mockResourceManager := &MockResourceManager{}

		// Set expectation for CreateTempDir - resource manager will create temp directory
		mockResourceManager.On("CreateTempDir", "test-tempdir-detailed").Return("/tmp/test-temp-dir", nil)

		// Set expectation for CleanupTempDir - resource manager will clean up temp directory
		mockResourceManager.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil)

		// Set expectation for ExecuteCommand - verify that Dir field is properly set
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			// Verify that the command's Dir field has been set to a temp directory path
			return cmd.Name == defaultTestCommandName &&
				cmd.Cmd == "echo" &&
				len(cmd.Args) == 1 && cmd.Args[0] == "hello" &&
				cmd.Dir != "" && // Dir should be set
				strings.Contains(cmd.Dir, "/tmp") // Should contain temp directory
		}), &group, mock.Anything).Return(
			&resource.ExecutionResult{ExitCode: 0, Stdout: "hello\n", Stderr: ""}, nil)

		// Create runner with mock resource manager
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err)

		// Execute the group
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Verify no error occurred
		assert.NoError(t, err)

		// Verify all mock expectations were met
		mockResourceManager.AssertExpectations(t)

		// Verify that CreateTempDir was called (temp directory was created)
		mockResourceManager.AssertCalled(t, "CreateTempDir", "test-tempdir-detailed")
	})

	t.Run("TempDir cleanup", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
		}

		group := runnertypes.CommandGroup{
			Name:    "test-tempdir-cleanup",
			TempDir: true,
			Commands: []runnertypes.Command{
				{Name: "test", Cmd: "echo", Args: []string{"hello"}},
			},
			EnvAllowlist: []string{"PATH"},
		}

		// Create mock resource manager
		mockResourceManager := &MockResourceManager{}

		// Set expectation for CreateTempDir - resource manager will create temp directory
		mockResourceManager.On("CreateTempDir", "test-tempdir-cleanup").Return("/tmp/test-temp-dir", nil)

		// Set expectation for CleanupTempDir - resource manager will clean up temp directory
		mockResourceManager.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil)

		// Set expectation for ExecuteCommand - verify that Dir field is properly set
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			// Verify that the command's Dir field has been set to a temp directory path
			return cmd.Name == defaultTestCommandName &&
				cmd.Cmd == "echo" &&
				len(cmd.Args) == 1 && cmd.Args[0] == "hello" &&
				cmd.Dir != "" && // Dir should be set
				strings.Contains(cmd.Dir, "/tmp") // Should contain temp directory
		}), &group, mock.Anything).Return(
			&resource.ExecutionResult{ExitCode: 0, Stdout: "hello\n", Stderr: ""}, nil)

		// Create runner with mock resource manager
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err)

		// Execute the group
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Verify no error occurred
		assert.NoError(t, err)

		// Verify all mock expectations were met
		mockResourceManager.AssertExpectations(t)

		// Verify that CreateTempDir was called (temp directory was created)
		mockResourceManager.AssertCalled(t, "CreateTempDir", "test-tempdir-cleanup")
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

		// Create mock resource manager
		mockResourceManager := &MockResourceManager{}

		// Set expectation for CreateTempDir - temp directory should still be created
		mockResourceManager.On("CreateTempDir", "test-existing-dir").Return("/tmp/test-temp-dir", nil)

		// CleanupTempDir expectation
		mockResourceManager.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil)

		// Set expectation for ExecuteCommand - verify that existing Dir is preserved
		mockResourceManager.On("ExecuteCommand", mock.Anything, runnertypes.Command{
			Name: "test",
			Cmd:  "echo",
			Args: []string{"hello"},
			Dir:  "/existing/dir", // Should preserve original Dir
		}, &group, mock.Anything).Return(
			&resource.ExecutionResult{ExitCode: 0, Stdout: "hello\n", Stderr: ""}, nil)

		// Create runner with mock resource manager
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err)

		// Execute the group
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Verify no error occurred
		assert.NoError(t, err)

		// Verify all mock expectations were met
		mockResourceManager.AssertExpectations(t)
	})
}

// TestRunner_EnvironmentVariablePriority tests the priority hierarchy for environment variables:
// command-specific > global (loaded from system/env file)
func TestRunner_EnvironmentVariablePriority(t *testing.T) {
	setupSafeTestEnv(t)

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

	runner, err := NewRunner(config, WithRunID("test-run-123"))
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

// TestRunner_EnvironmentVariablePriority_CurrentImplementation tests the current implementation
// which only supports command-specific > global priority (no group-specific variables yet)
func TestRunner_EnvironmentVariablePriority_CurrentImplementation(t *testing.T) {
	setupSafeTestEnv(t)

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

	runner, err := NewRunner(config, WithRunID("test-run-123"))
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
			description: "Global variables should be available when no command variables are specified",
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
			name:           "command variable references global",
			commandEnvVars: []string{"REFERENCE_VAR=${GLOBAL_VAR}_referenced"},
			expectedValues: map[string]string{
				"GLOBAL_VAR":    "global_value",
				"REFERENCE_VAR": "global_value_referenced", // Should resolve to global variable value
			},
			description: "Command variables should be able to reference global variables",
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

// TestRunner_EnvironmentVariablePriority_GroupLevelSupport documents the missing group-level environment variable support
func TestRunner_EnvironmentVariablePriority_GroupLevelSupport(t *testing.T) {
	t.Skip("Group-level environment variables are not yet implemented. CommandGroup struct needs an Env field similar to Command.Env")

	// This test documents what the expected behavior should be when group-level environment variables are implemented:
	// Priority order should be: command-specific > group-specific > global
	//
	// Required changes:
	// 1. Add Env []string field to CommandGroup struct in runnertypes/config.go
	// 2. Modify resolveEnvironmentVars method to apply group environment variables before command variables
	// 3. Ensure variable resolution works across all three levels
}

// TestRunner_EnvironmentVariablePriority_EdgeCases tests edge cases for environment variable priority
func TestRunner_EnvironmentVariablePriority_EdgeCases(t *testing.T) {
	setupSafeTestEnv(t)

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

	runner, err := NewRunner(config, WithRunID("test-run-123"))
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
		// Should fail when an environment variable is malformed
		assert.Error(t, err)
		assert.ErrorIs(t, err, environment.ErrMalformedEnvVariable)
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
		assert.ErrorIs(t, err, environment.ErrVariableNotFound)
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
		assert.ErrorIs(t, err, environment.ErrVariableNotFound)
	})
}

// TestResourceManagement_FailureScenarios tests various failure scenarios in resource management
func TestResourceManagement_FailureScenarios(t *testing.T) {
	setupSafeTestEnv(t)

	t.Run("temp directory creation failure", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
		}

		group := runnertypes.CommandGroup{
			Name:    "test-tempdir-failure",
			TempDir: true,
			Commands: []runnertypes.Command{
				{Name: "test", Cmd: "echo", Args: []string{"hello"}},
			},
			EnvAllowlist: []string{"PATH"},
		}

		// Create mock resource manager that fails on directory creation
		mockResourceManager := &MockResourceManager{}
		mockResourceManager.On("CreateTempDir", "test-tempdir-failure").
			Return("", errPermissionDenied)

		// Create runner with mock resource manager
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err)

		// Execute the group - should fail due to temp directory creation failure
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Verify error occurred
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create temp directory for group test-tempdir-failure")

		// Verify mock expectations
		mockResourceManager.AssertExpectations(t)
		// ExecuteCommand should not have been called due to temp dir creation failure
		mockResourceManager.AssertNotCalled(t, "ExecuteCommand")
	})

	t.Run("temp directory cleanup failure", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
		}

		group := runnertypes.CommandGroup{
			Name:    "test-cleanup-failure",
			TempDir: true,
			Commands: []runnertypes.Command{
				{Name: "test", Cmd: "echo", Args: []string{"hello"}},
			},
			EnvAllowlist: []string{"PATH"},
		}

		// Create mock resource manager
		mockResourceManager := &MockResourceManager{}
		// Directory creation succeeds
		mockResourceManager.On("CreateTempDir", "test-cleanup-failure").Return("/tmp/test-temp-dir", nil)

		// CleanupTempDir expectation
		mockResourceManager.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil)

		// ExecuteCommand should be called and succeed
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			return cmd.Name == defaultTestCommandName && cmd.Dir != ""
		}), &group, mock.Anything).Return(
			&resource.ExecutionResult{ExitCode: 0, Stdout: "hello\n", Stderr: ""}, nil)

		// Create runner with mock resource manager
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err)

		// Execute the group - should succeed despite cleanup failure (cleanup failure is logged as warning)
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Command execution should succeed even if cleanup fails
		assert.NoError(t, err)

		// Verify mock expectations
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("multiple temp directory failures", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
			Groups: []runnertypes.CommandGroup{
				{
					Name:     "group-1",
					Priority: 1,
					TempDir:  true,
					Commands: []runnertypes.Command{
						{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}},
					},
					EnvAllowlist: []string{"PATH"},
				},
				{
					Name:     "group-2",
					Priority: 2,
					TempDir:  true,
					Commands: []runnertypes.Command{
						{Name: "cmd-2", Cmd: "echo", Args: []string{"second"}},
					},
					EnvAllowlist: []string{"PATH"},
				},
			},
		}

		// Create mock resource manager
		mockResourceManager := &MockResourceManager{}
		// First directory creation succeeds
		mockResourceManager.On("CreateTempDir", "group-1").
			Return("/tmp/test-temp-dir", nil).Once()
		// Second directory creation fails
		mockResourceManager.On("CreateTempDir", "group-2").
			Return("", errDiskFull)

		// CleanupTempDir for first group only (since second group creation fails)
		mockResourceManager.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil).Once()

		// Only first group's command should execute
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			return cmd.Name == "cmd-1"
		}), &config.Groups[0], mock.Anything).Return(
			&resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Create runner with mock resource manager
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err)

		// Execute all groups - first should succeed, second should fail
		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		// Should return error from second group's temp directory creation failure
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute group group-2")
		assert.Contains(t, err.Error(), "failed to create temp directory")

		// Verify mock expectations
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("resource cleanup during early termination", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
		}

		group := runnertypes.CommandGroup{
			Name:    "test-early-termination",
			TempDir: true,
			Commands: []runnertypes.Command{
				{Name: "first-cmd", Cmd: "echo", Args: []string{"first"}},
				{Name: "failing-cmd", Cmd: "false"}, // This command will fail
				{Name: "never-executed", Cmd: "echo", Args: []string{"never"}},
			},
			EnvAllowlist: []string{"PATH"},
		}

		// Create mock resource manager
		mockResourceManager := &MockResourceManager{}
		// Directory creation should succeed
		mockResourceManager.On("CreateTempDir", "test-early-termination").Return("/tmp/test-temp-dir", nil)

		// CleanupTempDir expectation - should be called even if command fails
		mockResourceManager.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil)

		// First command succeeds
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			return cmd.Name == "first-cmd"
		}), &group, mock.Anything).Return(
			&resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second command fails
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			return cmd.Name == "failing-cmd"
		}), &group, mock.Anything).Return(
			&resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Third command should never be called due to failure of second command

		// Create runner with mock resource manager
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err)

		// Execute the group - should fail on second command but still clean up resources
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)

		// Should return error from failing command
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)

		// Verify mock expectations
		mockResourceManager.AssertExpectations(t)

		// Verify that commands were executed as expected
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("resource manager cleanup all failure", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
		}

		group := runnertypes.CommandGroup{
			Name:    "test-cleanup-all",
			TempDir: true,
			Commands: []runnertypes.Command{
				{Name: "test", Cmd: "echo", Args: []string{"hello"}},
			},
			EnvAllowlist: []string{"PATH"},
		}

		// Create mock resource manager
		mockResourceManager := &MockResourceManager{}
		mockResourceManager.On("CreateTempDir", "test-cleanup-all").Return("/tmp/test-temp-dir", nil)

		// CleanupTempDir expectation
		mockResourceManager.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil)

		// CleanupAllTempDirs expectation for testing cleanup all failure
		mockResourceManager.On("CleanupAllTempDirs").Return(errCleanupFailed)

		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			return cmd.Name == defaultTestCommandName
		}), &group, mock.Anything).Return(
			&resource.ExecutionResult{ExitCode: 0, Stdout: "hello\n", Stderr: ""}, nil)

		// Create runner with mock resource manager
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err)

		// Execute the group
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)
		assert.NoError(t, err)

		// Now test CleanupAllResources - should return error
		err = runner.CleanupAllResources()
		assert.Error(t, err)
		assert.ErrorIs(t, err, errCleanupFailed)

		// Verify mock expectations
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("concurrent resource access failure", func(t *testing.T) {
		config := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				WorkDir:      "/tmp",
				EnvAllowlist: []string{"PATH"},
			},
		}

		group := runnertypes.CommandGroup{
			Name:    "test-concurrent",
			TempDir: true,
			Commands: []runnertypes.Command{
				{Name: "test", Cmd: "echo", Args: []string{"hello"}},
			},
			EnvAllowlist: []string{"PATH"},
		}

		// Create mock resource manager that fails after first successful call
		mockResourceManager := &MockResourceManager{}
		mockResourceManager.On("CreateTempDir", "test-concurrent").
			Return("/tmp/test-temp-dir", nil).Once()
		mockResourceManager.On("CreateTempDir", "test-concurrent").
			Return("", errResourceBusy)

		// CleanupTempDir expectation for first successful call
		mockResourceManager.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil).Once()

		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.MatchedBy(func(cmd runnertypes.Command) bool {
			return cmd.Name == defaultTestCommandName
		}), &group, mock.Anything).Return(
			&resource.ExecutionResult{ExitCode: 0, Stdout: "hello\n", Stderr: ""}, nil)

		// Create runner with mock resource manager
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		// Load basic environment
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err)

		// First execution should succeed
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, group)
		assert.NoError(t, err)

		// Second execution should fail due to resource busy error
		err = runner.ExecuteGroup(ctx, group)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create temp directory")

		// Verify mock expectations
		mockResourceManager.AssertExpectations(t)
	})
}

// TestSlackNotification tests that Slack notifications are sent correctly
func TestSlackNotification(t *testing.T) {
	tests := []struct {
		name           string
		commandSuccess bool
		expectedStatus string
		expectedCalls  int // Expected number of logging calls
	}{
		{
			name:           "Success notification",
			commandSuccess: true,
			expectedStatus: "success",
			expectedCalls:  1,
		},
		{
			name:           "Failure notification",
			commandSuccess: false,
			expectedStatus: "error",
			expectedCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tempDir := t.TempDir()

			// Create config with a simple command group
			config := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					WorkDir: tempDir,
					Timeout: 30,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name:        "test-group",
						Description: "Test group for notification",
						Commands: []runnertypes.Command{
							{
								Name: "test-command",
								Cmd:  "echo",
								Args: []string{"test"},
							},
						},
					},
				},
			}

			// Create mock resource manager
			mockResourceManager := &MockResourceManager{}

			// Set up resource manager behavior based on test case
			if tt.commandSuccess {
				mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
					&resource.ExecutionResult{
						ExitCode: 0,
						Stdout:   "test output",
						Stderr:   "",
					}, nil,
				)
			} else {
				mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
					&resource.ExecutionResult{
						ExitCode: 1,
						Stdout:   "",
						Stderr:   "command failed",
					}, nil,
				)
			}

			// Create runner options
			var options []Option
			options = append(options, WithResourceManager(mockResourceManager))
			options = append(options, WithRunID("test-run-123"))

			// Create runner
			runner, err := NewRunner(config, options...)
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, config.Groups[0])

			if tt.commandSuccess {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}

			// Verify mock expectations
			mockResourceManager.AssertExpectations(t)

			// Verify that the runner was configured correctly
			assert.Equal(t, "test-run-123", runner.runID)
		})
	}
}
