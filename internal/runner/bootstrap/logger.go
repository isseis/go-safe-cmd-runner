package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
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
}

// SlackLoggerConfig is the Slack-handler-only config passed to AddSlackHandlers.
type SlackLoggerConfig struct {
	WebhookURLSuccess string // Webhook URL for success notifications (INFO)
	WebhookURLError   string // Webhook URL for error notifications (WARN/ERROR)
	AllowedHost       string // Allowed hostname (AC-L2-4)
	RunID             string
	DryRun            bool
}

// redactionErrorCollector is a global collector for redaction failures
// This is set during logger initialization and used for shutdown reporting
var redactionErrorCollector *redaction.InMemoryErrorCollector

// redactionReporter is a global reporter for shutdown
var redactionReporter *redaction.ShutdownReporter

// errPhase1NotInitialized is returned when AddSlackHandlers is called before SetupLoggerWithConfig.
var errPhase1NotInitialized = errors.New("AddSlackHandlers called before SetupLoggerWithConfig")

// phase1BaseHandlers holds the non-Slack handlers created by SetupLoggerWithConfig.
// AddSlackHandlers reads this to build a new MultiHandler that includes the Slack handlers.
var phase1BaseHandlers []slog.Handler

// phase1FailureLogger is the failureLogger created in Phase 1.
// AddSlackHandlers reuses it when rebuilding the RedactingHandler.
var phase1FailureLogger *slog.Logger

// newSlackHandlerFunc is the factory for creating Slack handlers.
// Replacing it in tests allows inspection of SlackHandlerOptions (AC-L2-19).
var newSlackHandlerFunc = logging.NewSlackHandler

// Webhook roles, passed to the handlers as SlackHandlerOptions.WebhookLabel.
const (
	slackRoleSuccess = "success"
	slackRoleError   = "error"
)

// slackHandlerEntry is one Slack handler together with the webhook role it
// serves. The role is stored rather than read back from the handler, which
// does not expose it.
type slackHandlerEntry struct {
	role    string
	handler *logging.SlackHandler
}

// slackHandlers holds the Slack handlers created by the last successful
// AddSlackHandlers call. Without an owner here, the worker goroutines started
// by NewSlackHandler would have nobody to flush or stop them. It is written
// only by AddSlackHandlers and FlushSlackNotifications, both of which run on
// the single-threaded bootstrap and shutdown paths.
var slackHandlers []slackHandlerEntry

// slackEnvSettings holds the Slack delivery settings taken from the
// environment.
type slackEnvSettings struct {
	// SendTimeout bounds one notification's delivery, FlushTimeout the whole
	// flush at process exit.
	SendTimeout  time.Duration
	FlushTimeout time.Duration
	Synchronous  bool
	// invalidVars names the variables whose value could not be used; each fell
	// back to its default. Only the names are kept: these are reported through
	// the send-failure logger, which does not pass through the redaction layer,
	// so a mistyped value -- which may be anything at all -- must not travel
	// with them.
	invalidVars []string
}

// parseSlackEnvSettings interprets the three Slack delivery environment
// variables. It logs nothing and takes getenv as an argument, so a caller can
// parse without emitting records; reportInvalidEnvSettings does the reporting.
func parseSlackEnvSettings(getenv func(string) string) slackEnvSettings {
	settings := slackEnvSettings{
		// Only the exact "1" switches modes, so a spelling nobody defined
		// ("true", "yes") leaves the supported asynchronous path in place.
		Synchronous: getenv(logging.SlackSyncEnvVar) == "1",
	}

	var ok bool
	if settings.SendTimeout, ok = parsePositiveDuration(getenv(logging.SlackSendTimeoutEnvVar), logging.DefaultSendTimeout); !ok {
		settings.invalidVars = append(settings.invalidVars, logging.SlackSendTimeoutEnvVar)
	}
	if settings.FlushTimeout, ok = parsePositiveDuration(getenv(logging.SlackFlushTimeoutEnvVar), logging.DefaultFlushTimeout); !ok {
		settings.invalidVars = append(settings.invalidVars, logging.SlackFlushTimeoutEnvVar)
	}

	return settings
}

// parsePositiveDuration returns the parsed duration, or fallback and false when
// the value cannot be used. An unset value takes the fallback without being
// reported: declining to override a default is not a mistake. A zero or
// negative duration is refused because it would expire the deadline before the
// first send, turning a typo into silently undelivered notifications.
func parsePositiveDuration(raw string, fallback time.Duration) (time.Duration, bool) {
	if raw == "" {
		return fallback, true
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback, false
	}
	return d, true
}

// reportInvalidEnvSettings warns about each setting that fell back to its
// default. It is called once, from AddSlackHandlers, so an operator who
// mistyped a value learns of it at startup rather than at exit.
func reportInvalidEnvSettings(settings slackEnvSettings) {
	if phase1FailureLogger == nil {
		return
	}
	for _, name := range settings.invalidVars {
		phase1FailureLogger.Warn("Ignoring unusable Slack delivery setting, using the default instead",
			"env_var", name)
	}
}

// closeSlackHandlers terminates the given handlers' workers without draining.
func closeSlackHandlers(entries []slackHandlerEntry) {
	for _, e := range entries {
		e.handler.Close()
	}
}

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
	// Reject malformed run IDs before any handler, file, or global state is
	// touched. This is a defense-in-depth layer independent of the entrypoint
	// validation in cmd/runner, so it must apply regardless of whether a log
	// directory is configured.
	if err := logging.ValidateRunID(config.RunID); err != nil {
		return fmt.Errorf("invalid run ID: %w", err)
	}

	hostname := common.GetHostname()
	// UTC keeps the trailing "Z" honest and makes log file names sort
	// chronologically across hosts in different time zones.
	timestamp := time.Now().UTC().Format("20060102T150405Z")

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

	// Create redaction error collector for monitoring failures
	// Limit to 1000 most recent failures to prevent unbounded growth
	const maxRedactionFailures = 1000
	collector := redaction.NewInMemoryErrorCollector(maxRedactionFailures)

	// Create MultiHandler with redaction (Phase 1 handlers only; Slack added via AddSlackHandlers)
	multiHandler, err := logging.NewMultiHandler(handlers...)
	if err != nil {
		return fmt.Errorf("failed to create multi handler: %w", err)
	}
	redactedHandler := redaction.NewRedactingHandler(multiHandler, nil, failureLogger).
		WithErrorCollector(collector)

	// Create shutdown reporter for redaction failures
	reporter := redaction.NewShutdownReporter(collector, os.Stderr, failureLogger)

	// Set as default logger
	slog.SetDefault(slog.New(redactedHandler))

	// All initialization succeeded -- commit Phase 1 state for AddSlackHandlers to reference.
	// These globals must only be set after every step above has completed without error,
	// so that AddSlackHandlers cannot run against a partially-initialized logger.
	phase1BaseHandlers = handlers
	phase1FailureLogger = failureLogger
	redactionErrorCollector = collector
	redactionReporter = reporter

	slog.Info("Logger initialized",
		"log-level", config.Level,
		"log-dir", config.LogDir,
		"run_id", config.RunID,
		"hostname", hostname,
		"interactive_mode", capabilities.IsInteractive(),
		"color_support", capabilities.SupportsColor())

	return nil
}

// AddSlackHandlers rebuilds the default logger by appending Slack handlers to the existing logger.
// Returns an error if validateWebhookURL fails for either successURL or errorURL.
// Returns an error if SetupLoggerWithConfig has not been called (phase1BaseHandlers is nil).
// On success, it also returns the redaction Config it built, so callers that
// construct their own redaction-aware components (e.g. runner.WithRedactionConfig)
// can share the exact same webhook-host masking instead of each rebuilding it.
//
// Like SetupLoggerWithConfig, this belongs to single-threaded bootstrap: it
// reads and replaces package-level state (the default logger and the registered
// Slack handlers) without synchronisation, so it must not run concurrently with
// itself or with FlushSlackNotifications.
func AddSlackHandlers(config SlackLoggerConfig) (*redaction.Config, error) {
	if phase1BaseHandlers == nil || phase1FailureLogger == nil || redactionErrorCollector == nil {
		return nil, errPhase1NotInitialized
	}

	// Closed once the rebuild has committed, not here: on a failed rebuild the
	// default logger still routes to these handlers, and closing them up front
	// would leave every later notification dropped with no owner able to flush
	// it.
	previous := slackHandlers

	settings := parseSlackEnvSettings(os.Getenv)
	reportInvalidEnvSettings(settings)

	allHandlers := make([]slog.Handler, len(phase1BaseHandlers))
	copy(allHandlers, phase1BaseHandlers)

	// Until the rebuild commits, this call is the only owner of the workers the
	// handlers below start, so every early return has to close them.
	var created []slackHandlerEntry
	committed := false
	defer func() {
		if !committed {
			closeSlackHandlers(created)
		}
	}()

	if config.WebhookURLSuccess != "" {
		sh, err := newSlackHandlerFunc(logging.SlackHandlerOptions{
			WebhookURL:   config.WebhookURLSuccess,
			RunID:        config.RunID,
			IsDryRun:     config.DryRun,
			LevelMode:    logging.LevelModeExactInfo,
			AllowedHost:  config.AllowedHost,
			WebhookLabel: slackRoleSuccess,
			// The send path's own diagnostics go to the Phase 1 handlers -- the
			// same console and log-file handlers phase1FailureLogger is built
			// from -- so they land where every other record does. Left empty
			// they would go to a bare stderr logger instead, dropping them from
			// the run's JSON log and ignoring the configured level.
			FailureHandlers: phase1BaseHandlers,
			SendTimeout:     settings.SendTimeout,
			Synchronous:     settings.Synchronous,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create success Slack handler: %w", err)
		}
		created = append(created, slackHandlerEntry{role: slackRoleSuccess, handler: sh})
		allHandlers = append(allHandlers, sh)
	}

	if config.WebhookURLError != "" {
		sh, err := newSlackHandlerFunc(logging.SlackHandlerOptions{
			WebhookURL:      config.WebhookURLError,
			RunID:           config.RunID,
			IsDryRun:        config.DryRun,
			LevelMode:       logging.LevelModeWarnAndAbove,
			AllowedHost:     config.AllowedHost,
			WebhookLabel:    slackRoleError,
			FailureHandlers: phase1BaseHandlers,
			SendTimeout:     settings.SendTimeout,
			Synchronous:     settings.Synchronous,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create error Slack handler: %w", err)
		}
		created = append(created, slackHandlerEntry{role: slackRoleError, handler: sh})
		allHandlers = append(allHandlers, sh)
	}

	multiHandler, err := logging.NewMultiHandler(allHandlers...)
	if err != nil {
		return nil, fmt.Errorf("failed to create multi handler: %w", err)
	}

	// Phase 1 built its RedactingHandler before the TOML was read, so webhook
	// masking was not active yet (see WithWebhookHost). The configured host is
	// known now, so rebuild the redaction Config with it: a deployment pointing
	// at a Slack-compatible endpoint gets its own webhook URLs masked from this
	// point on, which is where they can first appear in a log line. AllowedHost
	// is empty when Slack is switched off, and WithWebhookHost then changes
	// nothing.
	redactionConfig, err := redaction.NewConfig(redaction.WithWebhookHost(config.AllowedHost))
	if err != nil {
		return nil, fmt.Errorf("failed to create redaction config: %w", err)
	}
	redactedHandler := redaction.NewRedactingHandler(multiHandler, redactionConfig, phase1FailureLogger).
		WithErrorCollector(redactionErrorCollector)

	slog.SetDefault(slog.New(redactedHandler))

	// The rebuild has committed: the new handlers are reachable both through
	// the default logger and, from here, through the exit flush -- and the
	// previous registration is finally unreachable.
	committed = true
	slackHandlers = created
	closeSlackHandlers(previous)

	return redactionConfig, nil
}

// FlushSlackNotifications delivers whatever the Slack handlers still have
// queued and stops their workers. cmd/runner calls it once, after the run has
// returned and before ReportRedactionFailures, so records issued during the
// run are sent before the process exits.
//
// All webhooks are flushed concurrently under one shared deadline, so adding a
// second webhook does not double the time the process spends exiting. Whatever
// the deadline leaves undelivered is reported and then given up on: this
// returns no error and does not influence the exit code, because whether a
// notification got out is independent of whether the commands succeeded.
//
// Flushing deregisters the handlers, so a second call is a no-op. Like
// AddSlackHandlers it touches package-level state unsynchronised and belongs to
// the single-threaded shutdown path.
func FlushSlackNotifications() {
	if len(slackHandlers) == 0 {
		return
	}

	// Re-read rather than remembered from AddSlackHandlers, which has already
	// reported anything unusable, so nothing is reported twice.
	settings := parseSlackEnvSettings(os.Getenv)

	ctx, cancel := context.WithTimeout(context.Background(), settings.FlushTimeout)
	defer cancel()

	entries := slackHandlers
	// The flush terminates these handlers, so a second call has nothing left to
	// do -- and repeating the warning below would report the same lost
	// notifications twice.
	slackHandlers = nil

	stats := make([]logging.FlushStats, len(entries))
	// this WaitGroup is what makes the Slack flush concurrent: it lets one
	// goroutine per handler call Flush at the same time instead of serially.
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Go(func() {
			stats[i] = entry.handler.Flush(ctx)
		})
	}
	wg.Wait()

	reportUndeliveredNotifications(entries, stats)
}

// reportUndeliveredNotifications writes one stderr line per webhook that lost
// notifications, and nothing at all otherwise. The structured summary is not
// repeated here: each sender writes its own, complete with the per-message-type
// breakdown and the webhook label this package gave it.
func reportUndeliveredNotifications(entries []slackHandlerEntry, stats []logging.FlushStats) {
	for i, entry := range entries {
		s := stats[i]
		if undelivered := s.Failed + s.Dropped + s.Pending; undelivered > 0 {
			fmt.Fprintf(os.Stderr,
				"Warning: %d Slack notification(s) for the %s webhook were not delivered (failed: %d, dropped: %d, pending: %d)\n",
				undelivered, entry.role, s.Failed, s.Dropped, s.Pending)
		}
	}
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
