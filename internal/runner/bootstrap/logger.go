package bootstrap

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/terminal"
)

const (
	// File permissions for log files
	logFilePerm = 0o600
)

// LoggerConfig holds all configuration for logger setup
type LoggerConfig struct {
	Level                  slog.Level
	LogDir                 string
	RunID                  string
	SlackWebhookURLSuccess string    // Webhook URL for success (INFO) notifications
	SlackWebhookURLError   string    // Webhook URL for error (WARN/ERROR) notifications
	ConsoleWriter          io.Writer // Writer for console output (stdout/stderr)
	DryRun                 bool      // If true, Slack notifications are not sent
}

// redactionErrorCollector is a global collector for redaction failures
// This is set during logger initialization and used for shutdown reporting
var redactionErrorCollector *redaction.InMemoryErrorCollector

// redactionReporter is a global reporter for shutdown
var redactionReporter *redaction.ShutdownReporter

// SetupLoggerWithConfig initializes the logging system with all handlers atomically.
//
// IMPORTANT: This function must be called exactly once during application startup,
// before any logging operations occur. It is designed for single-threaded bootstrap
// initialization and should not be called concurrently or after the application
// has started processing.
//
// The global redactionErrorCollector and redactionReporter are initialized during
// this call and must not be accessed before initialization completes.
func SetupLoggerWithConfig(config LoggerConfig, forceInteractive, forceQuiet bool) error {
	hostname := common.GetHostname()
	timestamp := time.Now().Format("20060102T150405Z")

	var handlers []slog.Handler

	// Use the log level directly
	slogLevel := config.Level

	// Initialize terminal capabilities with command line overrides
	terminalOptions := terminal.Options{
		DetectorOptions: terminal.DetectorOptions{
			ForceInteractive:    forceInteractive,
			ForceNonInteractive: forceQuiet,
		},
		// PreferenceOptions use environment variables by default
	}
	capabilities := terminal.NewCapabilities(terminalOptions)

	// 1. Interactive handler (for colored output when appropriate)
	if capabilities.IsInteractive() {
		// Create message formatter and line tracker for interactive output
		formatter := logging.NewDefaultMessageFormatter()
		lineTracker := logging.NewDefaultLogLineTracker()

		interactiveHandler, err := logging.NewInteractiveHandler(logging.InteractiveHandlerOptions{
			Level:        slogLevel,
			Writer:       os.Stderr, // Interactive messages go to stderr
			Capabilities: capabilities,
			Formatter:    formatter,
			LineTracker:  lineTracker,
		})
		if err != nil {
			return fmt.Errorf("failed to create interactive handler: %w", err)
		}
		handlers = append(handlers, interactiveHandler)
	}

	// 2. Conditional text handler (for non-interactive console output)
	// Use configured console writer (stdout by default, can be overridden by caller)
	consoleWriter := config.ConsoleWriter
	if consoleWriter == nil {
		consoleWriter = os.Stdout // Default to stdout if not specified
	}
	conditionalTextHandler, err := logging.NewConditionalTextHandler(logging.ConditionalTextHandlerOptions{
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slogLevel,
		},
		Writer:       consoleWriter,
		Capabilities: capabilities,
	})
	if err != nil {
		return fmt.Errorf("failed to create conditional text handler: %w", err)
	}
	handlers = append(handlers, conditionalTextHandler)

	// 3. Machine-readable log handler (to file, per-run auto-named)
	if config.LogDir != "" {
		// Validate log directory
		if err := logging.ValidateLogDir(config.LogDir); err != nil {
			return fmt.Errorf("invalid log directory: %w", err)
		}

		logPath := filepath.Join(config.LogDir, fmt.Sprintf("%s_%s_%s.json", hostname, timestamp, config.RunID))
		fileOpener := logging.NewSafeFileOpener()
		logF, err := fileOpener.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logFilePerm)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}

		jsonHandler := slog.NewJSONHandler(logF, &slog.HandlerOptions{
			Level: slogLevel,
		})

		// Attach common attributes
		enrichedHandler := jsonHandler.WithAttrs([]slog.Attr{
			slog.String("hostname", hostname),
			slog.Int("pid", os.Getpid()),
			slog.Int("schema_version", 1),
			slog.String("run_id", config.RunID),
		})
		handlers = append(handlers, enrichedHandler)
	}

	// 4. Slack notification handlers (optional)
	var slackSuccessHandler, slackErrorHandler slog.Handler

	// Create success handler if URL is provided (INFO level only)
	if config.SlackWebhookURLSuccess != "" {
		sh, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
			WebhookURL: config.SlackWebhookURLSuccess,
			RunID:      config.RunID,
			IsDryRun:   config.DryRun,
			LevelMode:  logging.LevelModeExactInfo,
		})
		if err != nil {
			return fmt.Errorf("failed to create success Slack handler: %w", err)
		}
		slackSuccessHandler = sh
		handlers = append(handlers, sh)
	}

	// Create error handler if URL is provided (WARN and above)
	if config.SlackWebhookURLError != "" {
		sh, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
			WebhookURL: config.SlackWebhookURLError,
			RunID:      config.RunID,
			IsDryRun:   config.DryRun,
			LevelMode:  logging.LevelModeWarnAndAbove,
		})
		if err != nil {
			return fmt.Errorf("failed to create error Slack handler: %w", err)
		}
		slackErrorHandler = sh
		handlers = append(handlers, sh)
	}

	// Create failure logger (excludes Slack to prevent sensitive information leakage)
	// This logger is used for detailed error logging during redaction failures
	failureHandlers := make([]slog.Handler, 0, len(handlers))
	for _, h := range handlers {
		// Exclude Slack handlers from failure logger
		// Detailed panic values and stack traces should not be sent to Slack
		if h != slackSuccessHandler && h != slackErrorHandler {
			failureHandlers = append(failureHandlers, h)
		}
	}

	failureMultiHandler, err := logging.NewMultiHandler(failureHandlers...)
	if err != nil {
		return fmt.Errorf("failed to create failure multi handler: %w", err)
	}
	failureLogger := slog.New(failureMultiHandler)

	// Create redaction error collector for monitoring failures
	// Limit to 1000 most recent failures to prevent unbounded growth
	const maxRedactionFailures = 1000
	redactionErrorCollector = redaction.NewInMemoryErrorCollector(maxRedactionFailures)

	// Create MultiHandler with redaction (includes all handlers including Slack)
	multiHandler, err := logging.NewMultiHandler(handlers...)
	if err != nil {
		return fmt.Errorf("failed to create multi handler: %w", err)
	}
	redactedHandler := redaction.NewRedactingHandler(multiHandler, nil, failureLogger).
		WithErrorCollector(redactionErrorCollector)

	// Create shutdown reporter for redaction failures
	redactionReporter = redaction.NewShutdownReporter(redactionErrorCollector, os.Stderr, failureLogger)

	// Set as default logger
	logger := slog.New(redactedHandler)
	slog.SetDefault(logger)

	slog.Info("Logger initialized",
		"log-level", config.Level,
		"log-dir", config.LogDir,
		"run_id", config.RunID,
		"hostname", hostname,
		"interactive_mode", capabilities.IsInteractive(),
		"color_support", capabilities.SupportsColor(),
		"slack_success_enabled", config.SlackWebhookURLSuccess != "",
		"slack_error_enabled", config.SlackWebhookURLError != "")

	return nil
}

// ReportRedactionFailures reports any collected redaction failures
// This should be called during application shutdown
func ReportRedactionFailures() {
	if redactionReporter == nil {
		return
	}

	if err := redactionReporter.Report(); err != nil {
		// Use fmt.Fprintf since logger might be shutting down
		fmt.Fprintf(os.Stderr, "Warning: failed to report redaction failures: %v\n", err)
	}
}
