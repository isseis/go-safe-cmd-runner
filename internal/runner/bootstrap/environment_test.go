package bootstrap

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupLogging_Success(t *testing.T) {
	tests := []struct {
		name             string
		logLevel         slog.Level
		logDir           string
		runID            string
		forceInteractive bool
		forceQuiet       bool
		slackSuccessURL  string
		slackErrorURL    string
		wantErr          bool
	}{
		{
			name:     "minimal config with info level",
			logLevel: slog.LevelInfo,
			runID:    "test-run-001",
		},
		{
			name:     "with log directory",
			logLevel: slog.LevelDebug,
			logDir:   t.TempDir(),
			runID:    "test-run-002",
		},
		{
			name:            "with both Slack webhook URLs",
			logLevel:        slog.LevelWarn,
			runID:           "test-run-003",
			slackSuccessURL: "https://hooks.slack.com/services/test-success",
			slackErrorURL:   "https://hooks.slack.com/services/test-error",
		},
		{
			name:             "force interactive mode",
			logLevel:         slog.LevelInfo,
			runID:            "test-run-004",
			forceInteractive: true,
		},
		{
			name:       "force quiet mode",
			logLevel:   slog.LevelError,
			runID:      "test-run-005",
			forceQuiet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetupLogging(SetupLoggingOptions{
				LogLevel:               tt.logLevel,
				LogDir:                 tt.logDir,
				RunID:                  tt.runID,
				ForceInteractive:       tt.forceInteractive,
				ForceQuiet:             tt.forceQuiet,
				ConsoleWriter:          nil,
				SlackWebhookURLSuccess: tt.slackSuccessURL,
				SlackWebhookURLError:   tt.slackErrorURL,
			})

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
			name:      "invalid log directory - does not exist",
			logLevel:  slog.LevelInfo,
			logDir:    "/nonexistent/path/to/logs",
			runID:     "test-run-error-001",
			setupFunc: func(_ *testing.T) string { return "/nonexistent/path/to/logs" },
			wantErr:   true,
		},
		{
			name:     "invalid log directory - not writable",
			logLevel: slog.LevelInfo,
			runID:    "test-run-error-002",
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
			logDir := tt.logDir
			if tt.setupFunc != nil {
				logDir = tt.setupFunc(t)
			}

			err := SetupLogging(SetupLoggingOptions{
				LogLevel:         tt.logLevel,
				LogDir:           logDir,
				RunID:            tt.runID,
				ForceInteractive: tt.forceInteractive,
				ForceQuiet:       tt.forceQuiet,
			})

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

	err := SetupLogging(SetupLoggingOptions{
		LogLevel: slog.LevelInfo,
		LogDir:   readOnlyDir,
		RunID:    "test-run-perm",
	})

	assert.Error(t, err, "SetupLogging() expected error for read-only directory")
}

// TestValidateSlackWebhookEnv tests Slack webhook environment variable validation.
// Test cases: ENV-01 ~ ENV-07
func TestValidateSlackWebhookEnv(t *testing.T) {
	// Save and restore original environment variables
	origOld := os.Getenv(logging.SlackWebhookURLEnvVar)
	origSuccess := os.Getenv(logging.SlackWebhookURLSuccessEnvVar)
	origError := os.Getenv(logging.SlackWebhookURLErrorEnvVar)
	t.Cleanup(func() {
		setOrUnset(logging.SlackWebhookURLEnvVar, origOld)
		setOrUnset(logging.SlackWebhookURLSuccessEnvVar, origSuccess)
		setOrUnset(logging.SlackWebhookURLErrorEnvVar, origError)
	})

	tests := []struct {
		name        string
		successURL  string
		errorURL    string
		oldURL      string
		wantErr     error
		wantSuccess string
		wantError   string
	}{
		// ENV-01: 両方設定
		{
			name:        "ENV-01: both SUCCESS and ERROR set",
			successURL:  "https://hooks.slack.com/success",
			errorURL:    "https://hooks.slack.com/error",
			oldURL:      "",
			wantErr:     nil,
			wantSuccess: "https://hooks.slack.com/success",
			wantError:   "https://hooks.slack.com/error",
		},
		// ENV-02: ERROR のみ
		{
			name:        "ENV-02: only ERROR set",
			successURL:  "",
			errorURL:    "https://hooks.slack.com/error",
			oldURL:      "",
			wantErr:     nil,
			wantSuccess: "",
			wantError:   "https://hooks.slack.com/error",
		},
		// ENV-03: SUCCESS のみ
		{
			name:        "ENV-03: only SUCCESS set",
			successURL:  "https://hooks.slack.com/success",
			errorURL:    "",
			oldURL:      "",
			wantErr:     ErrSuccessWithoutError,
			wantSuccess: "",
			wantError:   "",
		},
		// ENV-04: 両方未設定
		{
			name:        "ENV-04: neither set (Slack disabled)",
			successURL:  "",
			errorURL:    "",
			oldURL:      "",
			wantErr:     nil,
			wantSuccess: "",
			wantError:   "",
		},
		// ENV-05: 旧変数設定
		{
			name:        "ENV-05: deprecated env var set",
			successURL:  "",
			errorURL:    "",
			oldURL:      "https://hooks.slack.com/old",
			wantErr:     ErrDeprecatedSlackWebhook,
			wantSuccess: "",
			wantError:   "",
		},
		// ENV-06: 旧変数+新変数
		{
			name:        "ENV-06: deprecated env var with new vars",
			successURL:  "https://hooks.slack.com/success",
			errorURL:    "https://hooks.slack.com/error",
			oldURL:      "https://hooks.slack.com/old",
			wantErr:     ErrDeprecatedSlackWebhook,
			wantSuccess: "",
			wantError:   "",
		},
		// ENV-07: 同一URL
		{
			name:        "ENV-07: same URL for both",
			successURL:  "https://hooks.slack.com/same",
			errorURL:    "https://hooks.slack.com/same",
			oldURL:      "",
			wantErr:     nil,
			wantSuccess: "https://hooks.slack.com/same",
			wantError:   "https://hooks.slack.com/same",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test
			setOrUnset(logging.SlackWebhookURLEnvVar, tt.oldURL)
			setOrUnset(logging.SlackWebhookURLSuccessEnvVar, tt.successURL)
			setOrUnset(logging.SlackWebhookURLErrorEnvVar, tt.errorURL)

			config, err := ValidateSlackWebhookEnv()

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr), "Expected error %v, got %v", tt.wantErr, err)
				assert.Nil(t, config)
			} else {
				require.NoError(t, err)
				require.NotNil(t, config)
				assert.Equal(t, tt.wantSuccess, config.SuccessURL)
				assert.Equal(t, tt.wantError, config.ErrorURL)
			}
		})
	}
}

// setOrUnset sets or unsets an environment variable
func setOrUnset(key, value string) {
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
}

func TestFormatDeprecatedSlackWebhookError(t *testing.T) {
	msg := FormatDeprecatedSlackWebhookError()
	assert.Contains(t, msg, "GSCR_SLACK_WEBHOOK_URL is deprecated")
	assert.Contains(t, msg, "GSCR_SLACK_WEBHOOK_URL_SUCCESS")
	assert.Contains(t, msg, "GSCR_SLACK_WEBHOOK_URL_ERROR")
}

func TestFormatSuccessWithoutErrorError(t *testing.T) {
	msg := FormatSuccessWithoutErrorError()
	assert.Contains(t, msg, "GSCR_SLACK_WEBHOOK_URL_SUCCESS is set")
	assert.Contains(t, msg, "GSCR_SLACK_WEBHOOK_URL_ERROR is not")
}
