package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	runnertesting "github.com/isseis/go-safe-cmd-runner/internal/runner/testing"
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
				false, // isDryRun
				false, // keepTempDirs
			)

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
			mockRM := new(runnertesting.MockResourceManager)

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
				false, // isDryRun
				false, // keepTempDirs
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
		false, // isDryRun
		false, // keepTempDirs
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
		false, // isDryRun
		false, // keepTempDirs
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
	mockRM := new(runnertesting.MockResourceManager)

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
		false, // isDryRun
		false, // keepTempDirs
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
	mockRM := new(runnertesting.MockResourceManager)

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
		false, // isDryRun
		false, // keepTempDirs
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
	mockRM := new(runnertesting.MockResourceManager)

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
		false, // isDryRun
		false, // keepTempDirs
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

	groupSpec := &runnertypes.GroupSpec{
		Name:    "test-group",
		WorkDir: "/work",
	}

	runtimeGroup, err := runnertypes.NewRuntimeGroup(groupSpec)
	require.NoError(t, err)

	expectedErr := errors.New("output path is outside work directory")
	mockRM.On("ValidateOutputPath", "/invalid/output/path", "/work").Return(expectedErr)

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: 30},
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
		false, // isDryRun
		false, // keepTempDirs
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
	mockRM := new(runnertesting.MockResourceManager)

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
		false, // isDryRun
		false, // keepTempDirs
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
		commandArgs        []string
		isDryRun           bool
		expectedWorkDir    string
		expectedArgPattern string // Pattern to match in expanded args
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
					Timeout: 30,
				},
			}

			ge := NewDefaultGroupExecutor(
				nil, // command executor - we'll test without executing actual commands
				configSpec,
				nil,          // validator
				nil,          // verificationManager
				mockExecutor, // resourceManager
				"test-run-123",
				mockNotificationFunc,
				tt.isDryRun,
				false, // keepTempDirs
			)

			group := &runnertypes.GroupSpec{
				Name:    "test-group",
				WorkDir: tt.groupWorkDir,
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test-cmd",
						Cmd:  "echo",
						Args: tt.commandArgs,
					},
				},
			}

			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec:         &runnertypes.GlobalSpec{Timeout: 30},
				ExpandedVars: map[string]string{},
			}

			// We cannot easily test the full ExecuteGroup without mocking the entire execution stack
			// Instead, let's test the workdir resolution and variable setting directly

			// 1. Test group workdir resolution
			runtimeGroup, err := config.ExpandGroup(group, runtimeGlobal.ExpandedVars)
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
			runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup.ExpandedVars, group.Name)
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
					} else if tt.expectedWorkDir == "" && containsPattern(arg, tt.expectedArgPattern) {
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

			// Cleanup temp dir if created
			if tempDirMgr != nil {
				tempDirMgr.Cleanup()
			}
		})
	}
}

// containsPattern checks if a string contains the expected pattern
func containsPattern(s, pattern string) bool {
	if len(pattern) == 0 || len(s) == 0 {
		return false
	}

	// Check if pattern is anywhere in the string (for substrings like "dryrun-")
	for i := 0; i <= len(s)-len(pattern); i++ {
		if s[i:i+len(pattern)] == pattern {
			return true
		}
	}

	return false
}
