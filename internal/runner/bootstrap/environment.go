package bootstrap

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// SlackWebhookConfig holds the validated Slack webhook configuration
type SlackWebhookConfig struct {
	SuccessURL string
	ErrorURL   string
}

// ErrDeprecatedSlackWebhook is returned when the deprecated env var is set
var ErrDeprecatedSlackWebhook = errors.New("GSCR_SLACK_WEBHOOK_URL is deprecated")

// ErrSuccessWithoutError is returned when SUCCESS is set but ERROR is not
var ErrSuccessWithoutError = errors.New("GSCR_SLACK_WEBHOOK_URL_SUCCESS requires GSCR_SLACK_WEBHOOK_URL_ERROR")

// ValidateSlackWebhookEnv validates Slack webhook environment variables
func ValidateSlackWebhookEnv() (*SlackWebhookConfig, error) {
	// Check for deprecated environment variable
	if os.Getenv(logging.SlackWebhookURLEnvVar) != "" {
		return nil, fmt.Errorf("%w: please migrate to GSCR_SLACK_WEBHOOK_URL_SUCCESS and GSCR_SLACK_WEBHOOK_URL_ERROR",
			ErrDeprecatedSlackWebhook)
	}

	successURL := os.Getenv(logging.SlackWebhookURLSuccessEnvVar)
	errorURL := os.Getenv(logging.SlackWebhookURLErrorEnvVar)

	// Validate combinations
	if successURL != "" && errorURL == "" {
		return nil, fmt.Errorf("%w: error notifications must be enabled",
			ErrSuccessWithoutError)
	}

	// Both empty is valid (Slack disabled)
	// ERROR only is valid (no success notifications)
	// Both set is valid

	return &SlackWebhookConfig{
		SuccessURL: successURL,
		ErrorURL:   errorURL,
	}, nil
}

// FormatDeprecatedSlackWebhookError formats the error message for deprecated env var
func FormatDeprecatedSlackWebhookError() string {
	return `Error: GSCR_SLACK_WEBHOOK_URL is deprecated.

Please migrate to the new webhook configuration:
  export GSCR_SLACK_WEBHOOK_URL_SUCCESS="<your_webhook_url>"
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"

For more information, see the migration guide at:
  https://github.com/isseis/go-safe-cmd-runner/docs/user/runner_command.md#slack-webhook-configuration`
}

// FormatSuccessWithoutErrorError formats the error message for invalid config
func FormatSuccessWithoutErrorError() string {
	return `Error: Invalid Slack webhook configuration.

GSCR_SLACK_WEBHOOK_URL_SUCCESS is set but GSCR_SLACK_WEBHOOK_URL_ERROR is not.
Error notifications must be enabled to prevent silent failures.

Please set GSCR_SLACK_WEBHOOK_URL_ERROR:
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"

To use the same webhook for both success and error notifications:
  export GSCR_SLACK_WEBHOOK_URL_SUCCESS="<your_webhook_url>"
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"`
}

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
