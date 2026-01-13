//go:build test

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockTemplateLoader struct {
	Called bool
	Path   string
}

func (m *MockTemplateLoader) LoadTemplateFile(path string) (map[string]runnertypes.CommandTemplate, error) {
	m.Called = true
	m.Path = path
	return map[string]runnertypes.CommandTemplate{
		"mock_template": {
			Cmd: "mock",
		},
	}, nil
}

func TestLoader_SetTemplateLoader(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Create a dummy config that includes a template
	// We don't need to create the actual template file because we mock the loader
	content := []byte(`
version = "1.0"
includes = ["mock_template.toml"]
`)

	// Create the actual template file on disk so the PathResolver passes existence check
	// Content doesn't matter as we are mocking the loader
	err := os.WriteFile(filepath.Join(tmpDir, "mock_template.toml"), []byte(""), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewLoader()
	mockLoader := &MockTemplateLoader{}
	loader.SetTemplateLoader(mockLoader)

	// We expect LoadConfig to call our mock loader instead of reading the file
	cfg, err := loader.LoadConfig(configPath, content)

	require.NoError(t, err)
	assert.True(t, mockLoader.Called)
	assert.Contains(t, mockLoader.Path, "mock_template.toml")

	// Check if template from mock was loaded
	assert.Contains(t, cfg.CommandTemplates, "mock_template")
	assert.Equal(t, "mock", cfg.CommandTemplates["mock_template"].Cmd)
}
