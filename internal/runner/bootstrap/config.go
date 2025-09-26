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

// LoadConfig performs atomic verification and loading to prevent TOCTOU attacks
func LoadConfig(verificationManager *verification.Manager, configPath, runID string) (*runnertypes.Config, error) {
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

	// Expand environment variables in cmd and args fields
	filter := environment.NewFilter(cfg)
	processor := environment.NewCommandEnvProcessor(filter)
	for i := range cfg.Groups {
		err := config.ExpandVariablesInGroup(&cfg.Groups[i], processor)
		if err != nil {
			return nil, &logging.PreExecutionError{
				Type:      logging.ErrorTypeConfigParsing,
				Message:   fmt.Sprintf("Failed to expand environment variables: %v", err),
				Component: "config",
				RunID:     runID,
			}
		}
	}

	return cfg, nil
}
