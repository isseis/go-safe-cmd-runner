// Package bootstrap provides application initialization and setup functionality.
package bootstrap

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// LoadAndPrepareConfig loads and verifies a configuration file.
//
// This function performs the following steps:
//  1. Verifies the configuration file's hash (TOCTOU protection)
//  2. Loads the configuration using config.Loader
//
// Note: All variable expansion (Global.Env, Group.Env, Command.Env, Cmd, Args)
// is now performed inside config.Loader.LoadConfig(). This function only handles
// verification and loading.
//
// The returned Config is ready for execution with all variables expanded.
//
// Parameters:
//   - verificationManager: Manager for secure file verification
//   - configPath: Path to the configuration file
//   - runID: Unique identifier for this execution run
//
// Returns:
//   - *Config: Prepared configuration ready for command execution
//   - error: Any error during verification, loading, or parsing
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

	// Load config from the verified content
	// All expansion (Global.Env, Group.Env, Command.Env, Cmd, Args) is now
	// performed inside config.Loader.LoadConfig()
	cfgLoader := config.NewLoader()
	cfg, err := cfgLoader.LoadConfig(content)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Failed to load config: %v", err),
			Component: "config",
			RunID:     runID,
		}
	}

	return cfg, nil
}
