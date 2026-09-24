//go:build test

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	resourcetestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/resource/testutil"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/require"
)

const (
	hashDirPerm    = 0o700
	configFilePerm = 0o600
)

// testEnvironment holds common test setup artifacts.
type testEnvironment struct {
	TestDir    string
	HashDir    string
	ConfigPath string
	RunID      string
}

// setupTestEnvironment creates the common test directory structure.
func setupTestEnvironment(t *testing.T, runID string) *testEnvironment {
	t.Helper()
	testDir := tu.SafeTempDir(t)
	hashDir := filepath.Join(testDir, "hashes")
	configPath := filepath.Join(testDir, "config.toml")

	err := os.MkdirAll(hashDir, hashDirPerm)
	require.NoError(t, err)

	return &testEnvironment{
		TestDir:    testDir,
		HashDir:    hashDir,
		ConfigPath: configPath,
		RunID:      runID,
	}
}

// writeConfig writes the configuration content to the config file.
func (env *testEnvironment) writeConfig(t *testing.T, configContent string) {
	t.Helper()
	err := os.WriteFile(env.ConfigPath, []byte(configContent), configFilePerm)
	require.NoError(t, err)
}

// createRunner creates and initializes a runner with the test configuration.
func (env *testEnvironment) createRunner(t *testing.T) *runner.Runner {
	t.Helper()

	verificationManager, err := verification.NewManagerForTest(env.HashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, env.ConfigPath, env.RunID)
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// These integration tests run real commands with file validation disabled, so
	// the production identity gate would deny every command. Inject a permissive
	// evaluator; risk classification itself is covered by the risk package tests.
	r, err := runner.NewRunner(
		cfg,
		runner.WithVerificationManager(verificationManager),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithRunID(env.RunID),
		runner.WithRiskEvaluator(resourcetestutil.NewAllowAllEvaluator()),
	)
	require.NoError(t, err)

	return r
}

// outputFilePath returns a path for the output.txt file in the test directory.
func (env *testEnvironment) outputFilePath() string {
	return filepath.Join(env.TestDir, "output.txt")
}

// echoPath returns the absolute path to the echo binary.
// On macOS (Apple Silicon), echo is at /bin/echo; on Linux it is at /usr/bin/echo.
func echoPath(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("echo")
	require.NoError(t, err, "echo command not found in PATH")
	return path
}

// requireExitCode asserts that cmd finished (so cmd.ProcessState is populated)
// and that its exit code matches want. Asserting ProcessState is non-nil first
// gives a clear failure message if the process never started/finished, instead
// of a silently misleading -1 from ExitCode()'s nil-safe behavior.
func requireExitCode(t *testing.T, cmd *exec.Cmd, want int) {
	t.Helper()
	require.NotNil(t, cmd.ProcessState, "cmd.ProcessState is nil; process did not finish")
	require.Equal(t, want, cmd.ProcessState.ExitCode(), "exit code mismatch")
}

// captureStdoutStderr runs fn with os.Stdout and os.Stderr redirected to pipes
// and returns what it wrote to each.
func captureStdoutStderr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	outReader, outWriter, err := os.Pipe()
	require.NoError(t, err)
	errReader, errWriter, err := os.Pipe()
	require.NoError(t, err)

	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outWriter, errWriter
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origStdout, origStderr
		// The write ends are normally already closed by the time fn returns;
		// closing again here only matters if fn panicked.
		_ = outWriter.Close()
		_ = errWriter.Close()
		_ = outReader.Close()
		_ = errReader.Close()
	})

	// Drain both pipes while fn runs: a pipe holds only a fixed kernel buffer,
	// and a writer that fills it would block forever with nobody reading.
	var wg sync.WaitGroup
	var outBuf, errBuf bytes.Buffer
	for _, drain := range []struct {
		dst *bytes.Buffer
		src *os.File
	}{{&outBuf, outReader}, {&errBuf, errReader}} {
		wg.Go(func() {
			_, _ = io.Copy(drain.dst, drain.src)
		})
	}

	fn()

	// Restore before closing, so os.Stdout and os.Stderr never name a closed
	// pipe -- not even for the two statements below, and not if a Close error
	// aborts this function early.
	os.Stdout, os.Stderr = origStdout, origStderr

	// Close the write ends so the drain goroutines see EOF and finish.
	require.NoError(t, outWriter.Close())
	require.NoError(t, errWriter.Close())
	wg.Wait()

	return outBuf.String(), errBuf.String()
}

// slackRun is what one in-process run of mainWithExitCode produced when its
// Slack handler was pointed at a mock server.
type slackRun struct {
	exitCode int
	stderr   string
	payloads []logging.SlackMessage
}

// slackRunSpec describes the run: configBody receives the mock Slack server's
// hostname (for slack_allowed_host) and returns the configuration file body;
// hashedFiles lists the files whose hash is recorded in the temporary hash
// directory in addition to the configuration file itself.
type slackRunSpec struct {
	configBody  func(slackHost string) string
	hashedFiles []string
	runID       string
}

// runMainWithSlackMock drives the production reporting boundary
// (mainWithExitCode) with a real configuration file and a temporary hash
// directory, delivering Slack notifications to an in-process mock server
// through the handler-factory seam (a separate process cannot be handed the
// mock server's TLS client). stdout and stderr are captured with
// captureStdoutStderr so a long report cannot fill a pipe and stall the run.
//
// The run always uses dryRun = false so hash verification is enforced. It
// replaces process-wide state (package-level flag variables, the default
// logger, the default hash directory, the handler factory), so a test that
// calls it must not call t.Parallel.
func runMainWithSlackMock(t *testing.T, spec slackRunSpec) slackRun {
	t.Helper()

	var (
		mu       sync.Mutex
		payloads []logging.SlackMessage
	)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var message logging.SlackMessage
		if err := json.Unmarshal(body, &message); err == nil {
			mu.Lock()
			payloads = append(payloads, message)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	configFile := filepath.Join(tu.SafeTempDir(t), "config.toml")
	require.NoError(t, os.WriteFile(configFile, []byte(spec.configBody(serverURL.Hostname())), configFilePerm))

	// Record the hashes with the same validator the production manager builds,
	// so the run gets past pre-registration verification of the configuration
	// and fails only on what the test leaves unrecorded.
	hashDir := tu.SafeTempDir(t)
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
	require.NoError(t, err)
	for _, file := range append([]string{configFile}, spec.hashedFiles...) {
		_, _, err = validator.SaveRecord(file, true)
		require.NoErrorf(t, err, "recording the hash of %s must succeed", file)
	}

	restoreHashDir := cmdcommon.DefaultHashDirectory
	cmdcommon.DefaultHashDirectory = hashDir
	t.Cleanup(func() { cmdcommon.DefaultHashDirectory = restoreHashDir })

	// Pin every Slack setting the bootstrap reads from the environment, so a
	// value left over from a local Slack experiment (a 1ms flush timeout,
	// say) cannot make the notification miss the mock server.
	t.Setenv(logging.SlackWebhookURLErrorEnvVar, "https://hooks.slack.com/services/error")
	t.Setenv(logging.SlackWebhookURLSuccessEnvVar, "")
	t.Setenv(logging.SlackSendTimeoutEnvVar, "")
	t.Setenv(logging.SlackFlushTimeoutEnvVar, "")
	t.Setenv(logging.SlackSyncEnvVar, "")

	restoreFactory := bootstrap.SetSlackHandlerFactory(func(opts logging.SlackHandlerOptions) (*logging.SlackHandler, error) {
		opts.WebhookURL = server.URL
		opts.AllowedHost = serverURL.Hostname()
		opts.HTTPClient = server.Client()
		return logging.NewSlackHandler(opts)
	})
	t.Cleanup(restoreFactory)

	originalLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	// The package-level flag values production reads.
	originalConfigPath, originalLogLevel, originalLogDir := configPath, logLevel, logDir
	originalDryRun, originalGroups, originalRunID := dryRun, groups, runID
	t.Cleanup(func() {
		configPath, logLevel, logDir = originalConfigPath, originalLogLevel, originalLogDir
		dryRun, groups, runID = originalDryRun, originalGroups, originalRunID
	})
	configPath = configFile
	logLevel = "info"
	logDir = tu.SafeTempDir(t)
	dryRun = false
	groups = ""
	runID = ""

	var exitCode int
	_, stderr := captureStdoutStderr(t, func() {
		exitCode = mainWithExitCode(spec.runID)
		bootstrap.FlushSlackNotifications()
	})
	// The run's logger was bound to the pipe that captureStdoutStderr has now
	// closed; restore the default logger right away rather than leaving it
	// pointing at a closed writer until the test ends.
	slog.SetDefault(originalLogger)

	mu.Lock()
	defer mu.Unlock()
	return slackRun{exitCode: exitCode, stderr: stderr, payloads: payloads}
}

// requireSinglePreExecutionError asserts that exactly one Slack payload
// arrived and returns it with its attachment fields.
func requireSinglePreExecutionError(t *testing.T, run slackRun) (logging.SlackMessage, []logging.SlackAttachmentField) {
	t.Helper()
	// stderr carries the undelivered-notification warning when delivery
	// failed, which is the one line that explains an empty payload list.
	require.Lenf(t, run.payloads, 1, "the pre-execution error should reach Slack exactly once; stderr:\n%s", run.stderr)
	message := run.payloads[0]
	require.Len(t, message.Attachments, 1)
	return message, message.Attachments[0].Fields
}

// attachmentField returns the value of the first field with the title.
func attachmentField(t *testing.T, fields []logging.SlackAttachmentField, title string) string {
	t.Helper()
	for _, field := range fields {
		if field.Title == title {
			return field.Value
		}
	}
	t.Fatalf("attachment has no field titled %q: %v", title, fields)
	return ""
}

// stderrDetailsLine returns the "  Details:" line handleErrorCommon writes to
// stderr. The rest of stderr also carries the structured log line (with
// failed_file_paths) and the per-file verification errors, so assertions
// about the human-readable report must look at this line only.
func stderrDetailsLine(t *testing.T, stderr string) string {
	t.Helper()
	for line := range strings.SplitSeq(stderr, "\n") {
		if strings.HasPrefix(line, "  Details:") {
			return line
		}
	}
	t.Fatalf("no \"  Details:\" line in stderr: %q", stderr)
	return ""
}
