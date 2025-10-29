//go:build !windows

package privilege

import (
	"log/slog"
	"os/user"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
}
