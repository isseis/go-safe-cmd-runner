package privilege

import (
	"log/slog"
	"os/user"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

const fallbackUser = "root"

func TestManager_Interface(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	// Test interface implementation
	assert.NotNil(t, manager)
	assert.Implements(t, (*Manager)(nil), manager)
}

func TestElevationContext(t *testing.T) {
	ctx := runnertypes.ElevationContext{
		Operation:   runnertypes.OperationHealthCheck,
		CommandName: "test",
		FilePath:    "/test/path",
		OriginalUID: 1000,
		TargetUID:   0,
	}

	assert.Equal(t, runnertypes.OperationHealthCheck, ctx.Operation)
	assert.Equal(t, "test", ctx.CommandName)
	assert.Equal(t, "/test/path", ctx.FilePath)
	assert.Equal(t, 1000, ctx.OriginalUID)
	assert.Equal(t, 0, ctx.TargetUID)
}

func TestPrivilegeError(t *testing.T) {
	err := &Error{
		Operation:   runnertypes.OperationCommandExecution,
		CommandName: "test_cmd",
		OriginalUID: 1000,
		TargetUID:   0,
		SyscallErr:  ErrPrivilegeElevationFailed,
		Timestamp:   time.Now(),
	}

	expectedMsg := "privilege operation 'command_execution' failed for command 'test_cmd' (uid 1000->0): failed to elevate privileges"
	assert.Equal(t, expectedMsg, err.Error())
	assert.Equal(t, ErrPrivilegeElevationFailed, err.Unwrap())
}

func TestOperationConstants(t *testing.T) {
	operations := []runnertypes.Operation{
		runnertypes.OperationFileHashCalculation,
		runnertypes.OperationCommandExecution,
		runnertypes.OperationFileAccess,
		runnertypes.OperationHealthCheck,
	}

	expected := []string{
		"file_hash_calculation",
		"command_execution",
		"file_access",
		"health_check",
	}

	for i, op := range operations {
		assert.Equal(t, expected[i], string(op))
	}
}

func TestManager_BasicFunctionality(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	// Test GetOriginalUID returns reasonable value
	originalUID := manager.GetOriginalUID()
	assert.GreaterOrEqual(t, originalUID, -1) // -1 for Windows, >= 0 for Unix

	// Test GetCurrentUID returns reasonable value
	currentUID := manager.GetCurrentUID()
	assert.GreaterOrEqual(t, currentUID, -1) // -1 for Windows, >= 0 for Unix
}

func TestManager_WithPrivileges_UnsupportedPlatform(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	// This test assumes we're running without setuid in normal test environment
	if manager.IsPrivilegedExecutionSupported() {
		t.Skip("Test environment has privileged execution enabled")
	}

	elevationCtx := runnertypes.ElevationContext{
		Operation:   runnertypes.OperationHealthCheck,
		CommandName: "test",
	}

	err := manager.WithPrivileges(elevationCtx, func() error {
		return nil
	})

	// Should fail because setuid is not configured in test environment
	assert.Error(t, err)
}

func TestManager_WithUserGroup_ValidUser(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	// Test with current user (should work in dry-run mode)
	currentUser := getCurrentUser(t)
	currentGroup := getCurrentGroup(t)

	t.Run("dry_run_mode", func(t *testing.T) {
		var executed bool
		err := manager.WithUserGroupDryRun(currentUser, currentGroup, func() error {
			executed = true
			return nil
		})

		assert.NoError(t, err)
		assert.True(t, executed)
	})

	// Only test actual user/group change if running as root
	if manager.GetCurrentUID() == 0 {
		t.Run("actual_change", func(t *testing.T) {
			var executed bool
			err := manager.WithUserGroup(currentUser, currentGroup, func() error {
				executed = true
				return nil
			})

			assert.NoError(t, err)
			assert.True(t, executed)
		})
	}
}

func TestManager_WithUserGroup_InvalidUser(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	t.Run("invalid_user", func(t *testing.T) {
		err := manager.WithUserGroupDryRun("nonexistent_user_12345", "", func() error {
			return nil
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to lookup user")
	})

	t.Run("invalid_group", func(t *testing.T) {
		err := manager.WithUserGroupDryRun("", "nonexistent_group_12345", func() error {
			return nil
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to lookup group")
	})
}

func TestManager_WithUserGroup_EmptyUserGroup(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	t.Run("empty_user_and_group", func(t *testing.T) {
		var executed bool
		err := manager.WithUserGroupDryRun("", "", func() error {
			executed = true
			return nil
		})

		// Should succeed with empty user/group (uses current user/group)
		assert.NoError(t, err)
		assert.True(t, executed)
	})
}

func TestManager_WithUserGroup_FunctionError(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	currentUser := getCurrentUser(t)

	expectedErr := assert.AnError
	err := manager.WithUserGroupDryRun(currentUser, "", func() error {
		return expectedErr
	})

	// Should return the function error
	assert.Equal(t, expectedErr, err)
}

func TestManager_UserGroupSupported(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	// Should match the privileged execution support status
	expected := manager.IsPrivilegedExecutionSupported()
	actual := manager.IsUserGroupSupported()

	assert.Equal(t, expected, actual)
}

// Helper functions for tests
func getCurrentUser(t *testing.T) string {
	t.Helper()

	// Try to get current user, fallback to "root" if we can't determine
	user, err := user.Current()
	if err != nil {
		t.Logf("Warning: Could not get current user: %v", err)
		return fallbackUser // Fallback for tests
	}
	return user.Username
}

func getCurrentGroup(t *testing.T) string {
	t.Helper()

	// Try to get current user's primary group
	currentUser, err := user.Current()
	if err != nil {
		t.Logf("Warning: Could not get current user: %v", err)
		return fallbackUser // Fallback for tests
	}

	group, err := user.LookupGroupId(currentUser.Gid)
	if err != nil {
		t.Logf("Warning: Could not get primary group: %v", err)
		return fallbackUser // Fallback for tests
	}
	return group.Name
}
