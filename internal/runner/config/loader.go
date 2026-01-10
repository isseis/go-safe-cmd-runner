// Package config provides functionality for loading, validating, and expanding
// configuration files for the command runner. It supports TOML format and
// includes complete variable expansion for all environment variables, commands,
// and verify_files fields. All expansion processing is consolidated in this package.
package config

import (
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/pelletier/go-toml/v2"
)

// Loader handles loading and validating configurations
type Loader struct {
	fs common.FileSystem
}

// Error definitions for the config package
var (
	// ErrInvalidConfigPath is returned when the config file path is invalid
	ErrInvalidConfigPath = errors.New("invalid config file path")
)

// NewLoader creates a new config loader
func NewLoader() *Loader {
	return NewLoaderWithFS(common.NewDefaultFileSystem())
}

// NewLoaderWithFS creates a new config loader with a custom FileSystem
func NewLoaderWithFS(fs common.FileSystem) *Loader {
	return &Loader{
		fs: fs,
	}
}

// LoadConfigWithPath loads and validates configuration from a file path,
// processing includes and merging templates
func (l *Loader) LoadConfigWithPath(configPath string, content []byte) (*runnertypes.ConfigSpec, error) {
	// Process includes if present
	cfg, err := l.loadConfigWithIncludes(configPath, content, make(map[string]bool))
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadConfig loads and validates configuration from byte content instead of file path
// This prevents TOCTOU attacks by using already-verified file content
// Note: This function does not process includes. Use LoadConfigWithPath for full functionality.
func (l *Loader) LoadConfig(content []byte) (*runnertypes.ConfigSpec, error) {
	return l.loadConfigInternal(content, nil)
}

// loadConfigWithIncludes recursively loads config and processes includes
func (l *Loader) loadConfigWithIncludes(configPath string, content []byte, visited map[string]bool) (*runnertypes.ConfigSpec, error) {
	// Check for circular reference
	if visited[configPath] {
		return nil, &ErrCircularInclude{
			Path:  configPath,
			Chain: nil, // Will be populated by caller
		}
	}
	visited[configPath] = true

	// Load the main config (without includes processing)
	cfg, err := l.loadConfigInternal(content, nil)
	if err != nil {
		return nil, err
	}

	// Process includes if present
	if len(cfg.Includes) > 0 {
		templateSources, err := l.processIncludes(configPath, cfg.Includes, visited)
		if err != nil {
			return nil, err
		}

		// Merge templates
		if len(templateSources) > 0 {
			merger := NewDefaultTemplateMerger()
			mergedTemplates, err := merger.MergeTemplates(templateSources)
			if err != nil {
				return nil, err
			}

			// Merge with main config templates
			if cfg.CommandTemplates == nil {
				cfg.CommandTemplates = make(map[string]runnertypes.CommandTemplate)
			}

			// Check for duplicates with main config
			for name := range mergedTemplates {
				if _, exists := cfg.CommandTemplates[name]; exists {
					// Find the source of the duplicate
					var locations []string
					locations = append(locations, configPath) // Main config
					for _, src := range templateSources {
						if _, found := src.Templates[name]; found {
							locations = append(locations, src.FilePath)
							break
						}
					}
					return nil, &ErrDuplicateTemplateName{
						Name:      name,
						Locations: locations,
					}
				}
			}

			// Merge templates from includes into main config
			for name, tmpl := range mergedTemplates {
				cfg.CommandTemplates[name] = tmpl
			}
		}
	}

	return cfg, nil
}

// processIncludes loads all included template files
func (l *Loader) processIncludes(baseConfigPath string, includes []string, visited map[string]bool) ([]TemplateSource, error) {
	if len(includes) == 0 {
		return nil, nil
	}

	// Get base directory from config path
	baseDir := baseConfigPath
	if idx := len(baseDir) - 1; idx >= 0 {
		for ; idx >= 0; idx-- {
			if baseDir[idx] == '/' {
				baseDir = baseDir[:idx]
				break
			}
		}
		if idx < 0 {
			baseDir = "."
		}
	}

	resolver := NewDefaultPathResolver(l.fs)
	loader := NewDefaultTemplateFileLoader()

	var sources []TemplateSource

	for _, includePath := range includes {
		// Resolve path
		resolvedPath, err := resolver.ResolvePath(includePath, baseDir)
		if err != nil {
			return nil, err
		}

		// Check for circular reference
		if visited[resolvedPath] {
			return nil, &ErrCircularInclude{
				Path:  resolvedPath,
				Chain: nil,
			}
		}

		// Load template file
		templates, err := loader.LoadTemplateFile(resolvedPath)
		if err != nil {
			return nil, err
		}

		sources = append(sources, TemplateSource{
			FilePath:  resolvedPath,
			Templates: templates,
		})
	}

	return sources, nil
}

// loadConfigInternal loads and validates configuration from byte content
func (l *Loader) loadConfigInternal(content []byte, _ []TemplateSource) (*runnertypes.ConfigSpec, error) {
	// Parse the config content
	var cfg runnertypes.ConfigSpec
	if err := toml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Check for prohibited "name" field in command_templates
	if err := checkTemplateNameField(content); err != nil {
		return nil, err
	}

	// Apply default values
	ApplyGlobalDefaults(&cfg.Global)

	// Validate timeout values are non-negative
	if err := ValidateTimeouts(&cfg); err != nil {
		return nil, err
	}

	// Validate group names
	if err := ValidateGroupNames(&cfg); err != nil {
		return nil, err
	}

	// Validate command templates
	if err := ValidateTemplates(&cfg); err != nil {
		return nil, err
	}

	// Validate commands (exclusivity check)
	if err := ValidateCommands(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// checkTemplateNameField checks if any template definition qs a "name" field.
// This is done by parsing the TOML content as a map to detect fields that would be
// ignored by the struct unmarshaling.
func checkTemplateNameField(content []byte) error {
	var raw map[string]any
	if err := toml.Unmarshal(content, &raw); err != nil {
		// If we can't parse as map, the structured parse will also fail
		// so we can skip this check
		return nil
	}

	// Check command_templates section
	templates, ok := raw["command_templates"].(map[string]any)
	if !ok {
		return nil
	}

	for templateName, templateData := range templates {
		templateMap, ok := templateData.(map[string]any)
		if !ok {
			continue
		}

		// Check if "name" field exists
		if _, hasName := templateMap["name"]; hasName {
			return &ErrTemplateContainsNameField{
				TemplateName: templateName,
			}
		}
	}

	return nil
}

// ValidateTemplates validates all command templates in the configuration.
// It checks for:
// - Duplicate template names
// - Invalid template names
// - Invalid template definitions (NF-006: no %{} in template definitions)
// - Missing required fields in templates
func ValidateTemplates(cfg *runnertypes.ConfigSpec) error {
	if cfg == nil || cfg.CommandTemplates == nil {
		return nil
	}

	// Track seen template names to detect duplicates
	seen := make(map[string]bool)

	for name, tmpl := range cfg.CommandTemplates {
		// Check for duplicate names
		if seen[name] {
			return &ErrDuplicateTemplateName{Name: name}
		}
		seen[name] = true

		// Validate template name
		if err := ValidateTemplateName(name); err != nil {
			return err
		}

		// Validate template definition
		if err := ValidateTemplateDefinition(name, &tmpl); err != nil {
			return err
		}
	}

	return nil
}
