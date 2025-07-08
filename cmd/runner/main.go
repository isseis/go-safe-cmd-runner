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
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
)

// Error definitions
var (
	ErrConfigPathRequired = errors.New("config file path is required")
)

var (
	configPath = flag.String("config", "", "path to config file")
	envFile    = flag.String("env-file", "", "path to environment file")
	logLevel   = flag.String("log-level", "", "log level (debug, info, warn, error)")
	dryRun     = flag.Bool("dry-run", false, "print commands without executing them")
)

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
		return fmt.Errorf("%w", ErrConfigPathRequired)
	}

	// Initialize config loader
	cfgLoader := config.NewLoader()
	cfg, err := cfgLoader.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize Runner with template engine from config loader
	runner, err := runner.NewRunnerWithComponents(cfg, cfgLoader.GetTemplateEngine(), nil)
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
