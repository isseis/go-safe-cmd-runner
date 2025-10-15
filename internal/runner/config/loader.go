// Package config provides functionality for loading, validating, and expanding
// configuration files for the command runner. It supports TOML format and
// includes complete variable expansion for all environment variables, commands,
// and verify_files fields. All expansion processing is consolidated in this package.
package config

import (
	"errors"
	"fmt"
	"maps"
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

// processConfig processes the configuration by expanding all environment variables and verify_files fields.
// This function performs complete variable expansion in the following steps:
//  1. Global level: from_env, vars, env (new system), and ${VAR} expansion (old system), verify_files expansion
//  2. Group level: from_env inheritance/override, vars, env (new + old system), verify_files expansion
//  3. Command level: vars, env (new + old system), cmd, args expansion
func processConfig(cfg *runnertypes.Config, filter *environment.Filter, expander *environment.VariableExpander) error {
	// Generate automatic environment variables (fixed at config load time)
	// These variables are available for expansion in Global.Env and Group.Env
	autoEnvProvider := environment.NewAutoEnvProvider(nil)
	autoEnv := autoEnvProvider.Generate()

	// Step 1: Expand Global configuration
	// 1.1: Process new variable system (from_env, vars, internal variables)
	if err := ExpandGlobalConfig(&cfg.Global, filter); err != nil {
		return fmt.Errorf("failed to expand global config: %w", err)
	}

	// 1.2: Expand Global.Env with old system (${VAR} syntax with automatic environment variables)
	if err := ExpandGlobalEnv(&cfg.Global, expander, autoEnv); err != nil {
		return fmt.Errorf("failed to expand global environment variables: %w", err)
	}

	// 1.3: Expand Global.VerifyFiles (now can reference both old and new variables)
	if err := ExpandGlobalVerifyFiles(&cfg.Global, filter, expander); err != nil {
		return fmt.Errorf("failed to expand global verify_files: %w", err)
	}

	// Step 2: Expand each Group configuration
	for i := range cfg.Groups {
		group := &cfg.Groups[i]

		// 2.1: Process new variable system (from_env inheritance/override, vars)
		if err := ExpandGroupConfig(group, &cfg.Global, filter); err != nil {
			return fmt.Errorf("failed to expand group[%s] config: %w", group.Name, err)
		}

		// 2.2: Expand Group.Env with old system (${VAR} syntax)
		if err := ExpandGroupEnv(group, expander, autoEnv, cfg.Global.ExpandedEnv, cfg.Global.EnvAllowlist); err != nil {
			return fmt.Errorf("failed to expand group environment variables for group %q: %w", group.Name, err)
		}

		// 2.3: Expand Group.VerifyFiles
		if err := ExpandGroupVerifyFiles(group, &cfg.Global, filter, expander); err != nil {
			return fmt.Errorf("failed to expand verify_files for group %q: %w", group.Name, err)
		}

		// Step 3: Expand each Command configuration
		for j := range group.Commands {
			cmd := &group.Commands[j]

			// 3.1: Process new variable system (vars, env with %{VAR})
			if err := ExpandCommandConfig(cmd, group); err != nil {
				return fmt.Errorf("failed to expand command[%s] in group[%s]: %w", cmd.Name, group.Name, err)
			}

			// Save results from new system
			newSystemCmd := cmd.ExpandedCmd
			newSystemArgs := cmd.ExpandedArgs
			newSystemEnv := cmd.ExpandedEnv

			// 3.2: Expand Command with old system (Cmd, Args, Env with ${VAR})
			expandedCmd, expandedArgs, expandedEnv, err := ExpandCommand(&ExpansionContext{
				Command:            cmd,
				Expander:           expander,
				AutoEnv:            autoEnv,
				GlobalEnv:          cfg.Global.ExpandedEnv,
				GroupEnv:           group.ExpandedEnv,
				GlobalEnvAllowlist: cfg.Global.EnvAllowlist,
				GroupName:          group.Name,
				GroupEnvAllowlist:  group.EnvAllowlist,
			})
			if err != nil {
				return fmt.Errorf("failed to expand command %q in group %q: %w",
					cmd.Name, group.Name, err)
			}

			// Merge: prefer new system if it expanded the value, otherwise use old system
			// For cmd: use new system if it differs from original, otherwise use old system
			if newSystemCmd != cmd.Cmd {
				cmd.ExpandedCmd = newSystemCmd
			} else {
				cmd.ExpandedCmd = expandedCmd
			}

			// For args: use new system if it differs from original, otherwise use old system
			// Note: need to compare each arg individually or check if any arg was expanded
			argsExpanded := false
			for i, arg := range cmd.Args {
				if i < len(newSystemArgs) && newSystemArgs[i] != arg {
					argsExpanded = true
					break
				}
			}
			if argsExpanded {
				cmd.ExpandedArgs = newSystemArgs
			} else {
				cmd.ExpandedArgs = expandedArgs
			}

			// For env: merge both systems (new system takes precedence)
			mergedEnv := make(map[string]string)
			if expandedEnv != nil {
				maps.Copy(mergedEnv, expandedEnv)
			}
			if newSystemEnv != nil {
				maps.Copy(mergedEnv, newSystemEnv)
			}
			cmd.ExpandedEnv = mergedEnv
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
