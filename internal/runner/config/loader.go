// Package config provides functionality for loading, validating, and expanding
// configuration files for the command runner. It supports TOML format and
// includes complete variable expansion for all environment variables, commands,
// and verify_files fields. All expansion processing is consolidated in this package.
package config

import (
	"errors"
	"fmt"
	"path/filepath"

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

// LoadConfig loads and validates configuration from a file path,
// processing includes and merging templates
func (l *Loader) LoadConfig(configPath string, content []byte) (*runnertypes.ConfigSpec, error) {
	// Process includes if present
	cfg, err := l.loadConfigWithIncludes(configPath, content, make(map[string]struct{}))
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadConfigWithIncludes recursively loads config and processes includes
func (l *Loader) loadConfigWithIncludes(configPath string, content []byte, visited map[string]struct{}) (*runnertypes.ConfigSpec, error) {
	// Check for circular reference
	if _, exists := visited[configPath]; exists {
		return nil, &ErrCircularInclude{
			Path:  configPath,
			Chain: nil, // Will be populated by caller
		}
	}
	visited[configPath] = struct{}{}

	// Load the main config (without includes processing)
	cfg, err := l.loadConfigInternal(content)
	if err != nil {
		return nil, err
	}

	// Process includes if present
	templateSources, err := l.processIncludes(configPath, cfg.Includes, visited)
	if err != nil {
		return nil, err
	}

	// Add the main config's templates as a source to let the merger handle all
	// duplicate checks and merging in one pass.
	if len(cfg.CommandTemplates) > 0 {
		templateSources = append(templateSources, TemplateSource{
			FilePath:  configPath,
			Templates: cfg.CommandTemplates,
		})
	}

	// Merge all templates. mergeTemplates will handle duplicate detection across all sources.
	if len(templateSources) > 0 {
		mergedTemplates, err := mergeTemplates(templateSources)
		if err != nil {
			return nil, err
		}
		cfg.CommandTemplates = mergedTemplates
	}

	return cfg, nil
}

// TemplateSource represents templates loaded from a single file.
// This structure is used during the merge process to track the origin
// of each template for error reporting.
type TemplateSource struct {
	// FilePath is the absolute path to the source file
	FilePath string

	// Templates is the map of template name to template definition
	Templates map[string]runnertypes.CommandTemplate
}

// processIncludes loads all included template files
func (l *Loader) processIncludes(baseConfigPath string, includes []string, visited map[string]struct{}) ([]TemplateSource, error) {
	if len(includes) == 0 {
		return nil, nil
	}

	// Get base directory from config path
	baseDir := filepath.Dir(baseConfigPath)

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
		if _, exists := visited[resolvedPath]; exists {
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

// mergeTemplates merges templates from multiple sources.
//
// Parameters:
//   - sources: List of template sources (in order)
//
// Returns:
//   - Merged map of template name to CommandTemplate
//   - Error if duplicate template names are found
//
// Behavior:
//   - Sources are processed in order
//   - Duplicate names across sources cause an error
//   - Error message includes all locations where duplicate is defined
func mergeTemplates(sources []TemplateSource) (map[string]runnertypes.CommandTemplate, error) {
	// Map to store merged templates
	merged := make(map[string]runnertypes.CommandTemplate)

	// Map to track the file where each template is defined
	locations := make(map[string][]string)

	// Process each source in order
	for _, source := range sources {
		for name, template := range source.Templates {
			// Check for duplicates
			if _, exists := merged[name]; exists {
				// Record this location
				locations[name] = append(locations[name], source.FilePath)

				// Return error with all locations
				return nil, &ErrDuplicateTemplateName{
					Name:      name,
					Locations: locations[name],
				}
			}

			// Add template to merged map
			merged[name] = template

			// Record location (for potential future error)
			locations[name] = []string{source.FilePath}
		}
	}

	return merged, nil
}

// loadConfigInternal loads and validates configuration from byte content
func (l *Loader) loadConfigInternal(content []byte) (*runnertypes.ConfigSpec, error) {
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
