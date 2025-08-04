package executor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	privtesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestDefaultExecutor_WithPrivilegeManager(t *testing.T) {
	mockPrivMgr := privtesting.NewMockPrivilegeManager(true)

	executor := executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(mockPrivMgr),
	)

	assert.NotNil(t, executor)
}

func TestDefaultExecutor_PrivilegedExecution(t *testing.T) {
	tests := []struct {
		name               string
		cmd                runnertypes.Command
		privilegeSupported bool
		expectError        bool
		errorMessage       string
		expectedErrorType  error // Expected error type for errors.Is() check
		noPrivilegeManager bool  // Whether to create executor without privilege manager
		expectElevations   []string
	}{
		{
			name: "privileged command executes with elevation",
			cmd: runnertypes.Command{
				Name:       "test_privileged",
				Cmd:        "/usr/bin/echo",
				Args:       []string{"test"},
				Privileged: true,
			},
			privilegeSupported: true,
			expectError:        false,
			noPrivilegeManager: false,
			expectElevations:   []string{"command_execution"},
		},
		{
			name: "privileged command fails when not supported",
			cmd: runnertypes.Command{
				Name:       "test_privileged",
				Cmd:        "/usr/bin/echo",
				Args:       []string{"test"},
				Privileged: true,
			},
			privilegeSupported: false,
			expectError:        true,
			errorMessage:       "privileged execution not supported",
			expectedErrorType:  runnertypes.ErrPlatformNotSupported,
			noPrivilegeManager: false,
		},
		{
			name: "privileged command fails with no manager",
			cmd: runnertypes.Command{
				Name:       "test_privileged",
				Cmd:        "/usr/bin/echo",
				Args:       []string{"test"},
				Privileged: true,
			},
			privilegeSupported: true,
			expectError:        true,
			errorMessage:       "no privilege manager available",
			expectedErrorType:  executor.ErrNoPrivilegeManager,
			noPrivilegeManager: true,
		},
		{
			name: "normal command bypasses privilege manager",
			cmd: runnertypes.Command{
				Name:       "test_normal",
				Cmd:        "/usr/bin/echo",
				Args:       []string{"test"},
				Privileged: false,
			},
			privilegeSupported: false, // Should not matter
			expectError:        false,
			noPrivilegeManager: false,
			expectElevations:   []string{}, // No elevations expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPrivMgr := privtesting.NewMockPrivilegeManager(tt.privilegeSupported)

			var exec executor.CommandExecutor
			if tt.noPrivilegeManager {
				// Create executor without privilege manager
				exec = executor.NewDefaultExecutor()
			} else {
				exec = executor.NewDefaultExecutor(
					executor.WithPrivilegeManager(mockPrivMgr),
				)
			}

			ctx := context.Background()
			envVars := map[string]string{"PATH": "/usr/bin"}

			result, err := exec.Execute(ctx, tt.cmd, envVars)

			if tt.expectError {
				assert.Error(t, err)
				// Check based on expected error type
				if tt.expectedErrorType != nil {
					assert.ErrorIs(t, err, tt.expectedErrorType)
				} else if tt.errorMessage != "" {
					// Fall back to message check only if no error type is specified
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			if !tt.noPrivilegeManager {
				if len(tt.expectElevations) == 0 && mockPrivMgr.ElevationCalls == nil {
					// Both nil and empty slice are acceptable for no elevations - no assertion needed
					assert.True(t, true, "No elevations expected and none occurred")
				} else {
					assert.Equal(t, tt.expectElevations, mockPrivMgr.ElevationCalls)
				}
			}
		})
	}
}

func TestDefaultExecutor_PrivilegeElevationFailure(t *testing.T) {
	mockPrivMgr := privtesting.NewFailingMockPrivilegeManager(true)

	exec := executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(mockPrivMgr),
	)

	cmd := runnertypes.Command{
		Name:       "test_fail",
		Cmd:        "/bin/echo", // Use absolute path to pass validation
		Args:       []string{"test"},
		Privileged: true,
	}

	ctx := context.Background()
	envVars := map[string]string{"PATH": "/usr/bin"}

	result, err := exec.Execute(ctx, cmd, envVars)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, privtesting.ErrMockPrivilegeElevationFailed))
}

func TestDefaultExecutor_BackwardCompatibility(t *testing.T) {
	// Test that existing code without privilege manager still works
	exec := executor.NewDefaultExecutor()

	cmd := runnertypes.Command{
		Name:       "test_normal",
		Cmd:        "echo",
		Args:       []string{"normal"},
		Privileged: false, // Normal command
	}

	ctx := context.Background()
	envVars := map[string]string{"PATH": "/usr/bin"}

	result, err := exec.Execute(ctx, cmd, envVars)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "normal")
}

// TestPrivilegedCommandValidation_PathRequirements tests the additional security validations for privileged commands
func TestPrivilegedCommandValidation_PathRequirements(t *testing.T) {
	tests := []struct {
		name          string
		cmd           runnertypes.Command
		expectError   bool
		errorContains string
	}{
		{
			name: "relative path fails for privileged command",
			cmd: runnertypes.Command{
				Name:       "test_relative_path",
				Cmd:        "echo", // Relative path
				Args:       []string{"test"},
				Privileged: true,
			},
			expectError:   true,
			errorContains: "privileged commands must use absolute paths",
		},
		{
			name: "absolute path works for privileged command",
			cmd: runnertypes.Command{
				Name:       "test_absolute_path",
				Cmd:        "/usr/bin/echo", // Absolute path
				Args:       []string{"test"},
				Privileged: true,
			},
			expectError: false,
		},
		{
			name: "relative working directory fails for privileged command",
			cmd: runnertypes.Command{
				Name:       "test_relative_dir",
				Cmd:        "/usr/bin/echo",
				Args:       []string{"test"},
				Dir:        "tmp", // Relative working directory
				Privileged: true,
			},
			expectError:   true,
			errorContains: "directory does not exist", // Basic validation fails first
		},
		{
			name: "absolute working directory works for privileged command",
			cmd: runnertypes.Command{
				Name:       "test_absolute_dir",
				Cmd:        "/usr/bin/echo",
				Args:       []string{"test"},
				Dir:        "/tmp", // Absolute working directory
				Privileged: true,
			},
			expectError: false,
		},
		// Relative path component checking is done by the Validate method,
		// so it has been removed from validatePrivilegedCommand
		{
			name: "path with . components fails in standard validation",
			cmd: runnertypes.Command{
				Name:       "test_path_with_dots",
				Cmd:        "/usr/bin/../bin/echo", // Absolute path but contains relative path components
				Args:       []string{"test"},
				Privileged: true,
			},
			expectError:   true,
			errorContains: "command path contains relative path components", // Error message from standard validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock filesystem for directory validation
			mockFS := &mockFileSystem{
				existingPaths: map[string]bool{
					"/tmp": true,
				},
			}
			mockPrivMgr := privtesting.NewMockPrivilegeManager(true)

			exec := executor.NewDefaultExecutor(
				executor.WithPrivilegeManager(mockPrivMgr),
				executor.WithFileSystem(mockFS),
			)

			ctx := context.Background()
			envVars := map[string]string{"PATH": "/usr/bin"}

			_, err := exec.Execute(ctx, tt.cmd, envVars)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
