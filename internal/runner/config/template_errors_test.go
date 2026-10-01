//go:build test

package config

import (
	"fmt"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// templateErrorCase pins one template error type's structured message against
// its legacy rendering.
type templateErrorCase struct {
	name string
	err  error
	// legacy is the string the old fmt.Sprintf Error() produced.
	legacy string
	// segments are the non-constant segments, in order (appendix A roles).
	segments errmsg.Segments
}

// templateErrorCases builds the shared table. Values that used to be rendered
// with %q carry a quote and a backslash so a non-quoting rewrite is caught.
func templateErrorCases() []templateErrorCase {
	const qName = "a\"b\\c"
	const qBody = "a\\\"b\\\\c"

	return []templateErrorCase{
		{
			name:     "ErrTemplateNotFound",
			err:      &ErrTemplateNotFound{CommandName: "cmd", TemplateName: "tpl"},
			legacy:   `template "tpl" not found (referenced by command "cmd")`,
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: "tpl"}, {Role: errmsg.RoleIdentifier, Text: "cmd"}},
		},
		{
			name:   "ErrTemplateFieldConflict",
			err:    &ErrTemplateFieldConflict{GroupName: "deploy", CommandIndex: 2, TemplateName: "tpl", Field: cmdField()},
			legacy: `group[deploy] command[2]: cannot specify both "template" and "cmd" fields in command definition`,
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "2"},
			},
		},
		{
			name:     "ErrDuplicateTemplateName single",
			err:      &ErrDuplicateTemplateName{Name: "backup"},
			legacy:   `duplicate template name "backup"`,
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: "backup"}},
		},
		{
			name:     "ErrDuplicateTemplateName multiple",
			err:      &ErrDuplicateTemplateName{Name: qName, Locations: []string{"/a.toml", "/b.toml"}},
			legacy:   fmt.Sprintf("duplicate command template name %q\n  Defined in:\n    - /a.toml\n    - /b.toml", qName),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RolePath, Text: "/a.toml"}, {Role: errmsg.RolePath, Text: "/b.toml"}},
		},
		{
			name:     "ErrInvalidTemplateName",
			err:      &ErrInvalidTemplateName{Name: qName, Reason: "bad"},
			legacy:   fmt.Sprintf("invalid template name %q: bad", qName),
			segments: errmsg.Segments{{Role: errmsg.RoleText, Text: qBody}, {Role: errmsg.RoleText, Text: "bad"}},
		},
		{
			name:     "ErrReservedTemplateName",
			err:      &ErrReservedTemplateName{Name: qName},
			legacy:   fmt.Sprintf("template name %q uses reserved prefix \"__\"", qName),
			segments: errmsg.Segments{{Role: errmsg.RoleText, Text: qBody}},
		},
		{
			name:     "ErrTemplateContainsNameField",
			err:      &ErrTemplateContainsNameField{TemplateName: "tpl"},
			legacy:   `template definition "tpl" cannot contain "name" field`,
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: "tpl"}},
		},
		{
			name:     "ErrMissingRequiredField template",
			err:      &ErrMissingRequiredField{TemplateName: "tpl", Field: cmdField()},
			legacy:   `template "tpl": required field "cmd" is missing`,
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: "tpl"}},
		},
		{
			name:   "ErrMissingRequiredField group",
			err:    &ErrMissingRequiredField{GroupName: "deploy", CommandIndex: 1, Field: argsFieldNoIndex()},
			legacy: `group[deploy] command[1]: required field "args" is missing`,
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "1"},
			},
		},
		{
			name:     "ErrRequiredParamMissing",
			err:      &ErrRequiredParamMissing{TemplateName: qName, Field: argsField(0), ParamName: qName},
			legacy:   fmt.Sprintf("template %q args[0]: required parameter %q not provided", qName, qName),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: "0"}, {Role: errmsg.RoleIdentifier, Text: qBody}},
		},
		{
			name:     "ErrTemplateTypeMismatch",
			err:      &ErrTemplateTypeMismatch{TemplateName: qName, Field: argsField(1), ParamName: qName, Expected: typeNameString, Actual: typeNameArray},
			legacy:   fmt.Sprintf("template %q args[1]: parameter %q expected %s, got %s", qName, qName, typeNameString, typeNameArray),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: "1"}, {Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: typeNameString}, {Role: errmsg.RoleText, Text: typeNameArray}},
		},
		{
			name:     "ErrPlaceholderInEnvKey",
			err:      &ErrPlaceholderInEnvKey{TemplateName: qName, EnvEntry: "K=${x}", Key: "K"},
			legacy:   fmt.Sprintf("template %q env: placeholder in key %q is not allowed (env entry: %q) - only values can contain placeholders", qName, "K", "K=${x}"),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: "K"}, {Role: errmsg.RoleText, Text: "K=${x}"}},
		},
		{
			name:     "ErrTemplateInvalidEnvFormat",
			err:      &ErrTemplateInvalidEnvFormat{TemplateName: qName, Field: envVarsField(0), Entry: "BAD"},
			legacy:   fmt.Sprintf("template %q env_vars[0]: invalid env format: %q (expected KEY=VALUE format)", qName, "BAD"),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: "0"}, {Role: errmsg.RoleText, Text: "BAD"}},
		},
		{
			name:     "ErrTemplateInvalidEnvFormat expanded",
			err:      &ErrTemplateInvalidEnvFormat{TemplateName: qName, Field: envVarsField(0), ExpandedIndex: 1, Entry: "BAD"},
			legacy:   fmt.Sprintf("template %q env_vars[0]: invalid env format in expanded element [1]: %q (expected KEY=VALUE format)", qName, "BAD"),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: "0"}, {Role: errmsg.RoleText, Text: "1"}, {Role: errmsg.RoleText, Text: "BAD"}},
		},
		{
			name:     "ErrArrayInMixedContext",
			err:      &ErrArrayInMixedContext{TemplateName: qName, Field: cmdField(), ParamName: qName},
			legacy:   fmt.Sprintf("template %q cmd: array parameter ${@%s} cannot be used in mixed context", qName, qName),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleIdentifier, Text: qName}},
		},
		{
			name:     "ErrTemplateInvalidArrayElement",
			err:      &ErrTemplateInvalidArrayElement{TemplateName: qName, Field: envVarsField(0), ParamName: qName, Index: 1, ActualType: "int"},
			legacy:   fmt.Sprintf("template %q env_vars[0]: array parameter %q contains non-string element at index %d (type: %s)", qName, qName, 1, "int"),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: "0"}, {Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: "1"}, {Role: errmsg.RoleText, Text: "int"}},
		},
		{
			name:     "ErrUnsupportedParamType",
			err:      &ErrUnsupportedParamType{TemplateName: qName, Field: varsField(), ParamName: qName, ActualType: "int"},
			legacy:   fmt.Sprintf("template %q vars: parameter %q has unsupported type %s (expected string or []string)", qName, qName, "int"),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: "int"}},
		},
		{
			name:     "ErrEmptyPlaceholderName",
			err:      &ErrEmptyPlaceholderName{Input: "${}", Position: 0},
			legacy:   `empty placeholder name at position 0 in "${}"`,
			segments: errmsg.Segments{{Role: errmsg.RoleText, Text: "0"}, {Role: errmsg.RoleText, Text: "${}"}},
		},
		{
			name:     "ErrUnclosedPlaceholder",
			err:      &ErrUnclosedPlaceholder{Input: "${x", Position: 0},
			legacy:   `unclosed placeholder at position 0 in "${x"`,
			segments: errmsg.Segments{{Role: errmsg.RoleText, Text: "0"}, {Role: errmsg.RoleText, Text: "${x"}},
		},
		{
			name:     "ErrEmptyPlaceholder",
			err:      &ErrEmptyPlaceholder{Input: "${}", Position: 0},
			legacy:   `empty placeholder at position 0 in "${}"`,
			segments: errmsg.Segments{{Role: errmsg.RoleText, Text: "0"}, {Role: errmsg.RoleText, Text: "${}"}},
		},
		{
			name:     "ErrInvalidPlaceholderName",
			err:      &ErrInvalidPlaceholderName{Input: "${1}", Position: 0, Name: "1", Reason: "bad"},
			legacy:   fmt.Sprintf("invalid placeholder name %q at position %d in %q: %s", "1", 0, "${1}", "bad"),
			segments: errmsg.Segments{{Role: errmsg.RoleText, Text: "1"}, {Role: errmsg.RoleText, Text: "0"}, {Role: errmsg.RoleText, Text: "${1}"}, {Role: errmsg.RoleText, Text: "bad"}},
		},
		{
			name:     "ErrTemplateCmdNotSingleValue zero",
			err:      &ErrTemplateCmdNotSingleValue{TemplateName: qName},
			legacy:   fmt.Sprintf("template %q: cmd field must resolve to exactly one non-empty value, got 0 values (check optional placeholders)", qName),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}},
		},
		{
			name:     "ErrTemplateCmdNotSingleValue many",
			err:      &ErrTemplateCmdNotSingleValue{TemplateName: qName, ResultCount: 2},
			legacy:   fmt.Sprintf("template %q: cmd field must resolve to exactly one value, got %d values", qName, 2),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: "2"}},
		},
		{
			name:     "ErrDuplicateEnvVariableDetail",
			err:      &ErrDuplicateEnvVariableDetail{TemplateName: qName, Field: envVarsFieldNoIndex(), EnvKey: qName},
			legacy:   fmt.Sprintf("template %q env_vars: duplicate environment variable key %q", qName, qName),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleText, Text: qBody}},
		},
		{
			name:     "ErrTemplateVarUnexpectedMultipleValues",
			err:      &ErrTemplateVarUnexpectedMultipleValues{TemplateName: qName, Field: varField(qName)},
			legacy:   fmt.Sprintf("template %q field %q: unexpected multiple values from expansion", qName, varField(qName)),
			segments: errmsg.Segments{{Role: errmsg.RoleIdentifier, Text: qBody}, {Role: errmsg.RoleIdentifier, Text: qBody}},
		},
	}
}

// TestTemplateErrorTypes_StructuredMessageSegments pins, for every template
// error type, the order and role of the declared value segments (appendix A).
func TestTemplateErrorTypes_StructuredMessageSegments(t *testing.T) {
	for _, tt := range templateErrorCases() {
		t.Run(tt.name, func(t *testing.T) {
			structured, ok := tt.err.(errmsg.Structured)
			require.Truef(t, ok, "%T must implement errmsg.Structured", tt.err)
			assert.Equal(t, tt.segments, nonConstantSegments(structured.StructuredMessage()))
		})
	}
}

// TestTemplateErrorTypes_ErrorMessageMatchesLegacyFormat pins that every
// template error type renders exactly what the old fmt.Sprintf Error() produced.
func TestTemplateErrorTypes_ErrorMessageMatchesLegacyFormat(t *testing.T) {
	for _, tt := range templateErrorCases() {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.legacy, tt.err.Error())
		})
	}
}
