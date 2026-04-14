package bootstrap

import (
	"errors"
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

// LoggerConfig holds all configuration for Phase 1 logger setup (console and file handlers).
// Slack handlers are configured separately via AddSlackHandlers after TOML is loaded.
type LoggerConfig struct {
	Level         slog.Level
	LogDir        string
	RunID         string
	ConsoleWriter io.Writer // Writer for console output (stdout/stderr)
	DryRun        bool      // If true, Slack notifications are not sent
}

// SlackLoggerConfig は AddSlackHandlers に渡す Slack ハンドラ専用の設定。
type SlackLoggerConfig struct {
	WebhookURLSuccess string // 成功通知用 webhook URL (INFO)
	WebhookURLError   string // エラー通知用 webhook URL (WARN/ERROR)
	AllowedHost       string // 許可ホスト名 (AC-L2-4)
	RunID             string
	DryRun            bool
}

// redactionErrorCollector is a global collector for redaction failures
// This is set during logger initialization and used for shutdown reporting
var redactionErrorCollector *redaction.InMemoryErrorCollector

// redactionReporter is a global reporter for shutdown
var redactionReporter *redaction.ShutdownReporter

// errPhase1NotInitialized は AddSlackHandlers が SetupLoggerWithConfig 前に呼ばれた場合のエラー。
var errPhase1NotInitialized = errors.New("AddSlackHandlers called before SetupLoggerWithConfig")

// phase1BaseHandlers は SetupLoggerWithConfig が作成した Slack を除くハンドラ群。
// AddSlackHandlers がこれを参照して Slack ハンドラを追加した新たな MultiHandler を構築する。
var phase1BaseHandlers []slog.Handler

// phase1FailureLogger は Phase 1 で作成した failureLogger。
// AddSlackHandlers が RedactingHandler 再構築時に継続使用する。
var phase1FailureLogger *slog.Logger

// newSlackHandlerFunc は Slack ハンドラの生成 factory。
// テストで差し替え可能にすることで SlackHandlerOptions の内容を検査できる (AC-L2-19)。
var newSlackHandlerFunc = logging.NewSlackHandler

// SetupLoggerWithConfig initializes the Phase 1 logging system (console and file handlers).
//
// IMPORTANT: This function must be called exactly once during application startup,
// before any logging operations occur. It is designed for single-threaded bootstrap
// initialization and should not be called concurrently or after the application
// has started processing.
//
// Slack handlers are NOT set up here. Call AddSlackHandlers after LoadAndPrepareConfig
// to add Slack handlers with the AllowedHost from the TOML configuration.
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

	// Create failure logger using all Phase 1 handlers.
	// Slack handlers are excluded from failureLogger by design (added later via AddSlackHandlers).
	// Detailed panic values and stack traces should not be sent to Slack.
	failureMultiHandler, err := logging.NewMultiHandler(handlers...)
	if err != nil {
		return fmt.Errorf("failed to create failure multi handler: %w", err)
	}
	failureLogger := slog.New(failureMultiHandler)

	// Save Phase 1 state for AddSlackHandlers to reference later.
	phase1BaseHandlers = handlers
	phase1FailureLogger = failureLogger

	// Create redaction error collector for monitoring failures
	// Limit to 1000 most recent failures to prevent unbounded growth
	const maxRedactionFailures = 1000
	redactionErrorCollector = redaction.NewInMemoryErrorCollector(maxRedactionFailures)

	// Create MultiHandler with redaction (Phase 1 handlers only; Slack added via AddSlackHandlers)
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
		"color_support", capabilities.SupportsColor())

	return nil
}

// AddSlackHandlers は既存のデフォルトロガーに Slack ハンドラを追加して再構築する。
// successURL/errorURL どちらかでも validateWebhookURL が失敗した場合はエラーを返す。
// SetupLoggerWithConfig が呼ばれていない場合 (phase1BaseHandlers が nil) はエラーを返す。
func AddSlackHandlers(config SlackLoggerConfig) error {
	if phase1BaseHandlers == nil || phase1FailureLogger == nil {
		return errPhase1NotInitialized
	}

	allHandlers := make([]slog.Handler, len(phase1BaseHandlers))
	copy(allHandlers, phase1BaseHandlers)

	if config.WebhookURLSuccess != "" {
		sh, err := newSlackHandlerFunc(logging.SlackHandlerOptions{
			WebhookURL:  config.WebhookURLSuccess,
			RunID:       config.RunID,
			IsDryRun:    config.DryRun,
			LevelMode:   logging.LevelModeExactInfo,
			AllowedHost: config.AllowedHost,
		})
		if err != nil {
			return fmt.Errorf("failed to create success Slack handler: %w", err)
		}
		allHandlers = append(allHandlers, sh)
	}

	if config.WebhookURLError != "" {
		sh, err := newSlackHandlerFunc(logging.SlackHandlerOptions{
			WebhookURL:  config.WebhookURLError,
			RunID:       config.RunID,
			IsDryRun:    config.DryRun,
			LevelMode:   logging.LevelModeWarnAndAbove,
			AllowedHost: config.AllowedHost,
		})
		if err != nil {
			return fmt.Errorf("failed to create error Slack handler: %w", err)
		}
		allHandlers = append(allHandlers, sh)
	}

	multiHandler, err := logging.NewMultiHandler(allHandlers...)
	if err != nil {
		return fmt.Errorf("failed to create multi handler: %w", err)
	}
	redactedHandler := redaction.NewRedactingHandler(multiHandler, nil, phase1FailureLogger).
		WithErrorCollector(redactionErrorCollector)

	slog.SetDefault(slog.New(redactedHandler))
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
