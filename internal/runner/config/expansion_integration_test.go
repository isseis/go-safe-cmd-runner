//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
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
		errContains  string
	}{
		{
			name: "expand required parameters",
			command: &runnertypes.CommandSpec{
				Name:     "backup",
				Template: "backup_tmpl",
				Params: map[string]interface{}{
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
				Params: map[string]interface{}{
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
				Params:   map[string]interface{}{},
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
				Params: map[string]interface{}{
					"flags": []interface{}{"--verbose", "--force"},
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
			name: "expand workdir",
			command: &runnertypes.CommandSpec{
				Name:     "workdir_cmd",
				Template: "workdir_tmpl",
				Params: map[string]interface{}{
					"project": "myapp",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "make",
				WorkDir: "/projects/${project}",
			},
			templateName: "workdir_tmpl",
			expectSpec: &runnertypes.CommandSpec{
				Name:    "workdir_cmd",
				Cmd:     "make",
				WorkDir: "/projects/myapp",
			},
			expectWarns: []string{},
		},
		{
			name: "unused parameter warning",
			command: &runnertypes.CommandSpec{
				Name:     "unused_param",
				Template: "simple_tmpl",
				Params: map[string]interface{}{
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
				Params:   map[string]interface{}{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${required}"},
			},
			templateName: "required_tmpl",
			expectErr:    true,
			wantErrType:  &ErrRequiredParamMissing{},
			errContains:  "required parameter \"required\" not provided",
		},
		{
			name: "invalid array parameter type",
			command: &runnertypes.CommandSpec{
				Name:     "invalid_array",
				Template: "array_tmpl",
				Params: map[string]interface{}{
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
			errContains:  "expected array, got string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, warns, err := expandTemplateToSpec(tt.command, tt.template, tt.templateName)

			if tt.expectErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					assert.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
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
		errContains string
	}{
		{
			name: "valid template expansion",
			spec: &runnertypes.CommandSpec{
				Name:     "hello",
				Template: "echo_tmpl",
				Params: map[string]interface{}{
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
				Params:   map[string]interface{}{},
			},
			expectErr:   true,
			wantErrType: &ErrTemplateNotFound{},
			errContains: "template \"nonexistent\" not found",
		},
		{
			name: "exclusivity violation - template with cmd",
			spec: &runnertypes.CommandSpec{
				Name:     "invalid",
				Template: "echo_tmpl",
				Cmd:      "ls",
				Params:   map[string]interface{}{},
			},
			expectErr:   true,
			wantErrType: &ErrTemplateFieldConflict{},
			errContains: "cannot specify both \"template\" and \"cmd\"",
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
					assert.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
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
		errContains  string
	}{
		{
			name: "array parameter in env - should fail",
			command: &runnertypes.CommandSpec{
				Name:     "bad_env",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"paths": []interface{}{"/path1", "/path2"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"PATHS=${@paths}"},
			},
			templateName: "env_tmpl",
			expectErr:    true,
			wantErrType:  &ErrArrayInMixedContext{},
			errContains:  "cannot be used in mixed context",
		},
		{
			name: "array parameter in workdir - should fail",
			command: &runnertypes.CommandSpec{
				Name:     "bad_workdir",
				Template: "workdir_tmpl",
				Params: map[string]interface{}{
					"dirs": []interface{}{"/dir1", "/dir2"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: "${@dirs}",
			},
			templateName: "workdir_tmpl",
			expectErr:    true,
			wantErrType:  &ErrArrayInMixedContext{},
			errContains:  "cannot be used in mixed context",
		},
		{
			name: "single-element array in env - should also be rejected",
			command: &runnertypes.CommandSpec{
				Name:     "single_array_env",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"path": []interface{}{"/single/path"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"PATH=${@path}"},
			},
			templateName: "env_tmpl",
			expectErr:    true,
			wantErrType:  &ErrArrayInMixedContext{},
			errContains:  "cannot be used in mixed context",
		},
		{
			name: "single-element array in workdir - should also be rejected",
			command: &runnertypes.CommandSpec{
				Name:     "single_array_workdir",
				Template: "workdir_tmpl",
				Params: map[string]interface{}{
					"dir": []interface{}{"/single/dir"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: "${@dir}",
			},
			templateName: "workdir_tmpl",
			expectErr:    true,
			wantErrType:  &ErrArrayInMixedContext{},
			errContains:  "cannot be used in mixed context",
		},
		{
			name: "regular string parameter in env - should work",
			command: &runnertypes.CommandSpec{
				Name:     "good_env",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"value": "test123",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"VALUE=${value}"},
			},
			templateName: "env_tmpl",
			expectErr:    false,
		},
		{
			name: "regular string parameter in workdir - should work",
			command: &runnertypes.CommandSpec{
				Name:     "good_workdir",
				Template: "workdir_tmpl",
				Params: map[string]interface{}{
					"path": "/working/dir",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: "${path}",
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
					assert.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains,
						"error should contain %q", tt.errContains)
				}
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}
