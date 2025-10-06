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
//     and expands per-command environment variables (Command.Env -> Command.ExpandedEnv).
//
// The returned Config is ready for execution with all command strings expanded.
// The loader sets Command.ExpandedCmd, Command.ExpandedArgs and Command.ExpandedEnv
// while leaving the original source values available on the command for auditing/debugging.
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

	// Generate automatic environment variables (fixed at config load time)
	envManager := environment.NewManager(nil)
	autoEnv, err := envManager.BuildEnv()
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Failed to generate automatic environment variables: %v", err),
			Component: "config",
			RunID:     runID,
		}
	}

	// Expand variables in Cmd, Args and Env fields and fills them into ExpandedCmd, ExpandedArgs and ExpandedEnv
	// in-place on cfg.Groups.
	filter := environment.NewFilter(cfg.Global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)
	for i := range cfg.Groups {
		group := &cfg.Groups[i]
		for j := range group.Commands {
			cmd := &group.Commands[j]

			// Expand Command.Cmd, Args, and Env for each command and store in ExpandedCmd, ExpandedArgs, and ExpandedEnv
			// Include automatic environment variables in the expansion context
			expandedCmd, expandedArgs, expandedEnv, err := config.ExpandCommand(cmd, expander, autoEnv, group.EnvAllowlist, group.Name)
			if err != nil {
				return nil, &logging.PreExecutionError{
					Type:      logging.ErrorTypeConfigParsing,
					Message:   fmt.Sprintf("Failed to expand command strings for command %s: %v", cmd.Name, err),
					Component: "config",
					RunID:     runID,
				}
			}
			cmd.ExpandedCmd = expandedCmd
			cmd.ExpandedArgs = expandedArgs
			cmd.ExpandedEnv = expandedEnv
		}
	}

	return cfg, nil
}
