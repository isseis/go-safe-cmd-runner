package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	securitytesting "github.com/isseis/go-safe-cmd-runner/internal/runner/security/testing"
	runnertesting "github.com/isseis/go-safe-cmd-runner/internal/runner/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	verificationtesting "github.com/isseis/go-safe-cmd-runner/internal/verification/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestCreateCommandContext tests the createCommandContext method
func TestCreateCommandContext(t *testing.T) {
	tests := []struct {
		name            string
		globalTimeout   *int
		commandTimeout  *int
		expectedTimeout time.Duration
	}{
		{
			name:            "use global timeout when command timeout is not set",
			globalTimeout:   common.IntPtr(30),
			commandTimeout:  nil,
			expectedTimeout: 30 * time.Second,
		},
		{
			name:            "use command timeout when set",
			globalTimeout:   common.IntPtr(30),
			commandTimeout:  common.IntPtr(60),
			expectedTimeout: 60 * time.Second,
		},
		{
			name:            "command timeout overrides global timeout",
			globalTimeout:   common.IntPtr(120),
			commandTimeout:  common.IntPtr(10),
			expectedTimeout: 10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: tt.globalTimeout,
				},
			}

			ge := &DefaultGroupExecutor{
				config: config,
			}

			cmd := &runnertypes.RuntimeCommand{
				Spec: &runnertypes.CommandSpec{
					Timeout: tt.commandTimeout,
				},
				EffectiveTimeout: int(tt.expectedTimeout.Seconds()),
			}

			ctx := context.Background()
			now := time.Now()
			cmdCtx, cancel := ge.createCommandContext(ctx, cmd)
			defer cancel()

			// Verify deadline is set correctly
			deadline, ok := cmdCtx.Deadline()
			require.True(t, ok, "context should have a deadline")

			// Check that deadline is approximately correct (within 100ms tolerance)
			expectedDeadline := now.Add(tt.expectedTimeout)
			timeDiff := deadline.Sub(expectedDeadline)
			assert.Less(t, timeDiff.Abs(), 100*time.Millisecond,
				"deadline should be within 100ms of expected value")
		})
	}
}

// TestCreateCommandContext_UnlimitedTimeout tests unlimited timeout handling (T1.1)
func TestCreateCommandContext_UnlimitedTimeout(t *testing.T) {
	tests := []struct {
		name             string
		effectiveTimeout int
	}{
		{
			name:             "zero timeout means unlimited",
			effectiveTimeout: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ge := &DefaultGroupExecutor{}

			cmd := &runnertypes.RuntimeCommand{
				Spec: &runnertypes.CommandSpec{
					Name: "unlimited-cmd",
				},
				EffectiveTimeout: tt.effectiveTimeout,
			}

			ctx := context.Background()
			cmdCtx, cancel := ge.createCommandContext(ctx, cmd)
			defer cancel()

			// Verify no deadline is set (unlimited execution)
			_, ok := cmdCtx.Deadline()
			assert.False(t, ok, "context should not have a deadline for unlimited timeout")

			// Verify context is cancellable (context.WithCancel was used)
			assert.NotNil(t, cmdCtx, "context should not be nil")
			assert.NotNil(t, cancel, "cancel function should not be nil")
		})
	}
}

// TestCreateCommandContext_NegativeTimeoutPanic tests that negative timeout causes panic
func TestCreateCommandContext_NegativeTimeoutPanic(t *testing.T) {
	ge := &DefaultGroupExecutor{}

	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name: "negative-timeout-cmd",
		},
		EffectiveTimeout: -1,
	}

	ctx := context.Background()

	// Verify that createCommandContext panics with negative timeout
	assert.PanicsWithValue(t,
		"program error: negative timeout -1 for command negative-timeout-cmd",
		func() {
			_, _ = ge.createCommandContext(ctx, cmd)
		},
		"createCommandContext should panic with negative timeout")
}

// TestExecuteGroup_WorkDirPriority tests the working directory priority logic
// Note: TempDir functionality is currently not implemented in GroupSpec, so these tests are skipped
func TestExecuteGroup_WorkDirPriority(t *testing.T) {
	t.Skip("TempDir functionality is not implemented in GroupSpec yet")
	tests := []struct {
		name               string
		groupTempDir       bool
		groupWorkDir       string
		commandDir         string
		expectedTempDir    string
		expectedCommandDir string
	}{
		{
			name:               "command dir takes precedence over everything",
			groupTempDir:       true,
			groupWorkDir:       "/group/work",
			commandDir:         "/cmd/dir",
			expectedTempDir:    "/tmp/test-group",
			expectedCommandDir: "/cmd/dir",
		},
		{
			name:               "temp dir takes precedence when command dir is empty",
			groupTempDir:       true,
			groupWorkDir:       "/group/work",
			commandDir:         "",
			expectedTempDir:    "/tmp/test-group",
			expectedCommandDir: "/tmp/test-group",
		},
		{
			name:               "group workdir used when no temp dir and no command dir",
			groupTempDir:       false,
			groupWorkDir:       "/group/work",
			commandDir:         "",
			expectedTempDir:    "",
			expectedCommandDir: "/group/work",
		},
		{
			name:               "no dir set when all are empty",
			groupTempDir:       false,
			groupWorkDir:       "",
			commandDir:         "",
			expectedTempDir:    "",
			expectedCommandDir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRM := new(runnertesting.MockResourceManager)

			config := &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: common.IntPtr(30),
				},
			}

			ge := NewTestGroupExecutor(config, mockRM)

			group := &runnertypes.GroupSpec{
				Name:    "test-group",
				WorkDir: tt.groupWorkDir,
				Commands: []runnertypes.CommandSpec{
					{
						Name:    "test-cmd",
						Cmd:     "/bin/echo",
						WorkDir: tt.commandDir,
					},
				},
			}

			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
			}

			// Setup mocks
			if tt.groupTempDir {
				mockRM.On("CreateTempDir", "test-group").Return(tt.expectedTempDir, nil)
				mockRM.On("CleanupTempDir", tt.expectedTempDir).Return(nil)
			}

			mockRM.On("ValidateOutputPath", mock.Anything, mock.Anything).Return(nil).Maybe()
			mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
				&resource.ExecutionResult{ExitCode: 0, Stdout: "", Stderr: ""}, nil)

			ctx := context.Background()
			err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

			require.NoError(t, err)

			// Verify cleanup was called if temp dir was created
			if tt.groupTempDir {
				mockRM.AssertCalled(t, "CleanupTempDir", tt.expectedTempDir)
			}

			// Verify the command was executed with the correct effective working directory
			mockRM.AssertCalled(t, "ExecuteCommand", mock.Anything,
				mock.MatchedBy(func(cmd *runnertypes.RuntimeCommand) bool {
					return cmd.EffectiveWorkDir == tt.expectedCommandDir
				}), mock.Anything, mock.Anything)
		})
	}
}

// TestExecuteGroup_TempDirCleanup tests that temp directories are cleaned up properly
// Note: TempDir functionality is currently not implemented in GroupSpec, so these tests are skipped
func TestExecuteGroup_TempDirCleanup(t *testing.T) {
	t.Skip("TempDir functionality is not implemented in GroupSpec yet")
	tests := []struct {
		name           string
		executionError error
		cleanupError   error
		expectCleanup  bool
	}{
		{
			name:           "cleanup on success",
			executionError: nil,
			cleanupError:   nil,
			expectCleanup:  true,
		},
		{
			name:           "cleanup on execution failure",
			executionError: errors.New("command failed"),
			cleanupError:   nil,
			expectCleanup:  true,
		},
		{
			name:           "cleanup even when cleanup fails",
			executionError: nil,
			cleanupError:   errors.New("cleanup failed"),
			expectCleanup:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRM := new(runnertesting.MockResourceManager)

			config := &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: common.IntPtr(30),
				},
			}

			ge := NewTestGroupExecutor(config, mockRM)

			group := &runnertypes.GroupSpec{
				Name: "test-group",
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test-cmd",
						Cmd:  "/bin/echo",
					},
				},
			}

			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
			}

			// Setup mocks
			tempDirPath := "/tmp/test-group"
			mockRM.On("CreateTempDir", "test-group").Return(tempDirPath, nil)
			mockRM.On("CleanupTempDir", tempDirPath).Return(tt.cleanupError)

			// Mock execution
			if tt.executionError != nil {
				mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
					nil, tt.executionError)
			} else {
				mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
					&resource.ExecutionResult{ExitCode: 0, Stdout: "", Stderr: ""}, nil)
			}
			mockRM.On("ValidateOutputPath", mock.Anything, mock.Anything).Return(nil).Maybe()

			ctx := context.Background()
			err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

			if tt.executionError != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify cleanup was called regardless of execution result
			if tt.expectCleanup {
				mockRM.AssertCalled(t, "CleanupTempDir", tempDirPath)
			}
		})
	}
}

// TestExecuteGroup_CreateTempDirFailure tests error handling when temp dir creation fails
// Note: TempDir functionality is currently not implemented in GroupSpec, so this test is skipped
func TestExecuteGroup_CreateTempDirFailure(t *testing.T) {
	t.Skip("TempDir functionality is not implemented in GroupSpec yet")
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	ge := NewTestGroupExecutor(config, mockRM)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/echo",
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
	}

	// Setup mock to fail temp dir creation
	expectedErr := errors.New("disk full")
	mockRM.On("CreateTempDir", "test-group").Return("", expectedErr)

	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create temp directory")
	assert.ErrorIs(t, err, expectedErr)
}

// TestExecuteGroup_CommandExecutionFailure tests error handling when command execution fails
func TestExecuteGroup_CommandExecutionFailure(t *testing.T) {
	mockRM := new(runnertesting.MockResourceManager)
	mockValidator := new(securitytesting.MockValidator)
	mockVerificationManager := new(verificationtesting.MockManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	var capturedNotification *groupExecutionResult
	notificationFunc := func(_ *runnertypes.GroupSpec, result *groupExecutionResult, _ time.Duration) {
		capturedNotification = result
	}

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVerificationManager,
			ResourceManager:     mockRM,
		},
		WithGroupNotificationFunc(notificationFunc),
	)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/false",
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
	}

	// Mock validator to allow all validations
	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)

	// Mock verification manager to verify group files and resolve paths
	mockVerificationManager.On("VerifyGroupFiles", mock.Anything).Return(&verification.Result{}, nil)
	mockVerificationManager.On("ResolvePath", "/bin/false").Return("/bin/false", nil)

	// Mock execution to return non-zero exit code
	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		&resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "command failed"}, nil)
	mockRM.On("ValidateOutputPath", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandFailed)

	// Verify notification was sent with error status
	require.NotNil(t, capturedNotification)
	assert.Equal(t, GroupExecutionStatusError, capturedNotification.status)
	assert.Equal(t, 1, capturedNotification.exitCode)
	assert.Equal(t, "test-cmd", capturedNotification.lastCommand)
}

// TestExecuteGroup_CommandExecutionFailure_NonStandardExitCode tests that non-standard exit codes are preserved
func TestExecuteGroup_CommandExecutionFailure_NonStandardExitCode(t *testing.T) {
	mockRM := new(runnertesting.MockResourceManager)
	mockValidator := new(securitytesting.MockValidator)
	mockVerificationManager := new(verificationtesting.MockManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	var capturedNotification *groupExecutionResult
	notificationFunc := func(_ *runnertypes.GroupSpec, result *groupExecutionResult, _ time.Duration) {
		capturedNotification = result
	}

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVerificationManager,
			ResourceManager:     mockRM,
		},
		WithGroupNotificationFunc(notificationFunc),
	)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/some-command",
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
	}

	// Mock validator to allow all validations
	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)

	// Mock verification manager to verify group files and resolve paths
	mockVerificationManager.On("VerifyGroupFiles", mock.Anything).Return(&verification.Result{}, nil)
	mockVerificationManager.On("ResolvePath", "/bin/some-command").Return("/bin/some-command", nil)

	// Mock execution to return exit code 127 (command not found)
	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		&resource.ExecutionResult{ExitCode: 127, Stdout: "", Stderr: "command not found"}, nil)
	mockRM.On("ValidateOutputPath", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandFailed)

	// Verify notification was sent with error status and correct exit code
	require.NotNil(t, capturedNotification)
	assert.Equal(t, GroupExecutionStatusError, capturedNotification.status)
	assert.Equal(t, 127, capturedNotification.exitCode)
	assert.Equal(t, "test-cmd", capturedNotification.lastCommand)
}

// TestExecuteGroup_SuccessNotification tests that success notification is sent properly
func TestExecuteGroup_SuccessNotification(t *testing.T) {
	mockRM := new(runnertesting.MockResourceManager)
	mockValidator, mockVerificationManager := setupMocksForTest(t)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	var capturedNotification *groupExecutionResult
	var capturedDuration time.Duration
	notificationFunc := func(_ *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration) {
		capturedNotification = result
		capturedDuration = duration
	}

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVerificationManager,
			ResourceManager:     mockRM,
		},
		WithGroupNotificationFunc(notificationFunc),
	)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/echo",
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
	}

	// Mock verification manager to resolve paths
	mockVerificationManager.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		&resource.ExecutionResult{ExitCode: 0, Stdout: "success", Stderr: ""}, nil)
	mockRM.On("ValidateOutputPath", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx := context.Background()
	startTime := time.Now()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)
	endTime := time.Now()

	require.NoError(t, err)

	// Verify notification was sent with success status
	require.NotNil(t, capturedNotification)
	assert.Equal(t, GroupExecutionStatusSuccess, capturedNotification.status)
	assert.Equal(t, 0, capturedNotification.exitCode)
	assert.Equal(t, "test-cmd", capturedNotification.lastCommand)
	assert.Equal(t, "success", capturedNotification.output)
	assert.Empty(t, capturedNotification.errorMsg)

	// Verify duration is reasonable
	assert.True(t, capturedDuration >= 0)
	assert.True(t, capturedDuration <= endTime.Sub(startTime)+100*time.Millisecond)
}

// TestExecuteCommandInGroup_OutputPathValidationFailure tests error handling for output path validation
func TestExecuteCommandInGroup_OutputPathValidationFailure(t *testing.T) {
	mockRM := new(runnertesting.MockResourceManager)
	mockValidator, mockVerificationManager := setupMocksForTest(t)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVerificationManager,
			ResourceManager:     mockRM,
		},
	)

	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name:       "test-cmd",
			Cmd:        "/bin/echo",
			OutputFile: "/invalid/output/path",
		},
		ExpandedCmd:  "/bin/echo",
		ExpandedArgs: []string{},
	}

	groupSpec := &runnertypes.GroupSpec{
		Name:    "test-group",
		WorkDir: "/work",
	}

	runtimeGroup, err := runnertypes.NewRuntimeGroup(groupSpec)
	require.NoError(t, err)

	// Mock verification manager to resolve paths
	mockVerificationManager.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	expectedErr := errors.New("output path is outside work directory")
	mockRM.On("ValidateOutputPath", "/invalid/output/path", "/work").Return(expectedErr)

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
	}

	ctx := context.Background()
	result, err := ge.executeCommandInGroup(ctx, cmd, groupSpec, runtimeGroup, runtimeGlobal)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "output path validation failed")
	assert.ErrorIs(t, err, expectedErr)
}

// TestExecuteGroup_MultipleCommands tests execution of multiple commands in sequence
func TestExecuteGroup_MultipleCommands(t *testing.T) {
	mockRM := new(runnertesting.MockResourceManager)
	mockValidator, mockVerificationManager := setupMocksForTest(t)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVerificationManager,
			ResourceManager:     mockRM,
		},
	)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "cmd1",
				Cmd:  "/bin/echo",
			},
			{
				Name: "cmd2",
				Cmd:  "/bin/echo",
			},
			{
				Name: "cmd3",
				Cmd:  "/bin/echo",
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
	}

	// Mock verification manager to resolve paths (all commands use /bin/echo)
	mockVerificationManager.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	// Mock all executions
	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		&resource.ExecutionResult{ExitCode: 0, Stdout: "ok", Stderr: ""}, nil)
	mockRM.On("ValidateOutputPath", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.NoError(t, err)

	// Verify all three commands were executed
	assert.Equal(t, 3, len(mockRM.Calls))
}

// TestExecuteGroup_StopOnFirstFailure tests that execution stops on first command failure
func TestExecuteGroup_StopOnFirstFailure(t *testing.T) {
	mockRM := new(runnertesting.MockResourceManager)
	mockValidator, mockVerificationManager := setupMocksForTest(t)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVerificationManager,
			ResourceManager:     mockRM,
		},
	)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "cmd1",
				Cmd:  "/bin/true",
			},
			{
				Name: "cmd2-fails",
				Cmd:  "/bin/false",
			},
			{
				Name: "cmd3-should-not-run",
				Cmd:  "/bin/echo",
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
	}

	// Mock verification manager to resolve paths for all commands
	mockVerificationManager.On("ResolvePath", "/bin/true").Return("/bin/true", nil)
	mockVerificationManager.On("ResolvePath", "/bin/false").Return("/bin/false", nil)
	mockVerificationManager.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	// First command succeeds
	mockRM.On("ExecuteCommand", mock.Anything,
		mock.MatchedBy(func(cmd *runnertypes.RuntimeCommand) bool {
			return cmd.Name() == "cmd1"
		}), mock.Anything, mock.Anything).Return(
		&resource.ExecutionResult{ExitCode: 0, Stdout: "", Stderr: ""}, nil).Once()

	// Second command fails
	mockRM.On("ExecuteCommand", mock.Anything,
		mock.MatchedBy(func(cmd *runnertypes.RuntimeCommand) bool {
			return cmd.Name() == "cmd2-fails"
		}), mock.Anything, mock.Anything).Return(
		&resource.ExecutionResult{ExitCode: 1, Stdout: "", Stderr: "error"}, nil).Once()

	mockRM.On("ValidateOutputPath", mock.Anything, mock.Anything).Return(nil).Maybe()

	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandFailed)

	// Verify that cmd3 was NOT executed (only cmd1 and cmd2)
	executeCalls := 0
	for _, call := range mockRM.Calls {
		if call.Method == "ExecuteCommand" {
			executeCalls++
		}
	}
	assert.Equal(t, 2, executeCalls, "should stop after second command fails")
}

// TestResolveGroupWorkDir tests the resolveGroupWorkDir method
func TestResolveGroupWorkDir(t *testing.T) {
	tests := []struct {
		name            string
		groupWorkDir    string
		groupVars       map[string]string
		isDryRun        bool
		expectTempDir   bool
		expectError     bool
		expectedWorkDir string // For fixed workdir cases
	}{
		{
			name:            "fixed workdir specified",
			groupWorkDir:    "/opt/app",
			groupVars:       map[string]string{},
			isDryRun:        false,
			expectTempDir:   false,
			expectError:     false,
			expectedWorkDir: "/opt/app",
		},
		{
			name:            "workdir with variable expansion",
			groupWorkDir:    "/opt/%{project}",
			groupVars:       map[string]string{"project": "myapp"},
			isDryRun:        false,
			expectTempDir:   false,
			expectError:     false,
			expectedWorkDir: "/opt/myapp",
		},
		{
			name:          "no workdir - temp dir created (normal mode)",
			groupWorkDir:  "",
			groupVars:     map[string]string{},
			isDryRun:      false,
			expectTempDir: true,
			expectError:   false,
		},
		{
			name:          "no workdir - temp dir created (dry-run mode)",
			groupWorkDir:  "",
			groupVars:     map[string]string{},
			isDryRun:      true,
			expectTempDir: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ge := &DefaultGroupExecutor{
				isDryRun: tt.isDryRun,
			}

			runtimeGroup := &runnertypes.RuntimeGroup{
				Spec: &runnertypes.GroupSpec{
					Name:    "test-group",
					WorkDir: tt.groupWorkDir,
				},
				ExpandedVars: tt.groupVars,
			}

			workDir, tempDirMgr, err := ge.resolveGroupWorkDir(runtimeGroup)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, workDir)

			if tt.expectTempDir {
				require.NotNil(t, tempDirMgr, "temp dir manager should be non-nil for temp directories")
				assert.Contains(t, workDir, "scr-test-group", "temp dir should contain group name")
				if tt.isDryRun {
					assert.Contains(t, workDir, "dryrun", "dry-run temp dir should contain 'dryrun'")
				}
			} else {
				assert.Nil(t, tempDirMgr, "temp dir manager should be nil for fixed directories")
				assert.Equal(t, tt.expectedWorkDir, workDir)
			}
		})
	}
}

// TestResolveCommandWorkDir tests the resolveCommandWorkDir method
func TestResolveCommandWorkDir(t *testing.T) {
	tests := []struct {
		name                  string
		commandWorkDir        string
		commandVars           map[string]string
		groupEffectiveWorkDir string
		expectedWorkDir       string
		expectError           bool
	}{
		{
			name:                  "command workdir takes priority",
			commandWorkDir:        "/cmd/workdir",
			commandVars:           map[string]string{},
			groupEffectiveWorkDir: "/group/workdir",
			expectedWorkDir:       "/cmd/workdir",
			expectError:           false,
		},
		{
			name:                  "use group workdir when command workdir is empty",
			commandWorkDir:        "",
			commandVars:           map[string]string{},
			groupEffectiveWorkDir: "/group/workdir",
			expectedWorkDir:       "/group/workdir",
			expectError:           false,
		},
		{
			name:                  "both empty returns empty",
			commandWorkDir:        "",
			commandVars:           map[string]string{},
			groupEffectiveWorkDir: "",
			expectedWorkDir:       "",
			expectError:           false,
		},
		{
			name:                  "command workdir with variable expansion",
			commandWorkDir:        "/opt/%{project}",
			commandVars:           map[string]string{"project": "myapp"},
			groupEffectiveWorkDir: "/group/workdir",
			expectedWorkDir:       "/opt/myapp",
			expectError:           false,
		},
		{
			name:                  "variable expansion error stops execution",
			commandWorkDir:        "/opt/%{undefined_var}",
			commandVars:           map[string]string{},
			groupEffectiveWorkDir: "/group/workdir",
			expectedWorkDir:       "",
			expectError:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ge := &DefaultGroupExecutor{}

			runtimeCmd := &runnertypes.RuntimeCommand{
				Spec: &runnertypes.CommandSpec{
					Name:    "test-cmd",
					WorkDir: tt.commandWorkDir,
				},
				ExpandedVars: tt.commandVars,
			}

			runtimeGroup := &runnertypes.RuntimeGroup{
				EffectiveWorkDir: tt.groupEffectiveWorkDir,
			}

			result, err := ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedWorkDir, result)
		})
	}
}

// TestExecuteGroup_RunnerWorkdirExpansion tests the __runner_workdir variable expansion
func TestExecuteGroup_RunnerWorkdirExpansion(t *testing.T) {
	tests := []struct {
		name               string
		groupWorkDir       string
		commandWorkDir     string // Command-level workdir
		commandArgs        []string
		isDryRun           bool
		expectedWorkDir    string
		expectedArgPattern string // Pattern to match in expanded args
		expectedCmdWorkDir string // Expected expanded command workdir
	}{
		{
			name:               "fixed workdir with __runner_workdir in args",
			groupWorkDir:       "/opt/app",
			commandArgs:        []string{"echo", "%{__runner_workdir}/output.txt"},
			isDryRun:           false,
			expectedWorkDir:    "/opt/app",
			expectedArgPattern: "/opt/app/output.txt",
		},
		{
			name:               "temp dir with __runner_workdir in args",
			groupWorkDir:       "", // Use temp dir
			commandArgs:        []string{"mkdir", "-p", "%{__runner_workdir}/backup"},
			isDryRun:           false,
			expectedWorkDir:    "",        // Will be temp dir (verified by pattern)
			expectedArgPattern: "/backup", // Will match temp path ending
		},
		{
			name:               "dry-run mode with __runner_workdir",
			groupWorkDir:       "",
			commandArgs:        []string{"touch", "%{__runner_workdir}/test.log"},
			isDryRun:           true,
			expectedWorkDir:    "",        // Will be virtual temp dir
			expectedArgPattern: "dryrun-", // Will contain dryrun in path
		},
		{
			name:               "command workdir with __runner_workdir",
			groupWorkDir:       "/opt/app",
			commandWorkDir:     "%{__runner_workdir}/src",
			commandArgs:        []string{"make", "build"},
			isDryRun:           false,
			expectedWorkDir:    "/opt/app",
			expectedCmdWorkDir: "/opt/app/src",
		},
		{
			name:               "command workdir with __runner_workdir in temp dir",
			groupWorkDir:       "", // Use temp dir
			commandWorkDir:     "%{__runner_workdir}/build",
			commandArgs:        []string{"cmake", ".."},
			isDryRun:           false,
			expectedWorkDir:    "",       // Will be temp dir
			expectedCmdWorkDir: "/build", // Pattern to verify
		},
		{
			name:               "dry-run with command workdir using __runner_workdir",
			groupWorkDir:       "",
			commandWorkDir:     "%{__runner_workdir}/test",
			commandArgs:        []string{"pytest"},
			isDryRun:           true,
			expectedWorkDir:    "",        // Will be virtual temp dir
			expectedCmdWorkDir: "dryrun-", // Pattern to verify
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockExecutor := new(runnertesting.MockResourceManager)
			mockNotificationFunc := func(_ *runnertypes.GroupSpec, _ *groupExecutionResult, _ time.Duration) {
				// Test notification function - no-op
			}

			configSpec := &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: common.IntPtr(30),
				},
			}

			var geOptions []GroupExecutorOption
			geOptions = append(geOptions, WithGroupNotificationFunc(mockNotificationFunc))
			if tt.isDryRun {
				geOptions = append(geOptions, WithGroupDryRun(&resource.DryRunOptions{
					DetailLevel:   resource.DetailLevelSummary,
					ShowSensitive: false,
				}))
			}

			ge := NewTestGroupExecutorWithConfig(
				TestGroupExecutorConfig{
					Config:          configSpec,
					ResourceManager: mockExecutor,
				},
				geOptions...,
			)

			group := &runnertypes.GroupSpec{
				Name:    "test-group",
				WorkDir: tt.groupWorkDir,
				Commands: []runnertypes.CommandSpec{
					{
						Name:    "test-cmd",
						Cmd:     "echo",
						Args:    tt.commandArgs,
						WorkDir: tt.commandWorkDir,
					},
				},
			}

			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
				ExpandedVars: map[string]string{},
			}

			// We cannot easily test the full ExecuteGroup without mocking the entire execution stack
			// Instead, let's test the workdir resolution and variable setting directly

			// 1. Test group workdir resolution
			runtimeGroup, err := config.ExpandGroup(group, runtimeGlobal)
			require.NoError(t, err)

			workDir, tempDirMgr, err := ge.resolveGroupWorkDir(runtimeGroup)
			require.NoError(t, err)

			if tt.expectedWorkDir != "" {
				assert.Equal(t, tt.expectedWorkDir, workDir)
			} else {
				// For temp dirs, just verify it's not empty
				assert.NotEmpty(t, workDir)
			}

			// 2. Test that __runner_workdir is set correctly
			runtimeGroup.EffectiveWorkDir = workDir
			if runtimeGroup.ExpandedVars == nil {
				runtimeGroup.ExpandedVars = make(map[string]string)
			}
			runtimeGroup.ExpandedVars["__runner_workdir"] = workDir

			// 3. Test command expansion with __runner_workdir
			cmdSpec := &group.Commands[0]
			runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout())
			require.NoError(t, err)

			// Verify __runner_workdir was expanded in arguments
			require.NotEmpty(t, runtimeCmd.ExpandedArgs, "Command should have expanded args")

			// Find the argument that should contain the expanded workdir
			foundExpectedPattern := false
			for _, arg := range runtimeCmd.ExpandedArgs {
				if tt.expectedArgPattern != "" {
					if tt.expectedWorkDir != "" && arg == tt.expectedArgPattern {
						foundExpectedPattern = true
						break
					} else if tt.expectedWorkDir == "" && containsPattern(t, arg, tt.expectedArgPattern) {
						foundExpectedPattern = true
						break
					}
				}
			}

			if tt.expectedArgPattern != "" {
				assert.True(t, foundExpectedPattern,
					"Expected pattern '%s' not found in expanded args: %v",
					tt.expectedArgPattern, runtimeCmd.ExpandedArgs)
			}

			// 4. Verify command-level workdir expansion with __runner_workdir
			if tt.commandWorkDir != "" {
				// Manually expand command workdir (normally done by executor)
				expandedCmdWorkDir, err := config.ExpandString(
					cmdSpec.WorkDir,
					runtimeGroup.ExpandedVars,
					fmt.Sprintf("command[%s]", cmdSpec.Name),
					"workdir",
				)
				require.NoError(t, err, "Command workdir expansion should succeed")

				if tt.expectedWorkDir != "" {
					// Fixed path test - expect exact match
					assert.Equal(t, tt.expectedCmdWorkDir, expandedCmdWorkDir,
						"Command workdir should be expanded to expected value")
				} else {
					// Temp dir or dry-run test - expect pattern match
					assert.True(t, containsPattern(t, expandedCmdWorkDir, tt.expectedCmdWorkDir),
						"Command workdir should contain pattern '%s', got: %s",
						tt.expectedCmdWorkDir, expandedCmdWorkDir)
				}
			}

			// Cleanup temp dir if created
			if tempDirMgr != nil {
				tempDirMgr.Cleanup()
			}
		})
	}
}

// containsPattern checks if a string contains the expected pattern
func containsPattern(t *testing.T, s, pattern string) bool {
	t.Helper()
	require.NotEmpty(t, pattern, "pattern must not be empty")
	if len(s) == 0 {
		return false
	}

	// Check if pattern is anywhere in the string (for substrings like "dryrun-")
	return strings.Contains(s, pattern)
}

// TestExecuteCommandInGroup_ValidateEnvironmentVarsFailure tests environment variable validation error (T1.2)
func TestExecuteCommandInGroup_ValidateEnvironmentVarsFailure(t *testing.T) {
	// Arrange
	mockValidator := new(securitytesting.MockValidator)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	expectedErr := errors.New("dangerous pattern detected: rm -rf")
	mockValidator.On("ValidateAllEnvironmentVars",
		mock.MatchedBy(func(envVars map[string]string) bool {
			val, exists := envVars["DANGEROUS_VAR"]
			return exists && strings.Contains(val, "rm -rf")
		})).Return(expectedErr)

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:          config,
			Validator:       mockValidator,
			ResourceManager: mockRM,
		},
	)

	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name: "dangerous-cmd",
		},
		ExpandedCmd:  "/bin/echo",
		ExpandedArgs: []string{},
		ExpandedEnv: map[string]string{
			"DANGEROUS_VAR": "rm -rf /",
		},
		ExpandedVars: map[string]string{},
	}

	groupSpec := &runnertypes.GroupSpec{
		Name: "test-group",
	}

	runtimeGroup, err := runnertypes.NewRuntimeGroup(groupSpec)
	require.NoError(t, err)

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{},
	}

	// Act
	ctx := context.Background()
	result, err := ge.executeCommandInGroup(ctx, cmd, groupSpec, runtimeGroup, runtimeGlobal)

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "environment variables security validation failed")
	assert.ErrorIs(t, err, expectedErr)

	mockRM.AssertNotCalled(t, "ExecuteCommand")
	mockValidator.AssertExpectations(t)
}

// TestExecuteCommandInGroup_ResolvePathFailure tests path resolution error (T1.3)
func TestExecuteCommandInGroup_ResolvePathFailure(t *testing.T) {
	// Arrange
	mockValidator := new(securitytesting.MockValidator)
	mockVM := new(verificationtesting.MockManager)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	// Setup: validator passes
	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)

	// Setup: path resolution fails
	expectedErr := errors.New("command not found in PATH")
	mockVM.On("ResolvePath", "/nonexistent/command").Return("", expectedErr)

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVM,
			ResourceManager:     mockRM,
		},
	)

	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name: "test-cmd",
		},
		ExpandedCmd:  "/nonexistent/command",
		ExpandedArgs: []string{},
		ExpandedEnv:  map[string]string{},
		ExpandedVars: map[string]string{},
	}

	groupSpec := &runnertypes.GroupSpec{
		Name: "test-group",
	}

	runtimeGroup, err := runnertypes.NewRuntimeGroup(groupSpec)
	require.NoError(t, err)

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{},
	}

	// Act
	ctx := context.Background()
	result, err := ge.executeCommandInGroup(ctx, cmd, groupSpec, runtimeGroup, runtimeGlobal)

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "command path resolution failed")
	assert.ErrorIs(t, err, expectedErr)

	// Verify mocks
	mockRM.AssertNotCalled(t, "ExecuteCommand")
	mockVM.AssertCalled(t, "ResolvePath", "/nonexistent/command")
	mockValidator.AssertExpectations(t)
	mockVM.AssertExpectations(t)
}

// setupMocksForTest creates commonly needed mocks for testing
func setupMocksForTest(t *testing.T) (*securitytesting.MockValidator, *verificationtesting.MockManager) {
	t.Helper()
	mockValidator := new(securitytesting.MockValidator)
	mockVerificationManager := new(verificationtesting.MockManager)

	// Setup default behaviors for validator
	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil).Maybe()

	// Setup default behaviors for verification manager - return the input path as-is
	// Note: Cannot use dynamic return in Maybe() mocks, so we don't set up a default mock here.
	// Tests that need ResolvePath should set it up explicitly.

	// Setup default behavior for file verification - return empty Result
	mockVerificationManager.On("VerifyGroupFiles", mock.Anything).Return(&verification.Result{}, nil).Maybe()

	return mockValidator, mockVerificationManager
}

// TestExecuteCommandInGroup_DryRunDetailLevelFull tests dry-run with DetailLevelFull (T2.1)
func TestExecuteCommandInGroup_DryRunDetailLevelFull(t *testing.T) {
	// Arrange
	mockValidator, mockVM := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	// Setup: path resolution succeeds
	mockVM.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVM,
			ResourceManager:     mockRM,
		},
		WithGroupDryRun(&resource.DryRunOptions{
			DetailLevel:   resource.DetailLevelFull,
			ShowSensitive: false,
		}),
	)

	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name: "test-cmd",
		},
		ExpandedCmd:  "/bin/echo",
		ExpandedArgs: []string{},
		ExpandedEnv: map[string]string{
			"TEST_VAR": "test_value",
			"SECRET":   "secret_value",
		},
		ExpandedVars: map[string]string{},
	}

	groupSpec := &runnertypes.GroupSpec{
		Name: "test-group",
	}

	runtimeGroup, err := runnertypes.NewRuntimeGroup(groupSpec)
	require.NoError(t, err)

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{},
	}

	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "[DRY-RUN] output"}, nil)

	// Act
	ctx := context.Background()
	result, err := ge.executeCommandInGroup(ctx, cmd, groupSpec, runtimeGroup, runtimeGlobal)

	// Capture output
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify environment output
	assert.Contains(t, output, "Final Process Environment")
	assert.Contains(t, output, "TEST_VAR")
	assert.Contains(t, output, "test_value")
	assert.Contains(t, output, "SECRET")
	// Note: Masking behavior depends on redaction implementation

	mockRM.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
	mockVM.AssertExpectations(t)
}

// TestExecuteGroup_DryRunVariableExpansion tests dry-run variable expansion debug output (T2.2)
func TestExecuteGroup_DryRunVariableExpansion(t *testing.T) {
	// Arrange
	mockValidator, mockVM := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	// Setup: path resolution succeeds
	mockVM.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVM,
			ResourceManager:     mockRM,
		},
		WithGroupDryRun(&resource.DryRunOptions{
			DetailLevel:   resource.DetailLevelSummary,
			ShowSensitive: false,
		}),
	)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Vars: []string{"TEST_VAR=test_value"},
		Commands: []runnertypes.CommandSpec{
			{Name: "test-cmd", Cmd: "/bin/echo"},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{},
	}

	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&resource.ExecutionResult{ExitCode: 0}, nil)

	// Act
	err := ge.ExecuteGroup(context.Background(), group, runtimeGlobal)

	// Capture output
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Assert
	require.NoError(t, err)
	assert.Contains(t, output, "Variable Expansion Debug Information")

	mockRM.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
	mockVM.AssertExpectations(t)
}

// TestExecuteCommandInGroup_VerificationManagerNil tests path resolution skip when verificationManager is nil (T3.1)
func TestExecuteCommandInGroup_VerificationManagerNil(t *testing.T) {
	// Arrange
	mockValidator, _ := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	// verificationManager = nil (no path resolution)
	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:          config,
			Validator:       mockValidator,
			ResourceManager: mockRM,
		},
	)

	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name: "test-cmd",
		},
		ExpandedCmd:  "/bin/echo",
		ExpandedArgs: []string{"hello"},
		ExpandedVars: map[string]string{},
	}

	groupSpec := &runnertypes.GroupSpec{
		Name: "test-group",
	}

	runtimeGroup, err := runnertypes.NewRuntimeGroup(groupSpec)
	require.NoError(t, err)

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{},
	}

	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "hello"}, nil)

	// Act
	ctx := context.Background()
	result, err := ge.executeCommandInGroup(ctx, cmd, groupSpec, runtimeGroup, runtimeGlobal)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)

	// Verify that command executed without path resolution
	mockRM.AssertCalled(t, "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockRM.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

// TestExecuteGroup_KeepTempDirs tests that cleanup is skipped when keepTempDirs is true (T3.2)
func TestExecuteGroup_KeepTempDirs(t *testing.T) {
	// Arrange
	mockValidator, mockVM := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	// Setup: path resolution succeeds
	mockVM.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	// keepTempDirs = true
	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVM,
			ResourceManager:     mockRM,
		},
		WithGroupKeepTempDirs(true),
	)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{Name: "test-cmd", Cmd: "/bin/echo", Args: []string{"hello"}},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{},
	}

	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "hello"}, nil)

	// Act
	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	// Assert
	require.NoError(t, err)

	// Note: We cannot directly verify that Cleanup was not called because
	// the tempDirMgr is created internally. However, we verify that the
	// execution completes successfully with keepTempDirs=true
	mockRM.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
	mockVM.AssertExpectations(t)
}

// TestExecuteGroup_NoNotificationFunc tests that notification is skipped when notificationFunc is nil (T3.3)
func TestExecuteGroup_NoNotificationFunc(t *testing.T) {
	// Arrange
	mockValidator, mockVM := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	// Setup: path resolution succeeds
	mockVM.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	// notificationFunc = nil
	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVM,
			ResourceManager:     mockRM,
		},
	)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{Name: "test-cmd", Cmd: "/bin/echo", Args: []string{"hello"}},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{},
	}

	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "hello"}, nil)

	// Act
	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	// Assert
	require.NoError(t, err)

	// Verify that execution completed successfully without notification
	// (notificationFunc is nil, so no notification is sent)
	mockRM.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
	mockVM.AssertExpectations(t)
}

// TestExecuteGroup_EmptyDescription tests log output when Description is empty (T3.4)
func TestExecuteGroup_EmptyDescription(t *testing.T) {
	// Arrange
	mockValidator, mockVM := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	// Setup: path resolution succeeds
	mockVM.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVM,
			ResourceManager:     mockRM,
		},
	)

	// Group with empty description
	group := &runnertypes.GroupSpec{
		Name:        "test-group",
		Description: "", // Empty description
		Commands: []runnertypes.CommandSpec{
			{Name: "test-cmd", Cmd: "/bin/echo", Args: []string{"hello"}},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{},
	}

	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "hello"}, nil)

	// Act
	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	// Assert
	require.NoError(t, err)

	// With empty description, only group name is logged (not description)
	// We verify the execution completes successfully
	mockRM.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
	mockVM.AssertExpectations(t)
}

// TestExecuteGroup_VariableExpansionError tests variable expansion error in WorkDir (T3.5)
func TestExecuteGroup_VariableExpansionError(t *testing.T) {
	// Arrange
	mockValidator, _ := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:          config,
			Validator:       mockValidator,
			ResourceManager: mockRM,
		},
	)

	// Group with WorkDir containing undefined variable
	group := &runnertypes.GroupSpec{
		Name:    "test-group",
		WorkDir: "/tmp/%{UNDEFINED_VAR}/path", // Undefined variable
		Commands: []runnertypes.CommandSpec{
			{Name: "test-cmd", Cmd: "/bin/echo", Args: []string{"hello"}},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{}, // No UNDEFINED_VAR defined
	}

	// Act
	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undefined variable")
	assert.Contains(t, err.Error(), "UNDEFINED_VAR")

	// Verify that ExecuteCommand was not called due to early error
	mockRM.AssertNotCalled(t, "ExecuteCommand")
	mockValidator.AssertExpectations(t)
}

// TestExecuteGroup_FileVerificationResultLog tests file verification result logging (T3.6)
func TestExecuteGroup_FileVerificationResultLog(t *testing.T) {
	// Arrange
	mockValidator, mockVM := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	// Setup: path resolution succeeds
	mockVM.On("ResolvePath", "/bin/echo").Return("/bin/echo", nil)

	// Setup: VerifyGroupFiles returns a result with files verified
	verifyResult := &verification.Result{
		TotalFiles:    2,
		VerifiedFiles: 2,
		SkippedFiles:  []string{},
		Duration:      100 * time.Millisecond,
	}
	mockVM.On("VerifyGroupFiles", mock.Anything).Return(verifyResult, nil)

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVM,
			ResourceManager:     mockRM,
		},
	)

	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{Name: "test-cmd", Cmd: "/bin/echo", Args: []string{"hello"}},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{},
	}

	mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&resource.ExecutionResult{ExitCode: 0, Stdout: "hello"}, nil)

	// Act
	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	// Assert
	require.NoError(t, err)

	// Verify that file verification was performed and logged
	// Note: We can't directly capture log output, but we verify the call was made
	mockVM.AssertCalled(t, "VerifyGroupFiles", mock.Anything)
	mockRM.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
	mockVM.AssertExpectations(t)
}

// TestExecuteGroup_ExpandCommandError tests ExpandCommand error in command loop (T4.1)
func TestExecuteGroup_ExpandCommandError(t *testing.T) {
	// Arrange
	mockValidator, mockVM := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVM,
			ResourceManager:     mockRM,
		},
	)

	// Group with command containing undefined variable in Args
	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/echo",
				Args: []string{"%{UNDEFINED_VAR}"}, // Undefined variable in Args
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{}, // No UNDEFINED_VAR defined
	}

	// Act
	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to expand command")
	assert.Contains(t, err.Error(), "test-cmd")

	// Verify that ExecuteCommand was not called due to early error
	mockRM.AssertNotCalled(t, "ExecuteCommand")
	mockValidator.AssertExpectations(t)
	mockVM.AssertExpectations(t)
}

// TestExecuteGroup_ResolveCommandWorkDirError tests resolveCommandWorkDir error in command loop (T4.2)
func TestExecuteGroup_ResolveCommandWorkDirError(t *testing.T) {
	// Arrange
	mockValidator, mockVM := setupMocksForTest(t)
	mockRM := new(runnertesting.MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}

	ge := NewTestGroupExecutorWithConfig(
		TestGroupExecutorConfig{
			Config:              config,
			Validator:           mockValidator,
			VerificationManager: mockVM,
			ResourceManager:     mockRM,
		},
	)

	// Group with command-level WorkDir containing undefined variable
	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name:    "test-cmd",
				Cmd:     "/bin/echo",
				WorkDir: "/tmp/%{UNDEFINED_VAR}/path", // Undefined variable in command WorkDir
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{Timeout: common.IntPtr(30)},
		ExpandedVars: map[string]string{}, // No UNDEFINED_VAR defined
	}

	// Act
	ctx := context.Background()
	err := ge.ExecuteGroup(ctx, group, runtimeGlobal)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve command workdir")
	assert.Contains(t, err.Error(), "test-cmd")

	// Verify that ExecuteCommand was not called due to early error
	mockRM.AssertNotCalled(t, "ExecuteCommand")
	mockValidator.AssertExpectations(t)
	mockVM.AssertExpectations(t)
}

// TestGroupExecutorOptions tests the option functions for DefaultGroupExecutor
func TestGroupExecutorOptions(t *testing.T) {
	testNotificationFunc := func(_ *runnertypes.GroupSpec, _ *groupExecutionResult, _ time.Duration) {}

	tests := []struct {
		name    string
		options []GroupExecutorOption
		want    groupExecutorOptions
	}{
		{
			name:    "default options",
			options: nil,
			want: groupExecutorOptions{
				notificationFunc: nil,
				dryRunOptions:    nil,
				keepTempDirs:     false,
			},
		},
		{
			name: "with notification func",
			options: []GroupExecutorOption{
				WithGroupNotificationFunc(testNotificationFunc),
			},
			want: groupExecutorOptions{
				notificationFunc: testNotificationFunc,
				dryRunOptions:    nil,
				keepTempDirs:     false,
			},
		},
		{
			name: "with dry-run",
			options: []GroupExecutorOption{
				WithGroupDryRun(&resource.DryRunOptions{
					DetailLevel:   resource.DetailLevelFull,
					ShowSensitive: true,
				}),
			},
			want: groupExecutorOptions{
				notificationFunc: nil,
				dryRunOptions: &resource.DryRunOptions{
					DetailLevel:   resource.DetailLevelFull,
					ShowSensitive: true,
				},
				keepTempDirs: false,
			},
		},
		{
			name: "with keep temp dirs",
			options: []GroupExecutorOption{
				WithGroupKeepTempDirs(true),
			},
			want: groupExecutorOptions{
				notificationFunc: nil,
				dryRunOptions:    nil,
				keepTempDirs:     true,
			},
		},
		{
			name: "all options combined",
			options: []GroupExecutorOption{
				WithGroupNotificationFunc(testNotificationFunc),
				WithGroupDryRun(&resource.DryRunOptions{
					DetailLevel:   resource.DetailLevelSummary,
					ShowSensitive: false,
				}),
				WithGroupKeepTempDirs(true),
			},
			want: groupExecutorOptions{
				notificationFunc: testNotificationFunc,
				dryRunOptions: &resource.DryRunOptions{
					DetailLevel:   resource.DetailLevelSummary,
					ShowSensitive: false,
				},
				keepTempDirs: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultGroupExecutorOptions()
			for _, opt := range tt.options {
				opt(opts)
			}

			// Compare results (excluding function pointers)
			if tt.want.notificationFunc != nil {
				assert.NotNil(t, opts.notificationFunc)
			} else {
				assert.Nil(t, opts.notificationFunc)
			}
			assert.Equal(t, tt.want.dryRunOptions, opts.dryRunOptions)
			assert.Equal(t, tt.want.keepTempDirs, opts.keepTempDirs)
		})
	}
}

// TestNewDefaultGroupExecutor_WithOptions tests the new option-based constructor
func TestNewDefaultGroupExecutor_WithOptions(t *testing.T) {
	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}
	mockRM := new(runnertesting.MockResourceManager)
	testNotificationFunc := func(_ *runnertypes.GroupSpec, _ *groupExecutionResult, _ time.Duration) {}

	t.Run("default options", func(t *testing.T) {
		ge := NewDefaultGroupExecutor(
			nil, config, nil, nil, mockRM, "test-run-123",
		)

		assert.NotNil(t, ge)
		assert.Nil(t, ge.notificationFunc)
		assert.False(t, ge.isDryRun)
		assert.Equal(t, resource.DetailLevelSummary, ge.dryRunDetailLevel)
		assert.False(t, ge.dryRunShowSensitive)
		assert.False(t, ge.keepTempDirs)
	})

	t.Run("with notification func", func(t *testing.T) {
		ge := NewDefaultGroupExecutor(
			nil, config, nil, nil, mockRM, "test-run-123",
			WithGroupNotificationFunc(testNotificationFunc),
		)

		assert.NotNil(t, ge)
		assert.NotNil(t, ge.notificationFunc)
		assert.False(t, ge.isDryRun)
	})

	t.Run("with dry-run full", func(t *testing.T) {
		ge := NewDefaultGroupExecutor(
			nil, config, nil, nil, mockRM, "test-run-123",
			WithGroupDryRun(&resource.DryRunOptions{
				DetailLevel:   resource.DetailLevelFull,
				ShowSensitive: true,
			}),
		)

		assert.NotNil(t, ge)
		assert.True(t, ge.isDryRun)
		assert.Equal(t, resource.DetailLevelFull, ge.dryRunDetailLevel)
		assert.True(t, ge.dryRunShowSensitive)
	})

	t.Run("with all options", func(t *testing.T) {
		ge := NewDefaultGroupExecutor(
			nil, config, nil, nil, mockRM, "test-run-123",
			WithGroupNotificationFunc(testNotificationFunc),
			WithGroupDryRun(&resource.DryRunOptions{
				DetailLevel:   resource.DetailLevelSummary,
				ShowSensitive: false,
			}),
			WithGroupKeepTempDirs(true),
		)

		assert.NotNil(t, ge)
		assert.NotNil(t, ge.notificationFunc)
		assert.True(t, ge.isDryRun)
		assert.Equal(t, resource.DetailLevelSummary, ge.dryRunDetailLevel)
		assert.False(t, ge.dryRunShowSensitive)
		assert.True(t, ge.keepTempDirs)
	})
}

// TestNewDefaultGroupExecutor_Validation tests input validation
func TestNewDefaultGroupExecutor_Validation(t *testing.T) {
	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}
	mockRM := new(runnertesting.MockResourceManager)

	t.Run("nil config panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDefaultGroupExecutor(nil, nil, nil, nil, mockRM, "test-run-123")
		})
	})

	t.Run("nil resourceManager panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDefaultGroupExecutor(nil, config, nil, nil, nil, "test-run-123")
		})
	})

	t.Run("empty runID panics", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDefaultGroupExecutor(nil, config, nil, nil, mockRM, "")
		})
	})
}

// TestNewTestGroupExecutor tests the test helper function
func TestNewTestGroupExecutor(t *testing.T) {
	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}
	mockRM := new(runnertesting.MockResourceManager)

	t.Run("basic helper", func(t *testing.T) {
		ge := NewTestGroupExecutor(config, mockRM)

		assert.NotNil(t, ge)
		assert.Nil(t, ge.executor)
		assert.Nil(t, ge.validator)
		assert.Nil(t, ge.verificationManager)
		assert.Equal(t, "test-run-123", ge.runID)
		assert.False(t, ge.isDryRun)
	})

	t.Run("helper with options", func(t *testing.T) {
		testNotificationFunc := func(_ *runnertypes.GroupSpec, _ *groupExecutionResult, _ time.Duration) {}

		ge := NewTestGroupExecutor(
			config, mockRM,
			WithGroupNotificationFunc(testNotificationFunc),
			WithGroupKeepTempDirs(true),
		)

		assert.NotNil(t, ge)
		assert.NotNil(t, ge.notificationFunc)
		assert.True(t, ge.keepTempDirs)
	})
}

// TestNewDefaultGroupExecutor_Performance tests allocation behavior
func TestNewDefaultGroupExecutor_Performance(t *testing.T) {
	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}
	mockRM := new(runnertesting.MockResourceManager)

	// Test allocation count
	allocs := testing.AllocsPerRun(100, func() {
		_ = NewDefaultGroupExecutor(
			nil, config, nil, nil, mockRM, "perf-test",
			WithGroupKeepTempDirs(false),
		)
	})

	// Expected: ~1-2 allocations (groupExecutorOptions struct + DefaultGroupExecutor struct)
	// Tolerance: <= 3 allocations to account for minor variations
	if allocs > 3 {
		t.Errorf("Too many allocations per call: got %.1f, want <= 3", allocs)
	}
}

// BenchmarkNewDefaultGroupExecutor benchmarks constructor performance
func BenchmarkNewDefaultGroupExecutor(b *testing.B) {
	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}
	mockRM := new(runnertesting.MockResourceManager)

	var ge *DefaultGroupExecutor
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ge = NewDefaultGroupExecutor(
			nil, config, nil, nil, mockRM, "bench-test",
			WithGroupNotificationFunc(nil),
			WithGroupDryRun(&resource.DryRunOptions{
				DetailLevel:   resource.DetailLevelFull,
				ShowSensitive: false,
			}),
			WithGroupKeepTempDirs(false),
		)
	}
	_ = ge // Prevent compiler optimization
}

// BenchmarkNewDefaultGroupExecutor_NoOptions benchmarks constructor with no options
func BenchmarkNewDefaultGroupExecutor_NoOptions(b *testing.B) {
	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: common.IntPtr(30),
		},
	}
	mockRM := new(runnertesting.MockResourceManager)

	var ge *DefaultGroupExecutor
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ge = NewDefaultGroupExecutor(
			nil, config, nil, nil, mockRM, "bench-test",
		)
	}
	_ = ge // Prevent compiler optimization
}
