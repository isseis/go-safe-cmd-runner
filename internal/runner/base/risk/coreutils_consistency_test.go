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
// The dry-run resource manager evaluates risk with the same
// StandardEvaluator.EvaluateRisk that normal mode uses, so the effective risk a
// command receives is identical in both modes by construction (single source).
// The tests below pin that shared effective risk for representative command
// classes, guarding against regressions in the shared evaluator.

// TestCoreutilsRiskConsistency_RuntimeVsDryRun pins the effective risk the shared
// evaluator assigns to coreutils commands (normal and dry-run both use it).
//
// The case set deliberately includes destructive commands with no args or
// minimal args (rm, shred, truncate, dd, unlink). For these the name/argument
// dimensions do not fire (IsDestructiveFileOperation matches the resolved full
// path rather than a basename, and the dangerous-argument patterns only react to
// specific forms such as "rm -rf" or "dd if="). Only the High set in
// CoreutilsCommandRisk guarantees High, so these cases verify the
// destructive-command guarantee mechanically without relying on other dimensions.
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
			// Both normal and dry-run modes surface this exact effective risk.
			assert.Equal(t, tt.expected, plan.Assessment.Level)
		})
	}
}

// TestCoreutilsRiskConsistency_Setuid verifies that a setuid coreutils binary
// (even with a safe name and a safe-zone destination that would otherwise be Low)
// is High: when axis 2 fully recognizes the file operation it suppresses the
// legacy destructive dimensions but re-establishes the setuid/setgid-binary signal
// from the existing lstat signal.
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

	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	evaluator := newZoningEvaluator(wd, zoningForeignIdent())
	// mkdir into a Trusted safe-zone would be Low on its destination alone; the
	// setuid bit on the binary keeps it High.
	plan, err := evaluator.EvaluateRisk(verifiedCmdInDir(path, []string{filepath.Join(wd, "d")}, wd))
	require.NoError(t, err)
	assert.Equal(t, runnertypes.RiskLevelHigh, plan.Assessment.Level)
}

// TestConsistency_DestructiveAbsolutePath verifies that, with axis-2 zoning, a
// destructive command given by absolute path is classified by its destination:
// a trust-critical destination is High, a Trusted safe-zone destination is Low
// (the legacy unconditional-High classification is replaced).
//
// Overriding coreutilsDir forbids t.Parallel().
func TestConsistency_DestructiveAbsolutePath(t *testing.T) {
	tmp := t.TempDir()
	security.SetCoreutilsDirForTest(t, tmp)
	rm := makeConsistencyBinary(t, tmp, "rm")

	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "build"), 0o700))
	ev := newZoningEvaluator(wd, zoningForeignIdent())

	high, err := ev.EvaluateRisk(verifiedCmdInDir(rm, []string{"-rf", "/usr/local/lib/x"}, wd))
	require.NoError(t, err)
	assert.Equal(t, runnertypes.RiskLevelHigh, high.Assessment.Level, "trust-critical destination")

	low, err := ev.EvaluateRisk(verifiedCmdInDir(rm, []string{"-rf", filepath.Join(wd, "build")}, wd))
	require.NoError(t, err)
	assert.Equal(t, runnertypes.RiskLevelLow, low.Assessment.Level, "Trusted safe-zone destination")
}

// TestConsistency_RmAllForms verifies rm classification through the shared
// evaluator across invocation forms. A trust-critical destination is High in
// every form. A Trusted safe-zone destination is Low for the direct forms
// (basename, absolute coreutils path), which axis 2 recognizes as rm; the
// coreutils multicall entrypoint ("coreutils rm ...") is named "coreutils", which
// axis 2 does not destructure into an rm operation, so it stays conservatively
// High via the retained legacy classification (fail-closed, not fail-open).
//
// Overriding coreutilsDir forbids t.Parallel().
func TestConsistency_RmAllForms(t *testing.T) {
	tmp := t.TempDir()
	security.SetCoreutilsDirForTest(t, tmp)
	rm := makeConsistencyBinary(t, tmp, "rm")
	coreutils := makeConsistencyBinary(t, tmp, "coreutils")

	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "build"), 0o700))
	ev := newZoningEvaluator(wd, zoningForeignIdent())

	critical := "/usr/local/lib/x"
	safe := filepath.Join(wd, "build")
	tests := []struct {
		name     string
		cmd      string
		crit     []string
		low      []string
		safeWant runnertypes.RiskLevel
	}{
		{"basename", "rm", []string{"-rf", critical}, []string{"-rf", safe}, runnertypes.RiskLevelLow},
		{"absolute coreutils", rm, []string{"-rf", critical}, []string{"-rf", safe}, runnertypes.RiskLevelLow},
		{"multicall", coreutils, []string{"rm", "-rf", critical}, []string{"rm", "-rf", safe}, runnertypes.RiskLevelHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			high, err := ev.EvaluateRisk(verifiedCmdInDir(tt.cmd, tt.crit, wd))
			require.NoError(t, err)
			assert.Equal(t, runnertypes.RiskLevelHigh, high.Assessment.Level, "trust-critical")

			low, err := ev.EvaluateRisk(verifiedCmdInDir(tt.cmd, tt.low, wd))
			require.NoError(t, err)
			assert.Equal(t, tt.safeWant, low.Assessment.Level, "safe-zone")
		})
	}
}

// TestConsistency_Systemctl verifies the shared evaluator (runtime and
// dry-run) classifies systemctl and package managers as High regardless of the
// subcommand, including read-only verbs (systemctl status) and queries (apt list).
// Both modes call the same EvaluateRisk, so a single assertion fixes both.
func TestConsistency_Systemctl(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"restart", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/sbin/systemctl", []string{"stop", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"status", "nginx"}))
	// Package managers are the new High additions; include one to exercise the
	// shared-evaluator consistency for them too.
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "apt", []string{"list", "--installed"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/bin/dpkg", []string{"-i", "pkg.deb"}))
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
