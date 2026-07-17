package resource

import (
	"context"
	"testing"

	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
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
// note (not an "unknown" status), and the exit code is
// DryRunExitVerificationUnavailable (the flag is removed; the deny now always
// surfaces as a non-zero exit).
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
	// The flag is removed; a verification-unavailable deny always surfaces.
	assert.Equal(t, DryRunExitVerificationUnavailable, mgr.PreviewExitCode())
}

// TestDryRun_VerificationUnavailableExitCode verifies the preview exit code
// distinguishes all-allow, policy deny, and verification-unavailable deny.
// The FailOnVerificationUnavailable flag is removed; a verification-unavailable
// deny always maps to DryRunExitVerificationUnavailable.
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
		expectedCode int
	}{
		{"all allow", risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow}, "high", DryRunExitAllow},
		{"policy deny", policyDeny, "low", DryRunExitPolicyDeny},
		{"verification unavailable", verifUnavail, "high", DryRunExitVerificationUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
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

// unverifiedSummaryHashMismatch returns a FileVerificationSummary with a
// single verify_failed_hash_mismatch entry (the only non-nil Failure that
// is classified as a tampering signal).
func unverifiedSummaryHashMismatch(path, context string) *verification.FileVerificationSummary {
	f := verification.ReasonHashMismatch
	return &verification.FileVerificationSummary{
		TotalFiles:            1,
		VerifiedFiles:         0,
		FailedFiles:           1,
		UsedUnverifiedContent: true,
		UnverifiedFiles: []verification.UnverifiedFileUsage{
			{Path: path, Reason: string(verification.UnverifiedReasonFromFailure(verification.ReasonHashMismatch)), Context: context, Failure: &f},
		},
	}
}

// unverifiedSummaryNoValidator returns a FileVerificationSummary with a
// single skipped_no_validator entry (environment cause).
func unverifiedSummaryNoValidator(path, context string) *verification.FileVerificationSummary {
	return &verification.FileVerificationSummary{
		TotalFiles:            1,
		VerifiedFiles:         0,
		FailedFiles:           0,
		UsedUnverifiedContent: true,
		UnverifiedFiles: []verification.UnverifiedFileUsage{
			{Path: path, Reason: string(verification.UnverifiedReasonNoValidator), Context: context, Failure: nil},
		},
	}
}

// failuresOnlySummaryFromReason returns a FileVerificationSummary with a
// single Failure entry (no UnverifiedFiles). The caller passes the
// FailureReason so tests can cover a Failures-only scenario (e.g. verify_files).
func failuresOnlySummaryFromReason(path, context string, reason verification.FailureReason) *verification.FileVerificationSummary {
	return &verification.FileVerificationSummary{
		TotalFiles:    1,
		VerifiedFiles: 0,
		FailedFiles:   1,
		Failures: []verification.FileVerificationFailure{
			{Path: path, Reason: reason, Context: context},
		},
	}
}

// TestHasTamperingSignal verifies that hasTamperingSignal delegates to
// verification.IsTamperingSignal. Only hash_mismatch qualifies as a tampering
// signal; all other reasons (including hash_file_not_found, file_read_error,
// permission_denied, and skipped_no_validator) are environment causes.
func TestHasTamperingSignal(t *testing.T) {
	mismatch := verification.ReasonHashMismatch
	notFound := verification.ReasonHashFileNotFound

	tests := []struct {
		name string
		in   []verification.UnverifiedFileUsage
		want bool
	}{
		{"nil usages", nil, false},
		{"empty usages", []verification.UnverifiedFileUsage{}, false},
		{"only environment cause (nil Failure)", []verification.UnverifiedFileUsage{{Path: "/etc/a.toml"}}, false},
		{"environment cause hash_file_not_found", []verification.UnverifiedFileUsage{{Path: "/etc/a.toml", Failure: &notFound}}, false},
		{"one tampering signal", []verification.UnverifiedFileUsage{{Path: "/etc/a.toml", Failure: &mismatch}}, true},
		{
			"environment cause mixed with a genuine tampering signal",
			[]verification.UnverifiedFileUsage{
				{Path: "/etc/env.toml", Failure: &notFound},
				{Path: "/etc/tampered.toml", Failure: &mismatch},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasTamperingSignal(tt.in))
		})
	}
}

// TestDryRun_UnverifiedContentExitCode verifies the dry-run preview exit code
// maps unverified content (UnverifiedFiles) and verification failures
// (Failures) to distinct exit codes: tampering signal (hash_mismatch) -> 1,
// environment cause only -> 3. The flag is removed so the behavior is always
// active.
func TestDryRun_UnverifiedContentExitCode(t *testing.T) {
	tests := []struct {
		name        string
		assessment  risktypes.RiskAssessment
		summary     *verification.FileVerificationSummary
		expected    int
		description string
	}{
		{
			name:        "no unverified content, all allow",
			assessment:  risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
			summary:     nil,
			expected:    DryRunExitAllow,
			description: "clean run stays exit 0",
		},
		{
			name:        "environment cause unverified",
			assessment:  risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
			summary:     unverifiedSummaryNoValidator("/etc/app/cfg.toml", "config"),
			expected:    DryRunExitVerificationUnavailable,
			description: "skipped_no_validator -> exit 3",
		},
		{
			name:        "tampering signal unverified",
			assessment:  risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
			summary:     unverifiedSummaryHashMismatch("/etc/app/cfg.toml", "config"),
			expected:    DryRunExitPolicyDeny,
			description: "hash_mismatch -> exit 1",
		},
		{
			name:       "mixed unverified, tampering dominates",
			assessment: risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
			summary: mergeUnverifiedSummaries(
				unverifiedSummaryNoValidator("/etc/app/cfg.toml", "config"),
				unverifiedSummaryHashMismatch("/etc/app/tmpl.toml", "template"),
			),
			expected:    DryRunExitPolicyDeny,
			description: "when tampering and environment coexist, tampering wins",
		},
		{
			name:        "policy deny dominates unverified tampering",
			assessment:  risktypes.RiskAssessment{Level: runnertypes.RiskLevelHigh},
			summary:     unverifiedSummaryHashMismatch("/etc/app/cfg.toml", "config"),
			expected:    DryRunExitPolicyDeny,
			description: "policy deny is recorded first; tampering would also map to exit 1",
		},
		{
			name:        "verify_files hash_mismatch alone",
			assessment:  risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
			summary:     failuresOnlySummaryFromReason("/usr/bin/suspicious", "global", verification.ReasonHashMismatch),
			expected:    DryRunExitPolicyDeny,
			description: "Failures-only hash_mismatch -> exit 1",
		},
		{
			name:        "verify_files hash_file_not_found alone",
			assessment:  risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
			summary:     failuresOnlySummaryFromReason("/usr/bin/tool", "global", verification.ReasonHashFileNotFound),
			expected:    DryRunExitVerificationUnavailable,
			description: "Failures-only environment cause -> exit 3",
		},
		{
			name:       "verify_files hash_mismatch mixed with environment-cause unverified content",
			assessment: risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
			summary: mergeMixedSummaries(
				unverifiedSummaryNoValidator("/etc/app/cfg.toml", "config"),
				failuresOnlySummaryFromReason("/usr/bin/suspicious", "global", verification.ReasonHashMismatch),
			),
			expected:    DryRunExitPolicyDeny,
			description: "Failures tampering mixed with UnverifiedFiles environment cause -> tampering wins",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
			mgr := newPreviewManager(t, opts, keyedRiskEvaluator{"cmd": tt.assessment})
			cmd := executortestutil.CreateRuntimeCommand("/usr/bin/cmd", nil,
				executortestutil.WithName("cmd"), executortestutil.WithRiskLevel("high"))
			_, _, err := mgr.ExecuteCommand(context.Background(), cmd, previewTestGroup(), map[string]string{})
			require.NoError(t, err)
			mgr.SetFileVerification(tt.summary)
			assert.Equal(t, tt.expected, mgr.PreviewExitCode(), tt.description)
		})
	}
}

// TestDryRun_SetFileVerificationNilClears verifies SetFileVerification(nil)
// restores the manager to its environment-only behavior, so a runner that
// defers verification does not pin a stale summary.
func TestDryRun_SetFileVerificationNilClears(t *testing.T) {
	opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
	mgr := newPreviewManager(t, opts, keyedRiskEvaluator{"cmd": {Level: runnertypes.RiskLevelLow}})

	mgr.SetFileVerification(unverifiedSummaryHashMismatch("/etc/app/cfg.toml", "config"))
	assert.Equal(t, DryRunExitPolicyDeny, mgr.PreviewExitCode(),
		"sanity: tampering signal -> exit 1")

	mgr.SetFileVerification(nil)
	assert.Equal(t, DryRunExitAllow, mgr.PreviewExitCode(),
		"clearing the summary should drop the tampering signal exit code")
}

// mergeUnverifiedSummaries combines two FileVerificationSummary values into a
// single one. The Total/Failed counts and HashDirStatus are not relevant to
// the exit-code decision, so the implementation only copies the
// UnverifiedFiles slice.
func mergeUnverifiedSummaries(a, b *verification.FileVerificationSummary) *verification.FileVerificationSummary {
	merged := *a
	merged.UnverifiedFiles = append([]verification.UnverifiedFileUsage{}, a.UnverifiedFiles...)
	merged.UnverifiedFiles = append(merged.UnverifiedFiles, b.UnverifiedFiles...)
	merged.UsedUnverifiedContent = true
	return &merged
}

// mergeMixedSummaries combines an UnverifiedFiles-only summary (a) with a
// Failures-only summary (b) into a single summary that has both, used by the
// verify_files-hash_mismatch-mixed-with-unverified test case.
func mergeMixedSummaries(a, b *verification.FileVerificationSummary) *verification.FileVerificationSummary {
	merged := *a
	merged.Failures = append([]verification.FileVerificationFailure{}, b.Failures...)
	merged.FailedFiles = len(merged.Failures)
	return &merged
}
