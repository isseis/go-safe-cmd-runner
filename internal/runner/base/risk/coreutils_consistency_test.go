//go:build test

package risk

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeConsistencyBinary creates an executable file in dir and returns its path.
func makeConsistencyBinary(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho test"), 0o755))
	return path
}

// TestCoreutilsRiskConsistency_RuntimeVsDryRun verifies that for the same
// coreutils command, the runtime path (StandardEvaluator.EvaluateRisk) and the
// dry-run path (security.AnalyzeCommandSecurity) produce the same final risk.
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

	evaluator := NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))

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
			name:     "chmod is medium",
			cmd:      filepath.Join(tmp, "chmod"),
			args:     []string{"+x", "file"},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name:     "cp overwrite is medium",
			cmd:      filepath.Join(tmp, "cp"),
			args:     []string{"a", "b"},
			expected: runnertypes.RiskLevelMedium,
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
			runtimeCmd := &runnertypes.RuntimeCommand{
				ExpandedCmd:  tt.cmd,
				ExpandedArgs: tt.args,
			}
			runtimeRisk, err := evaluator.EvaluateRisk(runtimeCmd)
			require.NoError(t, err)

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

	evaluator := NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))
	runtimeCmd := &runnertypes.RuntimeCommand{ExpandedCmd: path, ExpandedArgs: nil}
	runtimeRisk, err := evaluator.EvaluateRisk(runtimeCmd)
	require.NoError(t, err)

	dryRunRisk, _, _, err := security.AnalyzeCommandSecurity(path, nil, "")
	require.NoError(t, err)

	assert.Equal(t, runnertypes.RiskLevelHigh, runtimeRisk, "runtime risk")
	assert.Equal(t, runnertypes.RiskLevelHigh, dryRunRisk, "dry-run risk")
}
