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
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
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
	// ErrWorkdirNotAbsolute is returned when the workdir is not an absolute path
	ErrWorkdirNotAbsolute = errors.New("workdir must be an absolute path")
	// ErrWorkdirHasRelativeComponents is returned when the workdir contains relative path components
	ErrWorkdirHasRelativeComponents = errors.New("workdir contains relative path components ('.' or '..')")
)

const (
	// defaultTimeout is the default timeout for commands in second (600 = 10 minutes)
	defaultTimeout = 600
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

// LoadConfig loads and validates configuration from byte content instead of file path
// This prevents TOCTOU attacks by using already-verified file content
func (l *Loader) LoadConfig(content []byte) (*runnertypes.Config, error) {
	// Parse the config content
	var cfg runnertypes.Config
	if err := toml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set default values if not specified
	if cfg.Global.WorkDir == "" {
		cfg.Global.WorkDir = l.fs.TempDir()
	}
	if cfg.Global.Timeout == 0 {
		cfg.Global.Timeout = defaultTimeout
	}
	if cfg.Global.LogLevel == "" {
		cfg.Global.LogLevel = "info"
	}
	if cfg.Global.MaxOutputSize == 0 {
		cfg.Global.MaxOutputSize = output.DefaultMaxOutputSize
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

	// Validate that user-defined environment variables do not use reserved prefix
	if err := l.validateEnvironmentVariables(&cfg); err != nil {
		return nil, fmt.Errorf("environment variable validation failed: %w", err)
	}

	// Create Filter for environment variable filtering
	filter := environment.NewFilter(cfg.Global.EnvAllowlist)

	// Process config (expand variables)
	if err := processConfig(&cfg, filter); err != nil {
		return nil, fmt.Errorf("failed to process config: %w", err)
	}

	return &cfg, nil
}

// processConfig processes the configuration by expanding all environment variables and verify_files fields.
// This function performs complete variable expansion using the new %{VAR} system in the following steps:
//  1. Global level: from_env, vars, env (new system only)
//  2. Group level: from_env inheritance/override, vars, env (new system only)
//  3. Command level: vars, env, cmd, args expansion (new system only)
func processConfig(cfg *runnertypes.Config, filter *environment.Filter) error {
	// Step 1: Expand Global configuration
	if err := ExpandGlobalConfig(&cfg.Global, filter); err != nil {
		return fmt.Errorf("failed to expand global config: %w", err)
	}

	// Step 2: Expand each Group configuration
	for i := range cfg.Groups {
		group := &cfg.Groups[i]

		if err := ExpandGroupConfig(group, &cfg.Global, filter); err != nil {
			return fmt.Errorf("failed to expand group[%s] config: %w", group.Name, err)
		}

		// Step 3: Expand each Command configuration
		for j := range group.Commands {
			cmd := &group.Commands[j]
			if err := ExpandCommandConfig(cmd, group, &cfg.Global, filter); err != nil {
				return fmt.Errorf("failed to expand command %q in group %q: %w", cmd.Name, group.Name, err)
			}
		}
	}

	return nil
}

// validateEnvironmentVariables validates all environment variables in the config
func (l *Loader) validateEnvironmentVariables(cfg *runnertypes.Config) error {
	// Validate environment variables for each command in each group
	for _, group := range cfg.Groups {
		for _, cmd := range group.Commands {
			// Build environment map from command's Env slice
			_, err := cmd.BuildEnvironmentMap()
			if err != nil {
				return fmt.Errorf("failed to build environment map for command %q: %w", cmd.Name, err)
			}
		}
	}

	return nil
}
