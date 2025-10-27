package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func TestSetupLogging_Success(t *testing.T) {
	tests := []struct {
		name             string
		logLevel         runnertypes.LogLevel
		logDir           string
		runID            string
		forceInteractive bool
		forceQuiet       bool
		slackURL         string
		wantErr          bool
	}{
		{
			name:             "minimal config with info level",
			logLevel:         runnertypes.LogLevelInfo,
			logDir:           "",
			runID:            "test-run-001",
			forceInteractive: false,
			forceQuiet:       false,
			slackURL:         "",
			wantErr:          false,
		},
		{
			name:             "with log directory",
			logLevel:         runnertypes.LogLevelDebug,
			logDir:           t.TempDir(),
			runID:            "test-run-002",
			forceInteractive: false,
			forceQuiet:       false,
			slackURL:         "",
			wantErr:          false,
		},
		{
			name:             "with Slack webhook URL",
			logLevel:         runnertypes.LogLevelWarn,
			logDir:           "",
			runID:            "test-run-003",
			forceInteractive: false,
			forceQuiet:       false,
			slackURL:         "https://hooks.slack.com/services/test",
			wantErr:          false,
		},
		{
			name:             "force interactive mode",
			logLevel:         runnertypes.LogLevelInfo,
			logDir:           "",
			runID:            "test-run-004",
			forceInteractive: true,
			forceQuiet:       false,
			slackURL:         "",
			wantErr:          false,
		},
		{
			name:             "force quiet mode",
			logLevel:         runnertypes.LogLevelError,
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
			// Save and restore Slack webhook URL environment variable
			oldSlackURL := os.Getenv("SLACK_WEBHOOK_URL")
			defer func() {
				if oldSlackURL != "" {
					os.Setenv("SLACK_WEBHOOK_URL", oldSlackURL)
				} else {
					os.Unsetenv("SLACK_WEBHOOK_URL")
				}
			}()

			if tt.slackURL != "" {
				os.Setenv("SLACK_WEBHOOK_URL", tt.slackURL)
			} else {
				os.Unsetenv("SLACK_WEBHOOK_URL")
			}

			err := SetupLogging(tt.logLevel, tt.logDir, tt.runID, tt.forceInteractive, tt.forceQuiet)

			if (err != nil) != tt.wantErr {
				t.Errorf("SetupLogging() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetupLogging_InvalidConfig(t *testing.T) {
	tests := []struct {
		name             string
		logLevel         runnertypes.LogLevel
		logDir           string
		runID            string
		forceInteractive bool
		forceQuiet       bool
		setupFunc        func(t *testing.T) string
		wantErr          bool
	}{
		{
			name:             "invalid log directory - does not exist",
			logLevel:         runnertypes.LogLevelInfo,
			logDir:           "/nonexistent/path/to/logs",
			runID:            "test-run-error-001",
			forceInteractive: false,
			forceQuiet:       false,
			setupFunc:        func(_ *testing.T) string { return "/nonexistent/path/to/logs" },
			wantErr:          true,
		},
		{
			name:             "invalid log directory - not writable",
			logLevel:         runnertypes.LogLevelInfo,
			logDir:           "",
			runID:            "test-run-error-002",
			forceInteractive: false,
			forceQuiet:       false,
			setupFunc: func(t *testing.T) string {
				// Create a directory with no write permissions
				dir := filepath.Join(t.TempDir(), "readonly")
				if err := os.Mkdir(dir, 0o444); err != nil {
					t.Fatalf("Failed to create test directory: %v", err)
				}
				return dir
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unset Slack webhook URL
			oldSlackURL := os.Getenv("SLACK_WEBHOOK_URL")
			defer func() {
				if oldSlackURL != "" {
					os.Setenv("SLACK_WEBHOOK_URL", oldSlackURL)
				} else {
					os.Unsetenv("SLACK_WEBHOOK_URL")
				}
			}()
			os.Unsetenv("SLACK_WEBHOOK_URL")

			logDir := tt.logDir
			if tt.setupFunc != nil {
				logDir = tt.setupFunc(t)
			}

			err := SetupLogging(tt.logLevel, logDir, tt.runID, tt.forceInteractive, tt.forceQuiet)

			if (err != nil) != tt.wantErr {
				t.Errorf("SetupLogging() error = %v, wantErr %v", err, tt.wantErr)
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
		t.Fatalf("Failed to create read-only directory: %v", err)
	}

	// Ensure cleanup restores permissions for temp dir cleanup
	defer os.Chmod(readOnlyDir, 0o755)

	err := SetupLogging(runnertypes.LogLevelInfo, readOnlyDir, "test-run-perm", false, false)

	if err == nil {
		t.Error("SetupLogging() expected error for read-only directory, got nil")
	}
}
