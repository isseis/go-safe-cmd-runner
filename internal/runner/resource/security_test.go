package resource

import (
	"context"
	"testing"

	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// erroringPathResolver always fails to resolve, modeling a command that does not
// exist (a hard error, not a policy deny).
type erroringPathResolver struct{}

func (erroringPathResolver) ResolvePath(string) (string, error) {
	return "", assert.AnError
}

// newPreviewManager builds a dry-run manager wired with the given evaluator and a
// passthrough path resolver, for allow/deny preview tests.
func newPreviewManager(t *testing.T, opts *DryRunOptions, evaluator risk.Evaluator) *DryRunResourceManager {
	t.Helper()
	if opts == nil {
		opts = &DryRunOptions{DetailLevel: DetailLevelDetailed}
	}
	mgr, err := NewDryRunResourceManager(executortestutil.NewMockExecutor(), nil, passthroughPathResolver{}, opts, evaluator, nil)
	require.NoError(t, err)
	return mgr
}

func previewTestGroup() *runnertypes.GroupSpec {
	return &runnertypes.GroupSpec{Name: "preview-group", Description: "dry-run preview test group"}
}

// TestDryRun_EffectiveRiskShown verifies the dry-run preview surfaces the
// effective risk computed by the same evaluator the runtime uses.
func TestDryRun_EffectiveRiskShown(t *testing.T) {
	mgr := newPreviewManager(t, nil, keyedRiskEvaluator{
		"net": {Level: runnertypes.RiskLevelMedium},
	})
	cmd := executortestutil.CreateRuntimeCommand("/usr/bin/curl", []string{"https://example.com"},
		executortestutil.WithName("net"), executortestutil.WithRiskLevel("high"))

	_, result, err := mgr.ExecuteCommand(context.Background(), cmd, previewTestGroup(), map[string]string{})
	require.NoError(t, err)
	require.NotNil(t, result.Analysis)
	assert.Equal(t, "medium", result.Analysis.Impact.SecurityRisk)
	assert.Contains(t, result.Analysis.Impact.Description, "ALLOW")
}

// TestDryRun_AllowDenyPreview verifies the preview reports allow vs deny by
// comparing the effective risk against the configured risk_level.
func TestDryRun_AllowDenyPreview(t *testing.T) {
	mgr := newPreviewManager(t, nil, keyedRiskEvaluator{
		"allow-cmd": {Level: runnertypes.RiskLevelLow},
		"deny-cmd":  {Level: runnertypes.RiskLevelHigh},
	})
	ctx := context.Background()
	group := previewTestGroup()

	allowCmd := executortestutil.CreateRuntimeCommand("/bin/echo", []string{"hi"},
		executortestutil.WithName("allow-cmd"), executortestutil.WithRiskLevel("high"))
	_, allowRes, err := mgr.ExecuteCommand(ctx, allowCmd, group, map[string]string{})
	require.NoError(t, err)
	assert.Contains(t, allowRes.Analysis.Impact.Description, "ALLOW")

	denyCmd := executortestutil.CreateRuntimeCommand("/bin/rm", []string{"-rf", "/tmp/x"},
		executortestutil.WithName("deny-cmd"), executortestutil.WithRiskLevel("low"))
	_, denyRes, err := mgr.ExecuteCommand(ctx, denyCmd, group, map[string]string{})
	require.NoError(t, err) // a policy deny is a preview, not an error
	assert.Contains(t, denyRes.Analysis.Impact.Description, "DENY")
	assert.Equal(t, "high", denyRes.Analysis.Impact.SecurityRisk)

	assert.Equal(t, DryRunExitPolicyDeny, mgr.PreviewExitCode())
}

// TestDryRun_BinaryAnalysisReflected verifies a High/Medium risk derived
// from binary-analysis signals is reflected in the dry-run effective risk.
func TestDryRun_BinaryAnalysisReflected(t *testing.T) {
	mgr := newPreviewManager(t, nil, keyedRiskEvaluator{
		"tool": {
			Level:       runnertypes.RiskLevelHigh,
			ReasonCodes: []risktypes.ReasonCode{risktypes.ReasonBinaryAnalysisDynamicLoad},
		},
	})
	cmd := executortestutil.CreateRuntimeCommand("/usr/local/bin/tool", nil,
		executortestutil.WithName("tool"), executortestutil.WithRiskLevel("high"))

	_, result, err := mgr.ExecuteCommand(context.Background(), cmd, previewTestGroup(), map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, "high", result.Analysis.Impact.SecurityRisk)
}

// TestDryRun_DenyVsHardError verifies a policy deny is a preview (no
// error), while a hard error (path resolution failure) aborts with an error.
func TestDryRun_DenyVsHardError(t *testing.T) {
	t.Run("policy deny is a preview, not an error", func(t *testing.T) {
		mgr := newPreviewManager(t, nil, keyedRiskEvaluator{
			"deny": {Level: runnertypes.RiskLevelHigh},
		})
		cmd := executortestutil.CreateRuntimeCommand("/bin/rm", []string{"-rf"},
			executortestutil.WithName("deny"), executortestutil.WithRiskLevel("low"))
		_, result, err := mgr.ExecuteCommand(context.Background(), cmd, previewTestGroup(), map[string]string{})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Contains(t, result.Analysis.Impact.Description, "DENY")
	})

	t.Run("path resolution failure is a hard error", func(t *testing.T) {
		opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
		mgr, err := NewDryRunResourceManager(executortestutil.NewMockExecutor(), nil, erroringPathResolver{}, opts, permissiveTestEvaluator{}, nil)
		require.NoError(t, err)
		cmd := executortestutil.CreateRuntimeCommand("missing-cmd", nil, executortestutil.WithName("missing"))
		_, result, err := mgr.ExecuteCommand(context.Background(), cmd, previewTestGroup(), map[string]string{})
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "command analysis failed")
	})
}

// TestDryRun_AnalysisUnavailableDenyPreview verifies when analysis or
// verification is unavailable, the command is a deny preview with an operational
// note (not an "unknown" status), and the exit code marks it distinctly.
func TestDryRun_AnalysisUnavailableDenyPreview(t *testing.T) {
	mgr := newPreviewManager(t, nil, keyedRiskEvaluator{
		"unverifiable": {
			Level:          runnertypes.RiskLevelLow,
			Blocking:       true,
			BlockingReason: risktypes.ReasonAnalysisDisabled,
			ReasonCodes:    []risktypes.ReasonCode{risktypes.ReasonAnalysisDisabled},
		},
	})
	cmd := executortestutil.CreateRuntimeCommand("/usr/bin/whatever", nil,
		executortestutil.WithName("unverifiable"), executortestutil.WithRiskLevel("high"))

	_, result, err := mgr.ExecuteCommand(context.Background(), cmd, previewTestGroup(), map[string]string{})
	require.NoError(t, err)
	assert.Contains(t, result.Analysis.Impact.Description, "DENY")
	assert.Contains(t, result.Analysis.Impact.Description, "verification unavailable")
	// By default a verification-unavailable deny is reported as a note and does not
	// fail the dry-run; the exit code stays 0.
	assert.Equal(t, DryRunExitAllow, mgr.PreviewExitCode())
}

// TestDryRun_VerificationUnavailableExitCode verifies the preview exit code
// distinguishes all-allow, policy deny, and verification-unavailable deny, and the
// FailOnVerificationUnavailable option escalates the latter to a hard failure.
func TestDryRun_VerificationUnavailableExitCode(t *testing.T) {
	policyDeny := risktypes.RiskAssessment{Level: runnertypes.RiskLevelHigh}
	verifUnavail := risktypes.RiskAssessment{
		Level:          runnertypes.RiskLevelLow,
		Blocking:       true,
		BlockingReason: risktypes.ReasonUncertainUnverifiedIdentity,
		ReasonCodes:    []risktypes.ReasonCode{risktypes.ReasonUncertainUnverifiedIdentity},
	}

	tests := []struct {
		name         string
		assessment   risktypes.RiskAssessment
		maxAllowed   string
		failOnVerif  bool
		expectedCode int
	}{
		{"all allow", risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow}, "high", false, DryRunExitAllow},
		{"policy deny", policyDeny, "low", false, DryRunExitPolicyDeny},
		{"verification unavailable not a failure by default", verifUnavail, "high", false, DryRunExitAllow},
		{"verification unavailable escalated to distinct code", verifUnavail, "high", true, DryRunExitVerificationUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &DryRunOptions{DetailLevel: DetailLevelDetailed, FailOnVerificationUnavailable: tt.failOnVerif}
			mgr := newPreviewManager(t, opts, keyedRiskEvaluator{"cmd": tt.assessment})
			cmd := executortestutil.CreateRuntimeCommand("/usr/bin/cmd", nil,
				executortestutil.WithName("cmd"), executortestutil.WithRiskLevel(tt.maxAllowed))
			_, _, err := mgr.ExecuteCommand(context.Background(), cmd, previewTestGroup(), map[string]string{})
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCode, mgr.PreviewExitCode())
		})
	}
}

// TestDryRun_PrivilegeEscalationDenied verifies that a privilege-escalation
// (Critical) command is surfaced as a deny preview in dry-run.
func TestDryRun_PrivilegeEscalationDenied(t *testing.T) {
	mgr := newPreviewManager(t, nil, keyedRiskEvaluator{
		"sudo-cmd": {
			Level:          runnertypes.RiskLevelCritical,
			BlockingReason: risktypes.ReasonPrivilegeEscalation,
			ReasonCodes:    []risktypes.ReasonCode{risktypes.ReasonPrivilegeEscalation},
		},
	})
	cmd := executortestutil.CreateRuntimeCommand("/usr/bin/sudo", []string{"rm", "-rf", "/"},
		executortestutil.WithName("sudo-cmd"), executortestutil.WithRiskLevel("high"))

	_, result, err := mgr.ExecuteCommand(context.Background(), cmd, previewTestGroup(), map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, "critical", result.Analysis.Impact.SecurityRisk)
	assert.Contains(t, result.Analysis.Impact.Description, "DENY")
	assert.Equal(t, DryRunExitPolicyDeny, mgr.PreviewExitCode())
}
