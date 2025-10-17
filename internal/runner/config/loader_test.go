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
  workdir = "/tmp"

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
	// ExpandedEnv should now be populated after Config Loader's automatic expansion
	require.NotNil(t, cfg.Global.ExpandedEnv, "Global.ExpandedEnv should be populated after loading")
	assert.Equal(t, "value1", cfg.Global.ExpandedEnv["VAR1"])
	assert.Equal(t, "value2", cfg.Global.ExpandedEnv["VAR2"])

	// Verify Group.Env is parsed correctly
	require.Len(t, cfg.Groups, 1)
	assert.Equal(t, []string{"GROUP_VAR=group_value"}, cfg.Groups[0].Env)
	// Group.ExpandedEnv should be populated after configuration loading
	require.NotNil(t, cfg.Groups[0].ExpandedEnv, "Group.ExpandedEnv should be populated after configuration loading")
	assert.Equal(t, "group_value", cfg.Groups[0].ExpandedEnv["GROUP_VAR"])
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

	// Verify Global.Env expansion
	expectedGlobalEnv := map[string]string{
		"BASE_DIR":  "/opt",
		"LOG_LEVEL": "info",
	}
	assert.Equal(t, expectedGlobalEnv, cfg.Global.ExpandedEnv)

	// Verify groups are loaded
	require.Len(t, cfg.Groups, 3)

	// Basic verification that each group has expected fields populated
	inheritGroup := findGroupByName(cfg.Groups, "inherit_group")
	require.NotNil(t, inheritGroup)
	assert.NotNil(t, inheritGroup.ExpandedEnv)
	assert.NotEmpty(t, inheritGroup.ExpandedVerifyFiles)

	overrideGroup := findGroupByName(cfg.Groups, "override_group")
	require.NotNil(t, overrideGroup)
	assert.NotNil(t, overrideGroup.ExpandedEnv)

	rejectGroup := findGroupByName(cfg.Groups, "reject_group")
	require.NotNil(t, rejectGroup)
	assert.NotNil(t, rejectGroup.ExpandedEnv)
}

// Helper function to find a group by name
func findGroupByName(groups []runnertypes.CommandGroup, name string) *runnertypes.CommandGroup {
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

	// Verify Global.ExpandedVars is empty as it is not populated in Phase 1.
	assert.Empty(t, cfg.Global.ExpandedVars, "Global.ExpandedVars should be empty in Phase 1")

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

	// Verify Group.ExpandedVars is empty as it is not populated in Phase 1.
	assert.Empty(t, group.ExpandedVars, "Group.ExpandedVars should be empty in Phase 1")

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

	// Verify Command.ExpandedVars is empty as it is not populated in Phase 1.
	assert.Empty(t, cmd.ExpandedVars, "Command.ExpandedVars should be empty in Phase 1")
}

// TestPhase9Integration tests the full integration of variable expansion in the config loader
func TestPhase9Integration(t *testing.T) {
	// Set required environment variables for the test using t.Setenv for automatic
	// cleanup when the test completes.
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("PATH", "/usr/bin:/bin")

	// Read test configuration file
	content, err := os.ReadFile("testdata/phase9_integration.toml")
	require.NoError(t, err, "Failed to read phase9_integration.toml")

	// Load configuration
	loader := NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "LoadConfig should succeed")

	// Verify Global.ExpandedVars
	require.NotNil(t, cfg.Global.ExpandedVars, "Global.ExpandedVars should not be nil")
	assert.Equal(t, "/home/testuser", cfg.Global.ExpandedVars["home"], "home should be /home/testuser")
	assert.Equal(t, "/usr/bin:/bin", cfg.Global.ExpandedVars["system_path"], "system_path should be /usr/bin:/bin")
	assert.Equal(t, "myapp", cfg.Global.ExpandedVars["app_name"], "app_name should be myapp")
	assert.Equal(t, "/home/testuser/myapp", cfg.Global.ExpandedVars["app_dir"], "app_dir should be /home/testuser/myapp")
	assert.Equal(t, "/home/testuser/myapp/data", cfg.Global.ExpandedVars["data_dir"], "data_dir should be /home/testuser/myapp/data")

	// Verify Global.ExpandedEnv
	require.NotNil(t, cfg.Global.ExpandedEnv, "Global.ExpandedEnv should not be nil")
	assert.Equal(t, "/home/testuser/myapp", cfg.Global.ExpandedEnv["APP_DIR"], "APP_DIR should be /home/testuser/myapp")

	// Verify Global.ExpandedVerifyFiles
	require.Len(t, cfg.Global.ExpandedVerifyFiles, 1, "Should have 1 expanded verify file")
	assert.Equal(t, "/home/testuser/myapp/verify.sh", cfg.Global.ExpandedVerifyFiles[0], "verify_files should be expanded")

	// Verify Group.ExpandedVars (should inherit from Global and merge with group vars)
	require.Len(t, cfg.Groups, 1, "Should have 1 group")
	group := &cfg.Groups[0]
	require.NotNil(t, group.ExpandedVars, "Group.ExpandedVars should not be nil")

	// Check inherited variables from Global
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"], "home should be inherited")
	assert.Equal(t, "myapp", group.ExpandedVars["app_name"], "app_name should be inherited")
	assert.Equal(t, "/home/testuser/myapp", group.ExpandedVars["app_dir"], "app_dir should be inherited")
	assert.Equal(t, "/home/testuser/myapp/data", group.ExpandedVars["data_dir"], "data_dir should be inherited")

	// Check group-level variables
	assert.Equal(t, "/home/testuser/myapp/data/input", group.ExpandedVars["input_dir"], "input_dir should be expanded")
	assert.Equal(t, "/home/testuser/myapp/data/output", group.ExpandedVars["output_dir"], "output_dir should be expanded")

	// Verify Group.ExpandedEnv
	require.NotNil(t, group.ExpandedEnv, "Group.ExpandedEnv should not be nil")
	assert.Equal(t, "/home/testuser/myapp/data/input", group.ExpandedEnv["INPUT_DIR"], "INPUT_DIR should be expanded")

	// Verify Command.ExpandedVars
	require.Len(t, group.Commands, 1, "Should have 1 command")
	cmd := &group.Commands[0]
	require.NotNil(t, cmd.ExpandedVars, "Command.ExpandedVars should not be nil")

	// Check inherited variables
	assert.Equal(t, "/home/testuser/myapp/data/input", cmd.ExpandedVars["input_dir"], "input_dir should be inherited")

	// Check command-level variables
	assert.Equal(t, "/home/testuser/myapp/data/input/temp", cmd.ExpandedVars["temp_dir"], "temp_dir should be expanded")

	// Verify Command.ExpandedEnv
	require.NotNil(t, cmd.ExpandedEnv, "Command.ExpandedEnv should not be nil")
	assert.Equal(t, "/home/testuser/myapp/data/input/temp", cmd.ExpandedEnv["TEMP_DIR"], "TEMP_DIR should be expanded")

	// Verify Command.ExpandedCmd
	assert.Equal(t, "/usr/bin/process", cmd.ExpandedCmd, "cmd should be expanded")

	// Verify Command.ExpandedArgs
	require.Len(t, cmd.ExpandedArgs, 4, "Should have 4 expanded args")
	assert.Equal(t, "--input", cmd.ExpandedArgs[0])
	assert.Equal(t, "/home/testuser/myapp/data/input", cmd.ExpandedArgs[1], "arg should be expanded")
	assert.Equal(t, "--temp", cmd.ExpandedArgs[2])
	assert.Equal(t, "/home/testuser/myapp/data/input/temp", cmd.ExpandedArgs[3], "arg should be expanded")
}

// TestFromEnvMergeIntegration verifies that from_env is merged between Global and Group levels
func TestFromEnvMergeIntegration(t *testing.T) {
	// Set up system environment variables
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("USER", "testuser")
	t.Setenv("PATH", "/usr/bin:/bin")

	// Read and parse the test configuration that exercises from_env merge behavior
	configBytes, err := os.ReadFile("testdata/from_env_merge_test.toml")
	require.NoError(t, err, "Should read test data file")

	loader := NewLoader()
	cfg, err := loader.LoadConfig(configBytes)
	require.NoError(t, err, "Should load config without errors")
	require.NotNil(t, cfg, "Config should not be nil")

	// Verify Global-level from_env expansion
	require.NotNil(t, cfg.Global.ExpandedVars, "Global.ExpandedVars should be set")
	assert.Equal(t, "/home/testuser", cfg.Global.ExpandedVars["home"], "Global: home should be from HOME env var")
	assert.Equal(t, "testuser", cfg.Global.ExpandedVars["user"], "Global: user should be from USER env var")

	// Verify Group-level from_env merge: should have Global's variables + Group's new variables
	require.Len(t, cfg.Groups, 1, "Should have one group")
	group := cfg.Groups[0]
	assert.Equal(t, "merge_group", group.Name)

	require.NotNil(t, group.ExpandedVars, "Group.ExpandedVars should be set")

	// These should be inherited from Global.from_env
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"], "Group should inherit home from Global.from_env")
	assert.Equal(t, "testuser", group.ExpandedVars["user"], "Group should inherit user from Global.from_env")

	// This should be from Group.from_env
	assert.Equal(t, "/usr/bin:/bin", group.ExpandedVars["path"], "Group should have path from Group.from_env")

	// Verify that vars can reference all merged from_env variables
	assert.Equal(t, "/home/testuser/app", group.ExpandedVars["base_dir"], "base_dir should reference home from Global.from_env")
	assert.Equal(t, "/home/testuser/app/logs", group.ExpandedVars["log_dir"], "log_dir should reference base_dir")
	expectedCombined := "/home/testuser:testuser:/usr/bin:/bin"
	assert.Equal(t, expectedCombined, group.ExpandedVars["combined_env"], "combined_env should use all merged variables")

	// Verify Group.ExpandedEnv uses all merged variables
	require.NotNil(t, group.ExpandedEnv, "Group.ExpandedEnv should be set")
	assert.Equal(t, "/home/testuser", group.ExpandedEnv["HOME_VAR"], "HOME_VAR should be expanded with home")
	assert.Equal(t, "testuser", group.ExpandedEnv["USER_VAR"], "USER_VAR should be expanded with user")
	assert.Equal(t, "/usr/bin:/bin", group.ExpandedEnv["PATH_VAR"], "PATH_VAR should be expanded with path")
	assert.Equal(t, expectedCombined, group.ExpandedEnv["COMBINED"], "COMBINED should use merged variables")

	// Verify command-level expansion works with merged variables
	require.Len(t, group.Commands, 1, "Group should have one command")
	cmd := group.Commands[0]
	assert.Equal(t, "verify_merge", cmd.Name)
	require.NotNil(t, cmd.ExpandedArgs, "Command.ExpandedArgs should be set")
	assert.Equal(t, "Test merge: HOME=/home/testuser, USER=testuser, PATH=/usr/bin:/bin", cmd.ExpandedArgs[0], "Command args should reference all merged variables")
}
