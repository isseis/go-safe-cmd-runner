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
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
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

	// Initialize Runner
	runner := runner.NewRunner(cfg)

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

	// Initialize executor
	exec := executor.NewDefaultExecutor()

	// Run the command groups
	if err := runGroups(ctx, cfg, exec, *dryRun); err != nil {
		return fmt.Errorf("error running commands: %w", err)
	}
	return nil
}

func runGroups(ctx context.Context, cfg *runnertypes.Config, exec executor.CommandExecutor, dryRun bool) error {
	// TODO: Implement group execution with dependencies
	// For now, just run all commands in all groups in the order they appear in the config
	for _, group := range cfg.Groups {
		log.Printf("Running group: %s", group.Name)
		for _, cmd := range group.Commands {
			if dryRun {
				log.Printf("[DRY RUN] Would run: %s %v", cmd.Cmd, cmd.Args)
				continue
			}

			// TODO: Implement proper environment variable handling
			env := make(map[string]string)
			_, err := exec.Execute(ctx, cmd, env)
			if err != nil {
				return fmt.Errorf("command %q failed: %w", cmd.Name, err)
			}
		}
	}
	return nil
}
