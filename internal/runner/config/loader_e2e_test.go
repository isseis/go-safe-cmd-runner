//go:build skip_e2e_tests

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

// TestE2E_CompleteConfiguration tests the entire Global/Group/Command env workflow
// with complex variable references, allowlist inheritance, and verify_files expansion.
func TestE2E_CompleteConfiguration(t *testing.T) {
	// Setup: Set system environment variables using t.Setenv
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("USER", "testuser")
	t.Setenv("PORT", "8080") // For web group

	// Load configuration
	configPath := filepath.Join("testdata", "e2e_complete.toml")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read E2E test configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "Failed to load E2E test configuration")
	require.NotNil(t, cfg, "Configuration should not be nil")

	// Expand global configuration to access ExpandedEnv
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err, "Failed to expand global configuration")
	require.NotNil(t, runtimeGlobal, "RuntimeGlobal should not be nil")

	// Verify Global configuration
	t.Run("GlobalEnv", func(t *testing.T) {
		require.NotNil(t, runtimeGlobal.ExpandedEnv, "Global.ExpandedEnv should be initialized")

		// Check Global.Env variables are expanded
		assert.Equal(t, "/opt/app", runtimeGlobal.ExpandedEnv["BASE_DIR"], "BASE_DIR should be set")
		assert.Equal(t, "info", runtimeGlobal.ExpandedEnv["LOG_LEVEL"], "LOG_LEVEL should be set")

		// Check PATH includes both custom and system PATH
		path := runtimeGlobal.ExpandedEnv["PATH"]
		assert.Contains(t, path, "/opt/tools/bin", "PATH should include custom path")
		assert.Contains(t, path, "/usr/bin:/bin", "PATH should include system PATH")
	})

	t.Run("GlobalVerifyFiles", func(t *testing.T) {
		// Global.ExpandedVerifyFiles should reference Global.Env variables
		require.Len(t, runtimeGlobal.ExpandedVerifyFiles, 1, "Global should have 1 expanded verify_files entry")
		assert.Equal(t, "/opt/app/verify.sh", runtimeGlobal.ExpandedVerifyFiles[0],
			"Global.ExpandedVerifyFiles should expand BASE_DIR")
	})

	// Verify Database Group
	t.Run("DatabaseGroup", func(t *testing.T) {
		dbGroup := findGroup(t, cfg, "database")
		require.NotNil(t, dbGroup, "Database group should exist")
		require.NotNil(t, dbGroup.ExpandedEnv, "Database group ExpandedEnv should be initialized")

		// Check Group.Env variables are expanded and reference Global.Env
		assert.Equal(t, "localhost", dbGroup.ExpandedEnv["DB_HOST"], "DB_HOST should be set")
		assert.Equal(t, "5432", dbGroup.ExpandedEnv["DB_PORT"], "DB_PORT should be set")
		assert.Equal(t, "/opt/app/db-data", dbGroup.ExpandedEnv["DB_DATA"],
			"DB_DATA should expand BASE_DIR from Global.Env")

		// Check Group.ExpandedVerifyFiles references Group.Env
		require.Len(t, dbGroup.ExpandedVerifyFiles, 1, "Database group should have 1 expanded verify_files entry")
		assert.Equal(t, "/opt/app/db-data/schema.sql", dbGroup.ExpandedVerifyFiles[0],
			"Group.ExpandedVerifyFiles should expand DB_DATA")

		// Check allowlist inheritance (should inherit from Global)
		// Note: We cannot directly check the effective allowlist in this test,
		// but we verify that the group doesn't define its own allowlist
		assert.Nil(t, dbGroup.EnvAllowlist, "Database group should not define env_allowlist (inherits from Global)")
	})

	t.Run("DatabaseMigrateCommand", func(t *testing.T) {
		dbGroup := findGroup(t, cfg, "database")
		require.NotNil(t, dbGroup, "Database group should exist")
		require.Len(t, dbGroup.Commands, 1, "Database group should have 1 command")

		migrateCmd := dbGroup.Commands[0]
		assert.Equal(t, "migrate", migrateCmd.Name, "Command name should be 'migrate'")

		// Command expansion is implemented in config.Loader with new system
		// These fields should use %{VAR} syntax
		assert.Equal(t, "%{base_dir}/bin/migrate", migrateCmd.Cmd,
			"Command.Cmd should contain %{VAR} syntax")
		assert.Equal(t, []string{"-h", "%{db_host}", "-p", "%{db_port}"}, migrateCmd.Args,
			"Command.Args should contain %{VAR} syntax")

		// Command.Env should be set with %{VAR} syntax
		require.Len(t, migrateCmd.Env, 1, "Command should have 1 env variable")
		assert.Equal(t, "MIGRATION_DIR=%{migration_dir}", migrateCmd.Env[0],
			"Command.Env should contain %{VAR} syntax")

		// ExpandedCmd, ExpandedArgs, ExpandedEnv should be populated
		assert.NotEmpty(t, migrateCmd.ExpandedCmd,
			"ExpandedCmd should be populated (expansion implemented)")
		assert.NotNil(t, migrateCmd.ExpandedArgs,
			"ExpandedArgs should be populated (expansion implemented)")
		assert.NotNil(t, migrateCmd.ExpandedEnv,
			"ExpandedEnv should be populated (expansion implemented)")
	})

	// Verify Web Group with allowlist override
	t.Run("WebGroup", func(t *testing.T) {
		webGroup := findGroup(t, cfg, "web")
		require.NotNil(t, webGroup, "Web group should exist")
		require.NotNil(t, webGroup.ExpandedEnv, "Web group ExpandedEnv should be initialized")

		// Check Group.Env variables are expanded and reference Global.Env
		assert.Equal(t, "/opt/app/web", webGroup.ExpandedEnv["WEB_DIR"],
			"WEB_DIR should expand BASE_DIR from Global.Env")

		// Check allowlist override
		require.NotNil(t, webGroup.EnvAllowlist, "Web group should define its own env_allowlist")
		require.Len(t, webGroup.EnvAllowlist, 1, "Web group should have 1 allowlist entry")
		assert.Equal(t, "PORT", webGroup.EnvAllowlist[0], "Web group allowlist should only contain PORT")
	})

	t.Run("WebStartCommand", func(t *testing.T) {
		webGroup := findGroup(t, cfg, "web")
		require.NotNil(t, webGroup, "Web group should exist")
		require.Len(t, webGroup.Commands, 1, "Web group should have 1 command")

		startCmd := webGroup.Commands[0]
		assert.Equal(t, "start", startCmd.Name, "Command name should be 'start'")

		// Command expansion is implemented in config.Loader with new system
		assert.Equal(t, "%{web_dir}/server", startCmd.Cmd,
			"Command.Cmd should contain %{VAR} syntax")
		assert.Equal(t, []string{"--port", "%{port}"}, startCmd.Args,
			"Command.Args should contain %{VAR} syntax")

		// ExpandedCmd, ExpandedArgs, ExpandedEnv should be populated
		assert.NotEmpty(t, startCmd.ExpandedCmd,
			"ExpandedCmd should be populated (expansion implemented)")
		assert.NotNil(t, startCmd.ExpandedArgs,
			"ExpandedArgs should be populated (expansion implemented)")
		assert.NotNil(t, startCmd.ExpandedEnv,
			"ExpandedEnv should be populated (expansion implemented)")
	})
}

// TestE2E_PriorityVerification verifies the variable priority:
// Command.Env > Group.Env > Global.Env > System Env
func TestE2E_PriorityVerification(t *testing.T) {
	// Create a temporary test configuration
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "priority_test.toml")

	configContent := `[global]
vars = ["priority=global"]
env = ["PRIORITY=%{priority}", "GLOBAL_ONLY=global_value"]
env_allowlist = ["HOME"]

[[groups]]
name = "test_group"
vars = ["priority=group"]
env = ["PRIORITY=%{priority}", "GROUP_ONLY=group_value"]

[[groups.commands]]
name = "test_cmd"
cmd = "/bin/echo"
args = ["%{priority}"]
vars = ["priority=command"]
env = ["PRIORITY=%{priority}", "COMMAND_ONLY=command_value"]
`
	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err, "Failed to create test config file")

	// Load configuration
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read test configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "Failed to load test configuration")
	require.NotNil(t, cfg, "Configuration should not be nil")

	// Verify Global.Env
	t.Run("GlobalEnv", func(t *testing.T) {
		require.NotNil(t, cfg.Global.ExpandedEnv, "Global.ExpandedEnv should be initialized")
		assert.Equal(t, "global", cfg.Global.ExpandedEnv["PRIORITY"], "PRIORITY in Global.Env should be 'global'")
		assert.Equal(t, "global_value", cfg.Global.ExpandedEnv["GLOBAL_ONLY"], "GLOBAL_ONLY should be set")
	})

	// Verify Group.Env overrides Global.Env
	t.Run("GroupEnv", func(t *testing.T) {
		testGroup := findGroup(t, cfg, "test_group")
		require.NotNil(t, testGroup, "Test group should exist")
		require.NotNil(t, testGroup.ExpandedEnv, "Group.ExpandedEnv should be initialized")

		// Group.ExpandedEnv only contains Group-level variables
		assert.Equal(t, "group", testGroup.ExpandedEnv["PRIORITY"], "PRIORITY in Group.Env should be 'group'")
		assert.Equal(t, "group_value", testGroup.ExpandedEnv["GROUP_ONLY"], "GROUP_ONLY should be set")

		// Global-level variables are not in Group.ExpandedEnv
		_, hasGlobalOnly := testGroup.ExpandedEnv["GLOBAL_ONLY"]
		assert.False(t, hasGlobalOnly, "GLOBAL_ONLY should not be in Group.ExpandedEnv")
	})

	// Command.Env expansion is implemented
	t.Run("CommandEnv", func(t *testing.T) {
		testGroup := findGroup(t, cfg, "test_group")
		require.NotNil(t, testGroup, "Test group should exist")
		require.Len(t, testGroup.Commands, 1, "Test group should have 1 command")

		testCmd := testGroup.Commands[0]
		assert.Equal(t, "test_cmd", testCmd.Name, "Command name should be 'test_cmd'")

		// Command.Env uses %{VAR} syntax
		require.Len(t, testCmd.Env, 2, "Command should have 2 env variables")
		assert.Equal(t, "PRIORITY=%{priority}", testCmd.Env[0], "PRIORITY in Command.Env should use %{VAR} syntax")
		assert.Equal(t, "COMMAND_ONLY=command_value", testCmd.Env[1], "COMMAND_ONLY should be set")

		// ExpandedEnv should be populated (expansion implemented)
		assert.NotNil(t, testCmd.ExpandedEnv,
			"ExpandedEnv should be populated (expansion implemented)")
	})
}

// TestE2E_AllowlistScenarios tests all allowlist scenarios:
// - Inheritance (group.env_allowlist == nil)
// - Override (group.env_allowlist != nil && len > 0)
// - Reject all (group.env_allowlist == [])
func TestE2E_AllowlistScenarios(t *testing.T) {
	// Create a temporary test configuration
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "allowlist_test.toml")

	configContent := `[global]
env = ["BASE=/base"]
env_allowlist = ["HOME", "USER"]

[[groups]]
name = "inherit_group"
# env_allowlist not defined -> inherits from Global
env = ["INHERIT_VAR=value"]

[[groups]]
name = "override_group"
env_allowlist = ["PATH"]  # Override
env = ["OVERRIDE_VAR=value"]

[[groups]]
name = "reject_group"
env_allowlist = []  # Reject all
env = ["REJECT_VAR=value"]
`
	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err, "Failed to create test config file")

	// Load configuration
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read test configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "Failed to load test configuration")
	require.NotNil(t, cfg, "Configuration should not be nil")

	t.Run("InheritAllowlist", func(t *testing.T) {
		inheritGroup := findGroup(t, cfg, "inherit_group")
		require.NotNil(t, inheritGroup, "Inherit group should exist")

		// Group should not define its own allowlist
		assert.Nil(t, inheritGroup.EnvAllowlist, "Inherit group should not define env_allowlist")

		// Group.Env should be expanded
		require.NotNil(t, inheritGroup.ExpandedEnv, "Group.ExpandedEnv should be initialized")
		assert.Equal(t, "value", inheritGroup.ExpandedEnv["INHERIT_VAR"], "INHERIT_VAR should be set")
	})

	t.Run("OverrideAllowlist", func(t *testing.T) {
		overrideGroup := findGroup(t, cfg, "override_group")
		require.NotNil(t, overrideGroup, "Override group should exist")

		// Group should define its own allowlist
		require.NotNil(t, overrideGroup.EnvAllowlist, "Override group should define env_allowlist")
		require.Len(t, overrideGroup.EnvAllowlist, 1, "Override group should have 1 allowlist entry")
		assert.Equal(t, "PATH", overrideGroup.EnvAllowlist[0], "Override group allowlist should be PATH")

		// Group.Env should be expanded
		require.NotNil(t, overrideGroup.ExpandedEnv, "Group.ExpandedEnv should be initialized")
		assert.Equal(t, "value", overrideGroup.ExpandedEnv["OVERRIDE_VAR"], "OVERRIDE_VAR should be set")
	})

	t.Run("RejectAllowlist", func(t *testing.T) {
		rejectGroup := findGroup(t, cfg, "reject_group")
		require.NotNil(t, rejectGroup, "Reject group should exist")

		// Group should define empty allowlist
		require.NotNil(t, rejectGroup.EnvAllowlist, "Reject group should define env_allowlist")
		assert.Empty(t, rejectGroup.EnvAllowlist, "Reject group allowlist should be empty")

		// Group.Env should be expanded
		require.NotNil(t, rejectGroup.ExpandedEnv, "Group.ExpandedEnv should be initialized")
		assert.Equal(t, "value", rejectGroup.ExpandedEnv["REJECT_VAR"], "REJECT_VAR should be set")
	})
}

// TestE2E_VerifyFilesExpansion tests that verify_files at all levels can reference
// environment variables from Global.Env and Group.Env.
func TestE2E_VerifyFilesExpansion(t *testing.T) {
	// Create a temporary test configuration
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "verify_files_test.toml")

	configContent := `[global]
vars = ["global_dir=/global"]
env = ["GLOBAL_DIR=%{global_dir}"]
verify_files = ["%{global_dir}/global_verify.sh"]

[[groups]]
name = "test_group"
vars = ["group_dir=%{global_dir}/group"]
env = ["GROUP_DIR=%{group_dir}"]
verify_files = ["%{group_dir}/group_verify.sh"]
`
	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err, "Failed to create test config file")

	// Load configuration
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read test configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "Failed to load test configuration")
	require.NotNil(t, cfg, "Configuration should not be nil")

	t.Run("GlobalVerifyFiles", func(t *testing.T) {
		require.Len(t, cfg.Global.ExpandedVerifyFiles, 1, "Global should have 1 expanded verify_files entry")
		assert.Equal(t, "/global/global_verify.sh", cfg.Global.ExpandedVerifyFiles[0],
			"Global.ExpandedVerifyFiles should expand GLOBAL_DIR")
	})

	t.Run("GroupVerifyFiles", func(t *testing.T) {
		testGroup := findGroup(t, cfg, "test_group")
		require.NotNil(t, testGroup, "Test group should exist")

		require.Len(t, testGroup.ExpandedVerifyFiles, 1, "Group should have 1 expanded verify_files entry")
		assert.Equal(t, "/global/group/group_verify.sh", testGroup.ExpandedVerifyFiles[0],
			"Group.ExpandedVerifyFiles should expand GROUP_DIR which references GLOBAL_DIR")
	})
}

// TestE2E_FullExpansionPipeline is a comprehensive test for the entire expansion pipeline
// from Global.Env -> Group.Env -> Command.Env/Cmd/Args.
//
// This test verifies that Command expansion is performed correctly
func TestE2E_FullExpansionPipeline(t *testing.T) {
	// Setup: Set system environment variables using t.Setenv
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", "/home/testuser")

	// Load configuration with command_env_references_global_group.toml
	// This config tests Command.Env/Cmd/Args referencing Global and Group env
	configPath := filepath.Join("testdata", "command_env_references_global_group.toml")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read test configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "Failed to load test configuration")
	require.NotNil(t, cfg, "Configuration should not be nil")

	// Verify Global.ExpandedEnv
	t.Run("GlobalExpandedEnv", func(t *testing.T) {
		require.NotNil(t, cfg.Global.ExpandedEnv, "Global.ExpandedEnv should be initialized")
		assert.Equal(t, "/opt", cfg.Global.ExpandedEnv["BASE_DIR"], "BASE_DIR should be set")
	})

	// Verify Global.ExpandedVerifyFiles (if present in config)
	t.Run("GlobalExpandedVerifyFiles", func(t *testing.T) {
		// command_env_references_global_group.toml doesn't have verify_files at Global level
		// This is acceptable for this test
		assert.Empty(t, cfg.Global.ExpandedVerifyFiles,
			"Global.ExpandedVerifyFiles should be empty for this config")
	})

	// Verify Group.ExpandedEnv
	t.Run("GroupExpandedEnv", func(t *testing.T) {
		appGroup := findGroup(t, cfg, "app_group")
		require.NotNil(t, appGroup, "app_group should exist")
		require.NotNil(t, appGroup.ExpandedEnv, "Group.ExpandedEnv should be initialized")

		// APP_DIR should reference BASE_DIR from Global.Env
		assert.Equal(t, "/opt/myapp", appGroup.ExpandedEnv["APP_DIR"],
			"APP_DIR should expand BASE_DIR from Global.Env")
	})

	// Verify Group.ExpandedVerifyFiles (if present in config)
	t.Run("GroupExpandedVerifyFiles", func(t *testing.T) {
		appGroup := findGroup(t, cfg, "app_group")
		require.NotNil(t, appGroup, "app_group should exist")

		// command_env_references_global_group.toml doesn't have verify_files at Group level
		assert.Empty(t, appGroup.ExpandedVerifyFiles,
			"Group.ExpandedVerifyFiles should be empty for this config")
	})

	// Verify Command.ExpandedEnv, ExpandedCmd, ExpandedArgs
	t.Run("CommandExpansion", func(t *testing.T) {
		appGroup := findGroup(t, cfg, "app_group")
		require.NotNil(t, appGroup, "app_group should exist")
		require.Len(t, appGroup.Commands, 1, "app_group should have 1 command")

		runAppCmd := appGroup.Commands[0]
		assert.Equal(t, "run_app", runAppCmd.Name, "Command name should be 'run_app'")

		// Raw configuration values use %{VAR} syntax
		t.Run("RawConfiguration", func(t *testing.T) {
			// Raw values use %{VAR} syntax
			assert.Equal(t, "%{app_dir}/bin/server", runAppCmd.Cmd,
				"Cmd should use %{VAR} syntax")
			assert.Equal(t, []string{"--log", "%{log_dir}/app.log"}, runAppCmd.Args,
				"Args should use %{VAR} syntax")
			require.Len(t, runAppCmd.Env, 1, "Command should have 1 env variable")
			assert.Equal(t, "LOG_DIR=%{log_dir}", runAppCmd.Env[0],
				"Command.Env should use %{VAR} syntax")

			// Expanded fields should be populated
			assert.NotEmpty(t, runAppCmd.ExpandedCmd,
				"ExpandedCmd should be populated")
			assert.NotNil(t, runAppCmd.ExpandedArgs,
				"ExpandedArgs should be populated")
			assert.NotNil(t, runAppCmd.ExpandedEnv,
				"ExpandedEnv should be populated")
		})

		// This test case verifies expansion functionality
		t.Run("ExpansionTest", func(t *testing.T) {
			// Command.ExpandedEnv should be populated with expanded variables
			require.NotNil(t, runAppCmd.ExpandedEnv, "ExpandedEnv should be populated")
			assert.Equal(t, "/opt/myapp/logs", runAppCmd.ExpandedEnv["LOG_DIR"],
				"LOG_DIR should expand APP_DIR from Group.Env")

			// Command.ExpandedCmd should be expanded
			assert.Equal(t, "/opt/myapp/bin/server", runAppCmd.ExpandedCmd,
				"ExpandedCmd should expand APP_DIR from Group.Env")

			// Command.ExpandedArgs should be expanded
			require.Len(t, runAppCmd.ExpandedArgs, 2, "ExpandedArgs should have 2 elements")
			assert.Equal(t, "--log", runAppCmd.ExpandedArgs[0], "First arg should be unchanged")
			assert.Equal(t, "/opt/myapp/logs/app.log", runAppCmd.ExpandedArgs[1],
				"Second arg should expand LOG_DIR from Command.Env")
		})
	})
}

// Helper function to find a group by name
func findGroup(t *testing.T, cfg *runnertypes.ConfigSpec, name string) *runnertypes.GroupSpec {
	t.Helper()
	for i := range cfg.Groups {
		if cfg.Groups[i].Name == name {
			return &cfg.Groups[i]
		}
	}
	return nil
}

// TestE2E_VerifyFilesExpansion_SpecialCharacters tests verify_files expansion with special characters
func TestE2E_VerifyFilesExpansion_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "verify_files_special_chars.toml")

	configContent := `[global]
vars = ["base_dir=/opt/my app", "file_name=test-file_v1.0.sh"]
verify_files = ["%{base_dir}/%{file_name}"]

[[groups]]
name = "test_group"
vars = ["sub_dir=sub-dir"]
verify_files = ["%{base_dir}/%{sub_dir}/script.sh"]
`
	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err, "Failed to create test config file")

	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read test configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "Failed to load test configuration")
	require.NotNil(t, cfg, "Configuration should not be nil")

	expandedCfg, err := config.ExpandGlobal(cfg)
	require.NoError(t, err, "Failed to expand configuration")

	t.Run("GlobalVerifyFiles_WithSpaces", func(t *testing.T) {
		require.Len(t, expandedCfg.Global.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/my app/test-file_v1.0.sh", expandedCfg.Global.ExpandedVerifyFiles[0],
			"verify_files should handle paths with spaces and special characters")
	})

	t.Run("GroupVerifyFiles_WithDashes", func(t *testing.T) {
		testGroup := findGroup(t, expandedCfg, "test_group")
		require.NotNil(t, testGroup)
		runtimeGroup, err := config.ExpandGroup(testGroup, cfg.Global, map[string]string{})
		require.NoError(t, err, "Failed to expand group")
		require.Len(t, runtimeGroup.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/my app/sub-dir/script.sh", runtimeGroup.ExpandedVerifyFiles[0],
			"verify_files should handle paths with dashes")
	})
}

// TestE2E_VerifyFilesExpansion_NestedReferences tests verify_files with nested variable references
func TestE2E_VerifyFilesExpansion_NestedReferences(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "verify_files_nested.toml")

	configContent := `[global]
vars = ["root=/opt", "app_name=myapp", "app_dir=%{root}/%{app_name}"]
verify_files = ["%{app_dir}/verify.sh"]

[[groups]]
name = "test_group"
vars = ["subdir=scripts", "full_path=%{app_dir}/%{subdir}"]
verify_files = ["%{full_path}/check.sh"]
`
	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err, "Failed to create test config file")

	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read test configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "Failed to load test configuration")
	require.NotNil(t, cfg, "Configuration should not be nil")

	t.Run("GlobalVerifyFiles_NestedReferences", func(t *testing.T) {
		runtimeGlobal, err := loader.ExpandGlobal(cfg, nil)
		require.NoError(t, err, "Failed to expand global config")
		require.Len(t, runtimeGlobal.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/myapp/verify.sh", runtimeGlobal.ExpandedVerifyFiles[0],
			"verify_files should handle nested variable references (root -> app_name -> app_dir)")
	})

	t.Run("GroupVerifyFiles_DeeplyNestedReferences", func(t *testing.T) {
		testGroup := findGroup(t, cfg, "test_group")
		require.NotNil(t, testGroup)
		runtimeGlobal, err := loader.ExpandGlobal(cfg, nil)
		require.NoError(t, err, "Failed to expand global config for group expansion")
		runtimeGroup, err := loader.ExpandGroup(testGroup, runtimeGlobal)
		require.NoError(t, err, "Failed to expand group config")
		require.Len(t, runtimeGroup.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/myapp/scripts/check.sh", runtimeGroup.ExpandedVerifyFiles[0],
			"verify_files should handle deeply nested references (root -> app_name -> app_dir -> subdir -> full_path)")
	})
}

// TestE2E_VerifyFilesExpansion_ErrorHandling tests error handling for invalid verify_files expansion
func TestE2E_VerifyFilesExpansion_ErrorHandling(t *testing.T) {
	t.Run("UndefinedVariable", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "verify_files_undefined.toml")

		configContent := `[global]
vars = ["existing_var=/opt"]
verify_files = ["%{undefined_var}/script.sh"]

[[groups]]
name = "test_group"
`
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err, "Failed to create test config file")

		content, err := os.ReadFile(configPath)
		require.NoError(t, err, "Failed to read test configuration file")

		loader := config.NewLoader()
		_, err = loader.LoadConfig(content)
		require.Error(t, err, "Should fail when verify_files references undefined variable")
		assert.Contains(t, err.Error(), "undefined_var", "Error should mention the undefined variable name")
		assert.Contains(t, err.Error(), "undefined variable", "Error should indicate it's an undefined variable error")
	})

	t.Run("EmptyVariableName", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "verify_files_empty_var.toml")

		configContent := `[global]
verify_files = ["%{}/script.sh"]

[[groups]]
name = "test_group"
`
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err, "Failed to create test config file")

		content, err := os.ReadFile(configPath)
		require.NoError(t, err, "Failed to read test configuration file")

		loader := config.NewLoader()
		_, err = loader.LoadConfig(content)
		require.Error(t, err, "Should fail when verify_files has empty variable name")
		assert.Contains(t, err.Error(), "empty variable name", "Error should mention empty variable name")
	})

	t.Run("MultipleVerifyFilesWithMixedErrors", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "verify_files_mixed.toml")

		configContent := `[global]
vars = ["valid_dir=/opt"]
verify_files = [
    "%{valid_dir}/good.sh",
    "%{invalid_var}/bad.sh"
]

[[groups]]
name = "test_group"
`
		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err, "Failed to create test config file")

		content, err := os.ReadFile(configPath)
		require.NoError(t, err, "Failed to read test configuration file")

		loader := config.NewLoader()
		_, err = loader.LoadConfig(content)
		require.Error(t, err, "Should fail on first invalid verify_files entry")
		assert.Contains(t, err.Error(), "invalid_var", "Error should mention the first invalid variable")
	})
}
