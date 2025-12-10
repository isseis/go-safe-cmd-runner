//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOptionalParameter_InEnv tests optional parameter behavior in env field
func TestOptionalParameter_InEnv(t *testing.T) {
	tests := []struct {
		name        string
		command     *runnertypes.CommandSpec
		template    *runnertypes.CommandTemplate
		expectEnv   []string
		description string
	}{
		{
			name: "pure optional in env value - parameter provided",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"value": "myvalue",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"MYKEY=${?value}"},
			},
			expectEnv:   []string{"MYKEY=myvalue"},
			description: "Pure optional in env value should return the value when provided",
		},
		{
			name: "pure optional in env value - parameter missing",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params:   map[string]interface{}{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"MYKEY=${?value}"},
			},
			expectEnv:   []string{},
			description: "Pure optional in env value should be removed when parameter missing",
		},
		{
			name: "pure optional in env value - parameter is empty string",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"value": "",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"MYKEY=${?value}"},
			},
			expectEnv:   []string{},
			description: "Pure optional in env value should be removed when parameter is empty",
		},
		{
			name: "mixed optional in env value - parameter provided",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"value": "myvalue",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"MYKEY=prefix_${?value}_suffix"},
			},
			expectEnv:   []string{"MYKEY=prefix_myvalue_suffix"},
			description: "Mixed context optional in env value should substitute value",
		},
		{
			name: "mixed optional in env value - parameter missing",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params:   map[string]interface{}{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"MYKEY=prefix_${?value}_suffix"},
			},
			expectEnv:   []string{"MYKEY=prefix__suffix"},
			description: "Mixed context optional in env value should substitute empty when missing",
		},
		{
			name: "multiple env entries with optional - some removed",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"val1": "first",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"VAR1=${?val1}", "VAR2=${?val2}", "VAR3=static"},
			},
			expectEnv:   []string{"VAR1=first", "VAR3=static"},
			description: "Multiple env entries - only entries with values should be included",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, _, err := expandTemplateToSpec(tt.command, tt.template, "env_tmpl")
			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectEnv, spec.EnvVars, tt.description)
		})
	}
}

// TestOptionalParameter_EnvKeyWithPlaceholder tests that placeholders in env keys are rejected
func TestOptionalParameter_EnvKeyWithPlaceholder(t *testing.T) {
	tests := []struct {
		name        string
		command     *runnertypes.CommandSpec
		template    *runnertypes.CommandTemplate
		wantErrType error
		description string
	}{
		{
			name: "required placeholder in env key - should fail",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"key": "MYKEY",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"${key}=value"},
			},
			wantErrType: &ErrPlaceholderInEnvKey{},
			description: "Required placeholder in env key should be rejected",
		},
		{
			name: "optional placeholder in env key - should fail",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"key": "MYKEY",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"${?key}=value"},
			},
			wantErrType: &ErrPlaceholderInEnvKey{},
			description: "Optional placeholder in env key should be rejected",
		},
		{
			name: "array placeholder in env key - should fail",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"keys": []interface{}{"KEY1", "KEY2"},
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"${@keys}=value"},
			},
			wantErrType: &ErrPlaceholderInEnvKey{},
			description: "Array placeholder in env key should be rejected",
		},
		{
			name: "placeholder in middle of env key - should fail",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "env_tmpl",
				Params: map[string]interface{}{
					"prefix": "MY",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"${prefix}_KEY=value"},
			},
			wantErrType: &ErrPlaceholderInEnvKey{},
			description: "Placeholder in middle of env key should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := expandTemplateToSpec(tt.command, tt.template, "env_tmpl")
			require.Error(t, err, tt.description)
			if tt.wantErrType != nil {
				require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
			}
		})
	}
}

// TestOptionalParameter_InWorkDir tests optional parameter behavior in workdir field
func TestOptionalParameter_InWorkDir(t *testing.T) {
	tests := []struct {
		name        string
		command     *runnertypes.CommandSpec
		template    *runnertypes.CommandTemplate
		expectDir   string
		description string
	}{
		{
			name: "pure optional in workdir - parameter provided",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "dir_tmpl",
				Params: map[string]interface{}{
					"dir": "/my/dir",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: "${?dir}",
			},
			expectDir:   "/my/dir",
			description: "Pure optional should return the value when provided",
		},
		{
			name: "pure optional in workdir - parameter missing",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "dir_tmpl",
				Params:   map[string]interface{}{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: "${?dir}",
			},
			expectDir:   "",
			description: "Pure optional workdir should be empty when parameter missing",
		},
		{
			name: "mixed optional in workdir - parameter provided",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "dir_tmpl",
				Params: map[string]interface{}{
					"subdir": "myproject",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: "/home/${?subdir}",
			},
			expectDir:   "/home/myproject",
			description: "Mixed context optional should substitute value",
		},
		{
			name: "mixed optional in workdir - parameter missing",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "dir_tmpl",
				Params:   map[string]interface{}{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: "/home/${?subdir}",
			},
			expectDir:   "/home/",
			description: "Mixed context optional should substitute empty string when missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, _, err := expandTemplateToSpec(tt.command, tt.template, "dir_tmpl")
			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectDir, spec.WorkDir, tt.description)
		})
	}
}

// TestOptionalParameter_InCmd tests optional parameter behavior in cmd field
func TestOptionalParameter_InCmd(t *testing.T) {
	tests := []struct {
		name        string
		command     *runnertypes.CommandSpec
		template    *runnertypes.CommandTemplate
		expectCmd   string
		wantErr     bool
		wantErrType error
		description string
	}{
		{
			name: "pure optional in cmd - parameter provided",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "cmd_tmpl",
				Params: map[string]interface{}{
					"cmd": "echo",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "${?cmd}",
			},
			expectCmd:   "echo",
			description: "Pure optional cmd should work when provided",
		},
		{
			name: "pure optional in cmd - parameter missing",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "cmd_tmpl",
				Params:   map[string]interface{}{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "${?cmd}",
			},
			wantErr:     true,
			wantErrType: &ErrTemplateCmdNotSingleValue{},
			description: "Pure optional cmd missing should fail validation",
		},
		{
			name: "mixed optional in cmd - parameter provided",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "cmd_tmpl",
				Params: map[string]interface{}{
					"prefix": "/usr/local/bin/",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd: "${?prefix}echo",
			},
			expectCmd:   "/usr/local/bin/echo",
			description: "Mixed context optional in cmd should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, _, err := expandTemplateToSpec(tt.command, tt.template, "cmd_tmpl")

			if tt.wantErr {
				require.Error(t, err, tt.description)
				if tt.wantErrType != nil {
					require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
				return
			}

			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectCmd, spec.Cmd, tt.description)
		})
	}
}

// TestOptionalParameter_InArgs tests optional parameter behavior in args field
func TestOptionalParameter_InArgs(t *testing.T) {
	tests := []struct {
		name        string
		command     *runnertypes.CommandSpec
		template    *runnertypes.CommandTemplate
		expectArgs  []string
		description string
	}{
		{
			name: "pure optional in args - parameter provided",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "args_tmpl",
				Params: map[string]interface{}{
					"flag": "--verbose",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${?flag}", "file.txt"},
			},
			expectArgs:  []string{"--verbose", "file.txt"},
			description: "Pure optional arg should be included when provided",
		},
		{
			name: "pure optional in args - parameter missing",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "args_tmpl",
				Params:   map[string]interface{}{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${?flag}", "file.txt"},
			},
			expectArgs:  []string{"file.txt"},
			description: "Pure optional arg should be removed when missing",
		},
		{
			name: "mixed optional in args - parameter provided",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "args_tmpl",
				Params: map[string]interface{}{
					"level": "3",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"--level=${?level}"},
			},
			expectArgs:  []string{"--level=3"},
			description: "Mixed context optional in args should substitute value",
		},
		{
			name: "mixed optional in args - parameter missing",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "args_tmpl",
				Params:   map[string]interface{}{},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"--level=${?level}"},
			},
			expectArgs:  []string{"--level="},
			description: "Mixed context optional in args should substitute empty when missing",
		},
		{
			name: "multiple optional args - some missing",
			command: &runnertypes.CommandSpec{
				Name:     "test_cmd",
				Template: "args_tmpl",
				Params: map[string]interface{}{
					"arg1": "value1",
				},
			},
			template: &runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${?arg1}", "${?arg2}", "${?arg3}"},
			},
			expectArgs:  []string{"value1"},
			description: "Only provided optional args should be included",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, _, err := expandTemplateToSpec(tt.command, tt.template, "args_tmpl")
			require.NoError(t, err, tt.description)
			assert.Equal(t, tt.expectArgs, spec.Args, tt.description)
		})
	}
}
