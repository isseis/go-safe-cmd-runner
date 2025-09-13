package bootstrap

import (
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
)

// SetupEnvironmentAndLogging determines environment file and sets up logging system
func SetupEnvironmentAndLogging(envFile, logLevel, logDir, runID string, forceInteractive, forceQuiet bool) (string, error) {
	// Determine environment file to load
	envFileToLoad := ""
	if envFile != "" {
		envFileToLoad = envFile
	} else {
		// Try to load default '.env' file if exists
		if _, err := os.Stat(".env"); err == nil {
			envFileToLoad = ".env"
		}
	}

	// Get Slack webhook URL from OS environment variables (not from file)
	slackURL := os.Getenv(logging.SlackWebhookURLEnvVar)

	// Setup logging system with all configuration including Slack
	loggerConfig := LoggerConfig{
		Level:           logLevel,
		LogDir:          logDir,
		RunID:           runID,
		SlackWebhookURL: slackURL,
	}

	if err := SetupLoggerWithConfig(loggerConfig, forceInteractive, forceQuiet); err != nil {
		return "", &logging.PreExecutionError{
			Type:      logging.ErrorTypeLogFileOpen,
			Message:   fmt.Sprintf("Failed to setup logger: %v", err),
			Component: "logging",
			RunID:     runID,
		}
	}

	return envFileToLoad, nil
}
