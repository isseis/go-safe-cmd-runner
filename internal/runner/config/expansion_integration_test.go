//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExpandTemplateToSpec tests the expandTemplateToSpec function that expands
// template parameters into CommandSpec fields
func TestExpandTemplateToSpec(t *testing.T) {
	tests := []struct {
		name         string
		command      *runnertypes.CommandSpec
		template     *runnertypes.CommandTemplate
		templateName string
		expectSpec   *runnertypes.CommandSpec
		expectWarns  []string
		expectErr    bool
		wantErrType  error
	}{
		{
			name: "expand required parameters",
			command: &runnertypes.CommandSpec{
				Name:     "backup",
				Template: "backup_tmpl",
				Params: map[string]any{
					"source": "/data",
					"dest":   "/backup",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "rsync",
				Args: []string{"-av", "${source}", "${dest}"},
			},
			templateName: "backup_tmpl",
			expectSpec: &runnertypes.CommandSpec{
				Name: "backup",
				Cmd:  "rsync",
				Args: []string{"-av", "/data", "/backup"},
			},
			expectWarns: []string{},
		},
		{
			name: "expand optional parameter - provided",
			command: &runnertypes.CommandSpec{
				Name:     "verbose_cmd",
				Template: "optional_tmpl",
				Params: map[string]any{
					"verbose": "true",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${?verbose}"},
			},
			templateName: "optional_tmpl",
			expectSpec: &runnertypes.CommandSpec{
				Name: "verbose_cmd",
				Cmd:  "echo",
				Args: []string{"true"},
			},
			expectWarns: []string{},
		},
		{
			name: "expand optional parameter - missing",
			command: &runnertypes.CommandSpec{
				Name:     "quiet_cmd",
				Template: "optional_tmpl",
				Params:   map[string]any{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${?verbose}"},
			},
			templateName: "optional_tmpl",
			expectSpec: &runnertypes.CommandSpec{
				Name: "quiet_cmd",
				Cmd:  "echo",
				Args: nil, // nil when all args are optional and missing
			},
			expectWarns: nil,
		},
		{
			name: "expand array parameter",
			command: &runnertypes.CommandSpec{
				Name:     "multi_arg",
				Template: "array_tmpl",
				Params: map[string]any{
					"flags": []any{"--verbose", "--force"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "deploy",
				Args: []string{"${@flags}", "app"},
			},
			templateName: "array_tmpl",
			expectSpec: &runnertypes.CommandSpec{
				Name: "multi_arg",
				Cmd:  "deploy",
				Args: []string{"--verbose", "--force", "app"},
			},
			expectWarns: []string{},
		},
		{
			name: "expand workdir from template",
			command: &runnertypes.CommandSpec{
				Name:     "workdir_cmd",
				Template: "workdir_tmpl",
				Params: map[string]any{
					"project": "myapp",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "make",
				WorkDir: runnertypes.StringPtr("/projects/${project}"),
			},
			templateName: "workdir_tmpl",
			expectSpec: &runnertypes.CommandSpec{
				Name:    "workdir_cmd",
				Cmd:     "make",
				WorkDir: runnertypes.StringPtr("/projects/myapp"),
			},
			expectWarns: []string{},
		},
		{
			name: "command workdir overrides template workdir",
			command: &runnertypes.CommandSpec{
				Name:     "custom_workdir_cmd",
				Template: "workdir_tmpl",
				WorkDir:  runnertypes.StringPtr("/custom/dir"),
				Params: map[string]any{
					"project": "myapp",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "make",
				WorkDir: runnertypes.StringPtr("/projects/${project}"),
			},
			templateName: "workdir_tmpl",
			expectSpec: &runnertypes.CommandSpec{
				Name:    "custom_workdir_cmd",
				Cmd:     "make",
				WorkDir: runnertypes.StringPtr("/custom/dir"),
			},
			expectWarns: []string{},
		},
		{
			name: "empty template workdir uses command workdir",
			command: &runnertypes.CommandSpec{
				Name:     "cmd_only_workdir",
				Template: "no_workdir_tmpl",
				WorkDir:  runnertypes.StringPtr("/my/workdir"),
				Params:   map[string]any{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
			},
			templateName: "no_workdir_tmpl",
			expectSpec: &runnertypes.CommandSpec{
				Name:    "cmd_only_workdir",
				Cmd:     "echo",
				WorkDir: runnertypes.StringPtr("/my/workdir"),
			},
			expectWarns: []string{},
		},
		{
			name: "unused parameter warning",
			command: &runnertypes.CommandSpec{
				Name:     "unused_param",
				Template: "simple_tmpl",
				Params: map[string]any{
					"used":   "value1",
					"unused": "value2",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${used}"},
			},
			templateName: "simple_tmpl",
			expectSpec: &runnertypes.CommandSpec{
				Name: "unused_param",
				Cmd:  "echo",
				Args: []string{"value1"},
			},
			expectWarns: []string{
				"unused parameter \"unused\" in template \"simple_tmpl\" for command \"unused_param\"",
			},
		},
		{
			name: "missing required parameter",
			command: &runnertypes.CommandSpec{
				Name:     "missing_param",
				Template: "required_tmpl",
				Params:   map[string]any{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${required}"},
			},
			templateName: "required_tmpl",
			expectErr:    true,
			wantErrType:  &ErrRequiredParamMissing{},
		},
		{
			name: "invalid array parameter type",
			command: &runnertypes.CommandSpec{
				Name:     "invalid_array",
				Template: "array_tmpl",
				Params: map[string]any{
					"flags": "not-an-array",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "deploy",
				Args: []string{"${@flags}"},
			},
			templateName: "array_tmpl",
			expectErr:    true,
			wantErrType:  &ErrTemplateTypeMismatch{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, warns, err := expandTemplateToSpec(tt.command, tt.template, tt.templateName)

			if tt.expectErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectSpec.Name, spec.Name)
			assert.Equal(t, tt.expectSpec.Cmd, spec.Cmd)
			assert.Equal(t, tt.expectSpec.Args, spec.Args)
			assert.Equal(t, tt.expectSpec.WorkDir, spec.WorkDir)

			// Compare warnings - handle nil vs empty slice
			if len(tt.expectWarns) == 0 && len(warns) == 0 {
				// Both are effectively empty
				return
			}
			assert.Equal(t, tt.expectWarns, warns)
		})
	}
}

// TestExpandCommandWithTemplate tests end-to-end template expansion through ExpandCommand
func TestExpandCommandWithTemplate(t *testing.T) {
	templates := map[string]runnertypes.CommandTemplate{
		"echo_tmpl": {
			Cmd:  "echo",
			Args: []string{"${message}"},
		},
	}

	tests := []struct {
		name        string
		spec        *runnertypes.CommandSpec
		expectCmd   string
		expectArgs  []string
		expectErr   bool
		wantErrType error
	}{
		{
			name: "valid template expansion",
			spec: &runnertypes.CommandSpec{
				Name:     "hello",
				Template: "echo_tmpl",
				Params: map[string]any{
					"message": "Hello World",
				},
			},
			expectCmd:  "echo",
			expectArgs: []string{"Hello World"},
		},
		{
			name: "template not found",
			spec: &runnertypes.CommandSpec{
				Name:     "missing",
				Template: "nonexistent",
				Params:   map[string]any{},
			},
			expectErr:   true,
			wantErrType: &ErrTemplateNotFound{},
		},
		{
			name: "exclusivity violation - template with cmd",
			spec: &runnertypes.CommandSpec{
				Name:     "invalid",
				Template: "echo_tmpl",
				Cmd:      "ls",
				Params:   map[string]any{},
			},
			expectErr:   true,
			wantErrType: &ErrTemplateFieldConflict{},
		},
		{
			name: "unused parameter - should log warning",
			spec: &runnertypes.CommandSpec{
				Name:     "with_unused",
				Template: "echo_tmpl",
				Params: map[string]any{
					"message": "Hello",
					"unused":  "ignored",
				},
			},
			expectCmd:  "echo",
			expectArgs: []string{"Hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create minimal runtime environment
			runtimeGroup := &runnertypes.RuntimeGroup{
				Spec: &runnertypes.GroupSpec{
					Name: "test-group",
				},
			}
			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{},
			}

			runtime, err := ExpandCommand(
				tt.spec,
				templates,
				runtimeGroup,
				runtimeGlobal,
				common.NewUnsetTimeout(),
				commontesting.NewUnsetOutputSizeLimit(),
			)

			if tt.expectErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, runtime)
			assert.Equal(t, tt.expectCmd, runtime.Cmd())
			assert.Equal(t, tt.expectArgs, runtime.Args())
		})
	}
}

// TestExpandTemplateToSpec_ArrayInEnvWorkdir tests that array parameters
// are properly rejected in env and workdir fields (not silently truncated)
func TestExpandTemplateToSpec_ArrayInEnvWorkdir(t *testing.T) {
	tests := []struct {
		name         string
		command      *runnertypes.CommandSpec
		template     *runnertypes.CommandTemplate
		templateName string
		expectErr    bool
		wantErrType  error
	}{
		{
			name: "array parameter in env - should fail",
			command: &runnertypes.CommandSpec{
				Name:     "bad_env",
				Template: "env_tmpl",
				Params: map[string]any{
					"paths": []any{"/path1", "/path2"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				EnvVars: []string{"PATHS=${@paths}"},
			},
			templateName: "env_tmpl",
			expectErr:    true,
			wantErrType:  &ErrArrayInMixedContext{},
		},
		{
			name: "array parameter in workdir - should fail",
			command: &runnertypes.CommandSpec{
				Name:     "bad_workdir",
				Template: "workdir_tmpl",
				Params: map[string]any{
					"dirs": []any{"/dir1", "/dir2"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: runnertypes.StringPtr("${@dirs}"),
			},
			templateName: "workdir_tmpl",
			expectErr:    true,
			wantErrType:  &ErrArrayInMixedContext{},
		},
		{
			name: "single-element array in env - should also be rejected",
			command: &runnertypes.CommandSpec{
				Name:     "single_array_env",
				Template: "env_tmpl",
				Params: map[string]any{
					"path": []any{"/single/path"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				EnvVars: []string{"PATH=${@path}"},
			},
			templateName: "env_tmpl",
			expectErr:    true,
			wantErrType:  &ErrArrayInMixedContext{},
		},
		{
			name: "single-element array in workdir - should also be rejected",
			command: &runnertypes.CommandSpec{
				Name:     "single_array_workdir",
				Template: "workdir_tmpl",
				Params: map[string]any{
					"dir": []any{"/single/dir"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: runnertypes.StringPtr("${@dir}"),
			},
			templateName: "workdir_tmpl",
			expectErr:    true,
			wantErrType:  &ErrArrayInMixedContext{},
		},
		{
			name: "regular string parameter in env - should work",
			command: &runnertypes.CommandSpec{
				Name:     "good_env",
				Template: "env_tmpl",
				Params: map[string]any{
					"value": "test123",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				EnvVars: []string{"VALUE=${value}"},
			},
			templateName: "env_tmpl",
			expectErr:    false,
		},
		{
			name: "regular string parameter in workdir - should work",
			command: &runnertypes.CommandSpec{
				Name:     "good_workdir",
				Template: "workdir_tmpl",
				Params: map[string]any{
					"path": "/working/dir",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: runnertypes.StringPtr("${path}"),
			},
			templateName: "workdir_tmpl",
			expectErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := expandTemplateToSpec(tt.command, tt.template, tt.templateName)

			if tt.expectErr {
				require.Error(t, err, "expected error but got none")
				if tt.wantErrType != nil {
					require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}
