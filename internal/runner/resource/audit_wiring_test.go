package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// fixedPlanEvaluator returns a preset plan/error, so audit-wiring tests can drive
// the manager down a chosen allow/deny/error path deterministically.
type fixedPlanEvaluator struct {
	plan risktypes.VerifiedCommandPlan
	err  error
}

func (e fixedPlanEvaluator) EvaluateRisk(*runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error) {
	return e.plan, e.err
}

// newAuditingNormalManager builds a NormalResourceManager whose audit logger
// writes JSON to the returned buffer, so tests can inspect the emitted
// command_risk_profile entries.
func newAuditingNormalManager(evaluator risk.Evaluator) (*NormalResourceManager, *executortestutil.MockExecutor, *bytes.Buffer) {
	var buf bytes.Buffer
	auditLogger := audit.NewAuditLoggerWithCustom(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	mockExec := executortestutil.NewMockExecutor()
	mgr := newNormalManager(Config{
		Executor:      mockExec,
		FileSystem:    &MockFileSystem{},
		Logger:        slog.Default(),
		RiskEvaluator: evaluator,
		AuditLogger:   auditLogger,
	}, nil)
	return mgr, mockExec, &buf
}

// findRiskProfileEntry returns the single command_risk_profile log entry written
// to buf, failing the test if none is present.
func findRiskProfileEntry(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["audit_type"] == "command_risk_profile" {
			return entry
		}
	}
	t.Fatalf("no command_risk_profile audit entry found in: %q", buf.String())
	return nil
}

// TestExecute_EmitsRiskProfileAudit verifies a normal-mode allow decision
// emits a command_risk_profile audit entry with the correlation fields.
func TestExecute_EmitsRiskProfileAudit(t *testing.T) {
	mgr, mockExec, buf := newAuditingNormalManager(permissiveTestEvaluator{})
	cmd := executortestutil.CreateRuntimeCommand("/bin/echo", []string{"hello"}, executortestutil.WithName("echo_cmd"))
	group := createTestCommandGroup()
	ctx := context.Background()

	mockExec.On("Execute", ctx, mock.Anything, cmd, mock.Anything, mock.Anything).
		Return(&executor.Result{ExitCode: 0, Stdout: "hello"}, nil)

	_, _, err := mgr.ExecuteCommand(ctx, cmd, group, map[string]string{})
	require.NoError(t, err)

	entry := findRiskProfileEntry(t, buf)
	assert.Equal(t, "allow", entry["decision"])
	assert.Equal(t, "normal", entry["mode"])
	assert.Equal(t, "echo_cmd", entry["command_name"])
	assert.Equal(t, "/bin/echo", entry["resolved_path"])
	assert.Contains(t, entry, "max_allowed_risk")
	mockExec.AssertExpectations(t)
}

// TestExecute_RejectedCommandAuditable verifies a denied command is
// audited (decision=deny) at a deny-floor severity and is not executed.
func TestExecute_RejectedCommandAuditable(t *testing.T) {
	denyPlan := risktypes.VerifiedCommandPlan{
		Identity: &risktypes.VerifiedIdentity{ResolvedPath: "/usr/bin/rm", ContentHash: "sha256:abc"},
		Assessment: risktypes.RiskAssessment{
			Level:          runnertypes.RiskLevelHigh,
			Blocking:       true,
			BlockingReason: risktypes.ReasonDestructiveFileOperation,
			ReasonCodes:    []risktypes.ReasonCode{risktypes.ReasonDestructiveFileOperation},
		},
	}
	mgr, mockExec, buf := newAuditingNormalManager(fixedPlanEvaluator{plan: denyPlan})
	cmd := executortestutil.CreateRuntimeCommand("/usr/bin/rm", []string{"-rf", "/tmp/x"}, executortestutil.WithName("rm_cmd"))
	group := createTestCommandGroup()
	ctx := context.Background()

	_, _, err := mgr.ExecuteCommand(ctx, cmd, group, map[string]string{})
	require.ErrorIs(t, err, runnertypes.ErrCommandSecurityViolation)

	entry := findRiskProfileEntry(t, buf)
	assert.Equal(t, "deny", entry["decision"])
	assert.Equal(t, "/usr/bin/rm", entry["resolved_path"])
	assert.Equal(t, "sha256:abc", entry["content_hash"])
	assert.Equal(t, "destructive_file_operation", entry["blocking_reason"])
	// Deny severity floor: not Info/Debug even though the level mapping
	// alone would not require Warn for some levels.
	assert.Contains(t, []any{"WARN", "ERROR"}, entry["level"])
	// The command must not have been executed.
	mockExec.AssertNotCalled(t, "Execute", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
