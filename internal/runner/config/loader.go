// Package config provides functionality for loading and validating
// configuration files for the command runner. It supports TOML format
// and includes utilities for managing configuration settings.
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

	// Create Filter and VariableExpander for verify_files expansion
	filter := environment.NewFilter(cfg.Global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)

	// Process config (expand verify_files)
	if err := processConfig(&cfg, filter, expander); err != nil {
		return nil, fmt.Errorf("failed to process config: %w", err)
	}

	return &cfg, nil
}

// processConfig processes the configuration by expanding environment variables and verify_files fields
func processConfig(cfg *runnertypes.Config, filter *environment.Filter, expander *environment.VariableExpander) error {
	// Generate automatic environment variables (fixed at config load time)
	// These variables are available for expansion in Global.Env and Group.Env
	autoEnvProvider := environment.NewAutoEnvProvider(nil)
	autoEnv := autoEnvProvider.Generate()

	// Phase 1: Expand Global.Env variables (now with automatic environment variables)
	if err := ExpandGlobalEnv(&cfg.Global, expander, autoEnv); err != nil {
		return fmt.Errorf("failed to expand global environment variables: %w", err)
	}

	// Phase 2: Expand Global.VerifyFiles (now can reference Global.Env)
	if err := ExpandGlobalVerifyFiles(&cfg.Global, filter, expander); err != nil {
		return fmt.Errorf("failed to expand global verify_files: %w", err)
	}

	// Phase 3: Group processing
	for i := range cfg.Groups {
		// First expand Group.Env (can reference Global.ExpandedEnv and automatic environment variables)
		if err := ExpandGroupEnv(&cfg.Groups[i], expander, autoEnv, cfg.Global.ExpandedEnv, cfg.Global.EnvAllowlist); err != nil {
			return fmt.Errorf("failed to expand group environment variables for group %q: %w", cfg.Groups[i].Name, err)
		}

		// Then expand Group.VerifyFiles (can reference Group.ExpandedEnv and Global.ExpandedEnv)
		if err := ExpandGroupVerifyFiles(&cfg.Groups[i], &cfg.Global, filter, expander); err != nil {
			return fmt.Errorf("failed to expand verify_files for group %q: %w", cfg.Groups[i].Name, err)
		}

		// Note: Command.Env, Cmd, and Args expansion is performed later by bootstrap.InitConfig
		// which calls config.ExpandCommand(). This separation maintains clean architectural
		// boundaries: config.Loader handles configuration parsing and Global/Group-level expansion,
		// while bootstrap handles runtime initialization and Command-level expansion.
	}

	return nil
}

// validateEnvironmentVariables validates all environment variables in the config
func (l *Loader) validateEnvironmentVariables(cfg *runnertypes.Config) error {
	// Validate environment variables for each command in each group
	for _, group := range cfg.Groups {
		for _, cmd := range group.Commands {
			// Build environment map from command's Env slice
			envMap, err := cmd.BuildEnvironmentMap()
			if err != nil {
				return fmt.Errorf("failed to build environment map for command %q: %w", cmd.Name, err)
			}

			// Validate environment variable names
			if err := environment.ValidateUserEnvNames(envMap); err != nil {
				return fmt.Errorf("invalid environment variable in command %q: %w", cmd.Name, err)
			}
		}
	}

	return nil
}
