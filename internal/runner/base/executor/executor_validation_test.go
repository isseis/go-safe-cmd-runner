package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubFileSystem answers FileExists from fixed values. It is local to this
// internal test file because importing executor/testutil from package executor
// would create an import cycle.
type stubFileSystem struct {
	exists bool
	err    error
}

func (f stubFileSystem) CreateTempDir(string, string) (string, error) { return "", f.err }
func (f stubFileSystem) RemoveAll(string) error                       { return f.err }
func (f stubFileSystem) FileExists(string) (bool, error)              { return f.exists, f.err }

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

// TestExecutor_StructuredWrapTexts pins the wording and the Path roles of the
// validation and execution wraps on the in-scope command path. The value named
// in each message must be a Path segment, not Text (which would be redacted
// away) or Identifier (which would be exempted from whole-value replacement).
func TestExecutor_StructuredWrapTexts(t *testing.T) {
	statErr := errors.New("stat failed")

	tests := []struct {
		name      string
		exec      func() *DefaultExecutor
		run       func(e *DefaultExecutor) error
		want      string
		wantPaths []string
	}{
		{
			name: "validate rejects a non-local non-absolute path",
			run: func(e *DefaultExecutor) error {
				return e.Validate(&runnertypes.RuntimeCommand{ExpandedCmd: "../foo"})
			},
			want:      ErrInvalidPath.Error() + ": command path must be local or absolute: ../foo",
			wantPaths: []string{"../foo"},
		},
		{
			name: "validate rejects an unclean path",
			run: func(e *DefaultExecutor) error {
				return e.Validate(&runnertypes.RuntimeCommand{ExpandedCmd: "/usr//bin"})
			},
			want:      ErrInvalidPath.Error() + ": command path contains relative path components ('.' or '..'): /usr//bin",
			wantPaths: []string{"/usr//bin"},
		},
		{
			name: "validate reports a workdir stat failure",
			exec: func() *DefaultExecutor {
				return NewDefaultExecutor(WithFileSystem(stubFileSystem{err: statErr})).(*DefaultExecutor)
			},
			run: func(e *DefaultExecutor) error {
				return e.Validate(&runnertypes.RuntimeCommand{ExpandedCmd: "/usr/bin/echo", EffectiveWorkDir: "/work"})
			},
			want:      "failed to check directory /work: " + statErr.Error(),
			wantPaths: []string{"/work"},
		},
		{
			name: "validate reports a missing workdir",
			exec: func() *DefaultExecutor {
				return NewDefaultExecutor(WithFileSystem(stubFileSystem{exists: false})).(*DefaultExecutor)
			},
			run: func(e *DefaultExecutor) error {
				return e.Validate(&runnertypes.RuntimeCommand{ExpandedCmd: "/usr/bin/echo", EffectiveWorkDir: "/missing"})
			},
			want:      ErrDirNotExists.Error() + ": /missing",
			wantPaths: []string{"/missing"},
		},
		{
			name: "privileged validation rejects a relative command",
			run: func(e *DefaultExecutor) error {
				return e.validatePrivilegedCommand(&runnertypes.RuntimeCommand{ExpandedCmd: "systemctl"})
			},
			want:      ErrPrivilegedCmdSecurity.Error() + ": privileged commands must use absolute paths: systemctl",
			wantPaths: []string{"systemctl"},
		},
		{
			name: "privileged validation rejects a relative workdir",
			run: func(e *DefaultExecutor) error {
				return e.validatePrivilegedCommand(&runnertypes.RuntimeCommand{ExpandedCmd: "/usr/bin/systemctl", EffectiveWorkDir: "relative/path"})
			},
			want:      ErrPrivilegedCmdSecurity.Error() + ": privileged commands must use absolute working directory paths: relative/path",
			wantPaths: []string{"relative/path"},
		},
		{
			name: "executeNormal rejects a relative command path",
			run: func(e *DefaultExecutor) error {
				_, err := e.executeNormal(context.Background(), nil, &runnertypes.RuntimeCommand{ExpandedCmd: "foo"}, nil, nil)
				return err
			},
			want:      ErrPathNotAbsolute.Error() + ": foo",
			wantPaths: []string{"foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewDefaultExecutor().(*DefaultExecutor)
			if tt.exec != nil {
				e = tt.exec()
			}

			err := tt.run(e)
			require.Error(t, err)
			assert.Equal(t, tt.want, err.Error())

			structured, ok := errors.AsType[errmsg.Structured](err)
			require.True(t, ok, "the wrap must be a structured error; got %T", err)
			var paths []string
			for _, segment := range structured.StructuredMessage().Segments() {
				if segment.Role == errmsg.RolePath {
					paths = append(paths, segment.Text)
				}
			}
			assert.Equal(t, tt.wantPaths, paths)
		})
	}
}
