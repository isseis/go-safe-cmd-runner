package security

import (
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
// verb, via the shared getopt operand scanner. It returns the verb and a forceHigh
// flag. An unknown separate option might consume a following verb, so the scan is
// unreliable there and forceHigh is set (fail-safe; a hidden change verb must not
// pass as a read-only command). systemctl's own boolean options are listed so they
// are skipped rather than treated as unknown.
func firstSystemctlSubcommand(args []string) (verb string, forceHigh bool) {
	verb, reliable := firstOperand(args, optSpec{
		valueOpts: systemctlValueOptions,
		boolOpts:  systemctlBoolOptions,
		unknown:   anyUnknownIsUnreliable,
	})
	return verb, !reliable
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
