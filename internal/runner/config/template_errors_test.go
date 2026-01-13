//go:build test

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		{"ErrDuplicateEnvVariableDetail", &ErrDuplicateEnvVariableDetail{TemplateName: "tmpl", Field: "env", EnvKey: "PATH"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.err.Error(), "%s.Error() returned empty string", tt.name)
		})
	}
}

func TestErrorsAs(t *testing.T) {
	// Test that errors.As works correctly with our error types
	tests := []struct {
		name   string
		err    error
		assert func(t *testing.T, err error)
	}{
		{
			name: "ErrTemplateNotFound",
			err:  &ErrTemplateNotFound{CommandName: "cmd", TemplateName: "tmpl"},
			assert: func(t *testing.T, err error) {
				var target *ErrTemplateNotFound
				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name: "ErrTemplateFieldConflict",
			err:  &ErrTemplateFieldConflict{GroupName: "grp", Field: "cmd"},
			assert: func(t *testing.T, err error) {
				var target *ErrTemplateFieldConflict
				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name: "ErrUnclosedPlaceholder",
			err:  &ErrUnclosedPlaceholder{Input: "${path", Position: 0},
			assert: func(t *testing.T, err error) {
				var target *ErrUnclosedPlaceholder
				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name: "ErrRequiredParamMissing",
			err:  &ErrRequiredParamMissing{TemplateName: "tmpl", ParamName: "p"},
			assert: func(t *testing.T, err error) {
				var target *ErrRequiredParamMissing
				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name: "ErrForbiddenPatternInTemplate",
			err:  &ErrForbiddenPatternInTemplate{TemplateName: "tmpl", Field: "cmd"},
			assert: func(t *testing.T, err error) {
				var target *ErrForbiddenPatternInTemplate
				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name: "ErrPlaceholderInEnvKey",
			err:  &ErrPlaceholderInEnvKey{TemplateName: "tmpl", EnvEntry: "${key}=val", Key: "${key}"},
			assert: func(t *testing.T, err error) {
				var target *ErrPlaceholderInEnvKey
				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name: "ErrDuplicateEnvVariableDetail",
			err:  &ErrDuplicateEnvVariableDetail{TemplateName: "tmpl", Field: "env", EnvKey: "PATH"},
			assert: func(t *testing.T, err error) {
				var target *ErrDuplicateEnvVariableDetail
				assert.ErrorAs(t, err, &target)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.assert(t, tt.err)
		})
	}
}

func TestErrDuplicateTemplateName_ErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      *ErrDuplicateTemplateName
		expected string
	}{
		{
			name: "single location",
			err: &ErrDuplicateTemplateName{
				Name:      "backup",
				Locations: []string{"/tmp/template1.toml"},
			},
			expected: `duplicate template name "backup"`,
		},
		{
			name: "no locations",
			err: &ErrDuplicateTemplateName{
				Name:      "backup",
				Locations: []string{},
			},
			expected: `duplicate template name "backup"`,
		},
		{
			name: "multiple locations",
			err: &ErrDuplicateTemplateName{
				Name: "backup",
				Locations: []string{
					"/tmp/template1.toml",
					"/tmp/template2.toml",
					"/tmp/template3.toml",
				},
			},
			expected: `duplicate command template name "backup"
  Defined in:
    - /tmp/template1.toml
    - /tmp/template2.toml
    - /tmp/template3.toml`,
		},
		{
			name: "two locations",
			err: &ErrDuplicateTemplateName{
				Name: "restore",
				Locations: []string{
					"/etc/config.toml",
					"/home/user/.config/config.toml",
				},
			},
			expected: `duplicate command template name "restore"
  Defined in:
    - /etc/config.toml
    - /home/user/.config/config.toml`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.err.Error()
			assert.Equal(t, tt.expected, actual)
		})
	}
}
