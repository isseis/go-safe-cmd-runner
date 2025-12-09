//go:build test

package config

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// TestValidateTemplateName tests template name validation
func TestValidateTemplateName(t *testing.T) {
	tests := []struct {
		name     string
		tmplName string
		wantErr  bool
		errType  error
	}{
		{name: "valid name", tmplName: "restic_backup", wantErr: false},
		{name: "valid with number", tmplName: "backup_v2", wantErr: false},
		{name: "valid single underscore", tmplName: "_valid", wantErr: false},
		{name: "valid uppercase", tmplName: "MyTemplate", wantErr: false},
		{name: "invalid start with number", tmplName: "123invalid", wantErr: true, errType: &ErrInvalidTemplateName{}},
		{name: "reserved prefix double underscore", tmplName: "__reserved", wantErr: true, errType: &ErrReservedTemplateName{}},
		{name: "reserved prefix __internal", tmplName: "__internal_template", wantErr: true, errType: &ErrReservedTemplateName{}},
		{name: "empty name", tmplName: "", wantErr: true, errType: &ErrInvalidTemplateName{}},
		{name: "contains dash", tmplName: "my-template", wantErr: true, errType: &ErrInvalidTemplateName{}},
		{name: "contains space", tmplName: "my template", wantErr: true, errType: &ErrInvalidTemplateName{}},
		{name: "contains dot", tmplName: "my.template", wantErr: true, errType: &ErrInvalidTemplateName{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateName(tt.tmplName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errType != nil {
					// Check error type using errors.As
					switch tt.errType.(type) {
					case *ErrInvalidTemplateName:
						var target *ErrInvalidTemplateName
						if !errors.As(err, &target) {
							t.Errorf("expected ErrInvalidTemplateName, got %T: %v", err, err)
						}
					case *ErrReservedTemplateName:
						var target *ErrReservedTemplateName
						if !errors.As(err, &target) {
							t.Errorf("expected ErrReservedTemplateName, got %T: %v", err, err)
						}
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestValidateTemplateDefinition tests template definition validation (NF-006 enforcement)
func TestValidateTemplateDefinition(t *testing.T) {
	tests := []struct {
		name     string
		tmplName string
		template runnertypes.CommandTemplate
		wantErr  bool
		errType  error
	}{
		{
			name:     "valid template",
			tmplName: "restic_backup",
			template: runnertypes.CommandTemplate{Cmd: "restic", Args: []string{"backup", "${path}"}},
			wantErr:  false,
		},
		{
			name:     "valid template with env",
			tmplName: "with_env",
			template: runnertypes.CommandTemplate{Cmd: "cmd", Env: []string{"KEY=${value}"}},
			wantErr:  false,
		},
		{
			name:     "valid template with workdir",
			tmplName: "with_workdir",
			template: runnertypes.CommandTemplate{Cmd: "cmd", WorkDir: "/tmp/${path}"},
			wantErr:  false,
		},
		{
			name:     "forbidden %{ in cmd",
			tmplName: "bad_template",
			template: runnertypes.CommandTemplate{Cmd: "%{root}/bin/restic"},
			wantErr:  true,
			errType:  &ErrForbiddenPatternInTemplate{},
		},
		{
			name:     "forbidden %{ in args",
			tmplName: "bad_template",
			template: runnertypes.CommandTemplate{Cmd: "restic", Args: []string{"%{group_root}/data"}},
			wantErr:  true,
			errType:  &ErrForbiddenPatternInTemplate{},
		},
		{
			name:     "forbidden %{ in env",
			tmplName: "bad_template",
			template: runnertypes.CommandTemplate{Cmd: "cmd", Env: []string{"KEY=%{secret}"}},
			wantErr:  true,
			errType:  &ErrForbiddenPatternInTemplate{},
		},
		{
			name:     "forbidden %{ in workdir",
			tmplName: "bad_template",
			template: runnertypes.CommandTemplate{Cmd: "cmd", WorkDir: "%{base_dir}/work"},
			wantErr:  true,
			errType:  &ErrForbiddenPatternInTemplate{},
		},
		{
			name:     "missing cmd",
			tmplName: "incomplete",
			template: runnertypes.CommandTemplate{Args: []string{"backup"}},
			wantErr:  true,
			errType:  &ErrMissingRequiredField{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateDefinition(tt.tmplName, &tt.template)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errType != nil {
					switch tt.errType.(type) {
					case *ErrForbiddenPatternInTemplate:
						var target *ErrForbiddenPatternInTemplate
						if !errors.As(err, &target) {
							t.Errorf("expected ErrForbiddenPatternInTemplate, got %T: %v", err, err)
						}
					case *ErrMissingRequiredField:
						var target *ErrMissingRequiredField
						if !errors.As(err, &target) {
							t.Errorf("expected ErrMissingRequiredField, got %T: %v", err, err)
						}
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestValidateParams tests parameter validation
func TestValidateParams(t *testing.T) {
	tests := []struct {
		name         string
		params       map[string]interface{}
		templateName string
		wantErr      bool
		errType      error
	}{
		{
			name:         "valid string param",
			params:       map[string]interface{}{"path": "/data"},
			templateName: "test",
			wantErr:      false,
		},
		{
			name:         "valid string array param",
			params:       map[string]interface{}{"flags": []string{"-v", "-q"}},
			templateName: "test",
			wantErr:      false,
		},
		{
			name:         "valid interface array param with strings",
			params:       map[string]interface{}{"flags": []interface{}{"-v", "-q"}},
			templateName: "test",
			wantErr:      false,
		},
		{
			name:         "variable reference allowed in params (NF-006)",
			params:       map[string]interface{}{"path": "%{group_root}/data"},
			templateName: "test",
			wantErr:      false, // %{} is allowed in params
		},
		{
			name:         "multiple params",
			params:       map[string]interface{}{"path": "/data", "flags": []string{"-v"}},
			templateName: "test",
			wantErr:      false,
		},
		{
			name:         "empty params",
			params:       map[string]interface{}{},
			templateName: "test",
			wantErr:      false,
		},
		{
			name:         "nil params",
			params:       nil,
			templateName: "test",
			wantErr:      false,
		},
		{
			name:         "invalid param name starting with number",
			params:       map[string]interface{}{"123invalid": "value"},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrInvalidParamName{},
		},
		{
			name:         "invalid param name with dash",
			params:       map[string]interface{}{"my-param": "value"},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrInvalidParamName{},
		},
		{
			name:         "unsupported type int",
			params:       map[string]interface{}{"number": 123},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrUnsupportedParamType{},
		},
		{
			name:         "unsupported type float",
			params:       map[string]interface{}{"number": 3.14},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrUnsupportedParamType{},
		},
		{
			name:         "unsupported type bool",
			params:       map[string]interface{}{"flag": true},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrUnsupportedParamType{},
		},
		{
			name:         "array with non-string element",
			params:       map[string]interface{}{"mixed": []interface{}{"-v", 123}},
			templateName: "test",
			wantErr:      true,
			errType:      &ErrTemplateInvalidArrayElement{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParams(tt.params, tt.templateName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errType != nil {
					switch tt.errType.(type) {
					case *ErrInvalidParamName:
						var target *ErrInvalidParamName
						if !errors.As(err, &target) {
							t.Errorf("expected ErrInvalidParamName, got %T: %v", err, err)
						}
					case *ErrUnsupportedParamType:
						var target *ErrUnsupportedParamType
						if !errors.As(err, &target) {
							t.Errorf("expected ErrUnsupportedParamType, got %T: %v", err, err)
						}
					case *ErrTemplateInvalidArrayElement:
						var target *ErrTemplateInvalidArrayElement
						if !errors.As(err, &target) {
							t.Errorf("expected ErrTemplateInvalidArrayElement, got %T: %v", err, err)
						}
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestValidateCommandSpecExclusivity tests mutual exclusivity validation
func TestValidateCommandSpecExclusivity(t *testing.T) {
	tests := []struct {
		name    string
		spec    runnertypes.CommandSpec
		wantErr bool
		errType error
	}{
		{
			name:    "template only (valid)",
			spec:    runnertypes.CommandSpec{Name: "backup", Template: "restic_backup", Params: map[string]interface{}{"path": "/data"}},
			wantErr: false,
		},
		{
			name:    "cmd only (valid)",
			spec:    runnertypes.CommandSpec{Name: "backup", Cmd: "restic", Args: []string{"backup"}},
			wantErr: false,
		},
		{
			name:    "template + cmd (invalid)",
			spec:    runnertypes.CommandSpec{Name: "backup", Template: "restic_backup", Cmd: "restic"},
			wantErr: true,
			errType: &ErrTemplateFieldConflict{},
		},
		{
			name:    "template + args (invalid)",
			spec:    runnertypes.CommandSpec{Name: "backup", Template: "restic_backup", Args: []string{"backup"}},
			wantErr: true,
			errType: &ErrTemplateFieldConflict{},
		},
		{
			name:    "template + env_vars (invalid)",
			spec:    runnertypes.CommandSpec{Name: "backup", Template: "restic_backup", EnvVars: []string{"KEY=VALUE"}},
			wantErr: true,
			errType: &ErrTemplateFieldConflict{},
		},
		{
			name:    "template + workdir (invalid)",
			spec:    runnertypes.CommandSpec{Name: "backup", Template: "restic_backup", WorkDir: "/tmp"},
			wantErr: true,
			errType: &ErrTemplateFieldConflict{},
		},
		{
			name:    "no template and no cmd (invalid)",
			spec:    runnertypes.CommandSpec{Name: "backup"},
			wantErr: true,
			errType: &ErrMissingRequiredField{},
		},
		{
			name:    "template with params only (valid)",
			spec:    runnertypes.CommandSpec{Name: "test", Template: "tmpl", Params: map[string]interface{}{"key": "value"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommandSpecExclusivity("test_group", 0, &tt.spec)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errType != nil {
					switch tt.errType.(type) {
					case *ErrTemplateFieldConflict:
						var target *ErrTemplateFieldConflict
						if !errors.As(err, &target) {
							t.Errorf("expected ErrTemplateFieldConflict, got %T: %v", err, err)
						}
					case *ErrMissingRequiredField:
						var target *ErrMissingRequiredField
						if !errors.As(err, &target) {
							t.Errorf("expected ErrMissingRequiredField, got %T: %v", err, err)
						}
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestCollectUsedParams tests parameter collection from templates
func TestCollectUsedParams(t *testing.T) {
	tests := []struct {
		name     string
		template runnertypes.CommandTemplate
		expected map[string]struct{}
		wantErr  bool
	}{
		{
			name: "multiple params from different fields",
			template: runnertypes.CommandTemplate{
				Cmd:  "restic",
				Args: []string{"${@flags}", "backup", "${path}"},
				Env:  []string{"RESTIC_REPO=${repo}"},
			},
			expected: map[string]struct{}{
				"flags": {},
				"path":  {},
				"repo":  {},
			},
		},
		{
			name: "duplicate params",
			template: runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"${msg}", "${msg}"},
			},
			expected: map[string]struct{}{
				"msg": {},
			},
		},
		{
			name: "param in cmd",
			template: runnertypes.CommandTemplate{
				Cmd: "/usr/bin/${cmd_name}",
			},
			expected: map[string]struct{}{
				"cmd_name": {},
			},
		},
		{
			name: "param in workdir",
			template: runnertypes.CommandTemplate{
				Cmd:     "cmd",
				WorkDir: "${base_dir}/work",
			},
			expected: map[string]struct{}{
				"base_dir": {},
			},
		},
		{
			name: "optional and array params",
			template: runnertypes.CommandTemplate{
				Cmd:  "cmd",
				Args: []string{"${@flags}", "${?verbose}", "${path}"},
			},
			expected: map[string]struct{}{
				"flags":   {},
				"verbose": {},
				"path":    {},
			},
		},
		{
			name: "no params",
			template: runnertypes.CommandTemplate{
				Cmd:  "echo",
				Args: []string{"hello", "world"},
			},
			expected: map[string]struct{}{},
		},
		{
			name: "env value only (key not extracted)",
			template: runnertypes.CommandTemplate{
				Cmd: "cmd",
				Env: []string{"KEY=${value}", "OTHER=static"},
			},
			expected: map[string]struct{}{
				"value": {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CollectUsedParams(&tt.template)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d params, got %d", len(tt.expected), len(result))
			}

			for name := range tt.expected {
				if _, ok := result[name]; !ok {
					t.Errorf("expected param %q not found", name)
				}
			}

			for name := range result {
				if _, ok := tt.expected[name]; !ok {
					t.Errorf("unexpected param %q found", name)
				}
			}
		})
	}
}
