package bootstrap

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// SlackWebhookConfig holds the validated Slack webhook configuration
type SlackWebhookConfig struct {
	SuccessURL string
	ErrorURL   string
}

// ErrSuccessWithoutError is returned when SUCCESS is set but ERROR is not.
// Use errors.Is(err, ErrSuccessWithoutError) to check for this error type.
var ErrSuccessWithoutError = &SuccessWithoutErrorError{}

// SuccessWithoutErrorError indicates that GSCR_SLACK_WEBHOOK_URL_SUCCESS is set
// but GSCR_SLACK_WEBHOOK_URL_ERROR is not.
type SuccessWithoutErrorError struct{}

// Error returns the guidance shown when only the success webhook is configured.
//
// This message is emitted before the redaction config exists (cmd/runner/main.go
// validates the Slack environment ahead of SetupLogging), so nothing downstream
// can mask it. Never interpolate an environment variable *value* here -- naming
// the variables is safe, echoing a webhook URL would write the credential to a
// log line unredacted. TestSuccessWithoutErrorErrorLeaksNoWebhookValue pins this.
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

// SetupLoggingOptions holds configuration for SetupLogging (Phase 1: console and file handlers).
// Slack handlers are configured separately via SetupSlackLogging after TOML is loaded.
type SetupLoggingOptions struct {
	LogLevel         slog.Level
	LogDir           string
	RunID            string
	ForceInteractive bool
	ForceQuiet       bool
	ConsoleWriter    io.Writer // If nil, defaults to stdout for backward compatibility
	DryRun           bool      // If true, Slack notifications are not sent

	// SlackAllowedHost is the permitted hostname read from TOML.
	// SetupSlackLogging forwards it to SlackLoggerConfig.AllowedHost.
	SlackAllowedHost string
}

// SetupLogging sets up Phase 1 logging (console and file handlers).
// Slack handlers are NOT configured here; call SetupSlackLogging after LoadAndPrepareConfig.
func SetupLogging(opts SetupLoggingOptions) error {
	loggerConfig := LoggerConfig{
		Level:         opts.LogLevel,
		LogDir:        opts.LogDir,
		RunID:         opts.RunID,
		ConsoleWriter: opts.ConsoleWriter,
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

// SetupSlackLogging is called after TOML config is loaded and adds Slack handlers.
// Returns an ErrorTypeConfigParsing error if host validation fails (AC-L2-10).
// On success, it also returns the redaction Config AddSlackHandlers built (nil
// if Slack notifications are not configured), so callers can share the same
// webhook-host masking with other redaction-aware components instead of each
// rebuilding it from opts.SlackAllowedHost independently.
func SetupSlackLogging(slackConfig *SlackWebhookConfig, opts SetupLoggingOptions) (*redaction.Config, error) {
	if slackConfig == nil {
		return nil, nil
	}
	if slackConfig.SuccessURL == "" && slackConfig.ErrorURL == "" {
		return nil, nil
	}

	slackLoggerConfig := SlackLoggerConfig{
		WebhookURLSuccess: slackConfig.SuccessURL,
		WebhookURLError:   slackConfig.ErrorURL,
		AllowedHost:       opts.SlackAllowedHost,
		RunID:             opts.RunID,
		DryRun:            opts.DryRun,
	}

	redactionConfig, err := AddSlackHandlers(slackLoggerConfig)
	if err != nil {
		// Use a constant Message and store the raw error in Err rather than formatting it
		// into Message, because url.Parse errors embed the webhook URL verbatim and Message
		// is written to stderr/slog by HandlePreExecutionError.
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   "Slack webhook URL validation failed",
			Component: string(resource.ComponentLogging),
			RunID:     opts.RunID,
			Err:       err,
		}
	}

	return redactionConfig, nil
}
