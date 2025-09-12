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
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/terminal"
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
	ErrConfigPathRequired  = errors.New("config file path is required")
	ErrInvalidDetailLevel  = errors.New("invalid detail level - valid options are: summary, detailed, full")
	ErrInvalidOutputFormat = errors.New("invalid output format - valid options are: text, json")
)

// HashDirectoryErrorType represents different types of hash directory validation errors
type HashDirectoryErrorType int

const (
	// HashDirectoryErrorTypeRelativePath indicates a relative path was provided instead of absolute
	HashDirectoryErrorTypeRelativePath HashDirectoryErrorType = iota
	// HashDirectoryErrorTypeNotFound indicates the directory does not exist
	HashDirectoryErrorTypeNotFound
	// HashDirectoryErrorTypeNotDirectory indicates the path exists but is not a directory
	HashDirectoryErrorTypeNotDirectory
	// HashDirectoryErrorTypePermission indicates insufficient permissions to access the directory
	HashDirectoryErrorTypePermission
	// HashDirectoryErrorTypeSymlinkAttack indicates a potential symlink attack
	HashDirectoryErrorTypeSymlinkAttack
)

// HashDirectoryError represents an error in hash directory validation
type HashDirectoryError struct {
	Type  HashDirectoryErrorType
	Path  string
	Cause error
}

// Error implements the error interface for HashDirectoryError
func (e *HashDirectoryError) Error() string {
	switch e.Type {
	case HashDirectoryErrorTypeRelativePath:
		return fmt.Sprintf("hash directory must be absolute path, got relative path: %s", e.Path)
	case HashDirectoryErrorTypeNotFound:
		return fmt.Sprintf("hash directory not found: %s", e.Path)
	case HashDirectoryErrorTypeNotDirectory:
		return fmt.Sprintf("hash directory path is not a directory: %s", e.Path)
	case HashDirectoryErrorTypePermission:
		return fmt.Sprintf("insufficient permissions to access hash directory: %s", e.Path)
	case HashDirectoryErrorTypeSymlinkAttack:
		return fmt.Sprintf("potential symlink attack detected for hash directory: %s", e.Path)
	default:
		return fmt.Sprintf("unknown hash directory error for path: %s", e.Path)
	}
}

// Is implements error unwrapping for HashDirectoryError
func (e *HashDirectoryError) Is(target error) bool {
	if e.Cause != nil {
		return errors.Is(e.Cause, target)
	}
	return false
}

// Unwrap implements error unwrapping for HashDirectoryError
func (e *HashDirectoryError) Unwrap() error {
	return e.Cause
}

var (
	configPath       = flag.String("config", "", "path to config file")
	envFile          = flag.String("env-file", "", "path to environment file")
	logLevel         = flag.String("log-level", "info", "log level (debug, info, warn, error)")
	logDir           = flag.String("log-dir", "", "directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
	dryRun           = flag.Bool("dry-run", false, "print commands without executing them")
	dryRunFormat     = flag.String("dry-run-format", "text", "dry-run output format (text, json)")
	dryRunDetail     = flag.String("dry-run-detail", "detailed", "dry-run detail level (summary, detailed, full)")
	hashDirectory    = flag.String("hash-directory", "", "directory containing hash files (default: "+cmdcommon.DefaultHashDirectory+")")
	validateConfig   = flag.Bool("validate", false, "validate configuration file and exit")
	runID            = flag.String("run-id", "", "unique identifier for this execution run (auto-generates ULID if not provided)")
	forceInteractive = flag.Bool("interactive", false, "force interactive mode with colored output (overrides environment detection)")
	forceQuiet       = flag.Bool("quiet", false, "force non-interactive mode (disables colored output)")
)

// getHashDir determines the hash directory based on command line args and an embedded variable
func getHashDir() string {
	// Command line arguments take precedence over environment variables
	if *hashDirectory != "" {
		return *hashDirectory
	}
	// Set default hash directory if none specified
	return cmdcommon.DefaultHashDirectory
}

// validateHashDirectorySecurely validates hash directory with security checks
func validateHashDirectorySecurely(path string) (string, error) {
	// Check if path is absolute
	if !filepath.IsAbs(path) {
		return "", &HashDirectoryError{
			Type: HashDirectoryErrorTypeRelativePath,
			Path: path,
		}
	}

	// Clean the absolute path
	cleanPath := filepath.Clean(path)

	// Use safefileio pattern: recursively validate all parent directories for symlink attacks
	// This approach mirrors ensureParentDirsNoSymlinks but includes the target directory itself
	if err := validatePathComponentsSecurely(cleanPath); err != nil {
		// Convert safefileio errors to HashDirectoryError
		if errors.Is(err, safefileio.ErrIsSymlink) {
			return "", &HashDirectoryError{
				Type:  HashDirectoryErrorTypeSymlinkAttack,
				Path:  cleanPath,
				Cause: err,
			}
		}
		if errors.Is(err, safefileio.ErrInvalidFilePath) {
			return "", &HashDirectoryError{
				Type:  HashDirectoryErrorTypeNotDirectory,
				Path:  cleanPath,
				Cause: err,
			}
		}
		// Check for NotExist errors in the wrapped error
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && os.IsNotExist(pathErr.Err) {
			return "", &HashDirectoryError{
				Type:  HashDirectoryErrorTypeNotFound,
				Path:  cleanPath,
				Cause: err,
			}
		}
		return "", &HashDirectoryError{
			Type:  HashDirectoryErrorTypePermission,
			Path:  cleanPath,
			Cause: err,
		}
	}

	return cleanPath, nil
}

// validatePathComponentsSecurely validates all path components from root to target
// using the same secure approach as safefileio.ensureParentDirsNoSymlinks
func validatePathComponentsSecurely(absPath string) error {
	// Split path into components for step-by-step validation
	components := splitHashDirPathComponents(absPath)

	// Start from root and validate each component
	currentPath := filepath.VolumeName(absPath) + string(os.PathSeparator)

	for _, component := range components {
		currentPath = filepath.Join(currentPath, component)

		// Use os.Lstat to detect symlinks without following them
		fi, err := os.Lstat(currentPath)
		if err != nil {
			return fmt.Errorf("failed to validate path component %s: %w", currentPath, err)
		}

		// Reject any symlinks in the path hierarchy
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink found in path: %s", safefileio.ErrIsSymlink, currentPath)
		}

		// Ensure each component is a directory
		if !fi.IsDir() {
			return fmt.Errorf("%w: path component is not a directory: %s", safefileio.ErrInvalidFilePath, currentPath)
		}
	}

	return nil
}

// splitHashDirPathComponents splits directory path into components
// Similar to safefileio.splitPathComponents but includes target directory
func splitHashDirPathComponents(dirPath string) []string {
	components := []string{}
	current := dirPath

	for {
		parent := filepath.Dir(current)
		if parent == current {
			// Reached root directory
			break
		}

		components = append(components, filepath.Base(current))
		current = parent
	}

	// Reverse slice to get root-to-target order
	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}

	return components
}

// ErrDefaultHashDirectoryNotAbsolute is returned when DefaultHashDirectory is not an absolute path
var ErrDefaultHashDirectoryNotAbsolute = fmt.Errorf("default hash directory must be absolute path")

// validateDefaultHashDirectory validates that DefaultHashDirectory is an absolute path
func validateDefaultHashDirectory() error {
	if !filepath.IsAbs(cmdcommon.DefaultHashDirectory) {
		return fmt.Errorf("%w, got: %s", ErrDefaultHashDirectoryNotAbsolute, cmdcommon.DefaultHashDirectory)
	}
	return nil
}

// getHashDirectoryWithValidation determines hash directory with priority-based resolution and validation
func getHashDirectoryWithValidation() (string, error) {
	var path string

	// Priority 1: Command line argument
	if *hashDirectory != "" {
		path = *hashDirectory
	} else if envPath := os.Getenv("HASH_DIRECTORY"); envPath != "" {
		// Priority 2: Environment variable
		path = envPath
	} else {
		// Priority 3: Default value (already validated at startup)
		path = cmdcommon.DefaultHashDirectory
	}

	// Validate the resolved path securely
	return validateHashDirectorySecurely(path)
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

	// Validate DefaultHashDirectory early - this should never fail in production
	// but helps catch build-time configuration errors
	if err := validateDefaultHashDirectory(); err != nil {
		logging.HandlePreExecutionError(logging.ErrorTypeBuildConfig, fmt.Sprintf("Invalid default hash directory: %v", err), "main", *runID)
		os.Exit(1)
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

	// Load and validate configuration
	cfg, err := loadAndValidateConfig(runID)
	if err != nil {
		return err
	}

	// Handle validate command
	if *validateConfig {
		return validateConfigCommand(cfg)
	}

	// Setup environment and logging
	envFileToLoad, err := setupEnvironmentAndLogging(runID)
	if err != nil {
		return err
	}

	// Initialize verification and security
	verificationManager, err := initializeVerificationManager(runID)
	if err != nil {
		return err
	}

	// Perform file verification
	if err := performFileVerification(verificationManager, cfg, envFileToLoad, runID); err != nil {
		return err
	}

	// Initialize and execute runner
	return executeRunner(ctx, cfg, verificationManager, envFileToLoad, runID)
}

// loadAndValidateConfig loads configuration from file and validates basic requirements
func loadAndValidateConfig(runID string) (*runnertypes.Config, error) {
	if *configPath == "" {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeRequiredArgumentMissing,
			Message:   "Config file path is required",
			Component: "config",
			RunID:     runID,
		}
	}

	cfgLoader := config.NewLoader()
	cfg, err := cfgLoader.LoadConfig(*configPath)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Failed to load config: %v", err),
			Component: "config",
			RunID:     runID,
		}
	}

	return cfg, nil
}

// setupEnvironmentAndLogging determines environment file and sets up logging system
func setupEnvironmentAndLogging(runID string) (string, error) {
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
		return "", fmt.Errorf("failed to read Slack configuration from environment file: %w", err)
	}

	// Setup logging system with all configuration including Slack
	loggerConfig := LoggerConfig{
		Level:           *logLevel,
		LogDir:          *logDir,
		RunID:           runID,
		SlackWebhookURL: slackURL,
	}

	if err := setupLoggerWithConfig(loggerConfig); err != nil {
		return "", &logging.PreExecutionError{
			Type:      logging.ErrorTypeLogFileOpen,
			Message:   fmt.Sprintf("Failed to setup logger: %v", err),
			Component: "logging",
			RunID:     runID,
		}
	}

	return envFileToLoad, nil
}

// initializeVerificationManager creates and configures the verification manager
func initializeVerificationManager(runID string) (*verification.Manager, error) {
	// Get hash directory from command line args and validate early
	hashDir := getHashDir()
	if !filepath.IsAbs(hashDir) {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Hash directory must be absolute path, got relative path: %s", hashDir),
			Component: "file",
			RunID:     runID,
		}
	}
	info, err := os.Stat(hashDir)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Failed to access hash directory: %s", hashDir),
			Component: "file",
			RunID:     runID,
		}
	} else if !info.IsDir() {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Hash directory is not a directory: %s", hashDir),
			Component: "file",
			RunID:     runID,
		}
	}

	// Initialize privilege manager
	logger := slog.Default()
	privMgr := privilege.NewManager(logger)

	// Initialize verification manager with privilege support
	verificationManager, err := verification.NewManagerWithOpts(
		hashDir,
		verification.WithPrivilegeManager(privMgr),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize verification: %w", err)
	}

	return verificationManager, nil
}

// performFileVerification verifies configuration, environment, and global files
func performFileVerification(verificationManager *verification.Manager, cfg *runnertypes.Config, envFileToLoad, runID string) error {
	// Verify configuration file integrity
	if err := verificationManager.VerifyConfigFile(*configPath); err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Config verification failed: %v", err),
			Component: "verification",
			RunID:     runID,
		}
	}

	// Verify environment file integrity if specified
	if envFileToLoad != "" {
		if err := verificationManager.VerifyEnvironmentFile(envFileToLoad); err != nil {
			return &logging.PreExecutionError{
				Type:      logging.ErrorTypeFileAccess,
				Message:   fmt.Sprintf("Environment file verification failed: %v", err),
				Component: "verification",
				RunID:     runID,
			}
		}
	}

	// Verify global files - CRITICAL: Program must exit if global verification fails
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

	return nil
}

// executeRunner initializes and executes the runner with proper cleanup
func executeRunner(ctx context.Context, cfg *runnertypes.Config, verificationManager *verification.Manager, envFileToLoad, runID string) error {
	// Initialize privilege manager
	logger := slog.Default()
	privMgr := privilege.NewManager(logger)

	// Get hash directory from command line args
	hashDir := getHashDir()

	// Initialize Runner with privilege support and run ID
	runnerOptions := []runner.Option{
		runner.WithVerificationManager(verificationManager),
		runner.WithPrivilegeManager(privMgr),
		runner.WithRunID(runID),
	}

	// Parse dry-run options once for the entire function
	var detailLevel resource.DetailLevel
	var outputFormat resource.OutputFormat

	// Add dry-run mode if requested
	if *dryRun {
		// Parse detail level
		var err error
		detailLevel, err = parseDryRunDetailLevel(*dryRunDetail)
		if err != nil {
			return fmt.Errorf("invalid detail level %q: %w", *dryRunDetail, err)
		}

		// Parse output format
		outputFormat, err = parseDryRunOutputFormat(*dryRunFormat)
		if err != nil {
			return fmt.Errorf("invalid output format %q: %w", *dryRunFormat, err)
		}

		dryRunOpts := &resource.DryRunOptions{
			DetailLevel:       detailLevel,
			OutputFormat:      outputFormat,
			ShowSensitive:     false,
			VerifyFiles:       true,
			SkipStandardPaths: cfg.Global.SkipStandardPaths, // Use setting from TOML config
			HashDir:           hashDir,                      // Hash directory from command line args
		}
		runnerOptions = append(runnerOptions, runner.WithDryRun(dryRunOpts))
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

	// Ensure cleanup of all resources on exit
	defer func() {
		if err := runner.CleanupAllResources(); err != nil {
			slog.Warn("Failed to cleanup resources", "error", err, "run_id", runID)
		}
	}()

	// Execute all groups (works for both normal and dry-run modes)
	if err := runner.ExecuteAll(ctx); err != nil {
		return fmt.Errorf("error running commands: %w", err)
	}

	// If dry-run mode, display the analysis results
	if *dryRun {
		result := runner.GetDryRunResults()
		if result != nil {
			// Create appropriate formatter using pre-parsed values
			var formatter resource.Formatter
			switch outputFormat {
			case resource.OutputFormatText:
				formatter = resource.NewTextFormatter()
			case resource.OutputFormatJSON:
				formatter = resource.NewJSONFormatter()
			}

			output, err := formatter.FormatResult(result, resource.FormatterOptions{
				DetailLevel:   detailLevel,
				OutputFormat:  outputFormat,
				ShowSensitive: false,
			})
			if err != nil {
				return fmt.Errorf("formatting failed: %w", err)
			}
			fmt.Print(output)
		}
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

	// Parse log level for all handlers
	var slogLevel slog.Level
	if err := slogLevel.UnmarshalText([]byte(config.Level)); err != nil {
		slogLevel = slog.LevelInfo // Default to info on parse error
		invalidLogLevel = true
	}

	// Initialize terminal capabilities with command line overrides
	terminalOptions := terminal.Options{
		DetectorOptions: terminal.DetectorOptions{
			ForceInteractive:    *forceInteractive,
			ForceNonInteractive: *forceQuiet,
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

// parseDryRunDetailLevel converts string to DetailLevel enum
func parseDryRunDetailLevel(level string) (resource.DetailLevel, error) {
	switch level {
	case "summary":
		return resource.DetailLevelSummary, nil
	case "detailed":
		return resource.DetailLevelDetailed, nil
	case "full":
		return resource.DetailLevelFull, nil
	default:
		return resource.DetailLevelSummary, ErrInvalidDetailLevel
	}
}

// parseDryRunOutputFormat converts string to OutputFormat enum
func parseDryRunOutputFormat(format string) (resource.OutputFormat, error) {
	switch format {
	case "text":
		return resource.OutputFormatText, nil
	case "json":
		return resource.OutputFormatJSON, nil
	default:
		return resource.OutputFormatText, ErrInvalidOutputFormat
	}
}
