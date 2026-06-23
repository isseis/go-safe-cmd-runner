//go:build test

package risk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zoningForeignIdent is a run-as identity that does not own the test fixtures, so
// a safe-zone whose ancestors are owned by the real euid is Trusted (Low). It
// differs from the live euid, demonstrating the judgment uses the injected
// identity, not os.Geteuid.
func zoningForeignIdent() risktypes.RunAsIdent {
	return risktypes.RunAsIdent{UID: uint32(os.Geteuid()) + 1, GID: uint32(os.Getgid()) + 1}
}

func evalAssessInDir(t *testing.T, ev Evaluator, cmd string, args []string, workdir string) risktypes.RiskAssessment {
	t.Helper()
	plan, err := ev.EvaluateRisk(verifiedCmdInDir(cmd, args, workdir))
	require.NoError(t, err)
	return plan.Assessment
}

func evalLevelInDir(t *testing.T, ev Evaluator, cmd string, args []string, workdir string) runnertypes.RiskLevel {
	t.Helper()
	a := evalAssessInDir(t, ev, cmd, args, workdir)
	assert.False(t, a.Blocking, "command %q must not be Blocking", cmd)
	return a.Level
}

// TestAxis2ReplacesLegacyHigh: a fully recognized file operation is classified by
// its destination zone, replacing the legacy fixed-High destructive dimensions;
// an unrecognized form keeps the legacy High (fail-open avoidance).
func TestAxis2ReplacesLegacyHigh(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "build"), 0o700))
	ev := newZoningEvaluator(wd, zoningForeignIdent())

	assert.Equal(t, runnertypes.RiskLevelLow,
		evalLevelInDir(t, ev, "rm", []string{"-rf", filepath.Join(wd, "build")}, wd),
		"recursive delete confined to a Trusted safe-zone is Low")

	assert.Equal(t, runnertypes.RiskLevelMedium,
		evalLevelInDir(t, ev, "rm", []string{"/srv/app/cache.dat"}, wd),
		"delete of an ordinary path is Medium")

	assert.Equal(t, runnertypes.RiskLevelHigh,
		evalLevelInDir(t, ev, "rm", []string{"--zonk", "/srv/x"}, wd),
		"an unknown flag leaves the command unrecognized, so legacy High is retained")
}

// TestAxis1Axis2MaxComposition: the final risk is the max of axis 1 and axis 2; a
// copy into a trust-critical path is High.
func TestAxis1Axis2MaxComposition(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	src := filepath.Join(wd, "payload")
	require.NoError(t, os.WriteFile(src, nil, 0o644))
	ev := newZoningEvaluator(wd, zoningForeignIdent())

	assert.Equal(t, runnertypes.RiskLevelHigh,
		evalLevelInDir(t, ev, "cp", []string{"-a", src, "/usr/bin"}, wd),
		"a copy into the trust-critical /usr/bin is High")
}

// TestAxis2RecuperatesSuppressedHigh: the legacy dimensions removed on full
// recognition are re-established by axis 2's zone and operation-specific floors,
// so no dangerous form is downgraded (no fail-open gap).
func TestAxis2RecuperatesSuppressedHigh(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	ev := newZoningEvaluator(wd, zoningForeignIdent())

	// chmod 0777 in a safe-zone is still High (world-writable grant floor), even
	// though the destination zone alone would be Low.
	assert.Equal(t, runnertypes.RiskLevelHigh,
		evalLevelInDir(t, ev, "chmod", []string{"0777", filepath.Join(wd, "x")}, wd),
		"world-writable grant is High even in a safe-zone")

	// chown root onto a trust-critical path is High; onto an ordinary path Medium.
	assert.Equal(t, runnertypes.RiskLevelHigh,
		evalLevelInDir(t, ev, "chown", []string{"root", "/usr/bin/x"}, wd),
		"ownership change on a trust-critical path is High")
	assert.Equal(t, runnertypes.RiskLevelMedium,
		evalLevelInDir(t, ev, "chown", []string{"root", "/srv/app/x"}, wd),
		"ownership change on an ordinary path is Medium")
}

// TestAxis2NonFileOpUnaffected: a command that is not a file operation classifies
// identically whether or not axis-2 zoning is enabled (suppressLegacy is false).
func TestAxis2NonFileOpUnaffected(t *testing.T) {
	wd := t.TempDir()
	legacy := newVerifiedEvaluator()
	zoned := newZoningEvaluator(wd, zoningForeignIdent())

	cases := []struct {
		cmd  string
		args []string
	}{
		{"systemctl", []string{"restart", "nginx"}},
		{"nc", []string{"-l", "1234"}},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			want := evalLevel(t, legacy, tc.cmd, tc.args)
			got := evalLevelInDir(t, zoned, tc.cmd, tc.args, wd)
			assert.Equal(t, want, got, "zoning must not change a non-file-operation command")
		})
	}
}

// TestOperandZonesStored: the per-operand audit records are carried on the
// assessment for a file operation, and absent for a non-file-operation command.
func TestOperandZonesStored(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	ev := newZoningEvaluator(wd, zoningForeignIdent())

	a := evalAssessInDir(t, ev, "cp", []string{filepath.Join(wd, "src"), "/usr/bin/ls"}, wd)
	require.NotEmpty(t, a.OperandZones, "a file operation carries per-operand audit records")
	var dest risktypes.OperandZone
	for _, oz := range a.OperandZones {
		if oz.Role == risktypes.OperandRoleWrite {
			dest = oz
		}
	}
	assert.Equal(t, "/usr/bin/ls", dest.Raw, "the raw operand is recorded verbatim")
	// Resolved may differ from Raw when /usr/bin/ls is a symlink (e.g. rust
	// coreutils), but it stays under /usr, so the zone is trust-critical.
	assert.Equal(t, risktypes.ZoneTrustCritical, dest.Zone)
	assert.NotEmpty(t, dest.Resolved)

	// A non-file-operation command does not apply axis 2: empty carrier.
	nb := evalAssessInDir(t, ev, "systemctl", []string{"status", "nginx"}, wd)
	assert.Empty(t, nb.OperandZones, "axis 2 did not apply -> empty carrier")
}
