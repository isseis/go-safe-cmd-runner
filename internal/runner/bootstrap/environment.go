package bootstrap

import (
	"fmt"
	"io"
	"log/slog"

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
	SlackWebhookURL  string    // Slack webhook URL for notifications. Empty string disables Slack handler.
	DryRun           bool      // If true, Slack notifications are not sent
}

// SetupLogging sets up logging system without environment file handling
func SetupLogging(opts SetupLoggingOptions) error {
	// Setup logging system with all configuration including Slack
	// Empty SlackWebhookURL disables Slack notifications (e.g., in dry-run mode)
	loggerConfig := LoggerConfig{
		Level:           opts.LogLevel,
		LogDir:          opts.LogDir,
		RunID:           opts.RunID,
		SlackWebhookURL: opts.SlackWebhookURL,
		ConsoleWriter:   opts.ConsoleWriter,
		DryRun:          opts.DryRun,
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
