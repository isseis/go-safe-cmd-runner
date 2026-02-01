// Package bootstrap provides application initialization and setup functionality.
package bootstrap

import (
	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// LoadAndPrepareConfig loads and verifies a configuration file.
//
// This function performs the following steps:
//  1. Verifies the configuration file's hash (TOCTOU protection)
//  2. Loads the configuration using config.Loader
//
// Note: All variable expansion (Global.EnvVars, Group.EnvVars, Command.EnvVars, Cmd, Args)
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
//   - *ConfigSpec: Prepared configuration ready for command execution
//   - error: Any error during verification, loading, or parsing
func LoadAndPrepareConfig(verificationManager *verification.Manager, configPath, runID string) (*runnertypes.ConfigSpec, error) {
	if configPath == "" {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeRequiredArgumentMissing,
			Message:   "Config file path is required",
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}

	// Perform atomic verification and reading to prevent TOCTOU attacks
	// The verification manager reads the file once, verifies its hash, and returns the content
	content, err := verificationManager.VerifyAndReadConfigFile(configPath)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   err.Error(),
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}

	// Load config from the verified content
	// All expansion (Global.EnvVars, Group.EnvVars, Command.EnvVars, Cmd, Args) is now
	// performed inside config.Loader.LoadConfig()
	// LoadConfigWithPath processes includes and merges templates from multiple files
	// Use verified template manager to ensure included files are also verified against hashes
	cfgLoader := config.NewLoader(
		common.NewDefaultFileSystem(),
		verificationManager,
	)

	cfg, err := cfgLoader.LoadConfig(configPath, content)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   err.Error(),
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}

	return cfg, nil
}
