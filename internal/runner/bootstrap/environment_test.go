package bootstrap

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupLogging_Success(t *testing.T) {
	tests := []struct {
		name             string
		logLevel         slog.Level
		logDir           string
		runID            string
		forceInteractive bool
		forceQuiet       bool
		slackURL         string
		wantErr          bool
	}{
		{
			name:             "minimal config with info level",
			logLevel:         slog.LevelInfo,
			logDir:           "",
			runID:            "test-run-001",
			forceInteractive: false,
			forceQuiet:       false,
			slackURL:         "",
			wantErr:          false,
		},
		{
			name:             "with log directory",
			logLevel:         slog.LevelDebug,
			logDir:           t.TempDir(),
			runID:            "test-run-002",
			forceInteractive: false,
			forceQuiet:       false,
			slackURL:         "",
			wantErr:          false,
		},
		{
			name:             "with Slack webhook URL",
			logLevel:         slog.LevelWarn,
			logDir:           "",
			runID:            "test-run-003",
			forceInteractive: false,
			forceQuiet:       false,
			slackURL:         "https://hooks.slack.com/services/test",
			wantErr:          false,
		},
		{
			name:             "force interactive mode",
			logLevel:         slog.LevelInfo,
			logDir:           "",
			runID:            "test-run-004",
			forceInteractive: true,
			forceQuiet:       false,
			slackURL:         "",
			wantErr:          false,
		},
		{
			name:             "force quiet mode",
			logLevel:         slog.LevelError,
			logDir:           "",
			runID:            "test-run-005",
			forceInteractive: false,
			forceQuiet:       true,
			slackURL:         "",
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.slackURL != "" {
				t.Setenv("SLACK_WEBHOOK_URL", tt.slackURL)
			}

			err := SetupLogging(tt.logLevel, tt.logDir, tt.runID, tt.forceInteractive, tt.forceQuiet, nil)

			if (err != nil) != tt.wantErr {
				assert.NoError(t, err, "SetupLogging() should not error")
			}
		})
	}
}

func TestSetupLogging_InvalidConfig(t *testing.T) {
	tests := []struct {
		name             string
		logLevel         slog.Level
		logDir           string
		runID            string
		forceInteractive bool
		forceQuiet       bool
		setupFunc        func(t *testing.T) string
		wantErr          bool
	}{
		{
			name:             "invalid log directory - does not exist",
			logLevel:         slog.LevelInfo,
			logDir:           "/nonexistent/path/to/logs",
			runID:            "test-run-error-001",
			forceInteractive: false,
			forceQuiet:       false,
			setupFunc:        func(_ *testing.T) string { return "/nonexistent/path/to/logs" },
			wantErr:          true,
		},
		{
			name:             "invalid log directory - not writable",
			logLevel:         slog.LevelInfo,
			logDir:           "",
			runID:            "test-run-error-002",
			forceInteractive: false,
			forceQuiet:       false,
			setupFunc: func(t *testing.T) string {
				// Create a directory with no write permissions
				dir := filepath.Join(t.TempDir(), "readonly")
				if err := os.Mkdir(dir, 0o444); err != nil {
					assert.NoError(t, err, "Failed to create test directory")
				}
				return dir
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unset Slack webhook URL
			t.Setenv("SLACK_WEBHOOK_URL", "")

			logDir := tt.logDir
			if tt.setupFunc != nil {
				logDir = tt.setupFunc(t)
			}

			err := SetupLogging(tt.logLevel, logDir, tt.runID, tt.forceInteractive, tt.forceQuiet, nil)

			if (err != nil) != tt.wantErr {
				assert.Equal(t, tt.wantErr, err != nil, "SetupLogging() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetupLogging_FilePermissionError(t *testing.T) {
	// Skip if running as root (no permission errors)
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	// Create a directory with read-only permissions
	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0o444); err != nil {
		assert.NoError(t, err, "Failed to create read-only directory")
	}

	// Ensure cleanup restores permissions for temp dir cleanup
	defer os.Chmod(readOnlyDir, 0o755)

	err := SetupLogging(slog.LevelInfo, readOnlyDir, "test-run-perm", false, false, nil)

	assert.Error(t, err, "SetupLogging() expected error for read-only directory")
}
