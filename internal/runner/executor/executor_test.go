package executor_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

type mockFileSystem struct {
	// A map to configure which paths exist.
	existingPaths map[string]bool
	// An error to return from methods, for testing error paths.
	err error
}

func (m *mockFileSystem) CreateTempDir(prefix string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return os.MkdirTemp("", prefix)
}

func (m *mockFileSystem) RemoveAll(_ string) error {
	return m.err
}

func (m *mockFileSystem) FileExists(path string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	exists := m.existingPaths[path]
	return exists, nil
}

type mockOutputWriter struct {
	outputs []string
}

func (m *mockOutputWriter) Write(_ string, data []byte) error {
	m.outputs = append(m.outputs, string(data))
	return nil
}

func (m *mockOutputWriter) Close() error {
	return nil
}

type mockEnvManager struct{}

func (m *mockEnvManager) LoadFromFile(_ string) (map[string]string, error) {
	return map[string]string{"FROM_FILE": "value"}, nil
}

func (m *mockEnvManager) Merge(envs ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, env := range envs {
		for k, v := range env {
			result[k] = v
		}
	}
	return result
}

func (m *mockEnvManager) Resolve(s string, _ map[string]string) (string, error) {
	return s, nil
}

func TestNewDefaultExecutor(t *testing.T) {
	exec := executor.NewDefaultExecutor()
	assert.NotNil(t, exec, "NewDefaultExecutor should return a non-nil executor")
}

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
			name: "simple command",
			cmd: runnertypes.Command{
				Cmd:  "echo",
				Args: []string{"hello"},
			},
			env:              map[string]string{"TEST": "value"},
			wantErr:          false,
			expectedStdout:   "hello\n",
			expectedStderr:   "",
			expectedExitCode: 0,
		},
		{
			name: "command with working directory",
			cmd: runnertypes.Command{
				Cmd:  "pwd",
				Dir:  ".",
				Args: []string{},
			},
			env:              nil,
			wantErr:          false,
			expectedStdout:   "", // pwd output varies, so we'll just check it's not empty
			expectedStderr:   "",
			expectedExitCode: 0,
		},
		{
			name: "command with multiple arguments",
			cmd: runnertypes.Command{
				Cmd:  "echo",
				Args: []string{"-n", "test"},
			},
			env:              map[string]string{},
			wantErr:          false,
			expectedStdout:   "test",
			expectedStderr:   "",
			expectedExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &mockFileSystem{
				existingPaths: make(map[string]bool),
			}

			// Set up directory existence for working directory tests
			if tt.cmd.Dir != "" {
				fileSystem.existingPaths[tt.cmd.Dir] = true
			}

			outputWriter := &mockOutputWriter{}

			e := &executor.DefaultExecutor{
				FS:  fileSystem,
				Out: outputWriter,
				Env: &mockEnvManager{},
			}

			result, err := e.Execute(context.Background(), tt.cmd, tt.env)
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error")
				assert.NotNil(t, result, "Result should not be nil")
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
			name: "non-existent command",
			cmd: runnertypes.Command{
				Cmd:  "nonexistentcommand12345",
				Args: []string{},
			},
			env:     map[string]string{},
			wantErr: true,
			errMsg:  "failed to find command",
		},
		{
			name: "command with non-zero exit status",
			cmd: runnertypes.Command{
				Cmd:  "sh",
				Args: []string{"-c", "exit 1"},
			},
			env:     map[string]string{},
			wantErr: true,
			errMsg:  "command execution failed",
		},
		{
			name: "command writing to stderr",
			cmd: runnertypes.Command{
				Cmd:  "sh",
				Args: []string{"-c", "echo 'error message' >&2; exit 0"},
			},
			env:     map[string]string{},
			wantErr: false, // This should succeed but capture stderr
		},
		{
			name: "command that takes time (for timeout test)",
			cmd: runnertypes.Command{
				Cmd:  "sleep",
				Args: []string{"2"},
			},
			env:     map[string]string{},
			timeout: 100 * time.Millisecond,
			wantErr: true,
			errMsg:  "signal: killed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &mockFileSystem{
				existingPaths: make(map[string]bool),
			}

			// Set up directory existence for working directory tests
			if tt.cmd.Dir != "" {
				fileSystem.existingPaths[tt.cmd.Dir] = true
			}

			outputWriter := &mockOutputWriter{}

			e := &executor.DefaultExecutor{
				FS:  fileSystem,
				Out: outputWriter,
				Env: &mockEnvManager{},
			}

			ctx := context.Background()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			result, err := e.Execute(ctx, tt.cmd, tt.env)

			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg, "Error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Unexpected error")
				assert.NotNil(t, result, "Result should not be nil")

				// For the stderr test case, check that stderr was captured
				if tt.name == "command writing to stderr" {
					assert.NotEmpty(t, outputWriter.outputs, "Should have captured output")
				}
			}
		})
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	fileSystem := &mockFileSystem{
		existingPaths: make(map[string]bool),
	}

	e := &executor.DefaultExecutor{
		FS:  fileSystem,
		Out: &mockOutputWriter{},
		Env: &mockEnvManager{},
	}

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start a long-running command
	cmd := runnertypes.Command{
		Cmd:  "sleep",
		Args: []string{"10"},
	}

	// Cancel the context immediately
	cancel()

	result, err := e.Execute(ctx, cmd, map[string]string{})

	// Should get an error due to context cancellation
	assert.Error(t, err, "Expected error due to context cancellation")
	assert.Contains(t, err.Error(), "context canceled", "Error should indicate context cancellation")
	assert.NotNil(t, result, "Result should still be returned even on failure")
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
				Cmd: "",
			},
			wantErr: true,
		},
		{
			name: "valid command",
			cmd: runnertypes.Command{
				Cmd:  "echo",
				Args: []string{"hello"},
			},
			wantErr: false,
		},
		{
			name: "invalid directory",
			cmd: runnertypes.Command{
				Cmd: "ls",
				Dir: "/nonexistent/directory",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSystem := &mockFileSystem{
				existingPaths: make(map[string]bool),
			}

			// Set up directory existence based on test case
			if tt.cmd.Dir != "" {
				// For non-empty Dir, configure whether it exists
				fileSystem.existingPaths[tt.cmd.Dir] = !tt.wantErr
			}

			e := &executor.DefaultExecutor{
				FS:  fileSystem,
				Out: &mockOutputWriter{},
				Env: &mockEnvManager{},
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
