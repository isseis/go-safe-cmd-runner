package bootstrap

import (
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

// ErrDeprecatedSlackWebhook is returned when the deprecated env var is set.
// Use errors.Is(err, ErrDeprecatedSlackWebhook) to check for this error type.
var ErrDeprecatedSlackWebhook = &DeprecatedSlackWebhookError{}

// DeprecatedSlackWebhookError indicates that the deprecated GSCR_SLACK_WEBHOOK_URL is set.
type DeprecatedSlackWebhookError struct{}

func (e *DeprecatedSlackWebhookError) Error() string {
	return `Error: GSCR_SLACK_WEBHOOK_URL is deprecated.

Please migrate to the new webhook configuration:
  export GSCR_SLACK_WEBHOOK_URL_SUCCESS="<your_webhook_url>"
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"

For more information, see the migration guide at:
  https://github.com/isseis/go-safe-cmd-runner/docs/user/runner_command.md#slack-webhook-configuration`
}

// Is implements errors.Is support.
func (e *DeprecatedSlackWebhookError) Is(target error) bool {
	_, ok := target.(*DeprecatedSlackWebhookError)
	return ok
}

// ErrSuccessWithoutError is returned when SUCCESS is set but ERROR is not.
// Use errors.Is(err, ErrSuccessWithoutError) to check for this error type.
var ErrSuccessWithoutError = &SuccessWithoutErrorError{}

// SuccessWithoutErrorError indicates that GSCR_SLACK_WEBHOOK_URL_SUCCESS is set
// but GSCR_SLACK_WEBHOOK_URL_ERROR is not.
type SuccessWithoutErrorError struct{}

func (e *SuccessWithoutErrorError) Error() string {
	return `Error: Invalid Slack webhook configuration.

GSCR_SLACK_WEBHOOK_URL_SUCCESS is set but GSCR_SLACK_WEBHOOK_URL_ERROR is not.
Error notifications must be enabled to prevent silent failures.

Please set GSCR_SLACK_WEBHOOK_URL_ERROR:
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"

To use the same webhook for both success and error notifications:
  export GSCR_SLACK_WEBHOOK_URL_SUCCESS="<your_webhook_url>"
  export GSCR_SLACK_WEBHOOK_URL_ERROR="<your_webhook_url>"`
}

// Is implements errors.Is support.
func (e *SuccessWithoutErrorError) Is(target error) bool {
	_, ok := target.(*SuccessWithoutErrorError)
	return ok
}

// ValidateSlackWebhookEnv validates Slack webhook environment variables
func ValidateSlackWebhookEnv() (*SlackWebhookConfig, error) {
	// Check for deprecated environment variable
	if os.Getenv(logging.SlackWebhookURLEnvVar) != "" {
		return nil, &DeprecatedSlackWebhookError{}
	}

	successURL := os.Getenv(logging.SlackWebhookURLSuccessEnvVar)
	errorURL := os.Getenv(logging.SlackWebhookURLErrorEnvVar)

	// Validate combinations
	if successURL != "" && errorURL == "" {
		return nil, &SuccessWithoutErrorError{}
	}

	// Both empty is valid (Slack disabled)
	// ERROR only is valid (no success notifications)
	// Both set is valid

	return &SlackWebhookConfig{
		SuccessURL: successURL,
		ErrorURL:   errorURL,
	}, nil
}

// SetupLoggingOptions holds configuration for SetupLogging
type SetupLoggingOptions struct {
	LogLevel               slog.Level
	LogDir                 string
	RunID                  string
	ForceInteractive       bool
	ForceQuiet             bool
	ConsoleWriter          io.Writer // If nil, defaults to stdout for backward compatibility
	SlackWebhookURLSuccess string    // Slack webhook URL for success (INFO) notifications. Empty string disables.
	SlackWebhookURLError   string    // Slack webhook URL for error (WARN/ERROR) notifications. Empty string disables.
	DryRun                 bool      // If true, Slack notifications are not sent
}

// SetupLogging sets up logging system without environment file handling
func SetupLogging(opts SetupLoggingOptions) error {
	// Setup logging system with all configuration including Slack
	loggerConfig := LoggerConfig{
		Level:                  opts.LogLevel,
		LogDir:                 opts.LogDir,
		RunID:                  opts.RunID,
		SlackWebhookURLSuccess: opts.SlackWebhookURLSuccess,
		SlackWebhookURLError:   opts.SlackWebhookURLError,
		ConsoleWriter:          opts.ConsoleWriter,
		DryRun:                 opts.DryRun,
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
