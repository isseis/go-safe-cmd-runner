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
	cfg, err := loader.LoadConfig(configPath, content)

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
	cfg, err := loader.LoadConfig(configPath, configContent)

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
	cfg, err := loader.LoadConfig(configPath, configContent)

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
	cfg, err := loader.LoadConfig(configPath, configContent)

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
	_, err = loader.LoadConfig(configPath, configContent)

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
	_, err = loader.LoadConfig(configPath, configContent)

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
	_, err = loader.LoadConfig(configPath, configContent)

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
	_, err = loader.LoadConfig(configPath, configContent)

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
	cfg, err := loader.LoadConfigForTest(content)

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
	cfg, err := loader.LoadConfig(configPath, configContent)

	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Len(t, cfg.CommandTemplates, 2)
}

func TestLoadConfigWithPath_SymlinkBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure:
	// tmpDir/
	//   real_dir/
	//     config.toml (real file)
	//     templates.toml
	//   link_dir/
	//     config_link.toml -> ../real_dir/config.toml (symlink)
	realDir := filepath.Join(tmpDir, "real_dir")
	linkDir := filepath.Join(tmpDir, "link_dir")
	err := os.MkdirAll(realDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(linkDir, 0o755)
	require.NoError(t, err)

	// Create template file in real_dir
	templatePath := filepath.Join(realDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.test_template]
cmd = "echo"
args = ["from template"]
`)
	err = os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create real config file that includes relative path
	realConfigPath := filepath.Join(realDir, "config.toml")
	realConfigContent := []byte(`version = "1.0"
includes = ["templates.toml"]

[[groups]]
name = "test"
`)
	err = os.WriteFile(realConfigPath, realConfigContent, 0o644)
	require.NoError(t, err)

	// Create symlink in link_dir pointing to real config
	symlinkPath := filepath.Join(linkDir, "config_link.toml")
	err = os.Symlink(realConfigPath, symlinkPath)
	require.NoError(t, err)

	t.Run("access via real path - includes resolved from real_dir", func(t *testing.T) {
		loader := NewLoader()
		cfg, err := loader.LoadConfig(realConfigPath, realConfigContent)

		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Len(t, cfg.CommandTemplates, 1)
		assert.Equal(t, "echo", cfg.CommandTemplates["test_template"].Cmd)
	})

	t.Run("access via symlink - includes resolved from symlink location (link_dir)", func(t *testing.T) {
		// When accessing via symlink, includes are resolved relative to the symlink's directory (link_dir),
		// not the real file's directory (real_dir).
		// Since templates.toml does not exist in link_dir, this should fail.
		loader := NewLoader()
		cfg, err := loader.LoadConfig(symlinkPath, realConfigContent)

		// This should fail because templates.toml is not in link_dir
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "templates.toml")
	})

	t.Run("symlink with working relative path", func(t *testing.T) {
		// Create a case where relative path from symlink location works
		// Copy template to link_dir
		linkTemplateDir := linkDir
		linkTemplatePath := filepath.Join(linkTemplateDir, "templates.toml")
		err = os.WriteFile(linkTemplatePath, templateContent, 0o644)
		require.NoError(t, err)

		loader := NewLoader()
		cfg, err := loader.LoadConfig(symlinkPath, realConfigContent)

		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Len(t, cfg.CommandTemplates, 1)
		assert.Equal(t, "echo", cfg.CommandTemplates["test_template"].Cmd)
	})
}

func TestLoadConfig_WithVerifiedTemplateLoader(t *testing.T) {
	tmpDir := t.TempDir()

	// Create main config
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]

[[groups]]
name = "backup"

[[groups.commands]]
name = "backup_data"
template = "backup"

[groups.commands.params]
path = "/data"
`)
	err := os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]
`)
	err = os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create mock verified loader that tracks calls
	mockVerifiedLoader := &MockTemplateLoader{}

	// Create loader and set mock verified loader
	loader := NewLoader()
	loader.SetTemplateLoader(mockVerifiedLoader)

	// Load config - should use the mock loader for templates
	cfg, err := loader.LoadConfig(configPath, configContent)

	require.NoError(t, err)
	assert.NotNil(t, cfg)
	// Mock loader should have been called to load templates
	assert.True(t, mockVerifiedLoader.Called)
	assert.Contains(t, mockVerifiedLoader.Path, "templates.toml")
	// Template from mock should be present
	assert.Contains(t, cfg.CommandTemplates, "mock_template")
}
