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

// TestVerifyFilesExpansionIntegration tests end-to-end config loading with verify_files expansion
func TestVerifyFilesExpansionIntegration(t *testing.T) {
	tests := []struct {
		name                   string
		configTOML             string
		setupEnv               func(*testing.T)
		expectedGlobalExpanded []string
		expectedGroup1Expanded []string
		expectedGroup2Expanded []string
		expectError            bool
		errorContains          string
	}{
		{
			name: "E2E config load with expansion",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"
  env_allowlist = ["HOME"]
  from_env = ["home=HOME"]
  verify_files = ["%{home}/global1.txt", "%{home}/global2.txt"]

[[groups]]
  name = "group1"
  verify_files = ["%{home}/group/file.txt"]
  [[groups.commands]]
    name = "cmd1"
    cmd = "echo"
    args = ["test"]
`,
			setupEnv: func(t *testing.T) {
				t.Setenv("HOME", "/home/testuser")
			},
			expectedGlobalExpanded: []string{"/home/testuser/global1.txt", "/home/testuser/global2.txt"},
			expectedGroup1Expanded: []string{"/home/testuser/group/file.txt"},
		},
		{
			name: "multiple groups with expansion",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"
  env_allowlist = ["BASE"]
  from_env = ["base=BASE"]
  verify_files = ["%{base}/global.txt"]

[[groups]]
  name = "group1"
  verify_files = ["%{base}/group1.txt"]
  [[groups.commands]]
    name = "cmd1"
    cmd = "echo"
    args = ["test"]

[[groups]]
  name = "group2"
  verify_files = ["%{base}/group2.txt"]
  [[groups.commands]]
    name = "cmd2"
    cmd = "echo"
    args = ["test"]
`,
			setupEnv: func(t *testing.T) {
				t.Setenv("BASE", "/opt")
			},
			expectedGlobalExpanded: []string{"/opt/global.txt"},
			expectedGroup1Expanded: []string{"/opt/group1.txt"},
			expectedGroup2Expanded: []string{"/opt/group2.txt"},
		},
		{
			name: "global and group combination",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"
  env_allowlist = ["GLOBAL_VAR"]
  from_env = ["global_var=GLOBAL_VAR"]
  verify_files = ["%{global_var}/config.toml"]

[[groups]]
  name = "testgroup"
  env_allowlist = ["GROUP_VAR"]
  from_env = ["group_var=GROUP_VAR"]
  verify_files = ["%{group_var}/data.txt"]
  [[groups.commands]]
    name = "cmd1"
    cmd = "echo"
    args = ["test"]
`,
			setupEnv: func(t *testing.T) {
				t.Setenv("GLOBAL_VAR", "/etc/app")
				t.Setenv("GROUP_VAR", "/var/lib/app")
			},
			expectedGlobalExpanded: []string{"/etc/app/config.toml"},
			expectedGroup1Expanded: []string{"/var/lib/app/data.txt"},
		},
		{
			name: "error stops config loading",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"
  env_allowlist = ["SAFE_VAR"]
  from_env = ["forbidden_var=FORBIDDEN_VAR"]
  verify_files = ["%{forbidden_var}/config.toml"]

[[groups]]
  name = "group1"
  [[groups.commands]]
    name = "cmd1"
    cmd = "echo"
    args = ["test"]
`,
			setupEnv: func(t *testing.T) {
				t.Setenv("FORBIDDEN_VAR", "/forbidden")
			},
			expectError:   true,
			errorContains: "not in allowlist",
		},
		{
			name: "actual file verification flow",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"
  env_allowlist = ["TEST_DIR"]
  from_env = ["test_dir=TEST_DIR"]
  verify_files = ["%{test_dir}/file1.txt", "%{test_dir}/file2.txt"]

[[groups]]
  name = "group1"
  verify_files = ["%{test_dir}/group_file.txt"]
  [[groups.commands]]
    name = "cmd1"
    cmd = "echo"
    args = ["test"]
`,
			setupEnv: func(t *testing.T) {
				t.Setenv("TEST_DIR", "/tmp/test")
			},
			expectedGlobalExpanded: []string{"/tmp/test/file1.txt", "/tmp/test/file2.txt"},
			expectedGroup1Expanded: []string{"/tmp/test/group_file.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			if tt.setupEnv != nil {
				tt.setupEnv(t)
			}

			// Load config
			loader := NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.configTOML))

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)

			// Verify global expanded verify_files
			if tt.expectedGlobalExpanded != nil {
				assert.Equal(t, tt.expectedGlobalExpanded, cfg.Global.ExpandedVerifyFiles)
			}

			// Verify group1 expanded verify_files
			if tt.expectedGroup1Expanded != nil && len(cfg.Groups) > 0 {
				assert.Equal(t, tt.expectedGroup1Expanded, cfg.Groups[0].ExpandedVerifyFiles)
			}

			// Verify group2 expanded verify_files
			if tt.expectedGroup2Expanded != nil && len(cfg.Groups) > 1 {
				assert.Equal(t, tt.expectedGroup2Expanded, cfg.Groups[1].ExpandedVerifyFiles)
			}
		})
	}
}

// ===========================================
// Integration Tests
// ===========================================

// TestLoader_GroupEnvIntegration tests the complete integration of Group.Env functionality
func TestLoader_GroupEnvIntegration(t *testing.T) {
	configPath := "testdata/group_env.toml"

	// Read file content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Load configuration
	loader := NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err)
	require.NotNil(t, cfg) // Verify Global.Env expansion
	expectedGlobalEnv := map[string]string{
		"BASE_DIR":  "/opt",
		"LOG_LEVEL": "info",
	}
	assert.Equal(t, expectedGlobalEnv, cfg.Global.ExpandedEnv)

	// Verify groups
	require.Len(t, cfg.Groups, 3)

	// Test inherit_group (inherits from global allowlist)
	inheritGroup := findGroupByName(cfg.Groups, "inherit_group")
	require.NotNil(t, inheritGroup)

	expectedInheritEnv := map[string]string{
		"APP_DIR": "/opt/app",
	}
	assert.Equal(t, expectedInheritEnv, inheritGroup.ExpandedEnv)

	expectedInheritVerifyFiles := []string{"/opt/app/verify.sh"}
	assert.Equal(t, expectedInheritVerifyFiles, inheritGroup.ExpandedVerifyFiles)

	// Test override_group (overrides global allowlist)
	overrideGroup := findGroupByName(cfg.Groups, "override_group")
	require.NotNil(t, overrideGroup)

	expectedOverrideEnv := map[string]string{
		"DATA_DIR": "/data",
	}
	assert.Equal(t, expectedOverrideEnv, overrideGroup.ExpandedEnv)

	expectedOverrideVerifyFiles := []string{"/data/verify.sh"}
	assert.Equal(t, expectedOverrideVerifyFiles, overrideGroup.ExpandedVerifyFiles)

	// Test reject_group (rejects all system environment variables)
	rejectGroup := findGroupByName(cfg.Groups, "reject_group")
	require.NotNil(t, rejectGroup)

	expectedRejectEnv := map[string]string{
		"STATIC_DIR": "/static",
	}
	assert.Equal(t, expectedRejectEnv, rejectGroup.ExpandedEnv)

	expectedRejectVerifyFiles := []string{"/static/verify.sh"}
	assert.Equal(t, expectedRejectVerifyFiles, rejectGroup.ExpandedVerifyFiles)
}

// TestLoader_GlobalGroupEnvExpansion verifies that Global.Env and Group.Env are expanded
// during config loading, while Command-level expansion is deferred to bootstrap.
func TestConfigLoaderEnvExpansionIntegration(t *testing.T) {
	configPath := "testdata/command_env_references_global_group.toml"

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
		"BASE_DIR": "/opt",
	}
	assert.Equal(t, expectedGlobalEnv, cfg.Global.ExpandedEnv)

	// Verify groups
	require.Len(t, cfg.Groups, 1)

	// Test app_group
	appGroup := findGroupByName(cfg.Groups, "app_group")
	require.NotNil(t, appGroup)

	// Verify Group.Env expansion (references Global.Env)
	expectedGroupEnv := map[string]string{
		"APP_DIR": "/opt/myapp",
	}
	assert.Equal(t, expectedGroupEnv, appGroup.ExpandedEnv)

	// Verify commands
	require.Len(t, appGroup.Commands, 1)
	cmd := &appGroup.Commands[0]
	require.Equal(t, "run_app", cmd.Name)

	// Note: Command expansion happens in config.LoadConfig().
	// At this stage, we verify that:
	// - Global.ExpandedEnv contains only global.env values
	// - Group.ExpandedEnv contains only group.env values
	// - Command.ExpandedEnv contains only command.env values
	// - Command.ExpandedCmd and ExpandedArgs are expanded
	// - Final environment merging happens at execution time via BuildProcessEnvironment
	assert.Equal(t, []string{"LOG_DIR=%{log_dir}"}, cmd.Env)
	assert.Equal(t, "%{app_dir}/bin/server", cmd.Cmd)
	assert.Equal(t, []string{"--log", "%{log_dir}/app.log"}, cmd.Args)

	// Command.ExpandedEnv should contain only command-level env values
	assert.NotNil(t, cmd.ExpandedEnv)
	assert.Contains(t, cmd.ExpandedEnv, "LOG_DIR")
	assert.Equal(t, "/opt/myapp/logs", cmd.ExpandedEnv["LOG_DIR"])
	// BASE_DIR and APP_DIR are in Global/Group ExpandedEnv, not merged into Command.ExpandedEnv
	assert.NotContains(t, cmd.ExpandedEnv, "BASE_DIR")
	assert.NotContains(t, cmd.ExpandedEnv, "APP_DIR")

	// Command.ExpandedCmd should be expanded
	assert.Equal(t, "/opt/myapp/bin/server", cmd.ExpandedCmd)

	// Command.ExpandedArgs should be expanded
	expectedArgs := []string{"--log", "/opt/myapp/logs/app.log"}
	assert.Equal(t, expectedArgs, cmd.ExpandedArgs)
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
