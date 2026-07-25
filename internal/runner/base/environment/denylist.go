package environment

import "strings"

// forbiddenEnvVarPrefixes lists name prefixes whose matching environment variables must
// never reach a child process, regardless of source (system environment, TOML env_vars,
// or an `env NAME=VALUE` assignment in an indirect execution command line).
//
// LD_* and DYLD_* control dynamic linker behavior on Linux (ld.so) and macOS (dyld)
// respectively, letting an attacker redirect which libraries a verified binary loads.
// BASH_FUNC_* carries an exported shell function definition (the Shellshock /
// CVE-2014-6271 vector): a bash process that imports such a variable executes the
// embedded function body at startup, the same class of risk as BASH_ENV below.
var forbiddenEnvVarPrefixes = []string{
	"LD_",        // dynamic linker control (glibc ld.so)
	"DYLD_",      // dynamic linker control (macOS dyld)
	"BASH_FUNC_", // Shellshock-style exported shell function injection
}

// forbiddenEnvVarExact lists exact environment variable names that must never reach a
// child process, in addition to the prefix matches above. Each entry is grouped by the
// mechanism it abuses; see the requirements document's "target variable list" section
// for the canonical list this implementation follows.
var forbiddenEnvVarExact = map[string]struct{}{
	// Dynamic loader / locale / resolver control (exact match).
	"GCONV_PATH":     {}, // redirects iconv character-set conversion modules
	"LOCPATH":        {}, // overrides locale data directory
	"HOSTALIASES":    {}, // overrides hostname aliases for resolver
	"NLSPATH":        {}, // redirects NLS message catalogue search path
	"RES_OPTIONS":    {}, // overrides DNS resolver options
	"GLIBC_TUNABLES": {}, // glibc tunable overrides (CVE-2023-4911, "Looney Tunables")

	// Shell interpreter startup code injection.
	"BASH_ENV":  {}, // sourced by non-interactive bash before running a script
	"ENV":       {}, // sourced by POSIX sh-compatible shells at startup
	"SHELLOPTS": {}, // pre-sets shell options, can enable dangerous behavior on startup
	"PS4":       {}, // expanded and can execute command substitution under `set -x`

	// Python interpreter startup code injection.
	"PYTHONPATH":    {}, // prepends attacker-controlled module search path
	"PYTHONSTARTUP": {}, // executed at interactive interpreter startup
	"PYTHONHOME":    {}, // redirects the standard library location

	// Perl interpreter startup code injection.
	"PERL5LIB": {}, // prepends attacker-controlled module search path
	"PERL5OPT": {}, // injects interpreter command-line options at startup
	"PERL5DB":  {}, // injects code executed by the perl debugger hook

	// Node.js interpreter startup code injection.
	"NODE_OPTIONS": {}, // injects interpreter command-line options at startup
	"NODE_PATH":    {}, // prepends attacker-controlled module search path

	// Ruby interpreter startup code injection.
	"RUBYOPT": {}, // injects interpreter command-line options at startup
	"RUBYLIB": {}, // prepends attacker-controlled module search path

	// Git remote-helper / diff code execution.
	"GIT_SSH":           {}, // replaces the ssh command git invokes
	"GIT_SSH_COMMAND":   {}, // replaces the ssh command git invokes (takes precedence over GIT_SSH)
	"GIT_EXTERNAL_DIFF": {}, // replaces the diff command git invokes

	// `less` pager preprocessor code execution.
	"LESSOPEN":  {}, // replaces the input preprocessor `less` invokes
	"LESSCLOSE": {}, // replaces the cleanup command `less` invokes
}

// IsForbiddenEnvVar reports whether name is on the process-environment denylist: a
// dynamic-loader control variable (LD_*, DYLD_*, and the exact-match locale/resolver
// names), a code-injection variable read by an interpreter or pager at startup
// (BASH_ENV, PYTHONPATH, BASH_FUNC_*, LESSOPEN, ...), or another entry in the lists
// below — that must never reach a child process.
//
// Matching is case-sensitive: environment variable names are case-sensitive on Unix,
// and every mechanism this list defends against (the dynamic loader, and each
// interpreter's startup-file lookup) only recognizes the exact spelling shown here, so
// a differently-cased name has no effect on the corresponding program. Normalizing case
// would therefore reject harmless variables without shrinking the real attack surface.
// See docs/tasks/0156_env_denylist_consolidation/02_architecture.md section 6.2 for the
// full rationale.
//
// The canonical list of names is docs/tasks/0156_env_denylist_consolidation/01_requirements.md.
func IsForbiddenEnvVar(name string) bool {
	for _, prefix := range forbiddenEnvVarPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	_, ok := forbiddenEnvVarExact[name]
	return ok
}
