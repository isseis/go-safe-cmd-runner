package security

import (
	"os"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// indirectExecMaxDepth bounds the recursion when a wrapper wraps another wrapper
// (env timeout nice ...). Each level consumes at least the wrapper token, so the
// argv shrinks every step; the guard is a backstop against a pathological input.
const indirectExecMaxDepth = 16

// IndirectExecutionKind is the rank-2 (indirect execution) outcome of analyzing a
// command. It captures whether the command runs or loads an artifact other than
// the one that was verified, and how the evaluator must treat it.
type IndirectExecutionKind int

const (
	// IndirectNone means the command is not a recognized indirect-execution form.
	IndirectNone IndirectExecutionKind = iota
	// IndirectCritical means a privilege-escalation token (sudo/su/doas) was found
	// as the effective target; the command is Critical (always denied).
	IndirectCritical
	// IndirectReject means the form cannot be identity-bound until exec (an
	// unextractable wrapper, a forbidden loader-control variable, a find/xargs
	// child-process exec, a direct dynamic-loader invocation, a remote-shell
	// helper). It is a Blocking deny.
	IndirectReject
	// IndirectFloor means the form is allowable but contributes a minimum risk
	// level (env with no command -> Medium; inline shell/interpreter, package
	// script runner, or a wrapped dangerous inner command -> their level) that is
	// folded into the effective-risk maximum.
	IndirectFloor
)

// IndirectExecutionResult is the rank-2 outcome the evaluator folds into the
// effective risk.
//
// Scope: this resolver produces the evaluation-time decision (Critical / Reject /
// Floor) and records the artifacts that participate in the chain. The actual fd
// binding and hash gating of each artifact (populating Artifact.Identity and
// Disposition) is wired in the execution layer; here Artifacts carry their path
// and role for audit and for that later binding step.
type IndirectExecutionResult struct {
	Kind        IndirectExecutionKind
	Level       runnertypes.RiskLevel
	ReasonCodes []risktypes.ReasonCode
	ErrorClass  risktypes.ErrorClass
	Artifacts   []risktypes.ExecutedArtifact
}

// wrapperSpec describes how to skip a wrapper's own options and positional
// arguments to reach the inner COMMAND it runs. The runner re-implements these
// wrappers (it execs the extracted inner command itself), so the inner command
// is identity-bindable; that is why wrappers resolve rather than reject.
type wrapperSpec struct {
	// valueOpts are options that consume the following token as their value
	// (e.g. timeout -s SIGNAL), so that token is not mistaken for the COMMAND.
	valueOpts map[string]struct{}
	// positionals is the number of positional arguments that precede the COMMAND
	// (timeout DURATION, chrt PRIORITY, taskset MASK).
	positionals int
}

// wrapperSpecs is the curated set of wrappers whose inner command the runner can
// extract and exec directly. env is parsed separately (it also carries NAME=VALUE
// assignments and -S split-strings). xargs is intentionally excluded: it execs
// the helper from its own child process, so the runner cannot identity-bind it
// (handled as a child-process exec form below).
var wrapperSpecs = map[string]wrapperSpec{
	"timeout": {valueOpts: setOf("-s", "--signal", "-k", "--kill-after"), positionals: 1},
	"nice":    {valueOpts: setOf("-n", "--adjustment"), positionals: 0},
	"ionice":  {valueOpts: setOf("-c", "--class", "-n", "--classdata", "-p", "--pid"), positionals: 0},
	"nohup":   {positionals: 0},
	"stdbuf":  {valueOpts: setOf("-i", "--input", "-o", "--output", "-e", "--error"), positionals: 0},
	"setsid":  {positionals: 0},
	"time":    {valueOpts: setOf("-o", "--output", "-f", "--format"), positionals: 0},
	"chrt":    {valueOpts: setOf("-T", "--sched-runtime", "-P", "--sched-period", "-D", "--sched-deadline"), positionals: 1},
	"taskset": {positionals: 1},
}

// loaderControlEnvVars are environment variable names that change which shared
// objects a dynamic executable loads. Supplying them via a wrapper lets an
// attacker inject code into an otherwise-verified binary, so they are rejected.
// DYLD_* (macOS) is rejected on every OS, since the deny list is platform
// independent.
var loaderControlEnvVars = setOf(
	"LD_PRELOAD", "LD_LIBRARY_PATH", "LD_AUDIT", "LD_PROFILE", "LD_ORIGIN_PATH",
	"LD_CONFIG", "LD_DYNAMIC_WEAK",
)

// shellInlineCommands are shells whose inline-code flag is -c only. -e is the
// errexit boolean option, not inline code, so "bash -e script.sh" is not treated
// as an inline string.
var shellInlineCommands = setOf("bash", "sh", "dash", "zsh", "ksh", "csh", "tcsh", "fish")

// interpreterInlineCommands are interpreters whose inline-code flags are -e (eval)
// and -c.
var interpreterInlineCommands = setOf(
	"python", "python2", "python3", "node", "nodejs", "deno", "bun",
	"ruby", "perl", "php", "lua", "luajit", "tclsh",
)

// remoteShellOptionPrefixes map a command to the options that make it execute an
// external helper from its own child process (rsync's remote shell, tar's output
// filter / checkpoint action). The helper is not runner-execed, so it cannot be
// identity-bound and the form is rejected.
var remoteShellOptionPrefixes = map[string][]string{
	"rsync": {"-e", "--rsh"},
	"tar":   {"--to-command", "--checkpoint-action"},
}

// setOf builds a set from the given keys.
func setOf(keys ...string) map[string]struct{} {
	s := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		s[k] = struct{}{}
	}
	return s
}

// AnalyzeIndirectExecution detects whether the command executes or loads an
// artifact other than the verified one (a wrapper's inner command, an inline
// script, a loader-injected library, a find/xargs child-process helper, a remote
// shell helper) and returns how the evaluator must treat it. Detection is by
// basename and resolved symbolic links, mirroring the other name-based
// classifiers.
func AnalyzeIndirectExecution(cmdPath string, args []string) IndirectExecutionResult {
	return analyzeIndirect(cmdPath, args, 0)
}

func analyzeIndirect(cmdPath string, args []string, depth int) IndirectExecutionResult {
	if depth >= indirectExecMaxDepth {
		return reject()
	}

	names, _ := extractAllCommandNames(cmdPath)

	// env is parsed specially: it carries NAME=VALUE assignments and a -S
	// split-string in addition to an optional inner command.
	if _, ok := names["env"]; ok {
		return analyzeEnv(args, depth)
	}

	// Other wrappers the runner re-implements: extract the inner command and
	// evaluate it (the wrapper itself adds no execution beyond the inner command).
	for name, spec := range wrapperSpecs {
		if _, ok := names[name]; ok {
			return analyzeWrapper(spec, args, depth)
		}
	}

	// find/xargs run the helper from their own child process; the runner cannot
	// identity-bind it. A privilege token there is still Critical; any other
	// helper is rejected (cannot be bound).
	if _, ok := names["xargs"]; ok {
		return analyzeChildProcessExec(xargsTarget(args))
	}
	if _, ok := names["find"]; ok {
		if target, ok := findExecTarget(args); ok {
			return analyzeChildProcessExec(target, true)
		}
	}

	// Direct dynamic-loader invocation (ld-linux*.so --preload ...): the loader
	// loads arbitrary libraries the runner cannot bind. Reject.
	if hasDynamicLoaderName(names) {
		return reject()
	}

	// Remote-shell / output-filter helpers (rsync -e, tar --to-command): the
	// helper runs from the tool's child process. Reject.
	if res, ok := analyzeRemoteShellOption(names, args); ok {
		return res
	}

	// Package script runners (npm run / npx / yarn run / pnpm run): execute a
	// script from an unverified manifest. High.
	if level, ok := packageScriptRunnerRisk(names, args); ok {
		return floor(level, risktypes.ReasonArbitraryCodeExecution)
	}

	// Shell/interpreter inline code (bash -c, python -c/-e): High floor.
	if hasInlineCode(names, args) {
		return floor(runnertypes.RiskLevelHigh, risktypes.ReasonArbitraryCodeExecution)
	}

	// SysV service runs an unverified init script (/etc/init.d/<name>). service is
	// already High via system modification; record the init script as a chain
	// artifact so it is gated and audited.
	if _, ok := names["service"]; ok {
		return analyzeService(args)
	}

	// Direct script with a shebang (#!/usr/bin/env python): the kernel runs the
	// shebang interpreter, a separate artifact, so the interpreter chain is
	// evaluated and gated.
	if res, ok := analyzeShebang(cmdPath, args, depth); ok {
		return res
	}

	return IndirectExecutionResult{Kind: IndirectNone}
}

// analyzeShebang evaluates the interpreter chain of a direct script execution.
// When cmdPath is a regular file beginning with "#!", the shebang interpreter is
// the artifact actually executed, so its risk is folded and it is recorded as a
// RoleInterpreter chain artifact.
func analyzeShebang(cmdPath string, scriptArgs []string, depth int) (IndirectExecutionResult, bool) {
	interp, interpArgs, ok := readShebang(cmdPath)
	if !ok {
		return IndirectExecutionResult{}, false
	}
	// The interpreter runs with its shebang arguments followed by the script path
	// and the script's own arguments.
	args := append(append([]string{}, interpArgs...), cmdPath)
	args = append(args, scriptArgs...)
	// Evaluate the interpreter as the executed artifact, labeled RoleInterpreter.
	// evaluateInnerAs records that artifact on every outcome (Floor/Critical/
	// Reject), so the interpreter is always present in the audit chain.
	res := evaluateInnerAs(interp, args, depth, risktypes.RoleInterpreter)
	return res, true
}

// readShebang reads the interpreter and its inline arguments from a file's "#!"
// first line. It returns ok=false when the file cannot be read or does not start
// with a shebang.
func readShebang(path string) (interp string, args []string, ok bool) {
	const maxShebangLen = 256
	// The path is the resolved command path the evaluator is already classifying;
	// reading its first line to detect a shebang interpreter is intentional.
	f, err := os.Open(path) //nolint:gosec // reading the verified command path to detect a shebang
	if err != nil {
		return "", nil, false
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, maxShebangLen)
	n, _ := f.Read(buf)
	if n < 2 || buf[0] != '#' || buf[1] != '!' {
		return "", nil, false
	}
	line := string(buf[2:n])
	if idx := strings.IndexAny(line, "\n\r"); idx >= 0 {
		line = line[:idx]
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", nil, false
	}
	return fields[0], fields[1:], true
}

// envBooleanOptions are env's own value-less options.
var envBooleanOptions = setOf("-i", "--ignore-environment", "-0", "--null", "-v", "--debug")

// envValueOptions are env's own options that consume the following token.
var envValueOptions = setOf("-u", "--unset", "-C", "--chdir")

// analyzeEnv parses an env invocation: NAME=VALUE assignments, -S split-strings,
// option flags, and the optional inner command.
func analyzeEnv(args []string, depth int) IndirectExecutionResult {
	pathOverridden := false
	for i := 0; i < len(args); i++ {
		t := args[i]
		if payload, consumed, isSplit, valid := envSplitArg(args, i); isSplit {
			if !valid {
				return reject()
			}
			// env -S prepends the split tokens to the remaining argv (e.g.
			// `env -S "env" sudo ls` runs `env env sudo ls`), so the trailing
			// arguments must be carried along, not discarded.
			return analyzeEnvSplitString(payload, args[i+consumed:], depth)
		}
		if _, ok := envBooleanOptions[t]; ok {
			continue
		}
		if _, ok := envValueOptions[t]; ok {
			i++ // option consumes the following token
			continue
		}
		if isAssignment(t) {
			if res, rejected := checkEnvAssignment(t, &pathOverridden); rejected {
				return res
			}
			continue
		}
		if t == "--" {
			// Option terminator: the remaining tokens are operands (NAME=VALUE
			// assignments then the command). The command is taken literally even if
			// it begins with '-', so it is no longer subject to option parsing.
			for j := i + 1; j < len(args); j++ {
				if isAssignment(args[j]) {
					if res, rejected := checkEnvAssignment(args[j], &pathOverridden); rejected {
						return res
					}
					continue
				}
				return resolveInner(args[j], args[j+1:], pathOverridden, depth)
			}
			return floor(runnertypes.RiskLevelMedium, risktypes.ReasonIndirectExecutionWrapper)
		}
		if strings.HasPrefix(t, "-") {
			// Unknown env option: it may or may not consume a value, so the inner
			// command can no longer be located reliably. Fail closed.
			return reject()
		}
		// First non-option, non-assignment token is the inner command.
		return resolveInner(t, args[i+1:], pathOverridden, depth)
	}
	// env with no inner command (only assignments/options): suspicious but not a
	// concrete exec of another artifact. Medium floor.
	return floor(runnertypes.RiskLevelMedium, risktypes.ReasonIndirectExecutionWrapper)
}

// envSplitArg detects env's -S/--split-string at position i. isSplit is true when
// the token is a split-string form; valid is false when its payload is missing.
// consumed is the number of argv tokens the option occupies (2 for the separated
// `-S VALUE` form, 1 for the attached `-SVALUE` / `--split-string=VALUE` forms),
// so the caller can carry the remaining arguments after the option.
func envSplitArg(args []string, i int) (payload string, consumed int, isSplit, valid bool) {
	t := args[i]
	switch {
	case t == "-S" || t == "--split-string":
		if i+1 >= len(args) {
			return "", 1, true, false
		}
		// The separated form occupies two tokens: the option and its value.
		const separatedFormTokens = 2
		return args[i+1], separatedFormTokens, true, true
	case strings.HasPrefix(t, "-S"):
		return t[len("-S"):], 1, true, true
	case strings.HasPrefix(t, "--split-string="):
		return t[len("--split-string="):], 1, true, true
	}
	return "", 0, false, false
}

// checkEnvAssignment rejects loader-control assignments (LD_*/DYLD_*) and records
// a PATH override. rejected is true when the assignment must be denied.
func checkEnvAssignment(t string, pathOverridden *bool) (IndirectExecutionResult, bool) {
	name, _, _ := strings.Cut(t, "=")
	if _, bad := loaderControlEnvVars[strings.ToUpper(name)]; bad || isDyldVar(name) {
		return rejectClass(risktypes.ReasonForbiddenEnvVar, ""), true
	}
	if name == "PATH" {
		*pathOverridden = true
	}
	return IndirectExecutionResult{}, false
}

// analyzeEnvSplitString interprets env -S 'NAME=VALUE ... COMMAND ARG ...'.
//
// env -S applies its own escape/quote/variable processing (backslash escapes,
// single/double quotes, ${VAR} substitution, '#' comments) before splitting into
// argv. A plain whitespace split cannot reproduce that, so it would mis-tokenize a
// payload like 'sudo\tls' or "'sudo' ls" and miss the hidden command (fail open).
// To stay fail-closed (uninterpretable -> reject), any payload containing a
// character that triggers that extra processing is rejected; only payloads that
// reduce to a faithful whitespace split are interpreted. The split tokens are
// prepended to the remaining argv (the tokens after the -S option) and re-parsed.
func analyzeEnvSplitString(s string, remaining []string, depth int) IndirectExecutionResult {
	// Backtick is included so a command-substitution payload also fails closed.
	if strings.ContainsAny(s, "\\'\"$#`") {
		return reject()
	}
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return reject()
	}
	return analyzeEnv(append(tokens, remaining...), depth+1)
}

// resolveInner evaluates a wrapper's extracted inner command. When the resolution
// path is attacker-controlled (env overrode PATH) and the inner command is a bare
// name, it cannot be resolved safely and is rejected.
func resolveInner(inner string, innerArgs []string, pathOverridden bool, depth int) IndirectExecutionResult {
	if pathOverridden && !strings.Contains(inner, "/") {
		return reject()
	}
	return evaluateInner(inner, innerArgs, depth)
}

// analyzeWrapper skips a wrapper's options and positional arguments and evaluates
// the inner command it runs.
func analyzeWrapper(spec wrapperSpec, args []string, depth int) IndirectExecutionResult {
	idx := skipWrapperOptions(args, spec.valueOpts)
	idx += spec.positionals
	if idx >= len(args) {
		// No inner command (e.g. "timeout 5" with no COMMAND). Like env with no
		// command, treat as a Medium floor rather than rejecting outright.
		return floor(runnertypes.RiskLevelMedium, risktypes.ReasonIndirectExecutionWrapper)
	}
	return evaluateInner(args[idx], args[idx+1:], depth)
}

// skipWrapperOptions returns the index of the first positional argument after the
// wrapper's leading options, consuming the value of any value-taking option.
func skipWrapperOptions(args []string, valueOpts map[string]struct{}) int {
	i := 0
	for i < len(args) {
		t := args[i]
		if t == "--" {
			// Option terminator: the next token is the first positional argument.
			return i + 1
		}
		if !strings.HasPrefix(t, "-") || t == "-" {
			break
		}
		i++
		if _, ok := valueOpts[t]; ok {
			i++ // consume the option's value
		}
	}
	return i
}

// evaluateInner folds the inner command's name-based risk and recurses into a
// nested wrapper/form, recording the artifact with the RoleInner role.
func evaluateInner(inner string, innerArgs []string, depth int) IndirectExecutionResult {
	return evaluateInnerAs(inner, innerArgs, depth, risktypes.RoleInner)
}

// evaluateInnerAs is evaluateInner with an explicit artifact role (a shebang
// interpreter is recorded as RoleInterpreter rather than RoleInner). A privilege
// token is Critical; an inner form that cannot be bound (find/xargs, loader)
// propagates its rejection. The inner command is recorded as a chain artifact on
// every outcome (Floor/Critical/Reject) so the indirect-execution chain remains
// traceable in audits even on deny paths.
func evaluateInnerAs(inner string, innerArgs []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	artifact := risktypes.ExecutedArtifact{Path: inner, Role: role}

	if isPrivilegeCommand(inner) {
		res := critical()
		res.Artifacts = append(res.Artifacts, artifact)
		return res
	}

	// Recurse: the inner command may itself be a wrapper or another indirect form.
	nested := analyzeIndirect(inner, innerArgs, depth+1)
	switch nested.Kind {
	case IndirectCritical, IndirectReject:
		nested.Artifacts = append(nested.Artifacts, artifact)
		return nested
	}

	level := nested.Level
	codes := append([]risktypes.ReasonCode{}, nested.ReasonCodes...)

	if IsDestructiveFileOperation(inner, innerArgs) {
		level = max(level, runnertypes.RiskLevelHigh)
		codes = append(codes, risktypes.ReasonDestructiveFileOperation)
	}
	innerNames, _ := extractAllCommandNames(inner)
	if s := SystemModificationRisk(innerNames, inner, innerArgs); s > runnertypes.RiskLevelUnknown {
		level = max(level, s)
		codes = append(codes, risktypes.ReasonSystemModification)
	}
	if IsArbitraryCodeExecutionRunner(inner) {
		level = max(level, runnertypes.RiskLevelHigh)
		codes = append(codes, risktypes.ReasonArbitraryCodeExecution)
	}
	if l, _ := CheckDangerousArgPatterns(inner, innerArgs); l > runnertypes.RiskLevelUnknown {
		level = max(level, l)
		codes = append(codes, risktypes.ReasonDangerousArgPattern)
	}
	// Fold the inner command's risk profile (destruction / data exfiltration /
	// applicable network) so a profiled command (claude, curl, ssh, ...) is not
	// under-classified when wrapped. Privilege is handled above; system
	// modification is handled via SystemModificationRisk.
	if profile, ok := ResolveProfile(inner); ok {
		if pl, pcodes := ProfileFactorRisk(profile, innerArgs); pl > runnertypes.RiskLevelUnknown {
			level = max(level, pl)
			codes = append(codes, pcodes...)
		}
	}

	return IndirectExecutionResult{
		Kind:        IndirectFloor,
		Level:       level,
		ReasonCodes: codes,
		Artifacts:   append(nested.Artifacts, artifact),
	}
}

// analyzeChildProcessExec handles a helper run from find/xargs' own child process.
// A privilege token is Critical; any other helper cannot be identity-bound by the
// runner and is rejected.
func analyzeChildProcessExec(target string, hasTarget bool) IndirectExecutionResult {
	if hasTarget && isPrivilegeCommand(target) {
		return critical()
	}
	return reject()
}

// xargsTarget returns the helper command xargs would run: the first non-option
// token after xargs' own options.
func xargsTarget(args []string) (string, bool) {
	i := 0
	valueOpts := setOf("-I", "--replace", "-n", "--max-args", "-P", "--max-procs",
		"-L", "--max-lines", "-s", "--max-chars", "-E", "-d", "--delimiter", "-a", "--arg-file")
	for i < len(args) {
		t := args[i]
		if !strings.HasPrefix(t, "-") || t == "-" {
			return t, true
		}
		i++
		// -I{}/-n5 combined forms carry their value inline; only the separated
		// forms consume the next token.
		if _, ok := valueOpts[t]; ok {
			i++
		}
	}
	return "", false
}

// findExecTarget returns the command find would run for an -exec/-execdir/-ok/-okdir
// primary: the token immediately after the primary.
func findExecTarget(args []string) (string, bool) {
	for i, arg := range args {
		if _, ok := findExecActions[arg]; ok && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// analyzeService records the SysV init script the service command runs so it is
// gated and audited. service itself is High via system modification, so this adds
// a Floor at High plus the init-script artifact. When the unit name cannot be
// safely extracted (option-only forms, or a name that is not a simple basename),
// the init script cannot be identified or gated, so it fails closed like other
// unbindable indirect forms.
func analyzeService(args []string) IndirectExecutionResult {
	name, ok := serviceUnitName(args)
	if !ok || !isSimpleUnitName(name) {
		return reject()
	}
	res := floor(runnertypes.RiskLevelHigh, risktypes.ReasonSystemModification)
	// Record the init script path and role; identity binding / disposition is
	// populated when artifact gating is wired in the execution layer.
	res.Artifacts = []risktypes.ExecutedArtifact{{
		Path: "/etc/init.d/" + name,
		Role: risktypes.RoleExecTarget,
	}}
	return res
}

// isSimpleUnitName reports whether name is a plain basename, so building
// "/etc/init.d/<name>" cannot escape that directory via a slash or "..".
func isSimpleUnitName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.Contains(name, "/")
}

// serviceUnitName returns the unit name from "service <name> <action>", skipping
// service's own options (--status-all, --full-restart, etc.).
func serviceUnitName(args []string) (string, bool) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a, true
	}
	return "", false
}

// analyzeRemoteShellOption rejects rsync -e / tar --to-command style helpers,
// which the tool runs from its own child process.
func analyzeRemoteShellOption(names map[string]struct{}, args []string) (IndirectExecutionResult, bool) {
	for cmd, prefixes := range remoteShellOptionPrefixes {
		if _, ok := names[cmd]; !ok {
			continue
		}
		for _, a := range args {
			for _, p := range prefixes {
				if matchesRemoteShellOption(a, p) {
					return reject(), true
				}
			}
		}
	}
	return IndirectExecutionResult{}, false
}

// matchesRemoteShellOption reports whether arg selects the helper option p. It
// matches the exact form (-e), the long attached form (--rsh=ssh), the short
// attached form (-essh), and a short-option bundle that includes the option
// letter (-avze ssh), so an attached or bundled value cannot slip past.
func matchesRemoteShellOption(arg, p string) bool {
	if arg == p || strings.HasPrefix(arg, p+"=") {
		return true
	}
	// Short option (e.g. -e): rsync attaches the value (-essh) or bundles the
	// letter with other short flags (-avze), so any single-dash (non "--") token
	// that contains the option letter selects it.
	if len(p) == 2 && p[0] == '-' && p[1] != '-' {
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			return strings.IndexByte(arg[1:], p[1]) >= 0
		}
	}
	return false
}

// packageManagerBuiltins are subcommands of yarn/pnpm that manage packages
// rather than run a package.json script. Anything else passed to yarn/pnpm is a
// script invocation (yarn/pnpm treat "<name>" as shorthand for "run <name>").
// The set is deliberately conservative: an unrecognized verb falls through to the
// script-runner (High) classification, which is the safe side.
var packageManagerBuiltins = setOf(
	"install", "add", "remove", "up", "upgrade", "upgrade-interactive", "dedupe",
	"why", "list", "info", "init", "link", "unlink", "pack", "publish", "config",
	"cache", "audit", "outdated", "import", "licenses", "owner", "version",
	"workspace", "workspaces", "set", "plugin", "create", "global", "bin",
	"autoclean", "check", "login", "logout", "node", "rebuild", "store", "patch",
)

// packageScriptRunnerRisk reports the High risk of a package script runner
// (npm run / npx / yarn run / pnpm run / dlx / exec, lifecycle aliases, and the
// yarn/pnpm "<script>" shorthand).
func packageScriptRunnerRisk(names map[string]struct{}, args []string) (runnertypes.RiskLevel, bool) {
	if _, ok := names["npx"]; ok {
		return runnertypes.RiskLevelHigh, true
	}
	// Lifecycle aliases (test/start/stop/restart) run package.json scripts without
	// the explicit run/run-script verb, so they are arbitrary-code runners too.
	scriptVerbs := setOf("run", "run-script", "exec", "dlx", "test", "start", "stop", "restart")
	for _, runner := range []string{"npm", "pnpm", "yarn"} {
		if _, ok := names[runner]; !ok {
			continue
		}
		verb, ok := packageRunnerVerb(args)
		if !ok {
			continue
		}
		if _, isScript := scriptVerbs[verb]; isScript {
			return runnertypes.RiskLevelHigh, true
		}
		// yarn and pnpm run "<script>" as shorthand for "run <script>", so any verb
		// that is not a package-management builtin is a script invocation.
		if (runner == "yarn" || runner == "pnpm") && !isPackageManagerBuiltin(verb) {
			return runnertypes.RiskLevelHigh, true
		}
	}
	return runnertypes.RiskLevelUnknown, false
}

// isPackageManagerBuiltin reports whether verb is a yarn/pnpm package-management
// subcommand (not a package.json script).
func isPackageManagerBuiltin(verb string) bool {
	_, ok := packageManagerBuiltins[verb]
	return ok
}

// hasInlineCode reports whether a shell (-c) or interpreter (-c/-e) is invoked
// with an inline-code flag.
func hasInlineCode(names map[string]struct{}, args []string) bool {
	if anyNameInSet(names, shellInlineCommands) && hasFlag(args, "-c") {
		return true
	}
	if anyNameInSet(names, interpreterInlineCommands) && (hasFlag(args, "-c") || hasFlag(args, "-e")) {
		return true
	}
	return false
}

// packageRunnerVerb returns the package-runner subcommand: the first non-option
// token, skipping common value-taking options and their values so an option value
// is not mistaken for the subcommand (e.g. "yarn --cwd /dir install" -> install).
func packageRunnerVerb(args []string) (string, bool) {
	valueOpts := setOf("--cwd", "-C", "--prefix", "--registry", "--cache")
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if _, ok := valueOpts[a]; ok {
			skip = true // the option consumes the following token as its value
			continue
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			return a, true
		}
	}
	return "", false
}

// hasFlag reports whether flag appears in args (stopping at the "--" option
// terminator). A two-character short flag (e.g. "-c") is also detected inside a
// combined short-flag bundle (e.g. "-xc" includes "-c").
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == flag {
			return true
		}
		if len(flag) == 2 && flag[0] == '-' && flag[1] != '-' {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") &&
				strings.IndexByte(a[1:], flag[1]) >= 0 {
				return true
			}
		}
	}
	return false
}

// isAssignment reports whether a token is a NAME=VALUE environment assignment with
// a well-formed variable name.
func isAssignment(t string) bool {
	name, _, ok := strings.Cut(t, "=")
	if !ok || name == "" {
		return false
	}
	return ValidateVariableName(name) == nil
}

// isDyldVar reports whether name is a macOS dyld library-injection variable.
func isDyldVar(name string) bool {
	return strings.HasPrefix(strings.ToUpper(name), "DYLD_")
}

// isPrivilegeCommand reports whether the command escalates privilege
// (sudo/su/doas), matched through its risk profile by basename and symlinks.
func isPrivilegeCommand(cmd string) bool {
	p, ok := ResolveProfile(cmd)
	return ok && p.IsPrivilege()
}

// hasDynamicLoaderName reports whether any resolved name is the dynamic linker
// (ld-linux*.so, ld.so, ld-musl-*).
func hasDynamicLoaderName(names map[string]struct{}) bool {
	for n := range names {
		if strings.HasPrefix(n, "ld-linux") || strings.HasPrefix(n, "ld-musl") || n == "ld.so" || n == "dyld" {
			return true
		}
	}
	return false
}

// critical builds a Critical (privilege escalation) result.
func critical() IndirectExecutionResult {
	return IndirectExecutionResult{
		Kind:        IndirectCritical,
		Level:       runnertypes.RiskLevelCritical,
		ReasonCodes: []risktypes.ReasonCode{risktypes.ReasonPrivilegeEscalation},
	}
}

// reject builds a Blocking (rejected indirect form) result with the generic
// rejection reason. Forms that carry a more specific reason (a forbidden env var)
// use rejectClass directly.
func reject() IndirectExecutionResult {
	return rejectClass(risktypes.ReasonIndirectExecutionRejected, "")
}

// rejectClass builds a Blocking result carrying an error class.
func rejectClass(code risktypes.ReasonCode, errClass risktypes.ErrorClass) IndirectExecutionResult {
	return IndirectExecutionResult{
		Kind:        IndirectReject,
		ReasonCodes: []risktypes.ReasonCode{code},
		ErrorClass:  errClass,
	}
}

// floor builds a risk-floor result contributing level to the effective-risk max.
func floor(level runnertypes.RiskLevel, code risktypes.ReasonCode) IndirectExecutionResult {
	return IndirectExecutionResult{
		Kind:        IndirectFloor,
		Level:       level,
		ReasonCodes: []risktypes.ReasonCode{code},
	}
}
