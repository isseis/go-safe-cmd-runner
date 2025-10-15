package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackwardCompatibility_AllSampleFiles verifies that all existing sample files
// can be loaded without errors, ensuring backward compatibility with the new
// Global.Env and Group.Env features.
func TestBackwardCompatibility_AllSampleFiles(t *testing.T) {
	// Set up test environment variables for samples that reference system env vars
	if os.Getenv("HOME") == "" {
		t.Setenv("HOME", "/home/testuser")
	}
	if os.Getenv("USER") == "" {
		t.Setenv("USER", "testuser")
	}
	if os.Getenv("PATH") == "" {
		t.Setenv("PATH", "/usr/bin:/bin")
	}

	sampleFiles := []string{
		"slack-group-notification-test.toml",
		"slack-notify.toml",
		"risk-based-control.toml",
		"comprehensive.toml",
		"output_capture_single_error.toml",
		"output_capture_basic.toml",
		"output_capture_advanced.toml",
		"output_capture_error_test.toml",
		"output_capture_security.toml",
		"variable_expansion_test.toml",
		"variable_expansion_security.toml",
		"variable_expansion_basic.toml",
		"variable_expansion_advanced.toml",
		// "auto_env_error.toml", // Excluded: This file is expected to cause errors
		"auto_env_example.toml",
		"auto_env_group.toml",
		"auto_env_test.toml",
		// "verify_files_expansion.toml", // Excluded: Requires CONFIG_ROOT environment variable to be set
	}

	for _, filename := range sampleFiles {
		filename := filename // capture range variable
		t.Run(filename, func(t *testing.T) {
			configPath := filepath.Join("..", "..", "..", "sample", filename)
			content, err := os.ReadFile(configPath)
			require.NoError(t, err, "Failed to read sample file %s", filename)

			loader := config.NewLoader()
			cfg, err := loader.LoadConfig(content)

			// All sample files should load without errors
			require.NoError(t, err, "Sample file %s should load without errors", filename)
			require.NotNil(t, cfg, "Configuration should not be nil for %s", filename)

			// Verify basic structure is intact
			assert.NotNil(t, cfg.Global, "Global config should exist")
			assert.NotEmpty(t, cfg.Groups, "Should have at least one group")

			// Verify that Global.ExpandedEnv and Group.ExpandedEnv are initialized
			// even if Global.Env and Group.Env are not defined
			if len(cfg.Global.Env) == 0 {
				// If Global.Env is not defined, ExpandedEnv should be nil or empty
				if cfg.Global.ExpandedEnv != nil {
					assert.Empty(t, cfg.Global.ExpandedEnv, "Global.ExpandedEnv should be empty when Global.Env is not defined")
				}
			}

			for i := range cfg.Groups {
				group := &cfg.Groups[i]
				if len(group.Env) == 0 {
					// If Group.Env is not defined, ExpandedEnv should be nil or empty
					if group.ExpandedEnv != nil {
						assert.Empty(t, group.ExpandedEnv, "Group.ExpandedEnv should be empty when Group.Env is not defined for group %s", group.Name)
					}
				}
			}
		})
	}
}

// TestBackwardCompatibility_NoGlobalEnv verifies that configurations without
// Global.Env work as before (no regression).
func TestBackwardCompatibility_NoGlobalEnv(t *testing.T) {
	// Use an existing sample file that doesn't have Global.Env
	configPath := filepath.Join("..", "..", "..", "sample", "variable_expansion_basic.toml")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)

	require.NoError(t, err, "Configuration should load without errors")
	require.NotNil(t, cfg, "Configuration should not be nil")

	// Global.Env should be nil or empty
	if cfg.Global.Env != nil {
		assert.Empty(t, cfg.Global.Env, "Global.Env should be empty in this sample file")
	}

	// Global.ExpandedEnv should be nil or empty
	if cfg.Global.ExpandedEnv != nil {
		assert.Empty(t, cfg.Global.ExpandedEnv, "Global.ExpandedEnv should be empty when Global.Env is not defined")
	}

	// Configuration should still work normally
	assert.NotEmpty(t, cfg.Groups, "Should have groups")
}

// TestBackwardCompatibility_NoGroupEnv verifies that configurations without
// Group.Env work as before (no regression).
func TestBackwardCompatibility_NoGroupEnv(t *testing.T) {
	// Use an existing sample file
	configPath := filepath.Join("..", "..", "..", "sample", "output_capture_basic.toml")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)

	require.NoError(t, err, "Configuration should load without errors")
	require.NotNil(t, cfg, "Configuration should not be nil")

	// Groups should load normally
	require.NotEmpty(t, cfg.Groups, "Should have at least one group")

	for i := range cfg.Groups {
		group := &cfg.Groups[i]

		// Group.Env should be nil or empty
		if group.Env != nil {
			assert.Empty(t, group.Env, "Group.Env should be empty in this sample file for group %s", group.Name)
		}

		// Group.ExpandedEnv should be nil or empty
		if group.ExpandedEnv != nil {
			assert.Empty(t, group.ExpandedEnv, "Group.ExpandedEnv should be empty when Group.Env is not defined for group %s", group.Name)
		}

		// Commands should still exist
		assert.NotEmpty(t, group.Commands, "Group %s should have commands", group.Name)
	}
}

// TestBackwardCompatibility_ExistingBehavior verifies that the existing behavior
// of Command.Env, verify_files, and other features is preserved.
func TestBackwardCompatibility_ExistingBehavior(t *testing.T) {
	configPath := filepath.Join("..", "..", "..", "sample", "comprehensive.toml")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)

	require.NoError(t, err, "Configuration should load without errors")
	require.NotNil(t, cfg, "Configuration should not be nil")

	// Verify that existing features still work
	assert.NotNil(t, cfg.Global, "Global config should exist")
	assert.NotEmpty(t, cfg.Groups, "Should have groups")

	// Check that Command.Env is still present (not expanded at config load time)
	foundCommandWithEnv := false
	for i := range cfg.Groups {
		for j := range cfg.Groups[i].Commands {
			cmd := &cfg.Groups[i].Commands[j]
			if len(cmd.Env) > 0 {
				foundCommandWithEnv = true
				// Command.Env should not be expanded yet (happens in bootstrap)
				for _, envVar := range cmd.Env {
					assert.Contains(t, envVar, "=", "Command.Env should be in KEY=VALUE format")
				}
			}
		}
	}

	// comprehensive.toml should have at least one command with env
	assert.True(t, foundCommandWithEnv, "comprehensive.toml should contain at least one command with env variables")
}
