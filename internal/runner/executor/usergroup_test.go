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
	"github.com/stretchr/testify/assert"
)

// Use common mock implementations from testing.go

func TestDefaultExecutor_ExecuteUserGroupPrivileges(t *testing.T) {
	t.Run("user_group_execution_success", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_user_group",
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that user/group privilege escalation was called
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
	})

	t.Run("user_group_no_privilege_manager", func(t *testing.T) {
		// Create executor without privilege manager
		exec := executor.NewDefaultExecutor(
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_user_group",
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no privilege manager available")
	})

	t.Run("user_group_not_supported", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(false) // Not supported
		exec := executor.NewDefaultExecutor(
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_user_group",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege changes are not supported")
	})

	t.Run("user_group_privilege_execution_fails", func(t *testing.T) {
		mockPriv := privilegetesting.NewFailingMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_user_group",
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsUser:  "invaliduser",
			RunAsGroup: "invalidgroup",
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege execution failed")
	})

	t.Run("only_user_specified", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:      "test_user_only",
			Cmd:       "/bin/echo",
			Args:      []string{"test"},
			RunAsUser: "testuser",
			// RunAsGroup is empty
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that user/group privilege escalation was called with empty group
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:")
	})

	t.Run("only_group_specified", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_group_only",
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			RunAsGroup: "testgroup",
			// RunAsUser is empty
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that user/group privilege escalation was called with empty user
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change::testgroup")
	})
}

func TestDefaultExecutor_Execute_Integration(t *testing.T) {
	t.Run("privileged_with_user_group_both_specified", func(t *testing.T) {
		// Test case where both Privileged=true and user/group are specified
		// User/group should take precedence
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_both",
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			Privileged: true,       // Also privileged
			RunAsUser:  "testuser", // But user/group specified
			RunAsGroup: "testgroup",
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should use user/group execution, not privileged execution
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
		assert.NotContains(t, mockPriv.ElevationCalls, "command_execution")
	})

	t.Run("normal_execution_no_privileges", func(t *testing.T) {
		// Test case where neither Privileged nor user/group are specified
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executor.MockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name: "test_normal",
			Cmd:  "echo",
			Args: []string{"test"},
			// No privileged, no user/group
		}

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Should not call any privilege methods
		assert.Empty(t, mockPriv.ElevationCalls)
	})
}

// TestUserGroupCommandValidation_PathRequirements tests the additional security validations for user/group commands
func TestUserGroupCommandValidation_PathRequirements(t *testing.T) {
	t.Skip("Skipping user/group path requirement tests for now, as relative path component checking is done by the Validate method")
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
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
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

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

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
			executor.WithOutputWriter(&executor.MockOutputWriter{}),
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

		result, err := exec.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)
		// No assertions about logging since no logger is provided
	})
}
