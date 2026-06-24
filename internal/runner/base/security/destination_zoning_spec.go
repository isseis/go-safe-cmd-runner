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
	"mknod":   {KindWriteFile, extractMknod},
	"mount":   {KindMount, extractMount},
	"umount":  {KindMount, extractUmount},
	"chmod":   {KindPermission, extractChmod},
	"chown":   {KindPermission, extractOwner},
	"chgrp":   {KindPermission, extractOwner},
	"setfacl": {KindPermission, extractSetfacl},
	"chattr":  {KindPermission, extractChattr},
	"find":    {KindFindDestructive, extractFind},
	"curl":    {KindDataTransferWrite, extractCurl},
	"wget":    {KindDataTransferWrite, extractWget},
	"scp":     {KindDataTransferWrite, extractScp},
	"sftp":    {KindDataTransferWrite, extractSftp},
	"rsync":   {KindDataTransferWrite, extractRsync},
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
		// An unknown long flag (--foo): fail closed (it may take a value).
		if strings.HasPrefix(a, "--") {
			recognized = false
			continue
		}
		// A short cluster (single dash, e.g. -rf, or a value flag with an attached
		// value like -C/usr). A value flag inside the cluster captures the rest of
		// the token as its value, or the next token if none is attached; dropping
		// that value would default an extraction directory to the cwd (fail-open).
		clusterOK := true
		for k, c := range a[1:] {
			key := "-" + string(c)
			if _, isRec := recursiveFlags[key]; isRec {
				recursive = true
				continue
			}
			if _, isVal := valueFlags[key]; isVal {
				rest := a[2+k:] // characters after c within this token
				switch {
				case rest != "":
					captured[key] = append(captured[key], rest)
				case i+1 < len(args):
					captured[key] = append(captured[key], args[i+1])
					i++
				default:
					clusterOK = false
				}
				break // a value flag consumes the remainder of the cluster
			}
			if _, isBool := boolFlags[key]; isBool {
				continue
			}
			clusterOK = false
			break
		}
		if !clusterOK {
			recognized = false
		}
	}
	return positionals, captured, recursive, recognized
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

	if tdirs := append(append([]string{}, captured["-t"]...), captured["--target-directory"]...); len(tdirs) > 0 {
		appendTargetDir(&ext, tdirs)
		for _, s := range pos {
			ext.operands = append(ext.operands, rawOperand{raw: s, role: srcRole})
		}
		if len(pos) == 0 {
			// -t with no source files is an incomplete copy/move: fail closed.
			ext.recognized = false
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

	if tdirs := append(append([]string{}, captured["-t"]...), captured["--target-directory"]...); len(tdirs) > 0 {
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
	// link; a hard link's target resolves against the working directory.
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
	pos, captured, _, recognized := scanFlags(rest, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	// When the script is supplied via -e/-f, every positional is an edited file;
	// otherwise the first positional is the inline script and the rest are files.
	hasScriptFlag := len(captured["-e"]) > 0 || len(captured["--expression"]) > 0 ||
		len(captured["-f"]) > 0 || len(captured["--file"]) > 0
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

func extractSimpleWrite(args []string, valueFlags map[string]struct{}) extraction {
	boolFlags := set("-a", "--append", "-c", "--no-create", "-h", "--no-dereference", "-p", "--parents",
		"-v", "--verbose", "-f", "-i", "-r")
	pos, captured, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	// A mode-granting flag (e.g. mkdir -m 0777 / -m u+s) is a permission grant even
	// in a safe-zone (only meaningful when -m is in this command's valueFlags).
	for _, m := range append(append([]string{}, captured["-m"]...), captured["--mode"]...) {
		if chmodGrantsHigh(m) {
			ext.grantsPermission = true
		}
	}
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
	if len(pos) == 0 {
		// tee with no FILE writes only to stdout: axis 2 does not apply.
		return extraction{applies: false, recognized: true}
	}
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

	// Directory-creation mode: every positional is a directory to create (write).
	if hasAny(args, set("-d", "--directory")) {
		for _, p := range pos {
			ext.operands = append(ext.operands, rawOperand{raw: p, role: risktypes.OperandRoleWrite})
		}
		if len(pos) == 0 {
			ext.recognized = false
		}
		return ext
	}

	if tdirs := append(append([]string{}, captured["-t"]...), captured["--target-directory"]...); len(tdirs) > 0 {
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

func extractTar(args []string) extraction {
	valueFlags := set("-f", "--file", "-C", "--directory")
	boolFlags := set("-v", "--verbose", "-z", "--gzip", "-j", "--bzip2", "-J", "--xz", "-p",
		"--preserve-permissions", "-k", "--keep-old-files", "--no-same-owner", "-m", "--touch",
		// --one-top-level takes an OPTIONAL argument (only via =DIR); treat the flag
		// itself as boolean for recognition and read =DIR separately, so
		// `tar --one-top-level -xf a.tar` is not misparsed.
		"--one-top-level",
		// Mode letters/long forms, so a recognized extract form (e.g. -xf, --extract)
		// is not falsely treated as unparsed and floored to High.
		"-x", "-t", "-c", "--extract", "--get", "--list", "--create")
	// tar accepts a leading bundled mode token without a dash (e.g. "xzf").
	mode := tarMode(args)

	pos, captured, _, recognized := scanFlags(normalizeTarArgs(args), valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}

	switch mode {
	case 't':
		// Listing does not write.
		return extraction{applies: false, recognized: true}
	case 'x':
		dir := firstNonEmpty(captured["-C"], captured["--directory"], attachedValue(args, "--one-top-level"))
		if dir == "" {
			// A bare --one-top-level derives its directory from the archive name
			// under the working directory; default to the working directory.
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
	// dd with neither if= nor of= reads stdin and writes stdout: axis 2 does not
	// apply. (A parse failure above is preserved so a malformed dd still fails.)
	if len(ext.operands) == 0 && ext.recognized {
		return extraction{applies: false, recognized: true}
	}
	return ext
}

func extractMknod(args []string) extraction {
	valueFlags := set("-m", "--mode", "-Z", "--context")
	boolFlags := set("-v", "--verbose")
	pos, captured, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	for _, m := range append(append([]string{}, captured["-m"]...), captured["--mode"]...) {
		if chmodGrantsHigh(m) {
			ext.grantsPermission = true
		}
	}
	// mknod NAME TYPE [MAJOR MINOR]: only NAME is a path operand.
	if len(pos) == 0 {
		ext.recognized = false
		return ext
	}
	ext.operands = append(ext.operands, rawOperand{raw: pos[0], role: risktypes.OperandRoleWrite})
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
	// Only --reference removes the owner/group spec positional. With --from (a
	// filter) there is still a spec positional: `chown --from=alice bob file`.
	targets := pos
	if len(captured["--reference"]) == 0 {
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
				if i+1 < len(args) {
					i++ // skip the value token
				} else {
					ext.recognized = false // value flag missing its value
				}
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
	for _, c := range token[1:] {
		switch c {
		case 'a', 'A', 'c', 'C', 'd', 'D', 'e', 'F', 'i', 'j', 'm', 'P', 's', 'S', 't', 'T', 'u':
		default:
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
// daemon bare module host::module, AND the relative form host:file / host: (the
// last of which the global hasNetworkArguments misses because the path part has no
// '/'). A local path never has a ':' before its first '/'. URLs (rsync://...) are
// matched first. This detection stays inside the rsync/scp extractors, so the
// global network-argument check is unchanged and unrelated "::" arguments
// (std::string, HTTP::Tiny) on other commands are never misclassified.
func isRemoteTerminus(arg string) bool {
	if strings.Contains(arg, "://") {
		return true
	}
	colon := strings.IndexByte(arg, ':')
	if colon <= 0 {
		return false
	}
	if slash := strings.IndexByte(arg, '/'); slash >= 0 && slash < colon {
		return false // a '/' before the ':' means a local path (./a:b, /abs:x)
	}
	host := arg[:colon]
	if at := strings.IndexByte(host, '@'); at >= 0 {
		host = host[at+1:]
	}
	return host != "" && hostTokenRe.MatchString(host)
}

// extractCurl extracts curl's local write destination (-o FILE, or -O which writes
// the URL basename into the working directory). The URL is a remote read source;
// its egress Medium is supplied by curl's network profile.
func extractCurl(args []string) extraction {
	valueFlags := set("-o", "--output", "-H", "--header", "-d", "--data", "--data-raw", "--data-binary",
		"-u", "--user", "-A", "--user-agent", "-e", "--referer", "-x", "--proxy", "-b", "--cookie",
		"-c", "--cookie-jar", "-K", "--config", "-T", "--upload-file", "-w", "--write-out",
		"-m", "--max-time", "--connect-timeout", "-X", "--request", "--url", "--retry", "--limit-rate",
		"-C", "--continue-at", "-r", "--range", "--cacert", "--cert", "--key")
	boolFlags := set("-O", "--remote-name", "-L", "--location", "-s", "--silent", "-S", "--show-error",
		"-f", "--fail", "-k", "--insecure", "-v", "--verbose", "-i", "--include", "-I", "--head",
		"-g", "--progress-bar", "-J", "--remote-header-name", "-#", "-q", "-z")
	pos, captured, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	if out := firstNonEmpty(captured["-o"], captured["--output"]); out != "" && out != "-" {
		ext.operands = append(ext.operands, rawOperand{raw: out, role: risktypes.OperandRoleWrite})
	} else if hasAny(args, set("-O", "--remote-name")) {
		// -O writes a file named from the URL into the working directory.
		ext.operands = append(ext.operands, rawOperand{raw: ".", role: risktypes.OperandRoleWrite})
	}
	// An uploaded local file (-T/--upload-file) is a read source, so a sensitive
	// upload is detected and audited (parity with scp/rsync).
	for _, up := range append(append([]string{}, captured["-T"]...), captured["--upload-file"]...) {
		if up != "" && up != "-" {
			ext.operands = append(ext.operands, rawOperand{raw: up, role: risktypes.OperandRoleRead})
		}
	}
	_ = pos // URL positionals are remote read sources (egress via the profile).
	return ext
}

// extractWget extracts wget's local write destination (-O FILE, -P DIR, or the
// working directory by default).
func extractWget(args []string) extraction {
	valueFlags := set("-O", "--output-document", "-P", "--directory-prefix", "-o", "--output-file",
		"-a", "--append-output", "--header", "--user", "--password", "--limit-rate", "-t", "--tries",
		"-T", "--timeout", "--user-agent", "-U", "--referer", "--post-data", "--post-file", "-e", "--execute",
		"--ca-certificate", "--certificate")
	boolFlags := set("-q", "--quiet", "-v", "--verbose", "-c", "--continue", "-N", "--timestamping",
		"-r", "--recursive", "-np", "--no-parent", "-nc", "--no-clobber", "-nv", "--no-verbose",
		"--no-check-certificate", "-d", "--debug", "-b", "--background")
	pos, captured, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
	switch {
	case firstNonEmpty(captured["-O"], captured["--output-document"]) != "":
		out := firstNonEmpty(captured["-O"], captured["--output-document"])
		if out != "-" {
			ext.operands = append(ext.operands, rawOperand{raw: out, role: risktypes.OperandRoleWrite})
		}
	case firstNonEmpty(captured["-P"], captured["--directory-prefix"]) != "":
		ext.operands = append(ext.operands, rawOperand{raw: firstNonEmpty(captured["-P"], captured["--directory-prefix"]), role: risktypes.OperandRoleWrite})
	default:
		// wget writes the URL basename into the working directory by default.
		ext.operands = append(ext.operands, rawOperand{raw: ".", role: risktypes.OperandRoleWrite})
	}
	// An uploaded local file (--post-file) is a read source (sensitive-upload
	// detection + audit), parity with scp/rsync.
	for _, up := range captured["--post-file"] {
		if up != "" && up != "-" {
			ext.operands = append(ext.operands, rawOperand{raw: up, role: risktypes.OperandRoleRead})
		}
	}
	_ = pos
	return ext
}

// extractScp extracts scp's destination (the final operand). A remote destination
// is an upload (egress); a local destination is zone-classified.
func extractScp(args []string) extraction {
	valueFlags := set("-P", "-i", "-o", "-c", "-F", "-l", "-S", "-J", "-T")
	boolFlags := set("-r", "-p", "-q", "-v", "-C", "-B", "-3", "-4", "-6", "-A", "-O", "-R")
	return extractRemoteCopy(args, valueFlags, boolFlags)
}

// extractRsync extracts rsync's destination (the final operand). A remote
// destination (host:path / host::module / rsync://...) is an upload (egress); a
// local destination is zone-classified. --delete acts on the destination tree.
func extractRsync(args []string) extraction {
	valueFlags := set("-e", "--rsh", "--rsync-path", "--exclude", "--include", "--exclude-from",
		"--include-from", "-f", "--filter", "--files-from", "--compare-dest", "--copy-dest", "--link-dest",
		"--bwlimit", "--timeout", "--port", "--out-format", "--log-file", "-T", "--temp-dir",
		"--partial-dir", "--chmod", "--chown", "-M", "--remote-option", "--max-size", "--min-size", "--modify-window")
	boolFlags := set("-a", "--archive", "-v", "--verbose", "-r", "--recursive", "-z", "--compress",
		"-P", "--progress", "--partial", "-u", "--update", "-n", "--dry-run", "--delete", "--delete-after",
		"--delete-excluded", "-x", "--one-file-system", "-l", "-p", "-t", "-g", "-o", "-D", "-H", "-A", "-X",
		"-S", "-W", "--numeric-ids", "-q", "--quiet", "-h", "--human-readable", "-c", "--checksum",
		"--existing", "--ignore-existing", "-R", "--relative", "-L", "--copy-links", "-k", "-K")
	return extractRemoteCopy(args, valueFlags, boolFlags)
}

// extractRemoteCopy is the shared SRC... DEST extractor for scp/rsync.
func extractRemoteCopy(args []string, valueFlags, boolFlags map[string]struct{}) extraction {
	pos, _, _, recognized := scanFlags(args, valueFlags, boolFlags, nil)
	ext := extraction{applies: true, recognized: recognized}
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
func extractSftp(args []string) extraction {
	_ = args
	return extraction{applies: true, recognized: true, remoteEgress: true}
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

	// Privileged-metadata copy: cp -p/-a of a setuid or root-owned source.
	if spec.kind == KindCopyMove && op.role == risktypes.OperandRoleRead &&
		ext.preserveMeta && oz.Resolved != "" && r.sourceIsPrivileged(oz.Resolved) {
		raise(runnertypes.RiskLevelHigh, risktypes.ReasonPermissionGrant)
	}

	// Sensitive-source copy -> Medium read floor. Applies to both cp/mv and the
	// data-transfer copies (scp/rsync), so a sensitive source copied into a
	// safe-zone is Medium regardless of which copy command is used.
	if op.role == risktypes.OperandRoleRead &&
		(spec.kind == KindCopyMove || spec.kind == KindDataTransferWrite) {
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

// attachedValue returns the values of an optional-argument flag given in the
// attached --flag=value form (the only form GNU getopt accepts for optional args).
func attachedValue(args []string, flag string) []string {
	prefix := flag + "="
	var vals []string
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, prefix); ok {
			vals = append(vals, v)
		}
	}
	return vals
}
