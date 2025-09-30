//go:build !windows

package executor_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	privilegetesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/testhelpers"
	"github.com/stretchr/testify/assert"
)

// Use common mock implementations from testing.go

func TestDefaultExecutor_ExecuteUserGroupPrivileges(t *testing.T) {
	t.Run("user_group_execution_success", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}
		testhelpers.PrepareCommand(&cmd)

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that user/group privilege escalation was called
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
	})

	t.Run("user_group_no_privilege_manager", func(t *testing.T) {
		// Create executor without privilege manager
		exec := executor.NewDefaultExecutor(
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no privilege manager available")
	})

	t.Run("user_group_not_supported", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(false) // Not supported
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege changes are not supported")
	})

	t.Run("user_group_privilege_execution_fails", func(t *testing.T) {
		mockPriv := privilegetesting.NewFailingMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsUser:  "invaliduser",
			RunAsGroup: "invalidgroup",
		}
		testhelpers.PrepareCommand(&cmd)

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege execution failed")
	})

	t.Run("only_user_specified", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Cmd:       "/bin/echo",
			Args:      []string{"test"},
			RunAsUser: "testuser",
			// RunAsGroup is empty
		}
		testhelpers.PrepareCommand(&cmd)

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that user/group privilege escalation was called with empty group
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:")
	})

	t.Run("only_group_specified", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsGroup: "testgroup",
			// RunAsUser is empty
		}
		testhelpers.PrepareCommand(&cmd)

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that user/group privilege escalation was called with empty user
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change::testgroup")
	})
}

func TestDefaultExecutor_Execute_Integration(t *testing.T) {
	t.Run("privileged_with_user_group_both_specified", func(t *testing.T) {
		// Test case where user/group are specified
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser", // But user/group specified
			RunAsGroup: "testgroup",
		}
		testhelpers.PrepareCommand(&cmd)

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should use user/group execution, not privileged execution
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
		assert.NotContains(t, mockPriv.ElevationCalls, "command_execution")
	})

	t.Run("normal_execution_no_privileges", func(t *testing.T) {
		// Test case where user/group are not specified
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Cmd:  "echo",
			Args: []string{"test"},
			// No privileged, no user/group
		}
		testhelpers.PrepareCommand(&cmd)

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Should not call any privilege methods
		assert.Empty(t, mockPriv.ElevationCalls)
	})
}

// TestUserGroupCommandValidation_PathRequirements tests the basic validation for user/group commands
func TestUserGroupCommandValidation_PathRequirements(t *testing.T) {
	tests := []struct {
		name          string
		cmd           runnertypes.Command
		expectError   bool
		errorContains string
	}{
		{
			name: "valid absolute path works for user/group command",
			cmd: runnertypes.Command{
				Cmd:        "/bin/echo", // Absolute path
				Args:       []string{"test"},
				RunAsUser:  "testuser",
				RunAsGroup: "testgroup",
			},
			expectError: false,
		},
		{
			name: "relative working directory fails for user/group command",
			cmd: runnertypes.Command{
				Cmd:        "/bin/echo",
				Args:       []string{"test"},
				Dir:        "tmp", // Relative working directory
				RunAsUser:  "testuser",
				RunAsGroup: "testgroup",
			},
			expectError:   true,
			errorContains: "directory does not exist", // Basic validation fails first
		},
		{
			name: "absolute working directory works for user/group command",
			cmd: runnertypes.Command{
				Cmd:        "/bin/echo",
				Args:       []string{"test"},
				Dir:        "/tmp", // Absolute working directory
				RunAsUser:  "testuser",
				RunAsGroup: "testgroup",
			},
			expectError: false,
		},
		{
			name: "path with relative components fails in standard validation",
			cmd: runnertypes.Command{
				Cmd:        "/bin/../bin/echo", // Absolute path but contains relative path components
				Args:       []string{"test"},
				RunAsUser:  "testuser",
				RunAsGroup: "testgroup",
			},
			expectError:   true,
			errorContains: "command path contains relative path components", // Error message from standard validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock filesystem for directory validation
			mockFS := &executor.MockFileSystem{
				ExistingPaths: map[string]bool{
					"/tmp": true,
				},
			}
			mockPrivMgr := privilegetesting.NewMockPrivilegeManager(true)

			exec := executor.NewDefaultExecutor(
				executor.WithPrivilegeManager(mockPrivMgr),
				executor.WithFileSystem(mockFS),
			)

			ctx := context.Background()
			envVars := map[string]string{"PATH": "/usr/bin"}

			// Prepare command to set ExpandedCmd and ExpandedArgs
			testhelpers.PrepareCommand(&tt.cmd)

			_, err := exec.Execute(ctx, tt.cmd, envVars, nil)

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

// TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging tests audit logging for user/group command execution
func TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging(t *testing.T) {
	t.Run("audit_logging_successful_execution", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)

		// Create a real audit logger with custom slog handler to capture logs
		var logBuffer bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
		auditLogger := audit.NewAuditLoggerWithCustom(logger)

		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
			executor.WithAuditLogger(auditLogger),
		)

		cmd := runnertypes.Command{
			Name:       "test_audit_user_group",
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}
		testhelpers.PrepareCommand(&cmd)

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify audit logging was called
		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "user_group_execution")
		assert.Contains(t, logOutput, "test_audit_user_group")
		assert.Contains(t, logOutput, "testuser")
		assert.Contains(t, logOutput, "testgroup")
	})

	t.Run("no_audit_logging_when_logger_nil", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)

		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
			// No audit logger provided
		)

		cmd := runnertypes.Command{
			Name:       "test_no_audit",
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}
		testhelpers.PrepareCommand(&cmd)

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)
		// No assertions about logging since no logger is provided
	})
}

// TestDefaultExecutor_UserGroupPrivilegeElevationFailure tests privilege elevation failure scenarios
// This replaces the deleted TestDefaultExecutor_PrivilegeElevationFailure test for user/group commands
func TestDefaultExecutor_UserGroupPrivilegeElevationFailure(t *testing.T) {
	mockPrivMgr := privilegetesting.NewFailingMockPrivilegeManager(true)

	exec := executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(mockPrivMgr),
		executor.WithFileSystem(&executor.MockFileSystem{}),
	)

	cmd := runnertypes.Command{
		Cmd:        "/bin/echo", // Use absolute path to pass validation
		Args:       []string{"test"},
		RunAsUser:  "root", // Use run_as_user instead of privileged=true
		RunAsGroup: "wheel",
	}
	testhelpers.PrepareCommand(&cmd)

	ctx := context.Background()
	envVars := map[string]string{"PATH": "/usr/bin"}

	result, err := exec.Execute(ctx, cmd, envVars, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, privilegetesting.ErrMockPrivilegeElevationFailed)
}

// TestDefaultExecutor_UserGroupBackwardCompatibility tests backward compatibility with non-privileged commands
// This replaces the deleted TestDefaultExecutor_BackwardCompatibility test
func TestDefaultExecutor_UserGroupBackwardCompatibility(t *testing.T) {
	// Test that existing code without privilege manager still works for normal commands
	exec := executor.NewDefaultExecutor(
		executor.WithFileSystem(&executor.MockFileSystem{}),
	)

	cmd := runnertypes.Command{
		Cmd:  "echo",
		Args: []string{"normal"},
		// No run_as_user/run_as_group specified - normal command
	}
	testhelpers.PrepareCommand(&cmd)

	ctx := context.Background()
	envVars := map[string]string{"PATH": "/usr/bin"}

	result, err := exec.Execute(ctx, cmd, envVars, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "normal")
}

// TestDefaultExecutor_UserGroupRootExecution tests running commands as root user
// This provides equivalent functionality to the deleted privileged=true tests
func TestDefaultExecutor_UserGroupRootExecution(t *testing.T) {
	tests := []struct {
		name               string
		cmd                runnertypes.Command
		privilegeSupported bool
		expectError        bool
		errorMessage       string
		expectedErrorType  error
		noPrivilegeManager bool
		expectElevations   []string
	}{
		{
			name: "root user command executes with elevation",
			cmd: runnertypes.Command{
				Cmd:       "/usr/bin/whoami",
				Args:      []string{},
				RunAsUser: "root",
			},
			privilegeSupported: true,
			expectError:        false,
			noPrivilegeManager: false,
			expectElevations:   []string{"user_group_change:root:"},
		},
		{
			name: "root user command fails when not supported",
			cmd: runnertypes.Command{
				Cmd:       "/usr/bin/whoami",
				Args:      []string{},
				RunAsUser: "root",
			},
			privilegeSupported: false,
			expectError:        true,
			errorMessage:       "user/group privilege changes are not supported",
			expectedErrorType:  executor.ErrUserGroupPrivilegeUnsupported,
			noPrivilegeManager: false,
		},
		{
			name: "root user command fails with no manager",
			cmd: runnertypes.Command{
				Cmd:       "/usr/bin/whoami",
				Args:      []string{},
				RunAsUser: "root",
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
				Cmd:  "/bin/echo",
				Args: []string{"test"},
				// No run_as_user specified
			},
			privilegeSupported: false, // Should not matter
			expectError:        false,
			noPrivilegeManager: false,
			expectElevations:   []string{}, // No elevations expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPrivMgr := privilegetesting.NewMockPrivilegeManager(tt.privilegeSupported)

			var exec executor.CommandExecutor
			if tt.noPrivilegeManager {
				// Create executor without privilege manager
				exec = executor.NewDefaultExecutor(
					executor.WithFileSystem(&executor.MockFileSystem{}),
				)
			} else {
				exec = executor.NewDefaultExecutor(
					executor.WithPrivilegeManager(mockPrivMgr),
					executor.WithFileSystem(&executor.MockFileSystem{}),
				)
			}

			ctx := context.Background()
			envVars := map[string]string{"PATH": "/usr/bin"}

			// Prepare command to set ExpandedCmd and ExpandedArgs
			testhelpers.PrepareCommand(&tt.cmd)

			result, err := exec.Execute(ctx, tt.cmd, envVars, nil)

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
