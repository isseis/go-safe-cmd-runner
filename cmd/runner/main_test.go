package main

import (
	"errors"
	"flag"
	"os"
	"testing"
)

func TestVerifyConfigOption(t *testing.T) {
	// Save original command line arguments
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Reset flag package for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Re-initialize flags
	configPath = flag.String("config", "", "path to config file")
	envFile = flag.String("env-file", "", "path to environment file")
	logLevel = flag.String("log-level", "", "log level (debug, info, warn, error)")
	dryRun = flag.Bool("dry-run", false, "print commands without executing them")
	verifyConfig = flag.Bool("verify-config", false, "verify configuration file integrity (not implemented)")

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
	// Save original command line arguments
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Reset flag package for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	// Re-initialize flags
	configPath = flag.String("config", "", "path to config file")
	envFile = flag.String("env-file", "", "path to environment file")
	logLevel = flag.String("log-level", "", "log level (debug, info, warn, error)")
	dryRun = flag.Bool("dry-run", false, "print commands without executing them")
	verifyConfig = flag.Bool("verify-config", false, "verify configuration file integrity (not implemented)")

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
