// Package runner provides tests for runner-level security integration.
//
// # Test Scope
//
// This file contains END-TO-END (E2E) security integration tests for the runner.
// These tests validate the complete security flow from TOML file loading through
// configuration expansion, including file system operations.
//
// # Test Coverage
//
//   - Complete TOML file loading with security features
//   - File system integration (verify_files path resolution)
//   - Multi-layer configuration expansion (global + group + command)
//   - Real environment variable processing
//   - Allowlist enforcement in realistic scenarios
//
// # Complementary Tests
//
// For UNIT-LEVEL security logic tests without file I/O dependencies, see:
//   - internal/runner/config/security_integration_test.go (config package unit tests)
//   - internal/runner/config/security_integration_test.go (attack prevention tests)
package runner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunner_SecurityIntegration tests full-stack security verification at runner level.
// These are E2E tests that validate security from TOML loading through execution.
func TestRunner_SecurityIntegration(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testing.T) string // Returns config file path
		systemEnv   map[string]string
		expectError bool
		errorCheck  func(*testing.T, error)
		validate    func(*testing.T, *runnertypes.ConfigSpec)
	}{
		{
			name: "Basic allowlist + variable expansion (E2E)",
			systemEnv: map[string]string{
				"SAFE_VAR": "safe_value",
			},
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config.toml")

				configContent := `
[global]
env_import = ["MY_SAFE=SAFE_VAR"]
env_allowed = ["SAFE_VAR"]
env_vars = ["MY_ENV=%{derived}"]

[global.vars]
derived = "%{MY_SAFE}/subdir"
`
				err := os.WriteFile(configPath, []byte(configContent), 0o644)
				require.NoError(t, err)

				return configPath
			},
			expectError: false,
			validate: func(t *testing.T, cfg *runnertypes.ConfigSpec) {
				// Verify that allowed variable is properly expanded
				// Need to expand GlobalSpec to RuntimeGlobal to access ExpandedVars and ExpandedEnv
				runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
				require.NoError(t, err)
				require.NotNil(t, runtimeGlobal.ExpandedVars)
				require.NotNil(t, runtimeGlobal.ExpandedEnv)
				assert.Equal(t, "safe_value/subdir", runtimeGlobal.ExpandedVars["derived"])
				assert.Equal(t, "safe_value/subdir", runtimeGlobal.ExpandedEnv["MY_ENV"])
			},
		},
		{
			name: "Allowlist violation detection (E2E)",
			systemEnv: map[string]string{
				"SECRET_VAR": "super_secret",
				"SAFE_VAR":   "safe_value",
			},
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config.toml")

				configContent := `
[global]
env_import = ["MY_SECRET=SECRET_VAR"]
env_allowed = ["SAFE_VAR"]
`
				err := os.WriteFile(configPath, []byte(configContent), 0o644)
				require.NoError(t, err)

				return configPath
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not in allowlist")
			},
		},
		{
			name: "Complete config with security features",
			systemEnv: map[string]string{
				"SAFE_HOME":   "/home/user",
				"SECRET_KEY":  "super_secret",
				"ALLOWED_VAR": "allowed_value",
			},
			setup: func(t *testing.T) string {
				// Create a temporary config file with comprehensive security settings
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config.toml")

				configContent := `
[global]
env_import = ["home=SAFE_HOME", "allowed=ALLOWED_VAR"]
env_allowed = ["SAFE_HOME", "ALLOWED_VAR"]
env_vars = ["WORK_DIR=%{work_dir}", "CONFIG_DIR=%{config_dir}"]
verify_files = ["%{config_dir}/app.conf"]

[global.vars]
work_dir = "%{home}/work"
config_dir = "%{home}/config"

[[groups]]
name = "secure_group"
env_import = ["group_var=ALLOWED_VAR"]
env_allowed = ["ALLOWED_VAR"]
env_vars = ["GROUP_PATH=%{group_path}"]

[groups.vars]
group_path = "%{group_var}/data"

[[groups.commands]]
name = "secure_command"
cmd = "echo"
args = ["Running with security"]
env_vars = ["CMD_VAR=%{group_path}"]
`
				err := os.WriteFile(configPath, []byte(configContent), 0o644)
				require.NoError(t, err)

				// Create the expected config file
				configFile := filepath.Join(tmpDir, "config", "app.conf")
				err = os.MkdirAll(filepath.Dir(configFile), 0o755)
				require.NoError(t, err)
				err = os.WriteFile(configFile, []byte("test config"), 0o644)
				require.NoError(t, err)

				return configPath
			},
			expectError: false,
			validate: func(t *testing.T, cfg *runnertypes.ConfigSpec) {
				// Verify that global level expansion is correct
				runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
				require.NoError(t, err)
				require.NotNil(t, runtimeGlobal.ExpandedVars)
				require.NotNil(t, runtimeGlobal.ExpandedEnv)
				assert.Equal(t, "/home/user/work", runtimeGlobal.ExpandedVars["work_dir"])
				assert.Equal(t, "/home/user/config", runtimeGlobal.ExpandedVars["config_dir"])
				assert.Equal(t, "/home/user/work", runtimeGlobal.ExpandedEnv["WORK_DIR"])
				assert.Equal(t, "/home/user/config", runtimeGlobal.ExpandedEnv["CONFIG_DIR"])

				// Verify that verify_files are expanded
				require.NotNil(t, runtimeGlobal.ExpandedVerifyFiles)
				if len(runtimeGlobal.ExpandedVerifyFiles) > 0 {
					assert.Equal(t, "/home/user/config/app.conf", runtimeGlobal.ExpandedVerifyFiles[0])
				}

				// Verify group level expansion
				if len(cfg.Groups) > 0 {
					runtimeGroup, err := config.ExpandGroup(&cfg.Groups[0], runtimeGlobal)
					require.NoError(t, err)
					require.NotNil(t, runtimeGroup.ExpandedVars)
					require.NotNil(t, runtimeGroup.ExpandedEnv)
					assert.Equal(t, "allowed_value/data", runtimeGroup.ExpandedVars["group_path"])
					assert.Equal(t, "allowed_value/data", runtimeGroup.ExpandedEnv["GROUP_PATH"])
				}
			},
		},
		{
			name: "Multiple commands with different security contexts",
			systemEnv: map[string]string{
				"PUBLIC_VAR":  "public",
				"PRIVATE_VAR": "private",
			},
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config.toml")

				configContent := `
[global]
env_allowed = ["PUBLIC_VAR", "PRIVATE_VAR"]

[[groups]]
name = "public_group"
env_import = ["pub=PUBLIC_VAR"]
env_allowed = ["PUBLIC_VAR"]
env_vars = ["PUBLIC=%{pub}"]

[[groups.commands]]
name = "public_cmd"
cmd = "echo"
args = ["public"]

[[groups]]
name = "private_group"
env_import = ["priv=PRIVATE_VAR"]
env_allowed = ["PRIVATE_VAR"]
env_vars = ["PRIVATE=%{priv}"]

[[groups.commands]]
name = "private_cmd"
cmd = "echo"
args = ["private"]
`
				err := os.WriteFile(configPath, []byte(configContent), 0o644)
				require.NoError(t, err)

				return configPath
			},
			expectError: false,
			validate: func(t *testing.T, cfg *runnertypes.ConfigSpec) {
				// Verify that each group has proper isolation
				if len(cfg.Groups) == 2 {
					runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
					require.NoError(t, err)

					// Public group should have access to PUBLIC_VAR
					publicGroup, err := config.ExpandGroup(&cfg.Groups[0], runtimeGlobal)
					require.NoError(t, err)
					require.NotNil(t, publicGroup.ExpandedVars)
					assert.Equal(t, "public", publicGroup.ExpandedVars["pub"])

					// Private group should have access to PRIVATE_VAR
					privateGroup, err := config.ExpandGroup(&cfg.Groups[1], runtimeGlobal)
					require.NoError(t, err)
					require.NotNil(t, privateGroup.ExpandedVars)
					assert.Equal(t, "private", privateGroup.ExpandedVars["priv"])
				}
			},
		},
		{
			name: "Verify files security - path expansion with allowlist",
			systemEnv: map[string]string{
				"SAFE_DIR": "/tmp",
			},
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, "config.toml")

				// Create a test file in /tmp
				testFile := filepath.Join("/tmp", "test_security_file.txt")
				err := os.WriteFile(testFile, []byte("test"), 0o644)
				require.NoError(t, err)
				t.Cleanup(func() { os.Remove(testFile) })

				configContent := `
[global]
env_import = ["dir=SAFE_DIR"]
env_allowed = ["SAFE_DIR"]
verify_files = ["%{file_path}"]

[global.vars]
file_path = "%{dir}/test_security_file.txt"
`
				err = os.WriteFile(configPath, []byte(configContent), 0o644)
				require.NoError(t, err)

				return configPath
			},
			expectError: false,
			validate: func(t *testing.T, cfg *runnertypes.ConfigSpec) {
				// Verify that verify_files path is correctly expanded
				runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
				require.NoError(t, err)
				require.NotNil(t, runtimeGlobal.ExpandedVerifyFiles)
				require.Len(t, runtimeGlobal.ExpandedVerifyFiles, 1)
				assert.Equal(t, "/tmp/test_security_file.txt", runtimeGlobal.ExpandedVerifyFiles[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup system environment
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create config file
			configPath := tt.setup(t)

			// Read config file content
			content, err := os.ReadFile(configPath)
			require.NoError(t, err)

			// Load configuration using config loader directly
			cfgLoader := config.NewLoader()
			cfg, err := cfgLoader.LoadConfig(content)
			require.NoError(t, err) // LoadConfig should always succeed (it only parses TOML)
			require.NotNil(t, cfg)

			// Try to expand global configuration
			// This is where from_env allowlist validation happens
			_, expandErr := config.ExpandGlobal(&cfg.Global)

			// Check error expectations
			if tt.expectError {
				require.Error(t, expandErr)
				if tt.errorCheck != nil {
					tt.errorCheck(t, expandErr)
				}
			} else {
				require.NoError(t, expandErr)

				// Validate config
				if tt.validate != nil {
					tt.validate(t, cfg)
				}
			}
		})
	}
}
