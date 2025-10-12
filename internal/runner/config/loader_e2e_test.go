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
	// Setup: Set system environment variables
	originalPath := os.Getenv("PATH")
	originalHome := os.Getenv("HOME")
	originalUser := os.Getenv("USER")

	os.Setenv("PATH", "/usr/bin:/bin")
	os.Setenv("HOME", "/home/testuser")
	os.Setenv("USER", "testuser")
	os.Setenv("PORT", "8080") // For web group

	defer func() {
		os.Setenv("PATH", originalPath)
		os.Setenv("HOME", originalHome)
		os.Setenv("USER", originalUser)
		os.Unsetenv("PORT")
	}()

	// Load configuration
	configPath := filepath.Join("testdata", "e2e_complete.toml")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read E2E test configuration file")

	loader := config.NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "Failed to load E2E test configuration")
	require.NotNil(t, cfg, "Configuration should not be nil")

	// Verify Global configuration
	t.Run("GlobalEnv", func(t *testing.T) {
		require.NotNil(t, cfg.Global.ExpandedEnv, "Global.ExpandedEnv should be initialized")

		// Check Global.Env variables are expanded
		assert.Equal(t, "/opt/app", cfg.Global.ExpandedEnv["BASE_DIR"], "BASE_DIR should be set")
		assert.Equal(t, "info", cfg.Global.ExpandedEnv["LOG_LEVEL"], "LOG_LEVEL should be set")

		// Check PATH includes both custom and system PATH
		path := cfg.Global.ExpandedEnv["PATH"]
		assert.Contains(t, path, "/opt/tools/bin", "PATH should include custom path")
		assert.Contains(t, path, "/usr/bin:/bin", "PATH should include system PATH")
	})

	t.Run("GlobalVerifyFiles", func(t *testing.T) {
		// Global.ExpandedVerifyFiles should reference Global.Env variables
		require.Len(t, cfg.Global.ExpandedVerifyFiles, 1, "Global should have 1 expanded verify_files entry")
		assert.Equal(t, "/opt/app/verify.sh", cfg.Global.ExpandedVerifyFiles[0],
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

		// Note: Command.Env/Cmd/Args expansion happens in bootstrap.InitConfig(),
		// so we verify the raw values here and check that they will be expanded later
		assert.Equal(t, "${BASE_DIR}/bin/migrate", migrateCmd.Cmd,
			"Command.Cmd should contain unexpanded variable (expanded in bootstrap)")
		assert.Equal(t, []string{"-h", "${DB_HOST}", "-p", "${DB_PORT}"}, migrateCmd.Args,
			"Command.Args should contain unexpanded variables (expanded in bootstrap)")

		// Command.Env should be set but not yet expanded
		require.Len(t, migrateCmd.Env, 1, "Command should have 1 env variable")
		assert.Equal(t, "MIGRATION_DIR=${DB_DATA}/migrations", migrateCmd.Env[0],
			"Command.Env should contain unexpanded variable (expanded in bootstrap)")
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

		// Command.Cmd/Args should contain unexpanded variables
		assert.Equal(t, "${WEB_DIR}/server", startCmd.Cmd,
			"Command.Cmd should contain unexpanded variable (expanded in bootstrap)")
		assert.Equal(t, []string{"--port", "${PORT}"}, startCmd.Args,
			"Command.Args should contain unexpanded variables (expanded in bootstrap)")
	})
}

// TestE2E_PriorityVerification verifies the variable priority:
// Command.Env > Group.Env > Global.Env > System Env
func TestE2E_PriorityVerification(t *testing.T) {
	// Create a temporary test configuration
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "priority_test.toml")

	configContent := `[global]
env = ["PRIORITY=global", "GLOBAL_ONLY=global_value"]
env_allowlist = ["HOME"]

[[groups]]
name = "test_group"
env = ["PRIORITY=group", "GROUP_ONLY=group_value"]

[[groups.commands]]
name = "test_cmd"
cmd = "/bin/echo"
args = ["${PRIORITY}"]
env = ["PRIORITY=command", "COMMAND_ONLY=command_value"]
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

	// Command.Env expansion happens in bootstrap, so we just verify the structure here
	t.Run("CommandEnv", func(t *testing.T) {
		testGroup := findGroup(t, cfg, "test_group")
		require.NotNil(t, testGroup, "Test group should exist")
		require.Len(t, testGroup.Commands, 1, "Test group should have 1 command")

		testCmd := testGroup.Commands[0]
		assert.Equal(t, "test_cmd", testCmd.Name, "Command name should be 'test_cmd'")

		// Command.Env should be set but not yet expanded
		require.Len(t, testCmd.Env, 2, "Command should have 2 env variables")
		assert.Equal(t, "PRIORITY=command", testCmd.Env[0], "PRIORITY in Command.Env should be 'command'")
		assert.Equal(t, "COMMAND_ONLY=command_value", testCmd.Env[1], "COMMAND_ONLY should be set")
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
env = ["GLOBAL_DIR=/global"]
verify_files = ["${GLOBAL_DIR}/global_verify.sh"]

[[groups]]
name = "test_group"
env = ["GROUP_DIR=${GLOBAL_DIR}/group"]
verify_files = ["${GROUP_DIR}/group_verify.sh"]
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

// Helper function to find a group by name
func findGroup(t *testing.T, cfg *runnertypes.Config, name string) *runnertypes.CommandGroup {
	t.Helper()
	for i := range cfg.Groups {
		if cfg.Groups[i].Name == name {
			return &cfg.Groups[i]
		}
	}
	return nil
}
