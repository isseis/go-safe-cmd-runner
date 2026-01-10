//go:build test

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigWithPath_NoIncludes(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	content := []byte(`version = "1.0"

[command_templates.test]
cmd = "echo"
args = ["hello"]
`)

	err := os.WriteFile(configPath, content, 0o644)
	require.NoError(t, err)

	loader := NewLoader()
	cfg, err := loader.LoadConfigWithPath(configPath, content)

	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Len(t, cfg.CommandTemplates, 1)
	assert.Equal(t, "echo", cfg.CommandTemplates["test"].Cmd)
}

func TestLoadConfigWithPath_SingleInclude(t *testing.T) {
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

[command_templates.test]
cmd = "echo"
args = ["hello"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	loader := NewLoader()
	cfg, err := loader.LoadConfigWithPath(configPath, configContent)

	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Len(t, cfg.CommandTemplates, 2)
	assert.Equal(t, "echo", cfg.CommandTemplates["test"].Cmd)
	assert.Equal(t, "restic", cfg.CommandTemplates["backup"].Cmd)
}

func TestLoadConfigWithPath_MultipleIncludes(t *testing.T) {
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

[command_templates.test]
cmd = "echo"
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	loader := NewLoader()
	cfg, err := loader.LoadConfigWithPath(configPath, configContent)

	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Len(t, cfg.CommandTemplates, 3)
	assert.Equal(t, "echo", cfg.CommandTemplates["test"].Cmd)
	assert.Equal(t, "restic", cfg.CommandTemplates["backup"].Cmd)
	assert.Equal(t, "restic", cfg.CommandTemplates["restore"].Cmd)
}

func TestLoadConfigWithPath_RelativePath(t *testing.T) {
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

[command_templates.test]
cmd = "echo"
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	loader := NewLoader()
	cfg, err := loader.LoadConfigWithPath(configPath, configContent)

	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Len(t, cfg.CommandTemplates, 2)
}

func TestLoadConfigWithPath_DuplicateTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.test]
cmd = "other"
`)
	err := os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create main config with duplicate template name
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]

[command_templates.test]
cmd = "echo"
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	loader := NewLoader()
	_, err = loader.LoadConfigWithPath(configPath, configContent)

	require.Error(t, err)
	var errDup *ErrDuplicateTemplateName
	require.ErrorAs(t, err, &errDup)
	assert.Equal(t, "test", errDup.Name)
}

func TestLoadConfigWithPath_DuplicateAcrossIncludes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create first template file
	template1Path := filepath.Join(tmpDir, "template1.toml")
	template1Content := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	err := os.WriteFile(template1Path, template1Content, 0o644)
	require.NoError(t, err)

	// Create second template file with duplicate
	template2Path := filepath.Join(tmpDir, "template2.toml")
	template2Content := []byte(`version = "1.0"

[command_templates.backup]
cmd = "borg"
`)
	err = os.WriteFile(template2Path, template2Content, 0o644)
	require.NoError(t, err)

	// Create main config
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["template1.toml", "template2.toml"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	loader := NewLoader()
	_, err = loader.LoadConfigWithPath(configPath, configContent)

	require.Error(t, err)
	var errDup *ErrDuplicateTemplateName
	require.ErrorAs(t, err, &errDup)
	assert.Equal(t, "backup", errDup.Name)
}

func TestLoadConfigWithPath_CircularInclude(t *testing.T) {
	t.Skip("Circular includes are not possible with current design - template files cannot have includes field")
}

func TestLoadConfigWithPath_IncludeNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["nonexistent.toml"]
`)
	err := os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	loader := NewLoader()
	_, err = loader.LoadConfigWithPath(configPath, configContent)

	require.Error(t, err)
	var errNotFound *ErrIncludedFileNotFound
	require.ErrorAs(t, err, &errNotFound)
}

func TestLoadConfigWithPath_InvalidTemplateFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid template file (with disallowed field)
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
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	loader := NewLoader()
	_, err = loader.LoadConfigWithPath(configPath, configContent)

	require.Error(t, err)
	var errInvalid *ErrTemplateFileInvalidFormat
	require.ErrorAs(t, err, &errInvalid)
}

func TestLoadConfig_BackwardCompatibility(t *testing.T) {
	// Test that LoadConfig still works without includes
	content := []byte(`version = "1.0"

[command_templates.test]
cmd = "echo"
args = ["hello"]
`)

	loader := NewLoader()
	cfg, err := loader.LoadConfig(content)

	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Len(t, cfg.CommandTemplates, 1)
	assert.Equal(t, "echo", cfg.CommandTemplates["test"].Cmd)
}

func TestLoadConfigWithPath_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	err := os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create main config with absolute path
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["` + templatePath + `"]

[command_templates.test]
cmd = "echo"
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	loader := NewLoader()
	cfg, err := loader.LoadConfigWithPath(configPath, configContent)

	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Len(t, cfg.CommandTemplates, 2)
}
