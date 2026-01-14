//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTemplateContent(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		wantTemplates map[string]runnertypes.CommandTemplate
		wantErr       bool
		errType       interface{}
		errorContains string
	}{
		{
			name: "valid template file with one template",
			fileContent: `version = "1.0"

[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]
`,
			wantTemplates: map[string]runnertypes.CommandTemplate{
				"backup": {
					Cmd:  "restic",
					Args: []string{"backup", "${path}"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid template file with multiple templates",
			fileContent: `version = "1.0"

[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]

[command_templates.restore]
cmd = "restic"
args = ["restore", "${snapshot}", "--target", "${target}"]
`,
			wantTemplates: map[string]runnertypes.CommandTemplate{
				"backup": {
					Cmd:  "restic",
					Args: []string{"backup", "${path}"},
				},
				"restore": {
					Cmd:  "restic",
					Args: []string{"restore", "${snapshot}", "--target", "${target}"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty template file",
			fileContent: `version = "1.0"
`,
			wantTemplates: map[string]runnertypes.CommandTemplate{},
			wantErr:       false,
		},
		{
			name: "template file with only version",
			fileContent: `version = "1.0"

[command_templates]
`,
			wantTemplates: map[string]runnertypes.CommandTemplate{},
			wantErr:       false,
		},
		{
			name: "template file with unknown field global",
			fileContent: `version = "1.0"

[global]
env_allowed = ["PATH"]

[command_templates.backup]
cmd = "restic"
`,
			wantErr:       true,
			errType:       &ErrTemplateFileInvalidFormat{},
			errorContains: "invalid fields",
		},
		{
			name: "template file with unknown field groups",
			fileContent: `version = "1.0"

[[groups]]
name = "test"

[command_templates.backup]
cmd = "restic"
`,
			wantErr:       true,
			errType:       &ErrTemplateFileInvalidFormat{},
			errorContains: "invalid fields",
		},
		{
			name: "template file with includes field",
			fileContent: `version = "1.0"
includes = ["other.toml"]

[command_templates.backup]
cmd = "restic"
`,
			wantErr:       true,
			errType:       &ErrTemplateFileInvalidFormat{},
			errorContains: "invalid fields",
		},
		{
			name: "template file with unknown top-level field",
			fileContent: `version = "1.0"
custom_field = "value"

[command_templates.backup]
cmd = "restic"
`,
			wantErr:       true,
			errType:       &ErrTemplateFileInvalidFormat{},
			errorContains: "invalid fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTemplates, err := ParseTemplateContent([]byte(tt.fileContent), "test.toml")

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.IsType(t, tt.errType, err)
				}
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, len(tt.wantTemplates), len(gotTemplates))
				for name, wantTemplate := range tt.wantTemplates {
					gotTemplate, exists := gotTemplates[name]
					require.True(t, exists, "template %s should exist", name)
					assert.Equal(t, wantTemplate.Cmd, gotTemplate.Cmd)
					assert.Equal(t, wantTemplate.Args, gotTemplate.Args)
				}
			}
		})
	}
}

func TestParseTemplateContent_MalformedTOML(t *testing.T) {
	content := []byte("invalid toml content [[[")
	_, err := ParseTemplateContent(content, "malformed.toml")

	require.Error(t, err)
	var errInvalidFormat *ErrTemplateFileInvalidFormat
	require.ErrorAs(t, err, &errInvalidFormat)
	assert.Equal(t, "malformed.toml", errInvalidFormat.TemplateFile)
}
