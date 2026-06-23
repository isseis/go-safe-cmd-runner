package security

import (
	"io/fs"
	"path/filepath"
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
	operands         []rawOperand
}

// commandSpec maps a command family to its extraction rule.
type commandSpec struct {
	kind    LocationKind
	extract func(args []string) extraction
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

// Octal chmod mode lengths: three permission digits, optionally preceded by a
// special digit (setuid/setgid/sticky).
const (
	octalModeLen            = 3
	octalModeLenWithSpecial = 4
)

func set(items ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, it := range items {
		m[it] = struct{}{}
	}
	return m
}

// lookupSpec finds the spec for a command by matching its resolved name set (and
// the basename of cmdPath) against the registry.
func lookupSpec(names map[string]struct{}, cmdPath string) (commandSpec, bool) {
	if base := filepath.Base(cmdPath); base != "" && base != "." && base != string(filepath.Separator) {
		if s, ok := zoningSpecs[base]; ok {
			return s, true
		}
	}
	for n := range names {
		if s, ok := zoningSpecs[n]; ok {
			return s, true
		}
	}
	return commandSpec{}, false
}

// zoningSpecs is the single command -> extraction-rule table.
var zoningSpecs = map[string]commandSpec{
	"cp":       {KindCopyMove, func(a []string) extraction { return extractCopyMove(a, false) }},
	"mv":       {KindCopyMove, func(a []string) extraction { return extractCopyMove(a, true) }},
	"rm":       {KindRemove, extractRemove},
	"rmdir":    {KindRemove, extractRemove},
	"unlink":   {KindRemove, extractRemove},
	"shred":    {KindRemove, extractShred},
	"ln":       {KindLink, extractLink},
	"truncate": {KindInPlaceEdit, extractTruncate},
	"sed":      {KindInPlaceEdit, extractSed},
	"touch": {KindWriteFile, func(a []string) extraction {
		return extractSimpleWrite(a, set("-r", "--reference", "-d", "--date", "-t"))
	}},
	"mkdir":   {KindWriteFile, func(a []string) extraction { return extractSimpleWrite(a, set("-m", "--mode")) }},
	"tee":     {KindWriteFile, extractTee},
	"sponge":  {KindWriteFile, func(a []string) extraction { return extractSimpleWrite(a, nil) }},
	"install": {KindWriteFile, extractInstall},
	"tar":     {KindArchiveExtract, extractTar},
	"unzip":   {KindArchiveExtract, extractUnzip},
	"dd":      {KindDeviceIO, extractDD},
	"mount":   {KindMount, extractMount},
	"umount":  {KindMount, extractUmount},
	"chmod":   {KindPermission, extractChmod},
	"chown":   {KindPermission, extractOwner},
	"chgrp":   {KindPermission, extractOwner},
	"setfacl": {KindPermission, extractSetfacl},
	"chattr":  {KindPermission, extractChattr},
	"find":    {KindFindDestructive, extractFind},
}

// scanFlags separates positionals from flags. recognized is false when a token
// that starts with '-' is not a known flag (it could be a value-taking flag whose
// value would otherwise be misread as an operand) or when a value flag is missing
// its value. captured records the values of value-taking flags.
func scanFlags(args []string, valueFlags, boolFlags, recursiveFlags map[string]struct{}) (positionals []string, captured map[string][]string, recursive, recognized bool) {
	recognized = true
	captured = make(map[string][]string)
	endOpts := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !endOpts && a == "--" {
			endOpts = true
			continue
		}
		if endOpts || len(a) < 2 || a[0] != '-' {
			positionals = append(positionals, a)
			continue
		}

		name, val, hasEq := strings.Cut(a, "=")
		if _, ok := recursiveFlags[name]; ok {
			recursive = true
		}
		if _, ok := valueFlags[name]; ok {
			switch {
			case hasEq:
				captured[name] = append(captured[name], val)
			case i+1 < len(args):
				captured[name] = append(captured[name], args[i+1])
				i++
			default:
				recognized = false
			}
			continue
		}
		if _, ok := boolFlags[name]; ok {
			continue
		}
		// A long flag (--foo) we do not know, or a short cluster like -rf.
		if strings.HasPrefix(a, "--") {
			recognized = false
			continue
		}
		if rec, ok := scanShortFlagCluster(a, valueFlags, boolFlags, recursiveFlags); ok {
			if rec {
				recursive = true
			}
			continue
		}
		recognized = false
	}
	return positionals, captured, recursive, recognized
}

// scanShortFlagCluster handles a bundled single-dash flag group (e.g. -rf). It returns
// ok=false if any character is not a known short bool/recursive flag, so the
// caller fails closed.
func scanShortFlagCluster(token string, valueFlags, boolFlags, recursiveFlags map[string]struct{}) (recursive, ok bool) {
	for _, c := range token[1:] {
		key := "-" + string(c)
		if _, isVal := valueFlags[key]; isVal {
			// A value-taking flag inside a cluster consumes the remainder; treat as
			// recognized to avoid misreading it, without trying to locate operands.
			return recursive, true
		}
		if _, isRec := recursiveFlags[key]; isRec {
			recursive = true
			continue
		}
		if _, isBool := boolFlags[key]; isBool {
			continue
		}
		return recursive, false
	}
	return recursive, true
}

func extractCopyMove(args []string, isMove bool) extraction {
	valueFlags := set("-t", "--target-directory", "-S", "--suffix")
	boolFlags := set("-f", "--force", "-i", "--interactive", "-n", "--no-clobber", "-v", "--verbose",
		"-u", "--update", "-d", "-L", "--dereference", "-P", "--no-dereference", "-H", "-s", "--symbolic-link",
		"-l", "--link", "-T", "--no-target-directory", "-b", "--backup", "-x", "--one-file-system")
	recursiveFlags := set("-r", "-R", "--recursive", "-a", "--archive")
	preserveFlags := set("-a", "--archive", "-p", "--preserve")

	pos, captured, recursive, recognized := scanFlags(args, valueFlags, boolFlags, recursiveFlags)
	ext := extraction{applies: true, recognized: recognized, recursive: recursive}
	ext.preserveMeta = hasAny(args, preserveFlags)

	srcRole := risktypes.OperandRoleRead
	if isMove {
		// mv removes the source, so a trust-critical move source is itself a write.
		srcRole = risktypes.OperandRoleWrite
	}

	if tdirs := captured["-t"]; len(tdirs) > 0 {
		appendTargetDir(&ext, tdirs)
		for _, s := range pos {
			ext.operands = append(ext.operands, rawOperand{raw: s, role: srcRole})
		}
		return ext
	}
	if tdirs := captured["--target-directory"]; len(tdirs) > 0 {
		appendTargetDir(&ext, tdirs)
		for _, s := range pos {
			ext.operands = append(ext.operands, rawOperand{raw: s, role: srcRole})
		}
		return ext
	}

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

func extractRemove(args []string) extraction {
	boolFlags := set("-f", "--force", "-i", "-I", "--interactive", "-v", "--verbose", "-d", "--dir",
		"--one-file-system", "-p", "--parents", "--ignore-fail-on-non-empty")
	recursiveFlags := set("-r", "-R", "--recursive")
	pos, _, recursive, recognized := scanFlags(args, nil, boolFlags, recursiveFlags)
	ext := extraction{applies: true, recognized: recognized, recursive: recursive}
	for _, p := range pos {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	if len(pos) == 0 {
		ext.recognized = false
	}
	return ext
}

func extractShred(args []string) extraction {
	valueFlags := set("-n", "--iterations", "-s", "--size")
	boolFlags := set("-f", "--force", "-u", "--remove", "-v", "--verbose", "-z", "--zero", "-x", "--exact")
	pos, _, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	for _, p := range pos {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	if len(pos) == 0 {
		ext.recognized = false
	}
	return ext
}

func extractLink(args []string) extraction {
	valueFlags := set("-t", "--target-directory", "-S", "--suffix")
	boolFlags := set("-s", "--symbolic", "-f", "--force", "-n", "--no-dereference", "-r", "--relative",
		"-v", "--verbose", "-i", "--interactive", "-T", "--no-target-directory", "-b", "-L", "-P")
	pos, captured, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}

	if tdirs := captured["-t"]; len(tdirs) > 0 {
		appendTargetDir(&ext, tdirs)
		for _, t := range pos {
			ext.operands = append(ext.operands, rawOperand{raw: t, role: risktypes.OperandRoleRead})
		}
		return ext
	}

	switch len(pos) {
	case 0:
		ext.recognized = false
	case 1:
		// ln TARGET -> link with TARGET's basename in the working directory.
		ext.operands = append(ext.operands, rawOperand{raw: pos[0], role: risktypes.OperandRoleRead})
	default:
		// ln TARGET LINKNAME: a relative target resolves against the link's parent.
		linkName := pos[len(pos)-1]
		linkParent := filepath.Dir(linkName)
		for _, t := range pos[:len(pos)-1] {
			ext.operands = append(ext.operands, rawOperand{raw: t, role: risktypes.OperandRoleRead, base: linkParent})
		}
		ext.operands = append(ext.operands, rawOperand{raw: linkName, role: risktypes.OperandRoleWrite})
	}
	return ext
}

func extractTruncate(args []string) extraction {
	valueFlags := set("-s", "--size", "-r", "--reference")
	boolFlags := set("-c", "--no-create", "-o", "--io-blocks")
	pos, _, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	for _, p := range pos {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	if len(pos) == 0 {
		ext.recognized = false
	}
	return ext
}

func extractSed(args []string) extraction {
	// sed edits in place only with -i / --in-place (which may carry an attached
	// backup suffix, e.g. -i.bak). Without it, sed writes to stdout, so axis 2 does
	// not apply.
	valueFlags := set("-e", "--expression", "-f", "--file", "-l", "--line-length")
	boolFlags := set("-n", "--quiet", "--silent", "-r", "-E", "--regexp-extended", "-s", "--separate",
		"-z", "--null-data", "-u", "--unbuffered", "--posix", "--debug", "--sandbox", "--follow-symlinks")
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
	pos, _, _, recognized := scanFlags(rest, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	// The first positional is the sed script; the rest are edited files.
	if len(pos) <= 1 {
		ext.recognized = false
		return ext
	}
	for _, f := range pos[1:] {
		ext.operands = append(ext.operands, rawOperand{raw: f, role: risktypes.OperandRoleWrite})
	}
	return ext
}

func extractSimpleWrite(args []string, valueFlags map[string]struct{}) extraction {
	boolFlags := set("-a", "--append", "-c", "--no-create", "-m", "-h", "--no-dereference", "-p", "--parents",
		"-v", "--verbose", "-f", "-i", "-r")
	pos, _, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	for _, p := range pos {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	if len(pos) == 0 {
		ext.recognized = false
	}
	return ext
}

func extractTee(args []string) extraction {
	valueFlags := set("--output-error")
	boolFlags := set("-a", "--append", "-i", "--ignore-interrupts", "-p")
	pos, _, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	for _, p := range pos {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	return ext
}

func extractInstall(args []string) extraction {
	valueFlags := set("-m", "--mode", "-o", "--owner", "-g", "--group", "-t", "--target-directory",
		"-S", "--suffix", "-b", "--backup")
	boolFlags := set("-d", "--directory", "-D", "-v", "--verbose", "-p", "--preserve-timestamps",
		"-c", "-C", "--compare", "-s", "--strip", "-T", "--no-target-directory")
	pos, captured, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}

	// Permission/ownership grant: -m with a setuid/setgid mode, or any -o/-g.
	if modes := append(append([]string{}, captured["-m"]...), captured["--mode"]...); len(modes) > 0 {
		for _, m := range modes {
			if chmodGrantsHigh(m) {
				ext.grantsPermission = true
			}
		}
	}
	if len(captured["-o"]) > 0 || len(captured["--owner"]) > 0 || len(captured["-g"]) > 0 || len(captured["--group"]) > 0 {
		ext.grantsPermission = true
	}

	if tdirs := append(append([]string{}, captured["-t"]...), captured["--target-directory"]...); len(tdirs) > 0 {
		appendTargetDir(&ext, tdirs)
		for _, s := range pos {
			ext.operands = append(ext.operands, rawOperand{raw: s, role: risktypes.OperandRoleRead})
		}
		return ext
	}

	switch len(pos) {
	case 0:
		ext.recognized = false
	case 1:
		// install -d DIR, or a single FILE: treat as a write destination.
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

func extractTar(args []string) extraction {
	valueFlags := set("-f", "--file", "-C", "--directory", "--one-top-level")
	boolFlags := set("-v", "--verbose", "-z", "--gzip", "-j", "--bzip2", "-J", "--xz", "-p",
		"--preserve-permissions", "-k", "--keep-old-files", "--no-same-owner", "-m", "--touch")
	// tar accepts a leading bundled mode token without a dash (e.g. "xzf").
	mode := tarMode(args)

	pos, captured, _, recognized := scanFlags(normalizeTarArgs(args), valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}

	switch mode {
	case 't':
		// Listing does not write.
		return extraction{applies: false, recognized: true}
	case 'x':
		dir := firstNonEmpty(captured["-C"], captured["--directory"], captured["--one-top-level"])
		if dir == "" {
			dir = "."
		}
		ext.operands = append(ext.operands, rawOperand{raw: dir, role: risktypes.OperandRoleWrite})
		return ext
	case 'c':
		archive := firstNonEmpty(captured["-f"], captured["--file"])
		if archive != "" && archive != "-" {
			ext.operands = append(ext.operands, rawOperand{raw: archive, role: risktypes.OperandRoleWrite})
		}
		return ext
	default:
		ext.recognized = false
		_ = pos
		return ext
	}
}

// tarMode returns 'x'/'t'/'c' from the first mode-bearing token, or 0 if unknown.
func tarMode(args []string) byte {
	for _, a := range args {
		if a == "" {
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
	// A leading bundle like "xzf" -> "-xzf" so scanFlags reads it as flags.
	return append([]string{"-" + first}, args[1:]...)
}

func extractUnzip(args []string) extraction {
	valueFlags := set("-d", "-x")
	boolFlags := set("-o", "-n", "-q", "-qq", "-v", "-j", "-a", "-u", "-f")
	listing := hasAny(args, set("-l", "-Z"))
	pos, captured, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	if listing {
		return extraction{applies: false, recognized: true}
	}
	ext := extraction{applies: true, recognized: recognized}
	dir := firstNonEmpty(captured["-d"])
	if dir == "" {
		dir = "."
	}
	ext.operands = append(ext.operands, rawOperand{raw: dir, role: risktypes.OperandRoleWrite})
	_ = pos
	return ext
}

func extractDD(args []string) extraction {
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
	return ext
}

func extractMount(args []string) extraction {
	valueFlags := set("-t", "--types", "-o", "--options", "-O")
	boolFlags := set("-a", "--all", "-r", "--read-only", "-w", "--rw", "-v", "--verbose", "-n",
		"--bind", "--rbind", "--move", "-B", "-R", "-M", "-f", "--fake")
	pos, _, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	// Every positional (device/source and mountpoint) is a write target.
	for _, p := range pos {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	if len(pos) == 0 {
		ext.recognized = false
	}
	return ext
}

func extractUmount(args []string) extraction {
	valueFlags := set("-t", "--types", "-O")
	boolFlags := set("-a", "--all", "-r", "--read-only", "-v", "--verbose", "-n", "-l", "--lazy",
		"-f", "--force", "-R", "--recursive", "-d")
	pos, _, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized, umountAll: hasAny(args, set("-a", "--all"))}
	for _, p := range pos {
		ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
	}
	if len(pos) == 0 && !ext.umountAll {
		ext.recognized = false
	}
	return ext
}

func extractChmod(args []string) extraction {
	boolFlags := set("-R", "--recursive", "-v", "--verbose", "-c", "--changes", "-f", "--silent", "--quiet")
	recursiveFlags := set("-R", "--recursive")
	pos, _, recursive, recognized := scanFlags(args, nil, boolFlags, recursiveFlags)
	ext := extraction{applies: true, recognized: recognized, recursive: recursive}
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

func extractOwner(args []string) extraction {
	valueFlags := set("--from", "--reference")
	boolFlags := set("-R", "--recursive", "-v", "--verbose", "-c", "--changes", "-f", "--silent", "--quiet",
		"-h", "--no-dereference", "-H", "-L", "-P", "--dereference")
	recursiveFlags := set("-R", "--recursive")
	pos, captured, recursive, recognized := scanFlags(args, valueFlags, boolFlags, recursiveFlags)
	ext := extraction{applies: true, recognized: recognized, recursive: recursive}
	// chown/chgrp with --reference takes only files; otherwise pos[0] is the owner spec.
	targets := pos
	if len(captured["--reference"]) == 0 && len(captured["--from"]) == 0 {
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

func extractSetfacl(args []string) extraction {
	valueFlags := set("-m", "--modify", "-x", "--remove", "-M", "--modify-file", "-X", "--restore", "-n")
	boolFlags := set("-R", "--recursive", "-b", "--remove-all", "-k", "--remove-default", "-d", "--default",
		"-v", "--version", "-t", "-p", "--restore-stdin")
	recursiveFlags := set("-R", "--recursive")
	pos, captured, recursive, recognized := scanFlags(args, valueFlags, boolFlags, recursiveFlags)
	ext := extraction{applies: true, recognized: recognized, recursive: recursive}
	// An ACL entry that grants write to group or other expands permission.
	for _, m := range append(append([]string{}, captured["-m"]...), captured["--modify"]...) {
		if aclGrantsWrite(m) {
			ext.grantsPermission = true
		}
	}
	for _, t := range pos {
		ext.operands = append(ext.operands, rawOperand{raw: t, role: risktypes.OperandRoleWrite})
	}
	if len(pos) == 0 {
		ext.recognized = false
	}
	return ext
}

func extractChattr(args []string) extraction {
	valueFlags := set("-v", "-p")
	boolFlags := set("-R", "-f", "-V", "-H", "-L", "-P")
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
				// Adding or removing the immutable attribute is an integrity-control
				// change.
				ext.grantsPermission = true
			}
			continue
		}
		if len(a) >= 2 && a[0] == '-' {
			name := a
			if _, ok := valueFlags[name]; ok {
				i++ // skip value
				continue
			}
			if _, ok := boolFlags[name]; ok {
				continue
			}
			ext.recognized = false
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
	const attrLetters = "aAcCdDeFijmPsStTu"
	for _, c := range token[1:] {
		if !strings.ContainsRune(attrLetters, c) {
			return false
		}
	}
	return true
}

func extractFind(args []string) extraction {
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

// operandFloor returns the operation-specific risk floor for one operand (and the
// reason for it). Floors are applied after the zone level and never demote a
// safe-zone operand below their level.
func (r *operandResolver) operandFloor(oz risktypes.OperandZone, op rawOperand, spec commandSpec, ext extraction, input ZoningInput) (runnertypes.RiskLevel, risktypes.ReasonCode) {
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
	if spec.kind == KindLink && oz.Zone == risktypes.ZoneTrustCritical {
		raise(runnertypes.RiskLevelHigh, risktypes.ReasonTrustBoundaryWrite)
	}

	// Recursion reaching outside the safe-zone -> High.
	if ext.recursive && op.role == risktypes.OperandRoleWrite {
		switch oz.Zone {
		case risktypes.ZoneOrdinary, risktypes.ZoneTrustCritical, risktypes.ZoneUnresolved:
			raise(runnertypes.RiskLevelHigh, risktypes.ReasonRecursiveOutsideSafeZone)
		}
	}

	// Copy floors on the read source.
	if spec.kind == KindCopyMove && op.role == risktypes.OperandRoleRead {
		// Privileged-metadata copy: cp -p/-a of a setuid or root-owned source.
		if ext.preserveMeta && oz.Resolved != "" && r.sourceIsPrivileged(oz.Resolved) {
			raise(runnertypes.RiskLevelHigh, risktypes.ReasonPermissionGrant)
		}
		// Sensitive-source copy -> Medium read floor.
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
		switch len(mode) {
		case octalModeLenWithSpecial:
			special := mode[0] - '0'
			if special&0o6 != 0 { // setuid (4) or setgid (2)
				return true
			}
			return (mode[3]-'0')&0o2 != 0 // world-write
		case octalModeLen:
			return (mode[2]-'0')&0o2 != 0
		default:
			return false
		}
	}
	// Symbolic: an added/assigned setuid/setgid bit, or world-write for a clause
	// whose "who" includes other or all.
	if strings.Contains(mode, "+s") || strings.Contains(mode, "=s") {
		return true
	}
	for _, clause := range strings.Split(mode, ",") {
		for _, op := range []string{"+", "="} {
			if idx := strings.Index(clause, op); idx >= 0 {
				who := clause[:idx]
				perm := clause[idx+1:]
				if (who == "" || strings.ContainsAny(who, "oa")) && strings.Contains(perm, "w") {
					return true
				}
			}
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
