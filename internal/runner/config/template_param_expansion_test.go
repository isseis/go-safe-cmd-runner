//go:build test

package config

import (
	"errors"
	"reflect"
	"testing"
)

// Phase 3: Parameter expansion tests

func TestExpandSingleArg(t *testing.T) {
	tests := []struct {
		name         string
		arg          string
		params       map[string]interface{}
		templateName string
		field        string
		expected     []string
		wantErr      bool
		errType      interface{}
	}{
		// Required parameter tests
		{
			name:         "required param - success",
			arg:          "${path}",
			params:       map[string]interface{}{"path": "/backup/data"},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"/backup/data"},
		},
		{
			name:         "required param - missing",
			arg:          "${path}",
			params:       map[string]interface{}{},
			templateName: "test",
			field:        "args[0]",
			wantErr:      true,
			errType:      &ErrRequiredParamMissing{},
		},
		{
			name:         "required param - wrong type",
			arg:          "${path}",
			params:       map[string]interface{}{"path": []interface{}{"a", "b"}},
			templateName: "test",
			field:        "args[0]",
			wantErr:      true,
			errType:      &ErrTemplateTypeMismatch{},
		},
		// Optional parameter tests
		{
			name:         "optional param - provided",
			arg:          "${?verbose}",
			params:       map[string]interface{}{"verbose": "-v"},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"-v"},
		},
		{
			name:         "optional param - missing",
			arg:          "${?verbose}",
			params:       map[string]interface{}{},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{},
		},
		{
			name:         "optional param - empty string",
			arg:          "${?verbose}",
			params:       map[string]interface{}{"verbose": ""},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{},
		},
		// Array parameter tests
		{
			name:         "array param - provided",
			arg:          "${@flags}",
			params:       map[string]interface{}{"flags": []interface{}{"-v", "--quiet"}},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"-v", "--quiet"},
		},
		{
			name:         "array param - missing",
			arg:          "${@flags}",
			params:       map[string]interface{}{},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{},
		},
		{
			name:         "array param - empty array",
			arg:          "${@flags}",
			params:       map[string]interface{}{"flags": []interface{}{}},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{},
		},
		{
			name:         "array param - []string type",
			arg:          "${@flags}",
			params:       map[string]interface{}{"flags": []string{"-a", "-b"}},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"-a", "-b"},
		},
		{
			name:         "array param - wrong type (string)",
			arg:          "${@flags}",
			params:       map[string]interface{}{"flags": "not-an-array"},
			templateName: "test",
			field:        "args[0]",
			wantErr:      true,
			errType:      &ErrTemplateTypeMismatch{},
		},
		{
			name:         "array param - non-string element",
			arg:          "${@flags}",
			params:       map[string]interface{}{"flags": []interface{}{"-v", 123}},
			templateName: "test",
			field:        "args[0]",
			wantErr:      true,
			errType:      &ErrTemplateInvalidArrayElement{},
		},
		{
			name:         "array param - in mixed context",
			arg:          "prefix${@flags}",
			params:       map[string]interface{}{"flags": []interface{}{"-v"}},
			templateName: "test",
			field:        "args[0]",
			wantErr:      true,
			errType:      &ErrArrayInMixedContext{},
		},
		// String replacement tests
		{
			name:         "string with multiple placeholders",
			arg:          "${prefix}/${path}",
			params:       map[string]interface{}{"prefix": "/backup", "path": "data"},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"/backup/data"},
		},
		{
			name:         "optional in mixed context - provided",
			arg:          "--flag=${?value}",
			params:       map[string]interface{}{"value": "test"},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"--flag=test"},
		},
		{
			name:         "optional in mixed context - missing",
			arg:          "--flag=${?value}",
			params:       map[string]interface{}{},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"--flag="},
		},
		// No placeholders
		{
			name:         "no placeholders",
			arg:          "backup",
			params:       map[string]interface{}{},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"backup"},
		},
		// Escape sequences
		{
			name:         "escaped dollar",
			arg:          "\\$100",
			params:       map[string]interface{}{},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"$100"},
		},
		{
			name:         "escaped dollar with placeholder",
			arg:          "\\$${value}",
			params:       map[string]interface{}{"value": "100"},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"$100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandSingleArg(tt.arg, tt.params, tt.templateName, tt.field)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				// Check error type
				switch tt.errType.(type) {
				case *ErrRequiredParamMissing:
					var target *ErrRequiredParamMissing
					if !errors.As(err, &target) {
						t.Errorf("expected ErrRequiredParamMissing, got %T: %v", err, err)
					}
				case *ErrTemplateTypeMismatch:
					var target *ErrTemplateTypeMismatch
					if !errors.As(err, &target) {
						t.Errorf("expected ErrTemplateTypeMismatch, got %T: %v", err, err)
					}
				case *ErrTemplateInvalidArrayElement:
					var target *ErrTemplateInvalidArrayElement
					if !errors.As(err, &target) {
						t.Errorf("expected ErrTemplateInvalidArrayElement, got %T: %v", err, err)
					}
				case *ErrArrayInMixedContext:
					var target *ErrArrayInMixedContext
					if !errors.As(err, &target) {
						t.Errorf("expected ErrArrayInMixedContext, got %T: %v", err, err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExpandTemplateArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		params       map[string]interface{}
		templateName string
		expected     []string
		wantErr      bool
	}{
		{
			name:         "simple args",
			args:         []string{"backup", "${path}"},
			params:       map[string]interface{}{"path": "/data"},
			templateName: "test",
			expected:     []string{"backup", "/data"},
		},
		{
			name:         "array expansion",
			args:         []string{"${@flags}", "backup", "${path}"},
			params:       map[string]interface{}{"flags": []interface{}{"-v", "-q"}, "path": "/data"},
			templateName: "test",
			expected:     []string{"-v", "-q", "backup", "/data"},
		},
		{
			name:         "array expansion removes element when empty",
			args:         []string{"${@flags}", "backup"},
			params:       map[string]interface{}{},
			templateName: "test",
			expected:     []string{"backup"},
		},
		{
			name:         "optional removes element",
			args:         []string{"${?verbose}", "backup"},
			params:       map[string]interface{}{},
			templateName: "test",
			expected:     []string{"backup"},
		},
		{
			name:         "empty args",
			args:         []string{},
			params:       map[string]interface{}{},
			templateName: "test",
			expected:     nil,
		},
		{
			name:         "complex example",
			args:         []string{"${@verbose_flags}", "backup", "${path}", "${?exclude}"},
			params:       map[string]interface{}{"verbose_flags": []interface{}{"-v"}, "path": "/backup/data"},
			templateName: "restic_backup",
			expected:     []string{"-v", "backup", "/backup/data"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandTemplateArgs(tt.args, tt.params, tt.templateName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExpandArrayPlaceholder(t *testing.T) {
	tests := []struct {
		name         string
		paramName    string
		params       map[string]interface{}
		templateName string
		field        string
		expected     []string
		wantErr      bool
		errType      interface{}
	}{
		{
			name:         "[]interface{} with strings",
			paramName:    "flags",
			params:       map[string]interface{}{"flags": []interface{}{"-v", "--quiet"}},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"-v", "--quiet"},
		},
		{
			name:         "[]string type",
			paramName:    "flags",
			params:       map[string]interface{}{"flags": []string{"-a", "-b", "-c"}},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"-a", "-b", "-c"},
		},
		{
			name:         "missing param",
			paramName:    "flags",
			params:       map[string]interface{}{},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{},
		},
		{
			name:         "string instead of array",
			paramName:    "flags",
			params:       map[string]interface{}{"flags": "single"},
			templateName: "test",
			field:        "args[0]",
			wantErr:      true,
			errType:      &ErrTemplateTypeMismatch{},
		},
		{
			name:         "unsupported type",
			paramName:    "flags",
			params:       map[string]interface{}{"flags": 123},
			templateName: "test",
			field:        "args[0]",
			wantErr:      true,
			errType:      &ErrUnsupportedParamType{},
		},
		{
			name:         "non-string element in array",
			paramName:    "flags",
			params:       map[string]interface{}{"flags": []interface{}{"ok", 42, "also-ok"}},
			templateName: "test",
			field:        "args[0]",
			wantErr:      true,
			errType:      &ErrTemplateInvalidArrayElement{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandArrayPlaceholder(tt.paramName, tt.params, tt.templateName, tt.field)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExpandOptionalPlaceholder(t *testing.T) {
	tests := []struct {
		name         string
		paramName    string
		params       map[string]interface{}
		templateName string
		field        string
		expected     []string
		wantErr      bool
	}{
		{
			name:         "value provided",
			paramName:    "verbose",
			params:       map[string]interface{}{"verbose": "-v"},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{"-v"},
		},
		{
			name:         "empty string",
			paramName:    "verbose",
			params:       map[string]interface{}{"verbose": ""},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{},
		},
		{
			name:         "missing param",
			paramName:    "verbose",
			params:       map[string]interface{}{},
			templateName: "test",
			field:        "args[0]",
			expected:     []string{},
		},
		{
			name:         "wrong type",
			paramName:    "verbose",
			params:       map[string]interface{}{"verbose": []interface{}{"-v"}},
			templateName: "test",
			field:        "args[0]",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandOptionalPlaceholder(tt.paramName, tt.params, tt.templateName, tt.field)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
