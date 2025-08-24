package resource

import (
	"context"
	"fmt"
	"testing"

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

func createTestNormalResourceManager() (*NormalResourceManager, *MockExecutor, *MockFileSystem, *MockPrivilegeManager) {
	mockExec := &MockExecutor{}
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}

	manager := NewNormalResourceManager(mockExec, mockFS, mockPriv)

	return manager, mockExec, mockFS, mockPriv
}

func createTestCommand() runnertypes.Command {
	return runnertypes.Command{
		Name:        "test-command",
		Description: "Test command description",
		Cmd:         "echo",
		Args:        []string{"hello", "world"},
		Dir:         "/tmp",
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

	mockExec.On("Execute", ctx, cmd, env).Return(expectedResult, nil)

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
				Name:        "test-privilege-command",
				Description: "Test privilege escalation command",
				Cmd:         tc.cmd,
				Args:        tc.args,
				Dir:         "/tmp",
				Timeout:     30,
			}
			group := createTestCommandGroup()
			env := map[string]string{"TEST": "value"}
			ctx := context.Background()

			result, err := manager.ExecuteCommand(ctx, cmd, group, env)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, runnertypes.ErrCriticalRiskBlocked)
		})
	}
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
