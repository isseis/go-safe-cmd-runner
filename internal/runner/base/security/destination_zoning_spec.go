package security

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// rawOperand is one extracted acting operand before resolution. base overrides the
// resolution base directory (used for an `ln -s` relative target, which resolves
// against the link's parent rather than the working directory).
type rawOperand struct {
	raw  string
	role risktypes.OperandRole
	base string
}

// extraction is the result of parsing one command's argv into acting operands plus
// the argv-derived floor signals.
type extraction struct {
	applies          bool // false when this invocation is not a write (e.g. sed without -i, tar -t)
	recognized       bool // all argv parsed (no stray token, no unknown possibly-value flag)
	recursive        bool // a recursion flag (-r/-R/-a) is present
	grantsPermission bool // chmod setuid/world-writable, install -m setuid or -o/-g, chattr i
	preserveMeta     bool // cp -p / -a (privileged-metadata copy)
	umountAll        bool // umount -a (unconditional High)
	// remoteEgress marks a data-transfer command whose destination is remote
	// (e.g. rsync to host:path / host::module / rsync://...). There is no local
	// path to zone-classify; the command contributes a network-egress Medium floor.
	remoteEgress bool
	operands     []rawOperand
}

// minSpecAndTarget is the minimum positional count for a permission command: the
// mode/owner/attribute spec plus at least one target file.
const minSpecAndTarget = 2

// minACLFields is the minimum colon-separated field count of a parsable ACL entry
// (type:qualifier:perms).
const minACLFields = 3

// minChattrModeLen is the smallest chattr mode token: a +/-/= operator plus one
// attribute letter.
const minChattrModeLen = 2

// lnTargetDirThreshold: more positionals than this (i.e. 3+) means the final ln
// argument is a destination directory rather than a single link name.
const lnTargetDirThreshold = 2

func set(items ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, it := range items {
		m[it] = struct{}{}
	}
	return m
}

// lookupSpec finds the spec for a command by matching its resolved name set (and
// the basename of cmdPath) against the declarative registry.
func lookupSpec(names map[string]struct{}, cmdPath string) (CommandFlagSpec, bool) {
	if base := filepath.Base(cmdPath); base != "" && base != "." && base != string(filepath.Separator) {
		if s, ok := commandFlagSpecs[base]; ok {
			return s, true
		}
	}
	for n := range names {
		if s, ok := commandFlagSpecs[n]; ok {
			return s, true
		}
	}
	return CommandFlagSpec{}, false
}

// The functions below are the per-command ToExtraction semantics. A getopt-conformant
// command reads its flag values from the ParseResult alone (by canonical key = Names[0],
// never by ranging the Values map); the raw argv is used only for the cluster-blind
// hasAny floor/control checks (which legacy did on raw argv and must stay cluster-blind),
// and for the non-getopt grammars (dd's key=value, chattr's attribute tokens, tar's
// normalization, find's positional predicates).

func extractCopyMove(pr ParseResult, args []string, isMove bool) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized, recursive: pr.Recursive}
	// preserveMeta is a cluster-blind whole-token check on raw argv (legacy parity):
	// cp -ra does NOT see -a here. The -p/--preserve forms are not declared flags, so
	// this is the only place they are observed.
	ext.preserveMeta = hasAny(args, set("-a", "--archive", "-p", "--preserve"))

	srcRole := risktypes.OperandRoleRead
	if isMove {
		// mv removes the source, so a trust-critical move source is itself a write.
		srcRole = risktypes.OperandRoleWrite
	}

	if tdirs := pr.Values["-t"]; len(tdirs) > 0 {
		appendTargetDir(&ext, tdirs)
		for _, s := range pr.NonFlagArgs {
			ext.operands = append(ext.operands, rawOperand{raw: s, role: srcRole})
		}
		if len(pr.NonFlagArgs) == 0 {
			// -t with no source files is an incomplete copy/move: fail closed.
			ext.recognized = false
		}
		return ext
	}

	pos := pr.NonFlagArgs
	if len(pos) == 0 {
		ext.recognized = false
		return ext
	}
	dest := pos[len(pos)-1]
	srcs := pos[:len(pos)-1]
	ext.operands = append(ext.operands, rawOperand{raw: dest, role: risktypes.OperandRoleWrite})
	for _, s := range srcs {
		ext.operands = append(ext.operands, rawOperand{raw: s, role: srcRole})
	}
	return ext
}

func appendTargetDir(ext *extraction, dirs []string) {
	for _, d := range dirs {
		ext.operands = append(ext.operands, rawOperand{raw: d, role: risktypes.OperandRoleWrite})
	}
}

// extractAllWrite makes every non-flag argument a write operand and fails closed when
// there is none. Shared by rm/rmdir/unlink (KindRemove), shred, truncate, and mount.
func extractAllWrite(pr ParseResult, _ []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized, recursive: pr.Recursive}
	for _, p := range pr.NonFlagArgs {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	if len(pr.NonFlagArgs) == 0 {
		ext.recognized = false
	}
	return ext
}

func extractRemove(pr ParseResult, args []string) extraction { return extractAllWrite(pr, args) }

func extractLink(pr ParseResult, args []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized}
	pos := pr.NonFlagArgs

	if tdirs := pr.Values["-t"]; len(tdirs) > 0 {
		appendTargetDir(&ext, tdirs)
		for _, t := range pos {
			ext.operands = append(ext.operands, rawOperand{raw: t, role: risktypes.OperandRoleRead})
		}
		if len(pos) == 0 {
			ext.recognized = false
		}
		return ext
	}

	// A relative target resolves against the link's parent only for a symbolic
	// link; a hard link's target resolves against the working directory. This is a
	// cluster-blind whole-token check on raw argv (legacy parity).
	isSymlink := hasAny(args, set("-s", "--symbolic"))
	switch len(pos) {
	case 0:
		ext.recognized = false
	case 1:
		// ln TARGET -> a link named after TARGET's basename in the working
		// directory: record both the target (read) and the implicit link (write).
		ext.operands = append(ext.operands, rawOperand{raw: pos[0], role: risktypes.OperandRoleRead})
		ext.operands = append(ext.operands, rawOperand{raw: filepath.Base(pos[0]), role: risktypes.OperandRoleWrite})
	default:
		linkName := pos[len(pos)-1]
		var targetBase string
		if isSymlink {
			// More than one target plus a final argument means the final argument is
			// the directory the links are created in, so a relative target resolves
			// against it; the two-argument form resolves against the link's parent.
			if len(pos) > lnTargetDirThreshold {
				targetBase = linkName
			} else {
				targetBase = filepath.Dir(linkName)
			}
		}
		for _, t := range pos[:len(pos)-1] {
			ext.operands = append(ext.operands, rawOperand{raw: t, role: risktypes.OperandRoleRead, base: targetBase})
		}
		ext.operands = append(ext.operands, rawOperand{raw: linkName, role: risktypes.OperandRoleWrite})
	}
	return ext
}

func extractSed(_ ParseResult, args []string) extraction {
	// sed edits in place only with -i / --in-place (which may carry an attached
	// backup suffix, e.g. -i.bak). Without it, sed writes to stdout, so axis 2 does
	// not apply. The in-place detection and the inline-script positional rule do not
	// fit the generic getopt shape, so -i tokens are split out and the rest re-parsed.
	inPlace := false
	var rest []string
	for _, a := range args {
		switch {
		case a == "-i" || a == "--in-place" || strings.HasPrefix(a, "-i") || strings.HasPrefix(a, "--in-place="):
			inPlace = true
		default:
			rest = append(rest, a)
		}
	}
	if !inPlace {
		return extraction{applies: false, recognized: true}
	}
	pr := parseArgs(sedRestFlags(), rest)
	ext := extraction{applies: true, recognized: pr.Recognized}
	// When the script is supplied via -e/-f, every positional is an edited file;
	// otherwise the first positional is the inline script and the rest are files.
	hasScriptFlag := pr.HasFlag("-e") || pr.HasFlag("-f")
	pos := pr.NonFlagArgs
	files := pos
	if !hasScriptFlag {
		if len(pos) <= 1 {
			ext.recognized = false
			return ext
		}
		files = pos[1:]
	} else if len(pos) == 0 {
		ext.recognized = false
		return ext
	}
	for _, f := range files {
		ext.operands = append(ext.operands, rawOperand{raw: f, role: risktypes.OperandRoleWrite})
	}
	return ext
}

// sedRestFlags is the flag set used to re-parse sed's argv after the -i / --in-place
// tokens are stripped. It mirrors the sed flags declared in commandFlagSpecs minus -i.
func sedRestFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueNonPath, "-e", "--expression"),
		valueFlag(ValueNonPath, "-f", "--file"),
		valueFlag(ValueNonPath, "-l", "--line-length"),
		boolFlag("-n", "--quiet", "--silent"), boolFlag("-r", "-E", "--regexp-extended"),
		boolFlag("-s", "--separate"), boolFlag("-z", "--null-data"), boolFlag("-u", "--unbuffered"),
		boolFlag("--posix"), boolFlag("--debug"), boolFlag("--sandbox"), boolFlag("--follow-symlinks"),
	}
}

func extractSimpleWrite(pr ParseResult, _ []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized}
	// A mode-granting flag (e.g. mkdir -m 0777 / -m u+s) is a permission grant even
	// in a safe-zone. -m is declared only for the commands where it is value-taking
	// (mkdir); for sponge/touch the key is simply absent.
	for _, m := range pr.Values["-m"] {
		if chmodGrantsHigh(m) {
			ext.grantsPermission = true
		}
	}
	for _, p := range pr.NonFlagArgs {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	if len(pr.NonFlagArgs) == 0 {
		ext.recognized = false
	}
	return ext
}

func extractTee(pr ParseResult, _ []string) extraction {
	if len(pr.NonFlagArgs) == 0 {
		// tee with no FILE writes only to stdout: axis 2 does not apply.
		return extraction{applies: false, recognized: true}
	}
	ext := extraction{applies: true, recognized: pr.Recognized}
	for _, p := range pr.NonFlagArgs {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	return ext
}

func extractInstall(pr ParseResult, args []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized}

	// Permission/ownership grant: -m with a setuid/setgid mode, or any -o/-g.
	for _, m := range pr.Values["-m"] {
		if chmodGrantsHigh(m) {
			ext.grantsPermission = true
		}
	}
	if pr.HasFlag("-o") || pr.HasFlag("-g") {
		ext.grantsPermission = true
	}

	pos := pr.NonFlagArgs

	// Directory-creation mode: every positional is a directory to create (write).
	// Cluster-blind whole-token check on raw argv (legacy parity): install -dv does
	// NOT see -d here.
	if hasAny(args, set("-d", "--directory")) {
		for _, p := range pos {
			ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
		}
		if len(pos) == 0 {
			ext.recognized = false
		}
		return ext
	}

	if tdirs := pr.Values["-t"]; len(tdirs) > 0 {
		appendTargetDir(&ext, tdirs)
		for _, s := range pos {
			ext.operands = append(ext.operands, rawOperand{raw: s, role: risktypes.OperandRoleRead})
		}
		if len(pos) == 0 {
			// -t with no source files is an incomplete install: fail closed.
			ext.recognized = false
		}
		return ext
	}

	switch len(pos) {
	case 0:
		ext.recognized = false
	case 1:
		// A single FILE: treat as a write destination.
		ext.operands = append(ext.operands, rawOperand{raw: pos[0], role: risktypes.OperandRoleWrite})
	default:
		dest := pos[len(pos)-1]
		ext.operands = append(ext.operands, rawOperand{raw: dest, role: risktypes.OperandRoleWrite})
		for _, s := range pos[:len(pos)-1] {
			ext.operands = append(ext.operands, rawOperand{raw: s, role: risktypes.OperandRoleRead})
		}
	}
	return ext
}

func extractTar(_ ParseResult, args []string) extraction {
	// tar accepts a leading bundled mode token without a dash (e.g. "xzf"); mode is
	// read from the raw argv and the rest re-parsed after normalization. The dispatcher's
	// ParseResult is ignored because it was parsed without normalization.
	mode := tarMode(args)
	pr := parseArgs(tarFlags(), normalizeTarArgs(args))
	ext := extraction{applies: true, recognized: pr.Recognized}

	switch mode {
	case 't':
		// Listing does not write.
		return extraction{applies: false, recognized: true}
	case 'x':
		dir := firstNonEmpty(pr.Values["-C"], pr.Values["--one-top-level"])
		if dir == "" {
			// A bare --one-top-level derives its directory from the archive name
			// under the working directory; default to the working directory.
			dir = "."
		}
		ext.operands = append(ext.operands, rawOperand{raw: dir, role: risktypes.OperandRoleWrite})
		return ext
	case 'c':
		archive := firstNonEmpty(pr.Values["-f"])
		if archive != "" && archive != "-" {
			ext.operands = append(ext.operands, rawOperand{raw: archive, role: risktypes.OperandRoleWrite})
		}
		return ext
	default:
		ext.recognized = false
		return ext
	}
}

// tarMode returns 'x'/'t'/'c' from the first mode-bearing token, or 0 if unknown.
func tarMode(args []string) byte {
	for i, a := range args {
		if a == "" {
			continue
		}
		// Only the first token may be a dash-less mode bundle (e.g. "xzf"); any
		// other non-flag token is a positional (e.g. "a.tar") and must not be
		// scanned for mode letters, or "a.tar" would be misread as create/list.
		if i > 0 && !strings.HasPrefix(a, "-") {
			continue
		}
		token := a
		if strings.HasPrefix(token, "--") {
			switch token {
			case "--extract", "--get":
				return 'x'
			case "--list":
				return 't'
			case "--create":
				return 'c'
			}
			continue
		}
		token = strings.TrimPrefix(token, "-")
		for _, c := range token {
			switch c {
			case 'x':
				return 'x'
			case 't':
				return 't'
			case 'c':
				return 'c'
			}
		}
	}
	return 0
}

// normalizeTarArgs strips a leading dash-less mode bundle so the generic scanner
// treats the remaining tokens uniformly.
func normalizeTarArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	first := args[0]
	if first == "" || first[0] == '-' {
		return args
	}
	// A leading bundle like "xzf" -> "-xzf" so parseArgs reads it as flags.
	return append([]string{"-" + first}, args[1:]...)
}

func extractUnzip(pr ParseResult, args []string) extraction {
	// Listing (-l/-Z) is detected cluster-blind on raw argv (legacy parity): -l/-Z are
	// deliberately NOT declared flags, so unzip -lo is recognized=false (the cluster hits
	// the unknown -l) while unzip -l is applies=false.
	if hasAny(args, set("-l", "-Z")) {
		return extraction{applies: false, recognized: true}
	}
	ext := extraction{applies: true, recognized: pr.Recognized}
	dir := firstNonEmpty(pr.Values["-d"])
	if dir == "" {
		dir = "."
	}
	ext.operands = append(ext.operands, rawOperand{raw: dir, role: risktypes.OperandRoleWrite})
	return ext
}

func extractDD(_ ParseResult, args []string) extraction {
	// dd has no getopt flags: it parses if=/of= key=value pairs directly from raw argv.
	ext := extraction{applies: true, recognized: true}
	for _, a := range args {
		key, val, ok := strings.Cut(a, "=")
		if !ok {
			// dd takes only key=value operands; anything else is unparsed.
			ext.recognized = false
			continue
		}
		switch key {
		case "of":
			ext.operands = append(ext.operands, rawOperand{raw: val, role: risktypes.OperandRoleWrite})
		case "if":
			ext.operands = append(ext.operands, rawOperand{raw: val, role: risktypes.OperandRoleRead})
		}
	}
	// dd with neither if= nor of= reads stdin and writes stdout: axis 2 does not
	// apply. (A parse failure above is preserved so a malformed dd still fails.)
	if len(ext.operands) == 0 && ext.recognized {
		return extraction{applies: false, recognized: true}
	}
	return ext
}

func extractMknod(pr ParseResult, _ []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized}
	for _, m := range pr.Values["-m"] {
		if chmodGrantsHigh(m) {
			ext.grantsPermission = true
		}
	}
	// mknod NAME TYPE [MAJOR MINOR]: only NAME is a path operand.
	if len(pr.NonFlagArgs) == 0 {
		ext.recognized = false
		return ext
	}
	ext.operands = append(ext.operands, rawOperand{raw: pr.NonFlagArgs[0], role: risktypes.OperandRoleWrite})
	return ext
}

func extractUmount(pr ParseResult, args []string) extraction {
	// umountAll is a cluster-blind whole-token check on raw argv (legacy parity).
	ext := extraction{applies: true, recognized: pr.Recognized, umountAll: hasAny(args, set("-a", "--all"))}
	for _, p := range pr.NonFlagArgs {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	if len(pr.NonFlagArgs) == 0 && !ext.umountAll {
		ext.recognized = false
	}
	return ext
}

func extractChmod(pr ParseResult, _ []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized, recursive: pr.Recursive}
	pos := pr.NonFlagArgs
	if len(pos) < minSpecAndTarget {
		ext.recognized = false
		return ext
	}
	mode := pos[0]
	ext.grantsPermission = chmodGrantsHigh(mode)
	for _, t := range pos[1:] {
		ext.operands = append(ext.operands, rawOperand{raw: t, role: risktypes.OperandRoleWrite})
	}
	return ext
}

func extractOwner(pr ParseResult, _ []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized, recursive: pr.Recursive}
	pos := pr.NonFlagArgs
	// Only --reference removes the owner/group spec positional. With --from (a
	// filter) there is still a spec positional: `chown --from=alice bob file`.
	targets := pos
	if !pr.HasFlag("--reference") {
		if len(pos) < minSpecAndTarget {
			ext.recognized = false
			return ext
		}
		targets = pos[1:]
	} else if len(pos) == 0 {
		ext.recognized = false
		return ext
	}
	for _, t := range targets {
		ext.operands = append(ext.operands, rawOperand{raw: t, role: risktypes.OperandRoleWrite})
	}
	return ext
}

func extractSetfacl(pr ParseResult, _ []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized, recursive: pr.Recursive}
	// An ACL entry that grants write to group or other expands permission.
	for _, m := range pr.Values["-m"] {
		if aclGrantsWrite(m) {
			ext.grantsPermission = true
		}
	}
	for _, t := range pr.NonFlagArgs {
		ext.operands = append(ext.operands, rawOperand{raw: t, role: risktypes.OperandRoleWrite})
	}
	if len(pr.NonFlagArgs) == 0 {
		ext.recognized = false
	}
	return ext
}

func extractChattr(_ ParseResult, args []string) extraction {
	// chattr is parsed WHOLE-TOKEN, not via the getopt parser: the real chattr CLI (and
	// the pre-refactor extractor) match each option token in full and never split a
	// cluster, so -VR is an unknown token (recognized=false), not -V -R. parseArgs would
	// split it and wrongly recognize it. Attribute tokens (+i/-a/=j) carry the mode;
	// -v/-p take a value; the rest are options or target files. The flag sets are derived
	// from chattrFlags() so the knowledge still lives in the declarative table. A literal
	// "--" is an unknown whole token here (recognized=false), matching legacy (chattr has
	// no getopt "--" terminator). The dispatcher's ParseResult is ignored.
	valueNames := make(map[string]struct{})
	boolNames := make(map[string]struct{})
	for _, f := range chattrFlags() {
		for _, n := range f.Names {
			if f.Arity == ArityNone {
				boolNames[n] = struct{}{}
			} else {
				valueNames[n] = struct{}{}
			}
		}
	}

	ext := extraction{applies: true, recognized: true}
	var targets []string
	hasMode := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "" {
			continue
		}
		if isChattrMode(a) {
			hasMode = true
			if strings.ContainsRune(a[1:], 'i') {
				// Adding or removing the immutable attribute is an integrity-control change.
				ext.grantsPermission = true
			}
			continue
		}
		if len(a) >= 2 && a[0] == '-' {
			if _, ok := valueNames[a]; ok {
				if i+1 < len(args) {
					i++ // skip the value token
				} else {
					ext.recognized = false // value flag missing its value
				}
				continue
			}
			if _, ok := boolNames[a]; ok {
				continue
			}
			ext.recognized = false // unknown option token (whole-token match, no cluster split)
			continue
		}
		targets = append(targets, a)
	}
	for _, t := range targets {
		ext.operands = append(ext.operands, rawOperand{raw: t, role: risktypes.OperandRoleWrite})
	}
	if !hasMode || len(targets) == 0 {
		ext.recognized = false
	}
	return ext
}

// isChattrMode reports whether token is a chattr attribute change (+/-/= followed
// by attribute letters), as opposed to an option like -R.
func isChattrMode(token string) bool {
	if len(token) < minChattrModeLen {
		return false
	}
	if token[0] != '+' && token[0] != '-' && token[0] != '=' {
		return false
	}
	for _, c := range token[1:] {
		switch c {
		case 'a', 'A', 'c', 'C', 'd', 'D', 'e', 'F', 'i', 'j', 'm', 'P', 's', 'S', 't', 'T', 'u':
		default:
			return false
		}
	}
	return true
}

func extractFind(_ ParseResult, args []string) extraction {
	// find parses roots and predicates positionally, not as getopt flags. The
	// dispatcher's ParseResult is ignored.
	ext := extraction{applies: true, recognized: true}
	// Roots precede the first predicate (a token starting with '-', '(', '!').
	var roots []string
	destructive := false
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "" {
			i++
			continue
		}
		if a[0] == '-' || a == "(" || a == ")" || a == "!" {
			break
		}
		roots = append(roots, a)
		i++
	}
	for ; i < len(args); i++ {
		switch args[i] {
		case "-delete":
			destructive = true
		case "-fprint", "-fprint0", "-fprintf":
			destructive = true
			if i+1 < len(args) {
				ext.operands = append(ext.operands, rawOperand{raw: args[i+1], role: risktypes.OperandRoleWrite})
				i++
			} else {
				// Missing the required output-file argument: cannot parse the
				// destination, so fail closed rather than report a parsed result.
				ext.recognized = false
			}
		case "-exec", "-execdir", "-ok", "-okdir":
			// Inner execution is handled by the indirect-execution path, not axis 2.
		}
	}
	if !destructive {
		return extraction{applies: false, recognized: true}
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	for _, root := range roots {
		ext.operands = append(ext.operands, rawOperand{raw: root, role: risktypes.OperandRoleWrite})
	}
	return ext
}

// hostTokenRe matches a bare host token (no path separators, optional leading
// user@ stripped by the caller), used to recognize an rsync/scp remote host.
var hostTokenRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// isRemoteTerminus reports whether an rsync/scp operand denotes a remote location.
// It uses rsync's own positional rule: a ':' appearing before the first '/' is the
// remote host separator. This uniformly covers host:path, user@host:path, the
// daemon bare module host::module, the relative form host:file / host: (which the
// global hasNetworkArguments misses because the path part has no '/'), and the
// bracketed IPv6 form [::1]:path / user@[2001:db8::1]:/path. A local path never has
// a ':' before its first '/'. URLs (rsync://...) are matched first. This detection
// stays inside the rsync/scp extractors, so the global network-argument check is
// unchanged and unrelated "::" arguments (std::string, HTTP::Tiny) on other
// commands are never misclassified.
func isRemoteTerminus(arg string) bool {
	if strings.Contains(arg, "://") {
		return true
	}
	// Strip a leading user@ when the '@' precedes any '/' or ':' (otherwise it is
	// part of a path or the host token, not a user prefix).
	rest := arg
	if at := strings.IndexByte(rest, '@'); at >= 0 {
		slash := strings.IndexByte(rest, '/')
		colon := strings.IndexByte(rest, ':')
		if (slash < 0 || at < slash) && (colon < 0 || at < colon) {
			rest = rest[at+1:]
		}
	}
	// Bracketed IPv6 host: [ipv6]:path (the colons live inside the brackets, so the
	// positional rule below cannot be applied directly).
	if strings.HasPrefix(rest, "[") {
		if cb := strings.IndexByte(rest, ']'); cb > 1 && cb+1 < len(rest) && rest[cb+1] == ':' {
			return !strings.ContainsRune(rest[:cb], '/')
		}
		return false
	}
	colon := strings.IndexByte(rest, ':')
	if colon <= 0 {
		return false
	}
	if slash := strings.IndexByte(rest, '/'); slash >= 0 && slash < colon {
		return false // a '/' before the ':' means a local path (./a:b, /abs:x)
	}
	return hostTokenRe.MatchString(rest[:colon])
}

// extractCurl extracts curl's local write destination (-o FILE, or -O which writes
// the URL basename into the working directory). The URL is a remote read source;
// its egress Medium is supplied by curl's network profile.
func extractCurl(pr ParseResult, args []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized}
	if out := firstNonEmpty(pr.Values["-o"]); out != "" && out != "-" {
		ext.operands = append(ext.operands, rawOperand{raw: out, role: risktypes.OperandRoleWrite})
	} else if hasAny(args, set("-O", "--remote-name")) {
		// -O writes a file named from the URL into the working directory. Cluster-blind
		// whole-token check on raw argv (legacy parity): curl -OL does NOT see -O here.
		ext.operands = append(ext.operands, rawOperand{raw: ".", role: risktypes.OperandRoleWrite})
	}
	// An uploaded local file (-T/--upload-file) is a read source, so a sensitive
	// upload is detected and audited (parity with scp/rsync).
	for _, up := range pr.Values["-T"] {
		if up != "" && up != "-" {
			ext.operands = append(ext.operands, rawOperand{raw: up, role: risktypes.OperandRoleRead})
		}
	}
	// URL positionals are remote read sources (egress via the profile).
	return ext
}

// extractWget extracts wget's local write destination (-O FILE, -P DIR, or the
// working directory by default).
func extractWget(pr ParseResult, _ []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized}
	switch {
	case firstNonEmpty(pr.Values["-O"]) != "":
		out := firstNonEmpty(pr.Values["-O"])
		if out != "-" {
			ext.operands = append(ext.operands, rawOperand{raw: out, role: risktypes.OperandRoleWrite})
		}
	case firstNonEmpty(pr.Values["-P"]) != "":
		ext.operands = append(ext.operands, rawOperand{raw: firstNonEmpty(pr.Values["-P"]), role: risktypes.OperandRoleWrite})
	default:
		// wget writes the URL basename into the working directory by default.
		ext.operands = append(ext.operands, rawOperand{raw: ".", role: risktypes.OperandRoleWrite})
	}
	// An uploaded local file (--post-file) is a read source (sensitive-upload
	// detection + audit), parity with scp/rsync.
	for _, up := range pr.Values["--post-file"] {
		if up != "" && up != "-" {
			ext.operands = append(ext.operands, rawOperand{raw: up, role: risktypes.OperandRoleRead})
		}
	}
	return ext
}

// extractRemoteCopy is the shared SRC... DEST extractor for scp/rsync. A remote
// destination (host:path / host::module / rsync://...) is an upload (egress); a local
// destination is zone-classified. The flag grammars differ (declared per command), but
// the positional handling is identical, so both commands share this ToExtraction.
func extractRemoteCopy(pr ParseResult, _ []string) extraction {
	ext := extraction{applies: true, recognized: pr.Recognized}
	pos := pr.NonFlagArgs
	if len(pos) < minSpecAndTarget {
		ext.recognized = false
		return ext
	}
	dest := pos[len(pos)-1]
	srcs := pos[:len(pos)-1]
	if isRemoteTerminus(dest) {
		// Upload to a remote location: there is no local path to zone-classify for
		// the destination; the egress floor (Medium) applies. The local sources are
		// still read operands (sensitive/trust-critical source detection + audit).
		ext.remoteEgress = true
	} else {
		ext.operands = append(ext.operands, rawOperand{raw: dest, role: risktypes.OperandRoleWrite})
	}
	// Local sources are read operands; a remote source has no local path to resolve
	// (resolving it would fail and fail-close the whole command to High), so skip it.
	for _, src := range srcs {
		if !isRemoteTerminus(src) {
			ext.operands = append(ext.operands, rawOperand{raw: src, role: risktypes.OperandRoleRead})
		}
	}
	return ext
}

// extractSftp treats sftp as a network egress: its actual writes live in an
// interactive session or a -b batch file, not in argv, so there is no local path
// to zone-classify and the egress Medium floor applies.
func extractSftp(_ ParseResult, _ []string) extraction {
	return extraction{applies: true, recognized: true, remoteEgress: true}
}

// operandFloor returns the operation-specific risk floor for one operand (and the
// reason for it). Floors are applied after the zone level and never demote a
// safe-zone operand below their level.
func (r *operandResolver) operandFloor(oz risktypes.OperandZone, op rawOperand, spec CommandFlagSpec, ext extraction, input ZoningInput) (runnertypes.RiskLevel, risktypes.ReasonCode) {
	floor := runnertypes.RiskLevelLow
	reason := risktypes.ReasonCode("")
	raise := func(l runnertypes.RiskLevel, rc risktypes.ReasonCode) {
		if l > floor {
			floor = l
			reason = rc
		}
	}

	// Permission/ownership/attribute grant -> High on the written target.
	if ext.grantsPermission && op.role == risktypes.OperandRoleWrite {
		raise(runnertypes.RiskLevelHigh, risktypes.ReasonPermissionGrant)
	}

	// Linking to a trust-critical target is High (a symlink into a system path is a
	// trust-boundary handle), even though the target is read, not written.
	if spec.Kind == KindLink && oz.Zone == risktypes.ZoneTrustCritical {
		raise(runnertypes.RiskLevelHigh, risktypes.ReasonTrustBoundaryWrite)
	}

	// Recursion reaching outside the safe-zone -> High.
	if ext.recursive && op.role == risktypes.OperandRoleWrite {
		switch oz.Zone {
		case risktypes.ZoneOrdinary, risktypes.ZoneTrustCritical, risktypes.ZoneUnresolved:
			raise(runnertypes.RiskLevelHigh, risktypes.ReasonRecursiveOutsideSafeZone)
		}
	}

	// Privileged-metadata copy: cp -p/-a of a setuid or root-owned source.
	if spec.Kind == KindCopyMove && op.role == risktypes.OperandRoleRead &&
		ext.preserveMeta && oz.Resolved != "" && r.sourceIsPrivileged(oz.Resolved) {
		raise(runnertypes.RiskLevelHigh, risktypes.ReasonPermissionGrant)
	}

	// Sensitive-source copy -> Medium read floor. Applies to both cp/mv and the
	// data-transfer copies (scp/rsync), so a sensitive source copied into a
	// safe-zone is Medium regardless of which copy command is used.
	if op.role == risktypes.OperandRoleRead &&
		(spec.Kind == KindCopyMove || spec.Kind == KindDataTransferWrite) {
		if oz.Zone == risktypes.ZoneTrustCritical || matchesSensitive(oz.Resolved, input.OutputCriticalPathPatterns) {
			raise(runnertypes.RiskLevelMedium, risktypes.ReasonSensitiveSourceCopy)
		}
	}

	return floor, reason
}

// deviceOperandLevel computes the risk level of a dd operand by device kind. A
// harmless sink (/dev/null, /dev/zero, ...) is Low even though /dev is a critical
// path; a block or dangerous character device is High. A regular-file operand
// falls back to its path zone, plus the sensitive-source Medium read floor.
func (r *operandResolver) deviceOperandLevel(oz risktypes.OperandZone, op rawOperand, input ZoningInput) (runnertypes.RiskLevel, risktypes.ReasonCode) {
	if oz.Zone == risktypes.ZoneUnresolved {
		return zoneLevel(oz.Zone, oz.Role, oz.Trusted), zoneReason(oz)
	}
	if isDev, harmless := r.deviceKind(oz.Resolved); isDev {
		if harmless {
			return runnertypes.RiskLevelLow, ""
		}
		return runnertypes.RiskLevelHigh, risktypes.ReasonDeviceIO
	}
	lvl := zoneLevel(oz.Zone, oz.Role, oz.Trusted)
	reason := zoneReason(oz)
	if op.role == risktypes.OperandRoleRead &&
		(oz.Zone == risktypes.ZoneTrustCritical || matchesSensitive(oz.Resolved, input.OutputCriticalPathPatterns)) {
		if runnertypes.RiskLevelMedium > lvl {
			lvl = runnertypes.RiskLevelMedium
		}
		reason = risktypes.ReasonSensitiveSourceCopy
	}
	return lvl, reason
}

// deviceKind reports whether resolved is a device node and, if so, whether it is a
// harmless sink (only certain character devices like /dev/null are harmless; a
// block device is never harmless).
func (r *operandResolver) deviceKind(resolved string) (isDevice, harmless bool) {
	info, err := r.lstat(resolved)
	if err != nil {
		return false, false
	}
	mode := info.Mode()
	if mode&fs.ModeDevice == 0 {
		return false, false
	}
	if mode&fs.ModeCharDevice == 0 {
		return true, false // block device
	}
	switch filepath.Clean(resolved) {
	case "/dev/null", "/dev/zero", "/dev/full", "/dev/random", "/dev/urandom", "/dev/tty":
		return true, true
	}
	return true, false
}

// sourceIsPrivileged reports whether resolved is a setuid/setgid or root-owned
// file, used for the cp -p/-a privileged-metadata floor.
func (r *operandResolver) sourceIsPrivileged(resolved string) bool {
	info, err := r.lstat(resolved)
	if err != nil {
		return false
	}
	if info.Mode()&(fs.ModeSetuid|fs.ModeSetgid) != 0 {
		return true
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Uid == 0 {
		return true
	}
	return false
}

// matchesSensitive reports whether resolved contains any sensitive substring
// (reusing the OutputCriticalPathPatterns set), case-insensitively.
func matchesSensitive(resolved string, patterns []string) bool {
	if resolved == "" {
		return false
	}
	lower := strings.ToLower(resolved)
	for _, p := range patterns {
		if p != "" && strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// chmodGrantsHigh reports whether a chmod mode argument grants setuid/setgid or
// world-write.
func chmodGrantsHigh(mode string) bool {
	if mode == "" {
		return false
	}
	if isOctal(mode) {
		// Parse numerically so any digit count (3, 4, or a leading-zero 5 like
		// 04755) is handled: a missed special/world-write bit would be a
		// setuid/world-writable grant escaping the High floor (fail-open).
		v, err := strconv.ParseInt(mode, 8, 32)
		if err != nil {
			return false
		}
		const (
			setuidBit     = 0o4000
			setgidBit     = 0o2000
			worldWriteBit = 0o0002
		)
		return v&(setuidBit|setgidBit) != 0 || v&worldWriteBit != 0
	}
	// Symbolic: a clause that ADDS or ASSIGNS (+/=) an s bit grants setuid/setgid,
	// or grants world-write when its "who" includes other or all. The perm letters
	// must be parsed per clause (a substring match like "+s" misses "u+xs").
	for _, clause := range strings.Split(mode, ",") {
		idx := strings.IndexAny(clause, "+=-")
		if idx < 0 {
			continue
		}
		op := clause[idx]
		if op == '-' {
			continue // removal is not a grant
		}
		who := clause[:idx]
		perm := clause[idx+1:]
		if strings.ContainsRune(perm, 's') {
			return true
		}
		if (who == "" || strings.ContainsAny(who, "oa")) && strings.ContainsRune(perm, 'w') {
			return true
		}
	}
	return false
}

func isOctal(s string) bool {
	for _, c := range s {
		if c < '0' || c > '7' {
			return false
		}
	}
	return s != ""
}

// aclGrantsWrite reports whether a setfacl entry grants write to group or other.
func aclGrantsWrite(entry string) bool {
	for _, e := range strings.Split(entry, ",") {
		fields := strings.Split(e, ":")
		if len(fields) < minACLFields {
			continue
		}
		who := strings.ToLower(strings.TrimSpace(fields[0]))
		// A default-ACL entry (default:g:staff:rwx or d:g:...) shifts the who field
		// by one; without this the group/other class would be missed (fail-open).
		if (who == "d" || who == "default") && len(fields) > minACLFields {
			who = strings.ToLower(strings.TrimSpace(fields[1]))
		}
		perms := fields[len(fields)-1]
		if (who == "g" || who == "group" || who == "o" || who == "other") && strings.Contains(perms, "w") {
			return true
		}
	}
	return false
}

func hasAny(args []string, flags map[string]struct{}) bool {
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if _, ok := flags[name]; ok {
			return true
		}
	}
	return false
}

func firstNonEmpty(lists ...[]string) string {
	for _, l := range lists {
		if len(l) > 0 && l[0] != "" {
			return l[0]
		}
	}
	return ""
}
