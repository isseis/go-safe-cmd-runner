package executor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// Test error definitions
var (
	ErrMockPrivilegeElevationFailed = errors.New("mock privilege elevation failure")
)

// MockPrivilegeManager for testing
type MockPrivilegeManager struct {
	supported      bool
	elevationCalls []string
	shouldFail     bool
}

func (m *MockPrivilegeManager) WithPrivileges(_ context.Context, elevationCtx privilege.ElevationContext, fn func() error) error {
	m.elevationCalls = append(m.elevationCalls, string(elevationCtx.Operation))
	if m.shouldFail {
		return ErrMockPrivilegeElevationFailed
	}
	return fn()
}

func (m *MockPrivilegeManager) IsPrivilegedExecutionSupported() bool {
	return m.supported
}

func (m *MockPrivilegeManager) GetCurrentUID() int {
	return 1000
}

func (m *MockPrivilegeManager) GetOriginalUID() int {
	return 1000
}

func (m *MockPrivilegeManager) HealthCheck(_ context.Context) error {
	if !m.supported {
		return privilege.ErrPrivilegedExecutionNotAvailable
	}
	return nil
}

func (m *MockPrivilegeManager) GetHealthStatus(_ context.Context) privilege.HealthStatus {
	return privilege.HealthStatus{
		IsSupported:      m.supported,
		SetuidConfigured: m.supported,
		OriginalUID:      1000,
		CurrentUID:       1000,
		EffectiveUID:     1000,
		CanElevate:       m.supported,
	}
}

func (m *MockPrivilegeManager) GetMetrics() privilege.Metrics {
	return privilege.Metrics{}
}

func TestDefaultExecutor_WithPrivilegeManager(t *testing.T) {
	mockPrivMgr := &MockPrivilegeManager{supported: true}

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
			mockPrivMgr := &MockPrivilegeManager{
				supported: tt.privilegeSupported,
			}

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
				if len(tt.expectElevations) == 0 && mockPrivMgr.elevationCalls == nil {
					// Both nil and empty slice are acceptable for no elevations - no assertion needed
					assert.True(t, true, "No elevations expected and none occurred")
				} else {
					assert.Equal(t, tt.expectElevations, mockPrivMgr.elevationCalls)
				}
			}
		})
	}
}

func TestDefaultExecutor_PrivilegeElevationFailure(t *testing.T) {
	mockPrivMgr := &MockPrivilegeManager{
		supported:  true,
		shouldFail: true,
	}

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
