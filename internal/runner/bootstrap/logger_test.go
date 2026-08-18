package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
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
	origSlackHandlers := slackHandlers
	t.Cleanup(func() {
		// Close before restoring: any handler this test registered owns a
		// worker goroutine, and once the global is overwritten nothing can
		// reach it to stop it (rule R2).
		closeSlackHandlers(slackHandlers)
		slackHandlers = origSlackHandlers
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
	_, err = AddSlackHandlers(SlackLoggerConfig{
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
	_, err := AddSlackHandlers(SlackLoggerConfig{
		WebhookURLError: webhookURL,
		AllowedHost:     allowedHost,
		RunID:           runID,
	})
	require.NoError(t, err)

	slog.Info("phase 2", "url", webhookURL)
	output := consoleBuffer.String()
	assert.NotContains(t, output, "abcdefghijklmnopqrstuvwxyz",
		"the webhook path must not reach the log output")
	assert.Contains(t, output, "https://mattermost.example.com/[REDACTED]")
}

func TestParseSlackEnvSettings(t *testing.T) {
	tests := []struct {
		name            string
		env             map[string]string
		wantSend        time.Duration
		wantFlush       time.Duration
		wantSynchronous bool
		wantInvalid     []string
	}{
		{
			name:      "unset falls back to the package defaults",
			env:       nil,
			wantSend:  logging.DefaultSendTimeout,
			wantFlush: logging.DefaultFlushTimeout,
		},
		{
			name: "valid durations are taken as given",
			env: map[string]string{
				logging.SlackSendTimeoutEnvVar:  "5s",
				logging.SlackFlushTimeoutEnvVar: "1m30s",
			},
			wantSend:  5 * time.Second,
			wantFlush: 90 * time.Second,
		},
		{
			name:        "unparsable duration falls back and is reported",
			env:         map[string]string{logging.SlackSendTimeoutEnvVar: "30 seconds"},
			wantSend:    logging.DefaultSendTimeout,
			wantFlush:   logging.DefaultFlushTimeout,
			wantInvalid: []string{logging.SlackSendTimeoutEnvVar},
		},
		{
			name: "zero and negative durations are refused",
			env: map[string]string{
				logging.SlackSendTimeoutEnvVar:  "0s",
				logging.SlackFlushTimeoutEnvVar: "-5s",
			},
			wantSend:    logging.DefaultSendTimeout,
			wantFlush:   logging.DefaultFlushTimeout,
			wantInvalid: []string{logging.SlackSendTimeoutEnvVar, logging.SlackFlushTimeoutEnvVar},
		},
		{
			name:            "GSCR_SLACK_SYNC=1 selects synchronous sending",
			env:             map[string]string{logging.SlackSyncEnvVar: "1"},
			wantSend:        logging.DefaultSendTimeout,
			wantFlush:       logging.DefaultFlushTimeout,
			wantSynchronous: true,
		},
		{
			name:      "GSCR_SLACK_SYNC=true is not an accepted spelling",
			env:       map[string]string{logging.SlackSyncEnvVar: "true"},
			wantSend:  logging.DefaultSendTimeout,
			wantFlush: logging.DefaultFlushTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSlackEnvSettings(func(name string) string { return tt.env[name] })

			assert.Equal(t, tt.wantSend, got.SendTimeout)
			assert.Equal(t, tt.wantFlush, got.FlushTimeout)
			assert.Equal(t, tt.wantSynchronous, got.Synchronous)
			assert.Equal(t, tt.wantInvalid, got.invalidVars)
		})
	}
}

// TestParseSlackEnvSettings_ReportedNameCarriesNoValue pins that a mistyped
// value stays out of the report. The send-failure logger does not pass through
// the redaction layer, so whatever an operator put in the variable must not be
// carried along with the variable's name.
func TestParseSlackEnvSettings_ReportedNameCarriesNoValue(t *testing.T) {
	const secretish = "not-a-duration-s3cr3t"

	saveAndRestoreGlobals(t)
	var consoleBuffer bytes.Buffer
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:         slog.LevelInfo,
		RunID:         "test-env-report-001",
		ConsoleWriter: &consoleBuffer,
	}, false, true))

	settings := parseSlackEnvSettings(func(name string) string {
		if name == logging.SlackSendTimeoutEnvVar {
			return secretish
		}
		return ""
	})
	require.Equal(t, []string{logging.SlackSendTimeoutEnvVar}, settings.invalidVars)

	reportInvalidEnvSettings(settings)

	output := consoleBuffer.String()
	assert.Contains(t, output, logging.SlackSendTimeoutEnvVar,
		"the operator needs to know which variable was ignored")
	assert.NotContains(t, output, secretish,
		"the rejected value must not reach the log output")
}

// TestAddSlackHandlers_PropagatesEnvSettings verifies that the delivery
// settings read from the environment reach NewSlackHandler, and that the send
// path's own records are routed to the Phase 1 handlers.
func TestAddSlackHandlers_PropagatesEnvSettings(t *testing.T) {
	tests := []struct {
		name            string
		env             map[string]string
		wantSend        time.Duration
		wantSynchronous bool
	}{
		{
			name:     "defaults: asynchronous with the default send timeout",
			env:      nil,
			wantSend: logging.DefaultSendTimeout,
		},
		{
			name: "overrides reach the handler options",
			env: map[string]string{
				logging.SlackSendTimeoutEnvVar: "7s",
				logging.SlackSyncEnvVar:        "1",
			},
			wantSend:        7 * time.Second,
			wantSynchronous: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saveAndRestoreGlobals(t)
			for name, value := range tt.env {
				t.Setenv(name, value)
			}
			require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
				Level: slog.LevelInfo,
				RunID: "test-env-settings-001",
			}, false, true))

			var capturedOpts []logging.SlackHandlerOptions
			newSlackHandlerFunc = func(opts logging.SlackHandlerOptions) (*logging.SlackHandler, error) {
				capturedOpts = append(capturedOpts, opts)
				return &logging.SlackHandler{}, nil
			}

			_, err := AddSlackHandlers(SlackLoggerConfig{
				WebhookURLSuccess: "https://hooks.slack.com/services/success",
				WebhookURLError:   "https://hooks.slack.com/services/error",
				AllowedHost:       "hooks.slack.com",
				RunID:             "test-env-settings-001",
			})
			require.NoError(t, err)

			require.Len(t, capturedOpts, 2, "both the success and the error webhook should be configured")
			for i, opts := range capturedOpts {
				assert.Equal(t, tt.wantSend, opts.SendTimeout, "handler %d send timeout", i)
				assert.Equal(t, tt.wantSynchronous, opts.Synchronous, "handler %d send mode", i)
				assert.Equal(t, phase1BaseHandlers, opts.FailureHandlers,
					"handler %d should record send failures through the Phase 1 handlers", i)
			}
		})
	}
}

// TestAddSlackHandlers_SlackHandlersComeAfterPhase1Handlers pins the ordering
// the asynchronous design depends on: because MultiHandler calls its handlers
// in order, a record is written to the log file before it is queued for Slack,
// so a notification lost at exit still has a complete record on disk.
func TestAddSlackHandlers_SlackHandlersComeAfterPhase1Handlers(t *testing.T) {
	saveAndRestoreGlobals(t)
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:  slog.LevelInfo,
		RunID:  "test-handler-order-001",
		LogDir: t.TempDir(),
	}, false, true))

	newSlackHandlerFunc = func(_ logging.SlackHandlerOptions) (*logging.SlackHandler, error) {
		return &logging.SlackHandler{}, nil
	}

	basePhase1 := slices.Clone(phase1BaseHandlers)
	require.NotEmpty(t, basePhase1, "Phase 1 should have created at least one handler")

	_, err := AddSlackHandlers(SlackLoggerConfig{
		WebhookURLSuccess: "https://hooks.slack.com/services/success",
		WebhookURLError:   "https://hooks.slack.com/services/error",
		AllowedHost:       "hooks.slack.com",
		RunID:             "test-handler-order-001",
	})
	require.NoError(t, err)

	redactingHandler, ok := slog.Default().Handler().(*redaction.RedactingHandler)
	require.True(t, ok, "AddSlackHandlers should install a RedactingHandler")
	multiHandler, ok := redactingHandler.Handler().(*logging.MultiHandler)
	require.True(t, ok, "the RedactingHandler should wrap a MultiHandler")

	handlers := multiHandler.Handlers()
	require.Len(t, handlers, len(basePhase1)+2, "two Slack handlers should have been appended")
	assert.Equal(t, basePhase1, handlers[:len(basePhase1)],
		"the Phase 1 handlers should keep their positions at the front")
	for i, h := range handlers[len(basePhase1):] {
		assert.IsType(t, &logging.SlackHandler{}, h,
			"handler %d after the Phase 1 handlers should be a Slack handler", i)
	}
}

// TestAddSlackHandlers_ClosesFirstHandlerOnSecondFailure verifies the
// partial-failure rule: a handler created before the failure owns a worker this
// call is the only owner of, so it must not be left running. Closure is
// observed as rule R3 prescribes here -- a notification submitted to a closed
// handler is dropped with the sender_closed reason.
func TestAddSlackHandlers_ClosesFirstHandlerOnSecondFailure(t *testing.T) {
	saveAndRestoreGlobals(t)
	consoleBuffer := &syncBuffer{}
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:         slog.LevelInfo,
		RunID:         "test-partial-failure-001",
		ConsoleWriter: consoleBuffer,
	}, false, true))

	server, requests := newRecordingSlackServer(t)
	errSecondHandler := errors.New("second handler unavailable")
	created := installMockSlackFactory(t, server, 1, errSecondHandler)

	_, err := AddSlackHandlers(SlackLoggerConfig{
		WebhookURLSuccess: "https://hooks.slack.com/services/success",
		WebhookURLError:   "https://hooks.slack.com/services/error",
		AllowedHost:       "hooks.slack.com",
		RunID:             "test-partial-failure-001",
	})
	require.ErrorIs(t, err, errSecondHandler)
	require.Len(t, *created, 1, "the success handler should have been created before the failure")
	assert.Empty(t, slackHandlers, "a failed rebuild must register no handlers")

	consoleBuffer.Reset()
	submitSlackNotification(t, (*created)[0])

	assert.Contains(t, consoleBuffer.String(), "sender_closed",
		"a notification submitted to the closed handler should be recorded as dropped")
	assert.Empty(t, requests.paths(), "the closed handler must not send anything")
}

// TestAddSlackHandlers_ClosesPreviousHandlersOnReinvocation verifies the
// re-invocation rule: the second call replaces the default logger, after which
// nothing could reach the first call's workers to stop them.
func TestAddSlackHandlers_ClosesPreviousHandlersOnReinvocation(t *testing.T) {
	saveAndRestoreGlobals(t)
	consoleBuffer := &syncBuffer{}
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:         slog.LevelInfo,
		RunID:         "test-reinvocation-001",
		ConsoleWriter: consoleBuffer,
	}, false, true))

	server, requests := newRecordingSlackServer(t)
	created := installMockSlackFactory(t, server, 0, nil)

	config := SlackLoggerConfig{
		WebhookURLError: "https://hooks.slack.com/services/error",
		AllowedHost:     "hooks.slack.com",
		RunID:           "test-reinvocation-001",
	}
	_, err := AddSlackHandlers(config)
	require.NoError(t, err)
	_, err = AddSlackHandlers(config)
	require.NoError(t, err)

	require.Len(t, *created, 2, "each call should have created its own handler")
	require.Len(t, slackHandlers, 1, "only the second call's handler should stay registered")
	assert.Same(t, (*created)[1], slackHandlers[0].handler)

	consoleBuffer.Reset()
	submitSlackNotification(t, (*created)[0])
	assert.Contains(t, consoleBuffer.String(), "sender_closed",
		"the first call's handler should have been closed by the second call")

	consoleBuffer.Reset()
	submitSlackNotification(t, (*created)[1])
	assert.NotContains(t, consoleBuffer.String(), "sender_closed",
		"the second call's handler should still be accepting")
	FlushSlackNotifications()
	assert.Len(t, requests.paths(), 1,
		"exactly the notification submitted to the live handler should have been delivered")
}

// TestAddSlackHandlers_KeepsPreviousHandlersWhenRebuildFails pins the other
// half of the re-invocation rule: a failed rebuild leaves the default logger
// pointing at the previous call's handlers, so those handlers must still be
// accepting. Closing them up front would drop every later notification with
// nobody able to flush it.
func TestAddSlackHandlers_KeepsPreviousHandlersWhenRebuildFails(t *testing.T) {
	saveAndRestoreGlobals(t)
	consoleBuffer := &syncBuffer{}
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:         slog.LevelInfo,
		RunID:         "test-failed-rebuild-001",
		ConsoleWriter: consoleBuffer,
	}, false, true))

	server, requests := newRecordingSlackServer(t)
	errRebuild := errors.New("webhook unavailable")
	created := installMockSlackFactory(t, server, 1, errRebuild)

	config := SlackLoggerConfig{
		WebhookURLError: "https://hooks.slack.com/services/error",
		AllowedHost:     "hooks.slack.com",
		RunID:           "test-failed-rebuild-001",
	}
	_, err := AddSlackHandlers(config)
	require.NoError(t, err)
	_, err = AddSlackHandlers(config)
	require.ErrorIs(t, err, errRebuild)

	require.Len(t, slackHandlers, 1, "the failed rebuild should leave the previous registration in place")
	require.Len(t, *created, 1)
	assert.Same(t, (*created)[0], slackHandlers[0].handler)

	consoleBuffer.Reset()
	submitSlackNotification(t, (*created)[0])
	assert.NotContains(t, consoleBuffer.String(), "sender_closed",
		"the handler the default logger still routes to must keep accepting")

	FlushSlackNotifications()
	assert.Len(t, requests.paths(), 1,
		"the notification submitted after the failed rebuild should still be delivered")
}

// TestFlushSlackNotifications_FlushesAllHandlers verifies that every registered
// webhook is drained at exit and that each one gets its own summary.
func TestFlushSlackNotifications_FlushesAllHandlers(t *testing.T) {
	saveAndRestoreGlobals(t)
	consoleBuffer := &syncBuffer{}
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:         slog.LevelInfo,
		RunID:         "test-flush-all-001",
		ConsoleWriter: consoleBuffer,
	}, false, true))

	successServer, successRequests := newRecordingSlackServer(t)
	errorServer, errorRequests := newRecordingSlackServer(t)

	newSlackHandlerFunc = func(opts logging.SlackHandlerOptions) (*logging.SlackHandler, error) {
		server := successServer
		if opts.LevelMode == logging.LevelModeWarnAndAbove {
			server = errorServer
		}
		handler, err := logging.NewSlackHandler(withMockServer(t, opts, server))
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { handler.Close() })
		return handler, nil
	}

	_, err := AddSlackHandlers(SlackLoggerConfig{
		WebhookURLSuccess: "https://hooks.slack.com/services/success",
		WebhookURLError:   "https://hooks.slack.com/services/error",
		AllowedHost:       "hooks.slack.com",
		RunID:             "test-flush-all-001",
	})
	require.NoError(t, err)
	require.Len(t, slackHandlers, 2)

	// Through the default logger, so the level routing that decides which
	// webhook each record reaches is exercised as it is in production.
	slog.Info("run finished", "slack_notify", true, "message_type", "command_group_summary")
	slog.Error("run failed", "slack_notify", true, "message_type", "pre_execution_error")

	consoleBuffer.Reset()
	FlushSlackNotifications()

	assert.Len(t, successRequests.paths(), 1, "the success webhook should have received its notification")
	assert.Len(t, errorRequests.paths(), 1, "the error webhook should have received its notification")

	// The counts are part of the assertion, not just the webhook names: a
	// summary naming the webhook proves only that a sender terminated, not that
	// this flush drained it.
	output := consoleBuffer.String()
	assert.Contains(t, output, "Slack delivery summary")
	for role, messageType := range map[string]string{
		slackRoleSuccess: "command_group_summary",
		slackRoleError:   "pre_execution_error",
	} {
		assert.Contains(t, output,
			fmt.Sprintf("webhook=%s submitted=1 sent=1 failed=0 dropped=0 pending=0 sent_by_message_type=map[%s:1]",
				role, messageType),
			"the %s webhook should report its own notification as delivered", role)
	}
}

// TestFlushSlackNotifications_HonorsFlushDeadline pins that the exit flush is
// bounded by GSCR_SLACK_FLUSH_TIMEOUT: an unresponsive Slack must not hold the
// process open. Without the deadline this test would sit on the endpoint until
// the send timeout, well past the bound asserted below.
func TestFlushSlackNotifications_HonorsFlushDeadline(t *testing.T) {
	const flushTimeout = 200 * time.Millisecond

	saveAndRestoreGlobals(t)
	t.Setenv(logging.SlackFlushTimeoutEnvVar, flushTimeout.String())

	consoleBuffer := &syncBuffer{}
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:         slog.LevelInfo,
		RunID:         "test-flush-deadline-001",
		ConsoleWriter: consoleBuffer,
	}, false, true))

	// The endpoint never answers until the test releases it, so the flush can
	// only return by hitting its deadline.
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	// Registered after server.Close so that it runs before it: Close waits for
	// the handler above to return, which it cannot do while still parked.
	t.Cleanup(func() { close(release) })

	installMockSlackFactory(t, server, 0, nil)

	_, err := AddSlackHandlers(SlackLoggerConfig{
		WebhookURLError: "https://hooks.slack.com/services/error",
		AllowedHost:     "hooks.slack.com",
		RunID:           "test-flush-deadline-001",
	})
	require.NoError(t, err)
	require.Len(t, slackHandlers, 1)

	submitSlackNotification(t, slackHandlers[0].handler)

	start := time.Now()
	FlushSlackNotifications()
	elapsed := time.Since(start)

	// Generous against a loaded CI machine relative to the 200ms deadline, and
	// still below what ignoring the deadline costs: 5s for the per-send budget
	// (measured), 15s for a wholly unbounded flush.
	assert.Less(t, elapsed, 2*time.Second,
		"the flush should return on its own deadline rather than waiting for the send")
	assert.Contains(t, consoleBuffer.String(), "pending=1",
		"the notification the deadline cut short should be reported as pending")
}

// TestFlushSlackNotifications_NoSlackConfigured pins that a run without Slack
// configured -- the common case -- neither panics nor reports a delivery
// summary about webhooks that do not exist.
func TestFlushSlackNotifications_NoSlackConfigured(t *testing.T) {
	saveAndRestoreGlobals(t)
	consoleBuffer := &syncBuffer{}
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:         slog.LevelInfo,
		RunID:         "test-flush-none-001",
		ConsoleWriter: consoleBuffer,
	}, false, true))
	require.Empty(t, slackHandlers)
	consoleBuffer.Reset()

	FlushSlackNotifications()

	assert.Empty(t, consoleBuffer.String(),
		"a run with no Slack handler should produce no delivery summary")
}

// syncBuffer is a console writer that the test goroutine can read while the
// Slack worker goroutine writes the send path's records to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// newRecordingSlackServer starts a TLS mock Slack endpoint (rule R1) and
// returns the paths it was asked for.
func newRecordingSlackServer(t *testing.T) (*httptest.Server, *requestLog) {
	t.Helper()
	requests := &requestLog{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.add(r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server, requests
}

// requestLog collects the paths a mock server received. The worker sends from
// its own goroutine, so the slice needs a lock.
type requestLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *requestLog) add(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, path)
}

func (l *requestLog) paths() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.seen)
}

// installMockSlackFactory makes AddSlackHandlers build real handlers against
// server (rule R1), each registering its own Close (rule R2), and returns the
// handlers it created in order. When failAfter is positive, calls past that
// many successes return failErr instead, which is how the ownership rules for
// a half-built rebuild are exercised.
func installMockSlackFactory(t *testing.T, server *httptest.Server, failAfter int, failErr error) *[]*logging.SlackHandler {
	t.Helper()
	created := &[]*logging.SlackHandler{}
	newSlackHandlerFunc = func(opts logging.SlackHandlerOptions) (*logging.SlackHandler, error) {
		if failAfter > 0 && len(*created) >= failAfter {
			return nil, failErr
		}
		handler, err := logging.NewSlackHandler(withMockServer(t, opts, server))
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { handler.Close() })
		*created = append(*created, handler)
		return handler, nil
	}
	return created
}

// withMockServer points handler options at a mock server (rule R1).
// AddSlackHandlers builds the options itself and sets no HTTP client, so a test
// that wants a real handler talking to httptest has to inject the client here,
// in the newSlackHandlerFunc replacement, rather than in the config it passes.
func withMockServer(t *testing.T, opts logging.SlackHandlerOptions, server *httptest.Server) logging.SlackHandlerOptions {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err, "mock server URL should parse")

	// The path is kept so that two webhooks pointing at the same server stay
	// distinguishable in the request log.
	requested, err := url.Parse(opts.WebhookURL)
	require.NoError(t, err, "webhook URL should parse")
	opts.WebhookURL = server.URL + requested.Path

	opts.AllowedHost = parsed.Hostname()
	opts.HTTPClient = server.Client()
	return opts
}

// submitSlackNotification hands one notification straight to a handler,
// bypassing the default logger so that the test controls which handler
// receives it.
func submitSlackNotification(t *testing.T, handler *logging.SlackHandler) {
	t.Helper()
	record := slog.NewRecord(time.Now(), slog.LevelError, "notification", 0)
	record.AddAttrs(
		slog.Bool("slack_notify", true),
		slog.String("message_type", "pre_execution_error"),
	)
	require.NoError(t, handler.Handle(context.Background(), record))
}

// TestAddSlackHandlers_AcceptsInteractivePhase1Handlers pins that the real
// interactive Phase 1 composition passes NewSlackHandler's Slack-free check.
// The check fails closed, and go test's default environment is not interactive,
// so without forcing it the *InteractiveHandler branch would only ever be
// exercised in production -- where being rejected means the process cannot
// start.
func TestAddSlackHandlers_AcceptsInteractivePhase1Handlers(t *testing.T) {
	saveAndRestoreGlobals(t)

	logDir := t.TempDir()
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:  slog.LevelInfo,
		RunID:  "test-interactive-001",
		LogDir: logDir,
	}, true, false))

	require.Contains(t, handlerTypeNames(phase1BaseHandlers), "*logging.InteractiveHandler",
		"forceInteractive should have put an InteractiveHandler in the Phase 1 handlers")

	_, err := AddSlackHandlers(SlackLoggerConfig{
		WebhookURLError: "https://hooks.slack.com/services/error",
		AllowedHost:     "hooks.slack.com",
		RunID:           "test-interactive-001",
	})
	require.NoError(t, err)
}

// handlerTypeNames lists the concrete type of each handler, for assertions
// about the Phase 1 composition.
func handlerTypeNames(handlers []slog.Handler) []string {
	names := make([]string, len(handlers))
	for i, h := range handlers {
		names[i] = fmt.Sprintf("%T", h)
	}
	return names
}

// TestSetupLoggerWithConfig_LogFileNameTimestampIsUTC verifies that the "Z" in
// the log file name is honest: the timestamp is UTC regardless of the host time
// zone. It also pins the three-element name composition, which collection
// scripts slice by position, so a future change cannot switch to UTC by
// dropping the suffix instead.
//
// time.Local is process-wide state, so this test must not run in parallel.
func TestSetupLoggerWithConfig_LogFileNameTimestampIsUTC(t *testing.T) {
	saveAndRestoreGlobals(t)

	origLocal := time.Local
	t.Cleanup(func() { time.Local = origLocal })
	// Ahead of UTC, so a local-time implementation produces a visibly different
	// timestamp instead of an accidentally matching one.
	time.Local = time.FixedZone("TEST+09", 9*60*60)

	logDir := tu.SafeTempDir(t)
	const runID = "test-utf-timestamp-001"
	before := time.Now().UTC()
	require.NoError(t, SetupLoggerWithConfig(LoggerConfig{
		Level:  slog.LevelInfo,
		LogDir: logDir,
		RunID:  runID,
	}, false, true))

	entries, err := os.ReadDir(logDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one log file should have been created")
	name := entries[0].Name()

	hostname := common.GetHostname()
	prefix := hostname + "_"
	suffix := "_" + runID + ".json"
	require.True(t, strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix),
		"log file name %q should be <hostname>_<timestamp>_<runID>.json", name)

	timestamp := name[len(prefix) : len(name)-len(suffix)]
	require.Len(t, timestamp, len("20060102T150405Z"),
		"timestamp %q should keep its fixed width", timestamp)

	parsed, err := time.ParseInLocation("20060102T150405Z", timestamp, time.UTC)
	require.NoError(t, err, "timestamp %q should match the documented layout", timestamp)
	assert.WithinDuration(t, before, parsed, time.Minute,
		"timestamp %q should be the current UTC time, not the local time", timestamp)
}
