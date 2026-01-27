package main

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetupLoggerWithConfig_IntegrationWithNewHandlers tests the integration
// of the new terminal-aware handlers with the existing logging system
func TestSetupLoggerWithConfig_IntegrationWithNewHandlers(t *testing.T) {
	testCases := []struct {
		name           string
		config         bootstrap.LoggerConfig
		envVars        map[string]string
		expectHandlers int
		expectError    bool
	}{
		{
			name: "interactive_environment_with_color_support",
			config: bootstrap.LoggerConfig{
				Level:                  slog.LevelInfo,
				LogDir:                 "",
				RunID:                  "test-run-001",
				SlackWebhookURLSuccess: "",
				SlackWebhookURLError:   "",
			},
			envVars: map[string]string{
				"TERM":     "xterm-256color",
				"NO_COLOR": "",
			},
			expectHandlers: 2, // Interactive + Conditional text handlers
			expectError:    false,
		},
		{
			name: "non_interactive_environment",
			config: bootstrap.LoggerConfig{
				Level:                  slog.LevelDebug,
				LogDir:                 "",
				RunID:                  "test-run-002",
				SlackWebhookURLSuccess: "",
				SlackWebhookURLError:   "",
			},
			envVars: map[string]string{
				"CI":       "true",
				"NO_COLOR": "1",
			},
			expectHandlers: 1, // Only conditional text handler
			expectError:    false,
		},
		{
			name: "full_handler_chain_with_log_and_slack",
			config: bootstrap.LoggerConfig{
				Level:                  slog.LevelWarn,
				LogDir:                 t.TempDir(),
				RunID:                  "test-run-003",
				SlackWebhookURLSuccess: "https://hooks.slack.com/test/webhook-success",
				SlackWebhookURLError:   "https://hooks.slack.com/test/webhook-error",
			},
			envVars: map[string]string{
				"TERM": "xterm",
			},
			expectHandlers: 5, // Interactive + Conditional text + JSON + 2x Slack
			expectError:    false,
		},
		{
			name: "invalid_log_directory",
			config: bootstrap.LoggerConfig{
				Level:                  slog.LevelInfo,
				LogDir:                 "/invalid/nonexistent/path",
				RunID:                  "test-run-004",
				SlackWebhookURLSuccess: "",
				SlackWebhookURLError:   "",
			},
			envVars:        map[string]string{},
			expectHandlers: 0,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment variables using t.Setenv for automatic cleanup
			for key, value := range tc.envVars {
				t.Setenv(key, value)
			}

			// Capture original logger to restore later
			originalLogger := slog.Default()
			defer slog.SetDefault(originalLogger)

			// Run the function under test
			err := bootstrap.SetupLoggerWithConfig(tc.config, false, false)

			// Check error expectation
			if tc.expectError {
				assert.Error(t, err, "Expected error for invalid log directory")
				return
			}

			assert.NoError(t, err, "SetupLoggerWithConfig should not return error")

			// Verify logger was set up (basic smoke test)
			logger := slog.Default()
			assert.NotNil(t, logger, "Logger should not be nil after setup")

			// Test that logging works without panics
			logger.Info("Integration test log message",
				"test_case", tc.name,
				"run_id", tc.config.RunID,
			)

			logger.Error("Integration test error message",
				"test_case", tc.name,
				"component", "integration_test",
			)
		})
	}
}

// TestTerminalCapabilitiesIntegration tests that terminal capabilities
// are properly integrated with the logging system
func TestTerminalCapabilitiesIntegration(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping terminal capabilities test in CI environment (no tty support)")
	}
	testCases := []struct {
		name              string
		envVars           map[string]string
		expectColor       bool
		expectInteractive bool
	}{
		{
			name: "forced_color_mode",
			envVars: map[string]string{
				"CLICOLOR_FORCE": "1",
				"CI":             "true", // Would normally disable interactive
			},
			expectColor:       true,
			expectInteractive: false, // CI environment still not interactive
		},
		{
			name: "disabled_color_mode",
			envVars: map[string]string{
				"NO_COLOR": "1",
				"TERM":     "xterm-256color",
			},
			expectColor:       false,
			expectInteractive: true, // TERM is set, so considered interactive
		},
		{
			name: "auto_detection",
			envVars: map[string]string{
				"TERM": "xterm-256color",
			},
			expectColor:       true, // TERM indicates color support
			expectInteractive: true, // TERM indicates interactive environment
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment variables using t.Setenv for automatic cleanup
			for key, value := range tc.envVars {
				if value == "" {
					// For empty values, we want to unset the variable
					// but t.Setenv doesn't support unsetting, so we use a special marker
					// that our code will treat as unset
					t.Setenv(key, "")
				} else {
					t.Setenv(key, value)
				}
			}

			// Test terminal capabilities
			capabilities := terminal.NewCapabilities(terminal.Options{})

			assert.Equal(t, tc.expectColor, capabilities.SupportsColor(),
				"Color support mismatch for test case: %s", tc.name)

			assert.Equal(t, tc.expectInteractive, capabilities.IsInteractive(),
				"Interactive mode mismatch for test case: %s", tc.name)
		})
	}
}

// TestHandlerChainIntegration tests that messages flow correctly through
// the complete handler chain
func TestHandlerChainIntegration(t *testing.T) {
	// Create temporary directory for log files
	logDir := t.TempDir()

	config := bootstrap.LoggerConfig{
		Level:                  slog.LevelDebug,
		LogDir:                 logDir,
		RunID:                  "integration-test-run",
		SlackWebhookURLSuccess: "", // Skip Slack for this test
		SlackWebhookURLError:   "",
	}

	// Capture original logger and streams
	originalLogger := slog.Default()
	originalStderr := os.Stderr
	originalStdout := os.Stdout

	// Restore after test
	defer func() {
		slog.SetDefault(originalLogger)
		os.Stderr = originalStderr
		os.Stdout = originalStdout
	}()

	// Set environment for testing using t.Setenv for automatic cleanup
	t.Setenv("TERM", "dumb") // Disable interactive features

	// Note: We can't easily redirect stderr/stdout for this test since
	// the handlers are created internally, but we can test basic setup

	// Test setup
	err := bootstrap.SetupLoggerWithConfig(config, false, false)
	require.NoError(t, err, "SetupLoggerWithConfig should not return error")

	// Test logging through the chain
	logger := slog.Default()

	// Test different log levels
	logger.Debug("Debug message for integration test")
	logger.Info("Info message for integration test")
	logger.Warn("Warning message for integration test")
	logger.Error("Error message for integration test")

	// Test with attributes
	logger.With("component", "integration_test", "run_id", config.RunID).
		Info("Message with attributes")

	// Test with groups
	logger.WithGroup("test_group").
		With("nested_attr", "value").
		Info("Message with groups and attributes")

	// Verify log file was created (if logDir was specified)
	if config.LogDir != "" {
		entries, err := os.ReadDir(config.LogDir)
		require.NoError(t, err, "Failed to read log directory")

		assert.NotEmpty(t, entries, "Expected log file to be created, but directory is empty")

		// Check if log file contains our run ID
		for _, entry := range entries {
			if strings.Contains(entry.Name(), config.RunID) {
				// Found our log file
				return
			}
		}
		assert.Fail(t, "Expected to find log file with run ID, but none found")
	}
}

// TestErrorHandling tests error handling in the integrated system
func TestErrorHandling(t *testing.T) {
	testCases := []struct {
		name        string
		config      bootstrap.LoggerConfig
		expectError bool
		errorType   string
	}{
		{
			name: "invalid_slack_webhook_success",
			config: bootstrap.LoggerConfig{
				Level:                  slog.LevelInfo,
				LogDir:                 "",
				RunID:                  "test-error-002",
				SlackWebhookURLSuccess: "not-a-valid-url",
				SlackWebhookURLError:   "https://hooks.slack.com/valid",
			},
			expectError: true,
			errorType:   "slack handler creation",
		},
		{
			name: "nonexistent_log_directory",
			config: bootstrap.LoggerConfig{
				Level:                  slog.LevelInfo,
				LogDir:                 "/path/does/not/exist",
				RunID:                  "test-error-003",
				SlackWebhookURLSuccess: "",
				SlackWebhookURLError:   "",
			},
			expectError: true,
			errorType:   "log directory validation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture original logger to restore later
			originalLogger := slog.Default()
			defer slog.SetDefault(originalLogger)

			err := bootstrap.SetupLoggerWithConfig(tc.config, false, false)

			if tc.expectError {
				assert.Error(t, err, "Expected error (%s) but got none", tc.errorType)
			} else {
				assert.NoError(t, err, "Unexpected error")
			}
		})
	}
}
