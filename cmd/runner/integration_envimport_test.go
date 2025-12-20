package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
)

// TestRunner_GroupLevelEnvImport tests group-level env_import functionality
func TestRunner_GroupLevelEnvImport(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
		wantErr    bool
	}{
		{
			name: "group_env_import_basic",
			systemEnv: map[string]string{
				"PATH": "/usr/bin:/bin",
				"HOME": "/home/testuser",
			},
			configTOML: `
[global]
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "test_group"
env_import = ["group_path=PATH"]

[groups.vars]
log_dir = "%{group_path}/logs"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["%{log_dir}"]
`,
			expectVars: map[string]string{
				"PATH": "/usr/bin:/bin",  // system env preserved
				"HOME": "/home/testuser", // system env preserved
			},
		},
		{
			name: "group_env_import_with_var_reference",
			systemEnv: map[string]string{
				"PATH": "/usr/local/bin:/usr/bin",
			},
			configTOML: `
[global]
env_allowed = ["PATH"]

[[groups]]
name = "test_group"
env_import = ["grp_path=PATH"]
env_vars = ["CUSTOM_PATH=%{bin_dir}"]

[groups.vars]
bin_dir = "%{grp_path}"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"CUSTOM_PATH": "/usr/local/bin:/usr/bin",
			},
		},
		{
			name: "group_env_import_multiple_vars",
			systemEnv: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/test",
				"USER": "testuser",
			},
			configTOML: `
[global]
env_allowed = ["PATH", "HOME", "USER"]

[[groups]]
name = "test_group"
env_import = ["grp_path=PATH", "grp_home=HOME", "grp_user=USER"]
env_vars = ["CONFIG=%{config_file}"]

[groups.vars]
config_file = "%{grp_home}/.config"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["%{grp_user}"]
`,
			expectVars: map[string]string{
				"CONFIG": "/home/test/.config",
			},
		},
		{
			name: "group_env_import_overrides_global_vars",
			systemEnv: map[string]string{
				"PATH": "/group/path",
			},
			configTOML: `
[global]
env_allowed = ["PATH"]

[global.vars]
Mypath = "/global/path"

[[groups]]
name = "test_group"
env_import = ["mypath=PATH"]  # Override global var with system env
env_vars = ["RESULT=%{mypath}"]

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"RESULT": "/group/path", // Should use env_import value, not global var
			},
		},
		{
			name: "group_env_import_not_in_allowlist",
			systemEnv: map[string]string{
				"PATH": "/usr/bin",
				"USER": "testuser",
			},
			configTOML: `
[global]
env_allowed = ["PATH"]  # USER not allowed

[[groups]]
name = "test_group"
env_import = ["grp_user=USER"]  # Should fail - USER not in allowlist

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["test"]
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				// Test error case
				cfg := configSetupHelper(t, tt.systemEnv, tt.configTOML)
				runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
				require.NoError(t, err, "should expand global config without error")

				_, err = config.ExpandGroup(&cfg.Groups[0], runtimeGlobal)
				assert.Error(t, err, "should fail when env_import var not in allowlist")
			} else {
				// Test success case
				envPriorityTestHelper(t, tt.systemEnv, tt.configTOML, tt.expectVars)
			}
		})
	}
}

// TestRunner_CommandLevelEnvImport tests command-level env_import functionality
func TestRunner_CommandLevelEnvImport(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
		wantErr    bool
	}{
		{
			name: "command_env_import_basic",
			systemEnv: map[string]string{
				"HOME": "/home/testuser",
			},
			configTOML: `
[global]
env_allowed = ["HOME"]

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
env_import = ["cmd_home=HOME"]
cmd = "echo"
args = ["%{output_file}"]
env_vars = ["OUTPUT=%{output_file}"]

[groups.commands.vars]
output_file = "%{cmd_home}/output.txt"
`,
			expectVars: map[string]string{
				"OUTPUT": "/home/testuser/output.txt",
			},
		},
		{
			name: "command_env_import_overrides_group_vars",
			systemEnv: map[string]string{
				"USER": "cmduser",
			},
			configTOML: `
[global]
env_allowed = ["USER"]

[[groups]]
name = "test_group"

[groups.vars]
username = "groupuser"

[[groups.commands]]
name = "test_cmd"
env_import = ["username=USER"]  # Override group var
env_vars = ["RESULT=%{username}"]
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"RESULT": "cmduser", // Should use command-level env_import
			},
		},
		{
			name: "command_env_import_multiple",
			systemEnv: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/test",
			},
			configTOML: `
[global]
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
env_import = ["cmd_path=PATH", "cmd_home=HOME"]
env_vars = ["FULL_PATH=%{full_path}"]
cmd = "echo"
args = ["test"]

[groups.commands.vars]
full_path = "%{cmd_path}:%{cmd_home}/bin"
`,
			expectVars: map[string]string{
				"FULL_PATH": "/usr/bin:/home/test/bin",
			},
		},
		{
			name: "command_env_import_with_global_and_group_vars",
			systemEnv: map[string]string{
				"USER": "testuser",
			},
			configTOML: `
[global]
env_allowed = ["USER"]

[global.vars]
GlobalPrefix = "/opt"

[[groups]]
name = "test_group"

[groups.vars]
group_suffix = "data"

[[groups.commands]]
name = "test_cmd"
env_import = ["cmd_user=USER"]
env_vars = ["DATA_PATH=%{full_path}"]
cmd = "echo"
args = ["test"]

[groups.commands.vars]
full_path = "%{GlobalPrefix}/%{cmd_user}/%{group_suffix}"
`,
			expectVars: map[string]string{
				"DATA_PATH": "/opt/testuser/data",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				// Test error case
				cfg := configSetupHelper(t, tt.systemEnv, tt.configTOML)
				runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
				require.NoError(t, err, "should expand global config without error")

				runtimeGroup, err := config.ExpandGroup(&cfg.Groups[0], runtimeGlobal)
				require.NoError(t, err, "should expand group config without error")

				_, err = config.ExpandCommand(&cfg.Groups[0].Commands[0], nil, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit())
				assert.Error(t, err, "should fail when env_import var not in allowlist")
			} else {
				// Test success case
				envPriorityTestHelper(t, tt.systemEnv, tt.configTOML, tt.expectVars)
			}
		})
	}
}

// TestRunner_EnvImportAllowlistInheritance tests allowlist inheritance behavior
func TestRunner_EnvImportAllowlistInheritance(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
		wantErr    bool
		errLevel   string // "group" or "command"
	}{
		{
			name: "group_with_own_allowlist_restricts",
			systemEnv: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/test",
				"USER": "testuser",
			},
			configTOML: `
[global]
env_allowed = ["PATH", "HOME", "USER"]

[[groups]]
name = "restricted_group"
env_allowed = ["HOME"]  # More restrictive than global
env_import = ["grp_home=HOME"]
env_vars = ["RESULT=%{grp_home}"]

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"RESULT": "/home/test",
			},
		},
		{
			name: "group_allowlist_rejects_non_listed_var",
			systemEnv: map[string]string{
				"PATH": "/usr/bin",
				"USER": "testuser",
			},
			configTOML: `
[global]
env_allowed = ["PATH", "USER"]

[[groups]]
name = "restricted_group"
env_allowed = ["PATH"]  # USER not in group allowlist
env_import = ["grp_user=USER"]  # Should fail

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["test"]
`,
			wantErr:  true,
			errLevel: "group",
		},
		{
			name: "command_inherits_group_allowlist",
			systemEnv: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/test",
				"USER": "testuser",
			},
			configTOML: `
[global]
env_allowed = ["PATH"]  # Limited global allowlist

[[groups]]
name = "test_group"
env_allowed = ["HOME", "USER"]  # Override global allowlist

[[groups.commands]]
name = "test_cmd"
env_import = ["cmd_home=HOME"]  # Uses group allowlist
env_vars = ["RESULT=%{cmd_home}"]
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"RESULT": "/home/test",
			},
		},
		{
			name: "command_rejects_var_not_in_group_allowlist",
			systemEnv: map[string]string{
				"PATH": "/usr/bin",
				"USER": "testuser",
			},
			configTOML: `
[global]
env_allowed = ["PATH", "USER"]

[[groups]]
name = "test_group"
env_allowed = ["PATH"]  # USER not in group allowlist

[[groups.commands]]
name = "test_cmd"
env_import = ["cmd_user=USER"]  # Should fail - not in group allowlist
cmd = "echo"
args = ["test"]
`,
			wantErr:  true,
			errLevel: "command",
		},
		{
			name: "command_inherits_global_when_group_has_no_allowlist",
			systemEnv: map[string]string{
				"HOME": "/home/test",
			},
			configTOML: `
[global]
env_allowed = ["HOME", "USER"]

[[groups]]
name = "test_group"
# No env_allowed - should inherit from global

[[groups.commands]]
name = "test_cmd"
env_import = ["cmd_home=HOME"]  # Uses inherited global allowlist
env_vars = ["RESULT=%{cmd_home}"]
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"RESULT": "/home/test",
			},
		},
		{
			name: "empty_group_allowlist_rejects_all",
			systemEnv: map[string]string{
				"PATH": "/usr/bin",
			},
			configTOML: `
[global]
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "test_group"
env_allowed = []  # Explicitly reject all

[[groups.commands]]
name = "test_cmd"
env_import = ["cmd_path=PATH"]  # Should fail - empty allowlist
cmd = "echo"
args = ["test"]
`,
			wantErr:  true,
			errLevel: "command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				// Test error case
				cfg := configSetupHelper(t, tt.systemEnv, tt.configTOML)
				runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
				require.NoError(t, err, "should expand global config without error")

				switch tt.errLevel {
				case "group":
					_, err = config.ExpandGroup(&cfg.Groups[0], runtimeGlobal)
					assert.Error(t, err, "Expected error at group level but got none")
				case "command":
					runtimeGroup, err := config.ExpandGroup(&cfg.Groups[0], runtimeGlobal)
					if err != nil {
						// Error at group level is also acceptable
						return
					}
					_, err = config.ExpandCommand(&cfg.Groups[0].Commands[0], nil, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit())
					assert.Error(t, err, "should fail at command level")
				}
			} else {
				// Test success case
				envPriorityTestHelper(t, tt.systemEnv, tt.configTOML, tt.expectVars)
			}
		})
	}
}

// TestRunner_SystemEnvCache tests that SystemEnv is properly cached in RuntimeGlobal
// Note: SystemEnv contains ALL environment variables (not filtered by allowlist).
// The allowlist is used during ProcessEnvImport to control which variables can be imported.
func TestRunner_SystemEnvCache(t *testing.T) {
	tests := []struct {
		name                    string
		systemEnv               map[string]string
		configTOML              string
		verifySystemEnvNotEmpty bool
		verifyVariablesPresent  []string // Variables that must be present in SystemEnv
	}{
		{
			name: "systemenv_cached_all_variables",
			systemEnv: map[string]string{
				"PATH":       "/usr/bin:/bin",
				"HOME":       "/home/testuser",
				"USER":       "testuser",
				"SHELL":      "/bin/bash",
				"NOT_LISTED": "should_also_be_cached", // SystemEnv contains all variables
			},
			configTOML: `
[global]
env_allowed = ["PATH", "HOME", "USER"]
env_import = ["GlobalPath=PATH"]

[[groups]]
name = "group1"
env_import = ["group_home=HOME"]

[[groups.commands]]
name = "cmd1"
env_import = ["cmd_user=USER"]
cmd = "echo"
args = ["test"]
`,
			verifySystemEnvNotEmpty: true,
			verifyVariablesPresent: []string{
				"PATH", "HOME", "USER", "SHELL", "NOT_LISTED",
			},
		},
		{
			name: "systemenv_cached_even_with_empty_allowlist",
			systemEnv: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/test",
			},
			configTOML: `
[global]
env_allowed = []

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["test"]
`,
			verifySystemEnvNotEmpty: true,
			verifyVariablesPresent:  []string{"PATH", "HOME"},
		},
		{
			name: "systemenv_shared_across_multiple_levels",
			systemEnv: map[string]string{
				"PATH": "/usr/local/bin",
				"HOME": "/home/user",
			},
			configTOML: `
[global]
env_allowed = ["PATH", "HOME"]
env_import = ["GPath=PATH"]

[[groups]]
name = "group1"
env_import = ["grp_home=HOME"]

[[groups.commands]]
name = "cmd1"
env_import = ["cmd_path=PATH"]
cmd = "echo"
args = ["test"]
`,
			verifySystemEnvNotEmpty: true,
			verifyVariablesPresent:  []string{"PATH", "HOME"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configSetupHelper(t, tt.systemEnv, tt.configTOML)

			// Expand global configuration
			runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
			require.NoError(t, err, "should expand global config without error")

			// Verify SystemEnv is populated
			require.NotNil(t, runtimeGlobal.SystemEnv, "SystemEnv should not be nil")

			if tt.verifySystemEnvNotEmpty {
				assert.NotEmpty(t, runtimeGlobal.SystemEnv, "SystemEnv should not be empty")
			}

			// Check that expected variables are present in SystemEnv
			for _, varName := range tt.verifyVariablesPresent {
				expectedVal, exists := tt.systemEnv[varName]
				if !exists {
					continue // Skip if not in test environment
				}
				actualVal, ok := runtimeGlobal.SystemEnv[varName]
				require.True(t, ok, "SystemEnv should contain variable %s", varName)
				assert.Equal(t, expectedVal, actualVal, "SystemEnv[%s] should match", varName)
			}

			// Expand group and verify it uses cached SystemEnv
			if len(cfg.Groups) > 0 {
				runtimeGroup, err := config.ExpandGroup(&cfg.Groups[0], runtimeGlobal)
				require.NoError(t, err, "should expand group config without error")

				// Verify group's env_import used the cached SystemEnv
				if len(cfg.Groups[0].EnvImport) > 0 {
					// The group should have successfully imported from SystemEnv
					assert.NotEmpty(t, runtimeGroup.ExpandedVars, "Group should have expanded vars from env_import")
				}

				// Expand command and verify it uses cached SystemEnv
				if len(cfg.Groups[0].Commands) > 0 {
					runtimeCmd, err := config.ExpandCommand(&cfg.Groups[0].Commands[0], nil, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit())
					require.NoError(t, err, "should expand command config without error")

					// Verify command's env_import used the cached SystemEnv
					if len(cfg.Groups[0].Commands[0].EnvImport) > 0 {
						// The command should have successfully imported from SystemEnv
						assert.NotEmpty(t, runtimeCmd.ExpandedVars, "Command should have expanded vars from env_import")
					}
				}
			}
		})
	}
}

// TestRunner_EnvImportIntegration tests end-to-end integration scenarios
func TestRunner_EnvImportIntegration(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
	}{
		{
			name: "three_level_env_import_cascade",
			systemEnv: map[string]string{
				"BASE_PATH": "/opt",
				"APP_NAME":  "myapp",
				"VERSION":   "1.0",
			},
			configTOML: `
[global]
env_allowed = ["BASE_PATH", "APP_NAME", "VERSION"]
env_import = ["Base=BASE_PATH"]

[global.vars]
AppBase = "%{Base}/apps"

[[groups]]
name = "app_group"
env_import = ["app=APP_NAME"]

[groups.vars]
app_path = "%{AppBase}/%{app}"

[[groups.commands]]
name = "deploy_cmd"
env_import = ["ver=VERSION"]
env_vars = ["DEPLOY_PATH=%{deploy_path}"]
cmd = "echo"
args = ["%{deploy_path}"]

[groups.commands.vars]
deploy_path = "%{app_path}/v%{ver}"
`,
			expectVars: map[string]string{
				"DEPLOY_PATH": "/opt/apps/myapp/v1.0",
			},
		},
		{
			name: "mixed_env_import_and_vars_priority",
			systemEnv: map[string]string{
				"SYS_VAR": "from_system",
			},
			configTOML: `
[global]
env_allowed = ["SYS_VAR"]

[global.vars]
Myvar = "from_global_vars"

[[groups]]
name = "test_group"
env_import = ["myvar=SYS_VAR"]  # Override global vars with env_import
env_vars = ["RESULT=%{myvar}"]

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"RESULT": "from_system", // env_import should override vars
			},
		},
		{
			name: "complex_variable_expansion_chain",
			systemEnv: map[string]string{
				"USER":   "john",
				"DOMAIN": "example.com",
			},
			configTOML: `
[global]
env_allowed = ["USER", "DOMAIN"]
env_import = ["U=USER"]

[global.vars]
UserPrefix = "user"

[[groups]]
name = "test_group"
env_import = ["d=DOMAIN"]

[groups.vars]
email = "%{UserPrefix}_%{U}@%{d}"

[[groups.commands]]
name = "test_cmd"
env_vars = ["EMAIL=%{full_email}"]
cmd = "echo"
args = ["test"]

[groups.commands.vars]
full_email = "email:%{email}"
`,
			expectVars: map[string]string{
				"EMAIL": "email:user_john@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envPriorityTestHelper(t, tt.systemEnv, tt.configTOML, tt.expectVars)
		})
	}
}
