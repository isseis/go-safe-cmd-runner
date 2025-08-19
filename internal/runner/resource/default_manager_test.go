package resource

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for testing

// MockExecutor implements executor.CommandExecutor for testing
type MockExecutor struct {
	mock.Mock
}

func (m *MockExecutor) Execute(ctx context.Context, cmd runnertypes.Command, env map[string]string) (*executor.Result, error) {
	args := m.Called(ctx, cmd, env)
	return args.Get(0).(*executor.Result), args.Error(1)
}

func (m *MockExecutor) Validate(cmd runnertypes.Command) error {
	args := m.Called(cmd)
	return args.Error(0)
}

// MockFileSystem implements executor.FileSystem for testing
type MockFileSystem struct {
	mock.Mock
}

func (m *MockFileSystem) CreateTempDir(dir string, prefix string) (string, error) {
	args := m.Called(dir, prefix)
	return args.String(0), args.Error(1)
}

func (m *MockFileSystem) RemoveAll(path string) error {
	args := m.Called(path)
	return args.Error(0)
}

func (m *MockFileSystem) FileExists(path string) (bool, error) {
	args := m.Called(path)
	return args.Bool(0), args.Error(1)
}

// MockPrivilegeManager implements runnertypes.PrivilegeManager for testing
type MockPrivilegeManager struct {
	mock.Mock
}

func (m *MockPrivilegeManager) IsPrivilegedExecutionSupported() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) error {
	args := m.Called(elevationCtx, fn)
	return args.Error(0)
}

// Test constants
const testTempPath = "/tmp/scr-test-group-12345"

// Test errors
var (
	ErrTestExecutionFailed = fmt.Errorf("execution failed")
)

// Test helper functions

func createTestDefaultResourceManager() (*DefaultResourceManager, *MockExecutor, *MockFileSystem, *MockPrivilegeManager) {
	mockExec := &MockExecutor{}
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}

	manager := NewDefaultResourceManager(mockExec, mockFS, mockPriv)

	return manager, mockExec, mockFS, mockPriv
}

func createTestCommand() runnertypes.Command {
	return runnertypes.Command{
		Name:        "test-command",
		Description: "Test command description",
		Cmd:         "echo hello",
		Args:        []string{"world"},
		Dir:         "/tmp",
		Privileged:  false,
		Timeout:     30,
	}
}

func createTestCommandGroup() *runnertypes.CommandGroup {
	return &runnertypes.CommandGroup{
		Name:        "test-group",
		Description: "Test group description",
		Priority:    1,
		TempDir:     false,
		WorkDir:     "/tmp",
		Commands:    []runnertypes.Command{createTestCommand()},
	}
}

// Tests for Mode Management

func TestDefaultResourceManager_SetMode(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()

	tests := []struct {
		name string
		mode ExecutionMode
		opts *DryRunOptions
	}{
		{
			name: "set normal mode",
			mode: ExecutionModeNormal,
			opts: nil,
		},
		{
			name: "set dry-run mode",
			mode: ExecutionModeDryRun,
			opts: &DryRunOptions{
				DetailLevel:   DetailLevelDetailed,
				OutputFormat:  OutputFormatText,
				ShowSensitive: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager.SetMode(tt.mode, tt.opts)
			assert.Equal(t, tt.mode, manager.GetMode())

			if tt.mode == ExecutionModeDryRun {
				result := manager.GetDryRunResults()
				assert.NotNil(t, result)
				assert.NotNil(t, result.Metadata)
				assert.NotEmpty(t, result.Metadata.RunID)
			}
		})
	}
}

func TestDefaultResourceManager_GetMode(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()

	// Default mode should be normal
	assert.Equal(t, ExecutionModeNormal, manager.GetMode())

	// After setting dry-run mode
	manager.SetMode(ExecutionModeDryRun, &DryRunOptions{})
	assert.Equal(t, ExecutionModeDryRun, manager.GetMode())
}

// Tests for Command Execution

func TestDefaultResourceManager_ExecuteCommand_Normal(t *testing.T) {
	manager, mockExec, _, _ := createTestDefaultResourceManager()
	cmd := createTestCommand()
	group := createTestCommandGroup()
	env := map[string]string{"TEST": "value"}
	ctx := context.Background()

	expectedResult := &executor.Result{
		ExitCode: 0,
		Stdout:   "hello world",
		Stderr:   "",
	}

	mockExec.On("Execute", ctx, cmd, env).Return(expectedResult, nil)

	// Set to normal mode
	manager.SetMode(ExecutionModeNormal, nil)

	result, err := manager.ExecuteCommand(ctx, cmd, group, env)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ExitCode, result.ExitCode)
	assert.Equal(t, expectedResult.Stdout, result.Stdout)
	assert.Equal(t, expectedResult.Stderr, result.Stderr)
	assert.False(t, result.DryRun)
	assert.Nil(t, result.Analysis)

	mockExec.AssertExpectations(t)
}

func TestDefaultResourceManager_ExecuteCommand_DryRun(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()
	cmd := createTestCommand()
	group := createTestCommandGroup()
	env := map[string]string{"TEST": "value"}
	ctx := context.Background()

	// Set to dry-run mode
	manager.SetMode(ExecutionModeDryRun, &DryRunOptions{
		DetailLevel: DetailLevelDetailed,
	})

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

func TestDefaultResourceManager_ExecuteCommand_Error(t *testing.T) {
	manager, mockExec, _, _ := createTestDefaultResourceManager()
	cmd := createTestCommand()
	group := createTestCommandGroup()
	env := map[string]string{"TEST": "value"}
	ctx := context.Background()

	expectedError := ErrTestExecutionFailed

	mockExec.On("Execute", ctx, cmd, env).Return((*executor.Result)(nil), expectedError)

	// Set to normal mode
	manager.SetMode(ExecutionModeNormal, nil)

	result, err := manager.ExecuteCommand(ctx, cmd, group, env)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "command execution failed")

	mockExec.AssertExpectations(t)
}

// Tests for Filesystem Operations

func TestDefaultResourceManager_CreateTempDir_Normal(t *testing.T) {
	manager, _, mockFS, _ := createTestDefaultResourceManager()
	groupName := "test-group"
	expectedPath := testTempPath

	mockFS.On("CreateTempDir", "", fmt.Sprintf("scr-%s-", groupName)).Return(expectedPath, nil)

	// Set to normal mode
	manager.SetMode(ExecutionModeNormal, nil)

	path, err := manager.CreateTempDir(groupName)

	assert.NoError(t, err)
	assert.Equal(t, expectedPath, path)

	// Check that path is tracked
	manager.mu.RLock()
	assert.Contains(t, manager.tempDirs, expectedPath)
	manager.mu.RUnlock()

	mockFS.AssertExpectations(t)
}

func TestDefaultResourceManager_CreateTempDir_DryRun(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()
	groupName := "test-group"

	// Set to dry-run mode
	manager.SetMode(ExecutionModeDryRun, &DryRunOptions{})

	path, err := manager.CreateTempDir(groupName)

	assert.NoError(t, err)
	assert.Contains(t, path, fmt.Sprintf("scr-%s-", groupName))

	// Check that analysis was recorded
	result := manager.GetDryRunResults()
	assert.NotNil(t, result)
	assert.Len(t, result.ResourceAnalyses, 1)
	analysis := result.ResourceAnalyses[0]
	assert.Equal(t, ResourceTypeFilesystem, analysis.Type)
	assert.Equal(t, OperationCreate, analysis.Operation)
}

func TestDefaultResourceManager_CleanupTempDir_Normal(t *testing.T) {
	manager, _, mockFS, _ := createTestDefaultResourceManager()
	tempPath := testTempPath

	// Add path to tracking
	manager.mu.Lock()
	manager.tempDirs = append(manager.tempDirs, tempPath)
	manager.mu.Unlock()

	mockFS.On("RemoveAll", tempPath).Return(nil)

	// Set to normal mode
	manager.SetMode(ExecutionModeNormal, nil)

	err := manager.CleanupTempDir(tempPath)

	assert.NoError(t, err)

	// Check that path is no longer tracked
	manager.mu.RLock()
	assert.NotContains(t, manager.tempDirs, tempPath)
	manager.mu.RUnlock()

	mockFS.AssertExpectations(t)
}

func TestDefaultResourceManager_CleanupTempDir_DryRun(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()
	tempPath := testTempPath

	// Set to dry-run mode
	manager.SetMode(ExecutionModeDryRun, &DryRunOptions{})

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

func TestDefaultResourceManager_CleanupAllTempDirs(t *testing.T) {
	manager, _, mockFS, _ := createTestDefaultResourceManager()

	tempPaths := []string{
		"/tmp/scr-group1-12345",
		"/tmp/scr-group2-67890",
	}

	// Add paths to tracking
	manager.mu.Lock()
	manager.tempDirs = append(manager.tempDirs, tempPaths...)
	manager.mu.Unlock()

	// Set up mock expectations
	for _, path := range tempPaths {
		mockFS.On("RemoveAll", path).Return(nil)
	}

	// Set to normal mode
	manager.SetMode(ExecutionModeNormal, nil)

	err := manager.CleanupAllTempDirs()

	assert.NoError(t, err)

	// Check that no paths are tracked
	manager.mu.RLock()
	assert.Empty(t, manager.tempDirs)
	manager.mu.RUnlock()

	mockFS.AssertExpectations(t)
}

// Tests for Privilege Management

func TestDefaultResourceManager_WithPrivileges_Normal(t *testing.T) {
	manager, _, _, mockPriv := createTestDefaultResourceManager()
	ctx := context.Background()

	called := false
	fn := func() error {
		called = true
		return nil
	}

	mockPriv.On("WithPrivileges", mock.AnythingOfType("runnertypes.ElevationContext"), mock.AnythingOfType("func() error")).Return(nil).Run(func(args mock.Arguments) {
		// Call the provided function
		fnArg := args.Get(1).(func() error)
		fnArg()
	})

	// Set to normal mode
	manager.SetMode(ExecutionModeNormal, nil)

	err := manager.WithPrivileges(ctx, fn)

	assert.NoError(t, err)
	assert.True(t, called)

	mockPriv.AssertExpectations(t)
}

func TestDefaultResourceManager_WithPrivileges_DryRun(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()
	ctx := context.Background()

	called := false
	fn := func() error {
		called = true
		return nil
	}

	// Set to dry-run mode
	manager.SetMode(ExecutionModeDryRun, &DryRunOptions{})

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

func TestDefaultResourceManager_IsPrivilegeEscalationRequired(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()

	tests := []struct {
		name     string
		cmd      runnertypes.Command
		expected bool
	}{
		{
			name: "privileged command",
			cmd: runnertypes.Command{
				Cmd:        "ls",
				Privileged: true,
			},
			expected: true,
		},
		{
			name: "sudo in command",
			cmd: runnertypes.Command{
				Cmd:        "sudo ls",
				Privileged: false,
			},
			expected: true,
		},
		{
			name: "normal command",
			cmd: runnertypes.Command{
				Cmd:        "ls",
				Privileged: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			required, err := manager.IsPrivilegeEscalationRequired(tt.cmd)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, required)
		})
	}
}

// Tests for Network Operations

func TestDefaultResourceManager_SendNotification_Normal(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()
	message := "Test notification"
	details := map[string]interface{}{"key": "value"}

	// Set to normal mode
	manager.SetMode(ExecutionModeNormal, nil)

	err := manager.SendNotification(message, details)

	assert.NoError(t, err)
}

func TestDefaultResourceManager_SendNotification_DryRun(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()
	message := "Test notification"
	details := map[string]interface{}{"key": "value"}

	// Set to dry-run mode
	manager.SetMode(ExecutionModeDryRun, &DryRunOptions{})

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

// Tests for Security Analysis

func TestDefaultResourceManager_analyzeCommandSecurity(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()

	tests := []struct {
		name                 string
		cmd                  runnertypes.Command
		expectedSecurityRisk string
		expectedDescription  string
	}{
		{
			name: "dangerous rm command",
			cmd: runnertypes.Command{
				Cmd: "rm -rf /important/data",
			},
			expectedSecurityRisk: "high",
			expectedDescription:  "rm -rf",
		},
		{
			name: "privileged command",
			cmd: runnertypes.Command{
				Cmd:        "systemctl restart nginx",
				Privileged: true,
			},
			expectedSecurityRisk: "medium",
			expectedDescription:  "PRIVILEGE",
		},
		{
			name: "sudo command",
			cmd: runnertypes.Command{
				Cmd: "sudo apt update",
			},
			expectedSecurityRisk: "medium",
			expectedDescription:  "PRIVILEGE",
		},
		{
			name: "normal command",
			cmd: runnertypes.Command{
				Cmd: "ls -la",
			},
			expectedSecurityRisk: "",
			expectedDescription:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := ResourceAnalysis{
				Impact: ResourceImpact{
					Description: fmt.Sprintf("Execute command: %s", tt.cmd.Cmd),
				},
			}

			manager.analyzeCommandSecurity(tt.cmd, &analysis)

			assert.Equal(t, tt.expectedSecurityRisk, analysis.Impact.SecurityRisk)
			if tt.expectedDescription != "" {
				assert.Contains(t, analysis.Impact.Description, tt.expectedDescription)
			}
		})
	}
}

// Tests for Analysis Recording

func TestDefaultResourceManager_RecordAnalysis(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()

	// Set to dry-run mode to initialize result
	manager.SetMode(ExecutionModeDryRun, &DryRunOptions{})

	analysis := ResourceAnalysis{
		Type:      ResourceTypeCommand,
		Operation: OperationExecute,
		Target:    "test-command",
		Timestamp: time.Now(),
	}

	manager.RecordAnalysis(&analysis)

	// Check that analysis was recorded
	manager.mu.RLock()
	assert.Len(t, manager.resourceAnalyses, 1)
	assert.Equal(t, analysis, manager.resourceAnalyses[0])
	manager.mu.RUnlock()

	// Check that dry-run result was updated
	result := manager.GetDryRunResults()
	assert.NotNil(t, result)
	assert.Len(t, result.ResourceAnalyses, 1)
	assert.Equal(t, analysis, result.ResourceAnalyses[0])
}

func TestDefaultResourceManager_GetDryRunResults_NormalMode(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()

	// In normal mode, should return nil
	result := manager.GetDryRunResults()
	assert.Nil(t, result)
}

func TestDefaultResourceManager_GetDryRunResults_DryRunMode(t *testing.T) {
	manager, _, _, _ := createTestDefaultResourceManager()

	// Set to dry-run mode
	manager.SetMode(ExecutionModeDryRun, &DryRunOptions{})

	result := manager.GetDryRunResults()
	assert.NotNil(t, result)
	assert.NotNil(t, result.Metadata)
	assert.NotEmpty(t, result.Metadata.RunID)
	assert.NotNil(t, result.SecurityAnalysis)
	assert.Empty(t, result.ResourceAnalyses) // Should be empty initially
}

// Integration Tests

func TestDefaultResourceManager_Integration_CommandExecution(t *testing.T) {
	manager, mockExec, _, _ := createTestDefaultResourceManager()
	cmd := createTestCommand()
	group := createTestCommandGroup()
	env := map[string]string{"TEST": "value"}
	ctx := context.Background()

	// Test normal mode execution
	expectedResult := &executor.Result{
		ExitCode: 0,
		Stdout:   "hello world",
		Stderr:   "",
	}

	mockExec.On("Execute", ctx, cmd, env).Return(expectedResult, nil)

	manager.SetMode(ExecutionModeNormal, nil)
	result, err := manager.ExecuteCommand(ctx, cmd, group, env)

	assert.NoError(t, err)
	assert.False(t, result.DryRun)

	// Switch to dry-run mode and test
	manager.SetMode(ExecutionModeDryRun, &DryRunOptions{
		DetailLevel: DetailLevelDetailed,
	})

	result, err = manager.ExecuteCommand(ctx, cmd, group, env)

	assert.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.NotNil(t, result.Analysis)

	// Verify analysis was recorded
	dryRunResult := manager.GetDryRunResults()
	assert.NotNil(t, dryRunResult)
	assert.Len(t, dryRunResult.ResourceAnalyses, 1)

	mockExec.AssertExpectations(t)
}
