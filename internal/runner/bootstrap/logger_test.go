package bootstrap

import (
	"os"
	"path/filepath"
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
