package bootstrap

import (
	"fmt"
	"io"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// SetupLogging sets up logging system without environment file handling
// consoleWriter specifies where console logs should be written (stdout or stderr)
// If nil, defaults to stdout for backward compatibility
func SetupLogging(logLevel runnertypes.LogLevel, logDir, runID string, forceInteractive, forceQuiet bool, consoleWriter io.Writer) error {
	// Get Slack webhook URL from OS environment variables
	slackURL := os.Getenv(logging.SlackWebhookURLEnvVar)

	// Setup logging system with all configuration including Slack
	loggerConfig := LoggerConfig{
		Level:           logLevel,
		LogDir:          logDir,
		RunID:           runID,
		SlackWebhookURL: slackURL,
		ConsoleWriter:   consoleWriter,
	}

	if err := SetupLoggerWithConfig(loggerConfig, forceInteractive, forceQuiet); err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeLogFileOpen,
			Message:   fmt.Sprintf("Failed to setup logger: %v", err),
			Component: "logging",
			RunID:     runID,
		}
	}

	return nil
}
