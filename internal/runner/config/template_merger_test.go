//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultTemplateMerger_MergeTemplates(t *testing.T) {
	tests := []struct {
		name          string
		sources       []TemplateSource
		wantTemplates map[string]runnertypes.CommandTemplate
		wantErr       bool
		errType       interface{}
		errLocations  []string
	}{
		{
			name: "single source with one template",
			sources: []TemplateSource{
				{
					FilePath: "/tmp/template1.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"backup": {Cmd: "restic", Args: []string{"backup"}},
					},
				},
			},
			wantTemplates: map[string]runnertypes.CommandTemplate{
				"backup": {Cmd: "restic", Args: []string{"backup"}},
			},
			wantErr: false,
		},
		{
			name: "multiple sources with different templates",
			sources: []TemplateSource{
				{
					FilePath: "/tmp/template1.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"backup": {Cmd: "restic", Args: []string{"backup"}},
					},
				},
				{
					FilePath: "/tmp/template2.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"restore": {Cmd: "restic", Args: []string{"restore"}},
					},
				},
			},
			wantTemplates: map[string]runnertypes.CommandTemplate{
				"backup":  {Cmd: "restic", Args: []string{"backup"}},
				"restore": {Cmd: "restic", Args: []string{"restore"}},
			},
			wantErr: false,
		},
		{
			name: "multiple sources with many templates",
			sources: []TemplateSource{
				{
					FilePath: "/tmp/template1.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"backup":   {Cmd: "restic", Args: []string{"backup"}},
						"snapshot": {Cmd: "restic", Args: []string{"snapshots"}},
					},
				},
				{
					FilePath: "/tmp/template2.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"restore": {Cmd: "restic", Args: []string{"restore"}},
						"prune":   {Cmd: "restic", Args: []string{"prune"}},
					},
				},
				{
					FilePath: "/tmp/config.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"check": {Cmd: "restic", Args: []string{"check"}},
					},
				},
			},
			wantTemplates: map[string]runnertypes.CommandTemplate{
				"backup":   {Cmd: "restic", Args: []string{"backup"}},
				"snapshot": {Cmd: "restic", Args: []string{"snapshots"}},
				"restore":  {Cmd: "restic", Args: []string{"restore"}},
				"prune":    {Cmd: "restic", Args: []string{"prune"}},
				"check":    {Cmd: "restic", Args: []string{"check"}},
			},
			wantErr: false,
		},
		{
			name: "duplicate template name across sources",
			sources: []TemplateSource{
				{
					FilePath: "/tmp/template1.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"backup": {Cmd: "restic", Args: []string{"backup"}},
					},
				},
				{
					FilePath: "/tmp/template2.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"backup": {Cmd: "restic", Args: []string{"backup", "--verbose"}},
					},
				},
			},
			wantErr:      true,
			errType:      &ErrDuplicateTemplateName{},
			errLocations: []string{"/tmp/template1.toml", "/tmp/template2.toml"},
		},
		{
			name: "duplicate in third source",
			sources: []TemplateSource{
				{
					FilePath: "/tmp/template1.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"backup": {Cmd: "restic", Args: []string{"backup"}},
					},
				},
				{
					FilePath: "/tmp/template2.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"restore": {Cmd: "restic", Args: []string{"restore"}},
					},
				},
				{
					FilePath: "/tmp/config.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"backup": {Cmd: "restic", Args: []string{"backup", "--new"}},
					},
				},
			},
			wantErr:      true,
			errType:      &ErrDuplicateTemplateName{},
			errLocations: []string{"/tmp/template1.toml", "/tmp/config.toml"},
		},
		{
			name:          "empty sources",
			sources:       []TemplateSource{},
			wantTemplates: map[string]runnertypes.CommandTemplate{},
			wantErr:       false,
		},
		{
			name: "source with empty templates",
			sources: []TemplateSource{
				{
					FilePath:  "/tmp/template1.toml",
					Templates: map[string]runnertypes.CommandTemplate{},
				},
				{
					FilePath: "/tmp/template2.toml",
					Templates: map[string]runnertypes.CommandTemplate{
						"backup": {Cmd: "restic", Args: []string{"backup"}},
					},
				},
			},
			wantTemplates: map[string]runnertypes.CommandTemplate{
				"backup": {Cmd: "restic", Args: []string{"backup"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTemplates, err := mergeTemplates(tt.sources)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.IsType(t, tt.errType, err)

					if len(tt.errLocations) > 0 {
						var errDuplicate *ErrDuplicateTemplateName
						require.ErrorAs(t, err, &errDuplicate)
						assert.ElementsMatch(t, tt.errLocations, errDuplicate.Locations)
					}
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

func TestDefaultTemplateMerger_MergeOrder(t *testing.T) {
	// Verify that templates are merged in order and first wins
	sources := []TemplateSource{
		{
			FilePath: "/tmp/first.toml",
			Templates: map[string]runnertypes.CommandTemplate{
				"test": {Cmd: "first", Args: []string{"arg1"}},
			},
		},
		{
			FilePath: "/tmp/second.toml",
			Templates: map[string]runnertypes.CommandTemplate{
				"other": {Cmd: "second", Args: []string{"arg2"}},
			},
		},
	}

	merged, err := mergeTemplates(sources)

	require.NoError(t, err)
	assert.Equal(t, "first", merged["test"].Cmd)
	assert.Equal(t, "second", merged["other"].Cmd)
}
