//go:build test

package risk

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStandardEvaluator(t *testing.T) {
	evaluator := NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))
	require.NotNil(t, evaluator)
	assert.IsType(t, &StandardEvaluator{}, evaluator)
}

// evalLevel evaluates a verified command and returns the effective risk level,
// asserting the command was allowed (not Blocking) and no error occurred.
func evalLevel(t *testing.T, ev Evaluator, cmd string, args []string) runnertypes.RiskLevel {
	t.Helper()
	plan, err := ev.EvaluateRisk(verifiedCmd(cmd, args))
	require.NoError(t, err)
	assert.False(t, plan.Assessment.Blocking, "command %q must not be Blocking", cmd)
	return plan.Assessment.Level
}

func TestStandardEvaluator_EvaluateRisk_PrivilegeEscalation(t *testing.T) {
	ev := newVerifiedEvaluator()
	for _, cmd := range []string{"sudo", "su", "doas"} {
		t.Run(cmd, func(t *testing.T) {
			plan, err := ev.EvaluateRisk(verifiedCmd(cmd, []string{"ls"}))
			require.NoError(t, err)
			assert.Equal(t, runnertypes.RiskLevelCritical, plan.Assessment.Level)
			assert.Equal(t, risktypes.ReasonPrivilegeEscalation, plan.Assessment.BlockingReason)
		})
	}
}

// TestEvaluateRisk_PrivilegeEscalationViaSymlink verifies privilege escalation is
// detected through a symbolic link whose target basename is a privilege command,
// so a sudo alias cannot bypass the Critical gate.
func TestEvaluateRisk_PrivilegeEscalationViaSymlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "sudo")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755))
	link := filepath.Join(tmp, "my_sudo")
	require.NoError(t, os.Symlink(target, link))

	ev := newVerifiedEvaluator()
	plan, err := ev.EvaluateRisk(verifiedCmd(link, []string{"ls"}))
	require.NoError(t, err)
	assert.Equal(t, runnertypes.RiskLevelCritical, plan.Assessment.Level)
	assert.Equal(t, risktypes.ReasonPrivilegeEscalation, plan.Assessment.BlockingReason)
}

func TestStandardEvaluator_EvaluateRisk_DestructiveFileOperations(t *testing.T) {
	ev := newVerifiedEvaluator()
	tests := []struct {
		name string
		cmd  string
		args []string
	}{
		{"rm with recursive flag", "rm", []string{"-rf", "/tmp/files"}},
		{"find with delete", "find", []string{"/tmp", "-name", "*.tmp", "-delete"}},
		{"dd command", "dd", []string{"if=/dev/zero", "of=/tmp/test"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, tt.cmd, tt.args))
		})
	}
}

func TestStandardEvaluator_EvaluateRisk_NetworkOperations(t *testing.T) {
	ev := newVerifiedEvaluator()
	tests := []struct {
		name string
		cmd  string
		args []string
	}{
		{"wget download", "wget", []string{"https://example.com/file.txt"}},
		{"curl download", "curl", []string{"-O", "https://example.com/file.txt"}},
		{"nc command", "nc", []string{"-l", "8080"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, runnertypes.RiskLevelMedium, evalLevel(t, ev, tt.cmd, tt.args))
		})
	}
}

func TestStandardEvaluator_EvaluateRisk_SystemModifications(t *testing.T) {
	ev := newVerifiedEvaluator()
	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected runnertypes.RiskLevel
	}{
		// systemctl change verbs are now High, not Medium.
		{"systemctl restart", "systemctl", []string{"restart", "nginx"}, runnertypes.RiskLevelHigh},
		{"apt install", "apt", []string{"install", "vim"}, runnertypes.RiskLevelMedium},
		{"yum install", "yum", []string{"install", "vim"}, runnertypes.RiskLevelMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, evalLevel(t, ev, tt.cmd, tt.args))
		})
	}
}

func TestStandardEvaluator_EvaluateRisk_SafeCommands(t *testing.T) {
	ev := newVerifiedEvaluator()
	tests := []struct {
		name string
		cmd  string
		args []string
	}{
		{"echo command", "echo", []string{"Hello, World!"}},
		{"ls command", "ls", []string{"-la", "/home"}},
		{"cat command", "cat", []string{"/etc/passwd"}},
		{"apt list (query)", "apt", []string{"list", "--installed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, runnertypes.RiskLevelLow, evalLevel(t, ev, tt.cmd, tt.args))
		})
	}
}

// claude (DataExfilRisk=High, NetworkRisk=High) is High via profile factors.
func TestEvaluateRisk_ProfileMaxClaude(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "claude", []string{"--help"}))
}

// any profile factor declaring High floors the effective risk at High.
func TestEvaluateRisk_ProfileFactorFloor(t *testing.T) {
	ev := newVerifiedEvaluator()
	// gemini shares the claude profile (DataExfilRisk=High).
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "gemini", nil))
}

// profile reflection only raises risk; it never lowers another dimension.
// sudo stays Critical even though its profile would otherwise be evaluated.
func TestEvaluateRisk_ProfileSafeSideOnly(t *testing.T) {
	ev := newVerifiedEvaluator()
	plan, err := ev.EvaluateRisk(verifiedCmd("sudo", []string{"echo", "hi"}))
	require.NoError(t, err)
	assert.Equal(t, runnertypes.RiskLevelCritical, plan.Assessment.Level)
}

// a command without a profile is unaffected by the profile dimension.
func TestEvaluateRisk_ProfileStepNoChangeWithoutProfile(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelLow, evalLevel(t, ev, "echo", []string{"hi"}))
}

// an absolute-path destructive command is High.
func TestEvaluateRisk_AbsoluteRmRfHigh(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/bin/rm", []string{"-rf", "/tmp/x"}))
}

// systemctl change verbs and service (all actions) are High.
func TestEvaluateRisk_SystemctlChangeAndServiceHigh(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"restart", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/sbin/systemctl", []string{"stop", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "service", []string{"nginx", "start"}))
}

// systemctl read-only subcommands are a Medium floor; change/unknown are High.
func TestEvaluateRisk_SystemctlSubcommandConditional(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelMedium, evalLevel(t, ev, "systemctl", []string{"status", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelMedium, evalLevel(t, ev, "systemctl", []string{"show", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"restart", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"frobnicate", "nginx"}))
}

// service is High even for read-only-looking actions.
func TestEvaluateRisk_ServiceAllActionsHigh(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "service", []string{"nginx", "status"}))
}

// profile-less commands given as absolute paths get their correct risk.
func TestEvaluateRisk_NoProfileAbsolutePath(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/bin/rmdir", []string{"d"}), "rmdir is destructive")
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/bin/shred", []string{"f"}), "shred is destructive")
	assert.Equal(t, runnertypes.RiskLevelMedium, evalLevel(t, ev, "/usr/bin/mount", []string{"/dev/sda1", "/mnt"}), "mount is system modification")
	assert.Equal(t, runnertypes.RiskLevelMedium, evalLevel(t, ev, "/usr/bin/crontab", []string{"-l"}), "crontab is system modification")
}

// dangerous argument patterns contribute to the effective risk at runtime.
func TestEvaluateRisk_DangerousArgPatternsRuntime(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "chmod", []string{"-R", "777", "/"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/sbin/mkfs.ext4", []string{"/dev/sdX"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "dd", []string{"if=/dev/zero", "of=/dev/sdb"}))
	got := evalLevel(t, ev, "chown", []string{"-R", "root", "/tmp/x"})
	assert.GreaterOrEqual(t, got, runnertypes.RiskLevelMedium, "chown root is at least Medium")
}

// shells and interpreters are High regardless of arguments.
func TestEvaluateRisk_ShellInterpreterHigh(t *testing.T) {
	ev := newVerifiedEvaluator()
	for _, cmd := range []string{"bash", "python", "node", "ruby", "perl"} {
		assert.Equalf(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, cmd, []string{"--version"}),
			"%s must be High", cmd)
	}
}

// build/task runners are High.
func TestEvaluateRisk_BuildRunnerHigh(t *testing.T) {
	ev := newVerifiedEvaluator()
	for _, cmd := range []string{"make", "cmake", "gradle"} {
		assert.Equalf(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, cmd, nil), "%s must be High", cmd)
	}
}

// multiple dimensions take the maximum, independent of order.
func TestEvaluateRisk_MaxOfDimensionsOrderIndependent(t *testing.T) {
	ev := newVerifiedEvaluator()
	// rm matches destructive, profile destruction, and (with -rf) the dangerous
	// pattern; all High, max High.
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "rm", []string{"-rf", "/x"}))
	// A network profile command combined with a destructive arg form still ends up
	// at the maximum of the firing dimensions.
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/bin/rm", []string{"-rf", "/"}))
}

// The deny-path behaviors are covered by the tests below.

// an unverified hash blocks every evaluation path, never confirming Low.
func TestEvaluateRisk_UnverifiedHashUncertainAllPaths(t *testing.T) {
	ev := newVerifiedEvaluator()
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"coreutils-safe echo", "echo", nil},
		{"coreutils-destructive rm", "rm", []string{"-rf", "/"}},
		{"profile claude", "claude", nil},
		{"f015 python", "python", nil},
		{"no-profile destructive", "/usr/bin/rmdir", []string{"d"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No content hash -> identity gate denies.
			cmd := &runnertypes.RuntimeCommand{ExpandedCmd: tc.cmd, ExpandedArgs: tc.args}
			plan, err := ev.EvaluateRisk(cmd)
			require.NoError(t, err)
			assert.True(t, plan.Assessment.Blocking, "%s must be Blocking without a verified hash", tc.name)
			assert.Equal(t, risktypes.ReasonUncertainUnverifiedIdentity, plan.Assessment.BlockingReason)
			assert.Nil(t, plan.Identity, "denied plan must not carry a verified identity")
		})
	}
}

// with binary analysis disabled, every command is denied (including coreutils).
func TestEvaluateRisk_AnalysisDisabledAlwaysDeny(t *testing.T) {
	ev := newAnalysisDisabledEvaluator()
	for _, cmd := range []string{"echo", "ls", "/usr/bin/curl"} {
		plan, err := ev.EvaluateRisk(verifiedCmd(cmd, nil))
		require.NoError(t, err)
		assert.Truef(t, plan.Assessment.Blocking, "%s must be denied when analysis is disabled", cmd)
		assert.Equal(t, risktypes.ReasonAnalysisDisabled, plan.Assessment.BlockingReason)
	}
}

// an uncertain binary-analysis result blocks even when allowed at High.
func TestEvaluateRisk_UncertainBlockedEvenAtHigh(t *testing.T) {
	const cmdPath = "/opt/app/mybinary"
	store := fakeRecordStore{errs: map[string]error{cmdPath: fileErrNotFound()}}
	ev := newEvaluatorWithStore(store)
	plan, err := ev.EvaluateRisk(verifiedCmd(cmdPath, nil))
	require.NoError(t, err)
	assert.True(t, plan.Assessment.Blocking)
	assert.Equal(t, risktypes.ReasonUncertainMissingRecord, plan.Assessment.BlockingReason)
}

// a dangerous binary-analysis signal is High (allowable), not Blocking.
func TestEvaluateRisk_DangerousSignalsHighAllowable(t *testing.T) {
	const cmdPath = "/opt/app/loader"
	store := fakeRecordStore{records: map[string]*fileanalysis.Record{cmdPath: dlopenRecord()}}
	ev := newEvaluatorWithStore(store)
	plan, err := ev.EvaluateRisk(verifiedCmd(cmdPath, nil))
	require.NoError(t, err)
	assert.False(t, plan.Assessment.Blocking, "dangerous signal is allowable at high, not blocking")
	assert.Equal(t, runnertypes.RiskLevelHigh, plan.Assessment.Level)
}

// coreutils classification beats binary analysis. A safe coreutils command
// stays Low even with a dlopen signal; a destructive one is High.
func TestEvaluateRisk_CoreutilsPriorityOverBinaryAnalysis(t *testing.T) {
	tmp := t.TempDir()
	security.SetCoreutilsDirForTest(t, tmp)
	echoPath := filepath.Join(tmp, "echo")
	rmPath := filepath.Join(tmp, "rm")
	require.NoError(t, os.WriteFile(echoPath, []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(rmPath, []byte("#!/bin/sh\n"), 0o755))

	// Even if a record carried dlopen, coreutils suppresses binary analysis.
	store := fakeRecordStore{records: map[string]*fileanalysis.Record{
		echoPath: dlopenRecord(),
		rmPath:   dlopenRecord(),
	}}
	ev := newEvaluatorWithStore(store)

	assert.Equal(t, runnertypes.RiskLevelLow, evalLevel(t, ev, echoPath, nil))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, rmPath, []string{"-rf", "/tmp/x"}))
}

// a symlink chain matching multiple profiles takes the max; a resolution
// failure fails closed (see the resolution-failure test). Here a symlink whose
// target is a profiled command resolves to that profile.
func TestEvaluateRisk_SymlinkChainMaxAndFailSafe(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "curl")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755))
	link := filepath.Join(tmp, "mylink")
	require.NoError(t, os.Symlink(target, link))

	ev := newVerifiedEvaluator()
	// The link resolves to curl (network profile) -> Medium.
	assert.Equal(t, runnertypes.RiskLevelMedium, evalLevel(t, ev, link, nil))
}

// a symlink resolution failure (depth exceeded) is Blocking, not Low.
func TestEvaluateRisk_SymlinkResolutionFailureBlocking(t *testing.T) {
	tmp := t.TempDir()
	// Build a cycle: a -> b -> a, which exceeds the resolution depth.
	a := filepath.Join(tmp, "a")
	b := filepath.Join(tmp, "b")
	require.NoError(t, os.Symlink(b, a))
	require.NoError(t, os.Symlink(a, b))

	ev := newVerifiedEvaluator()
	plan, err := ev.EvaluateRisk(verifiedCmd(a, nil))
	require.NoError(t, err)
	assert.True(t, plan.Assessment.Blocking, "symlink resolution failure must be Blocking")
	assert.NotEqual(t, runnertypes.RiskLevelLow, plan.Assessment.Level, "must not fall to Low")
	assert.Equal(t, risktypes.ErrorClassSymlinkResolution, plan.Assessment.ErrorClass)
}

// deny (policy) and error (unexpected) are distinct. A missing analysis
// record is a deny (Blocking), while an unexpected record-load I/O error is an error.
func TestEvaluateRisk_DenyVsErrorClassification(t *testing.T) {
	const cmdPath = "/opt/app/x"

	t.Run("missing record is a policy deny", func(t *testing.T) {
		store := fakeRecordStore{errs: map[string]error{cmdPath: fileErrNotFound()}}
		ev := newEvaluatorWithStore(store)
		plan, err := ev.EvaluateRisk(verifiedCmd(cmdPath, nil))
		require.NoError(t, err)
		assert.True(t, plan.Assessment.Blocking)
	})

	t.Run("unexpected I/O error is returned as error", func(t *testing.T) {
		store := fakeRecordStore{errs: map[string]error{cmdPath: errUnexpectedIO}}
		ev := newEvaluatorWithStore(store)
		_, err := ev.EvaluateRisk(verifiedCmd(cmdPath, nil))
		require.Error(t, err)
	})
}

func TestStandardEvaluator_EvaluateRisk_Coreutils(t *testing.T) {
	tmp := t.TempDir()
	security.SetCoreutilsDirForTest(t, tmp)

	makeBinary := func(name string) string {
		path := filepath.Join(tmp, name)
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho test"), 0o755))
		return path
	}
	mkdirPath := makeBinary("mkdir")
	rmPath := makeBinary("rm")
	multicallPath := makeBinary("coreutils")

	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelLow, evalLevel(t, ev, mkdirPath, nil))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, rmPath, []string{"-rf", "/tmp/x"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, multicallPath, []string{"rm", "-rf", "/tmp/x"}))
}

func TestStandardEvaluator_EvaluateRisk_CoreutilsFileInfoFailureBlocks(t *testing.T) {
	// A path under the coreutils directory whose file does not exist makes the
	// setuid stat fail. The evaluator fails closed with a Blocking assessment
	// carrying the coreutils file-info error class.
	tmp := t.TempDir()
	security.SetCoreutilsDirForTest(t, tmp)

	ev := newVerifiedEvaluator()
	plan, err := ev.EvaluateRisk(verifiedCmd(filepath.Join(tmp, "mkdir"), nil)) // not created
	require.NoError(t, err)
	assert.True(t, plan.Assessment.Blocking)
	assert.Equal(t, risktypes.ErrorClassCoreutilsFileInfo, plan.Assessment.ErrorClass)
}

func TestStandardEvaluator_EvaluateRisk_RiskLevelHierarchy(t *testing.T) {
	ev := newVerifiedEvaluator()
	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected runnertypes.RiskLevel
	}{
		{"critical risk overrides all", "sudo", []string{"rm", "-rf", "/"}, runnertypes.RiskLevelCritical},
		{"high risk destructive operations", "rm", []string{"-rf", "/important/data"}, runnertypes.RiskLevelHigh},
		{"medium risk network operations", "wget", []string{"https://example.com/script.sh"}, runnertypes.RiskLevelMedium},
		{"high risk system modifications", "systemctl", []string{"stop", "important-service"}, runnertypes.RiskLevelHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, evalLevel(t, ev, tt.cmd, tt.args))
		})
	}
}
