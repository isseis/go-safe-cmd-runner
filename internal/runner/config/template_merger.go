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
