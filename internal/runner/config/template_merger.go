package config

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// TemplateSource represents templates loaded from a single file.
// This structure is used during the merge process to track the origin
// of each template for error reporting.
type TemplateSource struct {
	// FilePath is the absolute path to the source file
	FilePath string

	// Templates is the map of template name to template definition
	Templates map[string]runnertypes.CommandTemplate
}

// TemplateMerger merges templates from multiple sources.
type TemplateMerger interface {
	// MergeTemplates merges templates from multiple sources.
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
	MergeTemplates(sources []TemplateSource) (map[string]runnertypes.CommandTemplate, error)
}

// DefaultTemplateMerger is the production implementation.
type DefaultTemplateMerger struct{}

// NewDefaultTemplateMerger creates a new DefaultTemplateMerger.
func NewDefaultTemplateMerger() *DefaultTemplateMerger {
	return &DefaultTemplateMerger{}
}

// MergeTemplates merges templates from multiple sources.
func (m *DefaultTemplateMerger) MergeTemplates(sources []TemplateSource) (map[string]runnertypes.CommandTemplate, error) {
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
