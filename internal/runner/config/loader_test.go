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
