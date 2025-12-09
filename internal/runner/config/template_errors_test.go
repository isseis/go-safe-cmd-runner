//go:build test

package config

import (
	"errors"
	"testing"
)

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		// Template-related errors
		{
			name: "template not found",
			err: &ErrTemplateNotFound{
				CommandName:  "backup",
				TemplateName: "missing",
			},
			expected: `template "missing" not found (referenced by command "backup")`,
		},
		{
			name: "template field conflict",
			err: &ErrTemplateFieldConflict{
				GroupName:    "daily",
				CommandIndex: 0,
				TemplateName: "restic_backup",
				Field:        "cmd",
			},
			expected: `group[daily] command[0]: cannot specify both "template" and "cmd" fields in command definition`,
		},
		{
			name: "duplicate template name",
			err: &ErrDuplicateTemplateName{
				Name: "restic_backup",
			},
			expected: `duplicate template name "restic_backup"`,
		},
		{
			name: "invalid template name",
			err: &ErrInvalidTemplateName{
				Name:   "123invalid",
				Reason: "must start with a letter or underscore",
			},
			expected: `invalid template name "123invalid": must start with a letter or underscore`,
		},
		{
			name: "reserved template name",
			err: &ErrReservedTemplateName{
				Name: "__reserved",
			},
			expected: `template name "__reserved" uses reserved prefix "__"`,
		},
		{
			name: "template contains name field",
			err: &ErrTemplateContainsNameField{
				TemplateName: "bad_template",
			},
			expected: `template definition "bad_template" cannot contain "name" field`,
		},
		{
			name: "missing required field (template)",
			err: &ErrMissingRequiredField{
				TemplateName: "incomplete",
				Field:        "cmd",
			},
			expected: `template "incomplete": required field "cmd" is missing`,
		},
		{
			name: "missing required field (command)",
			err: &ErrMissingRequiredField{
				GroupName:    "daily",
				CommandIndex: 0,
				Field:        "cmd",
			},
			expected: `group[daily] command[0]: required field "cmd" is missing`,
		},
		// Parameter-related errors
		{
			name: "required param missing",
			err: &ErrRequiredParamMissing{
				TemplateName: "restic_backup",
				Field:        "args[0]",
				ParamName:    "path",
			},
			expected: `template "restic_backup" args[0]: required parameter "path" not provided`,
		},
		{
			name: "type mismatch",
			err: &ErrTemplateTypeMismatch{
				TemplateName: "restic_backup",
				Field:        "args[0]",
				ParamName:    "path",
				Expected:     "string",
				Actual:       "[]interface {}",
			},
			expected: `template "restic_backup" args[0]: parameter "path" expected string, got []interface {}`,
		},
		{
			name: "forbidden pattern in template",
			err: &ErrForbiddenPatternInTemplate{
				TemplateName: "dangerous",
				Field:        "cmd",
				Value:        "%{root}/bin/echo",
			},
			expected: `template "dangerous" contains forbidden pattern "%{" in cmd: variable references are not allowed in template definitions for security reasons (see NF-006)`,
		},
		{
			name: "array in mixed context",
			err: &ErrArrayInMixedContext{
				TemplateName: "bad_template",
				Field:        "args[0]",
				ParamName:    "flags",
			},
			expected: `template "bad_template" args[0]: array parameter ${@flags} cannot be used in mixed context`,
		},
		{
			name: "invalid array element",
			err: &ErrTemplateInvalidArrayElement{
				TemplateName: "bad_template",
				Field:        "params",
				ParamName:    "flags",
				Index:        2,
				ActualType:   "int",
			},
			expected: `template "bad_template" params: array parameter "flags" contains non-string element at index 2 (type: int)`,
		},
		{
			name: "unsupported param type",
			err: &ErrUnsupportedParamType{
				TemplateName: "bad_template",
				Field:        "params",
				ParamName:    "count",
				ActualType:   "int",
			},
			expected: `template "bad_template" params: parameter "count" has unsupported type int (expected string or []string)`,
		},
		{
			name: "invalid param name",
			err: &ErrInvalidParamName{
				TemplateName: "test_template",
				ParamName:    "123invalid",
				Reason:       "must start with a letter or underscore",
			},
			expected: `template "test_template": invalid parameter name "123invalid": must start with a letter or underscore`,
		},
		{
			name: "empty placeholder name",
			err: &ErrEmptyPlaceholderName{
				Input:    "${?}",
				Position: 0,
			},
			expected: `empty placeholder name at position 0 in "${?}"`,
		},
		{
			name: "multiple values in string context",
			err: &ErrMultipleValuesInStringContext{
				TemplateName: "bad_template",
				Field:        "cmd",
			},
			expected: `template "bad_template" cmd: array expansion produced multiple values in string context`,
		},
		// Placeholder parsing errors
		{
			name: "unclosed placeholder",
			err: &ErrUnclosedPlaceholder{
				Input:    "${path",
				Position: 0,
			},
			expected: `unclosed placeholder at position 0 in "${path"`,
		},
		{
			name: "empty placeholder",
			err: &ErrEmptyPlaceholder{
				Input:    "${}",
				Position: 0,
			},
			expected: `empty placeholder at position 0 in "${}"`,
		},
		{
			name: "invalid placeholder name",
			err: &ErrInvalidPlaceholderName{
				Input:    "${123invalid}",
				Position: 0,
				Name:     "123invalid",
				Reason:   "must start with a letter or underscore",
			},
			expected: `invalid placeholder name "123invalid" at position 0 in "${123invalid}": must start with a letter or underscore`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestErrorTypesImplementError(t *testing.T) {
	// Verify all error types implement the error interface by checking
	// that their Error() method returns non-empty strings
	tests := []struct {
		name string
		err  error
	}{
		{"ErrTemplateNotFound", &ErrTemplateNotFound{CommandName: "cmd", TemplateName: "tmpl"}},
		{"ErrTemplateFieldConflict", &ErrTemplateFieldConflict{GroupName: "grp", Field: "cmd"}},
		{"ErrDuplicateTemplateName", &ErrDuplicateTemplateName{Name: "tmpl"}},
		{"ErrInvalidTemplateName", &ErrInvalidTemplateName{Name: "tmpl", Reason: "reason"}},
		{"ErrReservedTemplateName", &ErrReservedTemplateName{Name: "__tmpl"}},
		{"ErrTemplateContainsNameField", &ErrTemplateContainsNameField{TemplateName: "tmpl"}},
		{"ErrMissingRequiredField", &ErrMissingRequiredField{TemplateName: "tmpl", Field: "cmd"}},
		{"ErrRequiredParamMissing", &ErrRequiredParamMissing{TemplateName: "tmpl", ParamName: "p"}},
		{"ErrTemplateTypeMismatch", &ErrTemplateTypeMismatch{TemplateName: "tmpl", ParamName: "p"}},
		{"ErrForbiddenPatternInTemplate", &ErrForbiddenPatternInTemplate{TemplateName: "tmpl", Field: "cmd"}},
		{"ErrArrayInMixedContext", &ErrArrayInMixedContext{TemplateName: "tmpl", ParamName: "p"}},
		{"ErrTemplateInvalidArrayElement", &ErrTemplateInvalidArrayElement{TemplateName: "tmpl", ParamName: "p"}},
		{"ErrUnsupportedParamType", &ErrUnsupportedParamType{TemplateName: "tmpl", ParamName: "p"}},
		{"ErrInvalidParamName", &ErrInvalidParamName{TemplateName: "tmpl", ParamName: "p"}},
		{"ErrEmptyPlaceholderName", &ErrEmptyPlaceholderName{Input: "${?}"}},
		{"ErrMultipleValuesInStringContext", &ErrMultipleValuesInStringContext{TemplateName: "tmpl"}},
		{"ErrUnclosedPlaceholder", &ErrUnclosedPlaceholder{Input: "${path"}},
		{"ErrEmptyPlaceholder", &ErrEmptyPlaceholder{Input: "${}"}},
		{"ErrInvalidPlaceholderName", &ErrInvalidPlaceholderName{Input: "${123}", Name: "123"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if msg := tt.err.Error(); msg == "" {
				t.Errorf("%s.Error() returned empty string", tt.name)
			}
		})
	}
}

func TestErrorsAs(t *testing.T) {
	// Test that errors.As works correctly with our error types
	tests := []struct {
		name   string
		err    error
		target interface{}
	}{
		{
			name:   "ErrTemplateNotFound",
			err:    &ErrTemplateNotFound{CommandName: "cmd", TemplateName: "tmpl"},
			target: &ErrTemplateNotFound{},
		},
		{
			name:   "ErrTemplateFieldConflict",
			err:    &ErrTemplateFieldConflict{GroupName: "grp", Field: "cmd"},
			target: &ErrTemplateFieldConflict{},
		},
		{
			name:   "ErrUnclosedPlaceholder",
			err:    &ErrUnclosedPlaceholder{Input: "${path", Position: 0},
			target: &ErrUnclosedPlaceholder{},
		},
		{
			name:   "ErrRequiredParamMissing",
			err:    &ErrRequiredParamMissing{TemplateName: "tmpl", ParamName: "p"},
			target: &ErrRequiredParamMissing{},
		},
		{
			name:   "ErrForbiddenPatternInTemplate",
			err:    &ErrForbiddenPatternInTemplate{TemplateName: "tmpl", Field: "cmd"},
			target: &ErrForbiddenPatternInTemplate{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use type switch to test errors.As behavior
			switch target := tt.target.(type) {
			case *ErrTemplateNotFound:
				if !errors.As(tt.err, &target) {
					t.Errorf("errors.As failed for %s", tt.name)
				}
			case *ErrTemplateFieldConflict:
				if !errors.As(tt.err, &target) {
					t.Errorf("errors.As failed for %s", tt.name)
				}
			case *ErrUnclosedPlaceholder:
				if !errors.As(tt.err, &target) {
					t.Errorf("errors.As failed for %s", tt.name)
				}
			case *ErrRequiredParamMissing:
				if !errors.As(tt.err, &target) {
					t.Errorf("errors.As failed for %s", tt.name)
				}
			case *ErrForbiddenPatternInTemplate:
				if !errors.As(tt.err, &target) {
					t.Errorf("errors.As failed for %s", tt.name)
				}
			}
		})
	}
}
