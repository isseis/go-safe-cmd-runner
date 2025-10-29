package executor

import (
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
			errContains: "command cannot be empty",
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
