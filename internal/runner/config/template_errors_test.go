//go:build test

package config

import (
	"errors"
	"testing"
)

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
		{"ErrPlaceholderInEnvKey", &ErrPlaceholderInEnvKey{TemplateName: "tmpl", EnvEntry: "${key}=val", Key: "${key}"}},
		{"ErrTemplateInvalidEnvFormat", &ErrTemplateInvalidEnvFormat{TemplateName: "tmpl", Field: "env[0]", Entry: "INVALID"}},
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
		{
			name:   "ErrPlaceholderInEnvKey",
			err:    &ErrPlaceholderInEnvKey{TemplateName: "tmpl", EnvEntry: "${key}=val", Key: "${key}"},
			target: &ErrPlaceholderInEnvKey{},
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
			case *ErrPlaceholderInEnvKey:
				if !errors.As(tt.err, &target) {
					t.Errorf("errors.As failed for %s", tt.name)
				}
			}
		})
	}
}
