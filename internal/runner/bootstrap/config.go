// Package bootstrap provides application initialization and setup functionality.
package bootstrap

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// LoadAndValidateConfig loads configuration from file and validates basic requirements
func LoadAndValidateConfig(configPath, runID string) (*runnertypes.Config, error) {
	if configPath == "" {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeRequiredArgumentMissing,
			Message:   "Config file path is required",
			Component: "config",
			RunID:     runID,
		}
	}

	cfgLoader := config.NewLoader()
	cfg, err := cfgLoader.LoadConfig(configPath)
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
