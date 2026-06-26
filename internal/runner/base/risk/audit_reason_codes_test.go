//go:build test

package risk

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logDenyReasonCodes runs a real evaluator's plan through audit.LogRiskProfile on
// a deny path and returns the emitted reason_codes. The decision is DecisionDeny
// because each subject command classifies High and would be policy-denied under
// any sub-High ceiling; the plans are non-blocking, so the deny is the manager's
// ceiling decision, modelled here by MaxAllowedRisk=Low + DecisionDeny. The
// evaluator uses a descriptor-free identity opener, so the plan holds no OS
// resource and needs no Close.
func logDenyReasonCodes(t *testing.T, plan risktypes.VerifiedCommandPlan) []any {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	require.NotNil(t, plan.Identity, "plan.Identity must not be nil before logging deny reason codes")
	audit.NewAuditLoggerWithCustom(logger).LogRiskProfile(context.Background(), risktypes.RiskAuditEntry{
		CommandName:    plan.Identity.ResolvedPath,
		Mode:           risktypes.ModeNormal,
		MaxAllowedRisk: runnertypes.RiskLevelLow,
		Decision:       risktypes.DecisionDeny,
		Assessment:     plan.Assessment,
	})

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry), "failed to parse JSON log output")
	assert.Equal(t, "deny", entry["decision"])
	// These subjects are ceiling denies (non-blocking High), so the assessment
	// carries no BlockingReason and the entry omits blocking_reason. Pin that
	// here so a subject that silently became a blocking deny would be noticed;
	// the blocking_reason of a blocking/Critical deny is covered separately by
	// audit_wiring_test.go.
	assert.NotContains(t, entry, "blocking_reason")
	codes, ok := entry["reason_codes"].([]any)
	require.True(t, ok, "reason_codes should be an array")
	return codes
}

// TestLogRiskProfile_DenyReasonCodes_EndToEnd closes the seam between the real
// risk evaluator and the audit output: a plan produced by the actual classifier
// is logged, and the command_risk_profile entry carries the reason code that
// drove the deny. It covers one representative deny per derivation path (axis 1
// name classification, axis 2 trust-zoning, dangerous argument pattern), which
// are otherwise unasserted at the evaluator level.
func TestLogRiskProfile_DenyReasonCodes_EndToEnd(t *testing.T) {
	t.Run("axis 1: insmod is system_modification", func(t *testing.T) {
		ev := newVerifiedEvaluator()
		plan, err := ev.EvaluateRisk(verifiedCmd("/sbin/insmod", []string{"mod.ko"}))
		require.NoError(t, err)
		require.Equal(t, runnertypes.RiskLevelHigh, plan.Assessment.Level)
		require.NotNil(t, plan.Identity)

		codes := logDenyReasonCodes(t, plan)
		assert.Contains(t, codes, string(risktypes.ReasonSystemModification))
	})

	t.Run("axis 2: trust-critical write is trust_boundary_write", func(t *testing.T) {
		wd := filepath.Join(t.TempDir(), "work")
		require.NoError(t, os.MkdirAll(wd, 0o700))
		src := filepath.Join(wd, "payload")
		require.NoError(t, os.WriteFile(src, nil, 0o644))
		ev := newZoningEvaluator(wd, zoningForeignIdent())

		plan, err := ev.EvaluateRisk(verifiedCmdInDir("cp", []string{"-a", src, "/usr/bin"}, wd))
		require.NoError(t, err)
		require.Equal(t, runnertypes.RiskLevelHigh, plan.Assessment.Level)
		require.NotNil(t, plan.Identity)

		codes := logDenyReasonCodes(t, plan)
		assert.Contains(t, codes, string(risktypes.ReasonTrustBoundaryWrite))
	})

	t.Run("dangerous arg pattern: dd of=/dev/sdb is dangerous_arg_pattern", func(t *testing.T) {
		ev := newVerifiedEvaluator()
		plan, err := ev.EvaluateRisk(verifiedCmd("dd", []string{"if=/dev/zero", "of=/dev/sdb"}))
		require.NoError(t, err)
		require.Equal(t, runnertypes.RiskLevelHigh, plan.Assessment.Level)
		require.NotNil(t, plan.Identity)

		codes := logDenyReasonCodes(t, plan)
		assert.Contains(t, codes, string(risktypes.ReasonDangerousArgPattern))
	})
}
