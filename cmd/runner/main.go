// Package main provides the entry point for the command runner application.
// It handles command-line arguments, configuration loading, and orchestrates
// the execution of commands based on the provided configuration.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// Error definitions
var (
	ErrConfigPathRequired = errors.New("config file path is required")
)

var (
	configPath     = flag.String("config", "", "path to config file")
	envFile        = flag.String("env-file", "", "path to environment file")
	logLevel       = flag.String("log-level", "", "log level (debug, info, warn, error)")
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
	// Wrap main logic in a separate function to properly handle errors and defer
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	flag.Parse()

	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Load configuration
	if *configPath == "" {
		return ErrConfigPathRequired
	}

	// Initialize config loader
	cfgLoader := config.NewLoader()
	cfg, err := cfgLoader.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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
		return fmt.Errorf("config verification failed: %w", err)
	}

	// Verify global files - CRITICAL: Program must exit if global verification fails
	// to prevent execution with potentially compromised files
	result, err := verificationManager.VerifyGlobalFiles(&cfg.Global)
	if err != nil {
		log.Printf("CRITICAL: Global file verification failed - terminating program for security")
		return fmt.Errorf("global files verification failed: %w", err)
	}

	// Log global verification results
	if result.TotalFiles > 0 {
		log.Printf("Global files verification completed successfully: %d verified, %d skipped, duration: %v",
			result.VerifiedFiles, len(result.SkippedFiles), result.Duration)
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
			log.Printf("Warning: Failed to cleanup resources: %v", err)
		}
	}()

	if err := runner.ExecuteAll(ctx); err != nil {
		return fmt.Errorf("error running commands: %w", err)
	}
	return nil
}
