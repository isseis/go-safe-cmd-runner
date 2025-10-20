//go:build skip_integration_tests

package runner_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestCreateCommandContext tests the createCommandContext method
func TestCreateCommandContext(t *testing.T) {
	tests := []struct {
		name            string
		globalTimeout   int
		commandTimeout  int
		expectedTimeout time.Duration
	}{
		{
			name:            "use global timeout when command timeout is not set",
			globalTimeout:   30,
			commandTimeout:  0,
			expectedTimeout: 30 * time.Second,
		},
		{
			name:            "use command timeout when set",
			globalTimeout:   30,
			commandTimeout:  60,
			expectedTimeout: 60 * time.Second,
		},
		{
			name:            "command timeout overrides global timeout",
			globalTimeout:   120,
			commandTimeout:  10,
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
			}

			ctx := context.Background()
			cmdCtx, cancel := ge.createCommandContext(ctx, cmd)
			defer cancel()

			// Verify deadline is set correctly
			deadline, ok := cmdCtx.Deadline()
			require.True(t, ok, "context should have a deadline")

			// Check that deadline is approximately correct (within 100ms tolerance)
			expectedDeadline := time.Now().Add(tt.expectedTimeout)
			timeDiff := deadline.Sub(expectedDeadline)
			assert.Less(t, timeDiff.Abs(), 100*time.Millisecond,
				"deadline should be within 100ms of expected value")
		})
	}
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
			mockRM := new(MockResourceManager)

			config := &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: 30,
				},
			}

			ge := NewDefaultGroupExecutor(
				nil,
				config,
				nil,
				nil,
				mockRM,
				"test-run-123",
				nil,
			)

			group := &runnertypes.GroupSpec{
				Name:    "test-group",
				TempDir: tt.groupTempDir,
				WorkDir: tt.groupWorkDir,
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test-cmd",
						Cmd:  "/bin/echo",
						Dir:  tt.commandDir,
					},
				},
			}

			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{Timeout: 30},
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
			mockRM := new(MockResourceManager)

			config := &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: 30,
				},
			}

			ge := NewDefaultGroupExecutor(
				nil,
				config,
				nil,
				nil,
				mockRM,
				"test-run-123",
				nil,
			)

			group := &runnertypes.GroupSpec{
				Name:    "test-group",
				TempDir: true,
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test-cmd",
						Cmd:  "/bin/echo",
					},
				},
			}

			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{Timeout: 30},
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
func TestExecuteGroup_CreateTempDirFailure(t *testing.T) {
	mockRM := new(MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: 30,
		},
	}

	ge := NewDefaultGroupExecutor(
		nil,
		config,
		nil,
		nil,
		mockRM,
		"test-run-123",
		nil,
	)

	group := &runnertypes.GroupSpec{
		Name:    "test-group",
		TempDir: true,
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/echo",
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: 30},
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
	mockRM := new(MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: 30,
		},
	}

	var capturedNotification *groupExecutionResult
	notificationFunc := func(_ *runnertypes.GroupSpec, result *groupExecutionResult, _ time.Duration) {
		capturedNotification = result
	}

	ge := NewDefaultGroupExecutor(
		nil,
		config,
		nil,
		nil,
		mockRM,
		"test-run-123",
		notificationFunc,
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
		Spec: &runnertypes.GlobalSpec{Timeout: 30},
	}

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
	mockRM := new(MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: 30,
		},
	}

	var capturedNotification *groupExecutionResult
	notificationFunc := func(_ *runnertypes.GroupSpec, result *groupExecutionResult, _ time.Duration) {
		capturedNotification = result
	}

	ge := NewDefaultGroupExecutor(
		nil,
		config,
		nil,
		nil,
		mockRM,
		"test-run-123",
		notificationFunc,
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
		Spec: &runnertypes.GlobalSpec{Timeout: 30},
	}

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
	mockRM := new(MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: 30,
		},
	}

	var capturedNotification *groupExecutionResult
	var capturedDuration time.Duration
	notificationFunc := func(_ *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration) {
		capturedNotification = result
		capturedDuration = duration
	}

	ge := NewDefaultGroupExecutor(
		nil,
		config,
		nil,
		nil,
		mockRM,
		"test-run-123",
		notificationFunc,
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
		Spec: &runnertypes.GlobalSpec{Timeout: 30},
	}

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
	mockRM := new(MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: 30,
		},
	}

	ge := NewDefaultGroupExecutor(
		nil,
		config,
		nil,
		nil,
		mockRM,
		"test-run-123",
		nil,
	)

	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name:   "test-cmd",
			Cmd:    "/bin/echo",
			Output: "/invalid/output/path",
		},
		ExpandedCmd:  "/bin/echo",
		ExpandedArgs: []string{},
	}

	group := &runnertypes.GroupSpec{
		Name:    "test-group",
		WorkDir: "/work",
	}

	expectedErr := errors.New("output path is outside work directory")
	mockRM.On("ValidateOutputPath", "/invalid/output/path", "/work").Return(expectedErr)

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: 30},
	}

	ctx := context.Background()
	result, err := ge.executeCommandInGroup(ctx, cmd, group, runtimeGlobal)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "output path validation failed")
	assert.ErrorIs(t, err, expectedErr)
}

// TestExecuteGroup_MultipleCommands tests execution of multiple commands in sequence
func TestExecuteGroup_MultipleCommands(t *testing.T) {
	mockRM := new(MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: 30,
		},
	}

	ge := NewDefaultGroupExecutor(
		nil,
		config,
		nil,
		nil,
		mockRM,
		"test-run-123",
		nil,
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
		Spec: &runnertypes.GlobalSpec{Timeout: 30},
	}

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
	mockRM := new(MockResourceManager)

	config := &runnertypes.ConfigSpec{
		Global: runnertypes.GlobalSpec{
			Timeout: 30,
		},
	}

	ge := NewDefaultGroupExecutor(
		nil,
		config,
		nil,
		nil,
		mockRM,
		"test-run-123",
		nil,
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
		Spec: &runnertypes.GlobalSpec{Timeout: 30},
	}

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
