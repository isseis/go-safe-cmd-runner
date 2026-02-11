package resource

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for testing

// MockExecutor is now imported from internal/runner/executor/testutil

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

// MockCaptureManager implements output.CaptureManager for testing
type MockCaptureManager struct {
	mock.Mock
}

func (m *MockCaptureManager) PrepareOutput(outputPath string, workDir string, maxSize int64) (*output.Capture, error) {
	args := m.Called(outputPath, workDir, maxSize)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*output.Capture), args.Error(1)
}

func (m *MockCaptureManager) ValidateOutputPath(outputPath string, workDir string) error {
	args := m.Called(outputPath, workDir)
	return args.Error(0)
}

func (m *MockCaptureManager) WriteOutput(capture *output.Capture, data []byte) error {
	args := m.Called(capture, data)
	return args.Error(0)
}

func (m *MockCaptureManager) FinalizeOutput(capture *output.Capture) error {
	args := m.Called(capture)
	return args.Error(0)
}

func (m *MockCaptureManager) CleanupOutput(capture *output.Capture) error {
	args := m.Called(capture)
	return args.Error(0)
}

func (m *MockCaptureManager) AnalyzeOutput(outputPath string, workDir string) (*output.Analysis, error) {
	args := m.Called(outputPath, workDir)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*output.Analysis), args.Error(1)
}

// Test constants
const testTempPath = "/tmp/scr-test-group-12345"

// Test errors
var (
	ErrTestExecutionFailed = fmt.Errorf("execution failed")
)

// Test helper functions

// testResourceManagerFixture holds all mocks and the manager for testing
type testResourceManagerFixture struct {
	Manager       *NormalResourceManager
	MockExec      *executortesting.MockExecutor
	MockFS        *MockFileSystem
	MockPriv      *MockPrivilegeManager
	MockOutputMgr *MockCaptureManager
}

func createTestNormalResourceManager() *testResourceManagerFixture {
	mockExec := executortesting.NewMockExecutor()
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}
	mockOutputMgr := &MockCaptureManager{}

	manager := NewNormalResourceManagerWithOutput(mockExec, mockFS, mockPriv, mockOutputMgr, 1024*1024, slog.Default())

	return &testResourceManagerFixture{
		Manager:       manager,
		MockExec:      mockExec,
		MockFS:        mockFS,
		MockPriv:      mockPriv,
		MockOutputMgr: mockOutputMgr,
	}
}

func createTestCommandGroup() *runnertypes.GroupSpec {
	return &runnertypes.GroupSpec{
		Name:        "test-group",
		Description: "Test group description",
		WorkDir:     "/tmp",
		Commands:    []runnertypes.CommandSpec{},
	}
}

// Tests for Normal Resource Manager

func TestNormalResourceManager_ExecuteCommand(t *testing.T) {
	f := createTestNormalResourceManager()
	cmd := executortesting.CreateRuntimeCommand("echo", []string{"hello", "world"})
	group := createTestCommandGroup()
	env := map[string]string{"TEST": "value"}
	ctx := context.Background()

	expectedResult := &executor.Result{
		ExitCode: 0,
		Stdout:   "hello world",
		Stderr:   "",
	}

	f.MockExec.On("Execute", ctx, cmd, env, mock.Anything).Return(expectedResult, nil)

	_, result, err := f.Manager.ExecuteCommand(ctx, cmd, group, env)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ExitCode, result.ExitCode)
	assert.Equal(t, expectedResult.Stdout, result.Stdout)
	assert.Equal(t, expectedResult.Stderr, result.Stderr)
	assert.False(t, result.DryRun)
	assert.Nil(t, result.Analysis)

	f.MockExec.AssertExpectations(t)
}

func TestNormalResourceManager_ExecuteCommand_PrivilegeEscalationBlocked(t *testing.T) {
	f := createTestNormalResourceManager()

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
			cmd := executortesting.CreateRuntimeCommand(
				tc.cmd,
				tc.args,
				executortesting.WithName("test-privilege-command"),
				executortesting.WithWorkDir("/tmp"),
				executortesting.WithTimeout(commontesting.Int32Ptr(30)),
				executortesting.WithRiskLevel("low"),
			)
			group := createTestCommandGroup()
			env := map[string]string{"TEST": "value"}
			ctx := context.Background()

			_, result, err := f.Manager.ExecuteCommand(ctx, cmd, group, env)

			assert.Error(t, err)
			assert.Nil(t, result)
			// Unified approach: should be blocked by security violation, not critical risk error
			assert.ErrorIs(t, err, runnertypes.ErrCommandSecurityViolation)
		})
	}
}

func TestNormalResourceManager_ExecuteCommand_RiskLevelControl(t *testing.T) {
	f := createTestNormalResourceManager()
	group := createTestCommandGroup()
	env := map[string]string{"TEST": "value"}
	ctx := context.Background()

	testCases := []struct {
		name          string
		cmd           string
		args          []string
		riskLevel     string
		shouldExecute bool
		expectedError error
	}{
		{
			name:          "low risk command with no risk_level (default low)",
			cmd:           "echo",
			args:          []string{"hello"},
			riskLevel:     "", // Default to low
			shouldExecute: true,
		},
		{
			name:          "low risk command with low risk_level",
			cmd:           "echo",
			args:          []string{"hello"},
			riskLevel:     "low",
			shouldExecute: true,
		},
		{
			name:          "medium risk command with high risk_level",
			cmd:           "wget",
			args:          []string{"http://example.com/file.txt"},
			riskLevel:     "high",
			shouldExecute: true,
		},
		{
			name:          "high risk command with high risk_level",
			cmd:           "rm",
			args:          []string{"-rf", "/tmp/test"},
			riskLevel:     "high",
			shouldExecute: true,
		},
		{
			name:          "high risk command with low risk_level should be blocked",
			cmd:           "rm",
			args:          []string{"-rf", "/tmp/test"},
			riskLevel:     "low",
			shouldExecute: false,
			expectedError: runnertypes.ErrCommandSecurityViolation,
		},
		{
			name:          "medium risk command with low risk_level should be blocked",
			cmd:           "wget",
			args:          []string{"http://example.com/file.txt"},
			riskLevel:     "low",
			shouldExecute: false,
			expectedError: runnertypes.ErrCommandSecurityViolation,
		},
		{
			name:          "invalid risk_level should return error",
			cmd:           "echo",
			args:          []string{"hello"},
			riskLevel:     "invalid",
			shouldExecute: false,
			expectedError: runnertypes.ErrInvalidRiskLevel,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := executortesting.CreateRuntimeCommand(
				tc.cmd,
				tc.args,
				executortesting.WithName("test-command"),
				executortesting.WithWorkDir("/tmp"),
				executortesting.WithTimeout(commontesting.Int32Ptr(30)),
				executortesting.WithRiskLevel(tc.riskLevel),
			)

			if tc.shouldExecute {
				expectedResult := &executor.Result{
					ExitCode: 0,
					Stdout:   "success",
					Stderr:   "",
				}
				f.MockExec.On("Execute", ctx, cmd, env, mock.Anything).Return(expectedResult, nil).Once()
			}

			_, result, err := f.Manager.ExecuteCommand(ctx, cmd, group, env)

			if tc.shouldExecute {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tc.expectedError != nil {
					assert.ErrorIs(t, err, tc.expectedError)
				}
			}
		})
	}

	f.MockExec.AssertExpectations(t)
}

func TestNormalResourceManager_CreateTempDir(t *testing.T) {
	f := createTestNormalResourceManager()
	groupName := "test-group"
	expectedPath := testTempPath

	f.MockFS.On("CreateTempDir", "", fmt.Sprintf("scr-%s-", groupName)).Return(expectedPath, nil)

	path, err := f.Manager.CreateTempDir(groupName)

	assert.NoError(t, err)
	assert.Equal(t, expectedPath, path)

	// Check that path is tracked
	f.Manager.mu.RLock()
	assert.Contains(t, f.Manager.tempDirs, expectedPath)
	f.Manager.mu.RUnlock()

	f.MockFS.AssertExpectations(t)
}

func TestNormalResourceManager_CleanupTempDir(t *testing.T) {
	f := createTestNormalResourceManager()
	tempPath := testTempPath

	// Add path to tracking
	f.Manager.mu.Lock()
	f.Manager.tempDirs = append(f.Manager.tempDirs, tempPath)
	f.Manager.mu.Unlock()

	f.MockFS.On("RemoveAll", tempPath).Return(nil)

	err := f.Manager.CleanupTempDir(tempPath)

	assert.NoError(t, err)

	// Check that path is no longer tracked
	f.Manager.mu.RLock()
	assert.NotContains(t, f.Manager.tempDirs, tempPath)
	f.Manager.mu.RUnlock()

	f.MockFS.AssertExpectations(t)
}

func TestNormalResourceManager_WithPrivileges(t *testing.T) {
	f := createTestNormalResourceManager()
	ctx := context.Background()

	called := false
	fn := func() error {
		called = true
		return nil
	}

	f.MockPriv.On("WithPrivileges", mock.AnythingOfType("runnertypes.ElevationContext"), mock.AnythingOfType("func() error")).Return(nil).Run(func(args mock.Arguments) {
		// Call the provided function
		fnArg := args.Get(1).(func() error)
		fnArg()
	})

	err := f.Manager.WithPrivileges(ctx, fn)

	assert.NoError(t, err)
	assert.True(t, called)

	f.MockPriv.AssertExpectations(t)
}

func TestNormalResourceManager_SendNotification(t *testing.T) {
	f := createTestNormalResourceManager()
	message := "Test notification"
	details := map[string]any{"key": "value"}

	err := f.Manager.SendNotification(message, details)

	assert.NoError(t, err)
}

func TestNormalResourceManager_ValidateOutputPath_PathTraversal(t *testing.T) {
	f := createTestNormalResourceManager()
	workDir := "/tmp/workdir"

	tests := []struct {
		name        string
		outputPath  string
		workDir     string
		expectError bool
		errorType   error
	}{
		{
			name:        "path_traversal_parent_directory",
			outputPath:  "../../../etc/passwd",
			workDir:     workDir,
			expectError: true,
			errorType:   output.ErrPathTraversal,
		},
		{
			name:        "path_traversal_with_dots",
			outputPath:  "../../sensitive.txt",
			workDir:     workDir,
			expectError: true,
			errorType:   output.ErrPathEscapesWorkDirectory,
		},
		{
			name:        "path_traversal_absolute_with_dots",
			outputPath:  "/tmp/../etc/passwd",
			workDir:     workDir,
			expectError: false, // Absolute paths with .. are cleaned but allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock expectations based on test case
			if tt.expectError && tt.errorType != nil {
				f.MockOutputMgr.On("ValidateOutputPath", tt.outputPath, tt.workDir).Return(tt.errorType).Once()
			} else if !tt.expectError {
				f.MockOutputMgr.On("ValidateOutputPath", tt.outputPath, tt.workDir).Return(nil).Once()
			}

			err := f.Manager.ValidateOutputPath(tt.outputPath, tt.workDir)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
			}

			f.MockOutputMgr.AssertExpectations(t)
		})
	}
}

func TestNormalResourceManager_ValidateOutputPath_SymlinkAttack(t *testing.T) {
	t.Skip("Symlink attack tests require actual filesystem setup and are better tested in integration tests")
}

func TestNormalResourceManager_ValidateOutputPath_AbsolutePath(t *testing.T) {
	f := createTestNormalResourceManager()
	workDir := "/tmp/workdir"

	tests := []struct {
		name        string
		outputPath  string
		expectError bool
	}{
		{
			name:        "valid_absolute_path",
			outputPath:  "/tmp/output.txt",
			expectError: false,
		},
		{
			name:        "absolute_path_with_special_chars",
			outputPath:  "/tmp/output-file_123.txt",
			expectError: false,
		},
		{
			name:        "absolute_path_with_dangerous_chars",
			outputPath:  "/tmp/output;rm -rf.txt",
			expectError: true, // Should detect dangerous characters
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock expectations based on test case
			if tt.expectError {
				f.MockOutputMgr.On("ValidateOutputPath", tt.outputPath, workDir).Return(output.ErrDangerousCharactersInPath).Once()
			} else {
				f.MockOutputMgr.On("ValidateOutputPath", tt.outputPath, workDir).Return(nil).Once()
			}

			err := f.Manager.ValidateOutputPath(tt.outputPath, workDir)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			f.MockOutputMgr.AssertExpectations(t)
		})
	}
}

func TestNormalResourceManager_ValidateOutputPath_RelativePath(t *testing.T) {
	f := createTestNormalResourceManager()
	workDir := "/tmp/workdir"

	tests := []struct {
		name        string
		outputPath  string
		workDir     string
		expectError bool
	}{
		{
			name:        "valid_relative_path",
			outputPath:  "output/result.txt",
			workDir:     workDir,
			expectError: false,
		},
		{
			name:        "relative_path_single_level",
			outputPath:  "result.txt",
			workDir:     workDir,
			expectError: false,
		},
		{
			name:        "relative_path_with_current_dir",
			outputPath:  "./output/result.txt",
			workDir:     workDir,
			expectError: false,
		},
		{
			name:        "relative_path_without_workdir",
			outputPath:  "output/result.txt",
			workDir:     "",
			expectError: true, // Should require workdir for relative paths
		},
		{
			name:        "empty_output_path",
			outputPath:  "",
			workDir:     workDir,
			expectError: false, // Empty path is allowed (no output capture)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock expectations based on test case
			// Empty path returns early without calling outputManager
			if tt.outputPath != "" {
				if tt.expectError {
					f.MockOutputMgr.On("ValidateOutputPath", tt.outputPath, tt.workDir).Return(output.ErrWorkDirRequired).Once()
				} else {
					f.MockOutputMgr.On("ValidateOutputPath", tt.outputPath, tt.workDir).Return(nil).Once()
				}
			}

			err := f.Manager.ValidateOutputPath(tt.outputPath, tt.workDir)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			f.MockOutputMgr.AssertExpectations(t)
		})
	}
}
