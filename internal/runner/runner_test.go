package runner

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"

	configpkg "github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var errCommandNotFound = errors.New("command not found")

var ErrExecutionFailed = errors.New("execution failed")

// MockSecurityValidator for output testing
type MockSecurityValidator struct {
	mock.Mock
}

func (m *MockSecurityValidator) ValidateOutputWritePermission(outputPath string, realUID int) error {
	args := m.Called(outputPath, realUID)
	return args.Error(0)
}

// setupDryRunVerification creates a verification manager for dry-run testing
func setupDryRunVerification(t *testing.T) *verification.Manager {
	t.Helper()
	vm, err := verification.NewManagerForDryRun()
	require.NoError(t, err)
	return vm
}

func TestNewRunner(t *testing.T) {
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: common.Int32Ptr(3600),
		},
	}

	t.Run("default configuration", func(t *testing.T) {
		runner, err := NewRunner(config,
			WithVerificationManager(setupDryRunVerification(t)),
			WithRunID("test-run-123"))
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

	t.Run("with multiple options", func(t *testing.T) {
		runner, err := NewRunner(config,
			WithVerificationManager(setupDryRunVerification(t)),
			WithRunID("test-run-123"))
		assert.NoError(t, err)
		assert.NotNil(t, runner)
	})

	t.Run("with custom group membership provider", func(t *testing.T) {
		// Create a custom group membership provider
		customProvider := groupmembership.New()
		runner, err := NewRunner(config,
			WithVerificationManager(setupDryRunVerification(t)),
			WithRunID("test-run-123"),
			WithGroupMembershipProvider(customProvider))
		require.NoError(t, err, "NewRunner should not return an error with custom group membership provider")
		assert.NotNil(t, runner)
		assert.NotNil(t, runner.validator)
	})
}

func TestRunner_ExecuteGroup(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		group       runnertypes.GroupSpec
		mockResults []*resource.ExecutionResult
		mockErrors  []error
		expectedErr error
	}{
		{
			name: "successful execution",
			group: runnertypes.GroupSpec{
				Name:        "test-group",
				Description: "Test group",
				Commands: []runnertypes.CommandSpec{
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
			group: runnertypes.GroupSpec{
				Name: "test-group",
				Commands: []runnertypes.CommandSpec{
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
			group: runnertypes.GroupSpec{
				Name: "test-group",
				Commands: []runnertypes.CommandSpec{
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
			config := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout: common.Int32Ptr(3600),
				},
				Groups: []runnertypes.GroupSpec{tt.group},
			}

			mockResourceManager := new(MockResourceManager)
			runner, err := NewRunner(config,
				WithResourceManager(mockResourceManager),
				WithVerificationManager(setupDryRunVerification(t)),
				WithRunID("test-run-123"))
			require.NoError(t, err, "NewRunner should not return an error with valid config")

			// Setup mock expectations
			for i := range config.Groups[0].Commands {
				// Create RuntimeCommand with EffectiveWorkDir and EffectiveTimeout set
				// Note: Global.WorkDir has been removed in Task 0034
				// Note: We use mock.Anything for RuntimeCommand because it contains __runner_workdir
				// which is set dynamically at runtime
				mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[0], mock.Anything).Return(resource.CommandToken(""), tt.mockResults[i], tt.mockErrors[i])
			}

			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &config.Groups[0])

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
		group := runnertypes.GroupSpec{
			Name: "test-first-fails",
			Commands: []runnertypes.CommandSpec{
				{Name: "cmd-1", Cmd: "false"}, // This fails
				{Name: "cmd-2", Cmd: "echo", Args: []string{"second"}},
				{Name: "cmd-3", Cmd: "echo", Args: []string{"third"}},
			},
		}

		config := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(3600),
			},
			Groups: []runnertypes.GroupSpec{group},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command fails with non-zero exit code
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &group, mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Subsequent commands should not be executed due to fail-fast behavior
		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, &group)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("multiple commands with middle failing", func(t *testing.T) {
		group := runnertypes.GroupSpec{
			Name: "test-middle-fails",
			Commands: []runnertypes.CommandSpec{
				{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}},
				{Name: "cmd-2", Cmd: "false"}, // This fails
				{Name: "cmd-3", Cmd: "echo", Args: []string{"third"}},
			},
		}

		config := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(3600),
			},
			Groups: []runnertypes.GroupSpec{group},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command succeeds
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &group, mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil).Once()

		// Second command fails
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &group, mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil).Once()
		// Third command should not be executed due to fail-fast behavior

		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, &group)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("executor returns error instead of non-zero exit code", func(t *testing.T) {
		group := runnertypes.GroupSpec{
			Name: "test-executor-error",
			Commands: []runnertypes.CommandSpec{
				{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}},
				{Name: "cmd-2", Cmd: "cat"}, // Use a real command that the executor will return an error for
				{Name: "cmd-3", Cmd: "echo", Args: []string{"third"}},
			},
		}

		config := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(3600),
			},
			Groups: []runnertypes.GroupSpec{group},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command succeeds
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &group, mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil).Once()

		// Second command returns executor error
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &group, mock.Anything).
			Return(resource.CommandToken(""), (*resource.ExecutionResult)(nil), errCommandNotFound).Once()
		// Third command should not be executed

		ctx := context.Background()
		err = runner.ExecuteGroup(ctx, &group)

		assert.Error(t, err)
		assert.ErrorIs(t, err, errCommandNotFound)
		mockResourceManager.AssertExpectations(t)
	})
}

func TestRunner_ExecuteAll(t *testing.T) {
	setupSafeTestEnv(t)

	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: common.Int32Ptr(3600),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:     "group-2",
				Priority: 2,
				Commands: []runnertypes.CommandSpec{
					{Name: "cmd-2", Cmd: "echo", Args: []string{"second"}},
				},
			},
			{
				Name:     "group-1",
				Priority: 1,
				Commands: []runnertypes.CommandSpec{
					{Name: "cmd-1", Cmd: "echo", Args: []string{"first"}},
				},
			},
		},
	}

	mockResourceManager := new(MockResourceManager)
	runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
	require.NoError(t, err)

	// Setup mock expectations - should be called in priority order
	mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[1], mock.Anything).Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "first\n"}, nil)
	mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[0], mock.Anything).Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "second\n"}, nil)

	ctx := context.Background()
	err = runner.ExecuteAll(ctx)

	assert.NoError(t, err)
	mockResourceManager.AssertExpectations(t)
}

func TestRunner_ExecuteAll_ComplexErrorScenarios(t *testing.T) {
	setupSafeTestEnv(t)

	t.Run("first group fails, but remaining groups should still execute", func(t *testing.T) {
		config := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(3600),
			},
			Groups: []runnertypes.GroupSpec{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.CommandSpec{
						{Name: "fail-cmd", Cmd: "false"},
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.CommandSpec{
						{Name: "success-cmd", Cmd: "echo", Args: []string{"should execute"}},
					},
				},
				{
					Name:     "group-3",
					Priority: 3,
					Commands: []runnertypes.CommandSpec{
						{Name: "another-cmd", Cmd: "echo", Args: []string{"also should execute"}},
					},
				},
			},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First group's command should be called and fail
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[0], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Remaining groups should still be executed
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[1], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "should execute\n", Stderr: ""}, nil)

		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[2], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "also should execute\n", Stderr: ""}, nil)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		// Should still return error from first group, but all groups executed
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("middle group fails, but remaining groups should still execute", func(t *testing.T) {
		config := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(3600),
			},
			Groups: []runnertypes.GroupSpec{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.CommandSpec{
						{Name: "success-cmd-1", Cmd: "echo", Args: []string{"first"}},
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.CommandSpec{
						{Name: "fail-cmd", Cmd: "false"},
					},
				},
				{
					Name:     "group-3",
					Priority: 3,
					Commands: []runnertypes.CommandSpec{
						{Name: "should-execute", Cmd: "echo", Args: []string{"third"}},
					},
				},
			},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First group should succeed
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[0], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil)

		// Second group should fail
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[1], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)

		// Third group should still be executed
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[2], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "third\n", Stderr: ""}, nil)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("group with multiple commands, second command fails, but next group still executes", func(t *testing.T) {
		config := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(3600),
			},
			Groups: []runnertypes.GroupSpec{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.CommandSpec{
						{Name: "success-cmd-1", Cmd: "echo", Args: []string{"first"}},
						{Name: "fail-cmd", Cmd: "false"},
						{Name: "should-not-execute", Cmd: "echo", Args: []string{"third"}},
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.CommandSpec{
						{Name: "group2-cmd", Cmd: "echo", Args: []string{"group2"}},
					},
				},
			},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command in group-1 should succeed
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[0], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "first\n", Stderr: ""}, nil).Once()

		// Second command in group-1 should fail
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[0], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil).Once()

		// Third command in group-1 should not be executed (group-level failure stops remaining commands in same group)
		// But group-2 should still be executed (new behavior)
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[1], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "group2\n", Stderr: ""}, nil).Once()

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandFailed)
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("executor error in first group, but remaining groups should still execute", func(t *testing.T) {
		config := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(3600),
			},
			Groups: []runnertypes.GroupSpec{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.CommandSpec{
						{Name: "executor-error-cmd", Cmd: "cat"}, // Use a real command that can return an error
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.CommandSpec{
						{Name: "should-execute", Cmd: "echo", Args: []string{"second"}},
					},
				},
			},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
		require.NoError(t, err)

		// First command should return executor error
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[0], mock.Anything).
			Return(resource.CommandToken(""), (*resource.ExecutionResult)(nil), errCommandNotFound)

		// Second group should still be executed
		mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, &config.Groups[1], mock.Anything).
			Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "second\n", Stderr: ""}, nil)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		assert.Error(t, err)
		assert.ErrorIs(t, err, errCommandNotFound)
		mockResourceManager.AssertExpectations(t)
	})

	t.Run("context cancellation during execution", func(t *testing.T) {
		config := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(3600),
			},
			Groups: []runnertypes.GroupSpec{
				{
					Name:     "group-1",
					Priority: 1,
					Commands: []runnertypes.CommandSpec{
						{Name: "long-running-cmd", Cmd: "sleep", Args: []string{"10"}},
					},
				},
				{
					Name:     "group-2",
					Priority: 2,
					Commands: []runnertypes.CommandSpec{
						{Name: "should-not-execute", Cmd: "echo", Args: []string{"second"}},
					},
				},
			},
		}

		mockResourceManager := new(MockResourceManager)
		runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
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
		config := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(3600),
			},
			Groups: []runnertypes.GroupSpec{}, // Empty groups
		}

		runner, err := NewRunner(config, WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
		require.NoError(t, err)

		ctx := context.Background()
		err = runner.ExecuteAll(ctx)

		// Should succeed with no groups to execute
		assert.NoError(t, err)
	})
}

// TestRunner_createCommandContext has been removed as it tested an internal implementation detail
// of GroupExecutor. Timeout behavior is already tested by TestRunner_CommandTimeoutBehavior.

func TestRunner_CommandTimeoutBehavior(t *testing.T) {
	t.Skip("Skipped: Requires actual sleep command execution which is not compatible with mock-based testing architecture")
	sleepCmd := runnertypes.CommandSpec{
		Cmd:  "sleep",
		Args: []string{"5"}, // Sleep for 5 seconds, longer than timeout
	}

	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: common.Int32Ptr(1), // 1 second timeout
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:     "timeout-test-group",
				Commands: []runnertypes.CommandSpec{sleepCmd},
			},
		},
	}

	t.Run("global timeout is enforced", func(t *testing.T) {
		runner, err := NewRunner(config, WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
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
		shortTimeoutCmd := runnertypes.CommandSpec{
			Cmd:     "sleep",
			Args:    []string{"5"},      // Sleep for 5 seconds
			Timeout: common.Int32Ptr(1), // But timeout after 1 second
		}

		configWithCmdTimeout := &runnertypes.ConfigSpec{
			Version: "1.0",
			Global: runnertypes.GlobalSpec{
				Timeout: common.Int32Ptr(10), // 10 seconds global timeout
			},
			Groups: []runnertypes.GroupSpec{
				{
					Name:     "cmd-timeout-test-group",
					Commands: []runnertypes.CommandSpec{shortTimeoutCmd},
				},
			},
		}

		runner, err := NewRunner(configWithCmdTimeout, WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
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
		runner, err := NewRunner(config, WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
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

func TestCommandGroup_NewFields(t *testing.T) {
	// Setup test environment
	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		group       runnertypes.GroupSpec
		expectError bool
		description string
	}{
		{
			name: "WorkDir specified",
			group: runnertypes.GroupSpec{
				Name:    "test-workdir",
				WorkDir: "/tmp",
				Commands: []runnertypes.CommandSpec{
					{Name: "test", Cmd: "echo", Args: []string{"hello"}},
				},
				EnvAllowed: []string{"PATH"},
			},
			expectError: false,
			description: "Should set working directory from group WorkDir field",
		},
		{
			name: "Command with existing WorkDir should not be overridden",
			group: runnertypes.GroupSpec{
				Name:    "test-existing-dir",
				WorkDir: "/tmp",
				Commands: []runnertypes.CommandSpec{
					{Name: "test", Cmd: "echo", Args: []string{"hello"}, WorkDir: "/usr"},
				},
				EnvAllowed: []string{"PATH"},
			},
			expectError: false,
			description: "Commands with existing WorkDir should not be overridden by group WorkDir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					EnvAllowed: []string{"PATH"},
				},
				Groups: []runnertypes.GroupSpec{tt.group},
			}

			// Create runner with mock resource manager to avoid actually executing commands
			mockResourceManager := &MockResourceManager{}
			mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
				resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: "test output", Stderr: ""}, nil)

			runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
			require.NoError(t, err) // Load basic environment
			err = runner.LoadSystemEnvironment()
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &tt.group)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)

				// Verify mock was called
				mockResourceManager.AssertExpectations(t)

				// Additional verification based on test case
				switch tt.name {
				case "WorkDir specified", "Command with existing WorkDir should not be overridden":
					// Verify the command was called with the expected working directory
					calls := mockResourceManager.Calls
					require.Len(t, calls, 1)
					cmd, ok := calls[0].Arguments[1].(*runnertypes.RuntimeCommand)
					require.True(t, ok, "expected calls[0].Arguments[1] to be of type *runnertypes.RuntimeCommand, but it was not")
					if tt.name == "WorkDir specified" {
						assert.Equal(t, "/tmp", cmd.EffectiveWorkDir)
					} else {
						assert.Equal(t, "/usr", cmd.EffectiveWorkDir) // Should not be overridden
					}
				}
			}
		})
	}
}

// TestRunner_EnvironmentVariablePriority_GroupLevelSupport tests the priority hierarchy for environment variables:
// Priority order: System < Global < Group < Command
func TestRunner_EnvironmentVariablePriority_GroupLevelSupport(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		systemEnv   string
		globalEnv   []string
		groupEnv    []string
		commandEnv  []string
		expectedVar string
	}{
		{
			name:        "Command overrides Group and Global",
			systemEnv:   "from_system",
			globalEnv:   []string{"TEST_VAR=from_global"},
			groupEnv:    []string{"TEST_VAR=from_group"},
			commandEnv:  []string{"TEST_VAR=from_command"},
			expectedVar: "from_command",
		},
		{
			name:        "Group overrides Global and System",
			systemEnv:   "from_system",
			globalEnv:   []string{"TEST_VAR=from_global"},
			groupEnv:    []string{"TEST_VAR=from_group"},
			commandEnv:  nil, // No command-level env
			expectedVar: "from_group",
		},
		{
			name:        "Global overrides System",
			systemEnv:   "from_system",
			globalEnv:   []string{"TEST_VAR=from_global"},
			groupEnv:    nil, // No group-level env
			commandEnv:  nil, // No command-level env
			expectedVar: "from_global",
		},
		{
			name:        "System environment used when not overridden",
			systemEnv:   "from_system",
			globalEnv:   nil, // No global-level env
			groupEnv:    nil, // No group-level env
			commandEnv:  nil, // No command-level env
			expectedVar: "from_system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_VAR", tt.systemEnv)

			config := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout:    common.Int32Ptr(3600),
					EnvAllowed: []string{"TEST_VAR"},
					EnvVars:    tt.globalEnv,
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name:    "test-group",
						EnvVars: tt.groupEnv,
						Commands: []runnertypes.CommandSpec{
							{
								Name:    "test-cmd",
								Cmd:     "printenv",
								Args:    []string{"TEST_VAR"},
								EnvVars: tt.commandEnv,
							},
						},
					},
				},
			}

			mockResourceManager := &MockResourceManager{}

			// Capture the actual envVars passed to ExecuteCommand
			var capturedEnv map[string]string
			mockResourceManager.On("ExecuteCommand",
				mock.Anything, // ctx
				mock.Anything, // cmd
				mock.Anything, // group
				mock.MatchedBy(func(env map[string]string) bool {
					capturedEnv = env
					return true
				})).
				Return(resource.CommandToken(""), &resource.ExecutionResult{ExitCode: 0, Stdout: tt.expectedVar + "\n", Stderr: ""}, nil)

			runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-run-123"))
			require.NoError(t, err)

			err = runner.LoadSystemEnvironment()
			require.NoError(t, err)

			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &config.Groups[0])
			require.NoError(t, err)

			// Verify environment variable priority
			assert.Equal(t, tt.expectedVar, capturedEnv["TEST_VAR"])
			mockResourceManager.AssertExpectations(t)
		})
	}
}

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
			// Create config with a simple command group
			config := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout: common.Int32Ptr(30),
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name:        "test-group",
						Description: "Test group for notification",
						Commands: []runnertypes.CommandSpec{
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
					resource.CommandToken(""), &resource.ExecutionResult{
						ExitCode: 0,
						Stdout:   "test output",
						Stderr:   "",
					}, nil,
				)
			} else {
				mockResourceManager.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
					resource.CommandToken(""), &resource.ExecutionResult{
						ExitCode: 1,
						Stdout:   "",
						Stderr:   "command failed",
					}, nil,
				)
			}

			// Create runner options
			var options []Option
			options = append(options, WithResourceManager(mockResourceManager))
			options = append(options, WithVerificationManager(setupDryRunVerification(t)))
			options = append(options, WithRunID("test-run-123"))

			// Create runner
			runner, err := NewRunner(config, options...)
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &config.Groups[0])

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
		commands    []runnertypes.CommandSpec
		expectError bool
		description string
	}{
		{
			name: "command with output configuration",
			commands: []runnertypes.CommandSpec{
				{
					Name:       "test-echo",
					Cmd:        "echo",
					Args:       []string{"Hello World"},
					OutputFile: "test-output.txt",
				},
			},
			expectError: false, // Note: This may fail due to output capture implementation, which is expected
			description: "Command with output configuration should be parsed correctly",
		},
		{
			name: "command without output capture",
			commands: []runnertypes.CommandSpec{
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
			commands: []runnertypes.CommandSpec{
				{
					Name:       "with-output",
					Cmd:        "echo",
					Args:       []string{"Captured"},
					OutputFile: "mixed-output.txt",
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
			// Create config with output capture settings
			config := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout:         common.Int32Ptr(30),
					OutputSizeLimit: common.Int64Ptr(1024 * 1024), // 1MB limit
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name:        "output-test-group",
						Description: "Test group for output capture",
						Commands:    tt.commands,
					},
				},
			}

			// Create runner
			runner, err := NewRunner(config, WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-end-to-end"))
			require.NoError(t, err, "NewRunner should not return an error")

			// Load basic environment
			err = runner.LoadSystemEnvironment()
			require.NoError(t, err, "LoadSystemEnvironment should not return an error")

			// Verify runner was created properly with output capture configuration
			runnerConfig := runner.config
			assert.Equal(t, config, runnerConfig)
			require.NotNil(t, runnerConfig.Global.OutputSizeLimit)
			assert.Equal(t, int64(1024*1024), *runnerConfig.Global.OutputSizeLimit)

			// Verify output field is preserved in configuration
			for i, originalCmd := range tt.commands {
				actualCmd := runnerConfig.Groups[0].Commands[i]
				assert.Equal(t, originalCmd.OutputFile, actualCmd.OutputFile, "Output field should be preserved")
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
		commands     []runnertypes.CommandSpec
		globalConfig runnertypes.GlobalSpec
		expectError  string
		description  string
	}{
		{
			name: "path traversal attempt",
			commands: []runnertypes.CommandSpec{
				{
					Name:       "path-traversal",
					Cmd:        "echo",
					Args:       []string{"attempt"},
					OutputFile: "../../../etc/passwd",
				},
			},
			globalConfig: runnertypes.GlobalSpec{
				Timeout:         common.Int32Ptr(30),
				OutputSizeLimit: common.Int64Ptr(1024),
			},
			expectError: "path traversal",
			description: "Path traversal attempts should be rejected",
		},
		{
			name: "non-existent directory",
			commands: []runnertypes.CommandSpec{
				{
					Name:       "non-existent-dir",
					Cmd:        "echo",
					Args:       []string{"test"},
					OutputFile: "/non/existent/directory/output.txt",
				},
			},
			globalConfig: runnertypes.GlobalSpec{
				Timeout:         common.Int32Ptr(30),
				OutputSizeLimit: common.Int64Ptr(1024),
			},
			expectError: "directory",
			description: "Non-existent directories should cause error",
		},
		{
			name: "permission denied directory",
			commands: []runnertypes.CommandSpec{
				{
					Name:       "permission-denied",
					Cmd:        "echo",
					Args:       []string{"test"},
					OutputFile: "/root/output.txt",
				},
			},
			globalConfig: runnertypes.GlobalSpec{
				Timeout:         common.Int32Ptr(30),
				OutputSizeLimit: common.Int64Ptr(1024),
			},
			expectError: "permission",
			description: "Permission denied should cause error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config
			config := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global:  tt.globalConfig,
				Groups: []runnertypes.GroupSpec{
					{
						Name:        "error-test-group",
						Description: "Test group for output capture errors",
						Commands:    tt.commands,
					},
				},
			}

			// Create runner
			runner, err := NewRunner(config, WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-error-scenarios"))
			require.NoError(t, err, "NewRunner should not return an error")

			// Load basic environment
			err = runner.LoadSystemEnvironment()
			require.NoError(t, err, "LoadSystemEnvironment should not return an error")

			// Execute the group - should fail
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &config.Groups[0])

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
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout:         common.Int32Ptr(30),
			OutputSizeLimit: common.Int64Ptr(1024),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:        "dryrun-test-group",
				Description: "Test group for dry-run output capture",
				Commands: []runnertypes.CommandSpec{
					{
						Name:       "dryrun-echo",
						Cmd:        "echo",
						Args:       []string{"Dry run test"},
						OutputFile: "dryrun-output.txt",
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
		resource.CommandToken(""), &resource.ExecutionResult{
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
	runner, err := NewRunner(config, WithResourceManager(mockResourceManager), WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-dry-run"))
	require.NoError(t, err, "NewRunner should not return an error")

	// Load basic environment
	err = runner.LoadSystemEnvironment()
	require.NoError(t, err, "LoadSystemEnvironment should not return an error")

	// Enable dry-run mode through mock resource manager
	mockResourceManager.SetMode(resource.ExecutionModeDryRun, nil)

	// Execute the group in dry-run mode
	ctx := context.Background()
	err = runner.ExecuteGroup(ctx, &config.Groups[0])

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
output_size_limit = 1048576

[[groups]]
name = "output_capture_group"
description = "Test group with output capture"

[[groups.commands]]
name = "simple-echo"
cmd = "echo"
args = ["Hello from TOML config"]
output_file = "toml-output.txt"

[[groups.commands]]
name = "multiline-output"
cmd = "sh"
args = ["-c", "echo 'Line 1'; echo 'Line 2'"]
output_file = "multiline-toml-output.txt"

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
		// Note: Global.WorkDir has been removed in Task 0034
		require.NotNil(t, config.Global.OutputSizeLimit)
		assert.Equal(t, int64(1048576), *config.Global.OutputSizeLimit)
		assert.Len(t, config.Groups, 1)
		assert.Equal(t, "output_capture_group", config.Groups[0].Name)
		assert.Len(t, config.Groups[0].Commands, 3)

		// Verify commands have correct output configuration
		assert.Equal(t, "toml-output.txt", config.Groups[0].Commands[0].OutputFile)
		assert.Equal(t, "multiline-toml-output.txt", config.Groups[0].Commands[1].OutputFile)
		assert.Equal(t, "", config.Groups[0].Commands[2].OutputFile) // No output field

		// Create runner to verify basic initialization works
		runner, err := NewRunner(config, WithVerificationManager(setupDryRunVerification(t)), WithRunID("test-toml-config"))
		require.NoError(t, err, "NewRunner should not return an error")

		// Load basic environment to verify runner setup
		err = runner.LoadSystemEnvironment()
		require.NoError(t, err, "LoadSystemEnvironment should not return an error")

		// Verify runner configuration
		runnerConfig := runner.config
		// Compare fields individually as ConfigSpec should have Version field
		assert.Equal(t, config.Global, runnerConfig.Global)
		assert.Equal(t, len(config.Groups), len(runnerConfig.Groups))
		if len(config.Groups) > 0 && len(runnerConfig.Groups) > 0 {
			assert.Equal(t, config.Groups[0].Name, runnerConfig.Groups[0].Name)
			assert.Equal(t, len(config.Groups[0].Commands), len(runnerConfig.Groups[0].Commands))
		}
	})

	// Test TOML config validation for output capture
	t.Run("TOML config validation", func(t *testing.T) {
		invalidTomlContent := `
[global]
timeout = 30
workdir = "` + tempDir + `"
output_size_limit = -1  # Invalid negative size

[[groups]]
name = "invalid_group"

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

		// Verify negative output_size_limit was loaded (validation happens later)
		require.NotNil(t, config.Global.OutputSizeLimit)
		assert.Equal(t, int64(-1), *config.Global.OutputSizeLimit)
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
				setupFailedMockExecution(mockRM, errors.New("invalid output format"))
			},
			expectError: "invalid output format",
		},
		{
			name: "SecurityViolation",
			setupMock: func(mockRM *MockResourceManager) {
				setupFailedMockExecution(mockRM, errors.New("security violation: path traversal detected"))
			},
			expectError: "security violation",
		},
		{
			name: "DiskFull",
			setupMock: func(mockRM *MockResourceManager) {
				setupFailedMockExecution(mockRM, errors.New("disk full: cannot write output"))
			},
			expectError: "disk full",
		},
		{
			name: "Unknown",
			setupMock: func(mockRM *MockResourceManager) {
				setupFailedMockExecution(mockRM, errors.New("unknown error occurred"))
			},
			expectError: "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create basic configuration with output capture
			cfg := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout:         common.Int32Ptr(30),
					OutputSizeLimit: common.Int64Ptr(1024),
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test-group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:       "test-cmd",
								Cmd:        "echo",
								Args:       []string{"test"},
								OutputFile: "output.txt",
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
				WithVerificationManager(setupDryRunVerification(t)),
				WithRunID("test-run-id"),
			}

			runner, err := NewRunner(cfg, options...)
			require.NoError(t, err)

			// Execute the group instead of full run
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &cfg.Groups[0])

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
				setupFailedMockExecution(mockRM, ErrPreValidationTest)
			},
			expectError: ErrPreValidationTest,
		},
		{
			name:  "ExecutionError",
			stage: "execution",
			setupMock: func(mockRM *MockResourceManager) {
				// Simulate execution error (during command execution)
				setupFailedMockExecution(mockRM, ErrExecutionTest)
			},
			expectError: ErrExecutionTest,
		},
		{
			name:  "PostProcessingError",
			stage: "post-processing",
			setupMock: func(mockRM *MockResourceManager) {
				// Simulate post-processing error (after command execution)
				setupFailedMockExecution(mockRM, ErrPostProcessingTest)
			},
			expectError: ErrPostProcessingTest,
		},
		{
			name:  "CleanupError",
			stage: "cleanup",
			setupMock: func(mockRM *MockResourceManager) {
				// Simulate cleanup error (during resource cleanup)
				setupFailedMockExecution(mockRM, ErrCleanupTest)
			},
			expectError: ErrCleanupTest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create basic configuration with output capture
			cfg := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout:         common.Int32Ptr(30),
					OutputSizeLimit: common.Int64Ptr(1024),
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test-group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:       "test-cmd",
								Cmd:        "echo",
								Args:       []string{"test"},
								OutputFile: "output.txt",
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
				WithVerificationManager(setupDryRunVerification(t)),
				WithRunID("test-run-id"),
			}

			runner, err := NewRunner(cfg, options...)
			require.NoError(t, err)

			// Execute the group instead of full run
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &cfg.Groups[0])

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
			description: "Path traversal attempts should be blocked",
		},
		{
			name:        "AbsolutePathBlocked",
			outputPath:  "/etc/shadow",
			expectError: true,
			description: "Absolute paths should be blocked for security",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create configuration with potentially malicious output path
			cfg := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout:         common.Int32Ptr(30),
					OutputSizeLimit: common.Int64Ptr(1024),
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "security-test-group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:       "security-test-cmd",
								Cmd:        "echo",
								Args:       []string{"test"},
								OutputFile: tt.outputPath,
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
					Return(resource.CommandToken(""), result, nil)
				options = []Option{
					WithResourceManager(mockRM),
					WithVerificationManager(setupDryRunVerification(t)),
					WithRunID("test-run-output-capture"),
				}
			} else {
				// For error cases, use default resource manager to allow real validation
				options = []Option{
					WithVerificationManager(setupDryRunVerification(t)),
					WithRunID("test-run-output-capture"),
				}
			}

			runner, err := NewRunner(cfg, options...)
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &cfg.Groups[0])

			if tt.expectError {
				require.Error(t, err, "Should return error for %s", tt.description)
			} else {
				require.NoError(t, err, "Should not return error for %s", tt.description)
			}

			// Verify mock expectations (only for success cases with mock)
			// Error cases use real resource manager, so no mock to verify
		})
	}
}

// TestWithExecutor tests the WithExecutor option
func TestWithExecutor(t *testing.T) {
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global:  runnertypes.GlobalSpec{},
	}

	// Create a simple test executor (we can't easily create a mock without defining it)
	// Instead, we just verify that the option doesn't cause an error
	runner, err := NewRunner(config,
		WithExecutor(nil), // nil is handled
		WithVerificationManager(setupDryRunVerification(t)),
		WithRunID("test-with-executor"))
	require.NoError(t, err)
	assert.NotNil(t, runner)
}

// TestWithPrivilegeManager tests the WithPrivilegeManager option
func TestWithPrivilegeManager(t *testing.T) {
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global:  runnertypes.GlobalSpec{},
	}

	// Test with nil privilege manager (acceptable)
	runner, err := NewRunner(config,
		WithPrivilegeManager(nil),
		WithVerificationManager(setupDryRunVerification(t)),
		WithRunID("test-with-privmgr"))
	require.NoError(t, err)
	assert.NotNil(t, runner)
}

// TestWithDryRun tests the WithDryRun option
func TestWithDryRun(t *testing.T) {
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global:  runnertypes.GlobalSpec{},
	}

	dryRunOpts := &resource.DryRunOptions{
		// Use fields that actually exist in DryRunOptions
	}

	runner, err := NewRunner(config,
		WithDryRun(dryRunOpts),
		WithVerificationManager(setupDryRunVerification(t)),
		WithRunID("test-with-dryrun"))
	require.NoError(t, err)
	assert.NotNil(t, runner)
	// Verify dry-run resource manager is used
	assert.NotNil(t, runner.resourceManager)
}

// TestWithKeepTempDirs tests the WithKeepTempDirs option
func TestWithKeepTempDirs(t *testing.T) {
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global:  runnertypes.GlobalSpec{},
	}

	// Test with true
	runner, err := NewRunner(config,
		WithKeepTempDirs(true),
		WithVerificationManager(setupDryRunVerification(t)),
		WithRunID("test-with-keeptmp"))
	require.NoError(t, err)
	assert.NotNil(t, runner)
	// Note: keepTempDirs is not directly accessible from Runner, but we can verify the option doesn't error

	// Test with false
	runner2, err := NewRunner(config,
		WithKeepTempDirs(false),
		WithVerificationManager(setupDryRunVerification(t)),
		WithRunID("test-with-keeptmp-false"))
	require.NoError(t, err)
	assert.NotNil(t, runner2)
}

func TestRunner_ExecutorUsesDefaultLogger(t *testing.T) {
	// This test verifies that when NewRunner creates a default executor,
	// it configures it with slog.Default() instead of io.Discard.
	// This ensures executor logs are visible in production.

	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: common.Int32Ptr(3600),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name:     "test_group",
				Priority: 1,
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test_cmd",
						Cmd:  "/bin/echo",
						Args: []string{"test"},
					},
				},
			},
		},
	}

	// Create runner without providing a custom executor
	// This will trigger the default executor creation with slog.Default()
	runner, err := NewRunner(config,
		WithVerificationManager(setupDryRunVerification(t)),
		WithRunID("test-logger"))
	require.NoError(t, err)
	assert.NotNil(t, runner)
	assert.NotNil(t, runner.executor)

	// Note: We cannot directly inspect the executor's logger field since it's private,
	// but we can verify that the executor was created successfully.
	// The actual logging behavior is tested in executor_logging_test.go
}

// TestCommandResult_LogValue tests the slog.LogValuer implementation
func TestCommandResult_LogValue(t *testing.T) {
	tests := []struct {
		name     string
		result   common.CommandResult
		expected map[string]any
	}{
		{
			name: "complete result",
			result: common.CommandResult{
				CommandResultFields: common.CommandResultFields{
					Name:     "test-cmd",
					ExitCode: 0,
					Output:   "success output",
					Stderr:   "",
				},
			},
			expected: map[string]any{
				"name":      "test-cmd",
				"exit_code": 0,
				"output":    "success output",
				"stderr":    "",
			},
		},
		{
			name: "failed command with stderr",
			result: common.CommandResult{
				CommandResultFields: common.CommandResultFields{
					Name:     "failing-cmd",
					ExitCode: 1,
					Output:   "",
					Stderr:   "error message",
				},
			},
			expected: map[string]any{
				"name":      "failing-cmd",
				"exit_code": 1,
				"output":    "",
				"stderr":    "error message",
			},
		},
		{
			name: "command with both output and stderr",
			result: common.CommandResult{
				CommandResultFields: common.CommandResultFields{
					Name:     "mixed-cmd",
					ExitCode: 2,
					Output:   "some output",
					Stderr:   "some error",
				},
			},
			expected: map[string]any{
				"name":      "mixed-cmd",
				"exit_code": 2,
				"output":    "some output",
				"stderr":    "some error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logValue := tt.result.LogValue()

			// LogValue returns a GroupValue, which contains []slog.Attr
			require.Equal(t, logValue.Kind(), slog.KindGroup)

			attrs := logValue.Group()
			attrMap := make(map[string]any)
			for _, attr := range attrs {
				switch attr.Value.Kind() {
				case slog.KindString:
					attrMap[attr.Key] = attr.Value.String()
				case slog.KindInt64:
					attrMap[attr.Key] = int(attr.Value.Int64())
				}
			}

			assert.Equal(t, tt.expected, attrMap)
		})
	}
}

func TestRunner_ExecuteFiltered(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name           string
		config         *runnertypes.ConfigSpec
		groupNames     []string
		expectedGroups []string // Expected groups to be executed (in order)
		expectError    bool
	}{
		{
			name: "nil input executes all groups",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "common", Priority: 1},
					{Name: "build", Priority: 2},
					{Name: "test", Priority: 3},
				},
			},
			groupNames:     nil,
			expectedGroups: []string{"common", "build", "test"},
			expectError:    false,
		},
		{
			name: "empty slice executes all groups",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "common", Priority: 1},
					{Name: "build", Priority: 2},
					{Name: "test", Priority: 3},
				},
			},
			groupNames:     []string{},
			expectedGroups: []string{"common", "build", "test"},
			expectError:    false,
		},
		{
			name: "single group filter",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "common", Priority: 1},
					{Name: "build", Priority: 2},
					{Name: "test", Priority: 3},
				},
			},
			groupNames:     []string{"build"},
			expectedGroups: []string{"build"},
			expectError:    false,
		},
		{
			name: "multiple groups filter",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "common", Priority: 1},
					{Name: "build", Priority: 2},
					{Name: "test", Priority: 3},
					{Name: "deploy", Priority: 4},
				},
			},
			groupNames:     []string{"build", "test"},
			expectedGroups: []string{"build", "test"},
			expectError:    false,
		},
		{
			name: "filter preserves priority order",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "test", Priority: 3},
					{Name: "build", Priority: 2},
					{Name: "common", Priority: 1},
				},
			},
			groupNames:     []string{"test", "common"},
			expectedGroups: []string{"common", "test"}, // Should be sorted by priority
			expectError:    false,
		},
		{
			name: "includes dependent groups when filtering subset",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "common", Priority: 1},
					{Name: "build", Priority: 2, DependsOn: []string{"common"}},
					{Name: "test", Priority: 3, DependsOn: []string{"build"}},
				},
			},
			groupNames:     []string{"test"},
			expectedGroups: []string{"common", "build", "test"},
			expectError:    false,
		},
		{
			name: "returns error when dependency missing",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "test", Priority: 1, DependsOn: []string{"build"}},
				},
			},
			groupNames:  []string{"test"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track executed groups
			var executedGroups []string

			// Create a mock group executor that tracks group execution
			mockGroupExecutor := &MockGroupExecutor{}
			if !tt.expectError {
				mockGroupExecutor.On("ExecuteGroup", mock.Anything, mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						groupSpec := args.Get(1).(*runnertypes.GroupSpec)
						executedGroups = append(executedGroups, groupSpec.Name)
					}).
					Return(nil)
			}

			// Create runner with mock executor
			runner, err := NewRunner(tt.config,
				WithVerificationManager(setupDryRunVerification(t)),
				WithRunID("test-run-123"),
				WithRuntimeGlobal(&runnertypes.RuntimeGlobal{}))
			require.NoError(t, err)

			// Replace group executor with mock
			runner.groupExecutor = mockGroupExecutor

			// Execute filtered
			ctx := context.Background()
			err = runner.ExecuteFiltered(ctx, tt.groupNames)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedGroups, executedGroups, "Executed groups should match expected groups")
				mockGroupExecutor.AssertExpectations(t)
			}
		})
	}
}

func TestRunner_filterConfigGroups(t *testing.T) {
	tests := []struct {
		name           string
		config         *runnertypes.ConfigSpec
		groupNames     []string
		expectedGroups []string
		expectError    bool
	}{
		{
			name: "filter single group",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "common"},
					{Name: "build"},
					{Name: "test"},
				},
			},
			groupNames:     []string{"build"},
			expectedGroups: []string{"build"},
		},
		{
			name: "filter multiple groups",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "common"},
					{Name: "build"},
					{Name: "test"},
					{Name: "deploy"},
				},
			},
			groupNames:     []string{"build", "deploy"},
			expectedGroups: []string{"build", "deploy"},
		},
		{
			name: "filter preserves original order",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "test"},
					{Name: "build"},
					{Name: "common"},
				},
			},
			groupNames:     []string{"common", "test"},
			expectedGroups: []string{"test", "common"}, // Original order from config
		},
		{
			name: "includes dependencies",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "common"},
					{Name: "build", DependsOn: []string{"common"}},
					{Name: "test", DependsOn: []string{"build"}},
				},
			},
			groupNames:     []string{"test"},
			expectedGroups: []string{"common", "build", "test"},
		},
		{
			name: "missing dependency returns error",
			config: &runnertypes.ConfigSpec{
				Version: "1.0",
				Groups: []runnertypes.GroupSpec{
					{Name: "test", DependsOn: []string{"missing"}},
				},
			},
			groupNames:  []string{"test"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &Runner{
				config: tt.config,
			}

			filteredConfig, err := runner.filterConfigGroups(tt.groupNames)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Extract group names from filtered config
			var filteredNames []string
			for _, group := range filteredConfig.Groups {
				filteredNames = append(filteredNames, group.Name)
			}

			assert.Equal(t, tt.expectedGroups, filteredNames)

			// Verify global config is preserved
			assert.Equal(t, tt.config.Version, filteredConfig.Version)
			assert.Equal(t, tt.config.Global, filteredConfig.Global)
		})
	}
}

func TestGroupFilteringE2E(t *testing.T) {
	setupSafeTestEnv(t)

	configPath := filepath.Join("testdata", "group_filtering_test.toml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var cfg runnertypes.ConfigSpec
	require.NoError(t, toml.Unmarshal(data, &cfg))

	runtimeGlobal, err := configpkg.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	runner, err := NewRunner(&cfg,
		WithVerificationManager(setupDryRunVerification(t)),
		WithRunID("test-e2e-run"),
		WithRuntimeGlobal(runtimeGlobal))
	require.NoError(t, err)

	var executedGroups []string
	mockGroupExecutor := &MockGroupExecutor{}
	mockGroupExecutor.On("ExecuteGroup", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			groupSpec := args.Get(1).(*runnertypes.GroupSpec)
			executedGroups = append(executedGroups, groupSpec.Name)
		}).
		Return(nil)
	runner.groupExecutor = mockGroupExecutor

	ctx := context.Background()
	require.NoError(t, runner.ExecuteFiltered(ctx, []string{"test"}))

	assert.Equal(t, []string{"common", "build_backend", "build_frontend", "test"}, executedGroups)
	mockGroupExecutor.AssertExpectations(t)
}
