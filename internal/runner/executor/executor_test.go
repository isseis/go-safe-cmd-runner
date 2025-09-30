package executor_test

import (
	"context"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecute_Success(t *testing.T) {
	tests := []struct {
		name             string
		cmd              runnertypes.Command
		env              map[string]string
		wantErr          bool
		expectedStdout   string
		expectedStderr   string
		expectedExitCode int
	}{
		{
			name:             "simple command",
			cmd:              testhelpers.NewCommand("test-cmd", "echo", []string{"hello"}),
			env:              map[string]string{"TEST": "value"},
			wantErr:          false,
			expectedStdout:   "hello\n",
			expectedStderr:   "",
			expectedExitCode: 0,
		},
		{
			name:             "command with working directory",
			cmd:              testhelpers.NewCommandWithDir("test-cmd", "pwd", []string{}, "."),
			env:              nil,
			wantErr:          false,
			expectedStdout:   "", // pwd output varies, so we'll just check it's not empty
			expectedStderr:   "",
			expectedExitCode: 0,
		},
		{
			name:             "command with multiple arguments",
			cmd:              testhelpers.NewCommand("test-cmd", "echo", []string{"-n", "test"}),
			env:              map[string]string{},
			wantErr:          false,
			expectedStdout:   "test",
			expectedStderr:   "",
			expectedExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &executor.MockFileSystem{
				ExistingPaths: make(map[string]bool),
			}

			// Set up directory existence for working directory tests
			if tt.cmd.Dir != "" {
				fileSystem.ExistingPaths[tt.cmd.Dir] = true
			}

			outputWriter := &executor.MockOutputWriter{}

			e := &executor.DefaultExecutor{
				FS: fileSystem,
			}

			result, err := e.Execute(context.Background(), tt.cmd, tt.env, outputWriter)
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
			} else {
				require.NoError(t, err, "Unexpected error")
				require.NotNil(t, result, "Result should not be nil")
				assert.Equal(t, tt.expectedExitCode, result.ExitCode, "Exit code should match expected value")

				// For pwd command, just check that stdout is not empty
				if tt.cmd.Cmd == "pwd" {
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
		cmd     runnertypes.Command
		env     map[string]string
		timeout time.Duration
		wantErr bool
		errMsg  string
	}{
		{
			name:    "non-existent command",
			cmd:     testhelpers.NewCommand("test-cmd", "nonexistentcommand12345", []string{}),
			env:     map[string]string{},
			wantErr: true,
			errMsg:  "failed to find command",
		},
		{
			name:    "command with non-zero exit status",
			cmd:     testhelpers.NewCommand("test-cmd", "sh", []string{"-c", "exit 1"}),
			env:     map[string]string{},
			wantErr: true,
			errMsg:  "command execution failed",
		},
		{
			name:    "command writing to stderr",
			cmd:     testhelpers.NewCommand("test-cmd", "sh", []string{"-c", "echo 'error message' >&2; exit 0"}),
			env:     map[string]string{},
			wantErr: false, // This should succeed but capture stderr
		},
		{
			name:    "command that takes time (for timeout test)",
			cmd:     testhelpers.NewCommand("test-cmd", "sleep", []string{"2"}),
			env:     map[string]string{},
			timeout: 100 * time.Millisecond,
			wantErr: true,
			errMsg:  "signal: killed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &executor.MockFileSystem{
				ExistingPaths: make(map[string]bool),
			}

			// Set up directory existence for working directory tests
			if tt.cmd.Dir != "" {
				fileSystem.ExistingPaths[tt.cmd.Dir] = true
			}

			outputWriter := &executor.MockOutputWriter{}

			e := &executor.DefaultExecutor{
				FS: fileSystem,
			}

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
	fileSystem := &executor.MockFileSystem{
		ExistingPaths: make(map[string]bool),
	}

	e := &executor.DefaultExecutor{
		FS: fileSystem,
	}

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start a long-running command
	cmd := testhelpers.NewCommand("test-cmd", "sleep", []string{"10"})

	// Cancel the context immediately
	cancel()

	result, err := e.Execute(ctx, cmd, map[string]string{}, &executor.MockOutputWriter{})

	// Should get an error due to context cancellation
	assert.Error(t, err, "Expected error due to context cancellation")
	assert.ErrorIs(t, err, context.Canceled, "Error should indicate context cancellation")
	assert.NotNil(t, result, "Result should still be returned even on failure")
}

func TestExecute_EnvironmentVariables(t *testing.T) {
	// Test that only filtered environment variables are passed to executed commands
	// and os.Environ() variables are not leaked through
	fileSystem := &executor.MockFileSystem{
		ExistingPaths: make(map[string]bool),
	}

	e := &executor.DefaultExecutor{
		FS: fileSystem,
	}

	// Set a test environment variable in the runner process
	t.Setenv("LEAKED_VAR", "should_not_appear")

	cmd := testhelpers.NewCommand("test-cmd", "printenv", []string{})

	// Only provide filtered variables through envVars parameter
	envVars := map[string]string{
		"FILTERED_VAR": "allowed_value",
		"PATH":         "/usr/bin:/bin", // Common required variable
	}

	ctx := context.Background()
	result, err := e.Execute(ctx, cmd, envVars, &executor.MockOutputWriter{})

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
		cmd     runnertypes.Command
		wantErr bool
	}{
		{
			name: "empty command",
			cmd: runnertypes.Command{
				Name:         "empty-cmd",
				Cmd:          "",
				Args:         []string{},
				ExpandedCmd:  "",
				ExpandedArgs: []string{},
			},
			wantErr: true,
		},
		{
			name:    "valid command",
			cmd:     testhelpers.NewCommand("test-cmd", "echo", []string{"hello"}),
			wantErr: false,
		},
		{
			name:    "invalid directory",
			cmd:     testhelpers.NewCommandWithDir("test-cmd", "ls", []string{}, "/nonexistent/directory"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &executor.MockFileSystem{
				ExistingPaths: make(map[string]bool),
			}

			// Set up directory existence based on test case
			if tt.cmd.Dir != "" {
				// For non-empty Dir, configure whether it exists
				fileSystem.ExistingPaths[tt.cmd.Dir] = !tt.wantErr
			}

			e := &executor.DefaultExecutor{
				FS: fileSystem,
			}

			err := e.Validate(tt.cmd)
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error")
			}
		})
	}
}
