package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverrideStringPointer(t *testing.T) {
	tests := []struct {
		name          string
		cmdValue      *string
		templateValue *string
		want          *string
		description   string
	}{
		{
			name:          "both nil",
			cmdValue:      nil,
			templateValue: nil,
			want:          nil,
			description:   "When both are nil, should return nil",
		},
		{
			name:          "command nil, template non-nil",
			cmdValue:      nil,
			templateValue: stringPtr("/template/dir"),
			want:          stringPtr("/template/dir"),
			description:   "When command is nil, should inherit from template",
		},
		{
			name:          "command non-nil, template non-nil",
			cmdValue:      stringPtr("/command/dir"),
			templateValue: stringPtr("/template/dir"),
			want:          stringPtr("/command/dir"),
			description:   "When command is non-nil, should use command value",
		},
		{
			name:          "command empty string, template non-nil",
			cmdValue:      stringPtr(""),
			templateValue: stringPtr("/template/dir"),
			want:          stringPtr(""),
			description:   "When command is empty string (non-nil), should use empty string",
		},
		{
			name:          "command non-nil, template nil",
			cmdValue:      stringPtr("/command/dir"),
			templateValue: nil,
			want:          stringPtr("/command/dir"),
			description:   "When command is non-nil and template is nil, should use command value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OverrideStringPointer(tt.cmdValue, tt.templateValue)
			if tt.want == nil {
				assert.Nil(t, got, tt.description)
			} else {
				assert.NotNil(t, got, tt.description)
				assert.Equal(t, *tt.want, *got, tt.description)
			}
		})
	}
}

func TestMergeEnvImport(t *testing.T) {
	tests := []struct {
		name           string
		templateImport []string
		cmdImport      []string
		want           []string
		description    string
	}{
		{
			name:           "both empty",
			templateImport: []string{},
			cmdImport:      []string{},
			want:           []string{},
			description:    "When both are empty, should return empty slice",
		},
		{
			name:           "template only",
			templateImport: []string{"TEMPLATE_VAR1", "TEMPLATE_VAR2"},
			cmdImport:      []string{},
			want:           []string{"TEMPLATE_VAR1", "TEMPLATE_VAR2"},
			description:    "When only template has imports, should return template imports",
		},
		{
			name:           "command only",
			templateImport: []string{},
			cmdImport:      []string{"CMD_VAR1", "CMD_VAR2"},
			want:           []string{"CMD_VAR1", "CMD_VAR2"},
			description:    "When only command has imports, should return command imports",
		},
		{
			name:           "no duplicates",
			templateImport: []string{"TEMPLATE_VAR1", "TEMPLATE_VAR2"},
			cmdImport:      []string{"CMD_VAR1", "CMD_VAR2"},
			want:           []string{"TEMPLATE_VAR1", "TEMPLATE_VAR2", "CMD_VAR1", "CMD_VAR2"},
			description:    "When no duplicates, should merge both lists",
		},
		{
			name:           "with duplicates",
			templateImport: []string{"VAR1", "VAR2", "VAR3"},
			cmdImport:      []string{"VAR2", "VAR4", "VAR1"},
			want:           []string{"VAR1", "VAR2", "VAR3", "VAR4"},
			description:    "When duplicates exist, first occurrence wins",
		},
		{
			name:           "template nil, command non-nil",
			templateImport: nil,
			cmdImport:      []string{"CMD_VAR1"},
			want:           []string{"CMD_VAR1"},
			description:    "When template is nil, should handle gracefully",
		},
		{
			name:           "template non-nil, command nil",
			templateImport: []string{"TEMPLATE_VAR1"},
			cmdImport:      nil,
			want:           []string{"TEMPLATE_VAR1"},
			description:    "When command is nil, should handle gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeEnvImport(tt.templateImport, tt.cmdImport)
			assert.Equal(t, tt.want, got, tt.description)
		})
	}
}

func TestMergeVars(t *testing.T) {
	tests := []struct {
		name         string
		templateVars map[string]any
		cmdVars      map[string]any
		want         map[string]any
		description  string
	}{
		{
			name:         "both empty",
			templateVars: map[string]any{},
			cmdVars:      map[string]any{},
			want:         map[string]any{},
			description:  "When both are empty, should return empty map",
		},
		{
			name:         "template only",
			templateVars: map[string]any{"key1": "template1", "key2": 123},
			cmdVars:      map[string]any{},
			want:         map[string]any{"key1": "template1", "key2": 123},
			description:  "When only template has vars, should return template vars",
		},
		{
			name:         "command only",
			templateVars: map[string]any{},
			cmdVars:      map[string]any{"key1": "cmd1", "key2": 456},
			want:         map[string]any{"key1": "cmd1", "key2": 456},
			description:  "When only command has vars, should return command vars",
		},
		{
			name:         "no key conflicts",
			templateVars: map[string]any{"template_key1": "value1", "template_key2": 123},
			cmdVars:      map[string]any{"cmd_key1": "value2", "cmd_key2": 456},
			want:         map[string]any{"template_key1": "value1", "template_key2": 123, "cmd_key1": "value2", "cmd_key2": 456},
			description:  "When no conflicts, should merge both maps",
		},
		{
			name:         "with key conflicts - command wins",
			templateVars: map[string]any{"key1": "template_value", "key2": 123},
			cmdVars:      map[string]any{"key1": "command_value", "key3": 456},
			want:         map[string]any{"key1": "command_value", "key2": 123, "key3": 456},
			description:  "When key conflicts, command value takes precedence",
		},
		{
			name:         "template nil, command non-nil",
			templateVars: nil,
			cmdVars:      map[string]any{"key1": "value1"},
			want:         map[string]any{"key1": "value1"},
			description:  "When template is nil, should handle gracefully",
		},
		{
			name:         "template non-nil, command nil",
			templateVars: map[string]any{"key1": "value1"},
			cmdVars:      nil,
			want:         map[string]any{"key1": "value1"},
			description:  "When command is nil, should handle gracefully",
		},
		{
			name:         "both nil",
			templateVars: nil,
			cmdVars:      nil,
			want:         map[string]any{},
			description:  "When both are nil, should return empty map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeVars(tt.templateVars, tt.cmdVars)
			assert.Equal(t, tt.want, got, tt.description)
		})
	}
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
