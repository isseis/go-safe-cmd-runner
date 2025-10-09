//go:build test

// Package config provides test-only helper functions for configuration loading.
// These functions are only available when building with the "test" build tag.
package config

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/pelletier/go-toml/v2"
)

// LoadConfigFromString parses TOML content from a string and returns a processed configuration.
// This function is only available in test builds.
//
// Parameters:
//   - tomlContent: TOML configuration as a string
//   - filter: Filter for environment variable filtering (can be nil for no filtering)
//   - expander: VariableExpander for variable expansion (can be nil to skip expansion)
//
// Returns:
//   - *runnertypes.Config: Parsed and processed configuration
//   - error: Any error that occurred during parsing or processing
func LoadConfigFromString(tomlContent string, filter *environment.Filter, expander *environment.VariableExpander) (*runnertypes.Config, error) {
	// Parse TOML content
	cfg, err := parseTOMLContent(tomlContent)
	if err != nil {
		return nil, err
	}

	// If both filter and expander are provided, process the config
	if filter != nil && expander != nil {
		if err := processConfig(cfg, filter, expander); err != nil {
			return nil, fmt.Errorf("failed to process config: %w", err)
		}
	}

	return cfg, nil
}

// parseTOMLContent parses TOML content from a string and returns the parsed configuration.
// This function is only available in test builds.
//
// Parameters:
//   - tomlContent: TOML configuration as a string
//
// Returns:
//   - *runnertypes.Config: Parsed configuration
//   - error: Any error that occurred during parsing
func parseTOMLContent(tomlContent string) (*runnertypes.Config, error) {
	var cfg runnertypes.Config
	if err := toml.Unmarshal([]byte(tomlContent), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}
	return &cfg, nil
}
