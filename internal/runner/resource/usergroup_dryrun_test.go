//go:build !windows

package resource

import (
	"context"
	"testing"

	privilegetesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockExecutor and related types are defined in normal_manager_test.go

func TestDryRunResourceManager_UserGroupValidation(t *testing.T) {
	t.Run("valid_user_group_specification", func(t *testing.T) {
		mockExec := &MockExecutor{}
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		mockPathResolver := &MockPathResolver{}
		mockPathResolver.On("ResolvePath", "echo").Return("/usr/bin/echo", nil)

		manager := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{})

		cmd := runnertypes.Command{
			Name:       "test_user_group",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		group := &runnertypes.CommandGroup{
			Name:        "test_group",
			Description: "Test group",
		}

		result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// Check that user/group validation was called
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_dry_run:testuser:testgroup")

		// Check analysis contains user/group information
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Equal(t, "testuser", analysis.Parameters["run_as_user"])
		assert.Equal(t, "testgroup", analysis.Parameters["run_as_group"])
		assert.Contains(t, analysis.Impact.Description, "[INFO: User/Group configuration validated]")
	})

	t.Run("invalid_user_group_specification", func(t *testing.T) {
		mockExec := &MockExecutor{}
		mockPriv := privilegetesting.NewFailingMockPrivilegeManager(true) // Will fail user/group validation

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{})

		cmd := runnertypes.Command{
			Name:       "test_invalid_user_group",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "nonexistent_user",
			RunAsGroup: "nonexistent_group",
		}

		group := &runnertypes.CommandGroup{
			Name:        "test_group",
			Description: "Test group",
		}

		result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err) // Dry-run should not fail, but report issues
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// Check analysis contains error information
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Equal(t, "nonexistent_user", analysis.Parameters["run_as_user"])
		assert.Equal(t, "nonexistent_group", analysis.Parameters["run_as_group"])
		assert.Contains(t, analysis.Impact.Description, "[ERROR: User/Group validation failed:")
		assert.Equal(t, riskLevelHigh, analysis.Impact.SecurityRisk)
	})

	t.Run("user_group_not_supported", func(t *testing.T) {
		mockExec := &MockExecutor{}
		mockPriv := privilegetesting.NewMockPrivilegeManager(false) // Not supported

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{})

		cmd := runnertypes.Command{
			Name:       "test_user_group_unsupported",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		group := &runnertypes.CommandGroup{
			Name:        "test_group",
			Description: "Test group",
		}

		result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// Check analysis contains warning
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Contains(t, analysis.Impact.Description, "[WARNING: User/Group privilege management not supported]")
	})

	t.Run("no_privilege_manager", func(t *testing.T) {
		mockExec := &MockExecutor{}
		// No privilege manager provided

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager := NewDryRunResourceManager(mockExec, nil, mockPathResolver, &DryRunOptions{})

		cmd := runnertypes.Command{
			Name:       "test_no_privmgr",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		group := &runnertypes.CommandGroup{
			Name:        "test_group",
			Description: "Test group",
		}

		result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// Check analysis contains warning
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Contains(t, analysis.Impact.Description, "[WARNING: User/Group privilege management not supported]")
	})

	t.Run("only_user_specified", func(t *testing.T) {
		mockExec := &MockExecutor{}
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{})

		cmd := runnertypes.Command{
			Name:      "test_user_only",
			Cmd:       "echo",
			Args:      []string{"test"},
			RunAsUser: "testuser",
			// RunAsGroup is empty
		}

		group := &runnertypes.CommandGroup{
			Name:        "test_group",
			Description: "Test group",
		}

		result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// Check that dry-run validation was called with empty group
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_dry_run:testuser:")

		// Check analysis
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Equal(t, "testuser", analysis.Parameters["run_as_user"])
		assert.Equal(t, "", analysis.Parameters["run_as_group"])
		assert.Contains(t, analysis.Impact.Description, "[INFO: User/Group configuration validated]")
	})

	t.Run("no_user_group_specification", func(t *testing.T) {
		mockExec := &MockExecutor{}
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{})

		cmd := runnertypes.Command{
			Name: "test_no_user_group",
			Cmd:  "echo",
			Args: []string{"test"},
			// No RunAsUser or RunAsGroup
		}

		group := &runnertypes.CommandGroup{
			Name:        "test_group",
			Description: "Test group",
		}

		result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// Should not call user/group validation
		assert.Empty(t, mockPriv.ElevationCalls)

		// Check analysis does not contain user/group info
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		_, hasRunAsUser := analysis.Parameters["run_as_user"]
		_, hasRunAsGroup := analysis.Parameters["run_as_group"]
		assert.False(t, hasRunAsUser)
		assert.False(t, hasRunAsGroup)
		assert.NotContains(t, analysis.Impact.Description, "User/Group")
	})
}
