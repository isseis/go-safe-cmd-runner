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
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/pelletier/go-toml/v2"
)

// Loader handles loading and validating configurations
type Loader struct{}

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
	return &Loader{}
}

// LoadConfig loads and validates the configuration from the given path
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
		cfg.Global.WorkDir = os.TempDir()
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

	return &cfg, nil
}
