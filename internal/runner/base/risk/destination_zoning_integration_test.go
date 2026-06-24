//go:build test

package risk

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
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

// TestDataTransferWriteComposition: the final risk of a data-transfer command is
// the max of its name-based egress Medium and its write destination's zone.
func TestDataTransferWriteComposition(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	ev := newZoningEvaluator(wd, zoningForeignIdent())

	// (i) Download into a safe-zone: the write destination is Low, so the Medium
	// comes only from the name-based egress (curl's network profile). Asserting the
	// reason code avoids the canary trap of matching the level alone.
	safe := evalAssessInDir(t, ev, "curl", []string{"http://example.com/f", "-o", filepath.Join(wd, "safe")}, wd)
	assert.Equal(t, runnertypes.RiskLevelMedium, safe.Level)
	assert.Contains(t, safe.ReasonCodes, risktypes.ReasonProfileNetwork,
		"the Medium must come from the name-based egress, not only the level")

	// (ii) Download into a trust-critical path: the write destination dominates.
	assert.Equal(t, runnertypes.RiskLevelHigh,
		evalLevelInDir(t, ev, "curl", []string{"http://example.com/f", "-o", "/usr/bin/x"}, wd),
		"a trust-critical write destination is High")

	// rsync to a daemon bare module (host::module) is remote egress -> Medium, sourced
	// from axis-2's network-egress floor (the global network-arg check misses a bare
	// module).
	mod := evalAssessInDir(t, ev, "rsync", []string{filepath.Join(wd, "src"), "host::module"}, wd)
	assert.Equal(t, runnertypes.RiskLevelMedium, mod.Level)
	assert.Contains(t, mod.ReasonCodes, risktypes.ReasonNetworkArgument)

	// A purely local rsync into a safe-zone is not over-classified (no false egress).
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "a"), 0o700))
	assert.Equal(t, runnertypes.RiskLevelLow,
		evalLevelInDir(t, ev, "rsync", []string{filepath.Join(wd, "a"), filepath.Join(wd, "b")}, wd),
		"local rsync into a safe-zone stays Low")

	// A local rsync into a trust-critical destination is High (write zone dominates).
	assert.Equal(t, runnertypes.RiskLevelHigh,
		evalLevelInDir(t, ev, "rsync", []string{filepath.Join(wd, "a"), "/usr/bin/x"}, wd),
		"local rsync into a trust-critical destination is High")
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

// TestConfigWiredEndToEnd: an evaluator built through the production
// NewStandardEvaluator(networkAnalyzer, securityConfig) path classifies file
// operations by zone, proving the security config's SystemCriticalPaths reach the
// axis-2 judgment (not only the directly-injected zoningParams used by other
// tests). With no run_as set, the default identity is the original execution
// identity, so a trust-critical write is High regardless of the live euid.
func TestConfigWiredEndToEnd(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	cfg := security.DefaultConfig()
	cfg.TrustedDirectories = []string{wd}
	ev := newConfigEvaluator(cfg)

	assert.Equal(t, runnertypes.RiskLevelHigh,
		evalLevelInDir(t, ev, "touch", []string{"/usr/bin/x"}, wd),
		"a system-critical write from config is High")
	assert.Equal(t, runnertypes.RiskLevelMedium,
		evalLevelInDir(t, ev, "touch", []string{"/srv/app/x"}, wd),
		"an ordinary write is Medium")
}

// evalLevelRunAs evaluates a command whose run_as_user is set, so the evaluator
// resolves a per-command run-as identity via its (injectable) resolveRunAs.
func evalLevelRunAs(t *testing.T, ev Evaluator, cmd string, args []string, workdir, runAsUser string) runnertypes.RiskLevel {
	t.Helper()
	plan, err := ev.EvaluateRisk(verifiedCmdRunAs(cmd, args, workdir, runAsUser))
	require.NoError(t, err)
	assert.False(t, plan.Assessment.Blocking, "command %q must not be Blocking", cmd)
	return plan.Assessment.Level
}

// TestRunAsIdentDifferential: the safe-zone Trusted predicate follows the
// per-command run-as identity resolved at dispatch, NOT the live euid. With the
// SAME live euid across both evaluations, a foreign identity (which does not own
// the safe-zone's ancestors) yields Trusted/Low, while the real owner identity
// yields non-Trusted/Medium. This also proves TrustedDirectories from config
// reaches the judgment (Low requires the destination be within a trusted dir).
func TestRunAsIdentDifferential(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	cfg := security.DefaultConfig()
	cfg.TrustedDirectories = []string{wd}
	ev := newConfigEvaluator(cfg)

	foreign := risktypes.RunAsIdent{UID: uint32(os.Geteuid()) + 1, GID: uint32(os.Getgid()) + 1}
	self := risktypes.RunAsIdent{UID: uint32(os.Geteuid()), GID: uint32(os.Getgid())}

	ev.resolveRunAs = func(_ risktypes.RunAsIdent, _, _ string) (risktypes.RunAsIdent, error) { return foreign, nil }
	assert.Equal(t, runnertypes.RiskLevelLow,
		evalLevelRunAs(t, ev, "touch", []string{filepath.Join(wd, "x")}, wd, "someuser"),
		"a foreign run-as that cannot reach the safe-zone's ancestors is Trusted -> Low")

	ev.resolveRunAs = func(_ risktypes.RunAsIdent, _, _ string) (risktypes.RunAsIdent, error) { return self, nil }
	assert.Equal(t, runnertypes.RiskLevelMedium,
		evalLevelRunAs(t, ev, "touch", []string{filepath.Join(wd, "x")}, wd, "someuser"),
		"the safe-zone owner is not Trusted -> Medium (verdict followed the injected identity, not the live euid)")
}

// TestRunAsResolutionFailsClosed: when a command's run-as name cannot be resolved,
// the judgment fails closed -- every operand is treated as unresolved, so a write
// is High rather than trusting an unknown identity.
func TestRunAsResolutionFailsClosed(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	cfg := security.DefaultConfig()
	cfg.TrustedDirectories = []string{wd}
	ev := newConfigEvaluator(cfg)

	ev.resolveRunAs = func(_ risktypes.RunAsIdent, _, _ string) (risktypes.RunAsIdent, error) {
		return risktypes.RunAsIdent{}, errors.New("unknown user")
	}
	assert.Equal(t, runnertypes.RiskLevelHigh,
		evalLevelRunAs(t, ev, "touch", []string{filepath.Join(wd, "x")}, wd, "ghost"),
		"an unresolved run-as identity fails closed -> the write is High")
}

// TestDeterminismRuntimeEqualsDryRun: the zoning judgment is a pure function of the
// command, the filesystem state, and the injected config/identity, so two evaluators
// built from the same config return the same risk level, reason codes, and
// per-operand zones. The runtime and dry-run paths are equivalent structurally
// rather than by this test: both build the evaluator through the single
// resolveRiskEvaluator(securityConfig) call site, and P6's TestConfigWiredEndToEnd
// covers that the config reaches the judgment. Independence from the live euid is
// covered by TestRunAsIdentDifferential; independence from the environment ($HOME)
// is enforced statically by TestNoLiveIdentityInZoning (the zoning code reads no env
// getter). This test pins the remaining property: determinism across constructions.
func TestDeterminismRuntimeEqualsDryRun(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "work")
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "build"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(wd, "src"), nil, 0o644))
	cfg := security.DefaultConfig()
	cfg.TrustedDirectories = []string{wd}

	cases := []struct {
		cmd  string
		args []string
	}{
		{"rm", []string{"-rf", filepath.Join(wd, "build")}},                        // safe-zone Low
		{"cp", []string{"-a", filepath.Join(wd, "src"), "/usr/bin"}},               // trust-critical High
		{"touch", []string{"/srv/app/x"}},                                          // ordinary Medium
		{"curl", []string{"http://example.com/f", "-o", filepath.Join(wd, "out")}}, // egress Medium
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			// Construct fresh evaluators per case so no accumulated state can let the
			// two stay in lockstep; each assertion compares independent constructions.
			runtimeEv := newConfigEvaluator(cfg)
			dryRunEv := newConfigEvaluator(cfg)
			r, err := runtimeEv.EvaluateRisk(verifiedCmdInDir(tc.cmd, tc.args, wd))
			require.NoError(t, err)
			d, err := dryRunEv.EvaluateRisk(verifiedCmdInDir(tc.cmd, tc.args, wd))
			require.NoError(t, err)
			assert.Equal(t, r.Assessment.Level, d.Assessment.Level, "level must be identical on both paths")
			assert.Equal(t, r.Assessment.ReasonCodes, d.Assessment.ReasonCodes, "reason codes must be identical")
			assert.Equal(t, r.Assessment.OperandZones, d.Assessment.OperandZones, "per-operand zones must be identical")
		})
	}
}
