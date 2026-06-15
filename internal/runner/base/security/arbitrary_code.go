package security

// arbitraryCodeExecutionRunners is the curated blocklist of commands that can
// execute arbitrary system calls or arbitrary code from inputs (scripts,
// recipes, build definitions) that are outside this tool's per-command risk
// evaluation and hash verification. They are classified High regardless of
// arguments.
//
// Package-script runners (npm run / npx / yarn <script> / pnpm run) are handled
// in the indirect-execution path because their High classification depends on
// the argument form; this set covers only the argument-independent High group.
// The interpreter and runtime entries are kept in sync with the AlwaysNetwork
// interpreter profiles in commandProfileDefinitions: any runtime that can execute
// arbitrary code or a script must be High here, not merely Medium via its network
// profile, so an alias like lua5.4 or Rscript is not under-classified relative to
// lua or python.
var arbitraryCodeExecutionRunners = map[string]struct{}{
	// Shells
	"bash": {}, "sh": {}, "dash": {}, "zsh": {}, "ksh": {}, "csh": {}, "tcsh": {}, "fish": {},
	// Script interpreters and runtimes (including profiled aliases)
	"python": {}, "python2": {}, "python3": {},
	"node": {}, "nodejs": {}, "deno": {}, "bun": {},
	"ruby": {}, "jruby": {}, "perl": {}, "php": {},
	"lua": {}, "lua5.1": {}, "lua5.2": {}, "lua5.3": {}, "lua5.4": {}, "luajit": {},
	"tclsh": {}, "tclsh8.5": {}, "tclsh8.6": {}, "wish": {}, "wish8.5": {}, "wish8.6": {},
	"R": {}, "Rscript": {}, "julia": {},
	"guile": {}, "guile2": {}, "guile3": {},
	"elixir": {}, "iex": {}, "erl": {}, "erlc": {}, "escript": {},
	"java": {}, "javaw": {}, "groovy": {}, "groovysh": {}, "groovyConsole": {},
	"kotlin": {}, "scala": {}, "scala3": {}, "clojure": {}, "clj": {}, "jython": {},
	"dotnet": {}, "mono": {}, "pwsh": {}, "powershell": {},
	// Build and task runners. They execute arbitrary commands written in an
	// unverified external definition (Makefile, build.gradle, build.rs, ...).
	"make": {}, "cmake": {}, "ninja": {}, "gradle": {}, "mvn": {}, "bazel": {},
	"rake": {}, "just": {}, "task": {}, "go": {}, "cargo": {},
}

// IsArbitraryCodeExecutionRunner reports whether the command is a shell,
// interpreter, or build/task runner that can execute arbitrary code.
// Matching is by basename and considers symbolic links, mirroring the other
// name-based classifiers. The match is argument-independent: even --version or
// --help invocations are treated as runners, because exhaustively distinguishing
// harmless invocations is unsafe.
func IsArbitraryCodeExecutionRunner(cmd string) bool {
	names, _ := extractAllCommandNames(cmd)
	return anyNameInSet(names, arbitraryCodeExecutionRunners)
}
