package main

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/terminal"
)

// TestSetupLoggerWithConfig_IntegrationWithNewHandlers tests the integration
// of the new terminal-aware handlers with the existing logging system
func TestSetupLoggerWithConfig_IntegrationWithNewHandlers(t *testing.T) {
	testCases := []struct {
		name           string
		config         LoggerConfig
		envVars        map[string]string
		expectHandlers int
		expectError    bool
	}{
		{
			name: "interactive_environment_with_color_support",
			config: LoggerConfig{
				Level:           "info",
				LogDir:          "",
				RunID:           "test-run-001",
				SlackWebhookURL: "",
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
			config: LoggerConfig{
				Level:           "debug",
				LogDir:          "",
				RunID:           "test-run-002",
				SlackWebhookURL: "",
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
			config: LoggerConfig{
				Level:           "warn",
				LogDir:          t.TempDir(),
				RunID:           "test-run-003",
				SlackWebhookURL: "https://hooks.slack.com/test/webhook",
			},
			envVars: map[string]string{
				"TERM": "xterm",
			},
			expectHandlers: 4, // Interactive + Conditional text + JSON + Slack
			expectError:    false,
		},
		{
			name: "invalid_log_directory",
			config: LoggerConfig{
				Level:           "info",
				LogDir:          "/invalid/nonexistent/path",
				RunID:           "test-run-004",
				SlackWebhookURL: "",
			},
			envVars:        map[string]string{},
			expectHandlers: 0,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment variables for test
			originalEnv := make(map[string]string)
			for key, value := range tc.envVars {
				originalEnv[key] = os.Getenv(key)
				if value == "" {
					os.Unsetenv(key)
				} else {
					os.Setenv(key, value)
				}
			}

			// Clean up environment after test
			defer func() {
				for key, originalValue := range originalEnv {
					if originalValue == "" {
						os.Unsetenv(key)
					} else {
						os.Setenv(key, originalValue)
					}
				}
			}()

			// Capture original logger to restore later
			originalLogger := slog.Default()
			defer slog.SetDefault(originalLogger)

			// Run the function under test
			err := setupLoggerWithConfig(tc.config)

			// Check error expectation
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify logger was set up (basic smoke test)
			logger := slog.Default()
			if logger == nil {
				t.Error("Logger should not be nil after setup")
			}

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
			// Set up environment variables
			originalEnv := make(map[string]string)
			for key, value := range tc.envVars {
				originalEnv[key] = os.Getenv(key)
				if value == "" {
					os.Unsetenv(key)
				} else {
					os.Setenv(key, value)
				}
			}

			defer func() {
				for key, originalValue := range originalEnv {
					if originalValue == "" {
						os.Unsetenv(key)
					} else {
						os.Setenv(key, originalValue)
					}
				}
			}()

			// Test terminal capabilities
			capabilities := terminal.NewCapabilities(terminal.Options{})

			if capabilities.SupportsColor() != tc.expectColor {
				t.Errorf("Color support mismatch: expected %v, got %v",
					tc.expectColor, capabilities.SupportsColor())
			}

			if capabilities.IsInteractive() != tc.expectInteractive {
				t.Errorf("Interactive mode mismatch: expected %v, got %v",
					tc.expectInteractive, capabilities.IsInteractive())
			}
		})
	}
}

// TestHandlerChainIntegration tests that messages flow correctly through
// the complete handler chain
func TestHandlerChainIntegration(t *testing.T) {
	// Create temporary directory for log files
	logDir := t.TempDir()

	config := LoggerConfig{
		Level:           "debug",
		LogDir:          logDir,
		RunID:           "integration-test-run",
		SlackWebhookURL: "", // Skip Slack for this test
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

	// Set environment for testing
	os.Setenv("TERM", "dumb") // Disable interactive features
	defer os.Unsetenv("TERM")

	// Note: We can't easily redirect stderr/stdout for this test since
	// the handlers are created internally, but we can test basic setup

	// Test setup
	err := setupLoggerWithConfig(config)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

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
		if err != nil {
			t.Fatalf("Failed to read log directory: %v", err)
		}

		if len(entries) == 0 {
			t.Error("Expected log file to be created, but directory is empty")
		}

		// Check if log file contains our run ID
		for _, entry := range entries {
			if strings.Contains(entry.Name(), config.RunID) {
				// Found our log file
				return
			}
		}
		t.Error("Expected to find log file with run ID, but none found")
	}
}

// TestErrorHandling tests error handling in the integrated system
func TestErrorHandling(t *testing.T) {
	testCases := []struct {
		name        string
		config      LoggerConfig
		expectError bool
		errorType   string
	}{
		{
			name: "invalid_log_level",
			config: LoggerConfig{
				Level:           "invalid-level",
				LogDir:          "",
				RunID:           "test-error-001",
				SlackWebhookURL: "",
			},
			expectError: false, // Should default to INFO and continue
			errorType:   "",
		},
		{
			name: "invalid_slack_webhook",
			config: LoggerConfig{
				Level:           "info",
				LogDir:          "",
				RunID:           "test-error-002",
				SlackWebhookURL: "not-a-valid-url",
			},
			expectError: true,
			errorType:   "slack handler creation",
		},
		{
			name: "nonexistent_log_directory",
			config: LoggerConfig{
				Level:           "info",
				LogDir:          "/path/does/not/exist",
				RunID:           "test-error-003",
				SlackWebhookURL: "",
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

			err := setupLoggerWithConfig(tc.config)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error (%s) but got none", tc.errorType)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}
