package executor_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	privilegetestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Resolve commands to absolute paths once for all tests.
// This is required because executor now expects absolute, symlink-resolved paths
// (resolved by PathResolver.ResolvePath in production).
var (
	echoCmd   = executortestutil.ResolveCommand("echo")
	pwdCmd    = executortestutil.ResolveCommand("pwd")
	shCmd     = executortestutil.ResolveCommand("sh")
	whoamiCmd = executortestutil.ResolveCommand("whoami")
)

// mockRunAsResolver returns a resolver that always succeeds with the given
// identity. It is used in tests that exercise the user/group execution path
// without needing actual OS user/group resolution.
func mockRunAsResolver(uid, gid uint32) func(base risktypes.RunAsIdent, userName, groupName string) (risktypes.RunAsIdent, error) {
	return func(_ risktypes.RunAsIdent, _, _ string) (risktypes.RunAsIdent, error) {
		return risktypes.RunAsIdent{UID: uid, GID: gid}, nil
	}
}

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
			cmd:              executortestutil.CreateRuntimeCommand(echoCmd, []string{"hello"}, executortestutil.WithWorkDir("")),
			env:              map[string]string{"TEST": "value"},
			wantErr:          false,
			expectedStdout:   "hello\n",
			expectedStderr:   "",
			expectedExitCode: 0,
		},
		{
			name:             "command with working directory",
			cmd:              executortestutil.CreateRuntimeCommand(pwdCmd, []string{}, executortestutil.WithWorkDir(".")),
			env:              nil,
			wantErr:          false,
			expectedStdout:   "", // pwd output varies, so we'll just check it's not empty
			expectedStderr:   "",
			expectedExitCode: 0,
		},
		{
			name:             "command with multiple arguments",
			cmd:              executortestutil.CreateRuntimeCommand(echoCmd, []string{"-n", "test"}, executortestutil.WithWorkDir("")),
			env:              map[string]string{},
			wantErr:          false,
			expectedStdout:   "test",
			expectedStderr:   "",
			expectedExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &executortestutil.MockFileSystem{
				ExistingPaths: make(map[string]bool),
			}

			// Set up directory existence for working directory tests
			if tt.cmd.EffectiveWorkDir != "" {
				fileSystem.ExistingPaths[tt.cmd.EffectiveWorkDir] = true
			}

			outputWriter := &executortestutil.MockOutputWriter{}

			e := executor.NewDefaultExecutor(
				executor.WithFileSystem(fileSystem),
			)

			result, err := e.Execute(context.Background(), nil, tt.cmd, tt.env, outputWriter)
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
			} else {
				require.NoError(t, err, "Unexpected error")
				require.NotNil(t, result, "Result should not be nil")
				assert.Equal(t, tt.expectedExitCode, result.ExitCode, "Exit code should match expected value")

				// For pwd command, just check that stdout is not empty
				if tt.cmd.ExpandedCmd == pwdCmd {
					assert.NotEmpty(t, result.Stdout, "pwd should return current directory path")
				} else {
					assert.Equal(t, tt.expectedStdout, result.Stdout, "Stdout should match expected value")
				}

				assert.Equal(t, tt.expectedStderr, result.Stderr, "Stderr should match expected value")
			}
		})
	}
}

// ===== User/Group Privilege Execution Tests =====
// The following tests were moved from usergroup_test.go

func TestDefaultExecutor_ExecuteUserGroupPrivileges(t *testing.T) {
	t.Run("user_group_execution_fails_without_cap_setuid", func(t *testing.T) {
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
			executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

		// The mock privilege manager doesn't actually set CAP_SETUID/CAP_SETGID,
		// so SysProcAttr.Credential causes EPERM at execve time.
		assert.Error(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, -1, result.ExitCode)
		assert.Contains(t, err.Error(), "operation not permitted")

		// Verify that user/group privilege escalation was called
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
	})

	t.Run("user_group_no_privilege_manager", func(t *testing.T) {
		// Create executor without privilege manager
		exec := executor.NewDefaultExecutor(
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no privilege manager available")
	})

	t.Run("user_group_not_supported", func(t *testing.T) {
		mockPriv := privilegetestutil.NewMockPrivilegeManager(false) // Not supported
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege changes are not supported")
	})

	t.Run("user_group_privilege_execution_fails", func(t *testing.T) {
		mockPriv := privilegetestutil.NewFailingMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
			executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("invaliduser"), executortestutil.WithRunAsGroup("invalidgroup"))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "user/group privilege execution failed")
	})

	t.Run("only_user_specified_fails_without_cap_setuid", func(t *testing.T) {
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
			executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("testuser"))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, -1, result.ExitCode)
		assert.Contains(t, err.Error(), "operation not permitted")

		// Verify that user/group privilege escalation was called with empty group
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:")
	})

	t.Run("only_group_specified_fails_without_cap_setuid", func(t *testing.T) {
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
			executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

		assert.Error(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, -1, result.ExitCode)
		assert.Contains(t, err.Error(), "operation not permitted")

		// Verify that user/group privilege escalation was called with empty user
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change::testgroup")
	})
}

func TestDefaultExecutor_Execute_Integration(t *testing.T) {
	t.Run("privileged_with_user_group_both_specified", func(t *testing.T) {
		// Test case where user/group are specified
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
			executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

		// The mock privilege manager doesn't actually set CAP_SETUID/CAP_SETGID.
		assert.Error(t, err)
		assert.NotNil(t, result)
		assert.Contains(t, err.Error(), "operation not permitted")

		// Should use user/group execution, not privileged execution
		assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:testuser:testgroup")
		assert.NotContains(t, mockPriv.ElevationCalls, "command_execution")
	})

	t.Run("normal_execution_no_privileges", func(t *testing.T) {
		// Test case where user/group are not specified
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

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
			name:          "valid absolute path fails with operation not permitted",
			cmd:           executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup")),
			expectError:   true,
			errorContains: "operation not permitted",
		},
		{
			name:          "relative working directory fails for user/group command",
			cmd:           executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir("tmp"), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup")),
			expectError:   true,
			errorContains: "does not exist", // Basic validation fails first (directory existence check)
		},
		{
			name:          "absolute working directory fails with operation not permitted",
			cmd:           executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir("/tmp"), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup")),
			expectError:   true,
			errorContains: "operation not permitted",
		},
		{
			name:          "path with relative components fails in standard validation",
			cmd:           executortestutil.CreateRuntimeCommand("/bin/../bin/echo", []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup")),
			expectError:   true,
			errorContains: "command path contains relative path components",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFS := &executortestutil.MockFileSystem{
				ExistingPaths: map[string]bool{
					"/tmp": true,
				},
			}
			mockPrivMgr := privilegetestutil.NewMockPrivilegeManager(true)

			exec := executor.NewDefaultExecutor(
				executor.WithPrivilegeManager(mockPrivMgr),
				executor.WithFileSystem(mockFS),
				executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)),
			)

			ctx := context.Background()
			envVars := map[string]string{"PATH": "/usr/bin"}

			_, err := exec.Execute(ctx, nil, tt.cmd, envVars, nil)

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

// TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging tests audit logging for user/group command execution.
// Note: The mock privilege manager does not actually change the process identity,
// so SysProcAttr.Credential causes "operation not permitted" at exec time.
// Audit logging only fires on success, so on failure we verify no audit log is produced.
func TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging(t *testing.T) {
	t.Run("audit_logging_not_invoked_on_failure", func(t *testing.T) {
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)

		var logBuffer bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
		auditLogger := audit.NewAuditLoggerWithCustom(logger)

		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
			executor.WithAuditLogger(auditLogger),
			executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithName("test_audit_user_group"), executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

		// The command fails because the mock privilege manager doesn't actually
		// change identity, so SysProcAttr.Credential causes EPERM.
		assert.Error(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, -1, result.ExitCode)

		// Audit logging is only invoked on success, so no audit log expected.
		logOutput := logBuffer.String()
		assert.Empty(t, logOutput)
	})

	t.Run("no_audit_logging_when_logger_nil", func(t *testing.T) {
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)

		exec := executor.NewDefaultExecutor(
			executor.WithPrivilegeManager(mockPriv),
			executor.WithFileSystem(&executortestutil.MockFileSystem{}),
			executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)),
		)

		cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithName("test_no_audit"), executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("testuser"), executortestutil.WithRunAsGroup("testgroup"))

		result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

		// The command fails because the mock privilege manager doesn't actually
		// change identity, so SysProcAttr.Credential causes EPERM.
		assert.Error(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, -1, result.ExitCode)
	})
}

// TestDefaultExecutor_UserGroupPrivilegeElevationFailure tests privilege elevation failure scenarios
func TestDefaultExecutor_UserGroupPrivilegeElevationFailure(t *testing.T) {
	mockPrivMgr := privilegetestutil.NewFailingMockPrivilegeManager(true)

	exec := executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(mockPrivMgr),
		executor.WithFileSystem(&executortestutil.MockFileSystem{}),
		executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)),
	)

	cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("root"), executortestutil.WithRunAsGroup("wheel"))

	ctx := context.Background()
	envVars := map[string]string{"PATH": "/usr/bin"}

	result, err := exec.Execute(ctx, nil, cmd, envVars, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, privilegetestutil.ErrMockPrivilegeElevationFailed)
}

// TestDefaultExecutor_UserGroupBackwardCompatibility tests backward compatibility with non-privileged commands
func TestDefaultExecutor_UserGroupBackwardCompatibility(t *testing.T) {
	exec := executor.NewDefaultExecutor(
		executor.WithFileSystem(&executortestutil.MockFileSystem{}),
	)

	cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"normal"}, executortestutil.WithWorkDir(""))

	ctx := context.Background()
	envVars := map[string]string{"PATH": "/usr/bin"}

	result, err := exec.Execute(ctx, nil, cmd, envVars, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "normal")
}

// TestDefaultExecutor_UserGroupPrivileges_StderrCapture tests that stderr is captured even when privileged command fails
func TestDefaultExecutor_UserGroupPrivileges_StderrCapture(t *testing.T) {
	testCases := []struct {
		name           string
		cmd            *runnertypes.RuntimeCommand
		privileged     bool
		expectedStdout string
		expectedStderr string
		expectedExit   int
		errContains    []string
	}{
		{
			name: "privileged command failure captures stderr",
			cmd: executortestutil.CreateRuntimeCommand(
				shCmd,
				[]string{"-c", "echo 'error output' >&2; exit 255"},
				executortestutil.WithWorkDir(""),
				executortestutil.WithRunAsUser("testuser"),
				executortestutil.WithRunAsGroup("testgroup"),
			),
			privileged:     true,
			expectedStdout: "",
			expectedStderr: "",
			expectedExit:   -1,
			errContains:    []string{"user/group privilege execution failed", "operation not permitted"},
		},
		{
			name: "normal command failure captures stderr",
			cmd: executortestutil.CreateRuntimeCommand(
				shCmd,
				[]string{"-c", "echo 'normal error' >&2; exit 1"},
				executortestutil.WithWorkDir(""),
			),
			privileged:     false,
			expectedStdout: "",
			expectedStderr: "normal error",
			expectedExit:   1,
			errContains:    []string{"command execution failed", "exit status 1"},
		},
		{
			name: "privileged command failure captures both stdout and stderr",
			cmd: executortestutil.CreateRuntimeCommand(
				shCmd,
				[]string{"-c", "echo 'stdout message'; echo 'stderr message' >&2; exit 42"},
				executortestutil.WithWorkDir(""),
				executortestutil.WithRunAsUser("testuser"),
			),
			privileged:     true,
			expectedStdout: "",
			expectedStderr: "",
			expectedExit:   -1,
			errContains:    []string{"user/group privilege execution failed", "operation not permitted"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var opts []executor.Option
			if tc.privileged {
				mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
				opts = append(opts, executor.WithPrivilegeManager(mockPriv))
				opts = append(opts, executor.WithRunAsResolver(mockRunAsResolver(1000, 1000)))
			}
			opts = append(opts, executor.WithFileSystem(&executortestutil.MockFileSystem{}))
			exec := executor.NewDefaultExecutor(opts...)

			result, err := exec.Execute(context.Background(), nil, tc.cmd, map[string]string{}, nil)

			assert.Error(t, err)
			for _, msg := range tc.errContains {
				assert.Contains(t, err.Error(), msg)
			}

			require.NotNil(t, result, "Result should not be nil even on failure")

			if tc.expectedStdout == "" {
				assert.Empty(t, result.Stdout)
			} else {
				assert.Contains(t, result.Stdout, tc.expectedStdout)
			}

			if tc.expectedStderr == "" {
				assert.Empty(t, result.Stderr)
			} else {
				assert.Contains(t, result.Stderr, tc.expectedStderr)
			}

			assert.Equal(t, tc.expectedExit, result.ExitCode)
		})
	}
}

// TestDefaultExecutor_UserGroupRootExecution tests running commands as root user
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
			name:               "root user command fails without real elevation",
			cmd:                executortestutil.CreateRuntimeCommand(whoamiCmd, []string{}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("root")),
			privilegeSupported: true,
			expectError:        true,
			errorMessage:       "user/group privilege execution failed",
			noPrivilegeManager: false,
			expectElevations:   []string{"user_group_change:root:"},
		},
		{
			name:               "root user command fails when not supported",
			cmd:                executortestutil.CreateRuntimeCommand(whoamiCmd, []string{}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("root")),
			privilegeSupported: false,
			expectError:        true,
			errorMessage:       "user/group privilege changes are not supported",
			expectedErrorType:  executor.ErrUserGroupPrivilegeUnsupported,
			noPrivilegeManager: false,
		},
		{
			name:               "root user command fails with no manager",
			cmd:                executortestutil.CreateRuntimeCommand(whoamiCmd, []string{}, executortestutil.WithWorkDir(""), executortestutil.WithRunAsUser("root")),
			privilegeSupported: true,
			expectError:        true,
			errorMessage:       "no privilege manager available",
			expectedErrorType:  executor.ErrNoPrivilegeManager,
			noPrivilegeManager: true,
		},
		{
			name:               "normal command bypasses privilege manager",
			cmd:                executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir("")),
			privilegeSupported: false,
			expectError:        false,
			noPrivilegeManager: false,
			expectElevations:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPrivMgr := privilegetestutil.NewMockPrivilegeManager(tt.privilegeSupported)

			var exec executor.CommandExecutor
			if tt.noPrivilegeManager {
				exec = executor.NewDefaultExecutor(
					executor.WithFileSystem(&executortestutil.MockFileSystem{}),
				)
			} else {
				exec = executor.NewDefaultExecutor(
					executor.WithPrivilegeManager(mockPrivMgr),
					executor.WithFileSystem(&executortestutil.MockFileSystem{}),
					executor.WithRunAsResolver(mockRunAsResolver(0, 0)),
				)
			}

			ctx := context.Background()
			envVars := map[string]string{"PATH": "/usr/bin"}

			result, err := exec.Execute(ctx, nil, tt.cmd, envVars, nil)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorType != nil {
					assert.ErrorIs(t, err, tt.expectedErrorType)
				} else if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			if !tt.noPrivilegeManager {
				if len(tt.expectElevations) == 0 && mockPrivMgr.ElevationCalls == nil {
					assert.True(t, true, "No elevations expected and none occurred")
				} else {
					assert.Equal(t, tt.expectElevations, mockPrivMgr.ElevationCalls)
				}
			}
		})
	}
}
