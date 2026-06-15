package security

import (
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// systemctl subcommand classification. Change verbs are High,
// read-only verbs are a Medium floor (they can expose unit configuration), and
// anything unknown or unidentifiable is High (fail-safe). The lists are
// maintained here; new verbs default to High because an unrecognized verb is
// treated as unidentifiable.
var (
	systemctlChangeVerbs = map[string]struct{}{
		"start": {}, "stop": {}, "restart": {}, "reload": {}, "reload-or-restart": {},
		"enable": {}, "disable": {}, "mask": {}, "unmask": {}, "isolate": {}, "kill": {},
		"set-property": {}, "set-default": {}, "daemon-reload": {}, "daemon-reexec": {},
		"edit": {}, "revert": {},
	}

	systemctlReadOnlyVerbs = map[string]struct{}{
		"status": {}, "show": {}, "cat": {}, "is-active": {}, "is-enabled": {}, "is-failed": {},
		"list-units": {}, "list-unit-files": {}, "list-timers": {}, "list-sockets": {},
		"list-dependencies": {}, "list-jobs": {}, "get-default": {}, "show-environment": {},
	}

	// systemctlValueOptions are options that consume the following token as their
	// value, so that token is not the subcommand.
	systemctlValueOptions = map[string]struct{}{
		"-H": {}, "--host": {}, "-M": {}, "--machine": {}, "-t": {}, "--type": {},
		"--state": {}, "-p": {}, "--property": {}, "-P": {}, "--what": {}, "--job-mode": {},
		"--root": {}, "--image": {}, "--drop-in": {}, "--when": {}, "--kill-whom": {},
		"-s": {}, "--signal": {}, "-o": {}, "--output": {}, "-n": {}, "--lines": {},
	}

	// systemctlBoolOptions are value-less options that are simply skipped.
	systemctlBoolOptions = map[string]struct{}{
		"--now": {}, "--no-pager": {}, "--quiet": {}, "-q": {}, "--user": {}, "--system": {},
		"--no-ask-password": {}, "--no-block": {}, "--no-reload": {}, "--no-legend": {},
		"--all": {}, "-a": {}, "-f": {}, "--force": {}, "-l": {}, "--full": {}, "-r": {},
		"--recursive": {}, "--global": {}, "--runtime": {}, "--show-types": {}, "--reverse": {},
		"--value": {}, "--plain": {},
	}
)

// firstSystemctlSubcommand parses systemctl argv to find the first subcommand
// verb, correctly skipping options and the option terminator. It returns the verb
// and a forceHigh flag.
//
// Rules:
//   - A value option (-t TYPE, --host HOST, ...) consumes the next token.
//   - A combined option (--type=foo) is self-contained.
//   - A boolean option (--now, --quiet, ...) is skipped.
//   - An unknown combined option (--unknown=x) cannot hide a verb, so it is
//     skipped safely.
//   - An unknown separate option (--unknown) might consume a following verb, so
//     forceHigh is returned (fail-safe; a hidden change verb must not pass as a
//     read-only command).
//   - "--" terminates options; the next token is unconditionally the verb.
func firstSystemctlSubcommand(args []string) (verb string, forceHigh bool) {
	skipNext := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if skipNext {
			skipNext = false
			continue
		}

		if arg == "--" {
			if i+1 < len(args) {
				return args[i+1], false
			}
			return "", false
		}

		if strings.HasPrefix(arg, "-") {
			if strings.Contains(arg, "=") {
				continue
			}
			if _, ok := systemctlValueOptions[arg]; ok {
				skipNext = true
				continue
			}
			if _, ok := systemctlBoolOptions[arg]; ok {
				continue
			}
			// Unknown separate option: it may take a value that is a verb name.
			return "", true
		}

		return arg, false
	}
	return "", false
}

// SystemctlSubcommandRisk derives the effective system-modification risk of a
// systemctl invocation from its subcommand. Change verbs and
// unknown/unidentifiable verbs are High; read-only verbs and an omitted
// subcommand are a Medium floor (never Low, since they can expose configuration).
func SystemctlSubcommandRisk(args []string) runnertypes.RiskLevel {
	verb, forceHigh := firstSystemctlSubcommand(args)
	if forceHigh {
		return runnertypes.RiskLevelHigh
	}
	if verb == "" {
		// No subcommand: informational use; keep a Medium floor.
		return runnertypes.RiskLevelMedium
	}
	if _, ok := systemctlChangeVerbs[verb]; ok {
		return runnertypes.RiskLevelHigh
	}
	if _, ok := systemctlReadOnlyVerbs[verb]; ok {
		return runnertypes.RiskLevelMedium
	}
	// Unknown verb: fail safe to High.
	return runnertypes.RiskLevelHigh
}
