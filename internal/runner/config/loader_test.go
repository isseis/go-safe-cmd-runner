package config

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
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
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Load config
	loader := NewLoader()
	cfg, err := loader.LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg == nil {
		t.Fatal("LoadConfig() returned nil config")
	}

	// The privileged field is now implemented, so no warnings should be logged
	logOutput := buf.String()
	if strings.Contains(logOutput, "privileged field is not yet implemented") {
		t.Errorf("unexpected warning about privileged field in log output: %s", logOutput)
	}

	// Verify config was loaded correctly despite warnings
	if len(cfg.Groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(cfg.Groups))
	}

	if len(cfg.Groups[0].Commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(cfg.Groups[0].Commands))
	}

	cmd := cfg.Groups[0].Commands[0]
	if cmd.Name != "test_cmd" {
		t.Errorf("expected command name 'test_cmd', got '%s'", cmd.Name)
	}

	if !cmd.Privileged {
		t.Error("expected privileged to be true")
	}
}

// TestLoadConfigSecurityWarning was removed as verification is now
// implemented and enabled by default through the verification package.
// This test was testing for security warnings when verification was not implemented (phase 1).
// The verification feature is now fully implemented.
