package config

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigWithWarnings(t *testing.T) {
	// Create a temporary config file with unimplemented fields
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
    privileged = true
`

	// Write config to temporary file
	tmpFile, err := os.CreateTemp("", "test_config_*.toml")
	require.NoError(t, err, "failed to create temp file")
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err, "failed to write config")
	require.NoError(t, tmpFile.Close(), "failed to close temp file")

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Load config
	loader := NewLoader()
	cfg, err := loader.LoadConfig(tmpFile.Name())
	require.NoError(t, err, "LoadConfig() returned error")

	require.NotNil(t, cfg, "LoadConfig() returned nil config")

	// The privileged field is now implemented, so no warnings should be logged
	logOutput := buf.String()
	assert.False(t, strings.Contains(logOutput, "privileged field is not yet implemented"), "unexpected warning about privileged field in log output: %s", logOutput)

	// Verify config was loaded correctly despite warnings
	assert.Len(t, cfg.Groups, 1, "expected 1 group")

	assert.Len(t, cfg.Groups[0].Commands, 1, "expected 1 command")

	cmd := cfg.Groups[0].Commands[0]
	assert.Equal(t, "test_cmd", cmd.Name, "expected command name 'test_cmd'")

	assert.True(t, cmd.Privileged, "expected privileged to be true")
}

// TestLoadConfigSecurityWarning was removed as verification is now
// implemented and enabled by default through the verification package.
// This test was testing for security warnings when verification was not implemented (phase 1).
// The verification feature is now fully implemented.
