// Package main provides the entry point for the command runner application.
// It handles command-line arguments, configuration loading, and orchestrates
// the execution of commands based on the provided configuration.
package main

import (
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
	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

const (
	// File permissions for log files
	logFilePerm = 0o600
)

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
	// Generate run ID early for error handling
	runID := logging.GenerateRunID()

	if err := syscall.Seteuid(syscall.Getuid()); err != nil {
		logging.HandlePreExecutionError(logging.ErrorTypePrivilegeDrop, fmt.Sprintf("Failed to drop privileges: %v", err), "main", runID)
		os.Exit(1)
	}

	// Wrap main logic in a separate function to properly handle errors and defer
	if err := run(runID); err != nil {
		// Check if this is a pre-execution error
		if preExecErr, ok := err.(*logging.PreExecutionError); ok {
			logging.HandlePreExecutionError(preExecErr.Type, preExecErr.Message, preExecErr.Component, runID)
		} else {
			logging.HandlePreExecutionError(logging.ErrorTypeSystemError, err.Error(), "main", runID)
		}
		os.Exit(1)
	}
}

func run(runID string) error {
	flag.Parse()

	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Setup logging system early
	if err := setupLogger(*logLevel, *logDir, runID); err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeLogFileOpen,
			Message:   fmt.Sprintf("Failed to setup logger: %v", err),
			Component: "logging",
			RunID:     runID,
		}
	}

	// Load configuration
	if *configPath == "" {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeInvalidArguments,
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

	// Initialize Runner with privilege support
	runner, err := runner.NewRunner(cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithPrivilegeManager(privMgr))
	if err != nil {
		return fmt.Errorf("failed to initialize runner: %w", err)
	}

	// Load environment variables
	envFileToLoad := ""
	if *envFile != "" {
		envFileToLoad = *envFile
	} else {
		// Try to load default '.env' file if exists
		if _, err := os.Stat(".env"); err == nil {
			envFileToLoad = ".env"
		}
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

// setupLogger initializes the logging system
func setupLogger(level, logDir, runID string) error {
	hostname, _ := os.Hostname()
	timestamp := time.Now().Format("20060102T150405Z")

	var handlers []slog.Handler

	// 1. Human-readable summary handler (to stdout)
	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	handlers = append(handlers, textHandler)

	// 2. Machine-readable log handler (to file, per-run auto-named)
	if logDir != "" {
		// Validate log directory
		if err := logging.ValidateLogDir(logDir); err != nil {
			return fmt.Errorf("invalid log directory: %w", err)
		}

		logPath := filepath.Join(logDir, fmt.Sprintf("%s_%s_%s.json", hostname, timestamp, runID))
		fileOpener := logging.NewSafeFileOpener()
		logF, err := fileOpener.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logFilePerm)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}

		var slogLevel slog.Level
		if err := slogLevel.UnmarshalText([]byte(level)); err != nil {
			slogLevel = slog.LevelInfo // Default to info on parse error
			slog.Warn("Invalid log level provided, defaulting to INFO", "provided", level)
		}

		jsonHandler := slog.NewJSONHandler(logF, &slog.HandlerOptions{
			Level: slogLevel,
		})

		// Attach common attributes
		gitCommit, buildVersion := logging.GetBuildInfo()
		enrichedHandler := jsonHandler.WithAttrs([]slog.Attr{
			slog.String("hostname", hostname),
			slog.Int("pid", os.Getpid()),
			slog.String("git_commit", gitCommit),
			slog.String("build_version", buildVersion),
			slog.Int("schema_version", 1),
			slog.String("run_id", runID),
		})
		handlers = append(handlers, enrichedHandler)
	}

	// 3. Slack notification handler (optional)
	if slackURL := logging.GetSlackWebhookURL(); slackURL != "" {
		slackHandler := logging.NewSlackHandler(slackURL, runID)
		handlers = append(handlers, slackHandler)
	}

	// Create MultiHandler with redaction
	multiHandler := logging.NewMultiHandler(handlers...)
	redactedHandler := logging.NewRedactingHandler(multiHandler, logging.DefaultRedactionConfig())

	// Set as default logger
	logger := slog.New(redactedHandler)
	slog.SetDefault(logger)

	slog.Info("Logger initialized",
		"log-level", level,
		"log-dir", logDir,
		"run_id", runID,
		"hostname", hostname)

	return nil
}
