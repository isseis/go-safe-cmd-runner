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
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/pelletier/go-toml/v2"
)

// Loader handles loading and validating configurations
type Loader struct {
	fs              common.FileSystem
	verificationMgr *verification.Manager
}

// Error definitions for the config package
var (
	// ErrInvalidConfigPath is returned when the config file path is invalid
	ErrInvalidConfigPath = errors.New("invalid config file path")

	// ErrSlackWebhookInTOML is returned when slack_webhook_url is found in the TOML config.
	// Slack webhook URLs must be configured via environment variables, not in the config file.
	ErrSlackWebhookInTOML = errors.New("slack_webhook_url in TOML is not supported")
)

// NewLoader creates a new config loader with specified dependencies.
// Both fs and verificationManager are required - nil values will panic.
// This constructor is designed for production use where verification is mandatory.
func NewLoader(fs common.FileSystem, verificationManager *verification.Manager) *Loader {
	if fs == nil {
		panic("FileSystem is required: programming error")
	}
	if verificationManager == nil {
		panic("VerificationManager is required: programming error")
	}
	return &Loader{
		fs:              fs,
		verificationMgr: verificationManager,
	}
}

// newLoaderInternal creates a new config loader without nil checks.
// This is an internal helper for testing code that needs to pass nil values.
// ONLY use this in test code - production code must use NewLoader.
func newLoaderInternal(fs common.FileSystem, verificationManager *verification.Manager) *Loader {
	return &Loader{
		fs:              fs,
		verificationMgr: verificationManager,
	}
}

// NewLoaderForTest creates a new config loader with default dependencies for testing.
// This convenience constructor should only be used in test code.
// It allows verificationManager to be nil for tests that don't need verification.
func NewLoaderForTest() *Loader {
	return newLoaderInternal(common.NewDefaultFileSystem(), nil)
}

// LoadConfig loads and validates configuration from a file path,
// processing includes and merging templates
func (l *Loader) LoadConfig(configPath string, content []byte) (*runnertypes.ConfigSpec, error) {
	// Process includes if present
	cfg, err := l.loadConfigWithIncludes(configPath, content)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadConfigWithIncludes loads config and processes includes.
// Note: Circular includes are not possible because template files cannot contain includes.
func (l *Loader) loadConfigWithIncludes(configPath string, content []byte) (*runnertypes.ConfigSpec, error) {
	// Load the main config (without includes processing)
	cfg, err := l.loadConfigInternal(content)
	if err != nil {
		return nil, err
	}

	// Process includes if present
	templateSources, err := l.processIncludes(configPath, cfg.Includes)
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

// loadTemplate loads a template file using verification if manager is available.
func (l *Loader) loadTemplate(path string) (map[string]runnertypes.CommandTemplate, error) {
	var content []byte
	var err error

	// In production code, verificationMgr is always non-nil (guaranteed by NewLoader).
	// In test code, it may be nil (via NewLoaderForTest with newLoaderInternal).
	if l.verificationMgr != nil {
		content, err = l.verificationMgr.VerifyAndReadTemplateFile(path)
	} else {
		// Test path: read without verification
		// This is only used in test code via NewLoaderForTest() -> newLoaderInternal()
		content, err = safefileio.SafeReadFile(path)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read template file %s: %w", path, err)
	}

	return ParseTemplateContent(content, path)
}

// processIncludes loads all included template files
func (l *Loader) processIncludes(baseConfigPath string, includes []string) ([]TemplateSource, error) {
	if len(includes) == 0 {
		return nil, nil
	}

	// Get base directory from config path
	baseDir := filepath.Dir(baseConfigPath)

	resolver := NewDefaultPathResolver(l.fs)

	var sources []TemplateSource

	for _, includePath := range includes {
		// Resolve path
		resolvedPath, err := resolver.ResolvePath(includePath, baseDir)
		if err != nil {
			return nil, err
		}

		// Load template file
		templates, err := l.loadTemplate(resolvedPath)
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
//   - First pass: collect all template locations across all sources
//   - Check for duplicates (templates defined in multiple files)
//   - Second pass: build merged map if no duplicates found
//   - Error message includes ALL locations where duplicate is defined
func mergeTemplates(sources []TemplateSource) (map[string]runnertypes.CommandTemplate, error) {
	// First pass: collect all locations for each template name
	locations := make(map[string][]string)
	templates := make(map[string]runnertypes.CommandTemplate)

	for _, source := range sources {
		for name, template := range source.Templates {
			locations[name] = append(locations[name], source.FilePath)
			// Keep the first definition for the merged result
			if _, exists := templates[name]; !exists {
				templates[name] = template
			}
		}
	}

	// Check for duplicates and return error with all locations
	for name, locs := range locations {
		if len(locs) > 1 {
			return nil, &ErrDuplicateTemplateName{
				Name:      name,
				Locations: locs,
			}
		}
	}

	return templates, nil
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

	// Check for slack_webhook_url in [global] section (not supported in TOML)
	if err := checkSlackWebhookField(content); err != nil {
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

	// Validate working directories (absolute path requirement)
	if err := ValidateGroupWorkDirs(&cfg); err != nil {
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

// checkSlackWebhookField checks if the TOML config contains a slack_webhook_url field
// in the [global] section. This field is not supported in TOML - Slack webhook URLs
// must be configured via environment variables (GSCR_SLACK_WEBHOOK_URL_SUCCESS,
// GSCR_SLACK_WEBHOOK_URL_ERROR).
func checkSlackWebhookField(content []byte) error {
	var raw map[string]any
	if err := toml.Unmarshal(content, &raw); err != nil {
		// If we can't parse as map, the structured parse will also fail
		// so we can skip this check
		return nil
	}

	// Check global section
	globalSection, ok := raw["global"].(map[string]any)
	if !ok {
		return nil
	}

	// Check for slack_webhook_url field (deprecated)
	if _, hasField := globalSection["slack_webhook_url"]; hasField {
		return fmt.Errorf("%w: use GSCR_SLACK_WEBHOOK_URL_SUCCESS and GSCR_SLACK_WEBHOOK_URL_ERROR environment variables instead", ErrSlackWebhookInTOML)
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
