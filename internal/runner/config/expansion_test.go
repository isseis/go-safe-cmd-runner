//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
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
			cmdWorkDir:      stringPtr("/cmd/dir"),
			templateWorkDir: stringPtr("/tmpl/dir"),
			expandedWorkDir: stringPtr("/tmpl/dir"),
			expectedWorkDir: stringPtr("/cmd/dir"),
		},
		{
			name:            "command inherits from template",
			cmdWorkDir:      nil,
			templateWorkDir: stringPtr("/tmpl/dir"),
			expandedWorkDir: stringPtr("/tmpl/dir"),
			expectedWorkDir: stringPtr("/tmpl/dir"),
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
			cmdWorkDir:      stringPtr(""),
			templateWorkDir: stringPtr("/tmpl/dir"),
			expandedWorkDir: stringPtr("/tmpl/dir"),
			expectedWorkDir: stringPtr(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandedSpec := &runnertypes.CommandSpec{}
			cmdSpec := &runnertypes.CommandSpec{WorkDir: tt.cmdWorkDir}
			template := &runnertypes.CommandTemplate{WorkDir: tt.templateWorkDir}

			ApplyTemplateInheritance(expandedSpec, cmdSpec, template, tt.expandedWorkDir, nil)

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
			cmdOutputFile:      stringPtr("/cmd/output.txt"),
			templateOutputFile: stringPtr("/tmpl/output.txt"),
			expandedOutputFile: stringPtr("/tmpl/output.txt"),
			expectedOutputFile: stringPtr("/cmd/output.txt"),
		},
		{
			name:               "command inherits from template",
			cmdOutputFile:      nil,
			templateOutputFile: stringPtr("/tmpl/output.txt"),
			expandedOutputFile: stringPtr("/tmpl/output.txt"),
			expectedOutputFile: stringPtr("/tmpl/output.txt"),
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
			cmdOutputFile:      stringPtr(""),
			templateOutputFile: stringPtr("/tmpl/output.txt"),
			expandedOutputFile: stringPtr("/tmpl/output.txt"),
			expectedOutputFile: stringPtr(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandedSpec := &runnertypes.CommandSpec{}
			cmdSpec := &runnertypes.CommandSpec{OutputFile: tt.cmdOutputFile}
			template := &runnertypes.CommandTemplate{OutputFile: tt.templateOutputFile}

			ApplyTemplateInheritance(expandedSpec, cmdSpec, template, nil, tt.expandedOutputFile)

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
			template := &runnertypes.CommandTemplate{EnvImport: tt.templateEnvImport}

			ApplyTemplateInheritance(expandedSpec, cmdSpec, template, nil, nil)

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
			template := &runnertypes.CommandTemplate{Vars: tt.templateVars}

			ApplyTemplateInheritance(expandedSpec, cmdSpec, template, nil, nil)

			assert.Equal(t, tt.expectedVars, expandedSpec.Vars)
		})
	}
}

func TestApplyTemplateInheritance_Combined(t *testing.T) {
	// Test all inheritance models together
	expandedSpec := &runnertypes.CommandSpec{}
	cmdSpec := &runnertypes.CommandSpec{
		WorkDir:    nil,                                    // Inherit from template
		OutputFile: stringPtr("/cmd/output.txt"),           // Override template
		EnvImport:  []string{"CMD_VAR", "SHARED"},          // Merge with template
		Vars:       map[string]any{"cmd_key": "cmd_value"}, // Merge with template
	}
	template := &runnertypes.CommandTemplate{
		WorkDir:    stringPtr("/tmpl/dir"),
		OutputFile: stringPtr("/tmpl/output.txt"),
		EnvImport:  []string{"TMPL_VAR", "SHARED"},
		Vars:       map[string]any{"tmpl_key": "tmpl_value", "cmd_key": "tmpl_override"},
	}

	ApplyTemplateInheritance(expandedSpec, cmdSpec, template, stringPtr("/tmpl/dir"), stringPtr("/tmpl/output.txt"))

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
