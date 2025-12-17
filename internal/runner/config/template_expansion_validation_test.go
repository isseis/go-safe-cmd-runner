//go:build test

// Package config provides configuration loading and validation for the command runner.
package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateTemplateVariableReferences tests validation of variable references in templates.
func TestValidateTemplateVariableReferences(t *testing.T) {
	tests := []struct {
		name         string
		template     runnertypes.CommandTemplate
		templateName string
		globalVars   map[string]string
		wantErr      bool
		errType      interface{}
	}{
		{
			name: "success: no variable references",
			template: runnertypes.CommandTemplate{
				Cmd:     "echo",
				Args:    []string{"hello", "world"},
				WorkDir: "/tmp",
				Env:     []string{"KEY=value"},
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      false,
		},
		{
			name: "success: global variables defined",
			template: runnertypes.CommandTemplate{
				Cmd:     "echo %{GREETING}",
				Args:    []string{"%{NAME}", "%{MESSAGE}"},
				WorkDir: "%{WORK_DIR}",
				Env:     []string{"KEY=%{VALUE}"},
			},
			templateName: "test",
			globalVars: map[string]string{
				"GREETING": "hello",
				"NAME":     "world",
				"MESSAGE":  "test",
				"WORK_DIR": "/tmp",
				"VALUE":    "42",
			},
			wantErr: false,
		},
		{
			name: "success: multiple global variables in different fields",
			template: runnertypes.CommandTemplate{
				Cmd:     "echo %{MSG}",
				Args:    []string{"%{ARG1}", "%{ARG2}"},
				WorkDir: "%{DIR}",
				Env:     []string{"KEY1=%{VAL1}", "KEY2=%{VAL2}"},
			},
			templateName: "test",
			globalVars: map[string]string{
				"MSG":  "hello",
				"ARG1": "arg1",
				"ARG2": "arg2",
				"DIR":  "/tmp",
				"VAL1": "value1",
				"VAL2": "value2",
			},
			wantErr: false,
		},
		{
			name: "error: local variable in cmd",
			template: runnertypes.CommandTemplate{
				Cmd: "echo %{local_var}",
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrLocalVariableInTemplate{},
		},
		{
			name: "error: local variable in args",
			template: runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"%{local_var}"},
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrLocalVariableInTemplate{},
		},
		{
			name: "error: local variable in workdir",
			template: runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: "%{local_dir}",
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrLocalVariableInTemplate{},
		},
		{
			name: "error: local variable in env",
			template: runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"KEY=%{local_value}"},
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrLocalVariableInTemplate{},
		},
		{
			name: "error: undefined global variable in cmd",
			template: runnertypes.CommandTemplate{
				Cmd: "echo %{UNDEFINED}",
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrUndefinedGlobalVariableInTemplate{},
		},
		{
			name: "error: undefined global variable in args",
			template: runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"%{UNDEFINED}"},
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrUndefinedGlobalVariableInTemplate{},
		},
		{
			name: "error: undefined global variable in workdir",
			template: runnertypes.CommandTemplate{
				Cmd:     "echo",
				WorkDir: "%{UNDEFINED_DIR}",
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrUndefinedGlobalVariableInTemplate{},
		},
		{
			name: "error: undefined global variable in env",
			template: runnertypes.CommandTemplate{
				Cmd: "echo",
				Env: []string{"KEY=%{UNDEFINED_VALUE}"},
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrUndefinedGlobalVariableInTemplate{},
		},
		{
			name: "error: mixed defined and undefined variables",
			template: runnertypes.CommandTemplate{
				Cmd:  "echo %{DEFINED}",
				Args: []string{"%{UNDEFINED}"},
			},
			templateName: "test",
			globalVars: map[string]string{
				"DEFINED": "value",
			},
			wantErr: true,
			errType: &ErrUndefinedGlobalVariableInTemplate{},
		},
		{
			name: "success: escaped percent sign",
			template: runnertypes.CommandTemplate{
				Cmd: "echo \\%{NOT_A_VAR}",
			},
			templateName: "test",
			globalVars:   map[string]string{},
			wantErr:      false,
		},
		{
			name: "success: multiple variables in same field",
			template: runnertypes.CommandTemplate{
				Cmd: "echo %{VAR1} %{VAR2} %{VAR3}",
			},
			templateName: "test",
			globalVars: map[string]string{
				"VAR1": "a",
				"VAR2": "b",
				"VAR3": "c",
			},
			wantErr: false,
		},
		{
			name: "error: one undefined among multiple variables",
			template: runnertypes.CommandTemplate{
				Cmd: "echo %{VAR1} %{UNDEFINED} %{VAR3}",
			},
			templateName: "test",
			globalVars: map[string]string{
				"VAR1": "a",
				"VAR3": "c",
			},
			wantErr: true,
			errType: &ErrUndefinedGlobalVariableInTemplate{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateVariableReferences(&tt.template, tt.templateName, tt.globalVars)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.IsType(t, tt.errType, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateStringFieldVariableReferences tests validation of variable references in a single string field.
func TestValidateStringFieldVariableReferences(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		templateName string
		fieldName    string
		globalVars   map[string]string
		wantErr      bool
		errType      interface{}
	}{
		{
			name:         "success: no variables",
			input:        "hello world",
			templateName: "test",
			fieldName:    "cmd",
			globalVars:   map[string]string{},
			wantErr:      false,
		},
		{
			name:         "success: global variable defined",
			input:        "echo %{GREETING}",
			templateName: "test",
			fieldName:    "cmd",
			globalVars: map[string]string{
				"GREETING": "hello",
			},
			wantErr: false,
		},
		{
			name:         "success: multiple global variables defined",
			input:        "%{VAR1} %{VAR2} %{VAR3}",
			templateName: "test",
			fieldName:    "cmd",
			globalVars: map[string]string{
				"VAR1": "a",
				"VAR2": "b",
				"VAR3": "c",
			},
			wantErr: false,
		},
		{
			name:         "error: local variable",
			input:        "echo %{local_var}",
			templateName: "test",
			fieldName:    "cmd",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrLocalVariableInTemplate{},
		},
		{
			name:         "error: undefined global variable",
			input:        "echo %{UNDEFINED}",
			templateName: "test",
			fieldName:    "cmd",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrUndefinedGlobalVariableInTemplate{},
		},
		{
			name:         "success: escaped percent",
			input:        "echo \\%{NOT_A_VAR}",
			templateName: "test",
			fieldName:    "cmd",
			globalVars:   map[string]string{},
			wantErr:      false,
		},
		{
			name:         "error: variable name with invalid characters",
			input:        "echo %{INVALID-NAME}",
			templateName: "test",
			fieldName:    "cmd",
			globalVars:   map[string]string{},
			wantErr:      true,
			errType:      &ErrInvalidVariableNameDetail{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStringFieldVariableReferences(tt.input, tt.templateName, tt.fieldName, tt.globalVars)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.IsType(t, tt.errType, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateAllTemplates tests validation of all templates in a configuration.
func TestValidateAllTemplates(t *testing.T) {
	tests := []struct {
		name       string
		templates  map[string]runnertypes.CommandTemplate
		globalVars map[string]string
		wantErr    bool
		errType    interface{}
	}{
		{
			name:       "success: no templates",
			templates:  map[string]runnertypes.CommandTemplate{},
			globalVars: map[string]string{},
			wantErr:    false,
		},
		{
			name: "success: all templates valid",
			templates: map[string]runnertypes.CommandTemplate{
				"template1": {
					Cmd:  "echo %{GREETING}",
					Args: []string{"%{NAME}"},
				},
				"template2": {
					Cmd:     "ls %{DIR}",
					WorkDir: "%{WORK_DIR}",
				},
			},
			globalVars: map[string]string{
				"GREETING": "hello",
				"NAME":     "world",
				"DIR":      "/tmp",
				"WORK_DIR": "/home",
			},
			wantErr: false,
		},
		{
			name: "error: one template has local variable",
			templates: map[string]runnertypes.CommandTemplate{
				"template1": {
					Cmd: "echo %{GREETING}",
				},
				"template2": {
					Cmd: "echo %{local_var}",
				},
			},
			globalVars: map[string]string{
				"GREETING": "hello",
			},
			wantErr: true,
			errType: &ErrLocalVariableInTemplate{},
		},
		{
			name: "error: one template has undefined global variable",
			templates: map[string]runnertypes.CommandTemplate{
				"template1": {
					Cmd: "echo %{GREETING}",
				},
				"template2": {
					Cmd: "echo %{UNDEFINED}",
				},
			},
			globalVars: map[string]string{
				"GREETING": "hello",
			},
			wantErr: true,
			errType: &ErrUndefinedGlobalVariableInTemplate{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAllTemplates(tt.templates, tt.globalVars)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.IsType(t, tt.errType, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestErrorMessages tests that error messages contain expected information.
func TestErrorMessages(t *testing.T) {
	t.Run("ErrLocalVariableInTemplate", func(t *testing.T) {
		err := &ErrLocalVariableInTemplate{
			TemplateName: "test_template",
			Field:        "cmd",
			VariableName: "local_var",
		}
		msg := err.Error()
		assert.Contains(t, msg, "test_template")
		assert.Contains(t, msg, "cmd")
		assert.Contains(t, msg, "local_var")
	})

	t.Run("ErrUndefinedGlobalVariableInTemplate", func(t *testing.T) {
		err := &ErrUndefinedGlobalVariableInTemplate{
			TemplateName: "test_template",
			Field:        "args[0]",
			VariableName: "UNDEFINED_VAR",
		}
		msg := err.Error()
		assert.Contains(t, msg, "test_template")
		assert.Contains(t, msg, "args[0]")
		assert.Contains(t, msg, "UNDEFINED_VAR")
	})
}
