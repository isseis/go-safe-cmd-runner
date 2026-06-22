package security

import (
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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
	// IndirectCritical means a privilege-escalation token (sudo/su/doas/pkexec/runuser/setpriv/capsh) was found
	// as the effective target; the command is Critical (always denied).
	IndirectCritical
	// IndirectReject means the form cannot be identity-bound until exec (an
	// unextractable wrapper, a forbidden loader-control variable, a find/xargs
	// child-process exec, a direct dynamic-loader invocation, a remote-shell
	// helper). It is a Blocking deny.
	IndirectReject
	// IndirectFloor means the form is allowable but contributes a minimum risk
	// level (env with no command -> Medium; inline shell/interpreter, package
	// script runner -> High; a wrapped extractable inner command -> a flat High
	// floor regardless of the inner's content) that is folded into the
	// effective-risk maximum.
	IndirectFloor
)

// IndirectExecutionResult is the rank-2 outcome the evaluator folds into the
// effective risk.
//
// Scope: this resolver produces the evaluation-time decision (Critical / Reject /
// Floor) and records the artifacts that participate in the chain. Artifacts carry
// their path and role for audit only; the runner does not fd-bind or hash-gate a
// wrapper's inner command (that design was withdrawn). A user who needs to pin an
// inner command's identity registers it explicitly in verify_files.
type IndirectExecutionResult struct {
	Kind        IndirectExecutionKind
	Level       runnertypes.RiskLevel
	ReasonCodes []risktypes.ReasonCode
	// Reasons carries human-readable reasons collected from the risk profile of a
	// shebang interpreter (the RoleInterpreter path of a direct script execution,
	// e.g. "Always performs network operations" for an interpreter that profiles as
	// a network command), so the audit log keeps the same descriptions as the
	// direct invocation. The evaluator folds these into the assessment's Reasons.
	// The wrapper-inner (RoleInner) path is a flat High floor and no longer
	// collects profile reasons.
	Reasons    []string
	ErrorClass risktypes.ErrorClass
	Artifacts  []risktypes.ExecutedArtifact
}

// wrapperSpec describes how to skip a wrapper's own options and positional
// arguments to reach the inner COMMAND it runs. The runner does not re-implement
// these wrappers or fd-bind the inner command; it parses the wrapper only to
// extract the inner command and assess its risk (a flat High floor, Critical for a
// privilege token, Reject for a forbidden form). That is why wrappers resolve
// rather than reject.
type wrapperSpec struct {
	// valueOpts are options that consume the following token as their value
	// (e.g. timeout -s SIGNAL), so that token is not mistaken for the COMMAND.
	valueOpts map[string]struct{}
	// positionals is the number of positional arguments that precede the COMMAND
	// (timeout DURATION, chrt PRIORITY, taskset MASK).
	positionals int
}

// wrapperSpecs is the curated set of wrappers whose inner command the runner can
// extract for risk assessment (the runner does not exec the inner command
// itself). env is parsed separately (it also carries NAME=VALUE
// assignments and -S split-strings). taskset is parsed separately too: whether a
// positional CPU mask precedes the command depends on whether -c/--cpu-list was
// given, which the fixed-positional model cannot express. xargs is intentionally
// excluded: it execs the helper from its own child process, so the runner cannot
// identity-bind it (handled as a child-process exec form below).
var wrapperSpecs = map[string]wrapperSpec{
	"timeout": {valueOpts: setOf("-s", "--signal", "-k", "--kill-after"), positionals: 1},
	"nice":    {valueOpts: setOf("-n", "--adjustment"), positionals: 0},
	"ionice":  {valueOpts: setOf("-c", "--class", "-n", "--classdata", "-p", "--pid"), positionals: 0},
	"nohup":   {positionals: 0},
	"stdbuf":  {valueOpts: setOf("-i", "--input", "-o", "--output", "-e", "--error"), positionals: 0},
	"setsid":  {positionals: 0},
	"time":    {valueOpts: setOf("-o", "--output", "-f", "--format"), positionals: 0},
	"chrt":    {valueOpts: setOf("-T", "--sched-runtime", "-P", "--sched-period", "-D", "--sched-deadline"), positionals: 1},
}

// wrapperNames is the sorted list of wrapperSpecs keys. analyzeIndirect iterates
// it (rather than ranging the map) so wrapper selection is deterministic when a
// command's symlink chain matches more than one wrapper name.
var wrapperNames = slices.Sorted(maps.Keys(wrapperSpecs))

// shellInlineCommands are shells whose inline-code flag is -c only. -e is the
// errexit boolean option, not inline code, so "bash -e script.sh" is not treated
// as an inline string.
var shellInlineCommands = setOf("bash", "sh", "ash", "dash", "zsh", "ksh", "csh", "tcsh", "fish")

// interpreterInlineCommands are interpreters whose inline-code flags are -e (eval)
// and -c.
var interpreterInlineCommands = setOf(
	"python", "python2", "python3", "node", "nodejs", "deno", "bun",
	"ruby", "perl", "php", "lua", "luajit", "tclsh",
	"Rscript", "julia", "guile", "elixir",
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

// unknownOptionPolicy decides how a leading-option scan treats an option it does
// not recognize. An unrecognized option's arity (whether it consumes the next
// token as a value) determines where the operands begin, so the policy is what
// lets a scan stay either lenient or fail-closed at its call site.
type unknownOptionPolicy int

const (
	// shortOptsAreBoolean treats an unrecognized short option (-x) as value-less
	// and only an unrecognized long option (--foo with no attached "=") as making
	// the operand boundary unreliable. Short options are predominantly flags, and
	// a wrapper/xargs invocation's leading tokens are mostly its own known options.
	shortOptsAreBoolean unknownOptionPolicy = iota
	// anyUnknownIsUnreliable treats every unrecognized option (short or long, with
	// no attached "=") as making the boundary unreliable. Used where an
	// unrecognized separated value-option could hide the following subcommand and
	// the fallback classification is not the safe side (package script runners,
	// which have short value-options like npm -w that the spec cannot fully list).
	anyUnknownIsUnreliable
	// allUnknownAreBoolean treats every unrecognized option (short or long) as
	// value-less and skips it. Used where under-locating the operand only weakens a
	// non-fail-closed dimension (e.g. git subcommand detection for the conditional
	// network factor, which also has a URL/SSH-argument fallback), so a lenient
	// scan that tolerates unlisted boolean options is preferred over over-blocking.
	allUnknownAreBoolean
)

// optSpec declares a command's own option grammar so a scan can locate the
// operands (the non-option tokens) without mistaking an option's value for one.
type optSpec struct {
	// valueOpts consume the following token as their value in the separated form
	// (-o VALUE). The attached forms (-oVALUE, --opt=VALUE) are self-contained and
	// need no special listing.
	valueOpts map[string]struct{}
	// boolOpts are known value-less options. They are skipped regardless of the
	// unknown-option policy, so a command that mixes known boolean options with a
	// fail-closed unknown policy (e.g. systemctl) does not treat its own flags as
	// unknown. May be nil.
	boolOpts map[string]struct{}
	// optionalArgOpts are options with a getopt optional_argument: they bind a value
	// only in the attached form (--opt=VALUE, -oVALUE), never by consuming the
	// following token. They differ from boolOpts ONLY inside a short cluster: an
	// optional-argument option consumes the remainder of the token as its optional
	// value, so a later value-option letter in the same cluster is NOT a separate
	// option. Because tools disagree on this (util-linux nsenter treats "-mS" as -m
	// with attached value "S"; unshare treats it as -m then -S), a clustered
	// optional-argument option that is not the last letter is ambiguous and the scan
	// fails closed. Listing such an option as a valueOpt would let it swallow a
	// separated operand (the inner command); omitting it entirely would let a later
	// value-option letter in a cluster do the same. May be nil.
	optionalArgOpts map[string]struct{}
	// singleDashLong treats a single-dash multi-character token (-json, -family) as
	// one whole long option rather than a getopt short-option cluster. It is for
	// tools that use single-dash long options (ip), where the whole token is looked
	// up in valueOpts/boolOpts/optionalArgOpts exactly like a "--" option. When
	// false (the default), single-dash tokens are parsed letter by letter as a short
	// cluster. May not be combined with short-cluster grammar on the same spec.
	singleDashLong bool
	// unknown selects how an unrecognized option is treated (see the policies).
	unknown unknownOptionPolicy
}

// skipLeadingOptions returns the index of the first operand (the first non-option
// token) in args, consuming the value of any recognized separated value-option.
// It is the single getopt-style operand scanner shared by the wrapper, xargs, and
// package-runner classifiers, so the option-surface handling (the "--" terminator,
// attached values, unknown-arity fail-closed) lives in one place instead of being
// re-derived — slightly differently each time — per call site.
//
// reliable is false when an option of unknown arity made the operand boundary
// indeterminate; the caller must then fail closed, because it cannot tell which
// token is the operand. The "--" terminator makes the following token an operand
// unconditionally (even if it begins with "-") and keeps the scan reliable. When
// args holds no operand, idx == len(args).
func skipLeadingOptions(args []string, spec optSpec) (idx int, reliable bool) {
	i := 0
	for i < len(args) {
		t := args[i]
		if t == "--" {
			return i + 1, true
		}
		if !strings.HasPrefix(t, "-") || t == "-" {
			break // operand
		}
		i++
		if strings.HasPrefix(t, "--") || spec.singleDashLong {
			// Long option (a "--opt" token, or a single-dash long option such as ip's
			// "-json"/"-family" when the spec is in singleDashLong mode). The whole
			// token is the option name, not a cluster of one-letter options.
			if _, ok := spec.valueOpts[t]; ok {
				i++ // separated value: skip the option's value too
				continue
			}
			if _, ok := spec.boolOpts[t]; ok {
				continue // known value-less option
			}
			if _, ok := spec.optionalArgOpts[t]; ok {
				continue // optional-argument long option without "=": binds no value
			}
			if strings.Contains(t, "=") {
				continue // attached value (--opt=value): self-contained
			}
			// Unrecognized long option with no attached value: arity unknown unless
			// the policy assumes unknown options are value-less.
			if spec.unknown == allUnknownAreBoolean {
				continue
			}
			return i, false
		}
		// Short-option token, possibly a cluster (-tc) where a value-taking option
		// can sit at the end and consume the next token ("-tc 2" -> -c takes "2").
		// Parse the cluster left to right so a value-option hidden in it is not
		// missed — missing it would put the operand boundary on the option's value
		// and let the real command (e.g. sudo) pass as that value's argument.
		consumesNext, ok := scanShortCluster(t, spec)
		if !ok {
			return i, false
		}
		if consumesNext {
			i++ // a value-taking option ended the cluster; its value is the next token
		}
	}
	return i, true
}

// firstOperand returns the first operand (non-option token) of args per spec.
// operand is "" when there is no operand or when the scan was unreliable (an
// unknown-arity option made the boundary indeterminate), so a caller that treats
// "" as "no subcommand" fails closed. It is the string-returning convenience over
// skipLeadingOptions for callers that want the subcommand/verb directly.
func firstOperand(args []string, spec optSpec) (operand string) {
	idx, reliable := skipLeadingOptions(args, spec)
	if !reliable || idx >= len(args) {
		return ""
	}
	return args[idx]
}

// optClass is the getopt arity class of a short option within a cluster. It is the
// single classification every short-cluster scan shares, so the rule "a value or
// optional-argument option ends the cluster (the remainder is its attached value)"
// is decided in exactly one place (classifyShortOpt) rather than re-derived per
// scan site. Any new caller that walks a short cluster MUST classify each letter
// through classifyShortOpt instead of testing the option maps directly.
type optClass int

const (
	// optClassValue takes a value: as the last letter of a cluster it consumes the
	// following token; otherwise the remainder of the token is its attached value.
	// Either way the cluster ends.
	optClassValue optClass = iota
	// optClassOptional has a getopt optional_argument: it binds a value only in the
	// attached form, so it never consumes the following token; the remainder of the
	// token (if any) is its attached value, ending the cluster.
	optClassOptional
	// optClassBool is a value-less flag: the cluster scan continues past it.
	optClassBool
	// optClassUnknown is an unrecognized option; the caller resolves its arity by
	// the unknown-option policy.
	optClassUnknown
)

// classifyShortOpt returns the arity class of the one-letter short option opt
// (e.g. "-c") per spec. valueOpts and optionalArgOpts are checked before boolOpts
// so a misregistration is caught by tests rather than silently treated as a flag.
func classifyShortOpt(opt string, spec optSpec) optClass {
	if _, ok := spec.valueOpts[opt]; ok {
		return optClassValue
	}
	if _, ok := spec.optionalArgOpts[opt]; ok {
		return optClassOptional
	}
	if _, ok := spec.boolOpts[opt]; ok {
		return optClassBool
	}
	return optClassUnknown
}

// scanShortCluster parses a clustered short-option token (e.g. "-tc") left to
// right. consumesNext is true when a value-taking option is the LAST character of
// the cluster and therefore takes the following token as its separated value; a
// value-taking option that is not last takes the remainder of the token as an
// attached value and ends the cluster either way. ok is false when the cluster's
// arity is indeterminate (an unrecognized option under the anyUnknownIsUnreliable
// policy, or an optional-argument option that is not the cluster's last letter,
// whose remainder is tool-specifically its value or more options), so the caller
// fails closed; under shortOptsAreBoolean an unrecognized short option is assumed
// value-less.
func scanShortCluster(t string, spec optSpec) (consumesNext, ok bool) {
	for j := 1; j < len(t); j++ {
		switch classifyShortOpt("-"+string(t[j]), spec) {
		case optClassValue:
			return j == len(t)-1, true
		case optClassOptional:
			if j == len(t)-1 {
				return false, true // last letter: no attached value, does not consume next
			}
			return false, false // ambiguous mid-cluster (see optSpec.optionalArgOpts): fail closed
		case optClassBool:
			continue // known value-less short option
		case optClassUnknown:
			if spec.unknown == anyUnknownIsUnreliable {
				return false, false
			}
			// shortOptsAreBoolean / allUnknownAreBoolean: assume value-less, keep scanning.
		}
	}
	return false, true
}

// leadingClusterHasFlag reports whether the value-less flag letter `flag` appears
// in the short-option cluster token t before any option that binds an attached
// value. It shares the cluster-termination rule with scanShortCluster via
// classifyShortOpt: a value or optional-argument option binds the remainder of the
// token as its attached value (not more flag letters), so the scan stops there.
// This is how a flag is detected without mistaking an option's attached value for
// it (e.g. the "x" in "-nx" is -n's value, and in "-dx" is -d's optional value,
// neither is the -x flag).
func leadingClusterHasFlag(t string, flag byte, spec optSpec) bool {
	for j := 1; j < len(t); j++ {
		if t[j] == flag {
			return true
		}
		switch classifyShortOpt("-"+string(t[j]), spec) {
		case optClassValue, optClassOptional:
			return false // the remainder of the token is this option's attached value
		}
	}
	return false
}

// shortFlagInBundle reports whether arg is a single-dash short-option token (not a
// "--" long option) whose letters include c. This matches both an attached value
// (-essh selects -e) and a bundle of short flags (-avze includes -e), so an
// option hidden inside such a token is not missed.
func shortFlagInBundle(arg string, c byte) bool {
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") || arg == "-" {
		return false
	}
	// A short-option token's flag letters precede any attached "=value" (e.g. in
	// "-e=foo" the flags are "e"); the value chars after "=" must not be matched as
	// flags, so search only the part before the first "=".
	flags := arg[1:]
	if i := strings.IndexByte(flags, '='); i >= 0 {
		flags = flags[:i]
	}
	return strings.IndexByte(flags, c) >= 0
}

// AnalyzeIndirectExecution detects whether the command executes or loads an
// artifact other than the verified one (a wrapper's inner command, an inline
// script, a loader-injected library, a find/xargs child-process helper, a remote
// shell helper) and returns how the evaluator must treat it. Detection is by
// basename and resolved symbolic links, mirroring the other name-based
// classifiers.
func AnalyzeIndirectExecution(cmdPath string, args []string) IndirectExecutionResult {
	// The top-level command is a wrapper-inner context: RoleInner drives the flat
	// High floor for any extractable inner command. The shebang branch overrides
	// this with RoleInterpreter for a direct script execution.
	return analyzeIndirect(cmdPath, args, 0, risktypes.RoleInner)
}

// analyzeIndirect classifies an indirect-execution form. role threads through the
// recursion so a shebang interpreter chain reached through env
// (#!/usr/bin/env <interp>) stays RoleInterpreter (fine-grained) while a wrapper's
// inner command stays RoleInner (flat High).
func analyzeIndirect(cmdPath string, args []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	if depth >= indirectExecMaxDepth {
		return reject()
	}

	// Resolve the symlink chain strictly: a broken chain or a depth/cycle overflow
	// yields an incomplete name set in lenient mode, which could miss a stricter
	// disposition (a privilege token or dynamic loader past the break) and fail
	// open. Fail closed the same way EvaluateRisk does for the top-level command.
	// A bare command name has no chain to walk and resolves to itself without error.
	names, err := ResolveCommandNames(cmdPath)
	if err != nil {
		return rejectClass(risktypes.ReasonSymlinkResolutionFailed, risktypes.ErrorClassSymlinkResolution)
	}

	// Direct script with a shebang (#!/usr/bin/env python): the kernel runs the
	// shebang interpreter, a separate artifact, so the interpreter chain is
	// evaluated and gated. Check this before basename-based wrapper matching so
	// an explicit script path whose basename matches a wrapper (e.g. /tmp/env
	// with #!/usr/bin/python3) is evaluated through its interpreter chain rather
	// than misidentified as the wrapper.
	if res, ok := analyzeShebang(cmdPath, args, depth); ok {
		return res
	}

	// The remaining detection is name-based, matching against every name in the
	// command's symlink chain (extractAllCommandNames). A single chain can carry
	// more than one matchable name (a symlink whose own name collides with a
	// wrapper while its target is find/xargs/ld-linux*), so the checks are ordered
	// by disposition strictness — the unbindable reject/Critical forms first —
	// rather than by category. That way a name collision fails closed toward the
	// stricter outcome instead of being short-circuited into a lenient
	// resolve-inner path by map-iteration order.

	// find/xargs run the helper from their own child process; the runner cannot
	// identity-bind it. A privilege token there is still Critical; any other
	// helper is rejected (cannot be bound).
	if _, ok := names["xargs"]; ok {
		return analyzeChildProcessExec(xargsTarget(args))
	}
	if _, ok := names["find"]; ok {
		target, hasTarget, hasPrimary := findExecTarget(args)
		if hasTarget {
			return analyzeChildProcessExec(target, true)
		}
		if hasPrimary {
			// An exec primary is present but its command token is missing (e.g.
			// "find /tmp -exec" with nothing after): the form would run a helper we
			// cannot extract or identity-bind, so fail closed rather than fall through
			// to IndirectNone and treat it as a plain search.
			return reject()
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

	// env is parsed specially: it carries NAME=VALUE assignments and a -S
	// split-string in addition to an optional inner command.
	if _, ok := names["env"]; ok {
		return analyzeEnv(args, depth, role)
	}

	// taskset is parsed specially: the CPU affinity is either a positional MASK or
	// the value of -c/--cpu-list, so whether a positional precedes the command is
	// conditional (the fixed-positional wrapper model would shift the inner command
	// and miss a privilege token for the -c form).
	if _, ok := names["taskset"]; ok {
		return analyzeTaskset(args, depth, role)
	}

	// ip netns exec / ip vrf exec transparently exec an inner command in a network
	// namespace / VRF, so the inner command is gated. Any other ip form (a non-exec
	// subcommand or a bare ip) returns IndirectNone from the handler and is left to
	// the normal ip (Medium) classification; no later check matches "ip", so
	// returning here is equivalent to falling through to IndirectNone.
	if _, ok := names["ip"]; ok {
		return analyzeIPExec(args, depth, role)
	}

	// Namespace / root-change wrappers (chroot/unshare/nsenter) and command-string
	// wrappers (flock/watch) transparently exec an inner COMMAND. They need
	// dedicated handlers (their option grammar does not fit the fixed wrapperSpec
	// model); the dispatch is factored out to keep this function's branching bounded.
	if res, ok := analyzeDedicatedWrapper(names, args, depth, role); ok {
		return res
	}

	// Other wrappers: extract the inner command and assess its risk (the runner
	// does not re-implement the wrapper; the wrapper itself adds no execution beyond
	// the inner command). wrapperNames is iterated in sorted order so selection is
	// deterministic when the chain contains more than one wrapper name.
	for _, name := range wrapperNames {
		if _, ok := names[name]; ok {
			return analyzeWrapper(wrapperSpecs[name], args, depth, role)
		}
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
	// common.MaxShebangLen is the larger of the two supported platforms' kernel
	// shebang limits (Linux 256, macOS 512). Using the larger bound avoids
	// truncating a long shebang line on macOS (which would drop characters that
	// should trigger the env -S fail-closed logic); on Linux it can only read past
	// what the kernel uses, which is fail-closed-safe, never an under-read. The same
	// constant bounds the record/verify shebang parser (internal/shebang), so the
	// two readers agree on what counts as a shebang. NOTE: this reader intentionally
	// keeps the post-interpreter remainder as a single token (kernel-accurate, to
	// preserve an env -S payload), whereas internal/shebang.Parse splits with Fields
	// for interpreter-identity resolution — a deliberate, documented difference.
	const maxShebangLen = common.MaxShebangLen
	// Only inspect explicit paths (containing '/'): bare command names are
	// resolved via PATH at exec time, not here, and opening a relative name
	// could accidentally read an unrelated local file from the CWD.
	if !strings.Contains(path, "/") {
		return "", nil, false
	}
	// The path is the resolved command path the evaluator is already classifying;
	// reading its first line to detect a shebang interpreter is intentional. Open
	// non-blocking and then fstat the opened descriptor, requiring a regular file:
	// opening a FIFO would otherwise block until a writer connects (a denial of
	// service), and a device file could have read side effects. O_NONBLOCK makes the
	// open of a FIFO/device return immediately, and fstat-ing the descriptor we
	// actually opened (rather than os.Stat-ing the path, then opening) closes the
	// TOCTOU window — a swap between check and open cannot make us block or read the
	// wrong object. O_NONBLOCK has no effect on regular-file reads.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0) //nolint:gosec // reading the verified command path to detect a shebang
	if err != nil {
		return "", nil, false
	}
	defer func() { _ = f.Close() }()
	if fi, statErr := f.Stat(); statErr != nil || !fi.Mode().IsRegular() {
		return "", nil, false
	}

	// Read up to maxShebangLen bytes. io.ReadAll over a LimitReader keeps reading
	// until the bound or EOF, so a short read (a single os.File.Read returning
	// fewer bytes than requested without error) cannot truncate the shebang line
	// and drop characters that should trigger the env -S fail-closed logic.
	buf, err := io.ReadAll(io.LimitReader(f, maxShebangLen))
	if err != nil {
		return "", nil, false
	}
	if len(buf) < 2 || buf[0] != '#' || buf[1] != '!' {
		return "", nil, false
	}
	line := string(buf[2:])
	if idx := strings.IndexAny(line, "\n\r"); idx >= 0 {
		line = line[:idx]
	}
	// Linux shebang parsing (fs/binfmt_script.c) takes the interpreter as the first
	// whitespace-delimited token and EVERYTHING after it (leading whitespace
	// trimmed) as a SINGLE optional argument — it does not split that remainder
	// further. Splitting it with strings.Fields would move quoted/escaped parts out
	// of an "env -S" payload (e.g. `#!/usr/bin/env -S rm '-rf' /` would reach env as
	// "-S" + "rm" instead of one "-S rm '-rf' /" token), bypassing the env -S
	// fail-closed check, so keep the remainder as one token.
	line = strings.Trim(line, " \t")
	if line == "" {
		return "", nil, false
	}
	sep := strings.IndexAny(line, " \t")
	if sep < 0 {
		return line, nil, true // interpreter only, no argument
	}
	interp = line[:sep]
	arg := strings.TrimLeft(line[sep+1:], " \t")
	if arg == "" {
		return interp, nil, true
	}
	return interp, []string{arg}, true
}

// envBooleanOptions are env's own value-less options.
var envBooleanOptions = setOf("-i", "--ignore-environment", "-0", "--null", "-v", "--debug")

// envValueOptions are env's own options that consume the following token.
// -C/--chdir is intentionally not here: it is rejected (see analyzeEnv) rather
// than skipped, because the directory change alters how a relative inner command
// resolves.
var envValueOptions = setOf("-u", "--unset")

// isEnvChdirOption reports whether t is env's -C/--chdir option in any surface
// form: separated (-C, --chdir), attached short (-Cdir), or attached long
// (--chdir=dir). env has no other option beginning with "-C", so the short
// prefix test is unambiguous.
func isEnvChdirOption(t string) bool {
	return strings.HasPrefix(t, "-C") || t == "--chdir" || strings.HasPrefix(t, "--chdir=")
}

// analyzeEnv parses an env invocation: NAME=VALUE assignments, -S split-strings,
// option flags, and the optional inner command.
func analyzeEnv(args []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
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
			return analyzeEnvSplitString(payload, args[i+consumed:], depth, role)
		}
		if isEnvChdirOption(t) {
			// env -C/--chdir changes the working directory before exec, which changes
			// how a relative inner command (its PATH lookup, a "." entry, or a shebang
			// read) resolves. We cannot model that, so fail closed rather than
			// evaluate/gate a different artifact than the one that would run.
			return reject()
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
				return resolveInner(args[j], args[j+1:], pathOverridden, depth, role)
			}
			return floor(runnertypes.RiskLevelMedium, risktypes.ReasonIndirectExecutionWrapper)
		}
		if strings.HasPrefix(t, "-") {
			// Unknown env option: it may or may not consume a value, so the inner
			// command can no longer be located reliably. Fail closed.
			return reject()
		}
		// First non-option, non-assignment token is the inner command.
		return resolveInner(t, args[i+1:], pathOverridden, depth, role)
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
	if isLoaderControlVar(name) {
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
func analyzeEnvSplitString(s string, remaining []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	// Backtick is included so a command-substitution payload also fails closed.
	if strings.ContainsAny(s, "\\'\"$#`") {
		return reject()
	}
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return reject()
	}
	return analyzeEnv(append(tokens, remaining...), depth+1, role)
}

// resolveInner evaluates a wrapper's extracted inner command. When the resolution
// path is attacker-controlled (env overrode PATH) and the inner command is a bare
// name, it cannot be resolved safely and is rejected.
func resolveInner(inner string, innerArgs []string, pathOverridden bool, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	if pathOverridden && !strings.Contains(inner, "/") {
		return reject()
	}
	return evaluateInnerAs(inner, innerArgs, depth, role)
}

// analyzeWrapper skips a wrapper's options and positional arguments and evaluates
// the inner command it runs.
func analyzeWrapper(spec wrapperSpec, args []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	idx, reliable := skipLeadingOptions(args, optSpec{valueOpts: spec.valueOpts, unknown: shortOptsAreBoolean})
	// An option of unknown arity may have consumed the real command token; the
	// parse position is unreliable, so fail closed.
	if !reliable {
		return reject()
	}
	idx += spec.positionals
	if idx >= len(args) {
		// No inner command (e.g. "timeout 5" with no COMMAND). Like env with no
		// command, treat as a Medium floor rather than rejecting outright.
		return floor(runnertypes.RiskLevelMedium, risktypes.ReasonIndirectExecutionWrapper)
	}
	// The extracted inner command should be a program name/path, never an option.
	// If it still begins with "-", option parsing mis-located the command (e.g. an
	// unknown value-taking option consumed the real positional), so fail closed
	// rather than evaluate the wrong token and under-classify the real command.
	if cmd := args[idx]; strings.HasPrefix(cmd, "-") {
		return reject()
	}
	return evaluateInnerAs(args[idx], args[idx+1:], depth, role)
}

// analyzeTaskset resolves the inner command of a taskset invocation. The CPU
// affinity is supplied either as a positional MASK ("taskset 0x3 CMD") or via
// -c/--cpu-list ("taskset -c 0-3 CMD", attached "-c0-3"/"--cpu-list=0-3", or
// clustered "-ac 0-3"); whether a positional MASK precedes the command therefore
// depends on the options, which the fixed-positional wrapper model cannot express
// (it would shift the inner command by one token for the -c form and miss a
// privilege token). The -p/--pid form acts on an existing process and runs no
// command. taskset's only value-taking short option is -c, so a 'c' anywhere in a
// short cluster denotes the cpu-list option.
func analyzeTaskset(args []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	cpuList := false // -c/--cpu-list supplies the affinity, so there is no positional MASK
	i := 0
	for i < len(args) {
		t := args[i]
		if t == "--" {
			i++ // option terminator: the next token is the command (or MASK)
			break
		}
		if !strings.HasPrefix(t, "-") || t == "-" {
			break // first operand
		}
		i++
		switch {
		case t == "--pid" || shortFlagInBundle(t, 'p'):
			// -p/--pid sets the affinity of an existing process; no command is
			// executed. -p is taskset's only short option containing 'p', so a 'p' in
			// a short cluster (e.g. "-ap 1234") denotes it; matching the cluster avoids
			// treating the PID as a positional MASK and a following token as a command.
			return floor(runnertypes.RiskLevelMedium, risktypes.ReasonIndirectExecutionWrapper)
		case t == "--cpu-list":
			cpuList = true
			i++ // separated value
		case strings.HasPrefix(t, "--cpu-list="):
			cpuList = true // attached value
		case strings.HasPrefix(t, "--"):
			if !strings.Contains(t, "=") {
				return reject() // unknown long option: arity unknown
			}
		default:
			// Short option(s). -c is the only value-taking short option; if it is the
			// last character its value is the next token, otherwise the rest of the
			// token is its attached value.
			if pos := strings.IndexByte(t[1:], 'c'); pos >= 0 {
				cpuList = true
				if 1+pos == len(t)-1 {
					i++ // -c is last in the cluster: its value is the next token
				}
			}
		}
	}
	if !cpuList {
		i++ // no -c/--cpu-list: a positional MASK precedes the command
	}
	if i >= len(args) {
		// No inner command (e.g. "taskset 0x3" alone). Like env with no command.
		return floor(runnertypes.RiskLevelMedium, risktypes.ReasonIndirectExecutionWrapper)
	}
	if strings.HasPrefix(args[i], "-") {
		return reject() // mis-located command boundary: fail closed
	}
	return evaluateInnerAs(args[i], args[i+1:], depth, role)
}

// Option specs for the namespace / root-change wrappers. Arities are taken from
// each tool's --help (GNU coreutils chroot; util-linux unshare/nsenter 2.41.3).
//
// getopt optional_argument options (unshare/nsenter namespace flags such as -m,
// and nsenter -r/-w) bind a value only in the attached form (-m=FILE / -mFILE); a
// separated next token is an operand (the inner COMMAND). They are listed in
// optionalArgOpts (both short and long forms). They must NOT be listed as
// valueOpts, which would swallow the inner command (e.g. "unshare -m sudo id"
// would lose sudo); listing the short form is required so scanShortCluster can
// detect the clustered-ambiguity case (-mS) and fail closed rather than continue
// into a later value-option that would consume the next token. Only options that
// consume the following token are listed in valueOpts. nsenter -S/-G empirically
// consume their separated value (verified against the real tool), so they are
// valueOpts even though --help renders them with the optional "[=<uid>]".
var chrootOptSpec = optSpec{
	valueOpts: setOf("--userspec", "--groups"),
	boolOpts:  setOf("--skip-chdir"),
	unknown:   shortOptsAreBoolean,
}

var unshareOptSpec = optSpec{
	valueOpts: setOf(
		"-l", "--load-interp", "--propagation", "-R", "--root", "-w", "--wd",
		"-S", "--setuid", "-G", "--setgid", "--map-user", "--map-group",
		"--map-users", "--map-groups", "--owner",
	),
	optionalArgOpts: setOf(
		"-m", "--mount", "-u", "--uts", "-i", "--ipc", "-n", "--net",
		"-p", "--pid", "-U", "--user", "-C", "--cgroup", "-T", "--time",
		"--mount-proc", "--mount-binfmt", "--kill-child",
	),
	boolOpts: setOf(
		"-r", "--map-root-user", "-c", "--map-current-user", "--map-auto",
		"-f", "--fork",
	),
	unknown: shortOptsAreBoolean,
}

var nsenterOptSpec = optSpec{
	valueOpts: setOf(
		"-t", "--target", "-N", "--net-socket", "-W", "--wdns",
		"-S", "--setuid", "-G", "--setgid",
	),
	optionalArgOpts: setOf(
		"-m", "--mount", "-u", "--uts", "-i", "--ipc", "-n", "--net",
		"-p", "--pid", "-U", "--user", "-C", "--cgroup", "-T", "--time",
		"-r", "--root", "-w", "--wd",
	),
	boolOpts: setOf(
		"-a", "--all", "--user-parent", "--preserve-credentials", "--keep-caps",
		"-e", "--env", "-F", "--no-fork", "-c", "--join-cgroup",
		"-Z", "--follow-context",
	),
	unknown: shortOptsAreBoolean,
}

// analyzeDedicatedWrapper dispatches the namespace/root-change wrappers
// (chroot/unshare/nsenter) and command-string wrappers (flock/watch) to their
// dedicated handlers. handled is false when none of these wrapper names is present
// in the command's resolved name set, so the caller continues with the remaining
// indirect-execution checks.
func analyzeDedicatedWrapper(names map[string]struct{}, args []string, depth int, role risktypes.ArtifactRole) (res IndirectExecutionResult, handled bool) {
	if _, ok := names["chroot"]; ok {
		return analyzeNamespaceWrapper(chrootOptSpec, 1, args, depth, role), true
	}
	if _, ok := names["unshare"]; ok {
		return analyzeNamespaceWrapper(unshareOptSpec, 0, args, depth, role), true
	}
	if _, ok := names["nsenter"]; ok {
		return analyzeNamespaceWrapper(nsenterOptSpec, 0, args, depth, role), true
	}
	if _, ok := names["flock"]; ok {
		return analyzeFlock(args, depth, role), true
	}
	if _, ok := names["watch"]; ok {
		return analyzeWatch(args, depth, role), true
	}
	return IndirectExecutionResult{}, false
}

// analyzeNamespaceWrapper gates the inner COMMAND of chroot/unshare/nsenter.
// positionals is the number of operands the wrapper consumes before COMMAND
// (chroot's NEWROOT = 1; unshare/nsenter = 0). A missing COMMAND means the tool
// spawns an implicit shell, so the floor is High (not the generic wrapper Medium)
// to avoid letting a namespace/privilege escape (unshare -r, nsenter -t 1) pass
// unevaluated. An operand boundary made unreliable by an unknown-arity option, or
// a COMMAND token that still begins with "-" (option parsing mislocated it), fails
// closed.
func analyzeNamespaceWrapper(spec optSpec, positionals int, args []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	idx, reliable := skipLeadingOptions(args, spec)
	if !reliable {
		return reject()
	}
	// A mandatory positional (chroot's NEWROOT) that is not present means the form
	// is malformed, not a no-command implicit shell, so fail closed.
	if idx+positionals > len(args) {
		return reject()
	}
	cmdIdx := idx + positionals
	if cmdIdx == len(args) {
		// Positionals present but no inner command: the tool launches an implicit
		// shell. High floor.
		return floor(runnertypes.RiskLevelHigh, risktypes.ReasonIndirectExecutionWrapper)
	}
	if strings.HasPrefix(args[cmdIdx], "-") {
		return reject()
	}
	return evaluateInnerAs(args[cmdIdx], args[cmdIdx+1:], depth, role)
}

// flockOptSpec lists flock's own leading options (those that precede the lock
// operand). flock is non-permutation: -c/--command is recognized only as the token
// immediately after the lock operand (the form "flock -c CMD FILE" is invalid on
// the real tool), so -c is deliberately absent here and handled in analyzeFlock.
var flockOptSpec = optSpec{
	valueOpts: setOf("-w", "--timeout", "-E", "--conflict-exit-code"),
	boolOpts: setOf(
		"-s", "--shared", "-x", "--exclusive", "-u", "--unlock", "-n", "--nonblock",
		"-o", "--close", "-F", "--no-fork", "--fcntl", "--verbose",
	),
	unknown: shortOptsAreBoolean,
}

// analyzeFlock gates the inner command of a flock invocation. flock takes a lock
// operand (a file/directory path or a numeric file descriptor) followed by either
// "<command> [args]" or "-c <command-string>"; the bare "flock <fd>" form runs no
// command. The lock operand is located by skipping flock's own leading options;
// flock does not parse its own options after the lock operand, so a "-" there
// belongs to the inner command.
func analyzeFlock(args []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	idx, reliable := skipLeadingOptions(args, flockOptSpec)
	if !reliable {
		return reject()
	}
	if idx >= len(args) {
		// Options only, no lock operand: an incomplete form with no extractable
		// command. Fail closed.
		return reject()
	}
	lockOperand := args[idx]
	rest := args[idx+1:]
	if len(rest) == 0 {
		// "flock <fd>" (a bare numeric descriptor) runs no command and is not an
		// indirect-execution form. A non-numeric lock operand with no command is an
		// incomplete form, so fail closed.
		if isAllDigits(lockOperand) {
			return IndirectExecutionResult{Kind: IndirectNone}
		}
		return reject()
	}
	if rest[0] == "-c" || rest[0] == "--command" {
		// "flock <file> -c <command-string>": the next token is a /bin/sh -c string.
		// rest[0] is the -c option, so a missing string means rest has only that token.
		if len(rest) == 1 {
			return reject()
		}
		return gateShellCommandString(rest[1], depth, role)
	}
	// "flock <file> <command> [args]": gate the command token directly.
	return evaluateInnerAs(rest[0], rest[1:], depth, role)
}

// watchOptSpec lists watch's own options. -x/--exec is a value-less flag here but
// also switches watch's execution mode (argv vs /bin/sh -c), which analyzeWatch
// detects separately. -c is --color (a flag), not a value option.
var watchOptSpec = optSpec{
	valueOpts:       setOf("-n", "--interval", "-q", "--equexit"),
	optionalArgOpts: setOf("-d", "--differences"),
	boolOpts: setOf(
		"-b", "--beep", "-c", "--color", "-C", "--no-color", "-e", "--errexit",
		"-g", "--chgexit", "-p", "--precise", "-r", "--no-rerun", "-t", "--no-title",
		"-w", "--no-wrap", "-x", "--exec",
	),
	unknown: shortOptsAreBoolean,
}

// analyzeWatch gates the inner command of a watch invocation. Without -x/--exec,
// watch joins all of its operands with single spaces and runs the result through
// /bin/sh -c, so the whole operand tail is one command string that must be split
// fail-closed (a ";"/"|"/"&" or newline between operands would otherwise hide a
// command). With -x/--exec, watch execvp's the operands directly as an argv list,
// so the first operand is the command and the rest are its arguments (even tokens
// beginning with "-", which belong to the inner command, not to watch).
func analyzeWatch(args []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	idx, reliable := skipLeadingOptions(args, watchOptSpec)
	if !reliable {
		return reject()
	}
	if idx >= len(args) {
		// watch requires a command; an option-only form has nothing to gate.
		return reject()
	}
	operands := args[idx:]
	if watchExecRequested(args[:idx]) {
		return evaluateInnerAs(operands[0], operands[1:], depth, role)
	}
	return gateShellCommandString(strings.Join(operands, " "), depth, role)
}

// watchExecRequested reports whether watch's -x/--exec flag appears among its
// leading options. It must not mistake an option's value for the flag: a separated
// value-option's value (e.g. the "x" in "-n x") and a short value/optional-argument
// option's attached value (e.g. the "x" in "-nx" is -n's value and in "-dx" is -d's
// value, neither is -x) must not switch the mode to argv and bypass the fail-closed
// command-string split (a fail-open). Short clusters are interpreted through the
// shared leadingClusterHasFlag so this stays consistent with the operand-boundary
// scan (scanShortCluster).
func watchExecRequested(opts []string) bool {
	i := 0
	for i < len(opts) {
		t := opts[i]
		if t == "--exec" {
			return true
		}
		if _, ok := watchOptSpec.valueOpts[t]; ok {
			i += 2 // separated value-option: skip the option and its value
			continue
		}
		if strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "--") && t != "-" &&
			leadingClusterHasFlag(t, 'x', watchOptSpec) {
			return true
		}
		i++
	}
	return false
}

// ipOptSpec lists ip's global options, those that precede the object word (such as
// "netns"/"vrf"). ip uses single-dash long options (-json, -family, -netns) rather
// than getopt short-option clusters, so the spec is scanned in singleDashLong mode:
// each "-token" is one whole option. Value-taking globals (-family/-f, -batch/-b,
// -loops/-l, -rcvbuf/-rc, -netns/-n) consume the following token; the rest are
// flags. -color/-c is a flag here, NOT a value option: iproute2 documents it as
// -c[olor][={always|auto|never}] and binds the value only in the attached "=" form
// (handled by the "=" branch of skipLeadingOptions); bare -color does not consume
// the next token. Listing -color in valueOpts would be a fail-open, swallowing the
// object word of "ip -color netns exec ns <cmd>" and missing the inner gate.
// An unrecognized global makes the operand boundary unreliable (a value option
// could hide the object), so the scan fails closed and analyzeIPExec rejects.
var ipOptSpec = optSpec{
	singleDashLong: true,
	valueOpts: setOf(
		"-family", "-f", "-batch", "-b", "-loops", "-l",
		"-rcvbuf", "-rc", "-netns", "-n",
	),
	boolOpts: setOf(
		"-json", "-j", "-pretty", "-p", "-stats", "-s", "-statistics",
		"-details", "-d", "-oneline", "-o", "-resolve", "-r",
		"-numeric", "-N", "-all", "-a", "-color", "-c",
		"-timestamp", "-t", "-tshort", "-ts", "-iec",
		"-brief", "-br", "-human", "-human-readable", "-h",
		"-force", "-echo", "-e", "-4", "-6", "-0", "-B", "-M",
	),
	unknown: anyUnknownIsUnreliable,
}

// analyzeIPExec gates the inner command of "ip netns exec <NAME> <cmd>" and
// "ip vrf exec <NAME> <cmd>", which transparently exec <cmd> in a network namespace
// or VRF. ip's global options are skipped first (in singleDashLong mode) so an
// inserted global such as "-json" or "-n NAME" cannot shift the object word and let
// the inner command slip past the gate. Only the netns/vrf "exec" form is indirect
// execution; any other ip form (a non-exec subcommand, a bare "ip", or a different
// object) is IndirectNone and left to the normal ip (Medium) classification rather
// than blocked. The exec form whose inner command cannot be safely extracted (a
// missing NAME or COMMAND, an inner token still beginning with "-", or an unreliable
// option boundary) fails closed.
func analyzeIPExec(args []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	idx, reliable := skipLeadingOptions(args, ipOptSpec)
	if !reliable {
		// A global option of unknown arity could hide a "netns exec" object behind it,
		// so fail closed rather than fall through to the Medium ip evaluation.
		return reject()
	}
	rest := args[idx:]
	// Operands: object ("netns"/"vrf"), then the "exec" subcommand, then NAME, then
	// COMMAND. Only this exact prefix is indirect execution.
	if len(rest) == 0 || (rest[0] != "netns" && rest[0] != "vrf") {
		return IndirectExecutionResult{Kind: IndirectNone}
	}
	sub := rest[1:]
	if len(sub) == 0 || sub[0] != "exec" {
		// A non-exec subcommand (ip netns list, ip vrf show) or a bare object: not
		// indirect execution; leave it to the normal ip (Medium) classification.
		return IndirectExecutionResult{Kind: IndirectNone}
	}
	// exec form confirmed: sub == ["exec", NAME, cmd, args...]. Skip NAME, gate cmd.
	operands := sub[1:]
	if len(operands) == 0 {
		return reject() // "ip <object> exec" with no NAME: cannot extract the command.
	}
	cmdPart := operands[1:] // operands == [NAME, cmd, args...]; drop NAME.
	if len(cmdPart) == 0 {
		return reject() // NAME present but no command.
	}
	inner := cmdPart[0]
	if strings.HasPrefix(inner, "-") {
		// The COMMAND position still begins with "-": option parsing mislocated it or
		// the form is malformed. Fail closed.
		return reject()
	}
	return evaluateInnerAs(inner, cmdPart[1:], depth, role)
}

// isShellCommandStringSafe reports whether a /bin/sh -c command string consists
// only of the conservative allowlist the runner is willing to split. Only ASCII
// letters, digits, whitespace, and "_./:%@,=+-" may appear; any other character
// (shell grouping,
// substitution, separators, redirection, glob, history, or a newline) forces a
// fail-closed Reject instead of a naive split that could miss a hidden command.
// The set passes legitimate ProxyCommand/--rsh values (e.g. "ssh -W %h:%p bastion",
// "nc -X connect -x proxy:3128 %h %p") while rejecting anything with shell meaning.
func isShellCommandStringSafe(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == ' ' || r == '\t':
		case r == '_', r == '.', r == '/', r == ':', r == '%', r == '@', r == ',', r == '=', r == '+', r == '-':
		default:
			return false
		}
	}
	return true
}

// gateShellCommandString extracts the inner command from a /bin/sh -c command
// string using the fail-closed allowlist split, then gates it as a wrapper inner.
// A value containing any character outside the safe set, or one with no first
// token (empty or whitespace-only), is rejected.
func gateShellCommandString(s string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	if !isShellCommandStringSafe(s) {
		return reject()
	}
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return reject()
	}
	return evaluateInnerAs(tokens[0], tokens[1:], depth, role)
}

// isAllDigits reports whether s is a non-empty run of ASCII digits, used to
// recognize flock's bare file-descriptor operand ("flock 9").
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// evaluateInnerAs evaluates a wrapper's extracted inner command (or a shebang
// interpreter), carrying the role of the enclosing context: RoleInner for a
// wrapper inner (flattened to a High floor), RoleInterpreter for a shebang
// interpreter chain (kept on the fine-grained path), recorded on the artifact.
// A privilege token is Critical; an inner form whose concrete target cannot be
// safely extracted (a find/xargs child-process helper, a dynamic loader)
// propagates its rejection. The inner command is recorded as a chain artifact on
// every outcome (Floor/Critical/Reject) so the indirect-execution chain remains
// traceable in audits even on deny paths.
func evaluateInnerAs(inner string, innerArgs []string, depth int, role risktypes.ArtifactRole) IndirectExecutionResult {
	artifact := risktypes.ExecutedArtifact{Path: inner, Role: role}

	// Strict-resolve the inner command's symlink chain once, up front. A broken or
	// over-deep chain yields an incomplete name set in lenient mode, so the
	// name-based checks below (privilege, system modification, ...) could
	// under-classify; fail closed instead, preserving the artifact chain for audit.
	// A bare inner name resolves to itself without error.
	innerNames, err := ResolveCommandNames(inner)
	if err != nil {
		res := rejectClass(risktypes.ReasonSymlinkResolutionFailed, risktypes.ErrorClassSymlinkResolution)
		res.Artifacts = []risktypes.ExecutedArtifact{artifact}
		return res
	}

	if isPrivilegeCommand(innerNames) {
		res := critical()
		res.Artifacts = append(res.Artifacts, artifact)
		return res
	}

	// Recurse: the inner command may itself be a wrapper or another indirect form.
	// Propagate role so an interpreter chain reached through env
	// (#!/usr/bin/env <interp>) keeps RoleInterpreter and is not collapsed to the
	// wrapper-inner High floor.
	nested := analyzeIndirect(inner, innerArgs, depth+1, role)
	switch nested.Kind {
	case IndirectCritical, IndirectReject:
		nested.Artifacts = append(nested.Artifacts, artifact)
		return nested
	}

	// A wrapper's inner command (RoleInner) is a flat High floor regardless of its
	// content: the runner does not fd-bind or hash-gate the inner, so a fine-grained
	// level would not be backed by an identity guarantee. Critical/Reject above
	// still take priority. Nested chain artifacts are preserved (e.g. for a nested
	// wrapper like "env timeout nice ls") so the inner chain stays auditable.
	if role == risktypes.RoleInner {
		return IndirectExecutionResult{
			Kind:        IndirectFloor,
			Level:       runnertypes.RiskLevelHigh,
			ReasonCodes: []risktypes.ReasonCode{risktypes.ReasonIndirectExecutionWrapper},
			Artifacts:   append(nested.Artifacts, artifact),
		}
	}

	// RoleInterpreter (a shebang interpreter of a direct script execution) keeps the
	// fine-grained level computation below: that path is out of scope for the
	// flat-High change and must stay unchanged.
	level := nested.Level
	codes := append([]risktypes.ReasonCode{}, nested.ReasonCodes...)
	reasons := append([]string{}, nested.Reasons...)

	if IsDestructiveFileOperation(innerNames, innerArgs) {
		level = max(level, runnertypes.RiskLevelHigh)
		codes = append(codes, risktypes.ReasonDestructiveFileOperation)
	}
	// Coreutils single-binary classification of the inner command. This applies
	// when inner is an absolute path under the coreutils dir (e.g. a wrapped
	// "/usr/lib/cargo/bin/coreutils/chmod"); a stat failure on its setuid check is
	// fail-closed, matching the top-level evaluator's coreutils handling, so a
	// wrapped coreutils command is not under-classified relative to the direct one.
	if cRisk, handled, err := CoreutilsCommandRisk(inner, innerArgs); err != nil {
		// Preserve the coreutils failure reason/error class (rather than a generic
		// reject) so audits can tell a failure-induced deny from a policy reject,
		// matching the top-level evaluator's coreutils handling.
		res := rejectClass(risktypes.ReasonCoreutilsClassification, risktypes.ErrorClassCoreutilsFileInfo)
		nested.Artifacts = append(nested.Artifacts, artifact)
		res.Artifacts = nested.Artifacts
		return res
	} else if handled {
		level = max(level, cRisk)
		codes = append(codes, risktypes.ReasonCoreutilsClassification)
	}
	if s := SystemModificationRisk(innerNames); s > runnertypes.RiskLevelUnknown {
		level = max(level, s)
		codes = append(codes, risktypes.ReasonSystemModification)
	}
	if IsArbitraryCodeExecutionRunner(innerNames) {
		level = max(level, runnertypes.RiskLevelHigh)
		codes = append(codes, risktypes.ReasonArbitraryCodeExecution)
	}
	if l, _ := CheckDangerousArgPatterns(innerNames, innerArgs); l > runnertypes.RiskLevelUnknown {
		level = max(level, l)
		codes = append(codes, risktypes.ReasonDangerousArgPattern)
	}
	// Fold the inner command's risk profile (destruction / data exfiltration /
	// applicable network) so a profiled command (claude, curl, ssh, ...) is not
	// under-classified when wrapped. Privilege is handled above; system
	// modification is handled via SystemModificationRisk. The profile's
	// human-readable reasons are carried so the audit log of a wrapped command
	// keeps the same descriptions as the direct invocation.
	if profile, ok := ResolveProfile(innerNames); ok {
		if pl, pcodes := ProfileFactorRisk(profile, innerArgs); pl > runnertypes.RiskLevelUnknown {
			level = max(level, pl)
			codes = append(codes, pcodes...)
			reasons = append(reasons, profile.GetRiskReasons()...)
		}
	}

	// A wrapped command can accumulate the same reason from more than one source
	// (e.g. "env bash -c" gets ReasonArbitraryCodeExecution from both the nested
	// inline-code fold and IsArbitraryCodeExecutionRunner); de-duplicate so the
	// audit chain is not noisy.
	return IndirectExecutionResult{
		Kind:        IndirectFloor,
		Level:       level,
		ReasonCodes: common.DedupeStable(codes),
		Reasons:     common.DedupeStable(reasons),
		Artifacts:   append(nested.Artifacts, artifact),
	}
}

// analyzeChildProcessExec handles a helper run from find/xargs' own child process.
// A privilege token is Critical; any other helper cannot be identity-bound by the
// runner and is rejected. The target is recorded as a chain artifact when known
// so the indirect-execution chain is traceable in audits on both outcomes.
func analyzeChildProcessExec(target string, hasTarget bool) IndirectExecutionResult {
	if !hasTarget {
		return reject()
	}
	// The target is a separate (usually bare) command run from find/xargs' child
	// process. Resolve its name set; a privilege token is Critical, anything else
	// (including a resolution failure) is rejected — it cannot be identity-bound.
	var res IndirectExecutionResult
	if names, err := ResolveCommandNames(target); err == nil && isPrivilegeCommand(names) {
		res = critical()
	} else {
		res = reject()
	}
	res.Artifacts = []risktypes.ExecutedArtifact{{
		Path: target,
		Role: risktypes.RoleExecTarget,
	}}
	return res
}

// xargsValueOpts are xargs' own options that consume the following token as their
// value, so the value is not mistaken for the helper command.
var xargsValueOpts = setOf("-I", "--replace", "-n", "--max-args", "-P", "--max-procs",
	"-L", "--max-lines", "-s", "--max-chars", "-E", "-d", "--delimiter", "-a", "--arg-file")

// xargsTarget returns the helper command xargs would run: the first non-option
// token after xargs' own options. ok is false when there is no such token or the
// option boundary is unreliable; the xargs form is deny-only either way, so the
// caller rejects, but a reliable scan still records the correct helper artifact.
func xargsTarget(args []string) (string, bool) {
	idx, reliable := skipLeadingOptions(args, optSpec{valueOpts: xargsValueOpts, unknown: shortOptsAreBoolean})
	if !reliable || idx >= len(args) {
		return "", false
	}
	return args[idx], true
}

// findExecTarget returns the command find would run for an -exec/-execdir/-ok/-okdir
// primary: the token immediately after the primary. hasTarget is true when such a
// token was found. hasPrimary reports whether any exec primary was present at all,
// so the caller can distinguish a plain search (no primary -> not an indirect
// form) from a malformed exec form (primary present but no following command ->
// fail closed).
func findExecTarget(args []string) (target string, hasTarget, hasPrimary bool) {
	for i, arg := range args {
		if _, ok := findExecActions[arg]; ok {
			hasPrimary = true
			if i+1 < len(args) {
				return args[i+1], true, true
			}
		}
	}
	return "", false, hasPrimary
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
	// Record the init script path and role for audit only; the runner does not
	// fd-bind or hash-gate it (that design was withdrawn).
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
// service's own options (--status-all, --full-restart, etc.). service's options
// are all value-less, so unlike the wrapper/xargs/package classifiers there is no
// option-arity ambiguity to resolve and this stays a dedicated single-pass scan
// rather than going through skipLeadingOptions.
func serviceUnitName(args []string) (string, bool) {
	for i, a := range args {
		if a == "--" {
			// Option terminator: the next token is the unit name, even if it begins
			// with "-". Without this, a unit whose name starts with "-" would be
			// skipped and the following action mis-recorded as the init script.
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a, true
	}
	return "", false
}

// remoteShellValueOpts lists, per command, the value-taking options whose value
// must be skipped before testing for a helper option, so a value that happens to
// look like a helper flag (e.g. a directory literally named "--to-command" passed
// to "tar -C") is not misread as the helper. The lists cover the common
// separated-value options; an omission only risks a fail-closed over-block on a
// pathological value, never an under-block. The helper options themselves are
// deliberately absent so they are matched, not skipped.
var remoteShellValueOpts = map[string]map[string]struct{}{
	"tar": setOf(
		"-C", "--directory", "-f", "--file", "-b", "--blocking-factor",
		"-X", "--exclude-from", "-T", "--files-from", "-L", "--tape-length",
		"-V", "--label", "--transform", "--strip-components", "--owner",
		"--group", "--mode", "--record-size", "--newer", "--after-date",
	),
	"rsync": setOf(
		"--port", "--bwlimit", "--timeout", "--contimeout", "--rsync-path",
		"--password-file", "--exclude-from", "--include-from", "--files-from",
		"--log-file", "-T", "--temp-dir", "--compare-dest", "--copy-dest",
		"--link-dest", "--chmod", "--block-size", "-B", "--modify-window",
	),
}

// analyzeRemoteShellOption rejects rsync -e / tar --to-command style helpers,
// which the tool runs from its own child process. It skips the values of known
// value-taking options and stops at the "--" terminator so an option value or a
// post-terminator operand that happens to look like a helper flag is not
// misclassified as one (a false Blocking deny).
func analyzeRemoteShellOption(names map[string]struct{}, args []string) (IndirectExecutionResult, bool) {
	for cmd, prefixes := range remoteShellOptionPrefixes {
		if _, ok := names[cmd]; !ok {
			continue
		}
		valueOpts := remoteShellValueOpts[cmd]
		skipValue := false
		for _, a := range args {
			if a == "--" {
				// Option terminator: subsequent tokens are operands, so an operand that
				// happens to look like "-e"/"--rsh" is not a helper option.
				break
			}
			if skipValue {
				skipValue = false // this token is the previous option's value
				continue
			}
			for _, p := range prefixes {
				if matchesRemoteShellOption(a, p) {
					return reject(), true
				}
			}
			if _, ok := valueOpts[a]; ok {
				// A known value-taking option: skip its value so a value that looks
				// like a helper flag is not matched on the next iteration.
				skipValue = true
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
	// letter with other short flags (-avze).
	if len(p) == 2 && p[0] == '-' && p[1] != '-' {
		return shortFlagInBundle(arg, p[1])
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
// (npm run / npx / bunx / yarn run / pnpm run / bun run / dlx / exec, lifecycle
// aliases, and the yarn/pnpm/bun "<script>" shorthand).
func packageScriptRunnerRisk(names map[string]struct{}, args []string) (runnertypes.RiskLevel, bool) {
	// npx and bunx run arbitrary packages without an explicit install step.
	if _, ok := names["npx"]; ok {
		return runnertypes.RiskLevelHigh, true
	}
	if _, ok := names["bunx"]; ok {
		return runnertypes.RiskLevelHigh, true
	}
	// Lifecycle aliases (test/start/stop/restart) run package.json scripts without
	// the explicit run/run-script verb, so they are arbitrary-code runners too.
	scriptVerbs := setOf("run", "run-script", "exec", "dlx", "test", "start", "stop", "restart")
	for _, runner := range []string{"npm", "pnpm", "yarn", "bun"} {
		if _, ok := names[runner]; !ok {
			continue
		}
		verb, hasUnknown, ok := packageRunnerVerb(args)
		if hasUnknown {
			// An unknown separated option may consume the next token as its value,
			// shifting the parse position and hiding a script subcommand. Fail closed
			// (High) rather than miss a possible script invocation.
			return runnertypes.RiskLevelHigh, true
		}
		if !ok {
			continue
		}
		if _, isScript := scriptVerbs[verb]; isScript {
			return runnertypes.RiskLevelHigh, true
		}
		// yarn, pnpm, and bun run "<script>" as shorthand for "run <script>", so
		// any verb that is not a package-management builtin is a script invocation.
		if (runner == "yarn" || runner == "pnpm" || runner == "bun") && !isPackageManagerBuiltin(verb) {
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

// packageRunnerValueOpts are the package-runner options known to consume the
// following token as their value, so the value is not mistaken for the verb.
var packageRunnerValueOpts = setOf("--cwd", "-C", "--prefix", "--registry", "--cache")

// packageRunnerVerb returns the package-runner subcommand: the first non-option
// token, skipping recognized value-taking options and their values so an option
// value is not mistaken for the subcommand (e.g. "yarn --cwd /dir install" ->
// install). hasUnknown is true when an unrecognized option of unknown arity is
// seen before the verb (anyUnknownIsUnreliable): it may consume the next token,
// hiding a script subcommand, so the caller must fail closed.
func packageRunnerVerb(args []string) (verb string, hasUnknown, ok bool) {
	idx, reliable := skipLeadingOptions(args, optSpec{valueOpts: packageRunnerValueOpts, unknown: anyUnknownIsUnreliable})
	if !reliable {
		return "", true, false
	}
	if idx >= len(args) {
		return "", false, false
	}
	return args[idx], false, true
}

// hasFlag reports whether flag appears in args (stopping at the "--" option
// terminator). A two-character short flag (e.g. "-c") is also detected inside a
// combined short-flag bundle (e.g. "-xc" includes "-c").
func hasFlag(args []string, flag string) bool {
	isShort := len(flag) == 2 && flag[0] == '-' && flag[1] != '-'
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == flag {
			return true
		}
		if isShort && shortFlagInBundle(a, flag[1]) {
			return true
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

// isLoaderControlVar reports whether name is a dynamic-loader control variable:
// any LD_* (ELF) or DYLD_* (macOS) variable. Supplying one via a wrapper lets an
// attacker change which shared objects an otherwise-verified binary loads, so
// these are rejected. The match is a prefix match, not an allowlist of the
// well-known names (LD_PRELOAD, LD_LIBRARY_PATH, …), so loader variables beyond
// those — LD_DEBUG, LD_BIND_NOW, LD_AUDIT, … — are also rejected, keeping the
// fail-closed posture. The DYLD_ family is rejected on every OS since the deny
// list is platform independent. Names are upper-cased before matching so a
// lower-case spelling cannot slip past.
func isLoaderControlVar(name string) bool {
	upper := strings.ToUpper(name)
	return strings.HasPrefix(upper, "LD_") || strings.HasPrefix(upper, "DYLD_")
}

// isPrivilegeCommand reports whether the command escalates privilege
// (e.g. sudo, su), matched through its risk profile by the given pre-resolved name
// set.
func isPrivilegeCommand(names map[string]struct{}) bool {
	p, ok := ResolveProfile(names)
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
