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
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// Build-time variables (set via ldflags)
var (
	// DefaultHashDirectory is set at build time via ldflags
	DefaultHashDirectory = "/usr/local/etc/go-safe-cmd-runner/hashes" // fallback default
)

// Error definitions
var (
	ErrConfigPathRequired = errors.New("config file path is required")
)

var (
	configPath          = flag.String("config", "", "path to config file")
	envFile             = flag.String("env-file", "", "path to environment file")
	logLevel            = flag.String("log-level", "", "log level (debug, info, warn, error)")
	dryRun              = flag.Bool("dry-run", false, "print commands without executing them")
	disableVerification = flag.Bool("disable-verification", false, "disable configuration file verification")
	hashDirectory       = flag.String("hash-directory", DefaultHashDirectory, "directory containing hash files")
)

// getVerificationConfig determines the verification settings based on command line args and environment variables
func getVerificationConfig() verification.Config {
	// Start with default: verification enabled
	enabled := true
	hashDir := *hashDirectory

	// Check environment variable for disabling verification
	if envDisable := os.Getenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION"); envDisable != "" {
		if parsedDisable, err := strconv.ParseBool(envDisable); err == nil && parsedDisable {
			enabled = false
		}
	}

	// Check environment variable for hash directory override
	if envHashDir := os.Getenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY"); envHashDir != "" {
		hashDir = envHashDir
	}

	// Command line arguments take precedence over environment variables
	if *disableVerification {
		enabled = false
	}

	return verification.Config{
		Enabled:       enabled,
		HashDirectory: hashDir,
	}
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

	// Get verification configuration from command line args and environment variables
	verificationConfig := getVerificationConfig()

	// Initialize verification manager
	verificationManager, err := verification.NewManager(verificationConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize verification: %w", err)
	}

	// Verify configuration file integrity
	if err := verificationManager.VerifyConfigFile(*configPath); err != nil {
		return fmt.Errorf("config verification failed: %w", err)
	}

	// Verify global files
	result, err := verificationManager.VerifyGlobalFiles(&cfg.Global)
	if err != nil {
		return fmt.Errorf("global files verification failed: %w", err)
	}

	// Log global verification results
	if result.TotalFiles > 0 {
		log.Printf("Global files verification completed: %d verified, %d skipped, duration: %v",
			result.VerifiedFiles, len(result.SkippedFiles), result.Duration)
	}

	// Initialize Runner with template engine from config loader
	runner, err := runner.NewRunnerWithComponents(cfg, cfgLoader.GetTemplateEngine(), verificationManager)
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

	// Ensure cleanup of resources on exit
	defer func() {
		if err := runner.CleanupAutoCleanupResources(); err != nil {
			log.Printf("Warning: Failed to cleanup resources: %v", err)
		}
	}()

	if err := runner.ExecuteAll(ctx); err != nil {
		return fmt.Errorf("error running commands: %w", err)
	}
	return nil
}
