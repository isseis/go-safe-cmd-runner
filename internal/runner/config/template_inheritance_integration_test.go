//go:build test

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTemplateInheritance_TOMLLoad tests that the template inheritance example
// TOML file loads correctly and preserves template references.
// This test verifies TOML parsing, not runtime expansion.
func TestTemplateInheritance_TOMLLoad(t *testing.T) {
	t.Parallel()

	// Load the sample TOML file
	samplePath := filepath.Join("..", "..", "..", "sample", "template_inheritance_example.toml")
	content, err := os.ReadFile(samplePath)
	require.NoError(t, err, "Failed to read sample TOML file")

	loader := config.NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest(content)
	require.NoError(t, err, "Failed to load sample TOML file")
	require.NotNil(t, cfg, "Config should not be nil")

	// Verify basic structure
	assert.Len(t, cfg.Groups, 1, "Should have one group")
	assert.Equal(t, "test_inheritance", cfg.Groups[0].Name)
	assert.Len(t, cfg.Groups[0].Commands, 10, "Should have 10 commands")

	// Verify templates are loaded
	assert.Contains(t, cfg.CommandTemplates, "full_template")
	assert.Contains(t, cfg.CommandTemplates, "minimal_template")
	assert.Contains(t, cfg.CommandTemplates, "cmd_only")

	// Verify full_template structure
	fullTemplate := cfg.CommandTemplates["full_template"]
	assert.Equal(t, "pwd", fullTemplate.Cmd)
	assert.NotNil(t, fullTemplate.WorkDir)
	assert.Equal(t, "/template/dir", *fullTemplate.WorkDir)
	assert.NotNil(t, fullTemplate.OutputFile)
	assert.Equal(t, "/var/log/template.log", *fullTemplate.OutputFile)
	assert.Equal(t, []string{"TEMPLATE_VAR_A", "TEMPLATE_VAR_B"}, fullTemplate.EnvImport)
	assert.Equal(t, "template_value", fullTemplate.Vars["template_key"])
}

// TestTemplateInheritance_CommandReferences tests that commands with
// template references preserve the reference and command-level overrides.
func TestTemplateInheritance_CommandReferences(t *testing.T) {
	t.Parallel()

	samplePath := filepath.Join("..", "..", "..", "sample", "template_inheritance_example.toml")
	content, err := os.ReadFile(samplePath)
	require.NoError(t, err)

	loader := config.NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest(content)
	require.NoError(t, err)

	group := cfg.Groups[0]

	tests := []struct {
		name            string
		templateRef     string
		hasWorkDir      bool
		workDirValue    string
		hasOutputFile   bool
		outputFileValue string
		hasEnvImport    bool
		envImportCount  int
		hasVars         bool
		varsCount       int
	}{
		{
			name:        "inherit_all",
			templateRef: "full_template",
			// All fields should be empty/nil - inherited at expansion time
			hasWorkDir:    false,
			hasOutputFile: false,
			hasEnvImport:  false,
			hasVars:       false,
		},
		{
			name:          "override_workdir",
			templateRef:   "full_template",
			hasWorkDir:    true,
			workDirValue:  "/custom/dir",
			hasOutputFile: false,
			hasEnvImport:  false,
			hasVars:       false,
		},
		{
			name:            "override_output",
			templateRef:     "full_template",
			hasWorkDir:      false,
			hasOutputFile:   true,
			outputFileValue: "/custom/output.log",
			hasEnvImport:    false,
			hasVars:         false,
		},
		{
			name:           "merge_env_import",
			templateRef:    "full_template",
			hasWorkDir:     false,
			hasOutputFile:  false,
			hasEnvImport:   true,
			envImportCount: 1, // Only command-level entry
			hasVars:        false,
		},
		{
			name:          "merge_vars",
			templateRef:   "full_template",
			hasWorkDir:    false,
			hasOutputFile: false,
			hasEnvImport:  false,
			hasVars:       true,
			varsCount:     1, // Only command-level var
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := findCommand(t, group.Commands, tt.name)
			assert.Equal(t, tt.templateRef, cmd.Template, "Template reference should match")

			if tt.hasWorkDir {
				require.NotNil(t, cmd.WorkDir, "WorkDir should be set")
				assert.Equal(t, tt.workDirValue, *cmd.WorkDir)
			} else {
				assert.Nil(t, cmd.WorkDir, "WorkDir should be nil (inherited)")
			}

			if tt.hasOutputFile {
				require.NotNil(t, cmd.OutputFile, "OutputFile should be set")
				assert.Equal(t, tt.outputFileValue, *cmd.OutputFile)
			} else {
				assert.Nil(t, cmd.OutputFile, "OutputFile should be nil (inherited or not set)")
			}

			if tt.hasEnvImport {
				assert.Len(t, cmd.EnvImport, tt.envImportCount, "EnvImport count should match")
			} else {
				assert.Empty(t, cmd.EnvImport, "EnvImport should be empty (inherited)")
			}

			if tt.hasVars {
				assert.Len(t, cmd.Vars, tt.varsCount, "Vars count should match")
			} else {
				assert.Empty(t, cmd.Vars, "Vars should be empty (inherited)")
			}
		})
	}
}

// TestTemplateInheritance_GlobalConfig tests that global configuration
// is loaded correctly.
func TestTemplateInheritance_GlobalConfig(t *testing.T) {
	t.Parallel()

	samplePath := filepath.Join("..", "..", "..", "sample", "template_inheritance_example.toml")
	content, err := os.ReadFile(samplePath)
	require.NoError(t, err)

	loader := config.NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest(content)
	require.NoError(t, err)

	// Verify env_allowed
	assert.Contains(t, cfg.Global.EnvAllowed, "TEMPLATE_VAR_A")
	assert.Contains(t, cfg.Global.EnvAllowed, "TEMPLATE_VAR_B")
	assert.Contains(t, cfg.Global.EnvAllowed, "COMMAND_VAR_C")

	// Verify global vars
	assert.Contains(t, cfg.Global.Vars, "global_key")
	assert.Equal(t, "global_value", cfg.Global.Vars["global_key"])
}

// findCommand is a helper to find a command by name.
func findCommand(t *testing.T, commands []runnertypes.CommandSpec, name string) *runnertypes.CommandSpec {
	t.Helper()
	for i := range commands {
		if commands[i].Name == name {
			return &commands[i]
		}
	}
	t.Fatalf("Command %q not found", name)
	return nil
}
