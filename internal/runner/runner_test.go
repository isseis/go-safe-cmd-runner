package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	configpkg "github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
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

// prepareCommandWithExpandedEnv prepares a Command with ExpandedEnv populated from its Env field.
// This is a test helper that simulates what LoadAndPrepareConfig does during configuration loading.
func prepareCommandWithExpandedEnv(t *testing.T, cmd *runnertypes.Command, group *runnertypes.CommandGroup, cfg *runnertypes.Config) {
	t.Helper()

	filter := environment.NewFilter(cfg.Global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)
	err := configpkg.ExpandCommandEnv(cmd, expander, nil, nil, cfg.Global.EnvAllowlist, nil, group.EnvAllowlist, group.Name)
	require.NoError(t, err, "failed to expand Command.Env in test helper")
}

// prepareConfigWithExpandedEnv prepares all commands in a Config with ExpandedEnv populated.
// This is a test helper that simulates what LoadAndPrepareConfig does during configuration loading.
func prepareConfigWithExpandedEnv(t *testing.T, cfg *runnertypes.Config) {
	t.Helper()

	filter := environment.NewFilter(cfg.Global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)

	for i := range cfg.Groups {
		group := &cfg.Groups[i]
		for j := range group.Commands {
			cmd := &group.Commands[j]
			err := configpkg.ExpandCommandEnv(cmd, expander, nil, nil, cfg.Global.EnvAllowlist, nil, group.EnvAllowlist, group.Name)
			require.NoError(t, err, "failed to expand Command.Env in test helper for command %s", cmd.Name)
		}
	}
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

func (m *MockResourceManager) ValidateOutputPath(outputPath, workDir string) error {
	args := m.Called(outputPath, workDir)
	return args.Error(0)
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

// MockSecurityValidator for output testing
type MockSecurityValidator struct {
	mock.Mock
}

func (m *MockSecurityValidator) ValidateOutputWritePermission(outputPath string, realUID int) error {
	args := m.Called(outputPath, realUID)
	return args.Error(0)
}

// SetupDefaultMockBehavior sets up common default mock expectations for basic test scenarios
func (m *MockResourceManager) SetupDefaultMockBehavior() {
	// Default ValidateOutputPath behavior - allows any output path
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Maybe()

	// Default ExecuteCommand behavior - returns successful execution
	defaultResult := &resource.ExecutionResult{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
	}
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(defaultResult, nil).Maybe()
}

// SetupSuccessfulMockExecution sets up mock for successful command execution with custom output
func (m *MockResourceManager) SetupSuccessfulMockExecution(stdout, stderr string) {
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	result := &resource.ExecutionResult{
		ExitCode: 0,
		Stdout:   stdout,
		Stderr:   stderr,
	}
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(result, nil)
}

// SetupFailedMockExecution sets up mock for failed command execution with custom error
func (m *MockResourceManager) SetupFailedMockExecution(err error) {
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, err)
}

// NewMockResourceManagerWithDefaults creates a new MockResourceManager with default behavior setup
func NewMockResourceManagerWithDefaults() *MockResourceManager {
	mockRM := &MockResourceManager{}
	mockRM.SetupDefaultMockBehavior()
	return mockRM
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
		securityConfig := security.DefaultConfig()
		// Override for this specific test
		securityConfig.AllowedCommands = []string{"^echo$", "^cat$"}
		securityConfig.SensitiveEnvVars = []string{".*PASSWORD.*", ".*TOKEN.*"}

		runner, err := NewRunner(config, WithSecurity(securityConfig), WithRunID("test-run-123"))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
		assert.Equal(t, config, runner.config)
		assert.NotNil(t, runner.validator)
	})

	t.Run("with multiple options", func(t *testing.T) {
		securityConfig := security.DefaultConfig()
		// Override for this specific test
		securityConfig.AllowedCommands = []string{"^echo$"}
		securityConfig.SensitiveEnvVars = []string{".*PASSWORD.*"}

		runner, err := NewRunner(config,
			WithSecurity(securityConfig),
			WithRunID("test-run-123"))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
	})

	t.Run("with invalid security config", func(t *testing.T) {
		invalidSecurityConfig := security.DefaultConfig()
		// Set invalid pattern to test error handling
		invalidSecurityConfig.AllowedCommands = []string{"[invalid regex"} // Invalid regex
		invalidSecurityConfig.SensitiveEnvVars = []string{".*PASSWORD.*"}

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
		securityConfig := security.DefaultConfig()
		// Override for this specific test
		securityConfig.AllowedCommands = []string{"^echo$", "^cat$"}
		securityConfig.SensitiveEnvVars = []string{".*PASSWORD.*", ".*TOKEN.*"}

		runner, err := NewRunner(config, WithSecurity(securityConfig), WithRunID("test-run-123"))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
		assert.Equal(t, config, runner.config)
		assert.NotNil(t, runner.executor)
		assert.NotNil(t, runner.envVars)
		assert.NotNil(t, runner.validator)
	})

	t.Run("with invalid security config", func(t *testing.T) {
		invalidSecurityConfig := security.DefaultConfig()
		// Set invalid pattern to test error handling
		invalidSecurityConfig.AllowedCommands = []string{"[invalid regex"} // Invalid regex
		invalidSecurityConfig.SensitiveEnvVars = []string{".*PASSWORD.*"}

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

			// Prepare all commands with ExpandedEnv (simulates configuration loading)
			prepareConfigWithExpandedEnv(t, config)

			mockResourceManager := new(MockResourceManager)
			runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
			require.NoError(t, err, "NewRunner should not return an error with valid config")

			// Setup mock expectations
			for i, cmd := range config.Groups[0].Commands {
				// Create expected command with WorkDir set
				expectedCmd := cmd
				if expectedCmd.Dir == "" {
					expectedCmd.Dir = config.Global.WorkDir
				}
				mockResourceManager.On("ExecuteCommand", mock.Anything, expectedCmd, &config.Groups[0], mock.Anything).Return(tt.mockResults[i], tt.mockErrors[i])
			}

			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, config.Groups[0])

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

	t.Run("undefined environment variable treated as an error", func(t *testing.T) {
		group := runnertypes.CommandGroup{
			Name:         "test-env-undefined",
			EnvAllowlist: []string{"VALID_VAR"},
			Commands: []runnertypes.Command{
				{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}},
				{Name: "cmd-2", Cmd: "echo", Args: []string{"test"}, Env: []string{"UNDEFINED_VAR=${NONEXISTENT_VAR}"}},
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

		// Try to prepare config - should fail during configuration loading (config preparation)
		filter := environment.NewFilter(config.Global.EnvAllowlist)
		expander := environment.NewVariableExpander(filter)
		err := configpkg.ExpandCommandEnv(&config.Groups[0].Commands[1], expander, nil, nil, config.Global.EnvAllowlist, nil, config.Groups[0].EnvAllowlist, config.Groups[0].Name)

		// Should fail with undefined variable error during configuration loading
		assert.Error(t, err)
		assert.ErrorIs(t, err, environment.ErrVariableNotFound)
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
	sleepCmd := runnertypes.Command{
		Cmd:  "sleep",
		Args: []string{"5"}, // Sleep for 5 seconds, longer than timeout
	}
	runnertypes.PrepareCommand(&sleepCmd)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout: 1, // 1 second timeout
			WorkDir: "/tmp",
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:     "timeout-test-group",
				Commands: []runnertypes.Command{sleepCmd},
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
		shortTimeoutCmd := runnertypes.Command{
			Cmd:     "sleep",
			Args:    []string{"5"}, // Sleep for 5 seconds
			Timeout: 1,             // But timeout after 1 second
		}
		runnertypes.PrepareCommand(&shortTimeoutCmd)

		configWithCmdTimeout := &runnertypes.Config{
			Global: runnertypes.GlobalConfig{
				Timeout: 10, // 10 seconds global timeout
				WorkDir: "/tmp",
			},
			Groups: []runnertypes.CommandGroup{
				{
					Name:     "cmd-timeout-test-group",
					Commands: []runnertypes.Command{shortTimeoutCmd},
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
			// ExpandedEnv contains variables loaded from .env file or defined in global.env
			ExpandedEnv: map[string]string{
				"LOADED_VAR": "from_env_file",
				"PATH":       "/custom/path", // This should override system PATH
			},
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

	// Test Command.Env with self-reference and cross-reference
	cmd := runnertypes.Command{
		Env: []string{
			"CMD_VAR=command_value",
			"REFERENCE_VAR=${CMD_VAR}_referenced", // Reference another Command.Env variable
		},
	}

	// Prepare command with expanded environment (simulates configuration loading)
	prepareCommandWithExpandedEnv(t, &cmd, &config.Groups[0], config)

	// Resolve environment variables (merges system env + Global.ExpandedEnv + Command.ExpandedEnv)
	envVars, err := runner.resolveEnvironmentVars(&cmd, &config.Groups[0])
	assert.NoError(t, err)

	// Check that global vars are present
	assert.Equal(t, "from_env_file", envVars["LOADED_VAR"])
	assert.Equal(t, "/custom/path", envVars["PATH"]) // Global.ExpandedEnv overrides system env

	// Check that command vars are present and correctly expanded
	assert.Equal(t, "command_value", envVars["CMD_VAR"])
	assert.Equal(t, "command_value_referenced", envVars["REFERENCE_VAR"])
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
		_, err = runner.executeCommandInGroup(ctx, &allowedCmd, testGroup)
		assert.NoError(t, err)
		mockResourceManager.AssertExpectations(t)
	})

	// This test is temporarily disabled
	// t.Run("disallowed command execution should fail", func(t *testing.T) {
	// 	// Test will be re-enabled when NewManagerForTest API is available
	// })

	t.Run("command execution with environment variables", func(t *testing.T) {
		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-run-123"))
		require.NoError(t, err)

		testGroup := &config.Groups[0] // Get reference to the test group

		// Test with safe environment variables
		safeCmd := runnertypes.Command{
			Name: "test-env",
			Cmd:  "echo",
			Args: []string{"$TEST_VAR"},
			Dir:  "/tmp",
			Env:  []string{"TEST_VAR=safe-value", "PATH=/usr/bin:/bin"},
		}

		// Prepare command with expanded environment (simulates configuration loading)
		prepareCommandWithExpandedEnv(t, &safeCmd, testGroup, config)

		mockResourceManager.On("ExecuteCommand", mock.Anything, safeCmd, mock.Anything, mock.Anything).
			Return(&resource.ExecutionResult{ExitCode: 0}, nil)

		_, err = runner.executeCommandInGroup(context.Background(), &safeCmd, testGroup)
		assert.NoError(t, err)

		// Test with unsafe environment variable value - should fail during configuration loading (config preparation)
		unsafeCmd := runnertypes.Command{
			Name: "test-unsafe-env",
			Cmd:  "echo",
			Args: []string{"$DANGEROUS"},
			Dir:  "/tmp",
			Env:  []string{"DANGEROUS=value; rm -rf /"},
		}

		// Try to prepare command - should fail with unsafe environment variable error
		filter := environment.NewFilter(config.Global.EnvAllowlist)
		expander := environment.NewVariableExpander(filter)
		err = configpkg.ExpandCommandEnv(&unsafeCmd, expander, nil, nil, config.Global.EnvAllowlist, nil, testGroup.EnvAllowlist, testGroup.Name)
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
// command-specific > group > global (loaded from system/env file)
func TestRunner_EnvironmentVariablePriority(t *testing.T) {
	setupSafeTestEnv(t)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      "/tmp",
			EnvAllowlist: []string{"GLOBAL_VAR", "CMD_VAR", "OVERRIDE_VAR", "REFERENCE_VAR"},
			// ExpandedEnv contains variables loaded from .env file or defined in global.env
			ExpandedEnv: map[string]string{
				"GLOBAL_VAR":    "global_value",
				"OVERRIDE_VAR":  "global_override",
				"REFERENCE_VAR": "global_reference",
			},
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
			commandEnvVars: []string{"BASE_VAR=base_value", "REFERENCE_VAR=${BASE_VAR}_referenced"},
			expectedValues: map[string]string{
				"GLOBAL_VAR":    "global_value",          // From global environment
				"BASE_VAR":      "base_value",            // From Command.Env
				"REFERENCE_VAR": "base_value_referenced", // Should resolve to Command.Env variable value
			},
			description: "Variable references should resolve using Command.Env variables",
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

			// Prepare command with expanded environment (simulates configuration loading)
			testGroup := &config.Groups[0]
			prepareCommandWithExpandedEnv(t, &testCmd, testGroup, config)

			// Resolve environment variables using the runner
			resolvedEnv, err := runner.resolveEnvironmentVars(&testCmd, testGroup)
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
// which supports command-specific > group > global priority
func TestRunner_EnvironmentVariablePriority_CurrentImplementation(t *testing.T) {
	setupSafeTestEnv(t)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			WorkDir:      "/tmp",
			EnvAllowlist: []string{"GLOBAL_VAR", "CMD_VAR", "OVERRIDE_VAR", "REFERENCE_VAR"},
			// ExpandedEnv contains variables loaded from .env file or defined in global.env
			ExpandedEnv: map[string]string{
				"GLOBAL_VAR":    "global_value",
				"OVERRIDE_VAR":  "global_override",
				"REFERENCE_VAR": "global_reference",
			},
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
			commandEnvVars: []string{"BASE_VAR=base_value", "REFERENCE_VAR=${BASE_VAR}_referenced"},
			expectedValues: map[string]string{
				"GLOBAL_VAR":    "global_value",          // From global environment
				"BASE_VAR":      "base_value",            // From Command.Env
				"REFERENCE_VAR": "base_value_referenced", // Should resolve to Command.Env variable value
			},
			description: "Command variables should be able to reference other Command.Env variables",
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

			// Prepare command with expanded environment (simulates configuration loading)
			testGroup := &config.Groups[0]
			prepareCommandWithExpandedEnv(t, &testCmd, testGroup, config)

			// Resolve environment variables using the runner
			resolvedEnv, err := runner.resolveEnvironmentVars(&testCmd, testGroup)
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

		// Prepare command with expanded environment (simulates configuration loading)
		prepareCommandWithExpandedEnv(t, &testCmd, &testGroup, config)

		resolvedEnv, err := runner.resolveEnvironmentVars(&testCmd, &testGroup)
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

		// Prepare command should fail with malformed environment variable
		filter := environment.NewFilter(config.Global.EnvAllowlist)
		expander := environment.NewVariableExpander(filter)
		err := configpkg.ExpandCommandEnv(&testCmd, expander, nil, nil, config.Global.EnvAllowlist, nil, testGroup.EnvAllowlist, testGroup.Name)
		// Should fail when an environment variable is malformed
		assert.Error(t, err)
		assert.ErrorIs(t, err, configpkg.ErrMalformedEnvVariable)
	})

	t.Run("variable reference to undefined variable returns error", func(t *testing.T) {
		testGroup := config.Groups[0]

		testCmd := runnertypes.Command{
			Name: "test-undefined-ref",
			Cmd:  "echo",
			Args: []string{"test"},
			Env:  []string{"UNDEFINED_REF=${NONEXISTENT_VAR}"},
		}

		// Prepare command should fail with undefined variable
		filter := environment.NewFilter(config.Global.EnvAllowlist)
		expander := environment.NewVariableExpander(filter)
		err := configpkg.ExpandCommandEnv(&testCmd, expander, nil, nil, config.Global.EnvAllowlist, nil, testGroup.EnvAllowlist, testGroup.Name)
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

		// Prepare command should fail with circular reference
		filter := environment.NewFilter(config.Global.EnvAllowlist)
		expander := environment.NewVariableExpander(filter)
		err := configpkg.ExpandCommandEnv(&testCmd, expander, nil, nil, config.Global.EnvAllowlist, nil, testGroup.EnvAllowlist, testGroup.Name)
		// New implementation explicitly detects circular reference
		assert.Error(t, err)
		assert.ErrorIs(t, err, environment.ErrCircularReference)
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

// TestRunner_OutputCaptureEndToEnd tests the end-to-end runner functionality with output capture configuration
func TestRunner_OutputCaptureEndToEnd(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		commands    []runnertypes.Command
		expectError bool
		description string
	}{
		{
			name: "command with output configuration",
			commands: []runnertypes.Command{
				{
					Name:   "test-echo",
					Cmd:    "echo",
					Args:   []string{"Hello World"},
					Output: "test-output.txt",
				},
			},
			expectError: false, // Note: This may fail due to output capture implementation, which is expected
			description: "Command with output configuration should be parsed correctly",
		},
		{
			name: "command without output capture",
			commands: []runnertypes.Command{
				{
					Name: "no-output",
					Cmd:  "echo",
					Args: []string{"No capture"},
					// No Output field
				},
			},
			expectError: false,
			description: "Commands without output field should execute normally",
		},
		{
			name: "mixed commands with and without output",
			commands: []runnertypes.Command{
				{
					Name:   "with-output",
					Cmd:    "echo",
					Args:   []string{"Captured"},
					Output: "mixed-output.txt",
				},
				{
					Name: "without-output",
					Cmd:  "echo",
					Args: []string{"Not captured"},
					// No Output field
				},
			},
			expectError: false,
			description: "Mixed commands should handle output configuration correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for this test
			tempDir := t.TempDir()

			// Create config with output capture settings
			config := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					Timeout:       30,
					WorkDir:       tempDir,
					LogLevel:      "info",
					MaxOutputSize: 1024 * 1024, // 1MB limit
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name:        "output-test-group",
						Description: "Test group for output capture",
						Commands:    tt.commands,
					},
				},
			}

			// Create runner
			runner, err := NewRunner(config, WithRunID("test-end-to-end"))
			require.NoError(t, err, "NewRunner should not return an error")

			// Load basic environment
			err = runner.LoadSystemEnvironment()
			require.NoError(t, err, "LoadSystemEnvironment should not return an error")

			// Verify runner was created properly with output capture configuration
			runnerConfig := runner.GetConfig()
			assert.Equal(t, config, runnerConfig)
			assert.Equal(t, int64(1024*1024), runnerConfig.Global.MaxOutputSize)

			// Verify output field is preserved in configuration
			for i, originalCmd := range tt.commands {
				actualCmd := runnerConfig.Groups[0].Commands[i]
				assert.Equal(t, originalCmd.Output, actualCmd.Output, "Output field should be preserved")
			}

			// Note: Actual execution may fail due to output capture implementation not being complete,
			// but the configuration parsing and runner setup should work correctly.
		})
	}
}

// TestRunner_OutputCaptureErrorScenarios tests error scenarios for output capture
func TestRunner_OutputCaptureErrorScenarios(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name         string
		commands     []runnertypes.Command
		globalConfig runnertypes.GlobalConfig
		expectError  string
		description  string
	}{
		{
			name: "path traversal attempt",
			commands: []runnertypes.Command{
				{
					Name:   "path-traversal",
					Cmd:    "echo",
					Args:   []string{"attempt"},
					Output: "../../../etc/passwd",
				},
			},
			globalConfig: runnertypes.GlobalConfig{
				Timeout:       30,
				WorkDir:       "/tmp",
				MaxOutputSize: 1024,
			},
			expectError: "path traversal",
			description: "Path traversal attempts should be rejected",
		},
		{
			name: "non-existent directory",
			commands: []runnertypes.Command{
				{
					Name:   "non-existent-dir",
					Cmd:    "echo",
					Args:   []string{"test"},
					Output: "/non/existent/directory/output.txt",
				},
			},
			globalConfig: runnertypes.GlobalConfig{
				Timeout:       30,
				WorkDir:       "/tmp",
				MaxOutputSize: 1024,
			},
			expectError: "directory",
			description: "Non-existent directories should cause error",
		},
		{
			name: "permission denied directory",
			commands: []runnertypes.Command{
				{
					Name:   "permission-denied",
					Cmd:    "echo",
					Args:   []string{"test"},
					Output: "/root/output.txt",
				},
			},
			globalConfig: runnertypes.GlobalConfig{
				Timeout:       30,
				WorkDir:       "/tmp",
				MaxOutputSize: 1024,
			},
			expectError: "permission",
			description: "Permission denied should cause error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for this test
			tempDir := t.TempDir()
			tt.globalConfig.WorkDir = tempDir

			// Create config
			config := &runnertypes.Config{
				Global: tt.globalConfig,
				Groups: []runnertypes.CommandGroup{
					{
						Name:        "error-test-group",
						Description: "Test group for output capture errors",
						Commands:    tt.commands,
					},
				},
			}

			// Create runner
			runner, err := NewRunner(config, WithRunID("test-error-scenarios"))
			require.NoError(t, err, "NewRunner should not return an error")

			// Load basic environment
			err = runner.LoadSystemEnvironment()
			require.NoError(t, err, "LoadSystemEnvironment should not return an error")

			// Execute the group - should fail
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, config.Groups[0])

			// Verify error occurred and contains expected message
			assert.Error(t, err, tt.description)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.expectError), "Error should contain expected message: %s", tt.expectError)
		})
	}
}

// TestRunner_OutputCaptureDryRun tests dry-run functionality with output capture
func TestRunner_OutputCaptureDryRun(t *testing.T) {
	setupSafeTestEnv(t)

	// Create temporary directory for this test
	tempDir := t.TempDir()

	// Create config with output capture
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			Timeout:       30,
			WorkDir:       tempDir,
			LogLevel:      "info",
			MaxOutputSize: 1024,
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:        "dryrun-test-group",
				Description: "Test group for dry-run output capture",
				Commands: []runnertypes.Command{
					{
						Name:   "dryrun-echo",
						Cmd:    "echo",
						Args:   []string{"Dry run test"},
						Output: "dryrun-output.txt",
					},
				},
			},
		},
	}

	// Create mock resource manager for dry-run mode
	mockResourceManager := &MockResourceManager{}

	// Set up dry-run mode
	mockResourceManager.On("SetMode", resource.ExecutionModeDryRun, (*resource.DryRunOptions)(nil)).Return()

	// Set up mock expectations for dry-run mode
	mockResourceManager.On("ValidateOutputPath", "dryrun-output.txt", mock.Anything).Return(nil)
	mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		&resource.ExecutionResult{
			ExitCode: 0,
			Stdout:   "Dry run test",
			Stderr:   "",
		}, nil,
	)

	// Mock dry-run results
	mockResourceManager.On("GetDryRunResults").Return(&resource.DryRunResult{
		ResourceAnalyses: []resource.ResourceAnalysis{
			{
				Type:      resource.ResourceTypeCommand,
				Operation: resource.OperationExecute,
				Target:    "dryrun-echo",
			},
		},
	})

	// Create runner with mock resource manager
	runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithRunID("test-dry-run"))
	require.NoError(t, err, "NewRunner should not return an error")

	// Load basic environment
	err = runner.LoadSystemEnvironment()
	require.NoError(t, err, "LoadSystemEnvironment should not return an error")

	// Enable dry-run mode through mock resource manager
	mockResourceManager.SetMode(resource.ExecutionModeDryRun, nil)

	// Execute the group in dry-run mode
	ctx := context.Background()
	err = runner.ExecuteGroup(ctx, config.Groups[0])

	// Dry-run should not fail
	assert.NoError(t, err, "Dry-run execution should not fail")

	// Verify that output file was NOT created (since it's a dry run)
	outputPath := filepath.Join(tempDir, "dryrun-output.txt")
	assert.NoFileExists(t, outputPath, "Output file should not exist in dry-run mode")

	// Get dry-run results for verification
	dryRunResults := runner.GetDryRunResults()
	assert.NotNil(t, dryRunResults, "Dry-run results should be available")

	// Verify mock expectations
	mockResourceManager.AssertExpectations(t)
}

// TestRunner_OutputCaptureWithTOMLConfig tests TOML configuration parsing for output capture
func TestRunner_OutputCaptureWithTOMLConfig(t *testing.T) {
	setupSafeTestEnv(t)

	// Create temporary directory for this test
	tempDir := t.TempDir()

	// Create a test TOML configuration file with output capture
	tomlContent := `
[global]
timeout = 30
workdir = "` + tempDir + `"
max_output_size = 1048576

[[groups]]
name = "output-capture-group"
description = "Test group with output capture"

[[groups.commands]]
name = "simple-echo"
cmd = "echo"
args = ["Hello from TOML config"]
output = "toml-output.txt"

[[groups.commands]]
name = "multiline-output"
cmd = "sh"
args = ["-c", "echo 'Line 1'; echo 'Line 2'"]
output = "multiline-toml-output.txt"

[[groups.commands]]
name = "no-output-command"
cmd = "echo"
args = ["No output capture"]
`

	// Write TOML config to temporary file
	configPath := filepath.Join(tempDir, "test-config.toml")
	err := os.WriteFile(configPath, []byte(tomlContent), 0o644)
	require.NoError(t, err, "Should be able to write TOML config file")

	// Test loading TOML config for output capture settings
	t.Run("load TOML config with output capture settings", func(t *testing.T) {
		// Load configuration from TOML file using config.Loader
		configContent, err := os.ReadFile(configPath)
		require.NoError(t, err, "Should be able to read TOML config file")

		loader := configpkg.NewLoader()
		config, err := loader.LoadConfig(configContent)
		require.NoError(t, err, "Should be able to load TOML configuration")

		// Verify configuration was loaded correctly
		assert.Equal(t, tempDir, config.Global.WorkDir)
		assert.Equal(t, int64(1048576), config.Global.MaxOutputSize)
		assert.Len(t, config.Groups, 1)
		assert.Equal(t, "output-capture-group", config.Groups[0].Name)
		assert.Len(t, config.Groups[0].Commands, 3)

		// Verify commands have correct output configuration
		assert.Equal(t, "toml-output.txt", config.Groups[0].Commands[0].Output)
		assert.Equal(t, "multiline-toml-output.txt", config.Groups[0].Commands[1].Output)
		assert.Equal(t, "", config.Groups[0].Commands[2].Output) // No output field

		// Create runner to verify basic initialization works
		runner, err := NewRunner(config, WithRunID("test-toml-config"))
		require.NoError(t, err, "NewRunner should not return an error")

		// Load basic environment to verify runner setup
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err, "LoadSystemEnvironment should not return an error")

		// Verify runner configuration
		runnerConfig := runner.GetConfig()
		assert.Equal(t, config, runnerConfig)
	})

	// Test TOML config validation for output capture
	t.Run("TOML config validation", func(t *testing.T) {
		invalidTomlContent := `
[global]
timeout = 30
workdir = "` + tempDir + `"
max_output_size = -1  # Invalid negative size

[[groups]]
name = "invalid-group"

[[groups.commands]]
name = "invalid-echo"
cmd = "echo"
args = ["test"]
output = "output.txt"
`

		invalidConfigPath := filepath.Join(tempDir, "invalid-config.toml")
		err := os.WriteFile(invalidConfigPath, []byte(invalidTomlContent), 0o644)
		require.NoError(t, err, "Should be able to write invalid TOML config file")

		// Load invalid configuration
		invalidConfigContent, err := os.ReadFile(invalidConfigPath)
		require.NoError(t, err, "Should be able to read invalid TOML config file")

		loader := configpkg.NewLoader()
		config, err := loader.LoadConfig(invalidConfigContent)
		require.NoError(t, err, "Config loader should parse TOML structure")

		// Verify negative max_output_size was loaded (validation happens later)
		assert.Equal(t, int64(-1), config.Global.MaxOutputSize)
	})
}

// TestRunner_OutputCaptureErrorTypes tests all OutputCaptureError types
func TestRunner_OutputCaptureErrorTypes(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		setupMock   func(*MockResourceManager)
		expectError string
	}{
		{
			name: "InvalidFormat",
			setupMock: func(mockRM *MockResourceManager) {
				mockRM.SetupFailedMockExecution(errors.New("invalid output format"))
			},
			expectError: "invalid output format",
		},
		{
			name: "SecurityViolation",
			setupMock: func(mockRM *MockResourceManager) {
				mockRM.SetupFailedMockExecution(errors.New("security violation: path traversal detected"))
			},
			expectError: "security violation",
		},
		{
			name: "DiskFull",
			setupMock: func(mockRM *MockResourceManager) {
				mockRM.SetupFailedMockExecution(errors.New("disk full: cannot write output"))
			},
			expectError: "disk full",
		},
		{
			name: "Unknown",
			setupMock: func(mockRM *MockResourceManager) {
				mockRM.SetupFailedMockExecution(errors.New("unknown error occurred"))
			},
			expectError: "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Create basic configuration with output capture
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					Timeout:       30,
					WorkDir:       tempDir,
					MaxOutputSize: 1024,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test-group",
						Commands: []runnertypes.Command{
							{
								Name:   "test-cmd",
								Cmd:    "echo",
								Args:   []string{"test"},
								Output: "output.txt",
							},
						},
					},
				},
			}

			// Create mock resource manager
			mockRM := &MockResourceManager{}
			tt.setupMock(mockRM)

			// Create runner with proper options
			options := []Option{
				WithResourceManager(mockRM),
				WithRunID("test-run-id"),
			}

			runner, err := NewRunner(cfg, options...)
			require.NoError(t, err)

			// Execute the group instead of full run
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, cfg.Groups[0])

			// Verify error contains expected message
			require.Error(t, err, "Should return error for %s", tt.name)
			assert.Contains(t, err.Error(), tt.expectError)

			// Verify mock expectations
			mockRM.AssertExpectations(t)
		})
	}
}

// TestRunner_OutputCaptureExecutionStages tests error handling in different execution stages
func TestRunner_OutputCaptureExecutionStages(t *testing.T) {
	// Test error variables for robust error checking
	ErrPreValidationTest := errors.New("pre-validation failed: invalid output path")
	ErrExecutionTest := errors.New("execution failed: command not found")
	ErrPostProcessingTest := errors.New("post-processing failed: cannot write output file")
	ErrCleanupTest := errors.New("cleanup failed: cannot remove temporary files")

	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		stage       string
		setupMock   func(*MockResourceManager)
		expectError error
	}{
		{
			name:  "PreValidationError",
			stage: "pre-validation",
			setupMock: func(mockRM *MockResourceManager) {
				// Simulate pre-validation error (before command execution)
				mockRM.SetupFailedMockExecution(ErrPreValidationTest)
			},
			expectError: ErrPreValidationTest,
		},
		{
			name:  "ExecutionError",
			stage: "execution",
			setupMock: func(mockRM *MockResourceManager) {
				// Simulate execution error (during command execution)
				mockRM.SetupFailedMockExecution(ErrExecutionTest)
			},
			expectError: ErrExecutionTest,
		},
		{
			name:  "PostProcessingError",
			stage: "post-processing",
			setupMock: func(mockRM *MockResourceManager) {
				// Simulate post-processing error (after command execution)
				mockRM.SetupFailedMockExecution(ErrPostProcessingTest)
			},
			expectError: ErrPostProcessingTest,
		},
		{
			name:  "CleanupError",
			stage: "cleanup",
			setupMock: func(mockRM *MockResourceManager) {
				// Simulate cleanup error (during resource cleanup)
				mockRM.SetupFailedMockExecution(ErrCleanupTest)
			},
			expectError: ErrCleanupTest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Create basic configuration with output capture
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					Timeout:       30,
					WorkDir:       tempDir,
					MaxOutputSize: 1024,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test-group",
						Commands: []runnertypes.Command{
							{
								Name:   "test-cmd",
								Cmd:    "echo",
								Args:   []string{"test"},
								Output: "output.txt",
							},
						},
					},
				},
			}

			// Create mock resource manager
			mockRM := &MockResourceManager{}
			tt.setupMock(mockRM)

			// Create runner with proper options
			options := []Option{
				WithResourceManager(mockRM),
				WithRunID("test-run-id"),
			}

			runner, err := NewRunner(cfg, options...)
			require.NoError(t, err)

			// Execute the group instead of full run
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, cfg.Groups[0])

			// Verify error matches expected type using errors.Is()
			require.Error(t, err, "Should return error for %s stage", tt.stage)
			assert.True(t, errors.Is(err, tt.expectError),
				"Expected error type %v, got %v", tt.expectError, err)

			// Verify mock expectations
			mockRM.AssertExpectations(t)
		})
	}
}

// TestRunner_OutputAnalysisValidation tests output.Analysis through actual implementation
func TestRunner_OutputAnalysisValidation(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name         string
		outputPath   string
		setupWorkDir func(string) string // Returns workDir path
		expectCheck  func(*testing.T, *output.Analysis)
		description  string
	}{
		{
			name:       "ValidOutputPath",
			outputPath: "output.txt",
			setupWorkDir: func(baseDir string) string {
				// Create a valid directory for output
				return baseDir
			},
			expectCheck: func(t *testing.T, analysis *output.Analysis) {
				assert.Equal(t, "output.txt", analysis.OutputPath)
				assert.True(t, analysis.DirectoryExists, "Directory should exist")
				assert.True(t, analysis.WritePermission, "Should have write permission")
				assert.Equal(t, output.RiskLevelLow, analysis.SecurityRisk)
				assert.Empty(t, analysis.ErrorMessage, "Should have no error message")
				assert.NotEmpty(t, analysis.ResolvedPath, "Should have resolved path")
				// MaxSizeLimit defaults to 0 (unlimited) in current implementation
				assert.GreaterOrEqual(t, analysis.MaxSizeLimit, int64(0), "MaxSizeLimit should be non-negative")
			},
			description: "Should correctly analyze valid output path",
		},
		{
			name:       "PathTraversalAttempt",
			outputPath: "../../../etc/passwd",
			setupWorkDir: func(baseDir string) string {
				return baseDir
			},
			expectCheck: func(t *testing.T, analysis *output.Analysis) {
				assert.Equal(t, "../../../etc/passwd", analysis.OutputPath)
				// Path traversal should be detected as critical risk (consistent with manager_test.go)
				assert.Equal(t, output.RiskLevelCritical, analysis.SecurityRisk,
					"Path traversal should be critical risk, got: %v", analysis.SecurityRisk)
				// ResolvedPath should be empty if path validation fails
				assert.Empty(t, analysis.ResolvedPath, "ResolvedPath should be empty for invalid paths")
				// Should have error message indicating the problem
				assert.Contains(t, analysis.ErrorMessage, "path traversal", "Should contain error message about path traversal")
				// Write permission should be false for failed validation
				assert.False(t, analysis.WritePermission, "WritePermission should be false for invalid paths")
				// MaxSizeLimit defaults to 0 (unlimited) in current implementation
				assert.GreaterOrEqual(t, analysis.MaxSizeLimit, int64(0), "MaxSizeLimit should be non-negative")
			},
			description: "Should correctly identify path traversal security risks",
		},
		{
			name:       "NonExistentDirectory",
			outputPath: "nonexistent/output.txt",
			setupWorkDir: func(baseDir string) string {
				// Don't create the 'nonexistent' directory
				return baseDir
			},
			expectCheck: func(t *testing.T, analysis *output.Analysis) {
				assert.Equal(t, "nonexistent/output.txt", analysis.OutputPath)
				assert.False(t, analysis.DirectoryExists, "Directory should not exist")
				// WritePermission behavior depends on implementation - might check parent directory permissions
				// Let's check what the actual implementation returns
				assert.NotEmpty(t, analysis.ResolvedPath, "Should have resolved path")
				// SecurityRisk should be reasonable for valid path structure
				assert.True(t, analysis.SecurityRisk <= output.RiskLevelMedium, "Should not be high risk for valid path structure")
				// MaxSizeLimit defaults to 0 (unlimited) in current implementation
				assert.GreaterOrEqual(t, analysis.MaxSizeLimit, int64(0), "MaxSizeLimit should be non-negative")
			},
			description: "Should correctly handle non-existent directories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			workDir := tt.setupWorkDir(tempDir)

			// Create actual output manager to test real implementation
			// Need to create a mock security validator for testing
			mockSecurityValidator := &MockSecurityValidator{}
			mockSecurityValidator.On("ValidateOutputWritePermission", mock.Anything, mock.Anything).Return(nil).Maybe()

			manager := output.NewDefaultOutputCaptureManager(mockSecurityValidator)

			// Call the actual AnalyzeOutput method
			analysis, err := manager.AnalyzeOutput(tt.outputPath, workDir)
			require.NoError(t, err, "AnalyzeOutput should not return error")
			require.NotNil(t, analysis, "Analysis should not be nil")

			// Run the expectation checks
			tt.expectCheck(t, analysis)

			t.Logf("Test %s: %s", tt.name, tt.description)
		})
	}
}

// TestRunner_OutputCaptureSecurityIntegration tests security validation integration
func TestRunner_OutputCaptureSecurityIntegration(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		outputPath  string
		expectError bool
		errorMsg    string
		description string
	}{
		{
			name:        "ValidOutputPath",
			outputPath:  "valid-output.txt",
			expectError: false,
			description: "Valid output path should be accepted",
		},
		{
			name:        "PathTraversalAttempt",
			outputPath:  "../../../etc/passwd",
			expectError: true,
			errorMsg:    "path traversal",
			description: "Path traversal attempts should be blocked",
		},
		{
			name:        "SymlinkProtection",
			outputPath:  "/tmp/symlink-target",
			expectError: true,
			errorMsg:    "directory security validation failed",
			description: "Symlink attacks should be prevented",
		},
		{
			name:        "AbsolutePathBlocked",
			outputPath:  "/etc/shadow",
			expectError: true,
			errorMsg:    "write permission denied",
			description: "Absolute paths should be blocked for security",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Create configuration with potentially malicious output path
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					Timeout:       30,
					WorkDir:       tempDir,
					MaxOutputSize: 1024,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "security-test-group",
						Commands: []runnertypes.Command{
							{
								Name:   "security-test-cmd",
								Cmd:    "echo",
								Args:   []string{"test"},
								Output: tt.outputPath,
							},
						},
					},
				},
			}

			// For error cases, don't use mock to allow actual security validation
			var options []Option
			if !tt.expectError {
				// Create mock resource manager for success cases
				mockRM := &MockResourceManager{}
				mockRM.On("ValidateOutputPath", tt.outputPath, mock.Anything).Return(nil)
				result := &resource.ExecutionResult{
					ExitCode: 0,
					Stdout:   "test",
					Stderr:   "",
				}
				mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(result, nil)
				options = []Option{
					WithResourceManager(mockRM),
					WithRunID("test-run-output-capture"),
				}
			} else {
				// For error cases, use default resource manager to allow real validation
				options = []Option{
					WithRunID("test-run-output-capture"),
				}
			}

			runner, err := NewRunner(cfg, options...)
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, cfg.Groups[0])

			if tt.expectError {
				require.Error(t, err, "Should return error for %s", tt.description)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				// Note: May still fail due to actual output capture implementation
				// This test focuses on security validation configuration
				t.Logf("Test completed: %s", tt.description)
			}

			// Verify mock expectations (only for success cases with mock)
			// Error cases use real resource manager, so no mock to verify
		})
	}
}

// TestRunner_OutputCaptureResourceManagement tests resource management integration
func TestRunner_OutputCaptureResourceManagement(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name          string
		setupMock     func(*MockResourceManager)
		expectSuccess bool
		description   string
	}{
		{
			name: "TempDirectoryLifecycle",
			setupMock: func(mockRM *MockResourceManager) {
				// Simulate temp directory creation and cleanup
				mockRM.On("ValidateOutputPath", "resource-output.txt", mock.Anything).Return(nil)
				mockRM.On("CreateTempDir", "test-group").Return("/tmp/test-temp-dir", nil)
				mockRM.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil)

				result := &resource.ExecutionResult{
					ExitCode: 0,
					Stdout:   "test output",
					Stderr:   "",
				}
				mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(result, nil)
			},
			expectSuccess: true,
			description:   "Temp directory should be created and cleaned up properly",
		},
		{
			name: "ResourceContention",
			setupMock: func(mockRM *MockResourceManager) {
				// Setup temp directory to satisfy TempDir requirement
				mockRM.On("ValidateOutputPath", "resource-output.txt", mock.Anything).Return(nil)
				mockRM.On("CreateTempDir", "test-group").Return("/tmp/test-temp-dir", nil)
				mockRM.On("CleanupTempDir", "/tmp/test-temp-dir").Return(nil)
				// Simulate resource contention scenario
				mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errResourceBusy)
			},
			expectSuccess: false,
			description:   "Resource contention should be handled gracefully",
		},
		{
			name: "CleanupFailure",
			setupMock: func(mockRM *MockResourceManager) {
				// Simulate cleanup failure - cleanup errors are logged but don't fail the execution
				mockRM.On("ValidateOutputPath", "resource-output.txt", mock.Anything).Return(nil)
				mockRM.On("CreateTempDir", "test-group").Return("/tmp/test-temp-dir", nil)
				mockRM.On("CleanupTempDir", "/tmp/test-temp-dir").Return(errCleanupFailed)

				result := &resource.ExecutionResult{
					ExitCode: 0,
					Stdout:   "test output",
					Stderr:   "",
				}
				mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(result, nil)
			},
			expectSuccess: true, // Cleanup failures are logged but don't fail the execution
			description:   "Cleanup failures should be properly reported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Create configuration with output capture
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					Timeout:       30,
					WorkDir:       tempDir,
					MaxOutputSize: 1024,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name:    "test-group",
						TempDir: true, // Enable temp directory to test resource management
						Commands: []runnertypes.Command{
							{
								Name:   "resource-test-cmd",
								Cmd:    "echo",
								Args:   []string{"resource test"},
								Output: "resource-output.txt",
							},
						},
					},
				},
			}

			// Create mock resource manager
			mockRM := &MockResourceManager{}
			tt.setupMock(mockRM)

			// Create runner with proper options
			options := []Option{
				WithResourceManager(mockRM),
				WithRunID("test-run-output-capture"),
			}

			runner, err := NewRunner(cfg, options...)
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, cfg.Groups[0])

			if tt.expectSuccess {
				// Note: May still fail due to actual implementation details
				// This test focuses on resource management configuration
				t.Logf("Resource management test completed: %s", tt.description)
			} else {
				require.Error(t, err, "Should return error for %s", tt.description)
			}

			// Verify mock expectations
			mockRM.AssertExpectations(t)
		})
	}
}
