package main

import (
	"errors"
	"flag"
	"os"
	"testing"
)

// setupTestFlags initializes the command-line flags for testing and returns a cleanup function
func setupTestFlags(errorHandling flag.ErrorHandling) func() {
	// Save original command line arguments and flag.CommandLine
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine

	// Create new flag set with the specified error handling
	flag.CommandLine = flag.NewFlagSet(os.Args[0], errorHandling)

	// Initialize all flags
	configPath = flag.String("config", "", "path to config file")
	envFile = flag.String("env-file", "", "path to environment file")
	logLevel = flag.String("log-level", "", "log level (debug, info, warn, error)")
	dryRun = flag.Bool("dry-run", false, "print commands without executing them")
	verifyConfig = flag.Bool("verify-config", false, "verify configuration file integrity (not implemented)")

	// Return cleanup function to restore original state
	return func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}
}

func TestVerifyConfigOption(t *testing.T) {
	// Setup test flags with ExitOnError handling
	cleanup := setupTestFlags(flag.ExitOnError)
	defer cleanup()

	// Test args with --verify-config
	os.Args = []string{"runner", "--verify-config"}

	// Test run() function
	err := run()
	if err == nil {
		t.Error("expected error when --verify-config is used")
	}

	if !errors.Is(err, ErrConfigVerificationNotImplemented) {
		t.Errorf("expected ErrConfigVerificationNotImplemented, got: %v", err)
	}

	// Verify the flag was parsed correctly
	if !*verifyConfig {
		t.Error("expected verifyConfig flag to be true")
	}
}

func TestVerifyConfigOptionHelp(t *testing.T) {
	// Setup test flags with ContinueOnError handling
	cleanup := setupTestFlags(flag.ContinueOnError)
	defer cleanup()

	// Test that the help text contains the expected description
	found := false
	flag.VisitAll(func(f *flag.Flag) {
		if f.Name == "verify-config" {
			expectedUsage := "verify configuration file integrity (not implemented)"
			if f.Usage == expectedUsage {
				found = true
			}
		}
	})

	if !found {
		t.Error("verify-config flag not found or has incorrect usage text")
	}
}
