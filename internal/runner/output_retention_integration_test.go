//go:build test

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	resourcetestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/resource/testutil"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	verificationtestutil "github.com/isseis/go-safe-cmd-runner/internal/verification/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestOutputRetention_SlackAndDebugFieldsFromBoundedOutput checks the fields
// users see for a command whose stdout overflows the executor's 64 KiB
// leading window, run through the real executor:
//
//   - (a) a successful run through the group executor: the debug log's stdout
//     and the command_group_summary record, delivered to a Slack handler;
//   - (b) a failed run whose Result feeds the user/group audit record, the
//     only path that records stdout of a failed command, delivered to Slack as
//     user_group_command_failure.
//
// Each command writes some complete lines and then a newline-free bulk that
// overflows the window, so the retained stdout is exactly those lines plus the
// omission marker counting the bulk. Whether the marker shows up in a field
// depends on whether the kept lines reach that field's cut position.
func TestOutputRetention_SlackAndDebugFieldsFromBoundedOutput(t *testing.T) {
	const bulk = 70 * 1024 // newline-free bytes beyond the leading window

	// The cut positions of the fields under test: the debug log keeps 500
	// bytes of stdout, Slack keeps 997 plus a three-byte "...".
	const debugCut, slackCut = 500, 997

	longLine := strings.Repeat("L", 1200) // longer than both cut positions

	tests := []struct {
		name    string
		leading string // complete lines written before the bulk
		// fieldsUnchanged: the kept lines reach every cut position, so the
		// fields read as they did when all of stdout was retained.
		fieldsUnchanged bool
	}{
		{name: "no newline in the leading window", leading: ""},
		{name: "kept lines shorter than the cut", leading: "short line\n"},
		{name: "kept lines longer than the cut", leading: longLine + "\n", fieldsUnchanged: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retained := tt.leading + fmt.Sprintf("\n... omitting %d bytes ...\n", bulk)

			var wantDebug, wantSlack string
			if tt.fieldsUnchanged {
				wantDebug = longLine[:debugCut] + "... (truncated)"
				wantSlack = "```\n" + longLine[:slackCut] + "...\n```"
			} else {
				require.Less(t, len(retained), debugCut, "the row must fit both fields whole")
				wantDebug = retained
				wantSlack = "```\n" + retained + "\n```"
			}

			script := fmt.Sprintf("printf '%%s' '%s'; head -c %d /dev/zero | tr -c x x", tt.leading, bulk)

			t.Run("group summary and debug log", func(t *testing.T) {
				records := runGroupRecordingLogs(t, script)

				debug := findRecord(t, records, slog.LevelDebug, "Command execution result")
				assert.Equal(t, wantDebug, recordAttr(t, debug, "stdout").String())

				summary := findRecord(t, records, slog.LevelInfo, "Command group execution completed")
				results, ok := recordAttr(t, summary, common.GroupSummaryAttrs.Commands).Any().(common.CommandResults)
				require.True(t, ok, "the commands attribute must carry common.CommandResults")
				require.Len(t, results, 1)
				assert.Equal(t, retained, results[0].Output)

				assert.Equal(t, wantSlack, slackOutputField(t, summary))
			})

			t.Run("user group failure audit", func(t *testing.T) {
				cmd := executortestutil.CreateRuntimeCommand(
					executortestutil.ResolveCommand("sh"), []string{"-c", script + "; exit 3"},
					executortestutil.WithName("overflowing-cmd"),
					executortestutil.WithWorkDir(""))
				result, err := executor.NewDefaultExecutor().Execute(context.Background(), nil, cmd, map[string]string{}, nil)
				require.Error(t, err)
				require.NotNil(t, result)
				require.Equal(t, 3, result.ExitCode)
				require.Equal(t, retained, result.Stdout)

				var records []slog.Record
				logger := slog.New(tu.NewCallbackHandler(func(r slog.Record) { records = append(records, r.Clone()) }))
				audit.NewAuditLoggerWithCustom(logger).LogUserGroupExecution(context.Background(), cmd,
					&audit.ExecutionResult{Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode},
					0, audit.PrivilegeMetrics{})

				failure := findRecord(t, records, slog.LevelError, "User/group command failed")
				assert.Equal(t, retained, recordAttr(t, failure, common.UserGroupCommandFailureAttrs.Stdout).String())

				assert.Equal(t, wantSlack, slackOutputField(t, failure))
			})
		})
	}
}

// runGroupRecordingLogs runs one command, script under sh, as a group through
// the real group executor, with the notification function Runner wires in
// production, and returns every record logged through slog.Default.
func runGroupRecordingLogs(t *testing.T, script string) []slog.Record {
	t.Helper()

	var mu sync.Mutex
	var records []slog.Record
	logger := slog.New(tu.NewCallbackHandler(func(r slog.Record) {
		mu.Lock()
		defer mu.Unlock()
		records = append(records, r.Clone())
	}))
	origDefault := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(origDefault) })

	validator, err := security.NewValidator(&security.Config{
		AllowedCommands: []string{"^/bin/.*", "^/usr/bin/.*"},
		LoggingOptions:  security.LoggingOptions{RedactSensitiveInfo: true},
	})
	require.NoError(t, err)

	pathResolver := &mockPathResolver{}
	pathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	exec := executor.NewDefaultExecutor()
	var outputMgr output.CaptureManager
	rm, err := resourcetestutil.NewDefaultResourceManager(
		exec,
		common.NewDefaultFileSystem(),
		nil, // no privilege manager: the command carries no run_as
		pathResolver,
		logger,
		resource.ExecutionModeNormal,
		nil, // dry-run disabled
		outputMgr,
		0, // default max output size
	)
	require.NoError(t, err)

	verificationMgr := new(verificationtestutil.MockManager)
	verificationMgr.On("VerifyGroupFiles", mock.Anything).Return(&verification.Result{}, nil)
	verificationMgr.On("ResolvePath", mock.Anything).Return("/bin/sh", nil)
	verificationMgr.On("VerifyCommandDependencies", mock.Anything, mock.Anything).Return(nil)

	r := &Runner{runID: "test-run-output-retention"}
	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:              &runnertypes.ConfigSpec{},
		Executor:            exec,
		ResourceManager:     rm,
		Validator:           validator,
		VerificationManager: verificationMgr,
		RunID:               r.runID,
	}, WithGroupNotificationFunc(r.logGroupExecutionSummary))

	group := &runnertypes.GroupSpec{
		Name: "retention-group",
		Commands: []runnertypes.CommandSpec{{
			Name:      "overflowing-cmd",
			Cmd:       "/bin/sh",
			Args:      []string{"-c", script},
			RiskLevel: runnertypes.RiskLevelMediumPtr,
		}},
	}
	runtimeGlobal := &runnertypes.RuntimeGlobal{Spec: &runnertypes.GlobalSpec{Timeout: new(int32(30))}}
	require.NoError(t, ge.ExecuteGroup(context.Background(), group, runtimeGlobal))

	mu.Lock()
	defer mu.Unlock()
	return records
}

// findRecord returns the single record at level with message msg.
func findRecord(t *testing.T, records []slog.Record, level slog.Level, msg string) slog.Record {
	t.Helper()
	var found []slog.Record
	for _, r := range records {
		if r.Level == level && r.Message == msg {
			found = append(found, r)
		}
	}
	require.Len(t, found, 1, "expected exactly one %s record %q", level, msg)
	return found[0]
}

// recordAttr returns the value of the record's attribute key.
func recordAttr(t *testing.T, r slog.Record, key string) slog.Value {
	t.Helper()
	var value slog.Value
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			value, found = a.Value, true
			return false
		}
		return true
	})
	require.True(t, found, "record %q has no attribute %q", r.Message, key)
	return value
}

// slackOutputField hands r to a Slack handler sending synchronously to a local
// TLS server and returns the value of the stdout field of the one message
// sent: the field titled "Output" (user_group_command_failure) or ending in
// " Output" (command_group_summary), never "Error Output".
func slackOutputField(t *testing.T, r slog.Record) string {
	t.Helper()

	var mu sync.Mutex
	var payloads [][]byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		payloads = append(payloads, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	handler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL:  server.URL,
		AllowedHost: serverURL.Hostname(),
		RunID:       "test-run-output-retention",
		HTTPClient:  server.Client(),
		Synchronous: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = handler.Close() })

	require.NoError(t, handler.Handle(context.Background(), r))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, payloads, 1, "the record must produce exactly one Slack message")
	var msg logging.SlackMessage
	require.NoError(t, json.Unmarshal(payloads[0], &msg))

	var values []string
	for _, attachment := range msg.Attachments {
		for _, field := range attachment.Fields {
			if field.Title == "Output" || (strings.HasSuffix(field.Title, " Output") && !strings.Contains(field.Title, "Error")) {
				values = append(values, field.Value)
			}
		}
	}
	require.Len(t, values, 1, "expected exactly one stdout field in %s", payloads[0])
	return values[0]
}
