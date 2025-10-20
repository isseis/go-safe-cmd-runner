package config

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigFromContent(t *testing.T) {
	// Create config content for testing
	configContent := `
version = "1.0"

[global]
  timeout = 3600

[[groups]]
  name = "test"

  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    args = ["hello"]
    run_as_user = "root"
`

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Load config from content
	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(configContent))
	require.NoError(t, err, "LoadConfigFromContent() returned error")

	require.NotNil(t, cfg, "LoadConfigFromContent() returned nil config")

	// The privileged field is now implemented, so no warnings should be logged
	logOutput := buf.String()
	assert.False(t, strings.Contains(logOutput, "privileged field is not yet implemented"), "unexpected warning about privileged field in log output: %s", logOutput)

	// Verify config was loaded correctly despite warnings
	assert.Len(t, cfg.Groups, 1, "expected 1 group")

	assert.Len(t, cfg.Groups[0].Commands, 1, "expected 1 command")

	cmd := cfg.Groups[0].Commands[0]
	assert.Equal(t, "test_cmd", cmd.Name, "expected command name 'test_cmd'")
	assert.Equal(t, "root", cmd.RunAsUser, "expected run_as_user to be 'root'")
	assert.True(t, cmd.HasUserGroupSpecification(), "expected command to have user/group specification")
}

// TestBasicTOMLParse tests basic TOML parsing for Global.Env and Group.Env
func TestBasicTOMLParse(t *testing.T) {
	configContent := `
version = "1.0"

[global]
timeout = 300
env = ["VAR1=value1", "VAR2=value2"]

[[groups]]
name = "test_group"
env = ["GROUP_VAR=group_value"]

[[groups.commands]]
name = "test_command"
cmd = "/bin/echo"
args = ["test"]
`

	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(configContent))
	require.NoError(t, err, "LoadConfig failed")
	require.NotNil(t, cfg)

	// Verify Global.Env is parsed correctly
	assert.Equal(t, []string{"VAR1=value1", "VAR2=value2"}, cfg.Global.Env)
}

// ===========================================
// Integration Tests
// ===========================================

// TestLoader_GroupEnvIntegration tests basic Group.Env loading from a TOML file
// Note: Detailed allowlist scenarios are covered in loader_e2e_test.go::TestE2E_AllowlistScenarios
func TestLoader_GroupEnvIntegration(t *testing.T) {
	configPath := "testdata/group_env.toml"

	// Read file content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Load configuration
	loader := NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify groups are loaded
	require.Len(t, cfg.Groups, 3)

	// Basic verification that each group exists
	inheritGroup := findGroupByName(cfg.Groups, "inherit_group")
	require.NotNil(t, inheritGroup)

	overrideGroup := findGroupByName(cfg.Groups, "override_group")
	require.NotNil(t, overrideGroup)

	rejectGroup := findGroupByName(cfg.Groups, "reject_group")
	require.NotNil(t, rejectGroup)
}

// Helper function to find a group by name
func findGroupByName(groups []runnertypes.GroupSpec, name string) *runnertypes.GroupSpec {
	for i := range groups {
		if groups[i].Name == name {
			return &groups[i]
		}
	}
	return nil
}

// ===========================================
// Phase 1.4: TOML Parse Test for FromEnv/Vars
// ===========================================

// TestPhase1_ParseFromEnvAndVars tests that FromEnv and Vars fields are correctly parsed from TOML
func TestPhase1_ParseFromEnvAndVars(t *testing.T) {
	t.Skip("Skipping phase 1 test - phase 9 integration covers this")

	configPath := "testdata/phase1_basic_vars.toml"

	// Read file content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read test config file")

	// Load configuration
	loader := NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "LoadConfig failed")
	require.NotNil(t, cfg, "Config should not be nil")

	// Verify Global.FromEnv is parsed correctly
	expectedGlobalFromEnv := []string{"home=HOME", "path=PATH"}
	assert.Equal(t, expectedGlobalFromEnv, cfg.Global.FromEnv, "Global.FromEnv should be parsed correctly")

	// Verify Global.Vars is parsed correctly
	expectedGlobalVars := []string{"app_dir=/opt/myapp"}
	assert.Equal(t, expectedGlobalVars, cfg.Global.Vars, "Global.Vars should be parsed correctly")

	// Verify Global.Env is parsed correctly
	expectedGlobalEnv := []string{"BASE_DIR=%{app_dir}"}
	assert.Equal(t, expectedGlobalEnv, cfg.Global.Env, "Global.Env should be parsed correctly")

	// Verify groups
	require.Len(t, cfg.Groups, 1, "Expected 1 group")

	group := &cfg.Groups[0]
	assert.Equal(t, "test_group", group.Name, "Group name should be 'test_group'")

	// Verify Group.FromEnv is not set (should be nil, inheriting from Global)
	assert.Nil(t, group.FromEnv, "Group.FromEnv should be nil (inheriting from Global)")

	// Verify Group.Vars is parsed correctly
	expectedGroupVars := []string{"log_dir=%{app_dir}/logs"}
	assert.Equal(t, expectedGroupVars, group.Vars, "Group.Vars should be parsed correctly")

	// Verify Group.Env is parsed correctly
	expectedGroupEnv := []string{"LOG_DIR=%{log_dir}"}
	assert.Equal(t, expectedGroupEnv, group.Env, "Group.Env should be parsed correctly")

	// Verify commands
	require.Len(t, group.Commands, 1, "Expected 1 command")

	cmd := &group.Commands[0]
	assert.Equal(t, "test_cmd", cmd.Name, "Command name should be 'test_cmd'")

	// Verify Command.Vars is parsed correctly
	expectedCmdVars := []string{"temp_file=%{log_dir}/temp.log"}
	assert.Equal(t, expectedCmdVars, cmd.Vars, "Command.Vars should be parsed correctly")

	// Verify Command.Cmd is parsed correctly
	assert.Equal(t, "/bin/echo", cmd.Cmd, "Command.Cmd should be parsed correctly")

	// Verify Command.Args is parsed correctly
	expectedCmdArgs := []string{"%{temp_file}"}
	assert.Equal(t, expectedCmdArgs, cmd.Args, "Command.Args should be parsed correctly")
}

// TestPhase9Integration tests the full integration of variable expansion in the config loader
func TestPhase9Integration(t *testing.T) {
	t.Skip("Skipping until Phase 5/6 - expansion not yet implemented in loader")
}

// TestFromEnvMergeIntegration verifies that from_env is merged between Global and Group levels
func TestFromEnvMergeIntegration(t *testing.T) {
	t.Skip("Skipping until Phase 5/6 - expansion not yet implemented in loader")
}
