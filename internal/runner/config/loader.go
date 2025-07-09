// Package config provides functionality for loading and validating
// configuration files for the command runner. It supports TOML format
// and includes utilities for managing configuration settings.
package config

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/template"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/pelletier/go-toml/v2"
)

// Loader handles loading and validating configurations
type Loader struct {
	templateEngine *template.Engine
	fs             common.FileSystem
}

// Error definitions for the config package
var (
	// ErrInvalidConfigPath is returned when the config file path is invalid
	ErrInvalidConfigPath = errors.New("invalid config file path")
	// ErrWorkdirNotAbsolute is returned when the workdir is not an absolute path
	ErrWorkdirNotAbsolute = errors.New("workdir must be an absolute path")
	// ErrWorkdirHasRelativeComponents is returned when the workdir contains relative path components
	ErrWorkdirHasRelativeComponents = errors.New("workdir contains relative path components ('.' or '..')")
)

const (
	// defaultTimeout is the default timeout for commands in second (3600 = 1 hour)
	defaultTimeout = 3600
)

// NewLoader creates a new config loader
func NewLoader() *Loader {
	return NewLoaderWithFS(common.NewDefaultFileSystem())
}

// NewLoaderWithFS creates a new config loader with a custom FileSystem
func NewLoaderWithFS(fs common.FileSystem) *Loader {
	return &Loader{
		templateEngine: template.NewEngine(),
		fs:             fs,
	}
}

// GetTemplateEngine returns the template engine instance
func (l *Loader) GetTemplateEngine() *template.Engine {
	return l.templateEngine
}

// LoadConfig loads, validates, and applies templates to the configuration from the given path.
func (l *Loader) LoadConfig(path string) (*runnertypes.Config, error) {
	// TODO: Validate config file with checksum
	// Read the config file safely
	data, err := safefileio.SafeReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse the config file
	var cfg runnertypes.Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set default values if not specified
	if cfg.Global.WorkDir == "" {
		cfg.Global.WorkDir = l.fs.TempDir()
	}
	if cfg.Global.Timeout == 0 {
		cfg.Global.Timeout = defaultTimeout
	}
	if cfg.Global.LogLevel == "" {
		cfg.Global.LogLevel = "info"
	}

	// Validate work directory path
	workDir := cfg.Global.WorkDir
	if !filepath.IsAbs(workDir) {
		return nil, fmt.Errorf("%w: %s", ErrWorkdirNotAbsolute, workDir)
	}
	// Check if the path contains any relative components
	if workDir != filepath.Clean(workDir) || workDir != filepath.ToSlash(filepath.Clean(workDir)) {
		return nil, fmt.Errorf("%w: %s", ErrWorkdirHasRelativeComponents, workDir)
	}
	cfg.Global.WorkDir = workDir

	// Always apply templates
	if err := l.applyTemplates(&cfg); err != nil {
		return nil, err
	}

	// Check for deprecated fields and log warnings
	l.validateUnimplementedFields(&cfg)

	return &cfg, nil
}

// applyTemplates registers templates from configuration and applies them to command groups
func (l *Loader) applyTemplates(cfg *runnertypes.Config) error {
	// Register templates from configuration
	for name, tmplConfig := range cfg.Templates {
		tmpl := &template.Template{
			Name:        name,
			Description: tmplConfig.Description,
			Verify:      tmplConfig.Verify,
			TempDir:     tmplConfig.TempDir,
			Cleanup:     tmplConfig.Cleanup,
			WorkDir:     tmplConfig.WorkDir,
			Env:         tmplConfig.Env,
			Privileged:  tmplConfig.Privileged,
			Variables:   tmplConfig.Variables,
		}

		if err := l.templateEngine.RegisterTemplate(name, tmpl); err != nil {
			return fmt.Errorf("failed to register template %s: %w", name, err)
		}
	}

	// Apply templates to command groups
	for i, group := range cfg.Groups {
		if group.Template != "" {
			appliedGroup, err := l.templateEngine.ApplyTemplate(&group, group.Template)
			if err != nil {
				return fmt.Errorf("failed to apply template %s to group %s: %w", group.Template, group.Name, err)
			}
			cfg.Groups[i] = *appliedGroup
		}
	}

	return nil
}

// validateUnimplementedFields checks for unimplemented fields and logs warnings
func (l *Loader) validateUnimplementedFields(cfg *runnertypes.Config) {
	var warnings []string

	for _, group := range cfg.Groups {
		for _, cmd := range group.Commands {
			if cmd.Privileged {
				warnings = append(warnings, fmt.Sprintf(
					"command '%s': privileged field is not yet implemented",
					cmd.Name))
			}
		}
	}

	// Check templates for privileged field usage
	for templateName, tmpl := range cfg.Templates {
		if tmpl.Privileged {
			warnings = append(warnings, fmt.Sprintf(
				"template '%s': privileged field is not yet implemented",
				templateName))
		}
	}

	if len(warnings) > 0 {
		for _, warning := range warnings {
			log.Printf("Warning: %s", warning)
		}
	}
}
