package config

import (
	"bytes"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/pelletier/go-toml/v2"
)

// TemplateFileSpec represents the structure of a template file.
// Template files can only contain 'version' and 'command_templates'.
// Any other fields will cause an error when parsed with DisallowUnknownFields().
type TemplateFileSpec struct {
	// Version specifies the template file version (e.g., "1.0")
	Version string `toml:"version"`

	// CommandTemplates contains template definitions
	CommandTemplates map[string]runnertypes.CommandTemplate `toml:"command_templates"`
}

// TemplateFileLoaderFunc defines the signature for loading template files.
type TemplateFileLoaderFunc func(path string) (map[string]runnertypes.CommandTemplate, error)

// LoadTemplateFile loads a template file from the given path.
// This is the default implementation that loads from the filesystem.
func LoadTemplateFile(path string) (map[string]runnertypes.CommandTemplate, error) {
	// Step 1: Read file content using safefileio for security
	content, err := safefileio.SafeReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file %s: %w", path, err)
	}

	return ParseTemplateContent(content, path)
}

// ParseTemplateContent parses the content of a template file.
func ParseTemplateContent(content []byte, path string) (map[string]runnertypes.CommandTemplate, error) {
	// Step 2: Create decoder with DisallowUnknownFields
	decoder := toml.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()

	// Step 3: Parse into TemplateFileSpec
	var spec TemplateFileSpec
	if err := decoder.Decode(&spec); err != nil {
		// Check if error is due to unknown field
		return nil, &ErrTemplateFileInvalidFormat{
			TemplateFile: path,
			ParseError:   err,
		}
	}

	// Step 4: Return command_templates
	// Note: spec.CommandTemplates may be nil if not defined
	if spec.CommandTemplates == nil {
		return make(map[string]runnertypes.CommandTemplate), nil
	}

	return spec.CommandTemplates, nil
}
