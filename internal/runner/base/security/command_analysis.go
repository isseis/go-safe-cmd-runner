package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
)

// NetworkOperationType indicates the type of network operation a command performs
type NetworkOperationType int

// Network operation type constants
const (
	NetworkTypeNone        NetworkOperationType = iota // Not a network command
	NetworkTypeAlways                                  // Always performs network operations
	NetworkTypeConditional                             // Conditional based on arguments
)

// commandProfileDefinitions defines command risk profiles using the new builder pattern
// This uses CommandRiskProfile with explicit risk factor separation
var commandProfileDefinitions = []CommandProfileDef{
	// Privilege escalation commands
	NewProfile("sudo", "su", "doas").
		PrivilegeRisk(runnertypes.RiskLevelCritical, "Allows execution with elevated privileges, can compromise entire system").
		Build(),

	// System modification commands
	NewProfile("systemctl", "service").
		SystemModRisk(runnertypes.RiskLevelHigh, "Can modify system services and configuration").
		Build(),

	// Destructive operations - separate definitions for different risk levels
	NewProfile("rm").
		DestructionRisk(runnertypes.RiskLevelHigh, "Can delete files and directories").
		Build(),
	NewProfile("dd").
		DestructionRisk(runnertypes.RiskLevelHigh, "Can overwrite entire disks, potential data loss").
		Build(),

	// AI service commands with multiple risk factors
	NewProfile("claude", "gemini", "chatgpt", "gpt", "openai", "anthropic").
		NetworkRisk(runnertypes.RiskLevelHigh, "Always communicates with external AI API").
		DataExfilRisk(runnertypes.RiskLevelHigh, "May send sensitive data to external service").
		AlwaysNetwork().
		Build(),

	// Network commands (always)
	NewProfile("curl", "wget").
		NetworkRisk(runnertypes.RiskLevelMedium, "Always performs network operations").
		AlwaysNetwork().
		Build(),
	NewProfile("nc", "netcat", "telnet").
		NetworkRisk(runnertypes.RiskLevelMedium, "Establishes network connections").
		AlwaysNetwork().
		Build(),
	NewProfile("ssh", "scp").
		NetworkRisk(runnertypes.RiskLevelMedium, "Remote operations via network").
		AlwaysNetwork().
		Build(),
	NewProfile("aws").
		NetworkRisk(runnertypes.RiskLevelMedium, "Cloud service operations via network").
		AlwaysNetwork().
		Build(),

	// Network commands (conditional)
	NewProfile("git").
		NetworkRisk(runnertypes.RiskLevelMedium, "Network operations for clone/fetch/pull/push/remote").
		ConditionalNetwork("clone", "fetch", "pull", "push", "remote").
		Build(),
	NewProfile("rsync").
		NetworkRisk(runnertypes.RiskLevelMedium, "Network operations when using remote sources/destinations").
		ConditionalNetwork().
		Build(),

	// Script interpreters and shells - can execute arbitrary network commands internally
	// These may not have network symbols in their main binary but can invoke network tools
	NewProfile("bash", "sh", "dash", "zsh", "ksh", "csh", "tcsh", "fish").
		NetworkRisk(runnertypes.RiskLevelMedium, "Shell can execute arbitrary commands including network tools").
		AlwaysNetwork().
		Build(),
	NewProfile("node", "nodejs", "deno", "bun").
		NetworkRisk(runnertypes.RiskLevelMedium, "JavaScript runtime with built-in network capabilities").
		AlwaysNetwork().
		Build(),
	NewProfile("python", "python2", "python3").
		NetworkRisk(runnertypes.RiskLevelMedium, "Python interpreter with built-in network libraries").
		AlwaysNetwork().
		Build(),
	NewProfile("perl").
		NetworkRisk(runnertypes.RiskLevelMedium, "Perl interpreter with built-in network capabilities").
		AlwaysNetwork().
		Build(),
	NewProfile("ruby").
		NetworkRisk(runnertypes.RiskLevelMedium, "Ruby interpreter with built-in network libraries").
		AlwaysNetwork().
		Build(),
	NewProfile("php").
		NetworkRisk(runnertypes.RiskLevelMedium, "PHP interpreter can perform network operations").
		AlwaysNetwork().
		Build(),

	// Lua interpreter
	NewProfile("lua", "lua5.1", "lua5.2", "lua5.3", "lua5.4", "luajit").
		NetworkRisk(runnertypes.RiskLevelMedium, "Lua interpreter can load network extensions (e.g. LuaSocket)").
		AlwaysNetwork().
		Build(),

	// Tcl/Tk interpreter
	NewProfile("tclsh", "tclsh8.5", "tclsh8.6", "wish", "wish8.5", "wish8.6").
		NetworkRisk(runnertypes.RiskLevelMedium, "Tcl interpreter with built-in socket command").
		AlwaysNetwork().
		Build(),

	// R language
	NewProfile("R", "Rscript").
		NetworkRisk(runnertypes.RiskLevelMedium, "R interpreter with network-capable packages").
		AlwaysNetwork().
		Build(),

	// Julia
	NewProfile("julia").
		NetworkRisk(runnertypes.RiskLevelMedium, "Julia interpreter with built-in network capabilities").
		AlwaysNetwork().
		Build(),

	// GNU Guile (Scheme)
	NewProfile("guile", "guile2", "guile3").
		NetworkRisk(runnertypes.RiskLevelMedium, "Guile Scheme interpreter with network module").
		AlwaysNetwork().
		Build(),

	// Erlang/Elixir
	NewProfile("elixir", "iex").
		NetworkRisk(runnertypes.RiskLevelMedium, "Elixir runtime with built-in network capabilities").
		AlwaysNetwork().
		Build(),
	NewProfile("erl", "erlc", "escript").
		NetworkRisk(runnertypes.RiskLevelMedium, "Erlang runtime, network-oriented language").
		AlwaysNetwork().
		Build(),

	// JVM-based runtimes
	NewProfile("java", "javaw").
		NetworkRisk(runnertypes.RiskLevelMedium, "JVM with built-in java.net network libraries").
		AlwaysNetwork().
		Build(),
	NewProfile("groovy", "groovysh", "groovyConsole").
		NetworkRisk(runnertypes.RiskLevelMedium, "Groovy runtime on JVM with network capabilities").
		AlwaysNetwork().
		Build(),
	NewProfile("kotlin").
		NetworkRisk(runnertypes.RiskLevelMedium, "Kotlin runtime on JVM with network capabilities").
		AlwaysNetwork().
		Build(),
	NewProfile("scala", "scala3").
		NetworkRisk(runnertypes.RiskLevelMedium, "Scala runtime on JVM with network capabilities").
		AlwaysNetwork().
		Build(),
	NewProfile("clojure", "clj").
		NetworkRisk(runnertypes.RiskLevelMedium, "Clojure runtime on JVM with network capabilities").
		AlwaysNetwork().
		Build(),
	NewProfile("jruby").
		NetworkRisk(runnertypes.RiskLevelMedium, "JRuby runtime with Ruby network libraries on JVM").
		AlwaysNetwork().
		Build(),
	NewProfile("jython").
		NetworkRisk(runnertypes.RiskLevelMedium, "Jython runtime with Python network libraries on JVM").
		AlwaysNetwork().
		Build(),

	// .NET runtimes
	NewProfile("dotnet").
		NetworkRisk(runnertypes.RiskLevelMedium, ".NET runtime with System.Net network libraries").
		AlwaysNetwork().
		Build(),
	NewProfile("mono").
		NetworkRisk(runnertypes.RiskLevelMedium, "Mono .NET runtime with network capabilities").
		AlwaysNetwork().
		Build(),
	NewProfile("pwsh", "powershell").
		NetworkRisk(runnertypes.RiskLevelMedium, "PowerShell with built-in network cmdlets").
		AlwaysNetwork().
		Build(),
}

// commandRiskProfiles is built from commandProfileDefinitions (new structure)
var commandRiskProfiles = buildCommandRiskProfiles()

func buildCommandRiskProfiles() map[string]CommandRiskProfile {
	profiles := make(map[string]CommandRiskProfile)
	for _, def := range commandProfileDefinitions {
		// Profile is already validated in Build()
		for _, cmd := range def.Commands() {
			profiles[cmd] = def.Profile()
		}
	}
	return profiles
}

// Pre-sorted patterns by risk level for efficient lookup
var (
	highRiskPatterns   []DangerousCommandPattern
	mediumRiskPatterns []DangerousCommandPattern
)

// dangerousCommandPatterns contains the static list of dangerous command patterns
var dangerousCommandPatterns = []DangerousCommandPattern{
	// File system destruction
	{[]string{"rm", "-rf"}, runnertypes.RiskLevelHigh, "Recursive file removal"},
	{[]string{"sudo", "rm"}, runnertypes.RiskLevelHigh, "Privileged file removal"},
	{[]string{"format"}, runnertypes.RiskLevelHigh, "Disk formatting"},
	{[]string{"mkfs"}, runnertypes.RiskLevelHigh, "File system creation"},
	{[]string{"fdisk"}, runnertypes.RiskLevelHigh, "Disk partitioning"},

	// Data manipulation
	{[]string{"dd", "if="}, runnertypes.RiskLevelHigh, "Low-level disk operations"},
	{[]string{"chmod", "777"}, runnertypes.RiskLevelHigh, "Overly permissive file permissions"},
	{[]string{"chown", "root"}, runnertypes.RiskLevelMedium, "Ownership change to root"},

	// Network operations
	{[]string{"wget"}, runnertypes.RiskLevelMedium, "File download"},
	{[]string{"curl"}, runnertypes.RiskLevelMedium, "Network request"},
	{[]string{"nc", "-"}, runnertypes.RiskLevelMedium, "Network connection"},
	{[]string{"netcat"}, runnertypes.RiskLevelMedium, "Network connection"},
}

// init initializes the pre-sorted pattern lists for efficient lookup
func init() {
	for _, p := range dangerousCommandPatterns {
		switch p.RiskLevel {
		case runnertypes.RiskLevelHigh:
			highRiskPatterns = append(highRiskPatterns, p)
		case runnertypes.RiskLevelMedium:
			mediumRiskPatterns = append(mediumRiskPatterns, p)
		case runnertypes.RiskLevelLow, runnertypes.RiskLevelUnknown:
			// Skip low and none risk patterns as they don't need checking
			continue
		default:
			// Skip invalid risk levels
			continue
		}
	}
}

// IsDangerousPrivilegedCommand checks if a command path is potentially dangerous when run with privileges
func (v *Validator) IsDangerousPrivilegedCommand(cmdPath string) bool {
	_, exists := v.dangerousPrivilegedCommands[cmdPath]
	return exists
}

// IsShellCommand checks if a command is a shell command
func (v *Validator) IsShellCommand(cmdPath string) bool {
	_, exists := v.shellCommands[cmdPath]
	return exists
}

// IsDangerousRootCommand checks if a command matches dangerous patterns when running as root.
//
// This function uses exact command name matching (after extracting the basename and converting
// to lowercase) to determine if a command is in the dangerous patterns list. This ensures that
// commands like "lsrm" are not incorrectly flagged just because they contain "rm" as a substring.
//
// Matching behavior:
//   - Extracts the basename from the command path (e.g., "/bin/rm" -> "rm")
//   - Converts to lowercase for case-insensitive comparison
//   - Performs exact match against DangerousRootPatterns list
//
// Examples:
//   - "/bin/rm" matches if DangerousRootPatterns contains "rm"
//   - "/usr/bin/lsrm" does NOT match even if DangerousRootPatterns contains "rm"
//   - "RM" matches if DangerousRootPatterns contains "rm" (case-insensitive)
//
// The DangerousRootPatterns list is validated at validator creation time to ensure
// all entries are suitable for exact matching (no paths, wildcards, or regex patterns).
func (v *Validator) IsDangerousRootCommand(cmdPath string) bool {
	cmdBase := filepath.Base(cmdPath)
	cmdLower := strings.ToLower(cmdBase)

	return slices.Contains(v.config.DangerousRootPatterns, cmdLower)
}

// HasDangerousRootArgs checks if any argument contains dangerous patterns for root commands
func (v *Validator) HasDangerousRootArgs(args []string) []int {
	var dangerousIndices []int

	for i, arg := range args {
		argLower := strings.ToLower(arg)
		for _, dangerousPattern := range v.config.DangerousRootArgPatterns {
			if strings.Contains(argLower, dangerousPattern) {
				dangerousIndices = append(dangerousIndices, i)
				break
			}
		}
	}
	return dangerousIndices
}

// HasWildcards checks if any argument contains wildcards
func (v *Validator) HasWildcards(args []string) []int {
	var wildcardIndices []int

	for i, arg := range args {
		if strings.Contains(arg, "*") || strings.Contains(arg, "?") {
			wildcardIndices = append(wildcardIndices, i)
		}
	}
	return wildcardIndices
}

// HasSystemCriticalPaths checks if any argument targets system-critical paths
func (v *Validator) HasSystemCriticalPaths(args []string) []int {
	var criticalIndices []int

	for i, arg := range args {
		for _, criticalPath := range v.config.GetSystemCriticalPaths() {
			if strings.HasPrefix(arg, criticalPath) &&
				(len(arg) == len(criticalPath) || arg[len(criticalPath)] == '/') {
				criticalIndices = append(criticalIndices, i)
				break
			}
		}
	}
	return criticalIndices
}

// checkCommandPatterns checks if a command (given by its pre-resolved name set)
// matches any patterns in the given list, returning the matched pattern's risk
// level and human-readable reason.
func checkCommandPatterns(names map[string]struct{}, cmdArgs []string, patterns []DangerousCommandPattern) (runnertypes.RiskLevel, string) {
	for _, pattern := range patterns {
		if matchesPattern(names, cmdArgs, pattern.Pattern) {
			return pattern.RiskLevel, pattern.Reason
		}
	}
	return runnertypes.RiskLevelUnknown, ""
}

// formatDetectedSymbols formats detected symbols for logging.
func formatDetectedSymbols(symbols []binaryanalyzer.DetectedSymbol) string {
	if len(symbols) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, s := range symbols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(s.Name)
		b.WriteByte('(')
		b.WriteString(s.Category)
		b.WriteByte(')')
	}
	b.WriteByte(']')
	return b.String()
}

// Regex patterns for SSH-style address detection.
//
// Pattern 1: user@host:path (e.g., "user@example.com:/path/to/file", "user@host:file.txt")
//   - .+@      : one or more chars before @  (user part)
//   - [^:@]+   : one or more chars that are not : or @  (host part)
//   - :        : literal colon separator
//   - [^ \t]   : first char after : must not be space/tab (excludes "user@host: /path")
//   - \S*      : remaining non-whitespace chars  (rest of path)
//   - The presence of user@host: is sufficient to identify SSH-style addresses;
//     the path can be absolute, relative, or a bare filename.
//
// Pattern 2: host:path without @ (e.g., "server:/path", "host:~/documents")
//   - ^[^@:]+  : one or more chars that are not @ or :  (host part, no @ allowed)
//   - :        : literal colon separator
//   - [^ \t]   : first char after : must not be space/tab
//   - \S*      : remaining non-whitespace chars
//   - The path portion (after :) must contain / or ~ to distinguish from
//     non-SSH uses of colons (e.g., "localhost:8080", "12:30:45").
var (
	sshUserHostPathRe = regexp.MustCompile(`.+@[^:@]+:[^ \t]\S*`)
	sshHostPathRe     = regexp.MustCompile(`^[^@:]+:[^ \t]\S*`)
)

// containsSSHStyleAddress checks if any argument contains SSH-style addresses ([user@]host:path).
// This is more specific than just checking for "@" to avoid false positives with email addresses,
// port numbers, time formats, and natural-language text containing colons.
func containsSSHStyleAddress(args []string) bool {
	for _, arg := range args {
		switch {
		case sshUserHostPathRe.MatchString(arg):
			// user@host:path — the user@host: pattern is sufficient to identify SSH.
			// Relative paths (e.g., "user@host:file.txt") are valid SSH addresses.
			return true
		case sshHostPathRe.MatchString(arg):
			// host:path (no @) — require / or ~ in the path to avoid false positives
			// with port numbers (e.g., "localhost:8080") and time formats.
			colonIndex := strings.Index(arg, ":")
			pathPart := arg[colonIndex+1:]
			if strings.Contains(pathPart, "/") || strings.HasPrefix(pathPart, "~") {
				return true
			}
		}
	}
	return false
}

// destructiveCommandNames is the set of base command names that perform
// destructive file operations regardless of arguments.
var destructiveCommandNames = map[string]struct{}{
	"rm":     {},
	"rmdir":  {},
	"unlink": {},
	"shred":  {},
	"dd":     {}, // Can be dangerous when used incorrectly
}

// findExecActions are the find primaries that run an external command on matched
// files. Their target command is gated the same way as a top-level command.
var findExecActions = map[string]struct{}{
	"-exec":    {},
	"-execdir": {},
	"-ok":      {},
	"-okdir":   {},
}

// systemModificationCommandNames is the set of base command names that modify
// system settings regardless of arguments.
var systemModificationCommandNames = map[string]struct{}{
	"systemctl":   {},
	"service":     {},
	"chkconfig":   {},
	"update-rc.d": {},
	"mount":       {},
	"umount":      {},
	"fdisk":       {},
	"parted":      {},
	"mkfs":        {},
	"fsck":        {},
	"crontab":     {},
	"at":          {},
	"batch":       {},
}

// packageManagerNames is the set of package managers whose install/remove style
// operations count as system modification.
var packageManagerNames = map[string]struct{}{
	"apt":     {},
	"apt-get": {},
	"yum":     {},
	"dnf":     {},
	"zypper":  {},
	"pacman":  {},
	"brew":    {},
	"pip":     {},
	"npm":     {},
	"yarn":    {},
}

// packageModifyingVerbs are package-manager subcommands that install, remove, or
// otherwise modify installed software. The list is non-exhaustive (the threat
// model backstops with allowlist + hash pinning); an unmatched verb falls through
// to a non-modifying classification.
var packageModifyingVerbs = map[string]struct{}{
	"install": {}, "remove": {}, "uninstall": {}, "upgrade": {}, "update": {},
	"purge": {}, "autoremove": {}, "dist-upgrade": {}, "full-upgrade": {},
	"dselect-upgrade": {}, "clean": {}, "autoclean": {},
	"groupinstall": {}, "groupremove": {}, "localinstall": {}, "localupdate": {},
	"reinstall": {}, "add": {}, "tap": {}, "untap": {},
	"i": {}, "un": {}, "up": {}, // common npm shorthands
}

// anyNameInSet reports whether any of the resolved command names is in set.
func anyNameInSet(names, set map[string]struct{}) bool {
	for n := range names {
		if _, ok := set[n]; ok {
			return true
		}
	}
	return false
}

// isDestructiveBaseCommand reports whether the command (matched by basename and
// resolved symlinks) is a destructive file operation. It is used for find's
// exec-action target — a nested command, typically a bare name, whose name set is
// resolved here on demand. The top-level/inner command is checked directly
// against its pre-resolved name set in IsDestructiveFileOperation.
func isDestructiveBaseCommand(cmd string) bool {
	names, _ := extractAllCommandNames(cmd)
	return anyNameInSet(names, destructiveCommandNames)
}

// IsDestructiveFileOperation checks if the command performs destructive file
// operations. The command is matched by basename and considers symbolic links,
// so an absolute path such as /usr/bin/rm or a coreutils-directory rm is detected
// (a substring like lsrm is not). find's exec actions (-exec/-execdir/-ok/-okdir)
// and -delete are covered, with the exec target itself basename/symlink matched.
func IsDestructiveFileOperation(names map[string]struct{}, args []string) bool {
	if anyNameInSet(names, destructiveCommandNames) {
		return true
	}

	if _, ok := names["find"]; ok {
		// This scans every -exec/-delete primary (a find may have several), so it
		// intentionally does not reuse findExecTarget, which returns only the first
		// exec target for the rank-2 indirect-execution gate.
		for i, arg := range args {
			if arg == "-delete" {
				return true
			}
			if _, isAction := findExecActions[arg]; isAction && i+1 < len(args) {
				// The token after the action primary is the command to run.
				if isDestructiveBaseCommand(args[i+1]) {
					return true
				}
			}
		}
	}

	if _, ok := names["rsync"]; ok {
		for _, arg := range args {
			if arg == "--delete" || arg == "--delete-before" || arg == "--delete-after" {
				return true
			}
		}
	}

	return false
}

// isSystemModificationByNames checks whether the command modifies system settings,
// deciding from an already-resolved name set (matched by basename and resolved
// symbolic links, so an absolute path such as /usr/sbin/systemctl is detected) so
// callers that have already walked the symlink chain do not repeat the work.
func isSystemModificationByNames(names map[string]struct{}, args []string) bool {
	if anyNameInSet(names, systemModificationCommandNames) {
		return true
	}

	if anyNameInSet(names, packageManagerNames) {
		// Only consider install/remove/upgrade-style operations as system
		// modification (a bare query such as "apt list" is not).
		for _, arg := range args {
			if _, ok := packageModifyingVerbs[arg]; ok {
				return true
			}
		}
	}

	// Flag-style managers (pacman/dpkg/rpm) select their operation from option
	// flags rather than a verb. This gate is independent of packageManagerNames so
	// managers absent from the verb set still reach their flag rule.
	for n := range names {
		if rule, ok := flagStyleManagers[n]; ok && isFlagStyleModification(rule, args) {
			return true
		}
	}

	return false
}

// SystemModificationRisk derives the system-modification risk for a command given
// its resolved name set: systemctl is subcommand-conditional (read-only verbs stay
// at a Medium floor, change/unknown verbs are High), service is always High (it
// runs an unverified init script), and any other system-modification command
// (mount, crontab, mkfs, package install/remove, ...) is Medium. It returns
// RiskLevelUnknown when no system-modification dimension applies. The decision is
// made entirely from the supplied resolved-name set (no re-extraction). This is
// the single source for the dimension, used both by the top-level evaluator and
// by the wrapped-inner indirect-execution path.
func SystemModificationRisk(names map[string]struct{}, args []string) runnertypes.RiskLevel {
	if _, ok := names["systemctl"]; ok {
		return SystemctlSubcommandRisk(args)
	}
	if _, ok := names["service"]; ok {
		return runnertypes.RiskLevelHigh
	}
	if isSystemModificationByNames(names, args) {
		return runnertypes.RiskLevelMedium
	}
	return runnertypes.RiskLevelUnknown
}

// CheckDangerousArgPatterns reports the risk level implied by a command together
// with its arguments (rm -rf, dd if=, chmod -R 777, mkfs.* ...). It is the public
// entry point used by runtime risk evaluation so dangerous argument patterns
// contribute to the effective risk, not only to the dry-run display. High
// patterns take precedence over Medium. Returns RiskLevelUnknown with an empty
// reason when no pattern matches.
func CheckDangerousArgPatterns(names map[string]struct{}, args []string) (runnertypes.RiskLevel, string) {
	// mkfs.<fstype> variants (mkfs.ext4, mkfs.xfs, ...) create filesystems and
	// are high risk; the static pattern list only covers the bare "mkfs" name.
	for n := range names {
		if strings.HasPrefix(n, "mkfs.") {
			return runnertypes.RiskLevelHigh, "Filesystem creation (mkfs family)"
		}
	}

	if level, reason := checkCommandPatterns(names, args, highRiskPatterns); level != runnertypes.RiskLevelUnknown {
		return level, reason
	}
	if level, reason := checkCommandPatterns(names, args, mediumRiskPatterns); level != runnertypes.RiskLevelUnknown {
		return level, reason
	}
	return runnertypes.RiskLevelUnknown, ""
}

// ErrSymlinkResolutionFailed is returned by ResolveCommandNames when a symbolic
// link in the chain cannot be read (link-target fetch failure), distinct from
// exceeding the depth limit (ErrSymlinkDepthExceeded).
var ErrSymlinkResolutionFailed = errors.New("symbolic link resolution failed")

// walkSymlinkChain resolves cmdName's symbolic-link chain and returns the set of
// all names encountered (the original, its basename, and every link target with
// its basename), used for name-based matching.
//
// The strict flag selects the error policy:
//   - strict=false (lenient): a mid-chain stat/readlink failure stops resolution
//     and returns the names gathered so far; a depth-limit overflow sets
//     exceededDepth. err is always nil. Used where partial resolution is
//     acceptable (matching helpers, dry-run display).
//   - strict=true: a depth-limit overflow returns ErrSymlinkDepthExceeded and a
//     mid-chain link-target fetch failure returns ErrSymlinkResolutionFailed, so
//     the evaluator can block rather than evaluate a partially resolved chain. A
//     bare command name or a regular file (no symlink to follow) resolves without
//     error.
func walkSymlinkChain(cmdName string, strict bool) (names map[string]struct{}, exceededDepth bool, err error) {
	seen := make(map[string]struct{})
	if cmdName == "" {
		return seen, false, nil
	}

	seen[cmdName] = struct{}{}
	seen[filepath.Base(cmdName)] = struct{}{}

	// A bare command name (no path separator) is resolved via PATH at exec time,
	// not here. Walking the filesystem for it would Lstat/follow a same-named entry
	// in the current working directory, making name-based classification depend on
	// CWD contents (e.g. a ./rm symlink to sudo could turn a wrapped "rm" into a
	// Critical). Match on the name itself only; this needs no filesystem access and
	// resolves without error in both modes.
	if !strings.Contains(cmdName, "/") {
		return seen, false, nil
	}

	// visited detects a symlink cycle directly, rather than only via the depth
	// limit, so a cycle is reported immediately without walking the full depth.
	visited := make(map[string]struct{})
	current := cmdName
	for depth := range MaxSymlinkDepth {
		if _, ok := visited[current]; ok {
			// Cycle detected. Treat it like a depth overflow: strict fails closed,
			// lenient reports exceededDepth so callers flag it (e.g. dry-run High).
			if strict {
				return nil, false, fmt.Errorf("%w: cyclic symlink at %q", ErrSymlinkResolutionFailed, current)
			}
			return seen, true, nil
		}
		visited[current] = struct{}{}

		fileInfo, statErr := os.Lstat(current)
		if statErr != nil {
			// At depth 0 the path may be a bare name or a not-yet-present file;
			// that is not a resolution failure. After following at least one link,
			// a stat failure means the link target could not be fetched.
			if !strict || depth == 0 {
				break
			}
			return nil, false, fmt.Errorf("%w: %q: %w", ErrSymlinkResolutionFailed, current, statErr)
		}

		if fileInfo.Mode()&os.ModeSymlink == 0 {
			break
		}

		if depth == MaxSymlinkDepth-1 {
			if strict {
				return nil, false, fmt.Errorf("%w: %q", ErrSymlinkDepthExceeded, cmdName)
			}
			return seen, true, nil
		}

		target, linkErr := os.Readlink(current)
		if linkErr != nil {
			if !strict {
				break
			}
			return nil, false, fmt.Errorf("%w: %q: %w", ErrSymlinkResolutionFailed, current, linkErr)
		}

		if !filepath.IsAbs(target) {
			current = filepath.Join(filepath.Dir(current), target)
		} else {
			current = target
		}

		seen[current] = struct{}{}
		seen[filepath.Base(current)] = struct{}{}
	}

	return seen, false, nil
}

// extractAllCommandNames extracts all possible command names for matching:
// 1. The original command name (could be full path or just filename)
// 2. Just the base filename from the original command
// 3. All symbolic link names in the chain (if any)
// 4. The final target filename after resolving all symbolic links
// Returns a map for O(1) lookup performance and a boolean indicating if symlink depth was exceeded.
func extractAllCommandNames(cmdName string) (map[string]struct{}, bool) {
	names, exceededDepth, _ := walkSymlinkChain(cmdName, false)
	return names, exceededDepth
}

// ResolveCommandNames resolves the command's symlink chain and returns the set of
// all names (original, basename, and every link target). It fails closed: a
// depth-limit overflow returns ErrSymlinkDepthExceeded and a link-target fetch
// failure mid-chain returns ErrSymlinkResolutionFailed.
func ResolveCommandNames(cmdName string) (map[string]struct{}, error) {
	names, _, err := walkSymlinkChain(cmdName, true)
	return names, err
}

// matchesPattern checks if the command matches the dangerous pattern.
//
// Pattern matching rules:
//  1. Empty commands are invalid (programming error) and always return false.
//  2. Empty patterns match all valid commands.
//  3. Command names (index 0): Matches against filename only, supporting full paths and symbolic links.
//  4. Argument matching is order-independent.
//  5. Argument count matching: Subset matching (command can have more arguments than pattern).
//  6. Argument patterns ending with "=": Use prefix matching (e.g., "if="
//     matches "if=/dev/zero").
//  7. Other arguments: Require exact string match.
func matchesPattern(names map[string]struct{}, cmdArgs []string, pattern []string) bool {
	// Empty pattern never matches any command
	if len(pattern) == 0 {
		return false
	}

	// names is the command's pre-resolved name set (original, base filename,
	// symlink targets), resolved once by the caller with its own policy. Check if
	// any of those names match pattern[0].
	if _, exists := names[pattern[0]]; !exists {
		return false
	}

	patternArgs := pattern[1:]

	// Default: subset match, require command to have at least as many args as pattern
	if len(cmdArgs) < len(patternArgs) {
		return false
	}

	// Order-independent matching with one-time use of command args
	matchedCommandArgs := make([]bool, len(cmdArgs))
	for _, patternArg := range patternArgs {
		foundMatch := false

		// Prefix pattern when ending with '=' (e.g., "if=")
		if strings.HasSuffix(patternArg, "=") {
			for i, commandArg := range cmdArgs {
				if matchedCommandArgs[i] {
					continue
				}
				if strings.HasPrefix(commandArg, patternArg) {
					matchedCommandArgs[i] = true
					foundMatch = true
					break
				}
			}
		} else {
			// Exact match
			for i, commandArg := range cmdArgs {
				if matchedCommandArgs[i] {
					continue
				}
				if commandArg == patternArg {
					matchedCommandArgs[i] = true
					foundMatch = true
					break
				}
			}
		}

		if !foundMatch {
			return false
		}
	}

	return true
}

// hasSetuidOrSetgidBit checks if the given command path has setuid or setgid bit set
// This function expects a resolved absolute path. Path resolution should be done
// by the caller using the unified path resolution system.
// Returns (hasSetuidOrSetgid, error)
func hasSetuidOrSetgidBit(cmdPath string) (bool, error) {
	if !filepath.IsAbs(cmdPath) {
		return false, fmt.Errorf("%w: path must be absolute, got relative path: %s", isec.ErrInvalidPath, cmdPath)
	}
	// Get file information
	fileInfo, err := os.Stat(cmdPath)
	if err != nil {
		// If we can't stat the file, assume it's not setuid/setgid
		return false, err
	}

	// Check if it's a regular file
	if !fileInfo.Mode().IsRegular() {
		return false, nil
	}

	// Check for setuid or setgid bits
	mode := fileInfo.Mode()
	hasSetuidBit := mode&os.ModeSetuid != 0
	hasSetgidBit := mode&os.ModeSetgid != 0

	return hasSetuidBit || hasSetgidBit, nil
}
