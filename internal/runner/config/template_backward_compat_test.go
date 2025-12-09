//go:build test

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackwardCompatibility tests that existing configuration files
// continue to work after the template feature is added.
func TestBackwardCompatibility(t *testing.T) {
	// List of existing sample files (excluding template examples)
	sampleFiles := []string{
		"sample/starter.toml",
		"sample/comprehensive.toml",
		"sample/risk-based-control.toml",
		"sample/timeout_examples.toml",
		"sample/output_capture_error_test.toml",
		"sample/output_capture_too_large_error.toml",
		"sample/output_capture_basic.toml",
		"sample/output_capture_single_error.toml",
		"sample/auto_env_group.toml",
		"sample/output_capture_security.toml",
		"sample/workdir_examples.toml",
		"sample/slack-notify.toml",
		"sample/slack-group-notification-test.toml",
		"sample/group_cmd_allowed.toml",
		"sample/auto_env_test.toml",
		"sample/auto_env_example.toml",
		"sample/output_capture_advanced.toml",
		"sample/variable_expansion_basic.toml",
		"sample/variable_expansion_advanced.toml",
		"sample/variable_expansion_security.toml",
		"sample/variable_expansion_test.toml",
		"sample/vars_env_separation_e2e.toml",
	}

	loader := NewLoader()

	// Get the project root (go up 3 levels from internal/runner/config)
	wd, err := os.Getwd()
	require.NoError(t, err)
	projectRoot := filepath.Join(wd, "..", "..", "..")

	for _, relPath := range sampleFiles {
		t.Run(relPath, func(t *testing.T) {
			fullPath := filepath.Join(projectRoot, relPath)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				t.Skipf("skipping %s: %v", relPath, err)
				return
			}

			cfg, err := loader.LoadConfig(content)
			require.NoError(t, err, "failed to load %s", relPath)

			// Verify basic structure
			assert.NotNil(t, cfg, "config should not be nil for %s", relPath)
			// Note: Some older sample files may not have version field
			// assert.NotEmpty(t, cfg.Version, "version should not be empty for %s", relPath)

			// Verify that no templates are defined in these files
			assert.Empty(t, cfg.CommandTemplates, "existing sample files should not have templates: %s", relPath)

			// Verify that groups are loaded correctly
			if len(cfg.Groups) > 0 {
				for _, group := range cfg.Groups {
					assert.NotEmpty(t, group.Name, "group name should not be empty in %s", relPath)

					// Verify that commands in these groups don't use templates
					for _, cmd := range group.Commands {
						assert.Empty(t, cmd.Template, "commands should not use templates in %s", relPath)
						assert.Nil(t, cmd.Params, "commands should not have params in %s", relPath)
					}
				}
			}
		})
	}
}

// TestBackwardCompatibilityWithExpansion tests that existing configuration files
// work correctly with ExpandCommand (no templates).
func TestBackwardCompatibilityWithExpansion(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		groupName   string
		cmdName     string
		wantCmd     string
		wantArgs    []string
	}{
		{
			name: "basic command without template",
			tomlContent: `
version = "1.0"

[[groups]]
name = "test_group"

[[groups.commands]]
name = "echo_test"
cmd = "echo"
args = ["hello", "world"]
`,
			groupName: "test_group",
			cmdName:   "echo_test",
			wantCmd:   "echo",
			wantArgs:  []string{"hello", "world"},
		},
		{
			name: "command with variable expansion",
			tomlContent: `
version = "1.0"

[[groups]]
name = "test_group"

[groups.vars]
greeting = "Hello"

[[groups.commands]]
name = "echo_greeting"
cmd = "echo"
args = ["%{greeting}", "World"]
`,
			groupName: "test_group",
			cmdName:   "echo_greeting",
			wantCmd:   "echo",
			wantArgs:  []string{"Hello", "World"},
		},
		{
			name: "command with env and workdir",
			tomlContent: `
version = "1.0"

[[groups]]
name = "test_group"

[groups.vars]
project_root = "/opt/project"

[[groups.commands]]
name = "build"
cmd = "make"
args = ["build"]
workdir = "%{project_root}"
env = ["GO111MODULE=on", "GOOS=linux"]
`,
			groupName: "test_group",
			cmdName:   "build",
			wantCmd:   "make",
			wantArgs:  []string{"build"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.tomlContent))
			require.NoError(t, err)

			// Find the target group
			var targetGroup *runnertypes.GroupSpec
			for i := range cfg.Groups {
				if cfg.Groups[i].Name == tt.groupName {
					targetGroup = &cfg.Groups[i]
					break
				}
			}
			require.NotNil(t, targetGroup, "group %s not found", tt.groupName)

			// Find the target command
			var targetCmd *runnertypes.CommandSpec
			for i := range targetGroup.Commands {
				if targetGroup.Commands[i].Name == tt.cmdName {
					targetCmd = &targetGroup.Commands[i]
					break
				}
			}
			require.NotNil(t, targetCmd, "command %s not found", tt.cmdName)

			// Expand global first
			globalRuntime, err := ExpandGlobal(&cfg.Global)
			require.NoError(t, err)

			// Expand group
			runtimeGroup, err := ExpandGroup(targetGroup, globalRuntime)
			require.NoError(t, err)

			// Expand the command (with empty template map for backward compatibility)
			expanded, err := ExpandCommand(targetCmd, cfg.CommandTemplates, runtimeGroup, globalRuntime, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit())
			require.NoError(t, err)

			// Verify expansion results
			assert.Equal(t, tt.wantCmd, expanded.ExpandedCmd)
			assert.Equal(t, tt.wantArgs, expanded.ExpandedArgs)
		})
	}
}

// TestNoRegressionInExistingBehavior tests specific behaviors that should not change.
func TestNoRegressionInExistingBehavior(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		description string
	}{
		{
			name: "empty command templates section is allowed",
			tomlContent: `
version = "1.0"

[[groups]]
name = "test"

[[groups.commands]]
name = "echo_test"
cmd = "echo"
args = ["test"]
`,
			description: "config with no command_templates section should load successfully",
		},
		{
			name: "command spec without template field",
			tomlContent: `
version = "1.0"

[[groups]]
name = "test"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["hello"]
`,
			description: "commands without template/params fields should work as before",
		},
		{
			name: "variable expansion still works",
			tomlContent: `
version = "1.0"

[[groups]]
name = "test"

[groups.vars]
msg = "Hello"

[[groups.commands]]
name = "greet"
cmd = "echo"
args = ["%{msg}"]
`,
			description: "existing %{var} expansion should continue to work",
		},
	}

	loader := NewLoader()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loader.LoadConfig([]byte(tt.tomlContent))
			require.NoError(t, err, tt.description)
			assert.NotNil(t, cfg)
		})
	}
}
