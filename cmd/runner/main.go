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

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/cli"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// SilentExitError indicates that the program should exit with status 1
// without printing additional error messages (e.g., for validation failures
// where the validation report has already been displayed)
type SilentExitError struct{}

func (e SilentExitError) Error() string {
	return "silent exit requested"
}

var (
	configPath       = flag.String("config", "", "path to config file")
	logLevel         = flag.String("log-level", "info", "log level (debug, info, warn, error)")
	logDir           = flag.String("log-dir", "", "directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
	dryRun           = flag.Bool("dry-run", false, "print commands without executing them")
	dryRunFormat     = flag.String("dry-run-format", "text", "dry-run output format (text, json)")
	dryRunDetail     = flag.String("dry-run-detail", "detailed", "dry-run detail level (summary, detailed, full)")
	showSensitive    = flag.Bool("show-sensitive", false, "show sensitive information in dry-run output (use with caution)")
	validateConfig   = flag.Bool("validate", false, "validate configuration file and exit")
	runID            = flag.String("run-id", "", "unique identifier for this execution run (auto-generates ULID if not provided)")
	forceInteractive = flag.Bool("interactive", false, "force interactive mode with colored output (overrides environment detection)")
	forceQuiet       = flag.Bool("quiet", false, "force non-interactive mode (disables colored output)")
	keepTempDirs     = flag.Bool("keep-temp-dirs", false, "keep temporary directories after execution")
)

func main() {
	// Parse command line flags early to get runID
	flag.Parse()

	// Use provided run ID or generate one for error handling
	if *runID == "" {
		*runID = logging.GenerateRunID()
	}

	// Validate DefaultHashDirectory early - this should never fail in production
	// but helps catch build-time configuration errors
	if !filepath.IsAbs(cmdcommon.DefaultHashDirectory) {
		logging.HandlePreExecutionError(logging.ErrorTypeBuildConfig, fmt.Sprintf("Invalid default hash directory: must be absolute path, got: %s", cmdcommon.DefaultHashDirectory), "main", *runID)
		os.Exit(1)
	}

	if err := syscall.Seteuid(syscall.Getuid()); err != nil {
		logging.HandlePreExecutionError(logging.ErrorTypePrivilegeDrop, fmt.Sprintf("Failed to drop privileges: %v", err), "main", *runID)
		os.Exit(1)
	}

	// Run main logic and capture exit code
	exitCode := mainWithExitCode(*runID)

	// Ensure redaction failures are reported before exit
	bootstrap.ReportRedactionFailures()

	// Exit with captured code
	os.Exit(exitCode)
}

// mainWithExitCode runs the main logic and returns the exit code
func mainWithExitCode(runID string) int {
	// Wrap main logic in a separate function to properly handle errors and defer
	if err := run(runID); err != nil {
		var silentErr SilentExitError
		var preExecErr *logging.PreExecutionError
		var execErr *logging.ExecutionError
		switch {
		case errors.As(err, &silentErr):
			// Check for silent exit error first (validation failure with report already printed)
			// revive:disable:empty-block This empty block is intentional to handle specific cases
		case errors.As(err, &preExecErr):
			// Check if this is a pre-execution error using errors.As for safe type checking
			logging.HandlePreExecutionError(preExecErr.Type, preExecErr.Message, preExecErr.Component, runID)
		case errors.As(err, &execErr):
			// Check if this is an execution error (error during command execution)
			logging.HandleExecutionError(execErr)
		default:
			logging.HandlePreExecutionError(logging.ErrorTypeSystemError, err.Error(), "main", runID)
		}
		return 1
	}
	return 0
}

// parseLogLevel parses a log level string and returns the corresponding LogLevel value.
// It returns a PreExecutionError if the log level string is invalid.
func parseLogLevel(logLevelStr string, runID string) (runnertypes.LogLevel, error) {
	var level runnertypes.LogLevel
	if err := level.UnmarshalText([]byte(logLevelStr)); err != nil {
		return level, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Invalid log level %q: %v", logLevelStr, err),
			Component: string(resource.ComponentMain),
			RunID:     runID,
		}
	}
	return level, nil
}

func run(runID string) error {
	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize verification manager with secure default hash directory
	// For dry-run mode, skip hash directory validation since no actual file verification is needed
	var verificationManager *verification.Manager
	var err error
	if *dryRun {
		verificationManager, err = verification.NewManagerForDryRun()
	} else {
		verificationManager, err = verification.NewManager()
	}
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Verification manager initialization failed: %v", err),
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}

	// Load and prepare configuration (verify, parse, and expand variables)
	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, *configPath, runID)
	if err != nil {
		return err
	}

	// Handle validate command (after verification and loading)
	if *validateConfig {
		err := cli.ValidateConfigCommand(cfg)
		if err != nil {
			if errors.Is(err, cli.ErrConfigValidationFailed) {
				// Config validation failed - return silent exit error to avoid additional error messages
				return SilentExitError{}
			}
			return err
		}
		return nil
	}

	// Setup logging (using bootstrap package)
	// Parse log level string to LogLevel type
	logLevelValue, err := parseLogLevel(*logLevel, runID)
	if err != nil {
		return err
	}
	// Determine console output destination based on dry-run format
	// For JSON format, send logs to stderr to keep stdout clean for JSON output
	consoleWriter := os.Stdout
	if *dryRun && *dryRunFormat == "json" {
		consoleWriter = os.Stderr
	}
	if err := bootstrap.SetupLogging(logLevelValue, *logDir, runID, *forceInteractive, *forceQuiet, consoleWriter); err != nil {
		return err
	}

	// Expand global configuration
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Failed to expand global configuration: %v", err),
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}

	// Perform global file verification (using verification manager directly)
	result, err := verificationManager.VerifyGlobalFiles(runtimeGlobal)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Global files verification failed: %v", err),
			Component: string(resource.ComponentVerification),
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

	// Initialize and execute runner with all verified data
	return executeRunner(ctx, cfg, runtimeGlobal, verificationManager, runID)
}

// executeRunner initializes and executes the runner with proper cleanup
func executeRunner(ctx context.Context, cfg *runnertypes.ConfigSpec, runtimeGlobal *runnertypes.RuntimeGlobal, verificationManager *verification.Manager, runID string) error {
	// Initialize privilege manager
	logger := slog.Default()
	privMgr := privilege.NewManager(logger)

	// Initialize Runner with privilege support and run ID
	runnerOptions := []runner.Option{
		runner.WithVerificationManager(verificationManager),
		runner.WithPrivilegeManager(privMgr),
		runner.WithRunID(runID),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithKeepTempDirs(*keepTempDirs),
	}

	// Parse dry-run options once for the entire function
	var detailLevel resource.DryRunDetailLevel
	var outputFormat resource.OutputFormat

	// Add dry-run mode if requested
	if *dryRun {
		// Parse detail level
		var err error
		detailLevel, err = cli.ParseDryRunDetailLevel(*dryRunDetail)
		if err != nil {
			return fmt.Errorf("invalid detail level %q: %w", *dryRunDetail, err)
		}

		// Parse output format
		outputFormat, err = cli.ParseDryRunOutputFormat(*dryRunFormat)
		if err != nil {
			return fmt.Errorf("invalid output format %q: %w", *dryRunFormat, err)
		}

		dryRunOpts := &resource.DryRunOptions{
			DetailLevel:         detailLevel,
			OutputFormat:        outputFormat,
			ShowSensitive:       *showSensitive,
			VerifyFiles:         true,
			VerifyStandardPaths: runnertypes.DetermineVerifyStandardPaths(cfg.Global.VerifyStandardPaths), // Use new verify logic
			HashDir:             cmdcommon.DefaultHashDirectory,                                           // Use secure default hash directory
		}
		runnerOptions = append(runnerOptions, runner.WithDryRun(dryRunOpts))
	}

	r, err := runner.NewRunner(cfg, runnerOptions...)
	if err != nil {
		return fmt.Errorf("failed to initialize runner: %w", err)
	}

	// Load system environment variables
	if err := r.LoadSystemEnvironment(); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	if *logLevel != "" {
		level, err := parseLogLevel(*logLevel, runID)
		if err != nil {
			return err
		}
		cfg.Global.LogLevel = level
	}

	// Ensure cleanup of all resources on exit
	defer func() {
		if err := r.CleanupAllResources(); err != nil {
			slog.Warn("Failed to cleanup resources", slog.Any("error", err), slog.String("run_id", runID))
		}
	}()

	// Execute all groups (works for both normal and dry-run modes)
	execErr := r.ExecuteAll(ctx)

	// Handle dry-run output (always output, even on error)
	if *dryRun {
		// If an execution error occurred, set error status before getting results
		if execErr != nil {
			// Set execution error in the resource manager
			r.SetDryRunExecutionError(
				string(resource.ErrorTypeExecutionError),
				execErr.Error(),
				string(resource.ComponentRunner),
				nil,
				resource.PhaseGroupExecution,
			)
		}

		result := r.GetDryRunResults()
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
				ShowSensitive: *showSensitive,
			})
			if err != nil {
				return fmt.Errorf("formatting failed: %w", err)
			}
			fmt.Print(output)
		}
	}

	// Return execution error after outputting results (if any)
	if execErr != nil {
		// Extract group and command context from error chain if available
		var cmdExecErr *runner.CommandExecutionError
		var groupName, commandName string
		if errors.As(execErr, &cmdExecErr) {
			groupName = cmdExecErr.GroupName
			commandName = cmdExecErr.CommandName
		}

		return &logging.ExecutionError{
			Message:     "error running commands",
			Component:   "runner",
			RunID:       runID,
			GroupName:   groupName,
			CommandName: commandName,
			Err:         execErr,
		}
	}

	return nil
}
