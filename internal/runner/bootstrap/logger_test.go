package bootstrap

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveAndRestoreGlobals registers a t.Cleanup that restores the package-level logger
// globals and the default slog logger.
// Call this at the start of any test that invokes SetupLoggerWithConfig.
func saveAndRestoreGlobals(t *testing.T) {
	t.Helper()
	origLogger := slog.Default()
	origHandlers := phase1BaseHandlers
	origFailureLogger := phase1FailureLogger
	origRedactionErrorCollector := redactionErrorCollector
	origRedactionReporter := redactionReporter
	origNewSlackHandlerFunc := newSlackHandlerFunc
	t.Cleanup(func() {
		slog.SetDefault(origLogger)
		phase1BaseHandlers = origHandlers
		phase1FailureLogger = origFailureLogger
		redactionErrorCollector = origRedactionErrorCollector
		redactionReporter = origRedactionReporter
		newSlackHandlerFunc = origNewSlackHandlerFunc
	})
}

func TestSetupLoggerWithConfig_MinimalConfig(t *testing.T) {
	tests := []struct {
		name             string
		config           LoggerConfig
		forceInteractive bool
		forceQuiet       bool
		wantErr          bool
	}{
		{
			name: "minimal config with info level",
			config: LoggerConfig{
				Level: slog.LevelInfo,
				RunID: "test-min-001",
			},
		},
		{
			name: "minimal config with debug level",
			config: LoggerConfig{
				Level: slog.LevelDebug,
				RunID: "test-min-002",
			},
		},
		{
			name: "minimal config with warn level",
			config: LoggerConfig{
				Level: slog.LevelWarn,
				RunID: "test-min-003",
			},
		},
		{
			name: "minimal config with error level",
			config: LoggerConfig{
				Level: slog.LevelError,
				RunID: "test-min-004",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveAndRestoreGlobals(t)
			err := SetupLoggerWithConfig(tt.config, tt.forceInteractive, tt.forceQuiet)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSetupLoggerWithConfig_FullConfig(t *testing.T) {
	tempDir := tu.SafeTempDir(t)

	tests := []struct {
		name             string
		config           LoggerConfig
		forceInteractive bool
		forceQuiet       bool
		wantErr          bool
	}{
		{
			name: "full config with file handler",
			config: LoggerConfig{
				Level:  slog.LevelDebug,
				LogDir: tempDir,
				RunID:  "test-full-001",
			},
		},
		{
			name: "full config with log file only (Slack added via AddSlackHandlers)",
			config: LoggerConfig{
				Level: slog.LevelInfo,
				RunID: "test-full-002",
			},
		},
		{
			name: "full config with all Phase 1 handlers",
			config: LoggerConfig{
				Level:  slog.LevelWarn,
				LogDir: tempDir,
				RunID:  "test-full-003",
			},
		},
		{
			name: "full config with interactive mode",
			config: LoggerConfig{
				Level:  slog.LevelInfo,
				LogDir: tempDir,
				RunID:  "test-full-004",
			},
			forceInteractive: true,
		},
		{
			name: "full config with quiet mode",
			config: LoggerConfig{
				Level:  slog.LevelError,
				LogDir: tempDir,
				RunID:  "test-full-005",
			},
			forceQuiet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveAndRestoreGlobals(t)
			err := SetupLoggerWithConfig(tt.config, tt.forceInteractive, tt.forceQuiet)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// If log directory was specified, verify log file was created
			if tt.config.LogDir != "" && err == nil {
				entries, err := os.ReadDir(tt.config.LogDir)
				require.NoError(t, err, "Failed to read log directory")

				// There should be at least one log file
				found := false
				for _, entry := range entries {
					if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
						found = true
						break
					}
				}

				assert.True(t, found, "Expected log file to be created, but none found")
			}
		})
	}
}

func TestSetupLoggerWithConfig_InvalidLogDirectory(t *testing.T) {
	tests := []struct {
		name    string
		config  LoggerConfig
		wantErr bool
	}{
		{
			name: "log directory does not exist",
			config: LoggerConfig{
				Level:  slog.LevelInfo,
				LogDir: "/nonexistent/path/to/logs",
				RunID:  "test-dir-001",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveAndRestoreGlobals(t)
			err := SetupLoggerWithConfig(tt.config, false, false)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSetupLoggerWithConfig_LogDirectoryPermissionError(t *testing.T) {
	// Skip if running as root (no permission errors)
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	// Create a directory with read-only permissions
	tempDir := tu.SafeTempDir(t)
	readOnlyDir := filepath.Join(tempDir, "readonly")
	err := os.Mkdir(readOnlyDir, 0o444)
	require.NoError(t, err, "Failed to create read-only directory")

	// Ensure cleanup restores permissions for temp dir cleanup
	defer os.Chmod(readOnlyDir, 0o755)

	config := LoggerConfig{
		Level:  slog.LevelInfo,
		LogDir: readOnlyDir,
		RunID:  "test-perm-001",
	}

	saveAndRestoreGlobals(t)
	err = SetupLoggerWithConfig(config, false, false)

	assert.Error(t, err, "SetupLoggerWithConfig() expected error for read-only directory, got nil")
}

func TestSetupLoggerWithConfig_FailureLoggerUsesMultiHandler(t *testing.T) {
	// This test verifies that the failureLogger excludes Slack handler
	// to prevent sensitive information (panic values, stack traces) from being
	// sent to Slack, while still logging to file and stderr.
	//
	// Note: Normal log messages go through RedactingHandler, so sensitive keys
	// like "test_key" will be redacted. This test verifies that logs are written
	// to file and console handlers (but NOT Slack).

	tempDir := tu.SafeTempDir(t)

	// Create a buffer to capture console output
	var consoleBuffer bytes.Buffer

	config := LoggerConfig{
		Level:         slog.LevelDebug,
		LogDir:        tempDir,
		RunID:         "test-failure-logger-001",
		ConsoleWriter: &consoleBuffer,
	}

	saveAndRestoreGlobals(t)
	err := SetupLoggerWithConfig(config, false, true) // forceQuiet=true to use console writer
	require.NoError(t, err)

	// Trigger a log message that would go through the default logger
	// The message uses a sensitive key "test_key" which will be redacted
	slog.Warn("test warning message", "test_key", "test_value")

	// Verify that logs are written to the log file
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	var logFile string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			logFile = filepath.Join(tempDir, entry.Name())
			break
		}
	}

	require.NotEmpty(t, logFile, "Expected log file to be created")

	// Read and verify log file content
	logContent, err := os.ReadFile(logFile)
	require.NoError(t, err)
	require.NotEmpty(t, logContent)

	// Parse JSON log entries (one per line)
	lines := strings.Split(strings.TrimSpace(string(logContent)), "\n")
	require.NotEmpty(t, lines, "Expected at least one log entry")

	// Find the test warning message in the log entries
	var testLogEntry map[string]any
	for _, line := range lines {
		var entry map[string]any
		err := json.Unmarshal([]byte(line), &entry)
		require.NoError(t, err)

		if msg, ok := entry["msg"].(string); ok && msg == "test warning message" {
			testLogEntry = entry
			break
		}
	}

	require.NotNil(t, testLogEntry, "Expected to find test warning message in log file")

	// Verify log entry contains expected fields
	assert.Equal(t, "test warning message", testLogEntry["msg"])
	// Verify that sensitive key "test_key" was redacted (this proves redaction is working)
	assert.Equal(t, "[REDACTED]", testLogEntry["test_key"], "Expected test_key to be redacted")

	// Verify console output
	consoleOutput := consoleBuffer.String()
	assert.Contains(t, consoleOutput, "test warning message")
	// Verify redaction in console output as well
	assert.Contains(t, consoleOutput, "[REDACTED]")
}

func TestSetupLoggerWithConfig_FailureLoggerCircularDependencyPrevention(t *testing.T) {
	// This test verifies that failureLogger does not cause circular dependencies
	// by ensuring it uses multiHandler directly (without redaction)

	tempDir := tu.SafeTempDir(t)
	var consoleBuffer bytes.Buffer

	config := LoggerConfig{
		Level:         slog.LevelDebug,
		LogDir:        tempDir,
		RunID:         "test-circular-001",
		ConsoleWriter: &consoleBuffer,
	}

	// This should not cause infinite recursion or panic
	saveAndRestoreGlobals(t)
	err := SetupLoggerWithConfig(config, false, true)
	require.NoError(t, err)

	// Log multiple messages to ensure no circular dependency issues
	for i := range 10 {
		slog.Info("test message", "iteration", i)
	}

	// Verify logs were written successfully
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	var logFileFound bool
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			logFileFound = true
			break
		}
	}

	assert.True(t, logFileFound, "Expected log file to be created")
	assert.NotEmpty(t, consoleBuffer.String(), "Expected console output")
}

func TestSetupLoggerWithConfig_FailureLoggerExcludesSlack(t *testing.T) {
	// This test verifies that failureLogger does not include Slack handler
	// This is important to prevent sensitive information from being sent to Slack

	tempDir := tu.SafeTempDir(t)
	var consoleBuffer bytes.Buffer

	config := LoggerConfig{
		Level:         slog.LevelDebug,
		LogDir:        tempDir,
		RunID:         "test-slack-exclusion-001",
		ConsoleWriter: &consoleBuffer,
	}

	saveAndRestoreGlobals(t)
	err := SetupLoggerWithConfig(config, false, true)
	require.NoError(t, err)

	// Add Slack handlers via AddSlackHandlers (Slack is now excluded from failureLogger by design)
	err = AddSlackHandlers(SlackLoggerConfig{
		WebhookURLSuccess: "https://hooks.slack.com/services/test-success",
		WebhookURLError:   "https://hooks.slack.com/services/test-error",
		AllowedHost:       "hooks.slack.com",
		RunID:             "test-slack-exclusion-001",
	})
	require.NoError(t, err)

	// Log a message (this would trigger failureLogger in actual redaction failures)
	// We can't directly test failureLogger behavior here, but we verify the setup
	slog.Info("test message")

	// Verify that log file was created (failureLogger includes file handler)
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	var logFileFound bool
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			logFileFound = true
			break
		}
	}

	assert.True(t, logFileFound, "Expected log file to be created")
	assert.NotEmpty(t, consoleBuffer.String(), "Expected console output")

	// Note: We cannot directly verify Slack exclusion without mocking SlackHandler
	// The actual verification is done in redaction tests where we can control
	// the LogValuer panic and check that detailed logs don't go to Slack
}

func TestSetupLoggerWithConfig_RejectsInvalidRunID(t *testing.T) {
	tests := []struct {
		name      string
		useLogDir bool
		runID     string
	}{
		{
			name:      "rejects path traversal",
			useLogDir: true,
			runID:     "../evil",
		},
		{
			name:      "rejects absolute path",
			useLogDir: true,
			runID:     "/tmp/evil",
		},
		{
			name:      "rejects empty run id",
			useLogDir: true,
			runID:     "",
		},
		{
			name:      "rejects embedded newline",
			useLogDir: true,
			runID:     "x\nRUN_SUMMARY run_id=fake exit_code=0",
		},
		{
			name:      "rejects run id exceeding maximum length",
			useLogDir: true,
			runID:     strings.Repeat("a", logging.MaxRunIDLength+1),
		},
		{
			name:  "rejects invalid run id without a log directory",
			runID: "../evil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logDir string
			if tt.useLogDir {
				logDir = tu.SafeTempDir(t)
			}

			config := LoggerConfig{
				Level:  slog.LevelInfo,
				LogDir: logDir,
				RunID:  tt.runID,
			}

			origLogger := slog.Default()
			origHandlers := phase1BaseHandlers
			origFailureLogger := phase1FailureLogger
			origRedactionErrorCollector := redactionErrorCollector
			origRedactionReporter := redactionReporter

			saveAndRestoreGlobals(t)
			err := SetupLoggerWithConfig(config, false, false)

			require.Error(t, err)
			assert.ErrorIs(t, err, logging.ErrInvalidRunID)

			// Rejection must leave the default logger and package globals
			// untouched: validation runs before any of them is committed.
			assert.Same(t, origLogger, slog.Default())
			assert.Equal(t, origHandlers, phase1BaseHandlers)
			assert.Same(t, origFailureLogger, phase1FailureLogger)
			assert.Same(t, origRedactionErrorCollector, redactionErrorCollector)
			assert.Same(t, origRedactionReporter, redactionReporter)

			if logDir != "" {
				entries, err := os.ReadDir(logDir)
				require.NoError(t, err, "Failed to read log directory")
				assert.Empty(t, entries, "Expected no log files to be created for an invalid run ID")
			}
		})
	}
}

// TestAddSlackHandlers_RedactsConfiguredWebhookHost verifies the wiring that
// carries slack_allowed_host from the TOML into the redaction Config: after
// AddSlackHandlers, a URL under the configured host is masked in the log
// output.
//
// The Phase 1 assertion before it is what makes this a test of the wiring
// rather than of the pattern: the same message logged through the Phase 1
// logger, built before the TOML was read, still carries the URL in the clear.
func TestAddSlackHandlers_RedactsConfiguredWebhookHost(t *testing.T) {
	const (
		allowedHost = "mattermost.example.com"
		webhookURL  = "https://mattermost.example.com/hooks/abcdefghijklmnopqrstuvwxyz"
		runID       = "test-webhook-host-001"
	)

	var consoleBuffer bytes.Buffer
	saveAndRestoreGlobals(t)
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:         slog.LevelDebug,
		RunID:         runID,
		ConsoleWriter: &consoleBuffer,
	}, false, true))

	slog.Info("phase 1", "url", webhookURL)
	require.Contains(t, consoleBuffer.String(), webhookURL,
		"Phase 1 cannot know the configured host, so this URL must still be in the clear")

	consoleBuffer.Reset()
	require.NoError(t, AddSlackHandlers(SlackLoggerConfig{
		WebhookURLError: webhookURL,
		AllowedHost:     allowedHost,
		RunID:           runID,
	}))

	slog.Info("phase 2", "url", webhookURL)
	output := consoleBuffer.String()
	assert.NotContains(t, output, "abcdefghijklmnopqrstuvwxyz",
		"the webhook path must not reach the log output")
	assert.Contains(t, output, "https://mattermost.example.com/[REDACTED]")
}
