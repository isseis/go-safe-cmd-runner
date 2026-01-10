package config

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// TemplateFileSpec represents the structure of a template file.
// Template files can only contain 'version' and 'command_templates'.
// Any other fields will cause an error when parsed with DisallowUnknownFields().
type TemplateFileSpec struct {
	// Version specifies the template file version (e.g., "1.0")
	Version string `toml:"version"`

	// CommandTemplates contains template definitions
	CommandTemplates map[string]runnertypes.CommandTemplate `toml:"command_templates"`
}
