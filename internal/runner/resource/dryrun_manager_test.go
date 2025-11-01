package resource

import (
	"context"
	"os"
	"testing"

	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Test helper functions for dry-run manager

func createTestDryRunResourceManager() *DryRunResourceManager {
	mockExec := executortesting.NewMockExecutor()
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

	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, opts)
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

	_, result, err := manager.ExecuteCommand(ctx, cmd, group, env)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "[DRY-RUN]")
	assert.Contains(t, result.Stdout, cmd.ExpandedCmd)
	assert.True(t, result.DryRun)
	assert.NotNil(t, result.Analysis)
	assert.Equal(t, ResourceTypeCommand, result.Analysis.Type)
	assert.Equal(t, OperationExecute, result.Analysis.Operation)
	assert.Equal(t, cmd.ExpandedCmd, result.Analysis.Target)
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
	mockExec := executortesting.NewMockExecutor()
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

	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, opts)
	require.NoError(t, err)

	tests := []struct {
		name                 string
		spec                 runnertypes.CommandSpec
		expectedSecurityRisk string
		expectedDescription  string
	}{
		{
			name: "dangerous rm command with args",
			spec: runnertypes.CommandSpec{
				Name: "dangerous-rm",
				Cmd:  "rm",
				Args: []string{"-rf", "/important/data"},
			},
			expectedSecurityRisk: "high",
			expectedDescription:  "Recursive file removal",
		},
		{
			name: "user/group privilege command",
			spec: runnertypes.CommandSpec{
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
			spec: runnertypes.CommandSpec{
				Name: "list-files",
				Cmd:  "ls",
				Args: []string{"-la"},
			},
			expectedSecurityRisk: "low", // Now expects low due to directory-based assessment
			expectedDescription:  "",
		},
		{
			name: "dangerous command with user specification should be high risk",
			spec: runnertypes.CommandSpec{
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
			spec: runnertypes.CommandSpec{
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
			spec: runnertypes.CommandSpec{
				Name: "disk-dd",
				Cmd:  "dd",
				Args: []string{"if=/dev/zero", "of=/dev/sda", "bs=1M"},
			},
			expectedSecurityRisk: "high",
			expectedDescription:  "Low-level disk operations",
		},
		{
			name: "chmod with separate args",
			spec: runnertypes.CommandSpec{
				Name: "change-perms",
				Cmd:  "chmod",
				Args: []string{"777", "/tmp/test"},
			},
			expectedSecurityRisk: "medium",
			expectedDescription:  "Overly permissive file permissions",
		},
		{
			name: "executable without setuid bit but with chmod 777 pattern",
			spec: runnertypes.CommandSpec{
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
		mockExec := executortesting.NewMockExecutor()
		mockPriv := &MockPrivilegeManager{}
		mockPathResolver := &MockPathResolver{}

		mockPriv.On("IsPrivilegedExecutionSupported").Return(true)
		mockPriv.On("WithPrivileges", mock.Anything, mock.Anything).Return(nil)

		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", "setuid-chmod").Return(setuidFile.Name(), nil)

		opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
		setuidManager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, opts)
		require.NoError(t, err)

		cmd := createRuntimeCommand(&runnertypes.CommandSpec{
			Name: "setuid-chmod",
			Cmd:  "setuid-chmod",
			Args: []string{"777", "/tmp/test"}, // This would normally be medium risk
		})

		ctx := context.Background()
		group := createTestCommandGroup()
		env := map[string]string{}

		_, result, err := setuidManager.ExecuteCommand(ctx, cmd, group, env)

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
			cmd := createRuntimeCommand(&tt.spec)

			_, result, err := manager.ExecuteCommand(ctx, cmd, group, env)

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
	mockExec := executortesting.NewMockExecutor()
	mockPriv := &MockPrivilegeManager{}
	opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}

	// Test that providing nil PathResolver returns an error
	_, err := NewDryRunResourceManager(mockExec, mockPriv, nil, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PathResolver is required")
}

func TestDryRunResourceManager_PathResolutionFailure(t *testing.T) {
	mockExec := executortesting.NewMockExecutor()
	mockPriv := &MockPrivilegeManager{}
	mockPathResolver := &MockPathResolver{}

	mockPriv.On("IsPrivilegedExecutionSupported").Return(true)
	mockPriv.On("WithPrivileges", mock.Anything, mock.Anything).Return(nil)

	// Mock path resolution failure
	mockPathResolver.On("ResolvePath", "nonexistent-cmd").Return("", assert.AnError)

	opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, opts)
	require.NoError(t, err)
	if err != nil {
		t.Fatalf("Failed to create DryRunResourceManager: %v", err)
	}
	require.NoError(t, err)

	cmd := createRuntimeCommand(&runnertypes.CommandSpec{
		Name: "test-failure",
		Cmd:  "nonexistent-cmd",
		Args: []string{"arg1"},
	})
	group := createTestCommandGroup()
	env := map[string]string{}
	ctx := context.Background()

	_, result, err := manager.ExecuteCommand(ctx, cmd, group, env)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "command analysis failed")
	assert.Contains(t, err.Error(), "failed to resolve command path")
}

func TestDryRunResourceManager_ValidateOutputPath(t *testing.T) {
	manager := createTestDryRunResourceManager()

	tests := []struct {
		name        string
		outputPath  string
		workDir     string
		expectError bool
		errorType   error
	}{
		{
			name:        "empty path",
			outputPath:  "",
			workDir:     "/tmp",
			expectError: false,
		},
		{
			name:        "valid path",
			outputPath:  "/tmp/output.log",
			workDir:     "/tmp",
			expectError: false,
		},
		{
			name:        "path traversal with ..",
			outputPath:  "../../../etc/passwd",
			workDir:     "/tmp",
			expectError: true,
			errorType:   ErrPathTraversalDetected,
		},
		{
			name:        "path traversal in middle",
			outputPath:  "/tmp/../etc/passwd",
			workDir:     "/tmp",
			expectError: true,
			errorType:   ErrPathTraversalDetected,
		},
		{
			name:        "relative path with traversal",
			outputPath:  "subdir/../../../etc/passwd",
			workDir:     "/tmp",
			expectError: true,
			errorType:   ErrPathTraversalDetected,
		},
		{
			name:        "file with .. in name (should not trigger)",
			outputPath:  "/tmp/backup..2023.log",
			workDir:     "/tmp",
			expectError: false,
		},
		{
			name:        "directory with .. in name (should not trigger)",
			outputPath:  "/tmp/app..backup/output.log",
			workDir:     "/tmp",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateOutputPath(tt.outputPath, tt.workDir)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDryRunResourceManager_RecordGroupAnalysis(t *testing.T) {
	tests := []struct {
		name        string
		groupName   string
		debugInfo   *DebugInfo
		expectError bool
	}{
		{
			name:      "Record group analysis with debug info",
			groupName: "test-group",
			debugInfo: &DebugInfo{
				InheritanceAnalysis: &InheritanceAnalysis{
					GlobalEnvImport: []string{"DB_HOST=db_host"},
					GlobalAllowlist: []string{"PATH"},
					InheritanceMode: runnertypes.InheritanceModeInherit,
				},
			},
			expectError: false,
		},
		{
			name:        "Record group analysis with nil debug info",
			groupName:   "test-group-2",
			debugInfo:   nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := createTestDryRunResourceManager()

			err := manager.RecordGroupAnalysis(tt.groupName, tt.debugInfo)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify the analysis was recorded
				result := manager.GetDryRunResults()
				require.NotNil(t, result)
				require.Greater(t, len(result.ResourceAnalyses), 0)

				// Find the group analysis
				var foundAnalysis *ResourceAnalysis
				for i := range result.ResourceAnalyses {
					if result.ResourceAnalyses[i].Type == ResourceTypeGroup {
						foundAnalysis = &result.ResourceAnalyses[i]
						break
					}
				}

				require.NotNil(t, foundAnalysis, "Group analysis should be recorded")
				assert.Equal(t, ResourceTypeGroup, foundAnalysis.Type)
				assert.Equal(t, OperationAnalyze, foundAnalysis.Operation)
				assert.Equal(t, tt.groupName, foundAnalysis.Target)
				assert.Equal(t, tt.debugInfo, foundAnalysis.DebugInfo)
			}
		})
	}
}

func TestDryRunResourceManager_RecordGroupAnalysis_NilManager(t *testing.T) {
	var manager *DryRunResourceManager
	err := manager.RecordGroupAnalysis("test-group", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource manager is nil")
}

func TestDryRunResourceManager_UpdateCommandDebugInfo(t *testing.T) {
	tests := []struct {
		name            string
		setupFunc       func(*DryRunResourceManager) CommandToken
		debugInfo       *DebugInfo
		useInvalidToken bool
		expectError     bool
		errorContains   string
		validateResult  func(*testing.T, *DryRunResourceManager)
	}{
		{
			name: "Update command with final environment using valid token",
			setupFunc: func(m *DryRunResourceManager) CommandToken {
				// Record a command analysis first
				cmd := createTestCommand()
				group := createTestCommandGroup()
				env := map[string]string{"TEST": "value"}
				ctx := context.Background()
				token, _, _ := m.ExecuteCommand(ctx, cmd, group, env)
				return token
			},
			debugInfo: &DebugInfo{
				FinalEnvironment: &FinalEnvironment{
					Variables: map[string]EnvironmentVariable{
						"TEST": {
							Value:  "value",
							Source: "vars",
							Masked: false,
						},
					},
				},
			},
			expectError: false,
			validateResult: func(t *testing.T, m *DryRunResourceManager) {
				result := m.GetDryRunResults()
				require.NotNil(t, result)
				require.Greater(t, len(result.ResourceAnalyses), 0)

				// Find the command analysis
				var cmdAnalysis *ResourceAnalysis
				for i := range result.ResourceAnalyses {
					if result.ResourceAnalyses[i].Type == ResourceTypeCommand {
						cmdAnalysis = &result.ResourceAnalyses[i]
						break
					}
				}

				require.NotNil(t, cmdAnalysis, "Command analysis should exist")
				require.NotNil(t, cmdAnalysis.DebugInfo, "DebugInfo should be set")
				require.NotNil(t, cmdAnalysis.DebugInfo.FinalEnvironment, "FinalEnvironment should be set")
				assert.Len(t, cmdAnalysis.DebugInfo.FinalEnvironment.Variables, 1)
			},
		},
		{
			name: "Update with invalid token",
			setupFunc: func(_ *DryRunResourceManager) CommandToken {
				return CommandToken("invalid-token")
			},
			debugInfo: &DebugInfo{
				FinalEnvironment: &FinalEnvironment{
					Variables: map[string]EnvironmentVariable{},
				},
			},
			expectError:   true,
			errorContains: "invalid command token",
		},
		{
			name: "Update with complete debug info including both fields",
			setupFunc: func(m *DryRunResourceManager) CommandToken {
				// Record a command
				cmd := createTestCommand()
				group := createTestCommandGroup()
				env := map[string]string{"TEST": "value"}
				ctx := context.Background()
				token, _, _ := m.ExecuteCommand(ctx, cmd, group, env)
				return token
			},
			debugInfo: &DebugInfo{
				InheritanceAnalysis: &InheritanceAnalysis{
					GlobalEnvImport: []string{"TEST=test"},
					InheritanceMode: runnertypes.InheritanceModeInherit,
				},
				FinalEnvironment: &FinalEnvironment{
					Variables: map[string]EnvironmentVariable{
						"TEST": {
							Value:  "value",
							Source: "vars",
							Masked: false,
						},
					},
				},
			},
			expectError: false,
			validateResult: func(t *testing.T, m *DryRunResourceManager) {
				result := m.GetDryRunResults()
				require.NotNil(t, result)

				// Find the command analysis
				var cmdAnalysis *ResourceAnalysis
				for i := range result.ResourceAnalyses {
					if result.ResourceAnalyses[i].Type == ResourceTypeCommand {
						cmdAnalysis = &result.ResourceAnalyses[i]
						break
					}
				}

				require.NotNil(t, cmdAnalysis)
				require.NotNil(t, cmdAnalysis.DebugInfo)
				assert.NotNil(t, cmdAnalysis.DebugInfo.InheritanceAnalysis, "InheritanceAnalysis should be set")
				assert.NotNil(t, cmdAnalysis.DebugInfo.FinalEnvironment, "FinalEnvironment should be set")
			},
		},
		{
			name: "Duplicate call should return error",
			setupFunc: func(m *DryRunResourceManager) CommandToken {
				// Record a command and update once
				cmd := createTestCommand()
				group := createTestCommandGroup()
				env := map[string]string{"TEST": "value"}
				ctx := context.Background()
				token, _, _ := m.ExecuteCommand(ctx, cmd, group, env)

				// First update - should succeed
				_ = m.UpdateCommandDebugInfo(token, &DebugInfo{
					InheritanceAnalysis: &InheritanceAnalysis{
						GlobalEnvImport: []string{"TEST=test"},
						InheritanceMode: runnertypes.InheritanceModeInherit,
					},
				})
				return token
			},
			debugInfo: &DebugInfo{
				FinalEnvironment: &FinalEnvironment{
					Variables: map[string]EnvironmentVariable{
						"TEST": {
							Value:  "value",
							Source: "vars",
							Masked: false,
						},
					},
				},
			},
			expectError:   true,
			errorContains: "called multiple times",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := createTestDryRunResourceManager()

			// Setup test state and get token
			var token CommandToken
			if tt.setupFunc != nil {
				token = tt.setupFunc(manager)
			}

			err := manager.UpdateCommandDebugInfo(token, tt.debugInfo)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.validateResult != nil {
					tt.validateResult(t, manager)
				}
			}
		})
	}
}

func TestDryRunResourceManager_UpdateCommandDebugInfo_NilManager(t *testing.T) {
	var manager *DryRunResourceManager
	err := manager.UpdateCommandDebugInfo(CommandToken(""), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource manager is nil")
}
