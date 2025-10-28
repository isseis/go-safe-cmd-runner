package executor

import (
	"context"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestDefaultExecutor_validatePrivilegedCommand(t *testing.T) {
	exec := &DefaultExecutor{}

	tests := []struct {
		name        string
		cmd         *runnertypes.RuntimeCommand
		wantErr     bool
		errContains string
	}{
		{
			name: "valid privileged command with absolute path",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/usr/bin/systemctl",
				ExpandedArgs:     []string{"start", "nginx"},
				EffectiveWorkDir: "/tmp",
			},
			wantErr: false,
		},
		{
			name: "invalid - empty command",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "",
				ExpandedArgs:     []string{"arg1"},
				EffectiveWorkDir: "/tmp",
			},
			wantErr:     true,
			errContains: "command cannot be empty", // Updated to match actual error message
		},
		{
			name: "invalid - relative command path",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "systemctl",
				ExpandedArgs:     []string{"start", "nginx"},
				EffectiveWorkDir: "/tmp",
			},
			wantErr:     true,
			errContains: "privileged commands must use absolute paths",
		},
		{
			name: "invalid - relative working directory",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/usr/bin/systemctl",
				ExpandedArgs:     []string{"start", "nginx"},
				EffectiveWorkDir: "relative/path",
			},
			wantErr:     true,
			errContains: "privileged commands must use absolute working directory paths",
		},
		{
			name: "valid - no working directory specified",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/usr/bin/systemctl",
				ExpandedArgs:     []string{"restart", "apache2"},
				EffectiveWorkDir: "",
			},
			wantErr: false,
		},
		{
			name: "valid - absolute paths for both command and workdir",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/bin/ls",
				ExpandedArgs:     []string{"-la", "/etc"},
				EffectiveWorkDir: "/var/log",
			},
			wantErr: false,
		},
		{
			name: "invalid - command with . prefix (relative)",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "./script.sh",
				ExpandedArgs:     []string{},
				EffectiveWorkDir: "/tmp",
			},
			wantErr:     true,
			errContains: "privileged commands must use absolute paths",
		},
		{
			name: "invalid - workdir with . prefix",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/usr/bin/make",
				ExpandedArgs:     []string{"install"},
				EffectiveWorkDir: "./build",
			},
			wantErr:     true,
			errContains: "privileged commands must use absolute working directory paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exec.validatePrivilegedCommand(tt.cmd)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateCommandContextWithTimeout(t *testing.T) {
	tests := []struct {
		name          string
		timeout       int
		shouldTimeout bool
		description   string
	}{
		{
			name:          "unlimited execution with zero timeout",
			timeout:       0,
			shouldTimeout: false,
			description:   "Context should not have a deadline",
		},
		{
			name:          "unlimited execution with negative timeout",
			timeout:       -1,
			shouldTimeout: false,
			description:   "Context should not have a deadline",
		},
		{
			name:          "limited execution with positive timeout",
			timeout:       5,
			shouldTimeout: true,
			description:   "Context should have a 5-second deadline",
		},
		{
			name:          "limited execution with 1 second timeout",
			timeout:       1,
			shouldTimeout: true,
			description:   "Context should have a 1-second deadline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			resultCtx, cancel := CreateCommandContextWithTimeout(ctx, tt.timeout)
			defer cancel()

			assert.NotNil(t, resultCtx, "Context should not be nil")
			assert.NotNil(t, cancel, "Cancel function should not be nil")

			if tt.shouldTimeout {
				deadline, ok := resultCtx.Deadline()
				assert.True(t, ok, "Context should have a deadline for timeout > 0")
				assert.False(t, deadline.IsZero(), "Deadline should not be zero")
			} else {
				_, ok := resultCtx.Deadline()
				assert.False(t, ok, "Context should not have a deadline for timeout <= 0")
			}

			// Test that cancel function works
			cancel()
			select {
			case <-resultCtx.Done():
				// Context was properly cancelled
			default:
				assert.Fail(t, "Context should be done after calling cancel")
			}
		})
	}
}
