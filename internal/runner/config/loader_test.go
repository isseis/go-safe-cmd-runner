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
