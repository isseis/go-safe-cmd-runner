// Package main provides the entry point for the command runner application.
// It handles command-line arguments, configuration loading, and orchestrates
// the execution of commands based on the provided configuration.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
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

// Error definitions
var (
	ErrConfigPathRequired = errors.New("config file path is required")
)

var (
	configPath     = flag.String("config", "", "path to config file")
	envFile        = flag.String("env-file", "", "path to environment file")
	logLevel       = flag.String("log-level", "info", "log level (debug, info, warn, error)")
	logDir         = flag.String("log-dir", "", "directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
	dryRun         = flag.Bool("dry-run", false, "print commands without executing them")
	hashDirectory  = flag.String("hash-directory", "", "directory containing hash files (default: "+cmdcommon.DefaultHashDirectory+")")
	validateConfig = flag.Bool("validate", false, "validate configuration file and exit")
	runID          = flag.String("run-id", "", "unique identifier for this execution run (auto-generates ULID if not provided)")
)

// getHashDir determines the hash directory based on command line args and environment variables
func getHashDir() string {
	// Command line arguments take precedence over environment variables
	if *hashDirectory != "" {
		return *hashDirectory
	}
	// Check environment variable for hash directory override
	if envHashDir := os.Getenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY"); envHashDir != "" {
		return envHashDir
	}
	// Set default hash directory if none specified
	return cmdcommon.DefaultHashDirectory
}

// validateConfigCommand implements config validation CLI command
func validateConfigCommand(cfg *runnertypes.Config) error {
	// Validate config
	validator := config.NewConfigValidator()
	result, err := validator.ValidateConfig(cfg)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Generate and display report
	report, err := validator.GenerateValidationReport(result)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Print(report)

	// Exit with appropriate code
	if !result.Valid {
		os.Exit(1)
	}

	return nil
}

func main() {
	// Parse command line flags early to get runID
	flag.Parse()

	// Use provided run ID or generate one for error handling
	if *runID == "" {
		*runID = logging.GenerateRunID()
	}

	if err := syscall.Seteuid(syscall.Getuid()); err != nil {
		logging.HandlePreExecutionError(logging.ErrorTypePrivilegeDrop, fmt.Sprintf("Failed to drop privileges: %v", err), "main", *runID)
		os.Exit(1)
	}

	// Wrap main logic in a separate function to properly handle errors and defer
	if err := run(*runID); err != nil {
		// Check if this is a pre-execution error using errors.As for safe type checking
		var preExecErr *logging.PreExecutionError
		if errors.As(err, &preExecErr) {
			logging.HandlePreExecutionError(preExecErr.Type, preExecErr.Message, preExecErr.Component, *runID)
		} else {
			logging.HandlePreExecutionError(logging.ErrorTypeSystemError, err.Error(), "main", *runID)
		}
		os.Exit(1)
	}
}

func run(runID string) error {
	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Load configuration first to access environment settings
	if *configPath == "" {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeRequiredArgumentMissing,
			Message:   "Config file path is required",
			Component: "config",
			RunID:     runID,
		}
	}

	// Initialize config loader
	cfgLoader := config.NewLoader()
	cfg, err := cfgLoader.LoadConfig(*configPath)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Failed to load config: %v", err),
			Component: "config",
			RunID:     runID,
		}
	}

	// Handle validate command
	if *validateConfig {
		return validateConfigCommand(cfg)
	}

	// Determine environment file to load
	envFileToLoad := ""
	if *envFile != "" {
		envFileToLoad = *envFile
	} else {
		// Try to load default '.env' file if exists
		if _, err := os.Stat(".env"); err == nil {
			envFileToLoad = ".env"
		}
	}

	// Get Slack webhook URL from environment file early
	slackURL, err := getSlackWebhookFromEnvFile(envFileToLoad)
	if err != nil {
		return fmt.Errorf("failed to read Slack configuration from environment file: %w", err)
	}

	// Setup logging system with all configuration including Slack
	loggerConfig := LoggerConfig{
		Level:           *logLevel,
		LogDir:          *logDir,
		RunID:           runID,
		SlackWebhookURL: slackURL,
	}

	if err := setupLoggerWithConfig(loggerConfig); err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeLogFileOpen,
			Message:   fmt.Sprintf("Failed to setup logger: %v", err),
			Component: "logging",
			RunID:     runID,
		}
	}

	// Get hash directory from command line args and environment variables
	hashDir := getHashDir()

	// Initialize privilege manager
	logger := slog.Default()
	privMgr := privilege.NewManager(logger)

	// Initialize verification manager with privilege support
	verificationManager, err := verification.NewManagerWithOpts(
		hashDir,
		verification.WithPrivilegeManager(privMgr),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize verification: %w", err)
	}

	// Verify configuration file integrity
	if err := verificationManager.VerifyConfigFile(*configPath); err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Config verification failed: %v", err),
			Component: "verification",
			RunID:     runID,
		}
	}

	// Verify global files - CRITICAL: Program must exit if global verification fails
	// to prevent execution with potentially compromised files
	result, err := verificationManager.VerifyGlobalFiles(&cfg.Global)
	if err != nil {
		slog.Error("CRITICAL: Global file verification failed - terminating program for security", "error", err)
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Global files verification failed: %v", err),
			Component: "verification",
			RunID:     runID,
		}
	}

	// Log global verification results
	if result.TotalFiles > 0 {
		slog.Info("Global files verification completed successfully",
			"verified", result.VerifiedFiles,
			"skipped", len(result.SkippedFiles),
			"duration_ms", result.Duration.Milliseconds(),
			"run_id", runID)
	}

	// Initialize Runner with privilege support and run ID
	runnerOptions := []runner.Option{
		runner.WithVerificationManager(verificationManager),
		runner.WithPrivilegeManager(privMgr),
		runner.WithRunID(runID),
	}

	runner, err := runner.NewRunner(cfg, runnerOptions...)
	if err != nil {
		return fmt.Errorf("failed to initialize runner: %w", err)
	}

	// Load environment variables from file and system environment
	if err := runner.LoadEnvironment(envFileToLoad, true); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	if *logLevel != "" {
		cfg.Global.LogLevel = *logLevel
	}

	// Run the command groups
	if *dryRun {
		fmt.Println("[DRY RUN] Would execute the following groups:")
		runner.ListCommands()
		return nil
	}

	// Ensure cleanup of all resources on exit (both auto-cleanup and manual cleanup resources)
	defer func() {
		if err := runner.CleanupAllResources(); err != nil {
			slog.Warn("Failed to cleanup resources", "error", err, "run_id", runID)
		}
	}()

	if err := runner.ExecuteAll(ctx); err != nil {
		return fmt.Errorf("error running commands: %w", err)
	}
	return nil
}

// getSlackWebhookFromEnvFile securely reads Slack webhook URL from .env file
// Returns the webhook URL and an error if any issues occur during file access or parsing
func getSlackWebhookFromEnvFile(envFile string) (string, error) {
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

// setupLoggerWithConfig initializes the logging system with all handlers atomically
func setupLoggerWithConfig(config LoggerConfig) error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}
	timestamp := time.Now().Format("20060102T150405Z")

	var handlers []slog.Handler
	var invalidLogLevel bool

	// 1. Human-readable summary handler (to stdout)
	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	handlers = append(handlers, textHandler)

	// 2. Machine-readable log handler (to file, per-run auto-named)
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

		var slogLevel slog.Level
		if err := slogLevel.UnmarshalText([]byte(config.Level)); err != nil {
			slogLevel = slog.LevelInfo // Default to info on parse error
			invalidLogLevel = true
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

	// 3. Slack notification handler (optional)
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
		"slack_enabled", config.SlackWebhookURL != "")

	// Warn about invalid log level after logger is properly set up
	if invalidLogLevel {
		slog.Warn("Invalid log level provided, defaulting to INFO", "provided", config.Level)
	}

	return nil
}
