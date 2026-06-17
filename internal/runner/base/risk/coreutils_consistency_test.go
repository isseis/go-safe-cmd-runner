//go:build test

package risk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeConsistencyBinary creates an executable file in dir and returns its path.
// The content begins with the ELF magic (not a "#!" shebang) so it is treated as
// a real binary; a shebang would be classified as an indirect script execution.
func makeConsistencyBinary(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("\x7fELF\x02\x01\x01\x00"), 0o755))
	return path
}

// Runtime/dry-run risk consistency.
//
// Since PR-5 the dry-run resource manager evaluates risk with the same
// StandardEvaluator.EvaluateRisk that normal mode uses, so the effective risk a
// command receives is identical in both modes by construction (single source).
// The tests below pin that shared effective risk for the command classes that
// historically diverged between the two paths, guarding against regressions in the
// shared evaluator. For coreutils/destructive commands they additionally
// cross-check security.AnalyzeCommandSecurity (the retained reference
// implementation) which still agrees; for the classes the old reference got wrong
// (systemctl levels, profile factors) only the evaluator's value is asserted.

// TestCoreutilsRiskConsistency_RuntimeVsDryRun verifies that for the same
// coreutils command, the evaluator (StandardEvaluator.EvaluateRisk) and the
// retained reference (security.AnalyzeCommandSecurity) produce the same final
// risk.
//
// This test lives in the risk package because it imports both risk and
// security; the dependency direction is risk -> security only, so it cannot
// live in the security package.
//
// The case set deliberately includes destructive commands with no args or
// minimal args (rm, shred, truncate, dd, unlink). For these, the earlier
// pre-steps do not fire (IsDestructiveFileOperation matches the resolved full
// path rather than a basename, and the dry-run high-risk patterns only react to
// specific argument forms such as "rm -rf" or "dd if="). Only the High set in
// CoreutilsCommandRisk guarantees High in both paths, so these cases verify the
// destructive-command guarantee mechanically without relying on pre-step
// behavior.
//
// Overriding coreutilsDir forbids t.Parallel().
func TestCoreutilsRiskConsistency_RuntimeVsDryRun(t *testing.T) {
	tmp := t.TempDir()
	security.SetCoreutilsDirForTest(t, tmp)

	for _, name := range []string{"mkdir", "chmod", "cp", "rm", "shred", "truncate", "dd", "unlink", "coreutils"} {
		makeConsistencyBinary(t, tmp, name)
	}

	evaluator := newVerifiedEvaluator()

	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected runnertypes.RiskLevel
	}{
		{
			name:     "mkdir no args is low",
			cmd:      filepath.Join(tmp, "mkdir"),
			args:     nil,
			expected: runnertypes.RiskLevelLow,
		},
		{
			// chmod is not in the safe set, so the coreutils step fails safe to High.
			name:     "chmod is high",
			cmd:      filepath.Join(tmp, "chmod"),
			args:     []string{"+x", "file"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "cp overwrite is high",
			cmd:      filepath.Join(tmp, "cp"),
			args:     []string{"a", "b"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "rm recursive is high",
			cmd:      filepath.Join(tmp, "rm"),
			args:     []string{"-rf", "/tmp/x"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			// No "-rf": pre-steps do not fire; the High set guarantees High.
			name:     "rm no args is high",
			cmd:      filepath.Join(tmp, "rm"),
			args:     nil,
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "shred file is high",
			cmd:      filepath.Join(tmp, "shred"),
			args:     []string{"file"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "truncate is high",
			cmd:      filepath.Join(tmp, "truncate"),
			args:     []string{"-s", "0", "file"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			// No "if=": the dry-run dd pattern does not fire; the High set guarantees High.
			name:     "dd no args is high",
			cmd:      filepath.Join(tmp, "dd"),
			args:     nil,
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "unlink is high",
			cmd:      filepath.Join(tmp, "unlink"),
			args:     []string{"x"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "multicall entrypoint rm is high",
			cmd:      filepath.Join(tmp, "coreutils"),
			args:     []string{"rm", "-rf", "/tmp/x"},
			expected: runnertypes.RiskLevelHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := evaluator.EvaluateRisk(verifiedCmd(tt.cmd, tt.args))
			require.NoError(t, err)
			runtimeRisk := plan.Assessment.Level

			// hashDir is "" to skip hash validation in the dry-run path.
			dryRunRisk, _, _, err := security.AnalyzeCommandSecurity(tt.cmd, tt.args, "")
			require.NoError(t, err)

			assert.Equal(t, tt.expected, runtimeRisk, "runtime risk")
			assert.Equal(t, tt.expected, dryRunRisk, "dry-run risk")
			assert.Equal(t, runtimeRisk, dryRunRisk, "runtime and dry-run must agree")
		})
	}
}

// TestCoreutilsRiskConsistency_Setuid verifies that a setuid coreutils binary
// (even with a safe name) is High in both paths. In the dry-run path the
// invariant is that the existing setuid step (Step 6) runs before the coreutils
// step and already returns High; in the runtime path the setuid check is the
// first thing CoreutilsCommandRisk does. Both must yield High.
//
// This is a separate test because the setuid bit may be silently ignored by the
// OS (non-root on macOS), in which case the test is skipped.
//
// Overriding coreutilsDir forbids t.Parallel().
func TestCoreutilsRiskConsistency_Setuid(t *testing.T) {
	tmp := t.TempDir()
	security.SetCoreutilsDirForTest(t, tmp)

	path := makeConsistencyBinary(t, tmp, "mkdir")
	require.NoError(t, os.Chmod(path, 0o755|os.ModeSetuid))

	info, err := os.Stat(path)
	require.NoError(t, err)
	if info.Mode()&os.ModeSetuid == 0 {
		t.Skip("Skipping: OS silently ignored setuid bit (non-root on macOS)")
	}

	evaluator := newVerifiedEvaluator()
	plan, err := evaluator.EvaluateRisk(verifiedCmd(path, nil))
	require.NoError(t, err)
	runtimeRisk := plan.Assessment.Level

	dryRunRisk, _, dryRunReason, err := security.AnalyzeCommandSecurity(path, nil, "")
	require.NoError(t, err)

	assert.Equal(t, runnertypes.RiskLevelHigh, runtimeRisk, "runtime risk")
	assert.Equal(t, runnertypes.RiskLevelHigh, dryRunRisk, "dry-run risk")

	// Assert provenance, not just the value: the dry-run High must come from the
	// setuid step (Step 6), which runs before the coreutils step (Step 7). Since
	// CoreutilsCommandRisk also returns High for a setuid binary, checking only
	// the risk level would not detect a regression that moved the coreutils step
	// ahead of Step 6. The reason string distinguishes the two: Step 6 returns
	// "Executable has setuid or setgid bit set" whereas the coreutils step
	// returns "Coreutils command risk classification".
	const setuidStepReason = "Executable has setuid or setgid bit set"
	assert.Equal(t, setuidStepReason, dryRunReason,
		"dry-run High must be produced by the setuid step (Step 6), before the coreutils step")
}

// TestConsistency_DestructiveAbsolutePath verifies a destructive command
// given by absolute path is High via the shared evaluator (used by both runtime
// and dry-run), and the retained reference agrees.
//
// Overriding coreutilsDir forbids t.Parallel().
func TestConsistency_DestructiveAbsolutePath(t *testing.T) {
	tmp := t.TempDir()
	security.SetCoreutilsDirForTest(t, tmp)
	rm := makeConsistencyBinary(t, tmp, "rm")

	ev := newVerifiedEvaluator()
	plan, err := ev.EvaluateRisk(verifiedCmd(rm, []string{"-rf", "/tmp/x"}))
	require.NoError(t, err)
	assert.Equal(t, runnertypes.RiskLevelHigh, plan.Assessment.Level, "evaluator (runtime and dry-run)")

	acsRisk, _, _, err := security.AnalyzeCommandSecurity(rm, []string{"-rf", "/tmp/x"}, "")
	require.NoError(t, err)
	assert.Equal(t, runnertypes.RiskLevelHigh, acsRisk, "retained reference")
}

// TestConsistency_RmAllForms verifies rm reaches High through the shared
// evaluator whether invoked by basename, absolute coreutils path, or coreutils
// multicall entrypoint.
//
// Overriding coreutilsDir forbids t.Parallel().
func TestConsistency_RmAllForms(t *testing.T) {
	tmp := t.TempDir()
	security.SetCoreutilsDirForTest(t, tmp)
	rm := makeConsistencyBinary(t, tmp, "rm")
	coreutils := makeConsistencyBinary(t, tmp, "coreutils")

	ev := newVerifiedEvaluator()
	tests := []struct {
		name string
		cmd  string
		args []string
	}{
		{"basename", "rm", []string{"-rf", "/tmp/x"}},         // destructive-name dimension
		{"absolute coreutils", rm, []string{"-rf", "/tmp/x"}}, // coreutils classification
		{"multicall", coreutils, []string{"rm", "-rf", "/tmp/x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ev.EvaluateRisk(verifiedCmd(tt.cmd, tt.args))
			require.NoError(t, err)
			assert.Equal(t, runnertypes.RiskLevelHigh, plan.Assessment.Level)
		})
	}
}

// TestConsistency_Systemctl verifies the shared evaluator (runtime and
// dry-run) classifies systemctl change verbs as High and read-only verbs at a
// Medium floor.
func TestConsistency_Systemctl(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"restart", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/sbin/systemctl", []string{"stop", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelMedium, evalLevel(t, ev, "systemctl", []string{"status", "nginx"}))
}

// TestConsistency_ProfileCommands verifies profile-derived risk (claude,
// curl) is identical for runtime and dry-run because both use the shared
// evaluator.
func TestConsistency_ProfileCommands(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "claude", []string{"--help"}))
	assert.Equal(t, runnertypes.RiskLevelMedium, evalLevel(t, ev, "curl", []string{"https://example.com"}))
}

// TestConsistency_UncertainCases verifies an uncertain binary (missing
// analysis record) is a Blocking deny under the shared evaluator, so runtime and
// dry-run both abort it identically.
func TestConsistency_UncertainCases(t *testing.T) {
	path := absCmd("mystery-tool")
	ev := newEvaluatorWithStore(fakeRecordStore{errs: map[string]error{path: fileErrNotFound()}})

	plan, err := ev.EvaluateRisk(verifiedCmd("mystery-tool", nil))
	require.NoError(t, err)
	assert.True(t, plan.Assessment.Blocking, "uncertain binary must be Blocking")
	assert.Equal(t, risktypes.ReasonUncertainMissingRecord, plan.Assessment.BlockingReason)
}
