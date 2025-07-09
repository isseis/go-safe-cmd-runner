package config

import (
	"bytes"
	"log"
	"log/slog"
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

	// Check that warnings were logged
	logOutput := buf.String()
	expectedWarnings := []string{
		"privileged field is not yet implemented",
	}

	for _, warning := range expectedWarnings {
		if !strings.Contains(logOutput, warning) {
			t.Errorf("expected warning '%s' in log output: %s", warning, logOutput)
		}
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

func TestLoadConfigSecurityWarning(t *testing.T) {
	// Create a simple config file
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

	// Capture slog output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// Load config
	loader := NewLoader()
	cfg, err := loader.LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if cfg == nil {
		t.Fatal("LoadConfig() returned nil config")
	}

	// Check that security warning was logged
	logOutput := buf.String()
	expectedTexts := []string{
		"Configuration file integrity verification is not implemented",
		"phase=1",
		"security_risk=\"Configuration files may be tampered without detection\"",
		"recommendation=\"Enable verification in production environments\"",
	}

	for _, text := range expectedTexts {
		if !strings.Contains(logOutput, text) {
			t.Errorf("expected text '%s' in log output: %s", text, logOutput)
		}
	}

	// Verify config was loaded correctly
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
}
