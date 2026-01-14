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
	loader := NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest([]byte(configContent))
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

// TestDefaultTimeout tests that RuntimeGlobal.Timeout() returns DefaultTimeout when not specified in config
func TestDefaultTimeout(t *testing.T) {
	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_command"
cmd = "/bin/echo"
args = ["test"]
`

	loader := NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest([]byte(configContent))
	require.NoError(t, err, "LoadConfig failed")
	require.NotNil(t, cfg)

	// Verify that ConfigSpec.Global.Timeout is unset (not set in TOML)
	assert.Nil(t, cfg.Global.Timeout, "Expected ConfigSpec.Global.Timeout to be nil when not specified in TOML")

	// Create RuntimeGlobal and verify timeout is unset
	runtimeGlobal, err := runnertypes.NewRuntimeGlobal(&cfg.Global)
	require.NoError(t, err, "NewRuntimeGlobal failed")
	timeout := runtimeGlobal.Timeout()
	assert.False(t, timeout.IsSet(), "Expected RuntimeGlobal.Timeout() to be unset")
	// When unset, caller should use DefaultTimeout (see common.DefaultTimeout)
}

// TestExplicitTimeoutNotOverridden tests that explicitly set timeout is preserved
func TestExplicitTimeoutNotOverridden(t *testing.T) {
	configContent := `
[global]
timeout = 120

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_command"
cmd = "/bin/echo"
args = ["test"]
`

	loader := NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest([]byte(configContent))
	require.NoError(t, err, "LoadConfig failed")
	require.NotNil(t, cfg)

	// Verify explicit timeout is preserved in ConfigSpec
	require.NotNil(t, cfg.Global.Timeout, "Expected ConfigSpec.Global.Timeout to be non-nil")
	assert.Equal(t, int32(120), *cfg.Global.Timeout, "Expected ConfigSpec.Global.Timeout to preserve explicit value")

	// Create RuntimeGlobal and verify explicit timeout is returned
	runtimeGlobal, err := runnertypes.NewRuntimeGlobal(&cfg.Global)
	require.NoError(t, err, "NewRuntimeGlobal failed")
	timeout := runtimeGlobal.Timeout()
	assert.True(t, timeout.IsSet(), "Expected RuntimeGlobal.Timeout() to be set")
	assert.Equal(t, int32(120), timeout.Value(), "Expected RuntimeGlobal.Timeout().Value() to return explicit value")
}

// TestBasicTOMLParse tests basic TOML parsing for Global.EnvVars and Group.EnvVars
func TestBasicTOMLParse(t *testing.T) {
	configContent := `
version = "1.0"

[global]
timeout = 300
env_vars = ["VAR1=value1", "VAR2=value2"]

[[groups]]
name = "test_group"
env_vars = ["GROUP_VAR=group_value"]

[[groups.commands]]
name = "test_command"
cmd = "/bin/echo"
args = ["test"]
`

	loader := NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest([]byte(configContent))
	require.NoError(t, err, "LoadConfig failed")
	require.NotNil(t, cfg)

	// Verify Global.EnvVars is parsed correctly
	assert.Equal(t, []string{"VAR1=value1", "VAR2=value2"}, cfg.Global.EnvVars)
}

// ===========================================
// Integration Tests
// ===========================================

// TestLoader_GroupEnvIntegration tests basic Group.EnvVars loading from a TOML file
// Note: Detailed allowlist scenarios are covered in loader_e2e_test.go::TestE2E_AllowlistScenarios
func TestLoader_GroupEnvIntegration(t *testing.T) {
	configPath := "testdata/group_env.toml"

	// Read file content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Load configuration
	loader := NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest(content)
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

// =================================================================
// TOML Parse Test for FromEnv/Vars (Variable Expansion Foundation)
// =================================================================

// TestTOML_ParseFromEnvAndVars tests that FromEnv and Vars fields are correctly parsed from TOML
func TestTOML_ParseFromEnvAndVars(t *testing.T) {
	t.Skip("Skipping - integration test covers this functionality")

	configPath := "testdata/phase1_basic_vars.toml"

	// Read file content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read test config file")

	// Load configuration
	loader := NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest(content)
	require.NoError(t, err, "LoadConfig failed")
	require.NotNil(t, cfg, "Config should not be nil")

	// Verify Global.EnvImport is parsed correctly
	expectedGlobalFromEnv := []string{"home=HOME", "path=PATH"}
	assert.Equal(t, expectedGlobalFromEnv, cfg.Global.EnvImport, "Global.EnvImport should be parsed correctly")

	// Verify Global.Vars is parsed correctly
	expectedGlobalVars := []string{"app_dir=/opt/myapp"}
	assert.Equal(t, expectedGlobalVars, cfg.Global.Vars, "Global.Vars should be parsed correctly")

	// Verify Global.EnvVars is parsed correctly
	expectedGlobalEnv := []string{"BASE_DIR=%{app_dir}"}
	assert.Equal(t, expectedGlobalEnv, cfg.Global.EnvVars, "Global.EnvVars should be parsed correctly")

	// Verify groups
	require.Len(t, cfg.Groups, 1, "Expected 1 group")

	group := &cfg.Groups[0]
	assert.Equal(t, "test_group", group.Name, "Group name should be 'test_group'")

	// Verify Group.EnvImport is not set (should be nil, inheriting from Global)
	assert.Nil(t, group.EnvImport, "Group.EnvImport should be nil (inheriting from Global)")

	// Verify Group.Vars is parsed correctly
	expectedGroupVars := []string{"log_dir=%{app_dir}/logs"}
	assert.Equal(t, expectedGroupVars, group.Vars, "Group.Vars should be parsed correctly")

	// Verify Group.EnvVars is parsed correctly
	expectedGroupEnv := []string{"LOG_DIR=%{log_dir}"}
	assert.Equal(t, expectedGroupEnv, group.EnvVars, "Group.EnvVars should be parsed correctly")

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

// TestVariableExpansionIntegration tests the full integration of variable expansion in the config loader
func TestVariableExpansionIntegration(t *testing.T) {
	t.Skip("Skipping - expansion not yet implemented in loader")
}

// TestFromEnvMergeIntegration verifies that from_env is merged between Global and Group levels
func TestFromEnvMergeIntegration(t *testing.T) {
	t.Skip("Skipping - expansion not yet implemented in loader")
}

// TestLoadConfig_NegativeTimeoutValidation tests that LoadConfig rejects negative timeouts
func TestLoadConfig_NegativeTimeoutValidation(t *testing.T) {
	tests := []struct {
		name        string
		configToml  string
		expectError bool
	}{
		{
			name: "negative global timeout",
			configToml: `
version = "1.0"

[global]
  timeout = -10

[[groups]]
  name = "test"

  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    args = ["hello"]
`,
			expectError: true,
		},
		{
			name: "negative command timeout",
			configToml: `
version = "1.0"

[[groups]]
  name = "test"

  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    args = ["hello"]
    timeout = -5
`,
			expectError: true,
		},
		{
			name: "valid zero timeout",
			configToml: `
version = "1.0"

[global]
  timeout = 0

[[groups]]
  name = "test"

  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    args = ["hello"]
`,
			expectError: false,
		},
		{
			name: "valid positive timeout",
			configToml: `
version = "1.0"

[global]
  timeout = 30

[[groups]]
  name = "test"

  [[groups.commands]]
    name = "test_cmd"
    cmd = "echo"
    args = ["hello"]
    timeout = 60
`,
			expectError: false,
		},
		{
			name: "negative template timeout",
			configToml: `
version = "1.0"

[command_templates.bad_template]
  cmd = "echo"
  args = ["test"]
  timeout = -15

[[groups]]
  name = "test"

  [[groups.commands]]
    name = "test_cmd"
    template = "bad_template"
    [groups.commands.params]
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoaderForTest()
			cfg, err := loader.LoadConfigForTest([]byte(tt.configToml))

			if tt.expectError {
				require.Error(t, err, "expected error but got none")
				require.ErrorIs(t, err, ErrNegativeTimeout, "error should be ErrNegativeTimeout")
				assert.Nil(t, cfg, "config should be nil when validation fails")
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
				require.NotNil(t, cfg, "config should not be nil")
			}
		})
	}
}

func TestLoaderWithTemplates(t *testing.T) {
	tests := []struct {
		name        string
		toml        string
		wantErr     bool
		wantErrType error
	}{
		{
			name: "valid template",
			toml: `
version = "1.0"
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]

[[groups]]
name = "backup"
[[groups.commands]]
name = "backup_data"
template = "restic_backup"
[groups.commands.params]
path = "/data"
`,
			wantErr: false,
		},
		{
			name: "duplicate template name",
			toml: `
version = "1.0"
[command_templates.duplicate]
cmd = "echo"
args = ["first"]
[command_templates.duplicate]
cmd = "ls"
`,
			wantErr:     true,
			wantErrType: &ErrDuplicateTemplateName{},
		},
		{
			name: "forbidden %{ in template cmd",
			toml: `
version = "1.0"
[command_templates.bad]
cmd = "%{var}"
`,
			wantErr:     true,
			wantErrType: &ErrForbiddenPatternInTemplate{},
		},
		{
			name: "forbidden %{ in template args",
			toml: `
version = "1.0"
[command_templates.bad]
cmd = "echo"
args = ["%{var}", "hello"]
`,
			wantErr:     true,
			wantErrType: &ErrForbiddenPatternInTemplate{},
		},
		{
			name: "forbidden %{ in template env",
			toml: `
version = "1.0"
[command_templates.bad]
cmd = "echo"
env_vars = ["VAR=%{value}"]
`,
			wantErr:     true,
			wantErrType: &ErrForbiddenPatternInTemplate{},
		},
		{
			name: "forbidden %{ in template workdir",
			toml: `
version = "1.0"
[command_templates.bad]
cmd = "echo"
workdir = "%{dir}"
`,
			wantErr:     true,
			wantErrType: &ErrForbiddenPatternInTemplate{},
		},
		{
			name: "missing cmd field",
			toml: `
version = "1.0"
[command_templates.no_cmd]
args = ["backup"]
`,
			wantErr:     true,
			wantErrType: &ErrMissingRequiredField{},
		},
		{
			name: "invalid template name",
			toml: `
version = "1.0"
[command_templates."123invalid"]
cmd = "echo"
`,
			wantErr:     true,
			wantErrType: &ErrInvalidTemplateName{},
		},
		{
			name: "reserved template name prefix",
			toml: `
version = "1.0"
[command_templates.__reserved]
cmd = "echo"
`,
			wantErr:     true,
			wantErrType: &ErrReservedTemplateName{},
		},
		{
			name: "template with name field",
			toml: `
version = "1.0"
[command_templates.bad_template]
name = "should_not_be_here"
cmd = "echo"
`,
			wantErr:     true,
			wantErrType: &ErrTemplateContainsNameField{},
		},
		{
			name: "valid template with placeholders",
			toml: `
version = "1.0"
[command_templates.restic_advanced]
cmd = "restic"
args = ["${@flags}", "backup", "${path}", "${?optional}"]
env_vars = ["RESTIC_REPO=${repo}"]
workdir = "${workdir}"

[[groups]]
name = "backup"
[[groups.commands]]
name = "backup_home"
template = "restic_advanced"
[groups.commands.params]
flags = ["-v", "--exclude-caches"]
path = "/home"
repo = "/backup/repo"
workdir = "/tmp"
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoaderForTest()
			cfg, err := loader.LoadConfigForTest([]byte(tt.toml))

			if tt.wantErr {
				require.Error(t, err, "expected error but got none")
				if tt.wantErrType != nil {
					require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
				assert.Nil(t, cfg, "config should be nil when validation fails")
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
				require.NotNil(t, cfg, "config should not be nil")
			}
		})
	}
}
