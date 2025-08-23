//go:build !windows

package executor

import (
	"context"
	"os"
	"testing"

	privilegetesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// mockOutputWriter implements OutputWriter for testing
type mockOutputWriter struct{}

func (m *mockOutputWriter) Write(_ string, _ []byte) error {
	return nil
}

func (m *mockOutputWriter) Close() error {
	return nil
}

// mockFileSystem implements FileSystem for testing
type mockFileSystem struct{}

func (m *mockFileSystem) CreateTempDir(dir, prefix string) (string, error) {
	return os.MkdirTemp(dir, prefix)
}

func (m *mockFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (m *mockFileSystem) FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func TestDefaultExecutor_ExecuteWithUserGroup(t *testing.T) {
	t.Run("user_group_execution_success", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		executor := NewDefaultExecutor(
			WithOutputWriter(&mockOutputWriter{}),
			WithPrivilegeManager(mockPriv),
			WithFileSystem(&mockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_user_group",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		result, err := executor.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that WithUserGroup was called
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
	})

	t.Run("user_group_no_privilege_manager", func(t *testing.T) {
		// Create executor without privilege manager
		executor := NewDefaultExecutor(
			WithOutputWriter(&mockOutputWriter{}),
			WithFileSystem(&mockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_user_group",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		result, err := executor.Execute(context.Background(), cmd, map[string]string{})

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no privilege manager available")
	})

	t.Run("user_group_not_supported", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(false) // Not supported
		executor := NewDefaultExecutor(
			WithOutputWriter(&mockOutputWriter{}),
			WithPrivilegeManager(mockPriv),
			WithFileSystem(&mockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_user_group",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "testuser",
			RunAsGroup: "testgroup",
		}

		result, err := executor.Execute(context.Background(), cmd, map[string]string{})

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege changes are not supported")
	})

	t.Run("user_group_privilege_execution_fails", func(t *testing.T) {
		mockPriv := privilegetesting.NewFailingMockPrivilegeManager(true)
		executor := NewDefaultExecutor(
			WithOutputWriter(&mockOutputWriter{}),
			WithPrivilegeManager(mockPriv),
			WithFileSystem(&mockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_user_group",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsUser:  "invaliduser",
			RunAsGroup: "invalidgroup",
		}

		result, err := executor.Execute(context.Background(), cmd, map[string]string{})

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege execution failed")
	})

	t.Run("only_user_specified", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		executor := NewDefaultExecutor(
			WithOutputWriter(&mockOutputWriter{}),
			WithPrivilegeManager(mockPriv),
			WithFileSystem(&mockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:      "test_user_only",
			Cmd:       "echo",
			Args:      []string{"test"},
			RunAsUser: "testuser",
			// RunAsGroup is empty
		}

		result, err := executor.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that WithUserGroup was called with empty group
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:")
	})

	t.Run("only_group_specified", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		executor := NewDefaultExecutor(
			WithOutputWriter(&mockOutputWriter{}),
			WithPrivilegeManager(mockPriv),
			WithFileSystem(&mockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_group_only",
			Cmd:        "echo",
			Args:       []string{"test"},
			RunAsGroup: "testgroup",
			// RunAsUser is empty
		}

		result, err := executor.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that WithUserGroup was called with empty user
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change::testgroup")
	})
}

func TestDefaultExecutor_Execute_Integration(t *testing.T) {
	t.Run("privileged_with_user_group_both_specified", func(t *testing.T) {
		// Test case where both Privileged=true and user/group are specified
		// User/group should take precedence
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		executor := NewDefaultExecutor(
			WithOutputWriter(&mockOutputWriter{}),
			WithPrivilegeManager(mockPriv),
			WithFileSystem(&mockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name:       "test_both",
			Cmd:        "/bin/echo",
			Args:       []string{"test"},
			Privileged: true,       // Also privileged
			RunAsUser:  "testuser", // But user/group specified
			RunAsGroup: "testgroup",
		}

		result, err := executor.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should use user/group execution, not privileged execution
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
		assert.NotContains(t, mockPriv.ElevationCalls, "command_execution")
	})

	t.Run("normal_execution_no_privileges", func(t *testing.T) {
		// Test case where neither Privileged nor user/group are specified
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		executor := NewDefaultExecutor(
			WithOutputWriter(&mockOutputWriter{}),
			WithPrivilegeManager(mockPriv),
			WithFileSystem(&mockFileSystem{}),
		)

		cmd := runnertypes.Command{
			Name: "test_normal",
			Cmd:  "echo",
			Args: []string{"test"},
			// No privileged, no user/group
		}

		result, err := executor.Execute(context.Background(), cmd, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Should not call any privilege methods
		assert.Empty(t, mockPriv.ElevationCalls)
	})
}
