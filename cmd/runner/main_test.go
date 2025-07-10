package main

import (
	"errors"
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupTestFlags initializes the command-line flags for testing and returns a cleanup function
func setupTestFlags() func() {
	// Save original command line arguments and flag.CommandLine
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine

	// Create new flag set with ExitOnError handling
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Initialize all flags
	configPath = flag.String("config", "", "path to config file")
	envFile = flag.String("env-file", "", "path to environment file")
	logLevel = flag.String("log-level", "", "log level (debug, info, warn, error)")
	dryRun = flag.Bool("dry-run", false, "print commands without executing them")
	disableVerification = flag.Bool("disable-verification", false, "disable configuration file verification")
	hashDirectory = flag.String("hash-directory", DefaultHashDirectory, "directory containing hash files")

	// Return cleanup function to restore original state
	return func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}
}

func TestConfigPathRequired(t *testing.T) {
	// Setup test flags
	cleanup := setupTestFlags()
	defer cleanup()

	// Test args without --config
	os.Args = []string{"runner"}

	// Test run() function
	err := run()
	if err == nil {
		t.Error("expected error when --config is not provided")
	}

	if !errors.Is(err, ErrConfigPathRequired) {
		t.Errorf("expected ErrConfigPathRequired, got: %v", err)
	}
}

func TestGetVerificationConfig(t *testing.T) {
	// Clear environment variables at start
	oldEnvDisable := os.Getenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION")
	oldEnvHashDir := os.Getenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")
	defer func() {
		os.Setenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION", oldEnvDisable)
		os.Setenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY", oldEnvHashDir)
	}()

	t.Run("default configuration", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		// Clear environment variables
		os.Unsetenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION")
		os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		// Reset flags to defaults
		os.Args = []string{"runner"}
		flag.Parse()

		config := getVerificationConfig()
		assert.True(t, config.Enabled, "verification should be enabled by default")
		assert.Equal(t, DefaultHashDirectory, config.HashDirectory)
	})

	t.Run("disabled via command line", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		// Clear environment variables
		os.Unsetenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION")
		os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		os.Args = []string{"runner", "--disable-verification"}
		flag.Parse()

		config := getVerificationConfig()
		assert.False(t, config.Enabled, "verification should be disabled via command line")
		assert.Equal(t, DefaultHashDirectory, config.HashDirectory)
	})

	t.Run("disabled via environment variable", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Setenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION", "true")
		defer os.Unsetenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION")

		os.Args = []string{"runner"}
		flag.Parse()

		config := getVerificationConfig()
		assert.False(t, config.Enabled, "verification should be disabled via environment variable")
	})

	t.Run("custom hash directory via command line", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		// Clear environment variables
		os.Unsetenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION")
		os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		os.Args = []string{"runner", "--hash-directory", "/custom/path"}
		flag.Parse()

		config := getVerificationConfig()
		assert.True(t, config.Enabled)
		assert.Equal(t, "/custom/path", config.HashDirectory)
	})

	t.Run("custom hash directory via environment variable", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Setenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY", "/env/path")
		defer os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		os.Args = []string{"runner"}
		flag.Parse()

		config := getVerificationConfig()
		assert.Equal(t, "/env/path", config.HashDirectory)
	})

	t.Run("command line takes precedence over environment", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Setenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION", "false")
		defer os.Unsetenv("GO_SAFE_CMD_RUNNER_DISABLE_VERIFICATION")

		os.Args = []string{"runner", "--disable-verification"}
		flag.Parse()

		config := getVerificationConfig()
		assert.False(t, config.Enabled, "command line should take precedence over environment variable")
	})
}
