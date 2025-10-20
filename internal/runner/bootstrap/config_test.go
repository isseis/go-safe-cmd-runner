//go:build test

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBootstrapCommandEnvExpansionIntegration verifies the complete expansion pipeline
// from Global.Env -> Group.Env -> Command.Env/Cmd/Args during bootstrap process.
// This test ensures that:
// 1. Global.Env is expanded correctly
// 2. Group.Env can reference Global.Env
// 3. Command.Env can reference Global.Env and Group.Env
// 4. Command.Cmd can reference Group.Env
// 5. Command.Args can reference Command.Env
func TestBootstrapCommandEnvExpansionIntegration(t *testing.T) {
	// Setup: Create temporary directory for hash storage
	tempDir := t.TempDir()
	hashDir := filepath.Join(tempDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create verification manager
	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Path to test config file (relative to internal/runner/config/testdata/)
	configPath := filepath.Join("..", "..", "..", "internal", "runner", "config", "testdata", "command_env_references_global_group.toml")
	configPath, err = filepath.Abs(configPath)
	require.NoError(t, err)

	// Record hash for the config file using filevalidator
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)
	_, err = validator.Record(configPath, false)
	require.NoError(t, err)

	// Load and prepare config (returns ConfigSpec)
	cfg, err := LoadAndPrepareConfig(verificationManager, configPath, "test-run-001")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Expand global spec to runtime
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)
	require.NotNil(t, runtimeGlobal)

	// Verify Global.Env expansion
	expectedGlobalEnv := map[string]string{
		"BASE_DIR": "/opt",
	}
	assert.Equal(t, expectedGlobalEnv, runtimeGlobal.ExpandedEnv, "Global.Env should be expanded correctly")

	// Verify groups
	require.Len(t, cfg.Groups, 1, "Should have exactly one group")

	// Find app_group
	var appGroupSpec *runnertypes.GroupSpec
	for i := range cfg.Groups {
		if cfg.Groups[i].Name == "app_group" {
			appGroupSpec = &cfg.Groups[i]
			break
		}
	}
	require.NotNil(t, appGroupSpec, "app_group should exist")

	// Expand group spec to runtime
	runtimeGroup, err := config.ExpandGroup(appGroupSpec, runtimeGlobal.ExpandedVars)
	require.NoError(t, err)
	require.NotNil(t, runtimeGroup)

	// Verify Group.Env expansion (references Global.Env)
	expectedGroupEnv := map[string]string{
		"APP_DIR": "/opt/myapp",
	}
	assert.Equal(t, expectedGroupEnv, runtimeGroup.ExpandedEnv, "Group.Env should reference Global.Env correctly")

	// Verify commands
	require.Len(t, appGroupSpec.Commands, 1, "app_group should have exactly one command")
	cmdSpec := &appGroupSpec.Commands[0]
	require.Equal(t, "run_app", cmdSpec.Name)

	// Expand command spec to runtime
	runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup.ExpandedVars, appGroupSpec.Name)
	require.NoError(t, err)
	require.NotNil(t, runtimeCmd)

	// Verify Command.Env expansion (uses internal variables)
	// Note: In new system, each level's ExpandedEnv contains only that level's env field values
	// The final process environment is built by BuildProcessEnvironment which merges all levels
	assert.Equal(t, "/opt/myapp/logs", runtimeCmd.ExpandedEnv["LOG_DIR"], "Command.Env variable LOG_DIR should be expanded correctly")

	// BASE_DIR and APP_DIR are in Global/Group ExpandedEnv respectively, not merged into Command.ExpandedEnv
	// They will be merged at execution time by BuildProcessEnvironment
	assert.Equal(t, "/opt", runtimeGlobal.ExpandedEnv["BASE_DIR"], "Global.Env variable BASE_DIR should be in Global.ExpandedEnv")
	assert.Equal(t, "/opt/myapp", runtimeGroup.ExpandedEnv["APP_DIR"], "Group.Env variable APP_DIR should be in Group.ExpandedEnv")

	// Verify Command.Cmd expansion (references internal variables)
	expectedCmd := "/opt/myapp/bin/server"
	assert.Equal(t, expectedCmd, runtimeCmd.ExpandedCmd, "Command.Cmd should reference internal variables correctly")

	// Verify Command.Args expansion (references internal variables)
	expectedArgs := []string{"--log", "/opt/myapp/logs/app.log"}
	assert.Equal(t, expectedArgs, runtimeCmd.ExpandedArgs, "Command.Args should reference internal variables correctly")

	// Also verify that raw values are preserved for debugging/auditing
	assert.Equal(t, "%{app_dir}/bin/server", cmdSpec.Cmd, "Raw Cmd should be preserved")
	assert.Equal(t, []string{"--log", "%{log_dir}/app.log"}, cmdSpec.Args, "Raw Args should be preserved")
	assert.Equal(t, []string{"LOG_DIR=%{log_dir}"}, cmdSpec.Env, "Raw Env should be preserved")
}

// TestLoadAndPrepareConfig_MissingConfigFile verifies error handling for missing config files
func TestLoadAndPrepareConfig_MissingConfigFile(t *testing.T) {
	// Setup: Create temporary directory for hash storage
	tempDir := t.TempDir()
	hashDir := filepath.Join(tempDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create verification manager
	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Try to load non-existent config file
	nonExistentPath := filepath.Join(tempDir, "nonexistent.toml")
	cfg, err := LoadAndPrepareConfig(verificationManager, nonExistentPath, "test-run-002")

	// Should return error
	assert.Error(t, err, "Should return error for non-existent config file")
	assert.Nil(t, cfg, "Config should be nil on error")
}

// TestLoadAndPrepareConfig_EmptyConfigPath verifies error handling for empty config path
func TestLoadAndPrepareConfig_EmptyConfigPath(t *testing.T) {
	// Setup: Create temporary directory for hash storage
	tempDir := t.TempDir()
	hashDir := filepath.Join(tempDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create verification manager
	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Try to load with empty config path
	cfg, err := LoadAndPrepareConfig(verificationManager, "", "test-run-003")

	// Should return error
	assert.Error(t, err, "Should return error for empty config path")
	assert.Nil(t, cfg, "Config should be nil on error")
	assert.Contains(t, err.Error(), "Config file path is required", "Error message should indicate required path")
}
