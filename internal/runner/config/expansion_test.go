//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyTemplateInheritance_WorkDir(t *testing.T) {
	tests := []struct {
		name            string
		cmdWorkDir      *string
		templateWorkDir *string
		expandedWorkDir *string
		expectedWorkDir *string
	}{
		{
			name:            "command overrides template",
			cmdWorkDir:      new("/cmd/dir"),
			templateWorkDir: new("/tmpl/dir"),
			expandedWorkDir: new("/tmpl/dir"),
			expectedWorkDir: new("/cmd/dir"),
		},
		{
			name:            "command inherits from template",
			cmdWorkDir:      nil,
			templateWorkDir: new("/tmpl/dir"),
			expandedWorkDir: new("/tmpl/dir"),
			expectedWorkDir: new("/tmpl/dir"),
		},
		{
			name:            "both nil",
			cmdWorkDir:      nil,
			templateWorkDir: nil,
			expandedWorkDir: nil,
			expectedWorkDir: nil,
		},
		{
			name:            "command empty string overrides template",
			cmdWorkDir:      new(""),
			templateWorkDir: new("/tmpl/dir"),
			expandedWorkDir: new("/tmpl/dir"),
			expectedWorkDir: new(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandedSpec := &runnertypes.CommandSpec{}
			cmdSpec := &runnertypes.CommandSpec{WorkDir: tt.cmdWorkDir}

			ApplyTemplateInheritance(expandedSpec, cmdSpec, tt.expandedWorkDir, nil, nil, nil)

			if tt.expectedWorkDir == nil {
				assert.Nil(t, expandedSpec.WorkDir)
			} else {
				assert.NotNil(t, expandedSpec.WorkDir)
				assert.Equal(t, *tt.expectedWorkDir, *expandedSpec.WorkDir)
			}
		})
	}
}

func TestApplyTemplateInheritance_OutputFile(t *testing.T) {
	tests := []struct {
		name               string
		cmdOutputFile      *string
		templateOutputFile *string
		expandedOutputFile *string
		expectedOutputFile *string
	}{
		{
			name:               "command overrides template",
			cmdOutputFile:      new("/cmd/output.txt"),
			templateOutputFile: new("/tmpl/output.txt"),
			expandedOutputFile: new("/tmpl/output.txt"),
			expectedOutputFile: new("/cmd/output.txt"),
		},
		{
			name:               "command inherits from template",
			cmdOutputFile:      nil,
			templateOutputFile: new("/tmpl/output.txt"),
			expandedOutputFile: new("/tmpl/output.txt"),
			expectedOutputFile: new("/tmpl/output.txt"),
		},
		{
			name:               "both nil",
			cmdOutputFile:      nil,
			templateOutputFile: nil,
			expandedOutputFile: nil,
			expectedOutputFile: nil,
		},
		{
			name:               "command empty string overrides template",
			cmdOutputFile:      new(""),
			templateOutputFile: new("/tmpl/output.txt"),
			expandedOutputFile: new("/tmpl/output.txt"),
			expectedOutputFile: new(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandedSpec := &runnertypes.CommandSpec{}
			cmdSpec := &runnertypes.CommandSpec{OutputFile: tt.cmdOutputFile}

			ApplyTemplateInheritance(expandedSpec, cmdSpec, nil, tt.expandedOutputFile, nil, nil)

			if tt.expectedOutputFile == nil {
				assert.Nil(t, expandedSpec.OutputFile)
			} else {
				assert.NotNil(t, expandedSpec.OutputFile)
				assert.Equal(t, *tt.expectedOutputFile, *expandedSpec.OutputFile)
			}
		})
	}
}

func TestApplyTemplateInheritance_EnvImport(t *testing.T) {
	tests := []struct {
		name              string
		templateEnvImport []string
		cmdEnvImport      []string
		expectedEnvImport []string
	}{
		{
			name:              "template only",
			templateEnvImport: []string{"TMPL_VAR1", "TMPL_VAR2"},
			cmdEnvImport:      nil,
			expectedEnvImport: []string{"TMPL_VAR1", "TMPL_VAR2"},
		},
		{
			name:              "command only",
			templateEnvImport: nil,
			cmdEnvImport:      []string{"CMD_VAR1"},
			expectedEnvImport: []string{"CMD_VAR1"},
		},
		{
			name:              "merged without duplicates",
			templateEnvImport: []string{"TMPL_VAR1", "SHARED"},
			cmdEnvImport:      []string{"CMD_VAR1", "SHARED"},
			expectedEnvImport: []string{"TMPL_VAR1", "SHARED", "CMD_VAR1"},
		},
		{
			name:              "both empty",
			templateEnvImport: []string{},
			cmdEnvImport:      []string{},
			expectedEnvImport: []string{},
		},
		{
			name:              "both nil",
			templateEnvImport: nil,
			cmdEnvImport:      nil,
			expectedEnvImport: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandedSpec := &runnertypes.CommandSpec{}
			cmdSpec := &runnertypes.CommandSpec{EnvImport: tt.cmdEnvImport}

			ApplyTemplateInheritance(expandedSpec, cmdSpec, nil, nil, tt.templateEnvImport, nil)

			assert.Equal(t, tt.expectedEnvImport, expandedSpec.EnvImport)
		})
	}
}

func TestApplyTemplateInheritance_Vars(t *testing.T) {
	tests := []struct {
		name         string
		templateVars map[string]any
		cmdVars      map[string]any
		expectedVars map[string]any
	}{
		{
			name:         "template only",
			templateVars: map[string]any{"tmpl_key": "tmpl_val"},
			cmdVars:      nil,
			expectedVars: map[string]any{"tmpl_key": "tmpl_val"},
		},
		{
			name:         "command only",
			templateVars: nil,
			cmdVars:      map[string]any{"cmd_key": "cmd_val"},
			expectedVars: map[string]any{"cmd_key": "cmd_val"},
		},
		{
			name:         "merged with command precedence",
			templateVars: map[string]any{"shared_key": "tmpl_val", "tmpl_only": "val1"},
			cmdVars:      map[string]any{"shared_key": "cmd_val", "cmd_only": "val2"},
			expectedVars: map[string]any{"shared_key": "cmd_val", "tmpl_only": "val1", "cmd_only": "val2"},
		},
		{
			name:         "both empty",
			templateVars: map[string]any{},
			cmdVars:      map[string]any{},
			expectedVars: map[string]any{},
		},
		{
			name:         "both nil",
			templateVars: nil,
			cmdVars:      nil,
			expectedVars: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandedSpec := &runnertypes.CommandSpec{}
			cmdSpec := &runnertypes.CommandSpec{Vars: tt.cmdVars}

			ApplyTemplateInheritance(expandedSpec, cmdSpec, nil, nil, nil, tt.templateVars)

			assert.Equal(t, tt.expectedVars, expandedSpec.Vars)
		})
	}
}

func TestApplyTemplateInheritance_Combined(t *testing.T) {
	// Test all inheritance models together
	expandedSpec := &runnertypes.CommandSpec{}
	cmdSpec := &runnertypes.CommandSpec{
		WorkDir:    nil,                                    // Inherit from template
		OutputFile: new("/cmd/output.txt"),                 // Override template
		EnvImport:  []string{"CMD_VAR", "SHARED"},          // Merge with template
		Vars:       map[string]any{"cmd_key": "cmd_value"}, // Merge with template
	}
	expandedWorkDir := new("/tmpl/dir")
	expandedOutputFile := new("/tmpl/output.txt")
	expandedEnvImport := []string{"TMPL_VAR", "SHARED"}
	expandedVars := map[string]any{"tmpl_key": "tmpl_value", "cmd_key": "tmpl_override"}

	ApplyTemplateInheritance(expandedSpec, cmdSpec, expandedWorkDir, expandedOutputFile, expandedEnvImport, expandedVars)

	// WorkDir: Inherited from template
	assert.NotNil(t, expandedSpec.WorkDir)
	assert.Equal(t, "/tmpl/dir", *expandedSpec.WorkDir)

	// OutputFile: Overridden by command
	assert.NotNil(t, expandedSpec.OutputFile)
	assert.Equal(t, "/cmd/output.txt", *expandedSpec.OutputFile)

	// EnvImport: Merged (template first, deduplicated)
	assert.Equal(t, []string{"TMPL_VAR", "SHARED", "CMD_VAR"}, expandedSpec.EnvImport)

	// Vars: Merged with command precedence
	expectedVars := map[string]any{
		"tmpl_key": "tmpl_value",
		"cmd_key":  "cmd_value", // Command overrides template
	}
	assert.Equal(t, expectedVars, expandedSpec.Vars)
}

func TestExpandGlobal_SystemEnvIncludesAllParsableEntries(t *testing.T) {
	t.Setenv("TEST_NOT_IN_ALLOWLIST", "test_value")

	spec := &runnertypes.GlobalSpec{
		Timeout:    new(int32(30)),
		EnvAllowed: []string{},
		EnvImport:  []string{},
		Vars:       map[string]any{},
	}

	runtime, err := ExpandGlobal(spec)
	require.NoError(t, err)

	assert.Contains(t, runtime.SystemEnv, "TEST_NOT_IN_ALLOWLIST")
	assert.Equal(t, "test_value", runtime.SystemEnv["TEST_NOT_IN_ALLOWLIST"])

	directEnv := environment.ParseSystemEnvironment()
	assert.Equal(t, directEnv["TEST_NOT_IN_ALLOWLIST"], runtime.SystemEnv["TEST_NOT_IN_ALLOWLIST"])
}

// TestExpansionWrapSites_StructuredMessage drives the wrap sites in
// expansion.go that add wording to a cause. Each must declare its fixed text as
// constants, its level/field/names with the right roles, keep the cause
// reachable through errors.Is, and render the same string as before.
func TestExpansionWrapSites_StructuredMessage(t *testing.T) {
	deploy := groupLevel("deploy")

	tests := []struct {
		name     string
		run      func() error
		want     string
		wantIs   error
		segments errmsg.Segments
	}{
		{
			name: "ProcessEnvImport rejects a forbidden system variable",
			run: func() error {
				_, err := ProcessEnvImport([]string{"x=LD_PRELOAD"}, []string{"LD_PRELOAD"}, map[string]string{}, deploy)
				return err
			},
			want:   "environment variable is forbidden: LD_PRELOAD cannot be imported via env_import (level: group[deploy])",
			wantIs: ErrForbiddenEnvVar,
			segments: errmsg.Segments{
				{Role: errmsg.RoleText, Text: "environment variable is forbidden"},
				{Role: errmsg.RoleText, Text: "LD_PRELOAD"},
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
			},
		},
		{
			name: "ProcessEnv rejects a forbidden key",
			run: func() error {
				_, err := ProcessEnv([]string{"LD_PRELOAD=x"}, nil, globalLevel())
				return err
			},
			want:   "environment variable is forbidden: LD_PRELOAD cannot be set via env_vars (level: global)",
			wantIs: ErrForbiddenEnvVar,
			segments: errmsg.Segments{
				{Role: errmsg.RoleText, Text: "environment variable is forbidden"},
				{Role: errmsg.RoleText, Text: "LD_PRELOAD"},
			},
		},
		{
			name: "expandCmdAllowed rejects an empty path",
			run: func() error {
				_, err := expandCmdAllowed([]string{""}, map[string]string{}, "deploy")
				return err
			},
			want:   "group[deploy] cmd_allowed[0]: path cannot be empty",
			wantIs: ErrEmptyPath,
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "0"},
				{Role: errmsg.RoleText, Text: "path cannot be empty"},
			},
		},
		{
			name: "ExpandGroup names the group whose env_import failed",
			run: func() error {
				_, err := ExpandGroup(&runnertypes.GroupSpec{
					Name:       "deploy",
					EnvAllowed: []string{"LD_PRELOAD"},
					EnvImport:  []string{"x=LD_PRELOAD"},
				}, nil)
				return err
			},
			want:   "failed to process group[deploy] env_import: environment variable is forbidden: LD_PRELOAD cannot be imported via env_import (level: group[deploy])",
			wantIs: ErrForbiddenEnvVar,
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "environment variable is forbidden"},
				{Role: errmsg.RoleText, Text: "LD_PRELOAD"},
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			require.Error(t, err)
			structured, ok := err.(errmsg.Structured)
			require.Truef(t, ok, "%T must implement errmsg.Structured", err)
			assert.Equal(t, tt.want, err.Error())
			assert.Equal(t, tt.segments, nonConstantSegments(structured.StructuredMessage()))
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}
		})
	}
}
