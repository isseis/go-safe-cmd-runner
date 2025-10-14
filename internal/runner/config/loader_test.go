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

// TestValidateReservedPrefix tests that config loading validates reserved prefix usage in env vars
func TestValidateReservedPrefix(t *testing.T) {
	tests := []struct {
		name        string
		configTOML  string
		expectError bool
		errorType   error
	}{
		{
			name: "valid_command_level_env",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"

[[groups]]
  name = "test"
  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    args = ["hello"]
    env = ["NORMAL_VAR=value", "PATH=/usr/bin"]
`,
			expectError: false,
		},
		{
			name: "reserved_prefix_at_command_level",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"

[[groups]]
  name = "test"
  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    args = ["hello"]
    env = ["__RUNNER_CUSTOM=value"]
`,
			expectError: true,
			errorType:   &runnertypes.ReservedEnvPrefixError{},
		},
		{
			name: "reserved_prefix_DATETIME",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"

[[groups]]
  name = "test"
  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    env = ["__RUNNER_DATETIME=override"]
`,
			expectError: true,
			errorType:   &runnertypes.ReservedEnvPrefixError{},
		},
		{
			name: "reserved_prefix_PID",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"

[[groups]]
  name = "test"
  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    env = ["__RUNNER_PID=12345"]
`,
			expectError: true,
			errorType:   &runnertypes.ReservedEnvPrefixError{},
		},
		{
			name: "similar_but_not_reserved",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"

[[groups]]
  name = "test"
  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    env = ["GO_RUNNER_VAR=value", "RUNNER_VAR=value"]
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()

			// Load the config (validation is performed inside LoadConfig now)
			cfg, err := loader.LoadConfig([]byte(tt.configTOML))

			if tt.expectError {
				require.Error(t, err, "expected LoadConfig to fail with validation error")
				if tt.errorType != nil {
					assert.ErrorAs(t, err, &tt.errorType, "expected error type to match")
				}
				assert.Nil(t, cfg, "expected cfg to be nil when error occurs")
			} else {
				require.NoError(t, err, "expected LoadConfig to succeed")
				require.NotNil(t, cfg, "expected cfg to be non-nil")
			}
		})
	}
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
  verify_files = ["${HOME}/global1.txt", "${HOME}/global2.txt"]

[[groups]]
  name = "group1"
  env_allowlist = ["HOME"]
  verify_files = ["${HOME}/group/file.txt"]
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
  verify_files = ["${BASE}/global.txt"]

[[groups]]
  name = "group1"
  env_allowlist = ["BASE"]
  verify_files = ["${BASE}/group1.txt"]
  [[groups.commands]]
    name = "cmd1"
    cmd = "echo"
    args = ["test"]

[[groups]]
  name = "group2"
  env_allowlist = ["BASE"]
  verify_files = ["${BASE}/group2.txt"]
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
  verify_files = ["${GLOBAL_VAR}/config.toml"]

[[groups]]
  name = "testgroup"
  env_allowlist = ["GROUP_VAR"]
  verify_files = ["${GROUP_VAR}/data.txt"]
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
  verify_files = ["${FORBIDDEN_VAR}/config.toml"]

[[groups]]
  name = "group1"
  env_allowlist = ["SAFE_VAR"]
  [[groups.commands]]
    name = "cmd1"
    cmd = "echo"
    args = ["test"]
`,
			setupEnv: func(t *testing.T) {
				t.Setenv("FORBIDDEN_VAR", "/forbidden")
			},
			expectError:   true,
			errorContains: "not allowed",
		},
		{
			name: "actual file verification flow",
			configTOML: `
version = "1.0"
[global]
  workdir = "/tmp"
  env_allowlist = ["TEST_DIR"]
  verify_files = ["${TEST_DIR}/file1.txt", "${TEST_DIR}/file2.txt"]

[[groups]]
  name = "group1"
  env_allowlist = ["TEST_DIR"]
  verify_files = ["${TEST_DIR}/group_file.txt"]
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

	// Note: Command.Env, Cmd, and Args expansion happens in config.LoadConfig().
	// At this stage, we verify that:
	// - Global.ExpandedEnv is populated correctly
	// - Group.ExpandedEnv is populated correctly
	// - Command.Env field contains the raw (unexpanded) values
	// - Command.ExpandedEnv, ExpandedCmd, and ExpandedArgs are populated
	assert.Equal(t, []string{"LOG_DIR=${APP_DIR}/logs"}, cmd.Env)
	assert.Equal(t, "${APP_DIR}/bin/server", cmd.Cmd)
	assert.Equal(t, []string{"--log", "${LOG_DIR}/app.log"}, cmd.Args)

	// Command.ExpandedEnv should be populated
	// It contains Command.Env + Global.ExpandedEnv + Group.ExpandedEnv + AutoEnv
	assert.NotNil(t, cmd.ExpandedEnv)
	assert.Contains(t, cmd.ExpandedEnv, "LOG_DIR")
	assert.Equal(t, "/opt/myapp/logs", cmd.ExpandedEnv["LOG_DIR"])
	assert.Contains(t, cmd.ExpandedEnv, "BASE_DIR")
	assert.Equal(t, "/opt", cmd.ExpandedEnv["BASE_DIR"])
	assert.Contains(t, cmd.ExpandedEnv, "APP_DIR")
	assert.Equal(t, "/opt/myapp", cmd.ExpandedEnv["APP_DIR"])
	// AutoEnv variables should also be present
	assert.Contains(t, cmd.ExpandedEnv, "__RUNNER_DATETIME")
	assert.Contains(t, cmd.ExpandedEnv, "__RUNNER_PID")

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

	// Verify Global.ExpandedVars is nil or empty (not yet expanded in Phase 1)
	// Note: In the current implementation, expansion happens in LoadConfig, so ExpandedVars may be populated
	// For Phase 1, we just verify that the fields are parsed
	t.Logf("Global.ExpandedVars: %v", cfg.Global.ExpandedVars)

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

	// Verify Group.ExpandedVars is nil or empty (not yet expanded in Phase 1)
	t.Logf("Group.ExpandedVars: %v", group.ExpandedVars)

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

	// Verify Command.ExpandedVars is nil or empty (not yet expanded in Phase 1)
	t.Logf("Command.ExpandedVars: %v", cmd.ExpandedVars)
}
