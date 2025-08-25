package resource

import (
	"context"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Test helper functions for dry-run manager

func createTestDryRunResourceManager() *DryRunResourceManager {
	mockExec := &MockExecutor{}
	mockPriv := &MockPrivilegeManager{}
	mockPathResolver := &MockPathResolver{}

	// Add default expectations for privilege manager
	mockPriv.On("IsPrivilegedExecutionSupported").Return(true)
	mockPriv.On("WithPrivileges", mock.Anything, mock.Anything).Return(nil)

	// Add default expectation for path resolver
	setupStandardCommandPaths(mockPathResolver) // fallback

	opts := &DryRunOptions{
		DetailLevel: DetailLevelDetailed,
	}

	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, opts, false, "")
	if err != nil {
		panic(err) // This is a test helper, so panic is acceptable here
	}

	return manager
}

// Tests for DryRun Resource Manager

func TestDryRunResourceManager_ExecuteCommand(t *testing.T) {
	manager := createTestDryRunResourceManager()
	cmd := createTestCommand()
	group := createTestCommandGroup()
	env := map[string]string{"TEST": "value"}
	ctx := context.Background()

	result, err := manager.ExecuteCommand(ctx, cmd, group, env)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "[DRY-RUN]")
	assert.Contains(t, result.Stdout, cmd.Cmd)
	assert.True(t, result.DryRun)
	assert.NotNil(t, result.Analysis)
	assert.Equal(t, ResourceTypeCommand, result.Analysis.Type)
	assert.Equal(t, OperationExecute, result.Analysis.Operation)
	assert.Equal(t, cmd.Cmd, result.Analysis.Target)
}

func TestDryRunResourceManager_CreateTempDir(t *testing.T) {
	manager := createTestDryRunResourceManager()
	groupName := "test-group"

	path, err := manager.CreateTempDir(groupName)

	assert.NoError(t, err)
	assert.Contains(t, path, groupName)

	// Check that analysis was recorded
	result := manager.GetDryRunResults()
	assert.NotNil(t, result)
	assert.Len(t, result.ResourceAnalyses, 1)
	analysis := result.ResourceAnalyses[0]
	assert.Equal(t, ResourceTypeFilesystem, analysis.Type)
	assert.Equal(t, OperationCreate, analysis.Operation)
}

func TestDryRunResourceManager_CleanupTempDir(t *testing.T) {
	manager := createTestDryRunResourceManager()
	tempPath := "/tmp/test-path"

	err := manager.CleanupTempDir(tempPath)

	assert.NoError(t, err)

	// Check that analysis was recorded
	result := manager.GetDryRunResults()
	assert.NotNil(t, result)
	assert.Len(t, result.ResourceAnalyses, 1)
	analysis := result.ResourceAnalyses[0]
	assert.Equal(t, ResourceTypeFilesystem, analysis.Type)
	assert.Equal(t, OperationDelete, analysis.Operation)
	assert.Equal(t, tempPath, analysis.Target)
}

func TestDryRunResourceManager_WithPrivileges(t *testing.T) {
	manager := createTestDryRunResourceManager()
	ctx := context.Background()

	called := false
	fn := func() error {
		called = true
		return nil
	}

	err := manager.WithPrivileges(ctx, fn)

	assert.NoError(t, err)
	assert.True(t, called) // Function should still be called in dry-run

	// Check that analysis was recorded
	result := manager.GetDryRunResults()
	assert.NotNil(t, result)
	assert.Len(t, result.ResourceAnalyses, 1)
	analysis := result.ResourceAnalyses[0]
	assert.Equal(t, ResourceTypePrivilege, analysis.Type)
	assert.Equal(t, OperationEscalate, analysis.Operation)
}

func TestDryRunResourceManager_SendNotification(t *testing.T) {
	manager := createTestDryRunResourceManager()
	message := "Test notification"
	details := map[string]any{"key": "value"}

	err := manager.SendNotification(message, details)

	assert.NoError(t, err)

	// Check that analysis was recorded
	result := manager.GetDryRunResults()
	assert.NotNil(t, result)
	assert.Len(t, result.ResourceAnalyses, 1)
	analysis := result.ResourceAnalyses[0]
	assert.Equal(t, ResourceTypeNetwork, analysis.Type)
	assert.Equal(t, OperationSend, analysis.Operation)
	assert.Equal(t, "notification_service", analysis.Target)
}

func TestDryRunResourceManager_GetDryRunResults(t *testing.T) {
	manager := createTestDryRunResourceManager()

	result := manager.GetDryRunResults()
	assert.NotNil(t, result)
	assert.NotNil(t, result.Metadata)
	assert.NotEmpty(t, result.Metadata.RunID)
	assert.NotNil(t, result.SecurityAnalysis)
	assert.Empty(t, result.ResourceAnalyses) // Should be empty initially
}

func TestDryRunResourceManager_SecurityAnalysis(t *testing.T) {
	// Create manager with standard command paths
	mockExec := &MockExecutor{}
	mockPriv := &MockPrivilegeManager{}
	mockPathResolver := &MockPathResolver{}

	// Add default expectations for privilege manager
	mockPriv.On("IsPrivilegedExecutionSupported").Return(true)
	mockPriv.On("WithPrivileges", mock.Anything, mock.Anything).Return(nil)

	// Setup standard command paths
	setupStandardCommandPaths(mockPathResolver)

	opts := &DryRunOptions{
		DetailLevel: DetailLevelDetailed,
	}

	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, opts, false, "")
	require.NoError(t, err)

	tests := []struct {
		name                 string
		cmd                  runnertypes.Command
		expectedSecurityRisk string
		expectedDescription  string
	}{
		{
			name: "dangerous rm command with args",
			cmd: runnertypes.Command{
				Name: "dangerous-rm",
				Cmd:  "rm",
				Args: []string{"-rf", "/important/data"},
			},
			expectedSecurityRisk: "high",
			expectedDescription:  "Recursive file removal",
		},
		{
			name: "user/group privilege command",
			cmd: runnertypes.Command{
				Name:      "restart-nginx",
				Cmd:       "systemctl",
				Args:      []string{"restart", "nginx"},
				RunAsUser: "root",
			},
			expectedSecurityRisk: "high", // Expect high due to systemctl command override
			expectedDescription:  "User/Group configuration validated",
		},
		{
			name: "normal command",
			cmd: runnertypes.Command{
				Name: "list-files",
				Cmd:  "ls",
				Args: []string{"-la"},
			},
			expectedSecurityRisk: "low", // Now expects low due to directory-based assessment
			expectedDescription:  "",
		},
		{
			name: "dangerous command with user specification should be high risk",
			cmd: runnertypes.Command{
				Name:      "privileged-rm",
				Cmd:       "sudo",
				Args:      []string{"rm", "-rf", "/important/data"},
				RunAsUser: "root",
			},
			expectedSecurityRisk: "high",
			expectedDescription:  "Privileged file removal",
		},
		{
			name: "dangerous command with args and user specification",
			cmd: runnertypes.Command{
				Name:      "rm-privileged",
				Cmd:       "rm",
				Args:      []string{"-rf", "/important/data"},
				RunAsUser: "root",
			},
			expectedSecurityRisk: "high",
			expectedDescription:  "Recursive file removal",
		},
		{
			name: "dd command with separate args",
			cmd: runnertypes.Command{
				Name: "disk-dd",
				Cmd:  "dd",
				Args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1M"},
			},
			expectedSecurityRisk: "high",
			expectedDescription:  "Low-level disk operations",
		},
		{
			name: "chmod with separate args",
			cmd: runnertypes.Command{
				Name: "change-perms",
				Cmd:  "chmod",
				Args: []string{"777", "/tmp/test"},
			},
			expectedSecurityRisk: "medium",
			expectedDescription:  "Overly permissive file permissions",
		},
		{
			name: "executable without setuid bit but with chmod 777 pattern",
			cmd: runnertypes.Command{
				Name: "chmod-test",
				Cmd:  "chmod", // Use the actual chmod command
				Args: []string{"777", "/tmp/test"},
			},
			expectedSecurityRisk: "medium",
			expectedDescription:  "Overly permissive file permissions",
		},
	}

	// Add a test case for setuid binary (high priority)
	t.Run("setuid binary takes priority over medium risk patterns", func(t *testing.T) {
		// Create a temporary file with setuid bit
		setuidFile, err := os.CreateTemp("", "setuid-test-*")
		require.NoError(t, err)
		defer os.Remove(setuidFile.Name())
		require.NoError(t, setuidFile.Close())

		// Set executable and setuid bit
		err = os.Chmod(setuidFile.Name(), 0o755|os.ModeSetuid)
		require.NoError(t, err)

		// Create a separate manager with setuid file path resolver
		mockExec := &MockExecutor{}
		mockPriv := &MockPrivilegeManager{}
		mockPathResolver := &MockPathResolver{}

		mockPriv.On("IsPrivilegedExecutionSupported").Return(true)
		mockPriv.On("WithPrivileges", mock.Anything, mock.Anything).Return(nil)

		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", "setuid-chmod").Return(setuidFile.Name(), nil)

		opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
		setuidManager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, opts, false, "")
		require.NoError(t, err)

		cmd := runnertypes.Command{
			Name: "setuid-chmod",
			Cmd:  "setuid-chmod",
			Args: []string{"777", "/tmp/test"}, // This would normally be medium risk
		}

		ctx := context.Background()
		group := createTestCommandGroup()
		env := map[string]string{}

		result, err := setuidManager.ExecuteCommand(ctx, cmd, group, env)

		assert.NoError(t, err)
		assert.NotNil(t, result.Analysis)
		assert.Equal(t, "high", result.Analysis.Impact.SecurityRisk) // setuid takes priority over medium risk
		assert.Contains(t, result.Analysis.Impact.Description, "setuid")
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			group := createTestCommandGroup()
			env := map[string]string{}

			result, err := manager.ExecuteCommand(ctx, tt.cmd, group, env)

			assert.NoError(t, err)
			assert.NotNil(t, result.Analysis)
			assert.Equal(t, tt.expectedSecurityRisk, result.Analysis.Impact.SecurityRisk)
			if tt.expectedDescription != "" {
				assert.Contains(t, result.Analysis.Impact.Description, tt.expectedDescription)
			}
		})
	}
}

func TestDryRunResourceManager_PathResolverRequired(t *testing.T) {
	mockExec := &MockExecutor{}
	mockPriv := &MockPrivilegeManager{}
	opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}

	// Test that providing nil PathResolver returns an error
	_, err := NewDryRunResourceManager(mockExec, mockPriv, nil, opts, false, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PathResolver is required")
}

func TestDryRunResourceManager_PathResolutionFailure(t *testing.T) {
	mockExec := &MockExecutor{}
	mockPriv := &MockPrivilegeManager{}
	mockPathResolver := &MockPathResolver{}

	mockPriv.On("IsPrivilegedExecutionSupported").Return(true)
	mockPriv.On("WithPrivileges", mock.Anything, mock.Anything).Return(nil)

	// Mock path resolution failure
	mockPathResolver.On("ResolvePath", "nonexistent-cmd").Return("", assert.AnError)

	opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, opts, false, "")
	require.NoError(t, err)
	if err != nil {
		t.Fatalf("Failed to create DryRunResourceManager: %v", err)
	}
	require.NoError(t, err)

	cmd := runnertypes.Command{
		Name: "test-failure",
		Cmd:  "nonexistent-cmd",
		Args: []string{"arg1"},
	}
	group := createTestCommandGroup()
	env := map[string]string{}
	ctx := context.Background()

	result, err := manager.ExecuteCommand(ctx, cmd, group, env)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "command analysis failed")
	assert.Contains(t, err.Error(), "failed to resolve command path")
}
