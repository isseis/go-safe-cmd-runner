package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackwardCompatibility_AllSampleFiles ensures all existing sample TOML files
// can be loaded without errors, verifying backward compatibility with the new
// WorkDir *string type change.
func TestBackwardCompatibility_AllSampleFiles(t *testing.T) {
	// List of all sample TOML files to test
	sampleFiles := []string{
		"starter.toml",
		"comprehensive.toml",
		"risk-based-control.toml",
		"command_template_example.toml",
		"auto_env_example.toml",
		"auto_env_group.toml",
		"auto_env_test.toml",
		"output_capture_basic.toml",
		"output_capture_advanced.toml",
		"output_capture_error_test.toml",
		"output_capture_security.toml",
		"output_capture_single_error.toml",
		"output_capture_too_large_error.toml",
		"variable_expansion_basic.toml",
		"variable_expansion_advanced.toml",
		"variable_expansion_security.toml",
		"variable_expansion_test.toml",
		"vars_env_separation_e2e.toml",
		"workdir_examples.toml",
		"timeout_examples.toml",
		"group_cmd_allowed.toml",
		"slack-notify.toml",
		"slack-group-notification-test.toml",
		"template_inheritance_example.toml",
	}

	// Get the absolute path to the sample directory
	// Assuming we're in internal/runner/config and sample is at repo root
	wd, err := os.Getwd()
	require.NoError(t, err)

	// Navigate up to repo root (from internal/runner/config -> ../../..)
	repoRoot := filepath.Join(wd, "..", "..", "..")
	sampleDir := filepath.Join(repoRoot, "sample")

	for _, filename := range sampleFiles {
		t.Run(filename, func(t *testing.T) {
			filePath := filepath.Join(sampleDir, filename)

			// Read the file
			content, err := os.ReadFile(filePath)
			require.NoError(t, err, "Failed to read %s", filename)

			// Create loader and load the config
			loader := NewLoader()
			cfg, err := loader.LoadConfig(content)

			// Assert no errors during loading
			assert.NoError(t, err, "Failed to load %s", filename)
			assert.NotNil(t, cfg, "Config should not be nil for %s", filename)

			// Basic structure validation
			// Note: Some test files may not have version field
			if cfg != nil && cfg.Version != "" {
				assert.NotEmpty(t, cfg.Version, "Version should not be empty for %s", filename)
			}
		})
	}
}

// TestBackwardCompatibility_WorkDirHandling specifically tests WorkDir field handling
// with the new *string type across various scenarios.
func TestBackwardCompatibility_WorkDirHandling(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		expectError bool
		checkFunc   func(t *testing.T, cfg *runnertypes.ConfigSpec)
	}{
		{
			name: "command with workdir string",
			tomlContent: `
version = "1.0"

[[groups]]
name = "test"
[[groups.commands]]
name = "test-cmd"
cmd = "echo"
args = ["test"]
workdir = "/tmp"
`,
			expectError: false,
			checkFunc: func(t *testing.T, cfg *runnertypes.ConfigSpec) {
				require.Len(t, cfg.Groups, 1)
				require.Len(t, cfg.Groups[0].Commands, 1)
				cmd := cfg.Groups[0].Commands[0]
				require.NotNil(t, cmd.WorkDir, "WorkDir should not be nil")
				assert.Equal(t, "/tmp", *cmd.WorkDir)
			},
		},
		{
			name: "command without workdir",
			tomlContent: `
version = "1.0"

[[groups]]
name = "test"
[[groups.commands]]
name = "test-cmd"
cmd = "echo"
args = ["test"]
`,
			expectError: false,
			checkFunc: func(t *testing.T, cfg *runnertypes.ConfigSpec) {
				require.Len(t, cfg.Groups, 1)
				require.Len(t, cfg.Groups[0].Commands, 1)
				cmd := cfg.Groups[0].Commands[0]
				assert.Nil(t, cmd.WorkDir, "WorkDir should be nil when not specified")
			},
		},
		{
			name: "template with workdir",
			tomlContent: `
version = "1.0"

[command_templates.test_template]
cmd = "echo"
args = ["test"]
workdir = "/opt"

[[groups]]
name = "test"
[[groups.commands]]
name = "test-cmd"
template = "test_template"
`,
			expectError: false,
			checkFunc: func(t *testing.T, cfg *runnertypes.ConfigSpec) {
				require.NotNil(t, cfg.CommandTemplates)
				template, exists := cfg.CommandTemplates["test_template"]
				require.True(t, exists, "Template should exist")
				require.NotNil(t, template.WorkDir, "Template WorkDir should not be nil")
				assert.Equal(t, "/opt", *template.WorkDir)
			},
		},
		{
			name: "template without workdir",
			tomlContent: `
version = "1.0"

[command_templates.minimal]
cmd = "ls"

[[groups]]
name = "test"
[[groups.commands]]
name = "test-cmd"
template = "minimal"
`,
			expectError: false,
			checkFunc: func(t *testing.T, cfg *runnertypes.ConfigSpec) {
				require.NotNil(t, cfg.CommandTemplates)
				template, exists := cfg.CommandTemplates["minimal"]
				require.True(t, exists, "Template should exist")
				assert.Nil(t, template.WorkDir, "Template WorkDir should be nil when not specified")
			},
		},
		{
			name: "empty workdir string",
			tomlContent: `
version = "1.0"

[[groups]]
name = "test"
[[groups.commands]]
name = "test-cmd"
cmd = "pwd"
workdir = ""
`,
			expectError: false,
			checkFunc: func(t *testing.T, cfg *runnertypes.ConfigSpec) {
				require.Len(t, cfg.Groups, 1)
				require.Len(t, cfg.Groups[0].Commands, 1)
				cmd := cfg.Groups[0].Commands[0]
				require.NotNil(t, cmd.WorkDir, "WorkDir should not be nil for empty string")
				assert.Equal(t, "", *cmd.WorkDir, "WorkDir should be empty string")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.tomlContent))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, cfg)
				if tt.checkFunc != nil {
					tt.checkFunc(t, cfg)
				}
			}
		})
	}
}
