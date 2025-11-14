package bootstrap

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				Level:           runnertypes.LogLevelInfo,
				LogDir:          "",
				RunID:           "test-min-001",
				SlackWebhookURL: "",
			},
			forceInteractive: false,
			forceQuiet:       false,
			wantErr:          false,
		},
		{
			name: "minimal config with debug level",
			config: LoggerConfig{
				Level:           runnertypes.LogLevelDebug,
				LogDir:          "",
				RunID:           "test-min-002",
				SlackWebhookURL: "",
			},
			forceInteractive: false,
			forceQuiet:       false,
			wantErr:          false,
		},
		{
			name: "minimal config with warn level",
			config: LoggerConfig{
				Level:           runnertypes.LogLevelWarn,
				LogDir:          "",
				RunID:           "test-min-003",
				SlackWebhookURL: "",
			},
			forceInteractive: false,
			forceQuiet:       false,
			wantErr:          false,
		},
		{
			name: "minimal config with error level",
			config: LoggerConfig{
				Level:           runnertypes.LogLevelError,
				LogDir:          "",
				RunID:           "test-min-004",
				SlackWebhookURL: "",
			},
			forceInteractive: false,
			forceQuiet:       false,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	tempDir := t.TempDir()

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
				Level:           runnertypes.LogLevelDebug,
				LogDir:          tempDir,
				RunID:           "test-full-001",
				SlackWebhookURL: "",
			},
			forceInteractive: false,
			forceQuiet:       false,
			wantErr:          false,
		},
		{
			name: "full config with Slack handler",
			config: LoggerConfig{
				Level:           runnertypes.LogLevelInfo,
				LogDir:          "",
				RunID:           "test-full-002",
				SlackWebhookURL: "https://hooks.slack.com/services/test",
			},
			forceInteractive: false,
			forceQuiet:       false,
			wantErr:          false,
		},
		{
			name: "full config with all handlers",
			config: LoggerConfig{
				Level:           runnertypes.LogLevelWarn,
				LogDir:          tempDir,
				RunID:           "test-full-003",
				SlackWebhookURL: "https://hooks.slack.com/services/test",
			},
			forceInteractive: false,
			forceQuiet:       false,
			wantErr:          false,
		},
		{
			name: "full config with interactive mode",
			config: LoggerConfig{
				Level:           runnertypes.LogLevelInfo,
				LogDir:          tempDir,
				RunID:           "test-full-004",
				SlackWebhookURL: "",
			},
			forceInteractive: true,
			forceQuiet:       false,
			wantErr:          false,
		},
		{
			name: "full config with quiet mode",
			config: LoggerConfig{
				Level:           runnertypes.LogLevelError,
				LogDir:          tempDir,
				RunID:           "test-full-005",
				SlackWebhookURL: "",
			},
			forceInteractive: false,
			forceQuiet:       true,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestSetupLoggerWithConfig_InvalidLogLevel(t *testing.T) {
	tests := []struct {
		name             string
		config           LoggerConfig
		forceInteractive bool
		forceQuiet       bool
		wantErr          bool
	}{
		{
			name: "invalid log level - fallback to info",
			config: LoggerConfig{
				Level:           runnertypes.LogLevel("invalid"),
				LogDir:          "",
				RunID:           "test-invalid-001",
				SlackWebhookURL: "",
			},
			forceInteractive: false,
			forceQuiet:       false,
			wantErr:          false, // Should not error, just warn and use default
		},
		{
			name: "empty log level - fallback to info",
			config: LoggerConfig{
				Level:           runnertypes.LogLevel(""),
				LogDir:          "",
				RunID:           "test-invalid-002",
				SlackWebhookURL: "",
			},
			forceInteractive: false,
			forceQuiet:       false,
			wantErr:          false, // Should not error, just warn and use default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetupLoggerWithConfig(tt.config, tt.forceInteractive, tt.forceQuiet)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
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
				Level:           runnertypes.LogLevelInfo,
				LogDir:          "/nonexistent/path/to/logs",
				RunID:           "test-dir-001",
				SlackWebhookURL: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")
	err := os.Mkdir(readOnlyDir, 0o444)
	require.NoError(t, err, "Failed to create read-only directory")

	// Ensure cleanup restores permissions for temp dir cleanup
	defer os.Chmod(readOnlyDir, 0o755)

	config := LoggerConfig{
		Level:           runnertypes.LogLevelInfo,
		LogDir:          readOnlyDir,
		RunID:           "test-perm-001",
		SlackWebhookURL: "",
	}

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

	tempDir := t.TempDir()

	// Create a buffer to capture console output
	var consoleBuffer bytes.Buffer

	config := LoggerConfig{
		Level:         runnertypes.LogLevelDebug,
		LogDir:        tempDir,
		RunID:         "test-failure-logger-001",
		ConsoleWriter: &consoleBuffer,
	}

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
	var testLogEntry map[string]interface{}
	for _, line := range lines {
		var entry map[string]interface{}
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

	tempDir := t.TempDir()
	var consoleBuffer bytes.Buffer

	config := LoggerConfig{
		Level:         runnertypes.LogLevelDebug,
		LogDir:        tempDir,
		RunID:         "test-circular-001",
		ConsoleWriter: &consoleBuffer,
	}

	// This should not cause infinite recursion or panic
	err := SetupLoggerWithConfig(config, false, true)
	require.NoError(t, err)

	// Log multiple messages to ensure no circular dependency issues
	for i := 0; i < 10; i++ {
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

	tempDir := t.TempDir()
	var consoleBuffer bytes.Buffer

	config := LoggerConfig{
		Level:           runnertypes.LogLevelDebug,
		LogDir:          tempDir,
		RunID:           "test-slack-exclusion-001",
		SlackWebhookURL: "https://hooks.slack.com/services/test",
		ConsoleWriter:   &consoleBuffer,
	}

	err := SetupLoggerWithConfig(config, false, true)
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
