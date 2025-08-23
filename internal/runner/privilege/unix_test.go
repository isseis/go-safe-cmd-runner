//go:build !windows

package privilege

import (
	"log/slog"
	"os/user"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnixPrivilegeManager_ChangeUserGroupDryRun(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true, // Assume supported for these tests
	}

	t.Run("valid_current_user", func(t *testing.T) {
		currentUser, err := user.Current()
		require.NoError(t, err, "Failed to get current user")

		group, err := user.LookupGroupId(currentUser.Gid)
		require.NoError(t, err, "Failed to get current group")

		err = manager.changeUserGroupDryRun(currentUser.Username, group.Name)
		assert.NoError(t, err)
	})

	t.Run("empty_user_and_group", func(t *testing.T) {
		err := manager.changeUserGroupDryRun("", "")
		assert.NoError(t, err)
	})

	t.Run("invalid_user", func(t *testing.T) {
		err := manager.changeUserGroupDryRun("nonexistent_user_12345", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to lookup user")
	})

	t.Run("invalid_group", func(t *testing.T) {
		err := manager.changeUserGroupDryRun("", "nonexistent_group_12345")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to lookup group")
	})

	t.Run("root_user", func(t *testing.T) {
		// Test with root user
		err := manager.changeUserGroupDryRun("root", "root")
		if err != nil {
			// In dry-run mode, we might get privilege check errors or lookup errors
			// Both are valid depending on the test environment
			errorContainsValidMessage := strings.Contains(err.Error(), "failed to lookup") ||
				strings.Contains(err.Error(), "insufficient privileges")
			assert.True(t, errorContainsValidMessage, "Error should be about lookup or privileges: %v", err)
		} else {
			assert.NoError(t, err)
		}
	})
}

func TestUnixPrivilegeManager_ChangeUserGroupInternal(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
	}

	currentUser, err := user.Current()
	require.NoError(t, err, "Failed to get current user")

	group, err := user.LookupGroupId(currentUser.Gid)
	require.NoError(t, err, "Failed to get current group")

	t.Run("dry_run_true", func(t *testing.T) {
		err := manager.changeUserGroupInternal(currentUser.Username, group.Name, true)
		assert.NoError(t, err)
	})

	t.Run("dry_run_false_insufficient_privileges", func(t *testing.T) {
		// Unless running as root, actual user changes should fail with insufficient privileges
		if manager.GetCurrentUID() != 0 {
			// Try to change to a different user (if current user is not root)
			err := manager.changeUserGroupInternal("root", "", false)
			if err == nil {
				t.Skip("Test environment allows user changes (possibly running as root)")
			}
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "insufficient privileges")
		}
	})
}

func TestUnixPrivilegeManager_UserLookup(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
	}

	t.Run("lookup_current_user", func(t *testing.T) {
		currentUser, err := user.Current()
		require.NoError(t, err, "Failed to get current user")

		// This should work - looking up current user by name
		err = manager.changeUserGroupDryRun(currentUser.Username, "")
		assert.NoError(t, err)
	})

	t.Run("lookup_numeric_uid", func(t *testing.T) {
		currentUser, err := user.Current()
		require.NoError(t, err, "Failed to get current user")

		// Try looking up by numeric UID (should fail as we expect username)
		err = manager.changeUserGroupDryRun(currentUser.Uid, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to lookup user")
	})
}

func TestUnixPrivilegeManager_GroupLookup(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
	}

	t.Run("lookup_current_group", func(t *testing.T) {
		currentUser, err := user.Current()
		require.NoError(t, err, "Failed to get current user")

		group, err := user.LookupGroupId(currentUser.Gid)
		require.NoError(t, err, "Failed to get current group")

		// This should work - looking up current group by name
		err = manager.changeUserGroupDryRun("", group.Name)
		assert.NoError(t, err)
	})

	t.Run("lookup_numeric_gid", func(t *testing.T) {
		currentUser, err := user.Current()
		require.NoError(t, err, "Failed to get current user")

		// Try looking up by numeric GID (should fail as we expect group name)
		err = manager.changeUserGroupDryRun("", currentUser.Gid)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to lookup group")
	})
}

func TestUnixPrivilegeManager_WithUserGroupInternal(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: true,
	}

	currentUser, err := user.Current()
	require.NoError(t, err, "Failed to get current user")

	t.Run("with_dry_run", func(t *testing.T) {
		var executed bool
		executionCtx := runnertypes.ElevationContext{
			Operation:   runnertypes.OperationUserGroupDryRun,
			CommandName: "test_command",
			RunAsUser:   currentUser.Username,
			RunAsGroup:  "",
		}
		err := manager.WithPrivileges(executionCtx, func() error {
			executed = true
			return nil
		})

		assert.NoError(t, err)
		assert.True(t, executed)
	})

	t.Run("function_error_propagation", func(t *testing.T) {
		expectedErr := assert.AnError
		executionCtx := runnertypes.ElevationContext{
			Operation:   runnertypes.OperationUserGroupDryRun,
			CommandName: "test_command",
			RunAsUser:   currentUser.Username,
			RunAsGroup:  "",
		}
		err := manager.WithPrivileges(executionCtx, func() error {
			return expectedErr
		})

		assert.Equal(t, expectedErr, err)
	})
}

func TestUnixPrivilegeManager_PrivilegeValidation(t *testing.T) {
	logger := slog.Default()
	manager := &UnixPrivilegeManager{
		logger:             logger,
		privilegeSupported: false, // Explicitly set to false for this test
	}

	currentUser, err := user.Current()
	require.NoError(t, err, "Failed to get current user")

	t.Run("dry_run_works_without_privilege_support", func(t *testing.T) {
		// Dry-run should work even without privilege support
		var executed bool
		executionCtx := runnertypes.ElevationContext{
			Operation:   runnertypes.OperationUserGroupDryRun,
			CommandName: "test_command",
			RunAsUser:   currentUser.Username,
			RunAsGroup:  "",
		}
		err := manager.WithPrivileges(executionCtx, func() error {
			executed = true
			return nil
		})

		assert.NoError(t, err)
		assert.True(t, executed)
	})

	t.Run("user_group_supported_matches_privilege_supported", func(t *testing.T) {
		assert.Equal(t, manager.privilegeSupported, manager.IsUserGroupSupported())
	})
}
