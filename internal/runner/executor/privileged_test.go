package executor_test

import (
	"context"
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
		expectElevations   []string
	}{
		{
			name: "privileged command executes with elevation",
			cmd: runnertypes.Command{
				Name:       "test_privileged",
				Cmd:        "echo",
				Args:       []string{"test"},
				Privileged: true,
			},
			privilegeSupported: true,
			expectError:        false,
			expectElevations:   []string{"file_access", "command_execution"},
		},
		{
			name: "privileged command fails when not supported",
			cmd: runnertypes.Command{
				Name:       "test_privileged",
				Cmd:        "echo",
				Args:       []string{"test"},
				Privileged: true,
			},
			privilegeSupported: false,
			expectError:        true,
			errorMessage:       "privileged execution not supported",
		},
		{
			name: "privileged command fails with no manager",
			cmd: runnertypes.Command{
				Name:       "test_privileged",
				Cmd:        "echo",
				Args:       []string{"test"},
				Privileged: true,
			},
			privilegeSupported: true,
			expectError:        true,
			errorMessage:       "no privilege manager available",
		},
		{
			name: "normal command bypasses privilege manager",
			cmd: runnertypes.Command{
				Name:       "test_normal",
				Cmd:        "echo",
				Args:       []string{"test"},
				Privileged: false,
			},
			privilegeSupported: false, // Should not matter
			expectError:        false,
			expectElevations:   []string{}, // No elevations expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPrivMgr := privtesting.NewMockPrivilegeManager(tt.privilegeSupported)

			var exec executor.CommandExecutor
			if tt.name == "privileged command fails with no manager" {
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
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			if tt.name != "privileged command fails with no manager" {
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
		Cmd:        "echo",
		Args:       []string{"test"},
		Privileged: true,
	}

	ctx := context.Background()
	envVars := map[string]string{"PATH": "/usr/bin"}

	result, err := exec.Execute(ctx, cmd, envVars)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "mock privilege elevation failure")
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
