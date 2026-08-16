package executor

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultExecutor_validatePrivilegedCommand(t *testing.T) {
	exec := &DefaultExecutor{}

	tests := []struct {
		name    string
		cmd     *runnertypes.RuntimeCommand
		wantErr error // nil when the command is expected to pass validation
		// wantErrDetail is the offending value the message must name. Both
		// rejections below share one sentinel, so the value is what says which
		// of the two — command path or working directory — was rejected.
		wantErrDetail string
	}{
		{
			name: "valid privileged command with absolute path",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/usr/bin/systemctl",
				ExpandedArgs:     []string{"start", "nginx"},
				EffectiveWorkDir: "/tmp",
			},
			wantErr: nil,
		},
		{
			name: "invalid - empty command",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "",
				ExpandedArgs:     []string{"arg1"},
				EffectiveWorkDir: "/tmp",
			},
			wantErr: ErrEmptyCommand,
		},
		{
			name: "invalid - relative command path",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "systemctl",
				ExpandedArgs:     []string{"start", "nginx"},
				EffectiveWorkDir: "/tmp",
			},
			wantErr:       ErrPrivilegedCmdSecurity,
			wantErrDetail: "systemctl",
		},
		{
			name: "invalid - relative working directory",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/usr/bin/systemctl",
				ExpandedArgs:     []string{"start", "nginx"},
				EffectiveWorkDir: "relative/path",
			},
			wantErr:       ErrPrivilegedCmdSecurity,
			wantErrDetail: "relative/path",
		},
		{
			name: "valid - no working directory specified",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/usr/bin/systemctl",
				ExpandedArgs:     []string{"restart", "apache2"},
				EffectiveWorkDir: "",
			},
			wantErr: nil,
		},
		{
			name: "valid - absolute paths for both command and workdir",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/bin/ls",
				ExpandedArgs:     []string{"-la", "/etc"},
				EffectiveWorkDir: "/var/log",
			},
			wantErr: nil,
		},
		{
			name: "invalid - command with . prefix (relative)",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "./script.sh",
				ExpandedArgs:     []string{},
				EffectiveWorkDir: "/tmp",
			},
			wantErr:       ErrPrivilegedCmdSecurity,
			wantErrDetail: "./script.sh",
		},
		{
			name: "invalid - workdir with . prefix",
			cmd: &runnertypes.RuntimeCommand{
				ExpandedCmd:      "/usr/bin/make",
				ExpandedArgs:     []string{"install"},
				EffectiveWorkDir: "./build",
			},
			wantErr:       ErrPrivilegedCmdSecurity,
			wantErrDetail: "./build",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exec.validatePrivilegedCommand(tt.cmd)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				if tt.wantErrDetail != "" {
					assert.ErrorContains(t, err, tt.wantErrDetail, "the message must name the value it rejected")
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
