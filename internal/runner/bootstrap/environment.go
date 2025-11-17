package bootstrap

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// SetupLoggingOptions holds configuration for SetupLogging
type SetupLoggingOptions struct {
	LogLevel         slog.Level
	LogDir           string
	RunID            string
	ForceInteractive bool
	ForceQuiet       bool
	ConsoleWriter    io.Writer // If nil, defaults to stdout for backward compatibility
	IsDryRun         bool      // Suppresses side effects like Slack notifications when true
}

// SetupLogging sets up logging system without environment file handling
func SetupLogging(opts SetupLoggingOptions) error {
	// Get Slack webhook URL from OS environment variables
	slackURL := os.Getenv(logging.SlackWebhookURLEnvVar)

	// Setup logging system with all configuration including Slack
	loggerConfig := LoggerConfig{
		Level:           opts.LogLevel,
		LogDir:          opts.LogDir,
		RunID:           opts.RunID,
		SlackWebhookURL: slackURL,
		ConsoleWriter:   opts.ConsoleWriter,
		DryRun:          opts.IsDryRun,
	}

	if err := SetupLoggerWithConfig(loggerConfig, opts.ForceInteractive, opts.ForceQuiet); err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeLogFileOpen,
			Message:   fmt.Sprintf("Failed to setup logger: %v", err),
			Component: string(resource.ComponentLogging),
			RunID:     opts.RunID,
		}
	}

	return nil
}
