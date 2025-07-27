package executor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	privtesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// Error variable for testing
var errCommandExecutionShouldNotHappen = errors.New("command execution should not happen")

// TestPrivilegedCommandValidation tests the validatePrivilegedCommand function through Execute
func TestPrivilegedCommandValidation(t *testing.T) {
	tests := []struct {
		name          string
		cmd           runnertypes.Command
		setupFS       map[string]bool // Paths that should exist in mock filesystem
		expectedError error
	}{
		{
			name: "non-absolute path should fail",
			cmd: runnertypes.Command{
				Name:       "test-relative-path",
				Cmd:        "relative/path/command", // This path is relative
				Args:       []string{"arg1", "arg2"},
				Privileged: true,
			},
			setupFS:       map[string]bool{},
			expectedError: executor.ErrPrivilegedCmdSecurity,
		},
		{
			name: "path with dot components should fail in standard validation",
			cmd: runnertypes.Command{
				Name:       "test-dot-path",
				Cmd:        "/path/../bin/echo", // Path with dot component
				Args:       []string{"arg1"},
				Privileged: true,
			},
			setupFS:       map[string]bool{},
			expectedError: executor.ErrInvalidPath, // Error from standard validation
		},
		{
			name: "non-absolute working dir should fail",
			cmd: runnertypes.Command{
				Name:       "test-relative-dir",
				Cmd:        "/bin/echo", // Valid absolute path
				Args:       []string{"test"},
				Dir:        "relative/dir", // Non-absolute dir
				Privileged: true,
			},
			setupFS: map[string]bool{
				"relative/dir": true, // Make the directory exist for basic validation
			},
			expectedError: executor.ErrPrivilegedCmdSecurity,
		},
		{
			name: "valid privileged command should pass validation",
			cmd: runnertypes.Command{
				Name:       "test-valid-command",
				Cmd:        "/usr/bin/echo",
				Args:       []string{"test"},
				Dir:        "/tmp",
				Privileged: true,
			},
			setupFS: map[string]bool{
				"/tmp": true,
			},
			expectedError: nil, // No error expected for validation, though execution might fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockFS := &mockFileSystem{
				existingPaths: tt.setupFS,
			}

			// Create mock privilege manager with custom execution function
			mockPrivMgr := privtesting.NewMockPrivilegeManagerWithExecFn(
				true, // supported
				func() error {
					// This would normally execute the command, but we're just testing validation
					return errCommandExecutionShouldNotHappen
				},
			)

			// Create executor with mocks
			exec := executor.NewDefaultExecutor(
				executor.WithFileSystem(mockFS),
				executor.WithPrivilegeManager(mockPrivMgr),
			)

			// Test execution which would trigger validation
			_, err := exec.Execute(context.Background(), tt.cmd, map[string]string{})

			// Assertion
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				// If no validation error is expected, we should still get an error from our mock execution function
				assert.Error(t, err)
				assert.False(t, errors.Is(err, executor.ErrPrivilegedCmdSecurity))
			}
		})
	}
}

// TestPrivilegedCommandExecution_NoManager tests behavior when no privilege manager is available
func TestPrivilegedCommandExecution_NoManager(t *testing.T) {
	// Setup
	mockFS := &mockFileSystem{
		existingPaths: map[string]bool{
			"/tmp": true,
		},
	}

	// Create executor without privilege manager
	exec := executor.NewDefaultExecutor(
		executor.WithFileSystem(mockFS),
	)

	// Test command
	cmd := runnertypes.Command{
		Name:       "test-no-manager",
		Cmd:        "/bin/echo",
		Args:       []string{"test"},
		Privileged: true,
	}

	// Execute
	_, err := exec.Execute(context.Background(), cmd, map[string]string{})

	// Assertion
	assert.ErrorIs(t, err, executor.ErrNoPrivilegeManager)
}

// TestPrivilegedCommandExecution_UnsupportedPlatform tests behavior when privilege elevation is unsupported
func TestPrivilegedCommandExecution_UnsupportedPlatform(t *testing.T) {
	// Setup
	mockFS := &mockFileSystem{
		existingPaths: map[string]bool{
			"/tmp": true,
		},
	}

	// Create mock privilege manager that doesn't support privilege execution
	mockPrivMgr := privtesting.NewMockPrivilegeManager(false)

	// Create executor with mock privilege manager that doesn't support elevation
	exec := executor.NewDefaultExecutor(
		executor.WithFileSystem(mockFS),
		executor.WithPrivilegeManager(mockPrivMgr),
	)

	// Test command
	cmd := runnertypes.Command{
		Name:       "test-unsupported",
		Cmd:        "/bin/echo",
		Args:       []string{"test"},
		Privileged: true,
	}

	// Execute
	_, err := exec.Execute(context.Background(), cmd, map[string]string{})

	// Assertion
	assert.ErrorIs(t, err, privilege.ErrPlatformNotSupported)
}
