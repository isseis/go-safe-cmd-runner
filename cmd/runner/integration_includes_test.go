//go:build test

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_IncludesFeature_SingleInclude tests loading a config with a single include file
func TestIntegration_IncludesFeature_SingleInclude(t *testing.T) {
	tmpDir := t.TempDir()

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]
`)
	err := os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create main config
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]

[global]
timeout = 60

[command_templates.test]
cmd = "echo"
args = ["hello"]

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["hello"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	hashDir := filepath.Join(tmpDir, "hashes")
	err = os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithDryRunMode())
	require.NoError(t, err)

	// Load config using bootstrap
	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-001")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify templates are merged
	assert.Len(t, cfg.CommandTemplates, 2)
	assert.Equal(t, "restic", cfg.CommandTemplates["backup"].Cmd)
	assert.Equal(t, "echo", cfg.CommandTemplates["test"].Cmd)

	// Verify groups are loaded
	assert.Len(t, cfg.Groups, 1)
	assert.Equal(t, "test_group", cfg.Groups[0].Name)
}

// TestIntegration_IncludesFeature_MultipleIncludes tests loading a config with multiple include files
func TestIntegration_IncludesFeature_MultipleIncludes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create first template file
	template1Path := filepath.Join(tmpDir, "backup.toml")
	template1Content := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
args = ["backup"]
`)
	err := os.WriteFile(template1Path, template1Content, 0o644)
	require.NoError(t, err)

	// Create second template file
	template2Path := filepath.Join(tmpDir, "restore.toml")
	template2Content := []byte(`version = "1.0"

[command_templates.restore]
cmd = "restic"
args = ["restore"]
`)
	err = os.WriteFile(template2Path, template2Content, 0o644)
	require.NoError(t, err)

	// Create main config
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["backup.toml", "restore.toml"]

[global]
timeout = 60
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	hashDir := filepath.Join(tmpDir, "hashes")
	err = os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithDryRunMode())
	require.NoError(t, err)

	// Load config
	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-002")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify all templates are merged
	assert.Len(t, cfg.CommandTemplates, 2)
	assert.Equal(t, "restic", cfg.CommandTemplates["backup"].Cmd)
	assert.Equal(t, "restic", cfg.CommandTemplates["restore"].Cmd)

	// Expand global and verify template usage
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// Validate templates
	err = config.ValidateAllTemplates(cfg.CommandTemplates, runtimeGlobal.ExpandedVars)
	require.NoError(t, err)
}

// TestIntegration_IncludesFeature_RelativePath tests include paths with relative references
func TestIntegration_IncludesFeature_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "templates")
	err := os.MkdirAll(subDir, 0o755)
	require.NoError(t, err)

	// Create template file in subdirectory
	templatePath := filepath.Join(subDir, "common.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	err = os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create main config
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates/common.toml"]

[global]
timeout = 60
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	hashDir := filepath.Join(tmpDir, "hashes")
	err = os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithDryRunMode())
	require.NoError(t, err)

	// Load config
	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-003")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify template is loaded
	assert.Len(t, cfg.CommandTemplates, 1)
	assert.Equal(t, "restic", cfg.CommandTemplates["backup"].Cmd)
}

// TestIntegration_IncludesFeature_DuplicateTemplate tests error handling for duplicate template names
func TestIntegration_IncludesFeature_DuplicateTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create template file with duplicate name
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.test]
cmd = "other"
`)
	err := os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create main config with same template name
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]

[global]
timeout = 60

[command_templates.test]
cmd = "echo"
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	hashDir := filepath.Join(tmpDir, "hashes")
	err = os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithDryRunMode())
	require.NoError(t, err)

	// Load config should fail with duplicate error
	_, err = bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-004")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate command template name")
	assert.Contains(t, err.Error(), "test")
}

// TestIntegration_IncludesFeature_IncludeNotFound tests error handling for missing include files
func TestIntegration_IncludesFeature_IncludeNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Create main config referencing non-existent include
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["nonexistent.toml"]

[global]
timeout = 60
`)
	err := os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	hashDir := filepath.Join(tmpDir, "hashes")
	err = os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithDryRunMode())
	require.NoError(t, err)

	// Load config should fail with file not found error
	_, err = bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-005")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "included file not found")
	assert.Contains(t, err.Error(), "nonexistent.toml")
}

// TestIntegration_IncludesFeature_InvalidTemplateFile tests error handling for invalid template files
func TestIntegration_IncludesFeature_InvalidTemplateFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid template file (contains disallowed field)
	templatePath := filepath.Join(tmpDir, "invalid.toml")
	templateContent := []byte(`version = "1.0"

[global]
timeout = 60

[command_templates.test]
cmd = "echo"
`)
	err := os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create main config
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["invalid.toml"]

[global]
timeout = 60
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	hashDir := filepath.Join(tmpDir, "hashes")
	err = os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithDryRunMode())
	require.NoError(t, err)

	// Load config should fail with invalid format error
	_, err = bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-006")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template file contains invalid fields")
	assert.Contains(t, err.Error(), "invalid.toml")
}

// TestIntegration_IncludesFeature_BackwardCompatibility tests that configs without includes still work
func TestIntegration_IncludesFeature_BackwardCompatibility(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config without includes
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"

[global]
timeout = 60

[command_templates.test]
cmd = "echo"
args = ["hello"]
`)
	err := os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	hashDir := filepath.Join(tmpDir, "hashes")
	err = os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithDryRunMode())
	require.NoError(t, err)

	// Load config should work without includes
	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-007")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify config is loaded correctly
	assert.Len(t, cfg.CommandTemplates, 1)
	assert.Equal(t, "echo", cfg.CommandTemplates["test"].Cmd)
}
