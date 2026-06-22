//go:build test

package risk

import (
	"errors"
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
	for _, cmd := range []string{"sudo", "su", "doas", "pkexec", "runuser", "setpriv", "capsh"} {
		t.Run(cmd, func(t *testing.T) {
			plan, err := ev.EvaluateRisk(verifiedCmd(cmd, []string{"ls"}))
			require.NoError(t, err)
			assert.Equal(t, runnertypes.RiskLevelCritical, plan.Assessment.Level)
			assert.Equal(t, risktypes.ReasonPrivilegeEscalation, plan.Assessment.BlockingReason)
		})
	}
}

// TestEvaluateRisk_SystemModHighNotPrivilegeCritical verifies that
// permission/auth-boundary and kernel-module commands (visudo, useradd, insmod)
// are High system modification and are not escalated to the Critical privilege
// rank, so a per-command allow can still run a legitimate privileged batch.
func TestEvaluateRisk_SystemModHighNotPrivilegeCritical(t *testing.T) {
	ev := newVerifiedEvaluator()
	for _, cmd := range []string{"/usr/sbin/visudo", "/usr/sbin/useradd", "/sbin/insmod"} {
		t.Run(cmd, func(t *testing.T) {
			plan, err := ev.EvaluateRisk(verifiedCmd(cmd, []string{"x"}))
			require.NoError(t, err)
			assert.Equal(t, runnertypes.RiskLevelHigh, plan.Assessment.Level, "%s must be High", cmd)
			assert.NotEqual(t, risktypes.ReasonPrivilegeEscalation, plan.Assessment.BlockingReason,
				"%s must not be flagged as privilege escalation", cmd)
		})
	}
}

// TestEvaluateRisk_PrivilegeEscalationViaSymlink verifies privilege escalation is
// detected through a symbolic link whose target basename is a privilege command,
// so a sudo alias cannot bypass the Critical gate.
func TestEvaluateRisk_PrivilegeEscalationViaSymlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "sudo")
	require.NoError(t, os.WriteFile(target, []byte("\x7fELF\x02\x01\x01\x00"), 0o755))
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
		// Package managers and systemctl are High regardless of arguments.
		{"systemctl restart", "systemctl", []string{"restart", "nginx"}, runnertypes.RiskLevelHigh},
		{"apt install", "apt", []string{"install", "vim"}, runnertypes.RiskLevelHigh},
		{"yum install", "yum", []string{"install", "vim"}, runnertypes.RiskLevelHigh},
		// Same command, different arguments: a query is classified identically to
		// an install, demonstrating the classification ignores arguments.
		{"apt list (query)", "apt", []string{"list", "--installed"}, runnertypes.RiskLevelHigh},
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
func TestEvaluateRisk_ProfileSafeSideOnly(t *testing.T) {
	ev := newVerifiedEvaluator()
	// node carries a Medium network profile but is also a High arbitrary-code
	// runner. Reflecting the lower profile factor must not pull the result below
	// the High from the runner dimension.
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/bin/node", nil))
	// Likewise, a privilege-escalation Critical from an earlier step must not be
	// lowered by the profile dimension.
	plan, err := ev.EvaluateRisk(verifiedCmd("/usr/bin/sudo", []string{"echo", "hi"}))
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

// systemctl is always High, including read-only subcommands; the classification
// no longer inspects the subcommand.
func TestEvaluateRisk_SystemctlAlwaysHigh(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"status", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"show", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"restart", "nginx"}))
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "systemctl", []string{"frobnicate", "nginx"}))
}

// service is High even for read-only-looking actions.
func TestEvaluateRisk_ServiceAllActionsHigh(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "service", []string{"nginx", "status"}))
}

// a privilege token wrapping a system-modification command is Critical.
func TestEvaluateRisk_SudoSystemModificationCritical(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelCritical, evalLevel(t, ev, "/usr/bin/sudo", []string{"dpkg", "-i", "pkg.deb"}))
	assert.Equal(t, runnertypes.RiskLevelCritical, evalLevel(t, ev, "/usr/bin/sudo", []string{"systemctl", "restart", "nginx"}))
}

// a directly executed system-modification command carries the system-modification
// reason code in its assessment, so a deny is auditable as such.
func TestEvaluateRisk_SystemModificationReasonCode(t *testing.T) {
	ev := newVerifiedEvaluator()
	for _, cmd := range []string{"dpkg", "systemctl"} {
		plan, err := ev.EvaluateRisk(verifiedCmd(cmd, []string{"x"}))
		require.NoError(t, err)
		assert.Containsf(t, plan.Assessment.ReasonCodes, risktypes.ReasonSystemModification,
			"%s must carry the system-modification reason code", cmd)
	}
}

// profile-less commands given as absolute paths get their correct risk.
func TestEvaluateRisk_NoProfileAbsolutePath(t *testing.T) {
	ev := newVerifiedEvaluator()
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/bin/rmdir", []string{"d"}), "rmdir is destructive")
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/bin/shred", []string{"f"}), "shred is destructive")
	assert.Equal(t, runnertypes.RiskLevelMedium, evalLevel(t, ev, "/usr/bin/mount", []string{"/dev/sda1", "/mnt"}), "mount is system modification")
	assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, "/usr/bin/crontab", []string{"-l"}), "crontab is a High scheduler even for a query form")
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

// folding the indirect-execution floor must not duplicate a reason code already
// contributed by another dimension. "bash -c" yields ReasonArbitraryCodeExecution
// from both the rank-2 inline floor and the rank-7 runner dimension.
func TestEvaluateRisk_FloorReasonCodesDeduped(t *testing.T) {
	ev := newVerifiedEvaluator()
	plan, err := ev.EvaluateRisk(verifiedCmd("bash", []string{"-c", "echo hi"}))
	require.NoError(t, err)
	assert.Equal(t, runnertypes.RiskLevelHigh, plan.Assessment.Level)
	count := 0
	for _, c := range plan.Assessment.ReasonCodes {
		if c == risktypes.ReasonArbitraryCodeExecution {
			count++
		}
	}
	assert.Equal(t, 1, count, "floor folding must not duplicate a reason code already present")
}

// build/task runners are High.
func TestEvaluateRisk_BuildRunnerHigh(t *testing.T) {
	ev := newVerifiedEvaluator()
	for _, cmd := range []string{"make", "cmake", "gradle"} {
		assert.Equalf(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, cmd, nil), "%s must be High", cmd)
	}
}

// multiple dimensions take the maximum, independent of order. Each case fires
// both a Medium and a High dimension, so a result of Medium would prove the
// evaluator returned a lower dimension instead of the maximum.
func TestEvaluateRisk_MaxOfDimensionsOrderIndependent(t *testing.T) {
	ev := newVerifiedEvaluator()
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		// profile network (Medium) + arbitrary-code runner (High)
		{"interpreter: medium profile and high runner", "/usr/bin/python", nil},
		// network-style argument (Medium, unprofiled) + destructive (High)
		{"destructive with remote-style arg", "/usr/bin/rmdir", []string{"user@host:/srv"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, runnertypes.RiskLevelHigh, evalLevel(t, ev, tc.cmd, tc.args))
		})
	}
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
		{"coreutils-safe echo", "/usr/bin/echo", nil},
		{"coreutils-destructive rm", "/usr/bin/rm", []string{"-rf", "/"}},
		{"profile claude", "/usr/bin/claude", nil},
		{"f015 python", "/usr/bin/python", nil},
		{"no-profile destructive", "/usr/bin/rmdir", []string{"d"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No content hash -> identity gate denies (paths are absolute so the
			// non-absolute gate is not what fires here).
			cmd := &runnertypes.RuntimeCommand{ExpandedCmd: tc.cmd, ExpandedArgs: tc.args}
			plan, err := ev.EvaluateRisk(cmd)
			require.NoError(t, err)
			assert.True(t, plan.Assessment.Blocking, "%s must be Blocking without a verified hash", tc.name)
			assert.Equal(t, risktypes.ReasonUncertainUnverifiedIdentity, plan.Assessment.BlockingReason)
			assert.Nil(t, plan.Identity, "denied plan must not carry a verified identity")
		})
	}
}

// a non-absolute command path is denied fail-closed (it cannot be analyzed).
func TestEvaluateRisk_NonAbsolutePathBlocked(t *testing.T) {
	ev := newVerifiedEvaluator()
	for _, cmd := range []string{"echo", "bash", "./script.sh", "relative/rm", ""} {
		plan, err := ev.EvaluateRisk(&runnertypes.RuntimeCommand{
			ExpandedCmd:            cmd,
			ExpandedCmdContentHash: testContentHash,
		})
		require.NoError(t, err)
		assert.Truef(t, plan.Assessment.Blocking, "%q must be Blocking (non-absolute)", cmd)
		assert.Equal(t, risktypes.ReasonNonAbsolutePath, plan.Assessment.BlockingReason, cmd)
		assert.Nil(t, plan.Identity, cmd)
	}
}

// a network-style argument makes an unprofiled command a Medium network operation.
func TestEvaluateRisk_NetworkArgumentUnprofiled(t *testing.T) {
	ev := newVerifiedEvaluator()
	// /usr/bin/myhelper has no profile; the URL argument raises it to Medium.
	assert.Equal(t, runnertypes.RiskLevelMedium,
		evalLevel(t, ev, "/usr/bin/myhelper", []string{"--fetch", "https://example.com/x"}))
	assert.Equal(t, runnertypes.RiskLevelMedium,
		evalLevel(t, ev, "/usr/bin/myhelper", []string{"user@host:/remote/path"}))
	// Without a network-style argument it stays Low.
	assert.Equal(t, runnertypes.RiskLevelLow,
		evalLevel(t, ev, "/usr/bin/myhelper", []string{"--local", "/tmp/x"}))
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
	// The identity gate (rank 1) passed, so a later-dimension deny must still carry
	// the verified identity for audit/artifact gating (nil is reserved for denies
	// that never established an identity).
	require.NotNil(t, plan.Identity, "dimension-blocking deny must preserve the verified identity")
	assert.Equal(t, cmdPath, plan.Identity.ResolvedPath)
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
	require.NoError(t, os.WriteFile(echoPath, []byte("\x7fELF\x02\x01\x01\x00"), 0o755))
	require.NoError(t, os.WriteFile(rmPath, []byte("\x7fELF\x02\x01\x01\x00"), 0o755))

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
	require.NoError(t, os.WriteFile(target, []byte("\x7fELF\x02\x01\x01\x00"), 0o755))
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
		require.NoError(t, os.WriteFile(path, []byte("\x7fELF\x02\x01\x01\x00"), 0o755))
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

// TestEvaluateRisk_IndirectExecutionDeny verifies that indirect-execution forms
// are evaluated and denied (or elevated) end to end through EvaluateRisk: the
// rank-2 resolver is wired so env sudo is Critical, find/xargs and dynamic-loader
// forms are Blocking, a forbidden loader env var is Blocking, and inline shells or
// wrapped destructive commands are elevated to High.
func TestEvaluateRisk_IndirectExecutionDeny(t *testing.T) {
	ev := newVerifiedEvaluator()
	tests := []struct {
		name        string
		cmd         string
		args        []string
		wantBlock   bool
		wantLevel   runnertypes.RiskLevel
		wantReason  risktypes.ReasonCode
		checkReason bool
	}{
		{
			name: "env sudo is Critical", cmd: "env", args: []string{"sudo", "ls"},
			wantLevel: runnertypes.RiskLevelCritical, wantReason: risktypes.ReasonPrivilegeEscalation, checkReason: true,
		},
		{
			name: "env LD_PRELOAD is Blocking", cmd: "env", args: []string{"LD_PRELOAD=/tmp/x.so", "ls"},
			wantBlock: true, wantReason: risktypes.ReasonForbiddenEnvVar, checkReason: true,
		},
		{
			name: "find -exec is Blocking", cmd: "find", args: []string{"/tmp", "-exec", "rm", "{}", ";"},
			wantBlock: true, wantReason: risktypes.ReasonIndirectExecutionRejected, checkReason: true,
		},
		{
			name: "xargs rm is Blocking", cmd: "xargs", args: []string{"rm", "-rf"},
			wantBlock: true,
		},
		{
			name: "bash -c is High", cmd: "bash", args: []string{"-c", "rm -rf /"},
			wantLevel: runnertypes.RiskLevelHigh,
		},
		{
			name: "env rm -rf is High", cmd: "env", args: []string{"rm", "-rf", "/tmp/x"},
			wantLevel: runnertypes.RiskLevelHigh,
		},
		{
			name: "npm run is High", cmd: "npm", args: []string{"run", "build"},
			wantLevel: runnertypes.RiskLevelHigh,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ev.EvaluateRisk(verifiedCmd(tt.cmd, tt.args))
			require.NoError(t, err)
			if tt.wantBlock {
				assert.True(t, plan.Assessment.Blocking, "expected Blocking")
				// IndirectReject: the command passed the identity gate, so the
				// verified identity must be preserved for audit and artifact gating.
				assert.NotNil(t, plan.Identity, "IndirectReject plan must carry the verified identity")
			} else {
				assert.False(t, plan.Assessment.Blocking, "expected allowed")
				assert.Equal(t, tt.wantLevel, plan.Assessment.Level)
			}
			if tt.checkReason {
				assert.Equal(t, tt.wantReason, plan.Assessment.BlockingReason)
			}
		})
	}
}

// TestEvaluateRisk_DynamicLoaderBlocking verifies a direct dynamic-loader
// invocation is denied through EvaluateRisk.
func TestEvaluateRisk_DynamicLoaderBlocking(t *testing.T) {
	ev := newVerifiedEvaluator()
	plan, err := ev.EvaluateRisk(verifiedCmd("/lib64/ld-linux-x86-64.so.2", []string{"/bin/sh"}))
	require.NoError(t, err)
	assert.True(t, plan.Assessment.Blocking)
}

// TestEvaluateRisk_AllowedPlanCarriesVerifiedFd exercises the production identity
// opener: an allowed command's plan must carry a real verified file descriptor so
// the executor can bind execution to that inode.
func TestEvaluateRisk_AllowedPlanCarriesVerifiedFd(t *testing.T) {
	// Use the real opener (default), so a real on-disk file is required.
	ev := NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{RecordStore: fakeRecordStore{}}))

	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

	plan, err := ev.EvaluateRisk(&runnertypes.RuntimeCommand{
		ExpandedCmd:            bin,
		ExpandedCmdContentHash: testContentHash,
	})
	require.NoError(t, err)
	defer func() { _ = plan.Close() }()

	require.False(t, plan.Assessment.Blocking, "a verified clean binary should be allowed")
	require.NotNil(t, plan.Identity)
	require.NotNil(t, plan.Identity.FD, "an allowed plan must carry a verified fd")
	assert.Greater(t, plan.Identity.FD.Fd(), 2, "fd should be a real descriptor")
}

// TestEvaluateRisk_OpenFailureBlocks confirms the fail-closed contract: when the
// verified binary cannot be opened for fd-bound execution, the command is denied
// rather than executed via an unbound path.
func TestEvaluateRisk_OpenFailureBlocks(t *testing.T) {
	ev := newVerifiedEvaluator().(*StandardEvaluator)
	openErr := errors.New("open failed")
	ev.openIdentity = func(_ *runnertypes.RuntimeCommand) (*risktypes.VerifiedIdentity, error) {
		return nil, openErr
	}

	plan, err := ev.EvaluateRisk(verifiedCmd("/usr/bin/echo", nil))
	require.NoError(t, err)
	defer func() { _ = plan.Close() }()

	assert.True(t, plan.Assessment.Blocking, "an unopenable verified binary must be denied")
	assert.Equal(t, risktypes.ReasonIdentityUnbound, plan.Assessment.BlockingReason)
}
