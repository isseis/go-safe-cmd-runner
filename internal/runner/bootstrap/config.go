// Package bootstrap provides application initialization and setup functionality.
package bootstrap

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// LoadAndPrepareConfig loads a configuration file and prepares it for execution.
//
// This function performs the following steps:
//  1. Verifies the configuration file's hash (TOCTOU protection)
//  2. Reads and parses the TOML configuration
//  3. Expands variables in command strings (${VAR} syntax in cmd/args fields)
//
// The returned Config is ready for execution with all command strings expanded.
// Variable expansion is immutable - original config data is not modified.
//
// Parameters:
//   - verificationManager: Manager for secure file verification
//   - configPath: Path to the configuration file
//   - runID: Unique identifier for this execution run
//
// Returns:
//   - *Config: Prepared configuration ready for command execution
//   - error: Any error during verification, loading, parsing, or expansion
func LoadAndPrepareConfig(verificationManager *verification.Manager, configPath, runID string) (*runnertypes.Config, error) {
	if configPath == "" {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeRequiredArgumentMissing,
			Message:   "Config file path is required",
			Component: "config",
			RunID:     runID,
		}
	}

	// Perform atomic verification and reading to prevent TOCTOU attacks
	// The verification manager reads the file once, verifies its hash, and returns the content
	content, err := verificationManager.VerifyAndReadConfigFile(configPath)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Config verification and reading failed: %v", err),
			Component: "verification",
			RunID:     runID,
		}
	}

	// Load config from the verified content (no file path required)
	// This eliminates TOCTOU vulnerability since we use the already-verified content
	cfgLoader := config.NewLoader()
	cfg, err := cfgLoader.LoadConfig(content)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Failed to parse config from verified content: %v", err),
			Component: "config",
			RunID:     runID,
		}
	}

	// Expand variables in command strings (cmd and args fields) and Command.Env
	// This creates new CommandGroups with expanded ${VAR} references, leaving originals unchanged
	filter := environment.NewFilter(cfg)
	expander := environment.NewVariableExpander(filter)
	expandedGroups := make([]runnertypes.CommandGroup, len(cfg.Groups))
	for i := range cfg.Groups {
		// 1. Expand cmd/args fields
		expandedGroup, err := config.ExpandCommandStrings(&cfg.Groups[i], expander)
		if err != nil {
			return nil, &logging.PreExecutionError{
				Type:      logging.ErrorTypeConfigParsing,
				Message:   fmt.Sprintf("Failed to expand command strings: %v", err),
				Component: "config",
				RunID:     runID,
			}
		}

		// 2. Pre-expand Command.Env for each command
		for j := range expandedGroup.Commands {
			expandedEnv, err := expander.ExpandCommandEnv(&expandedGroup.Commands[j], &cfg.Groups[i])
			if err != nil {
				return nil, &logging.PreExecutionError{
					Type:      logging.ErrorTypeConfigParsing,
					Message:   fmt.Sprintf("Failed to expand command environment for command %s: %v", expandedGroup.Commands[j].Name, err),
					Component: "config",
					RunID:     runID,
				}
			}
			expandedGroup.Commands[j].ExpandedEnv = expandedEnv
		}

		expandedGroups[i] = *expandedGroup
	}
	cfg.Groups = expandedGroups

	return cfg, nil
}
