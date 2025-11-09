package executor_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
	privilegetesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecute_Success(t *testing.T) {
	tests := []struct {
		name             string
		cmd              *runnertypes.RuntimeCommand
		env              map[string]string
		wantErr          bool
		expectedStdout   string
		expectedStderr   string
		expectedExitCode int
	}{
		{
			name:             "simple command",
			cmd:              executortesting.CreateRuntimeCommand("echo", []string{"hello"}, executortesting.WithWorkDir("")),
			env:              map[string]string{"TEST": "value"},
			wantErr:          false,
			expectedStdout:   "hello\n",
			expectedStderr:   "",
			expectedExitCode: 0,
		},
		{
			name:             "command with working directory",
			cmd:              executortesting.CreateRuntimeCommand("pwd", []string{}, executortesting.WithWorkDir(".")),
			env:              nil,
			wantErr:          false,
			expectedStdout:   "", // pwd output varies, so we'll just check it's not empty
			expectedStderr:   "",
			expectedExitCode: 0,
		},
		{
			name:             "command with multiple arguments",
			cmd:              executortesting.CreateRuntimeCommand("echo", []string{"-n", "test"}, executortesting.WithWorkDir("")),
			env:              map[string]string{},
			wantErr:          false,
			expectedStdout:   "test",
			expectedStderr:   "",
			expectedExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &executortesting.MockFileSystem{
				ExistingPaths: make(map[string]bool),
			}

			// Set up directory existence for working directory tests
			if tt.cmd.EffectiveWorkDir != "" {
				fileSystem.ExistingPaths[tt.cmd.EffectiveWorkDir] = true
			}

			outputWriter := &executortesting.MockOutputWriter{}

			e := executor.NewDefaultExecutor(
				executor.WithFileSystem(fileSystem),
			).(*executor.DefaultExecutor)

			result, err := e.Execute(context.Background(), tt.cmd, tt.env, outputWriter)
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
			} else {
				require.NoError(t, err, "Unexpected error")
				require.NotNil(t, result, "Result should not be nil")
				assert.Equal(t, tt.expectedExitCode, result.ExitCode, "Exit code should match expected value")

				// For pwd command, just check that stdout is not empty
				if tt.cmd.ExpandedCmd == "pwd" {
					assert.NotEmpty(t, result.Stdout, "pwd should return current directory path")
				} else {
					assert.Equal(t, tt.expectedStdout, result.Stdout, "Stdout should match expected value")
				}

				assert.Equal(t, tt.expectedStderr, result.Stderr, "Stderr should match expected value")
			}
		})
	}
}

func TestExecute_Failure(t *testing.T) {
	tests := []struct {
		name    string
		cmd     *runnertypes.RuntimeCommand
		env     map[string]string
		timeout time.Duration
		wantErr bool
		errMsg  string
	}{
		{
			name:    "non-existent command",
			cmd:     executortesting.CreateRuntimeCommand("nonexistentcommand12345", []string{}, executortesting.WithWorkDir("")),
			env:     map[string]string{},
			wantErr: true,
			errMsg:  "failed to find command",
		},
		{
			name:    "command with non-zero exit status",
			cmd:     executortesting.CreateRuntimeCommand("sh", []string{"-c", "exit 1"}, executortesting.WithWorkDir("")),
			env:     map[string]string{},
			wantErr: true,
			errMsg:  "command execution failed",
		},
		{
			name:    "command writing to stderr",
			cmd:     executortesting.CreateRuntimeCommand("sh", []string{"-c", "echo 'error message' >&2; exit 0"}, executortesting.WithWorkDir("")),
			env:     map[string]string{},
			wantErr: false, // This should succeed but capture stderr
		},
		{
			name:    "command that takes time (for timeout test)",
			cmd:     executortesting.CreateRuntimeCommand("sleep", []string{"2"}, executortesting.WithWorkDir("")),
			env:     map[string]string{},
			timeout: 100 * time.Millisecond,
			wantErr: true,
			errMsg:  "signal: killed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &executortesting.MockFileSystem{
				ExistingPaths: make(map[string]bool),
			}

			// Set up directory existence for working directory tests
			if tt.cmd.EffectiveWorkDir != "" {
				fileSystem.ExistingPaths[tt.cmd.EffectiveWorkDir] = true
			}

			outputWriter := &executortesting.MockOutputWriter{}

			e := executor.NewDefaultExecutor(
				executor.WithFileSystem(fileSystem),
			).(*executor.DefaultExecutor)

			ctx := context.Background()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			result, err := e.Execute(ctx, tt.cmd, tt.env, outputWriter)

			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg, "Error message should contain expected text")
				}
			} else {
				require.NoError(t, err, "Unexpected error")
				require.NotNil(t, result, "Result should not be nil")

				// For the stderr test case, check that stderr was captured
				if tt.name == "command writing to stderr" {
					assert.NotEmpty(t, outputWriter.Outputs, "Should have captured output")
				}
			}
		})
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	fileSystem := &executortesting.MockFileSystem{
		ExistingPaths: make(map[string]bool),
	}

	e := executor.NewDefaultExecutor(
		executor.WithFileSystem(fileSystem),
	).(*executor.DefaultExecutor)

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start a long-running command
	cmd := executortesting.CreateRuntimeCommand("sleep", []string{"10"}, executortesting.WithWorkDir(""))

	// Cancel the context immediately
	cancel()

	result, err := e.Execute(ctx, cmd, map[string]string{}, &executortesting.MockOutputWriter{})

	// Should get an error due to context cancellation
	assert.Error(t, err, "Expected error due to context cancellation")
	assert.ErrorIs(t, err, context.Canceled, "Error should indicate context cancellation")
	assert.NotNil(t, result, "Result should still be returned even on failure")
}

func TestExecute_EnvironmentVariables(t *testing.T) {
	// Test that only filtered environment variables are passed to executed commands
	// and os.Environ() variables are not leaked through
	fileSystem := &executortesting.MockFileSystem{
		ExistingPaths: make(map[string]bool),
	}

	e := executor.NewDefaultExecutor(
		executor.WithFileSystem(fileSystem),
	).(*executor.DefaultExecutor)

	// Set a test environment variable in the runner process
	t.Setenv("LEAKED_VAR", "should_not_appear")

	cmd := executortesting.CreateRuntimeCommand("printenv", []string{}, executortesting.WithWorkDir(""))

	// Only provide filtered variables through envVars parameter
	envVars := map[string]string{
		"FILTERED_VAR": "allowed_value",
		"PATH":         "/usr/bin:/bin", // Common required variable
	}

	ctx := context.Background()
	result, err := e.Execute(ctx, cmd, envVars, &executortesting.MockOutputWriter{})

	require.NoError(t, err, "Execute should not return an error")
	require.NotNil(t, result, "Result should not be nil")

	// Check that only allowed variables are present in the output
	assert.Contains(t, result.Stdout, "FILTERED_VAR=allowed_value", "Filtered variable should be present")
	assert.Contains(t, result.Stdout, "PATH=/usr/bin:/bin", "PATH variable should be present")
	assert.NotContains(t, result.Stdout, "LEAKED_VAR=should_not_appear", "Leaked variable should not be present")
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cmd     *runnertypes.RuntimeCommand
		wantErr bool
	}{
		{
			name:    "empty command",
			cmd:     executortesting.CreateRuntimeCommand("", []string{}, executortesting.WithWorkDir("")),
			wantErr: true,
		},
		{
			name:    "valid command",
			cmd:     executortesting.CreateRuntimeCommand("echo", []string{"hello"}, executortesting.WithWorkDir("")),
			wantErr: false,
		},
		{
			name:    "invalid directory",
			cmd:     executortesting.CreateRuntimeCommand("ls", []string{}, executortesting.WithWorkDir("/nonexistent/directory")),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &executortesting.MockFileSystem{
				ExistingPaths: make(map[string]bool),
			}

			// Set up directory existence based on test case
			if tt.cmd.EffectiveWorkDir != "" {
				// For non-empty EffectiveWorkDir, configure whether it exists
				fileSystem.ExistingPaths[tt.cmd.EffectiveWorkDir] = !tt.wantErr
			}

			e := executor.NewDefaultExecutor(
				executor.WithFileSystem(fileSystem),
			).(*executor.DefaultExecutor)

			err := e.Validate(tt.cmd)
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error")
			}
		})
	}
}

// ===== User/Group Privilege Execution Tests =====
// The following tests were moved from usergroup_test.go

func TestDefaultExecutor_ExecuteUserGroupPrivileges(t *testing.T) {
	t.Run("user_group_execution_success", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
		)

		cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that user/group privilege escalation was called
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
	})

	t.Run("user_group_no_privilege_manager", func(t *testing.T) {
		// Create executor without privilege manager
		exec := executor.NewDefaultExecutor(
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
		)

		cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no privilege manager available")
	})

	t.Run("user_group_not_supported", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(false) // Not supported
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
		)

		cmd := executortesting.CreateRuntimeCommand("echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege changes are not supported")
	})

	t.Run("user_group_privilege_execution_fails", func(t *testing.T) {
		mockPriv := privilegetesting.NewFailingMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
		)

		cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("invaliduser"), executortesting.WithRunAsGroup("invalidgroup"))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege execution failed")
	})

	t.Run("only_user_specified", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
		)

		cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("testuser"))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that user/group privilege escalation was called with empty group
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:")
	})

	t.Run("only_group_specified", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
		)

		cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify that user/group privilege escalation was called with empty user
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change::testgroup")
	})
}

func TestDefaultExecutor_Execute_Integration(t *testing.T) {
	t.Run("privileged_with_user_group_both_specified", func(t *testing.T) {
		// Test case where user/group are specified
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
		)

		cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should use user/group execution, not privileged execution
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
		assert.NotContains(t, mockPriv.ElevationCalls, "command_execution")
	})

	t.Run("normal_execution_no_privileges", func(t *testing.T) {
		// Test case where user/group are not specified
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
		)

		cmd := executortesting.CreateRuntimeCommand("echo", []string{"test"}, executortesting.WithWorkDir(""))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Should not call any privilege methods
		assert.Empty(t, mockPriv.ElevationCalls)
	})
}

// TestUserGroupCommandValidation_PathRequirements tests the basic validation for user/group commands
func TestUserGroupCommandValidation_PathRequirements(t *testing.T) {
	tests := []struct {
		name          string
		cmd           *runnertypes.RuntimeCommand
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid absolute path works for user/group command",
			cmd:         executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup")),
			expectError: false,
		},
		{
			name:          "relative working directory fails for user/group command",
			cmd:           executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir("tmp"), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup")),
			expectError:   true,
			errorContains: "does not exist", // Basic validation fails first (directory existence check)
		},
		{
			name:        "absolute working directory works for user/group command",
			cmd:         executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir("/tmp"), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup")),
			expectError: false,
		},
		{
			name:          "path with relative components fails in standard validation",
			cmd:           executortesting.CreateRuntimeCommand("/bin/../bin/echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup")),
			expectError:   true,
			errorContains: "command path contains relative path components", // Error message from standard validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock filesystem for directory validation
			mockFS := &executortesting.MockFileSystem{
				ExistingPaths: map[string]bool{
					"/tmp": true,
				},
			}
			mockPrivMgr := privilegetesting.NewMockPrivilegeManager(true)

			exec := executor.NewDefaultExecutor(
				executor.WithPrivilegeManager(mockPrivMgr),
				executor.WithFileSystem(mockFS),
			)

			ctx := context.Background()
			envVars := map[string]string{"PATH": "/usr/bin"}

			_, err := exec.Execute(ctx, tt.cmd, envVars, nil)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging tests audit logging for user/group command execution
func TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging(t *testing.T) {
	t.Run("audit_logging_successful_execution", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)

		// Create a real audit logger with custom slog handler to capture logs
		var logBuffer bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
		auditLogger := audit.NewAuditLoggerWithCustom(logger)

		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
			executor.WithAuditLogger(auditLogger),
		)

		cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithName("test_audit_user_group"), executortesting.WithWorkDir(""), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)

		// Verify audit logging was called
		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "user_group_execution")
		assert.Contains(t, logOutput, "test_audit_user_group")
		assert.Contains(t, logOutput, "testuser")
		assert.Contains(t, logOutput, "testgroup")
	})

	t.Run("no_audit_logging_when_logger_nil", func(t *testing.T) {
		mockPriv := privilegetesting.NewMockPrivilegeManager(true)

		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortesting.MockFileSystem{}),
			// No audit logger provided
		)

		cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithName("test_no_audit"), executortesting.WithWorkDir(""), executortesting.WithRunAsUser("testuser"), executortesting.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), cmd, map[string]string{}, nil)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.ExitCode)
		// No assertions about logging since no logger is provided
	})
}

// TestDefaultExecutor_UserGroupPrivilegeElevationFailure tests privilege elevation failure scenarios
// This replaces the deleted TestDefaultExecutor_PrivilegeElevationFailure test for user/group commands
func TestDefaultExecutor_UserGroupPrivilegeElevationFailure(t *testing.T) {
	mockPrivMgr := privilegetesting.NewFailingMockPrivilegeManager(true)

	exec := executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(mockPrivMgr),
		executor.WithFileSystem(&executortesting.MockFileSystem{}),
	)

	cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("root"), executortesting.WithRunAsGroup("wheel"))

	ctx := context.Background()
	envVars := map[string]string{"PATH": "/usr/bin"}

	result, err := exec.Execute(ctx, cmd, envVars, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, privilegetesting.ErrMockPrivilegeElevationFailed)
}

// TestDefaultExecutor_UserGroupBackwardCompatibility tests backward compatibility with non-privileged commands
// This replaces the deleted TestDefaultExecutor_BackwardCompatibility test
func TestDefaultExecutor_UserGroupBackwardCompatibility(t *testing.T) {
	// Test that existing code without privilege manager still works for normal commands
	exec := executor.NewDefaultExecutor(
		executor.WithFileSystem(&executortesting.MockFileSystem{}),
	)

	cmd := executortesting.CreateRuntimeCommand("echo", []string{"normal"}, executortesting.WithWorkDir(""))

	ctx := context.Background()
	envVars := map[string]string{"PATH": "/usr/bin"}

	result, err := exec.Execute(ctx, cmd, envVars, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "normal")
}

// TestDefaultExecutor_UserGroupRootExecution tests running commands as root user
// This provides equivalent functionality to the deleted privileged=true tests
func TestDefaultExecutor_UserGroupRootExecution(t *testing.T) {
	tests := []struct {
		name               string
		cmd                *runnertypes.RuntimeCommand
		privilegeSupported bool
		expectError        bool
		errorMessage       string
		expectedErrorType  error
		noPrivilegeManager bool
		expectElevations   []string
	}{
		{
			name:               "root user command executes with elevation",
			cmd:                executortesting.CreateRuntimeCommand("/usr/bin/whoami", []string{}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("root")),
			privilegeSupported: true,
			expectError:        false,
			noPrivilegeManager: false,
			expectElevations:   []string{"user_group_change:root:"},
		},
		{
			name:               "root user command fails when not supported",
			cmd:                executortesting.CreateRuntimeCommand("/usr/bin/whoami", []string{}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("root")),
			privilegeSupported: false,
			expectError:        true,
			errorMessage:       "user/group privilege changes are not supported",
			expectedErrorType:  executor.ErrUserGroupPrivilegeUnsupported,
			noPrivilegeManager: false,
		},
		{
			name:               "root user command fails with no manager",
			cmd:                executortesting.CreateRuntimeCommand("/usr/bin/whoami", []string{}, executortesting.WithWorkDir(""), executortesting.WithRunAsUser("root")),
			privilegeSupported: true,
			expectError:        true,
			errorMessage:       "no privilege manager available",
			expectedErrorType:  executor.ErrNoPrivilegeManager,
			noPrivilegeManager: true,
		},
		{
			name:               "normal command bypasses privilege manager",
			cmd:                executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"}, executortesting.WithWorkDir("")),
			privilegeSupported: false, // Should not matter
			expectError:        false,
			noPrivilegeManager: false,
			expectElevations:   []string{}, // No elevations expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPrivMgr := privilegetesting.NewMockPrivilegeManager(tt.privilegeSupported)

			var exec executor.CommandExecutor
			if tt.noPrivilegeManager {
				// Create executor without privilege manager
				exec = executor.NewDefaultExecutor(
					executor.WithFileSystem(&executortesting.MockFileSystem{}),
				)
			} else {
				exec = executor.NewDefaultExecutor(
					executor.WithPrivilegeManager(mockPrivMgr),
					executor.WithFileSystem(&executortesting.MockFileSystem{}),
				)
			}

			ctx := context.Background()
			envVars := map[string]string{"PATH": "/usr/bin"}

			result, err := exec.Execute(ctx, tt.cmd, envVars, nil)

			if tt.expectError {
				assert.Error(t, err)
				// Check based on expected error type
				if tt.expectedErrorType != nil {
					assert.ErrorIs(t, err, tt.expectedErrorType)
				} else if tt.errorMessage != "" {
					// Fall back to message check only if no error type is specified
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			if !tt.noPrivilegeManager {
				if len(tt.expectElevations) == 0 && mockPrivMgr.ElevationCalls == nil {
					// Both nil and empty slice are acceptable for no elevations - no assertion needed
					assert.True(t, true, "No elevations expected and none occurred")
				} else {
					assert.Equal(t, tt.expectElevations, mockPrivMgr.ElevationCalls)
				}
			}
		})
	}
}
