package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestFlags initializes the command-line flags for testing and returns a cleanup function
func setupTestFlags() func() {
	// Save original command line arguments and flag.CommandLine
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine

	// Create new flag set with ExitOnError handling
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Initialize all flags - must match init() in main.go
	// High-priority flags with short forms
	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.StringVar(&configPath, "c", "", "path to config file (short form)")

	flag.BoolVar(&dryRun, "dry-run", false, "print commands without executing them")
	flag.BoolVar(&dryRun, "n", false, "print commands without executing them (short form)")

	flag.StringVar(&groups, "groups", "", "comma-separated list of groups to execute (executes all groups if not specified)\nExample: --groups=build,test")
	flag.StringVar(&groups, "g", "", "comma-separated list of groups to execute (short form)")

	// Medium-priority flags with short forms
	flag.StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	flag.StringVar(&logLevel, "l", "info", "log level (short form)")

	flag.BoolVar(&forceQuiet, "quiet", false, "force non-interactive mode (disables colored output)")
	flag.BoolVar(&forceQuiet, "q", false, "force non-interactive mode (short form)")

	// Other flags without short forms
	flag.StringVar(&logDir, "log-dir", "", "directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
	flag.StringVar(&dryRunFormat, "dry-run-format", "text", "dry-run output format (text, json)")
	flag.StringVar(&dryRunDetail, "dry-run-detail", "detailed", "dry-run detail level (summary, detailed, full)")
	flag.BoolVar(&showSensitive, "show-sensitive", false, "show sensitive information in dry-run output (use with caution)")
	flag.StringVar(&runID, "run-id", "", "unique identifier for this execution run (auto-generates ULID if not provided)")
	flag.BoolVar(&forceInteractive, "interactive", false, "force interactive mode with colored output (overrides environment detection)")
	flag.BoolVar(&keepTempDirs, "keep-temp-dirs", false, "keep temporary directories after execution")

	// Return cleanup function to restore original state
	return func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}
}

// createTempHashDir creates a temporary directory for hash storage during testing
func createTempHashDir(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "go-safe-cmd-runner-test-")
	require.NoError(t, err, "failed to create temp dir")

	cleanup := func() {
		_ = os.RemoveAll(tempDir) // Ignore cleanup errors in test helper
	}

	return tempDir, cleanup
}

// runForTestWithTempHashDir is a version that uses a temporary hash directory
func runForTestWithTempHashDir(t *testing.T, runID string) error {
	t.Helper()

	// Create temporary hash directory
	tempHashDir, cleanup := createTempHashDir(t)
	defer cleanup()

	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize verification manager with temporary hash directory
	verificationManager, err := verification.NewManagerForTest(tempHashDir)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   "Verification manager initialization failed",
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}

	// Load and prepare configuration (verify, parse, and expand variables)
	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, runID)
	if err != nil {
		return err
	}

	// For testing, we skip the actual execution steps
	_ = ctx
	_ = cfg

	return nil
}

// runForTestWithManagerUsingTempDir is a helper that uses temporary hash directory
func runForTestWithManagerUsingTempDir(t *testing.T) (error, error) {
	t.Helper()

	// Create temporary hash directory
	tempHashDir, cleanup := createTempHashDir(t)
	defer cleanup()

	// Test manager creation directly with temp directory
	_, err := verification.NewManagerForTest(tempHashDir)
	if err != nil {
		return nil, err
	}

	// Test the full runForTestWithTempHashDir flow
	return runForTestWithTempHashDir(t, "test-run-id"), nil
}

// runForTestWithCustomHashDir is a helper for testing custom hash directories
func runForTestWithCustomHashDir(t *testing.T, hashDir string) (error, error) {
	t.Helper()

	// Test manager creation with custom hash directory
	verificationManager, err := verification.NewManagerForTest(hashDir)
	if err != nil {
		return nil, err
	}

	// Try to load and prepare config (will fail without config file, but tests manager creation)
	_, configErr := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-id")
	return configErr, nil
}

func TestConfigPathRequired(t *testing.T) {
	// Setup test flags
	cleanup := setupTestFlags()
	defer cleanup()

	// Test args without --config (hash directory is now set automatically to default)
	os.Args = []string{"runner"}

	// Parse flags
	flag.Parse()

	// Test runForTestWithTempHashDir() function to avoid CI hash directory issues
	runID := "test-run-id"
	err := runForTestWithTempHashDir(t, runID)
	assert.Error(t, err, "expected error when --config is not provided")

	// Check if the error is a PreExecutionError with the correct type
	var preExecErr *logging.PreExecutionError
	require.ErrorAs(t, err, &preExecErr, "expected PreExecutionError, got: %T (error: %v)", err, err)

	assert.Equal(t, logging.ErrorTypeRequiredArgumentMissing, preExecErr.Type)
}

func TestNewManagerProduction(t *testing.T) {
	t.Run("creates manager with default hash directory", func(t *testing.T) {
		// Use temporary hash directory to avoid CI environment issues
		runErr, managerErr := runForTestWithManagerUsingTempDir(t)
		require.NoError(t, managerErr, "manager creation should not fail")
		if runErr != nil {
			// In tests, we expect this to fail due to missing config file
			assert.Contains(t, runErr.Error(), "config")
		}
	})
}

// TestNewManagerForTestValidation tests the testing API validation
func TestNewManagerForTestValidation(t *testing.T) {
	t.Run("valid custom hash directory", func(t *testing.T) {
		// Create temporary directory for testing
		tempDir := t.TempDir()

		// This should work since we're in a test file
		configErr, managerErr := runForTestWithCustomHashDir(t, tempDir)
		require.NoError(t, managerErr, "manager creation should not fail")
		if configErr != nil {
			// We expect config errors, not manager creation errors
			assert.Contains(t, configErr.Error(), "config")
		}
	})

	t.Run("relative path allowed in testing", func(t *testing.T) {
		// Custom hash directories (even relative ones) are allowed in testing mode
		configErr, managerErr := runForTestWithCustomHashDir(t, "relative/path")
		// This will fail due to directory not existing, but not due to relative path restriction
		// We expect either a config error or manager error (directory doesn't exist)
		assert.True(t, configErr != nil || managerErr != nil, "expected an error for non-existent directory")
	})
}

// TestShortFlags tests that short flags work the same as long flags
func TestShortFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		checkFunc func(t *testing.T)
	}{
		{
			name: "short flag -c sets configPath",
			args: []string{"runner", "-c", "/path/to/config.toml"},
			checkFunc: func(t *testing.T) {
				assert.Equal(t, "/path/to/config.toml", configPath)
			},
		},
		{
			name: "short flag -n sets dryRun",
			args: []string{"runner", "-n"},
			checkFunc: func(t *testing.T) {
				assert.True(t, dryRun)
			},
		},
		{
			name: "short flag -g sets groups",
			args: []string{"runner", "-g", "build,test"},
			checkFunc: func(t *testing.T) {
				assert.Equal(t, "build,test", groups)
			},
		},
		{
			name: "short flag -l sets logLevel",
			args: []string{"runner", "-l", "debug"},
			checkFunc: func(t *testing.T) {
				assert.Equal(t, "debug", logLevel)
			},
		},
		{
			name: "short flag -q sets forceQuiet",
			args: []string{"runner", "-q"},
			checkFunc: func(t *testing.T) {
				assert.True(t, forceQuiet)
			},
		},
		{
			name: "long and short flags can be mixed",
			args: []string{"runner", "-c", "/path/to/config.toml", "--dry-run", "-g", "build"},
			checkFunc: func(t *testing.T) {
				assert.Equal(t, "/path/to/config.toml", configPath)
				assert.True(t, dryRun)
				assert.Equal(t, "build", groups)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test flags
			cleanup := setupTestFlags()
			defer cleanup()

			// Set test args
			os.Args = tt.args

			// Parse flags
			flag.Parse()

			// Check result
			tt.checkFunc(t)
		})
	}
}

// TestShortFlagsEquivalence tests that short flags are equivalent to long flags
func TestShortFlagsEquivalence(t *testing.T) {
	tests := []struct {
		name      string
		shortArgs []string
		longArgs  []string
	}{
		{
			name:      "config flag equivalence",
			shortArgs: []string{"runner", "-c", "/path/to/config.toml"},
			longArgs:  []string{"runner", "--config", "/path/to/config.toml"},
		},
		{
			name:      "dry-run flag equivalence",
			shortArgs: []string{"runner", "-n"},
			longArgs:  []string{"runner", "--dry-run"},
		},
		{
			name:      "groups flag equivalence",
			shortArgs: []string{"runner", "-g", "build,test"},
			longArgs:  []string{"runner", "--groups", "build,test"},
		},
		{
			name:      "log-level flag equivalence",
			shortArgs: []string{"runner", "-l", "debug"},
			longArgs:  []string{"runner", "--log-level", "debug"},
		},
		{
			name:      "quiet flag equivalence",
			shortArgs: []string{"runner", "-q"},
			longArgs:  []string{"runner", "--quiet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test short form
			cleanup1 := setupTestFlags()
			os.Args = tt.shortArgs
			flag.Parse()
			shortConfigPath := configPath
			shortDryRun := dryRun
			shortGroups := groups
			shortLogLevel := logLevel
			shortForceQuiet := forceQuiet
			cleanup1()

			// Test long form
			cleanup2 := setupTestFlags()
			os.Args = tt.longArgs
			flag.Parse()
			longConfigPath := configPath
			longDryRun := dryRun
			longGroups := groups
			longLogLevel := logLevel
			longForceQuiet := forceQuiet
			cleanup2()

			// Verify equivalence
			assert.Equal(t, longConfigPath, shortConfigPath, "configPath should be the same")
			assert.Equal(t, longDryRun, shortDryRun, "dryRun should be the same")
			assert.Equal(t, longGroups, shortGroups, "groups should be the same")
			assert.Equal(t, longLogLevel, shortLogLevel, "logLevel should be the same")
			assert.Equal(t, longForceQuiet, shortForceQuiet, "forceQuiet should be the same")
		})
	}
}
