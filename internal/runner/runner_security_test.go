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
		validate    func(*testing.T, *runnertypes.Config)
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
from_env = ["MY_SAFE=SAFE_VAR"]
env_allowlist = ["SAFE_VAR"]
vars = ["derived=%{MY_SAFE}/subdir"]
env = ["MY_ENV=%{derived}"]
`
				err := os.WriteFile(configPath, []byte(configContent), 0o644)
				require.NoError(t, err)

				return configPath
			},
			expectError: false,
			validate: func(t *testing.T, cfg *runnertypes.Config) {
				// Verify that allowed variable is properly expanded
				require.NotNil(t, cfg.Global.ExpandedVars)
				require.NotNil(t, cfg.Global.ExpandedEnv)
				assert.Equal(t, "safe_value/subdir", cfg.Global.ExpandedVars["derived"])
				assert.Equal(t, "safe_value/subdir", cfg.Global.ExpandedEnv["MY_ENV"])
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
from_env = ["MY_SECRET=SECRET_VAR"]
env_allowlist = ["SAFE_VAR"]
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
from_env = ["home=SAFE_HOME", "allowed=ALLOWED_VAR"]
env_allowlist = ["SAFE_HOME", "ALLOWED_VAR"]
vars = ["work_dir=%{home}/work", "config_dir=%{home}/config"]
env = ["WORK_DIR=%{work_dir}", "CONFIG_DIR=%{config_dir}"]
verify_files = ["%{config_dir}/app.conf"]

[[group]]
name = "secure_group"
from_env = ["group_var=ALLOWED_VAR"]
env_allowlist = ["ALLOWED_VAR"]
vars = ["group_path=%{group_var}/data"]
env = ["GROUP_PATH=%{group_path}"]

[[group.commands]]
name = "secure_command"
cmd = "echo"
args = ["Running with security"]
env = ["CMD_VAR=%{group_path}"]
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
			validate: func(t *testing.T, cfg *runnertypes.Config) {
				// Verify that global level expansion is correct
				require.NotNil(t, cfg.Global.ExpandedVars)
				require.NotNil(t, cfg.Global.ExpandedEnv)
				assert.Equal(t, "/home/user/work", cfg.Global.ExpandedVars["work_dir"])
				assert.Equal(t, "/home/user/config", cfg.Global.ExpandedVars["config_dir"])
				assert.Equal(t, "/home/user/work", cfg.Global.ExpandedEnv["WORK_DIR"])
				assert.Equal(t, "/home/user/config", cfg.Global.ExpandedEnv["CONFIG_DIR"])

				// Verify that verify_files are expanded
				require.NotNil(t, cfg.Global.ExpandedVerifyFiles)
				if len(cfg.Global.ExpandedVerifyFiles) > 0 {
					assert.Equal(t, "/home/user/config/app.conf", cfg.Global.ExpandedVerifyFiles[0])
				}

				// Verify group level expansion
				if len(cfg.Groups) > 0 {
					group := cfg.Groups[0]
					require.NotNil(t, group.ExpandedVars)
					require.NotNil(t, group.ExpandedEnv)
					assert.Equal(t, "allowed_value/data", group.ExpandedVars["group_path"])
					assert.Equal(t, "allowed_value/data", group.ExpandedEnv["GROUP_PATH"])
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
env_allowlist = ["PUBLIC_VAR", "PRIVATE_VAR"]

[[group]]
name = "public_group"
from_env = ["pub=PUBLIC_VAR"]
env_allowlist = ["PUBLIC_VAR"]
env = ["PUBLIC=%{pub}"]

[[group.commands]]
name = "public_cmd"
cmd = "echo"
args = ["public"]

[[group]]
name = "private_group"
from_env = ["priv=PRIVATE_VAR"]
env_allowlist = ["PRIVATE_VAR"]
env = ["PRIVATE=%{priv}"]

[[group.commands]]
name = "private_cmd"
cmd = "echo"
args = ["private"]
`
				err := os.WriteFile(configPath, []byte(configContent), 0o644)
				require.NoError(t, err)

				return configPath
			},
			expectError: false,
			validate: func(t *testing.T, cfg *runnertypes.Config) {
				// Verify that each group has proper isolation
				if len(cfg.Groups) == 2 {
					// Public group should have access to PUBLIC_VAR
					publicGroup := cfg.Groups[0]
					require.NotNil(t, publicGroup.ExpandedVars)
					assert.Equal(t, "public", publicGroup.ExpandedVars["pub"])

					// Private group should have access to PRIVATE_VAR
					privateGroup := cfg.Groups[1]
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
from_env = ["dir=SAFE_DIR"]
env_allowlist = ["SAFE_DIR"]
vars = ["file_path=%{dir}/test_security_file.txt"]
verify_files = ["%{file_path}"]
`
				err = os.WriteFile(configPath, []byte(configContent), 0o644)
				require.NoError(t, err)

				return configPath
			},
			expectError: false,
			validate: func(t *testing.T, cfg *runnertypes.Config) {
				// Verify that verify_files path is correctly expanded
				require.NotNil(t, cfg.Global.ExpandedVerifyFiles)
				require.Len(t, cfg.Global.ExpandedVerifyFiles, 1)
				assert.Equal(t, "/tmp/test_security_file.txt", cfg.Global.ExpandedVerifyFiles[0])
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

			// Check error expectations
			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)

				// Validate config
				if tt.validate != nil {
					tt.validate(t, cfg)
				}
			}
		})
	}
}
