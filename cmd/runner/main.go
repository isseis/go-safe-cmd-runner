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
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/cli"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/hashdir"
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
	validateConfig   = flag.Bool("validate", false, "validate configuration file and exit")
	runID            = flag.String("run-id", "", "unique identifier for this execution run (auto-generates ULID if not provided)")
	forceInteractive = flag.Bool("interactive", false, "force interactive mode with colored output (overrides environment detection)")
	forceQuiet       = flag.Bool("quiet", false, "force non-interactive mode (disables colored output)")
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
	if err := hashdir.ValidateDefaultHashDirectory(cmdcommon.DefaultHashDirectory); err != nil {
		logging.HandlePreExecutionError(logging.ErrorTypeBuildConfig, fmt.Sprintf("Invalid default hash directory: %v", err), "main", *runID)
		os.Exit(1)
	}

	if err := syscall.Seteuid(syscall.Getuid()); err != nil {
		logging.HandlePreExecutionError(logging.ErrorTypePrivilegeDrop, fmt.Sprintf("Failed to drop privileges: %v", err), "main", *runID)
		os.Exit(1)
	}

	// Wrap main logic in a separate function to properly handle errors and defer
	if err := run(*runID); err != nil {
		var silentErr SilentExitError
		var preExecErr *logging.PreExecutionError
		switch {
		case errors.As(err, &silentErr):
			// Check for silent exit error first (validation failure with report already printed)
			// revive:disable:empty-block This empty block is intentional to handle specific cases
		case errors.As(err, &preExecErr):
			// Check if this is a pre-execution error using errors.As for safe type checking
			logging.HandlePreExecutionError(preExecErr.Type, preExecErr.Message, preExecErr.Component, *runID)
		default:
			logging.HandlePreExecutionError(logging.ErrorTypeSystemError, err.Error(), "main", *runID)
		}
		os.Exit(1)
	}
}

func run(runID string) error {
	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Phase 1: Initialize verification manager with secure default hash directory
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
			Component: "verification",
			RunID:     runID,
		}
	}

	// Phase 3: Verify and load configuration atomically (to prevent TOCTOU attacks)
	cfg, err := bootstrap.LoadConfig(verificationManager, *configPath, runID)
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

	// Phase 4: Setup logging (using bootstrap package)
	if err := bootstrap.SetupLogging(*logLevel, *logDir, runID, *forceInteractive, *forceQuiet); err != nil {
		return err
	}

	// Phase 5: Perform global file verification (using verification manager directly)
	result, err := verificationManager.VerifyGlobalFiles(&cfg.Global)
	if err != nil {
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

	// Phase 6: Initialize and execute runner with all verified data
	return executeRunner(ctx, cfg, verificationManager, runID)
}

// executeRunner initializes and executes the runner with proper cleanup
func executeRunner(ctx context.Context, cfg *runnertypes.Config, verificationManager *verification.Manager, runID string) error {
	// Initialize privilege manager
	logger := slog.Default()
	privMgr := privilege.NewManager(logger)

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
			DetailLevel:       detailLevel,
			OutputFormat:      outputFormat,
			ShowSensitive:     false,
			VerifyFiles:       true,
			SkipStandardPaths: cfg.Global.SkipStandardPaths,   // Use setting from TOML config
			HashDir:           cmdcommon.DefaultHashDirectory, // Use secure default hash directory
		}
		runnerOptions = append(runnerOptions, runner.WithDryRun(dryRunOpts))
	}

	runner, err := runner.NewRunner(cfg, runnerOptions...)
	if err != nil {
		return fmt.Errorf("failed to initialize runner: %w", err)
	}

	// Load system environment variables
	if err := runner.LoadSystemEnvironment(); err != nil {
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
