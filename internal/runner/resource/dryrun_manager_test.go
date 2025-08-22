package resource

import (
	"context"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// Test helper functions for dry-run manager

func createTestDryRunResourceManager() *DryRunResourceManager {
	mockExec := &MockExecutor{}
	mockPriv := &MockPrivilegeManager{}

	opts := &DryRunOptions{
		DetailLevel: DetailLevelDetailed,
	}

	manager := NewDryRunResourceManager(mockExec, mockPriv, opts)

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
	manager := createTestDryRunResourceManager()

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
			name: "privileged command",
			cmd: runnertypes.Command{
				Name:       "restart-nginx",
				Cmd:        "systemctl",
				Args:       []string{"restart", "nginx"},
				Privileged: true,
			},
			expectedSecurityRisk: "medium",
			expectedDescription:  "PRIVILEGE",
		},
		{
			name: "normal command",
			cmd: runnertypes.Command{
				Name: "list-files",
				Cmd:  "ls",
				Args: []string{"-la"},
			},
			expectedSecurityRisk: "",
			expectedDescription:  "",
		},
		{
			name: "dangerous command with privilege should be high risk",
			cmd: runnertypes.Command{
				Name:       "privileged-rm",
				Cmd:        "sudo",
				Args:       []string{"rm", "-rf", "/important/data"},
				Privileged: true,
			},
			expectedSecurityRisk: "high",
			expectedDescription:  "Privileged file removal",
		},
		{
			name: "dangerous command with args and privilege",
			cmd: runnertypes.Command{
				Name:       "rm-privileged",
				Cmd:        "rm",
				Args:       []string{"-rf", "/important/data"},
				Privileged: true,
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
			name: "sudo with absolute path detected as privileged",
			cmd: runnertypes.Command{
				Name: "sudo-ls",
				Cmd:  "/usr/bin/sudo",
				Args: []string{"ls"},
			},
			expectedSecurityRisk: "medium",
			expectedDescription:  "PRIVILEGE",
		},
		{
			name: "pseudo-tool not detected as sudo",
			cmd: runnertypes.Command{
				Name: "pseudo-tool",
				Cmd:  "/usr/bin/pseudo-tool",
				Args: []string{"--help"},
			},
			expectedSecurityRisk: "",
			expectedDescription:  "",
		},
		{
			name: "command with sudo in name but not sudo itself",
			cmd: runnertypes.Command{
				Name: "sudo-wrapper",
				Cmd:  "my-sudo-wrapper",
				Args: []string{"ls"},
			},
			expectedSecurityRisk: "",
			expectedDescription:  "",
		},
	}

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
