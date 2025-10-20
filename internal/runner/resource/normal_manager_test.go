//go:build test && skip_integration_tests
// +build test,skip_integration_tests

package resource

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for testing

// MockExecutor is now imported from internal/runner/executor/testing

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

// MockPathResolver for testing
type MockPathResolver struct {
	mock.Mock
}

func (m *MockPathResolver) ResolvePath(command string) (string, error) {
	args := m.Called(command)
	return args.String(0), args.Error(1)
}

// setupStandardCommandPaths sets up common command path mappings for MockPathResolver
func setupStandardCommandPaths(mockPathResolver *MockPathResolver) {
	mockPathResolver.On("ResolvePath", "dd").Return("/usr/bin/dd", nil)
	mockPathResolver.On("ResolvePath", "chmod").Return("/bin/chmod", nil)
	mockPathResolver.On("ResolvePath", "echo").Return("/bin/echo", nil)
	mockPathResolver.On("ResolvePath", "ls").Return("/bin/ls", nil)
	mockPathResolver.On("ResolvePath", "rm").Return("/bin/rm", nil)
	mockPathResolver.On("ResolvePath", "systemctl").Return("/usr/sbin/systemctl", nil)
	mockPathResolver.On("ResolvePath", "sudo").Return("/usr/bin/sudo", nil)
	mockPathResolver.On("ResolvePath", "curl").Return("/usr/bin/curl", nil)
	mockPathResolver.On("ResolvePath", "wget").Return("/usr/bin/wget", nil)
	mockPathResolver.On("ResolvePath", "my-sudo-wrapper").Return("/usr/bin/my-sudo-wrapper", nil)
}

func (m *MockPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) error {
	args := m.Called(elevationCtx, fn)
	return args.Error(0)
}

func (m *MockPrivilegeManager) WithUserGroup(user, group string, fn func() error) error {
	args := m.Called(user, group, fn)
	return args.Error(0)
}

func (m *MockPrivilegeManager) IsUserGroupSupported() bool {
	args := m.Called()
	return args.Bool(0)
}

// Test constants
const testTempPath = "/tmp/scr-test-group-12345"

// Test errors
var (
	ErrTestExecutionFailed = fmt.Errorf("execution failed")
)

// Test helper functions

func createTestNormalResourceManager() (*NormalResourceManager, *executortesting.MockExecutor, *MockFileSystem, *MockPrivilegeManager) {
	mockExec := executortesting.NewMockExecutor()
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}

	manager := NewNormalResourceManager(mockExec, mockFS, mockPriv, slog.Default())

	return manager, mockExec, mockFS, mockPriv
}

func createTestCommand() runnertypes.Command {
	cmd := runnertypes.Command{
		Name:        "test-command",
		Description: "Test command description",
		Cmd:         "echo",
		Args:        []string{"hello", "world"},
		Dir:         "/tmp",
		Timeout:     30,
	}
	runnertypes.PrepareCommand(&cmd)
	return cmd
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

// Tests for Normal Resource Manager

func TestNormalResourceManager_ExecuteCommand(t *testing.T) {
	manager, mockExec, _, _ := createTestNormalResourceManager()
	cmd := createTestCommand()
	group := createTestCommandGroup()
	env := map[string]string{"TEST": "value"}
	ctx := context.Background()

	expectedResult := &executor.Result{
		ExitCode: 0,
		Stdout:   "hello world",
		Stderr:   "",
	}

	mockExec.On("Execute", ctx, cmd, env, mock.Anything).Return(expectedResult, nil)

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

func TestNormalResourceManager_ExecuteCommand_PrivilegeEscalationBlocked(t *testing.T) {
	manager, _, _, _ := createTestNormalResourceManager()

	// Test various privilege escalation commands
	testCases := []struct {
		name string
		cmd  string
		args []string
	}{
		{
			name: "sudo command blocked",
			cmd:  "sudo",
			args: []string{"ls", "/root"},
		},
		{
			name: "su command blocked",
			cmd:  "su",
			args: []string{"root"},
		},
		{
			name: "doas command blocked",
			cmd:  "doas",
			args: []string{"ls", "/root"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := runnertypes.Command{
				Name:         "test-privilege-command",
				Description:  "Test privilege escalation command",
				Cmd:          tc.cmd,
				Args:         tc.args,
				Dir:          "/tmp",
				Timeout:      30,
				MaxRiskLevel: "low", // Default max risk level to ensure Critical risk is blocked
			}
			runnertypes.PrepareCommand(&cmd)
			group := createTestCommandGroup()
			env := map[string]string{"TEST": "value"}
			ctx := context.Background()

			result, err := manager.ExecuteCommand(ctx, cmd, group, env)

			assert.Error(t, err)
			assert.Nil(t, result)
			// Unified approach: should be blocked by security violation, not critical risk error
			assert.ErrorIs(t, err, runnertypes.ErrCommandSecurityViolation)
		})
	}
}

func TestNormalResourceManager_ExecuteCommand_MaxRiskLevelControl(t *testing.T) {
	manager, mockExec, _, _ := createTestNormalResourceManager()
	group := createTestCommandGroup()
	env := map[string]string{"TEST": "value"}
	ctx := context.Background()

	testCases := []struct {
		name          string
		cmd           string
		args          []string
		maxRiskLevel  string
		shouldExecute bool
		expectedError string
	}{
		{
			name:          "low risk command with no max_risk_level (default low)",
			cmd:           "echo",
			args:          []string{"hello"},
			maxRiskLevel:  "", // Default to low
			shouldExecute: true,
		},
		{
			name:          "low risk command with low max_risk_level",
			cmd:           "echo",
			args:          []string{"hello"},
			maxRiskLevel:  "low",
			shouldExecute: true,
		},
		{
			name:          "medium risk command with high max_risk_level",
			cmd:           "wget",
			args:          []string{"http://example.com/file.txt"},
			maxRiskLevel:  "high",
			shouldExecute: true,
		},
		{
			name:          "high risk command with high max_risk_level",
			cmd:           "rm",
			args:          []string{"-rf", "/tmp/test"},
			maxRiskLevel:  "high",
			shouldExecute: true,
		},
		{
			name:          "high risk command with low max_risk_level should be blocked",
			cmd:           "rm",
			args:          []string{"-rf", "/tmp/test"},
			maxRiskLevel:  "low",
			shouldExecute: false,
			expectedError: "command security violation",
		},
		{
			name:          "medium risk command with low max_risk_level should be blocked",
			cmd:           "wget",
			args:          []string{"http://example.com/file.txt"},
			maxRiskLevel:  "low",
			shouldExecute: false,
			expectedError: "command security violation",
		},
		{
			name:          "invalid max_risk_level should return error",
			cmd:           "echo",
			args:          []string{"hello"},
			maxRiskLevel:  "invalid",
			shouldExecute: false,
			expectedError: "invalid max_risk_level configuration",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := runnertypes.Command{
				Name:         "test-command",
				Description:  "Test command",
				Cmd:          tc.cmd,
				Args:         tc.args,
				MaxRiskLevel: tc.maxRiskLevel,
				Dir:          "/tmp",
				Timeout:      30,
			}
			runnertypes.PrepareCommand(&cmd)

			if tc.shouldExecute {
				expectedResult := &executor.Result{
					ExitCode: 0,
					Stdout:   "success",
					Stderr:   "",
				}
				mockExec.On("Execute", ctx, cmd, env, mock.Anything).Return(expectedResult, nil).Once()
			}

			result, err := manager.ExecuteCommand(ctx, cmd, group, env)

			if tc.shouldExecute {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tc.expectedError != "" {
					assert.Contains(t, err.Error(), tc.expectedError)
				}
			}
		})
	}

	mockExec.AssertExpectations(t)
}

func TestNormalResourceManager_CreateTempDir(t *testing.T) {
	manager, _, mockFS, _ := createTestNormalResourceManager()
	groupName := "test-group"
	expectedPath := testTempPath

	mockFS.On("CreateTempDir", "", fmt.Sprintf("scr-%s-", groupName)).Return(expectedPath, nil)

	path, err := manager.CreateTempDir(groupName)

	assert.NoError(t, err)
	assert.Equal(t, expectedPath, path)

	// Check that path is tracked
	manager.mu.RLock()
	assert.Contains(t, manager.tempDirs, expectedPath)
	manager.mu.RUnlock()

	mockFS.AssertExpectations(t)
}

func TestNormalResourceManager_CleanupTempDir(t *testing.T) {
	manager, _, mockFS, _ := createTestNormalResourceManager()
	tempPath := testTempPath

	// Add path to tracking
	manager.mu.Lock()
	manager.tempDirs = append(manager.tempDirs, tempPath)
	manager.mu.Unlock()

	mockFS.On("RemoveAll", tempPath).Return(nil)

	err := manager.CleanupTempDir(tempPath)

	assert.NoError(t, err)

	// Check that path is no longer tracked
	manager.mu.RLock()
	assert.NotContains(t, manager.tempDirs, tempPath)
	manager.mu.RUnlock()

	mockFS.AssertExpectations(t)
}

func TestNormalResourceManager_WithPrivileges(t *testing.T) {
	manager, _, _, mockPriv := createTestNormalResourceManager()
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

	err := manager.WithPrivileges(ctx, fn)

	assert.NoError(t, err)
	assert.True(t, called)

	mockPriv.AssertExpectations(t)
}

func TestNormalResourceManager_SendNotification(t *testing.T) {
	manager, _, _, _ := createTestNormalResourceManager()
	message := "Test notification"
	details := map[string]any{"key": "value"}

	err := manager.SendNotification(message, details)

	assert.NoError(t, err)
}
