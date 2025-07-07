// Package config provides functionality for loading and validating
// configuration files for the command runner. It supports TOML format
// and includes utilities for managing configuration settings.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/pelletier/go-toml/v2"
)

// Loader handles loading and validating configurations
type Loader struct{}

// Error definitions for the config package
var (
	// ErrInvalidConfigPath is returned when the config file path is invalid
	ErrInvalidConfigPath = errors.New("invalid config file path")
)

const (
	// defaultTimeout is the default timeout for commands in second (3600 = 1 hour)
	defaultTimeout = 3600
)

// NewLoader creates a new config loader
func NewLoader() *Loader {
	return &Loader{}
}

// LoadConfig loads and validates the configuration from the given path
func (l *Loader) LoadConfig(path string) (*runnertypes.Config, error) {
	// TODO: Validate config file with checksum
	// Sanitize the path to prevent directory traversal
	if !filepath.IsLocal(path) && !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfigPath, path)
	}

	// Read the config file
	data, err := os.ReadFile(filepath.Clean(path))
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
		cfg.Global.WorkDir = os.TempDir()
	}
	if cfg.Global.Timeout == 0 {
		cfg.Global.Timeout = defaultTimeout
	}
	if cfg.Global.LogLevel == "" {
		cfg.Global.LogLevel = "info"
	}

	// Convert relative paths to absolute
	if !filepath.IsAbs(cfg.Global.WorkDir) {
		absPath, err := filepath.Abs(cfg.Global.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for workdir: %w", err)
		}
		cfg.Global.WorkDir = absPath
	}

	return &cfg, nil
}
