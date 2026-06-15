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
var arbitraryCodeExecutionRunners = map[string]struct{}{
	// Shells
	"bash": {}, "sh": {}, "dash": {}, "zsh": {}, "ksh": {}, "csh": {}, "tcsh": {}, "fish": {},
	// Script interpreters and runtimes
	"python": {}, "python2": {}, "python3": {},
	"node": {}, "nodejs": {}, "deno": {}, "bun": {},
	"ruby": {}, "perl": {}, "php": {}, "lua": {}, "luajit": {},
	"java": {}, "dotnet": {}, "pwsh": {}, "powershell": {},
	// Build and task runners. They execute arbitrary commands written in an
	// unverified external definition (Makefile, build.gradle, ...).
	"make": {}, "cmake": {}, "ninja": {}, "gradle": {}, "mvn": {}, "bazel": {}, "rake": {}, "just": {}, "task": {},
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
