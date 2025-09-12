package bootstrap

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/terminal"
	"github.com/joho/godotenv"
)

const (
	// File permissions for log files
	logFilePerm = 0o600
)

// LoggerConfig holds all configuration for logger setup
type LoggerConfig struct {
	Level           string
	LogDir          string
	RunID           string
	SlackWebhookURL string
}

// GetSlackWebhookFromEnvFile securely reads Slack webhook URL from .env file
// Returns the webhook URL and an error if any issues occur during file access or parsing
func GetSlackWebhookFromEnvFile(envFile string) (string, error) {
	if envFile == "" {
		return "", nil
	}

	// Use safefileio for secure file reading (includes path validation and permission checks)
	content, err := safefileio.SafeReadFile(envFile)
	if err != nil {
		return "", fmt.Errorf("failed to read environment file %q securely: %w", envFile, err)
	}

	// Parse content directly using godotenv.Parse (no temporary file needed)
	envMap, err := godotenv.Parse(bytes.NewReader(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse environment file %q: %w", envFile, err)
	}

	// Look for Slack webhook URL
	if slackURL, exists := envMap[logging.SlackWebhookURLEnvVar]; exists && slackURL != "" {
		slog.Debug("Found Slack webhook URL in env file", "key", logging.SlackWebhookURLEnvVar, "file", envFile)
		return slackURL, nil
	}

	slog.Debug("No Slack webhook URL found in env file", "file", envFile)
	return "", nil
}

// SetupLoggerWithConfig initializes the logging system with all handlers atomically
func SetupLoggerWithConfig(config LoggerConfig, forceInteractive, forceQuiet bool) error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}
	timestamp := time.Now().Format("20060102T150405Z")

	var handlers []slog.Handler
	var invalidLogLevel bool

	// Parse log level for all handlers
	var slogLevel slog.Level
	if err := slogLevel.UnmarshalText([]byte(config.Level)); err != nil {
		slogLevel = slog.LevelInfo // Default to info on parse error
		invalidLogLevel = true
	}

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

	// 2. Conditional text handler (for non-interactive stdout output)
	conditionalTextHandler, err := logging.NewConditionalTextHandler(logging.ConditionalTextHandlerOptions{
		TextHandlerOptions: &slog.HandlerOptions{
			Level: slogLevel,
		},
		Writer:       os.Stdout,
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

	// 4. Slack notification handler (optional)
	if config.SlackWebhookURL != "" {
		slackHandler, err := logging.NewSlackHandler(config.SlackWebhookURL, config.RunID)
		if err != nil {
			return fmt.Errorf("failed to create Slack handler: %w", err)
		}
		handlers = append(handlers, slackHandler)
	}

	// Create MultiHandler with redaction
	multiHandler, err := logging.NewMultiHandler(handlers...)
	if err != nil {
		return fmt.Errorf("failed to create multi handler: %w", err)
	}
	redactedHandler := redaction.NewRedactingHandler(multiHandler, nil)

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
		"slack_enabled", config.SlackWebhookURL != "")

	// Warn about invalid log level after logger is properly set up
	if invalidLogLevel {
		slog.Warn("Invalid log level provided, defaulting to INFO", "provided", config.Level)
	}

	return nil
}
