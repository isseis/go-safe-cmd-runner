package security

// FlagArity describes whether a flag takes an argument and, if it does, whether the
// argument is required or optional.
type FlagArity int

// Flag arities recognized by the parser.
const (
	ArityNone     FlagArity = iota // boolean flag: takes no argument
	ArityRequired                  // takes a required argument (attached form or the next token)
	ArityOptional                  // optional argument: attached form only (--flag=v / -ov), never the next token
)

// ValueRole classifies the value captured by an argument-taking flag. ValueUnset is
// the zero value and marks a flag that takes no value; it is valid only for ArityNone.
// Every argument-taking flag (ArityRequired or ArityOptional) must declare a concrete
// role.
type ValueRole int

// Value roles for argument-taking flags.
const (
	ValueUnset   ValueRole = iota // no value captured; valid only for ArityNone flags
	ValueNonPath                  // a non-path value that may be ignored (explicitly classified)
	ValueWrite                    // the value is a write-destination operand
	ValueRead                     // the value is a read-source operand
)

// FlagSpec is the declarative specification of one logical flag. Every spelling
// (short and long) is collected in Names; Names[0] is the canonical key under which
// the parser records the flag. Names must hold at least one element.
type FlagSpec struct {
	Names     []string
	Arity     FlagArity
	Recursive bool      // a recursion flag (e.g. -r / -R / -a)
	Value     ValueRole // role of the captured value when Arity is Required/Optional; ValueUnset when ArityNone
}

// CommandFlagSpec is one command's declarative flag set plus the thin function that
// maps a parsed result into an extraction. The raw argv is passed to ToExtraction
// only for the non-getopt value grammars (dd's if=/of= key=value, chattr's attribute
// tokens); getopt-conformant commands read flag values from the ParseResult alone.
type CommandFlagSpec struct {
	Kind         LocationKind
	Flags        []FlagSpec
	ToExtraction func(ParseResult, []string) extraction
}

// The builders below keep the declarative table compact. A boolean flag is just its
// names (Arity and Value are the zero values ArityNone/ValueUnset); the others set the
// distinguishing fields.

// boolFlag declares a no-argument flag.
func boolFlag(names ...string) FlagSpec { return FlagSpec{Names: names} }

// recursiveFlag declares a no-argument flag that requests recursion (-r/-R/-a).
func recursiveFlag(names ...string) FlagSpec { return FlagSpec{Names: names, Recursive: true} }

// valueFlag declares a flag whose argument is required (attached or the next token).
func valueFlag(role ValueRole, names ...string) FlagSpec {
	return FlagSpec{Names: names, Arity: ArityRequired, Value: role}
}

// optionalFlag declares a flag whose argument is optional (attached form only).
func optionalFlag(role ValueRole, names ...string) FlagSpec {
	return FlagSpec{Names: names, Arity: ArityOptional, Value: role}
}

// commandFlagSpecs is the single declarative registry for every zoned command: its Kind,
// declared flags, and the thin ToExtraction that maps a parsed result into an extraction.
// The dispatcher (classifyDestinationZone) looks a command up here and runs
// spec.ToExtraction(parseArgs(spec.Flags, args), args). Each value flag's role records how
// the extractor uses its captured value: ValueWrite/ValueRead when the value becomes an
// operand, ValueNonPath when the value is consumed only for a floor signal or ignored.
// Arity mirrors the real CLI behavior (a flag that consumes the next token is
// ArityRequired) so parsing matches the pre-refactor extractors exactly.
var commandFlagSpecs = map[string]CommandFlagSpec{
	// cp and mv share ToExtraction (extractCopyMove) but have distinct flag sets: mv
	// lacks cp's recursion, link-creation, and dereference flags (cp --help vs mv --help).
	"cp": {Kind: KindCopyMove, Flags: cpFlags(), ToExtraction: func(pr ParseResult, args []string) extraction {
		return extractCopyMove(pr, args, false)
	}},
	"mv": {Kind: KindCopyMove, Flags: mvFlags(), ToExtraction: func(pr ParseResult, args []string) extraction {
		return extractCopyMove(pr, args, true)
	}},

	// rm/rmdir/unlink share extractRemove (via extractAllWrite) but have distinct flag
	// sets: rmdir's options are -p/-v/--ignore-fail-on-non-empty only; unlink has none.
	"rm":     {Kind: KindRemove, Flags: rmFlags(), ToExtraction: extractRemove},
	"rmdir":  {Kind: KindRemove, Flags: rmdirFlags(), ToExtraction: extractRemove},
	"unlink": {Kind: KindRemove, Flags: nil, ToExtraction: extractRemove},

	"shred": {Kind: KindRemove, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-n", "--iterations"),
		valueFlag(ValueNonPath, "-s", "--size"),
		valueFlag(ValueNonPath, "--random-source"),
		// -u/--remove[=HOW]: optional argument (shred --help); HOW is non-path.
		boolFlag("-f", "--force"), optionalFlag(ValueNonPath, "-u", "--remove"), boolFlag("-v", "--verbose"),
		boolFlag("-z", "--zero"), boolFlag("-x", "--exact"),
	}, ToExtraction: extractAllWrite},

	"ln": {Kind: KindLink, Flags: []FlagSpec{
		valueFlag(ValueWrite, "-t", "--target-directory"),
		valueFlag(ValueNonPath, "-S", "--suffix"),
		boolFlag("-s", "--symbolic"), boolFlag("-f", "--force"), boolFlag("-n", "--no-dereference"),
		boolFlag("-r", "--relative"), boolFlag("-v", "--verbose"), boolFlag("-i", "--interactive"),
		boolFlag("-T", "--no-target-directory"),
		boolFlag("-b"), optionalFlag(ValueNonPath, "--backup"), boolFlag("-L", "--logical"), boolFlag("-P", "--physical"),
	}, ToExtraction: extractLink},

	"truncate": {Kind: KindInPlaceEdit, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-s", "--size"),
		valueFlag(ValueNonPath, "-r", "--reference"),
		boolFlag("-c", "--no-create"), boolFlag("-o", "--io-blocks"),
	}, ToExtraction: extractAllWrite},

	"sed": {Kind: KindInPlaceEdit, Flags: []FlagSpec{
		optionalFlag(ValueNonPath, "-i", "--in-place"),
		valueFlag(ValueNonPath, "-e", "--expression"),
		valueFlag(ValueNonPath, "-f", "--file"),
		valueFlag(ValueNonPath, "-l", "--line-length"),
		boolFlag("-n", "--quiet", "--silent"), boolFlag("-r", "-E", "--regexp-extended"),
		boolFlag("-s", "--separate"), boolFlag("-z", "--null-data"), boolFlag("-u", "--unbuffered"),
		boolFlag("--posix"), boolFlag("--debug"), boolFlag("--sandbox"), boolFlag("--follow-symlinks"),
	}, ToExtraction: extractSed},

	// touch: -r takes a reference file (value flag, shadows any shared boolean -r).
	// Real touch has no --append long form for -a; -p/-v/-i are not real touch flags.
	"touch": {Kind: KindWriteFile, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-r", "--reference"),
		valueFlag(ValueNonPath, "-d", "--date"),
		valueFlag(ValueNonPath, "-t"),
		valueFlag(ValueNonPath, "--time"),
		boolFlag("-a"), // no --append long form in real touch (touch --help)
		boolFlag("-c", "--no-create"), boolFlag("-h", "--no-dereference"),
		boolFlag("-f"), boolFlag("-m"),
	}, ToExtraction: extractSimpleWrite},

	// mkdir: -a/-c/-h/-f/-i/-r are not real mkdir flags (mkdir --help); only -m/-p/-v/-Z.
	"mkdir": {Kind: KindWriteFile, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-m", "--mode"),
		boolFlag("-p", "--parents"), boolFlag("-v", "--verbose"),
		boolFlag("-Z"), optionalFlag(ValueNonPath, "--context"),
	}, ToExtraction: extractSimpleWrite},
	// sponge: only -a/--append is a real sponge flag (moreutils sponge(1)).
	"sponge": {Kind: KindWriteFile, Flags: []FlagSpec{
		boolFlag("-a", "--append"),
	}, ToExtraction: extractSimpleWrite},

	"tee": {Kind: KindWriteFile, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "--output-error"),
		boolFlag("-a", "--append"), boolFlag("-i", "--ignore-interrupts"), boolFlag("-p"),
	}, ToExtraction: extractTee},

	"install": {Kind: KindWriteFile, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-m", "--mode"),
		valueFlag(ValueNonPath, "-o", "--owner"),
		valueFlag(ValueNonPath, "-g", "--group"),
		valueFlag(ValueWrite, "-t", "--target-directory"),
		valueFlag(ValueNonPath, "-S", "--suffix"),
		// -b is boolean; --backup[=CONTROL] takes an optional non-path control word (install --help).
		boolFlag("-b"), optionalFlag(ValueNonPath, "--backup"),
		valueFlag(ValueNonPath, "--strip-program"),
		boolFlag("-d", "--directory"), boolFlag("-D"), boolFlag("-v", "--verbose"),
		boolFlag("-p", "--preserve-timestamps"), boolFlag("-c"), boolFlag("-C", "--compare"),
		boolFlag("-s", "--strip"), boolFlag("-T", "--no-target-directory"),
		boolFlag("-P", "--preserve-context"), boolFlag("-U", "--unprivileged"),
		// -Z and --context[=CTX] are aliases with an optional SELinux context value (install --help).
		boolFlag("-Z"), optionalFlag(ValueNonPath, "--context"),
	}, ToExtraction: extractInstall},

	"tar": {Kind: KindArchiveExtract, Flags: tarFlagSet, ToExtraction: extractTar},

	"unzip": {Kind: KindArchiveExtract, Flags: []FlagSpec{
		valueFlag(ValueWrite, "-d"),
		valueFlag(ValueNonPath, "-x"),
		// -l/-Z (listing mode) are deliberately NOT declared. Legacy leaves them out of
		// its flag sets and detects listing with a cluster-blind whole-token hasAny:
		// `unzip -l` early-returns applies=false,recognized=true, while `unzip -lo` is
		// recognized=false (the cluster hits the unknown -l). Declaring -l/-Z would make
		// the parser recognize -lo (recognized=true), diverging. Phase 3's ToExtraction
		// reproduces the legacy behavior with the same whole-token hasAny.
		boolFlag("-o"), boolFlag("-n"), boolFlag("-q"), boolFlag("-qq"), boolFlag("-v"),
		boolFlag("-j"), boolFlag("-a"), boolFlag("-u"), boolFlag("-f"),
	}, ToExtraction: extractUnzip},

	// dd has no getopt flags: its if=/of= key=value grammar is parsed in ToExtraction
	// (the passed ParseResult is ignored).
	"dd": {Kind: KindDeviceIO, Flags: nil, ToExtraction: extractDD},

	// mknod: -v/--verbose is not a real mknod flag (mknod --help).
	// -Z/--context[=CTX] takes an optional SELinux context value (mknod --help).
	"mknod": {Kind: KindWriteFile, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-m", "--mode"),
		boolFlag("-Z"), optionalFlag(ValueNonPath, "--context"),
	}, ToExtraction: extractMknod},

	"mount": {Kind: KindMount, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-t", "--types"),
		valueFlag(ValueNonPath, "-o", "--options"),
		valueFlag(ValueNonPath, "-O"),
		boolFlag("-a", "--all"), boolFlag("-r", "--read-only"), boolFlag("-w", "--rw"),
		boolFlag("-v", "--verbose"), boolFlag("-n"), boolFlag("--bind"), boolFlag("--rbind"),
		boolFlag("--move"), boolFlag("-B"), boolFlag("-R"), boolFlag("-M"), boolFlag("-f", "--fake"),
	}, ToExtraction: extractAllWrite},

	"umount": {Kind: KindMount, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-t", "--types"),
		valueFlag(ValueNonPath, "-O"),
		boolFlag("-a", "--all"), boolFlag("-r", "--read-only"), boolFlag("-v", "--verbose"),
		boolFlag("-n"), boolFlag("-l", "--lazy"), boolFlag("-f", "--force"),
		boolFlag("-R", "--recursive"), boolFlag("-d"),
	}, ToExtraction: extractUmount},

	// chmod: -H/-L/-P and -h/--no-dereference/--dereference added per chmod --help (uutils
	// 0.8.0); --dereference is uutils-only but included per the GNU+uutils union policy.
	"chmod": {Kind: KindPermission, Flags: []FlagSpec{
		recursiveFlag("-R", "--recursive"),
		boolFlag("-v", "--verbose"), boolFlag("-c", "--changes"), boolFlag("-f", "--silent", "--quiet"),
		boolFlag("-H"), boolFlag("-L"), boolFlag("-P"),
		boolFlag("-h", "--no-dereference"), boolFlag("--dereference"),
	}, ToExtraction: extractChmod},

	"chown": {Kind: KindPermission, Flags: ownerFlags(), ToExtraction: extractOwner},
	"chgrp": {Kind: KindPermission, Flags: ownerFlags(), ToExtraction: extractOwner},

	"setfacl": {Kind: KindPermission, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-m", "--modify"),
		valueFlag(ValueNonPath, "-x", "--remove"),
		valueFlag(ValueNonPath, "-M", "--modify-file"),
		valueFlag(ValueNonPath, "-X", "--restore"),
		valueFlag(ValueNonPath, "-n"),
		recursiveFlag("-R", "--recursive"),
		boolFlag("-b", "--remove-all"), boolFlag("-k", "--remove-default"), boolFlag("-d", "--default"),
		boolFlag("-v", "--version"), boolFlag("-t"), boolFlag("-p", "--restore-stdin"),
	}, ToExtraction: extractSetfacl},

	// chattr: attribute mode tokens (+i/-a/=j) are split out before parseArgs; the
	// remaining options are declared here.
	"chattr": {Kind: KindPermission, Flags: chattrFlagSet, ToExtraction: extractChattr},

	// find parses roots and predicates positionally, not as getopt flags (the passed
	// ParseResult is ignored).
	"find": {Kind: KindFindDestructive, Flags: nil, ToExtraction: extractFind},

	"curl":  {Kind: KindDataTransferWrite, Flags: curlFlags(), ToExtraction: extractCurl},
	"wget":  {Kind: KindDataTransferWrite, Flags: wgetFlags(), ToExtraction: extractWget},
	"scp":   {Kind: KindDataTransferWrite, Flags: scpFlags(), ToExtraction: extractRemoteCopy},
	"rsync": {Kind: KindDataTransferWrite, Flags: rsyncFlags(), ToExtraction: extractRemoteCopy},

	// sftp's writes live in an interactive session / -b batch file, not argv.
	"sftp": {Kind: KindDataTransferWrite, Flags: nil, ToExtraction: extractSftp},
}

// cpFlags is cp's declarative flag set. cp retains all former copyMoveFlags entries
// because every one exists in the real cp CLI (cp --help).
func cpFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueWrite, "-t", "--target-directory"),
		valueFlag(ValueNonPath, "-S", "--suffix"),
		recursiveFlag("-r", "-R", "--recursive"),
		recursiveFlag("-a", "--archive"),
		boolFlag("-f", "--force"), boolFlag("-i", "--interactive"), boolFlag("-n", "--no-clobber"),
		boolFlag("-v", "--verbose"), boolFlag("-u", "--update"), boolFlag("-d"),
		boolFlag("-L", "--dereference"), boolFlag("-P", "--no-dereference"), boolFlag("-H"),
		boolFlag("-s", "--symbolic-link"), boolFlag("-l", "--link"), boolFlag("-T", "--no-target-directory"),
		boolFlag("-b"), optionalFlag(ValueNonPath, "--backup"), boolFlag("-x", "--one-file-system"),
	}
}

// mvFlags is mv's declarative flag set. mv does not have cp's recursion (-r/-R/-a),
// link-creation (-s/-l), dereference (-d/-L/-P/-H), or one-file-system (-x) flags
// (mv --help). -Z/--context and --strip-trailing-slashes are real mv flags.
func mvFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueWrite, "-t", "--target-directory"),
		valueFlag(ValueNonPath, "-S", "--suffix"),
		boolFlag("-f", "--force"), boolFlag("-i", "--interactive"), boolFlag("-n", "--no-clobber"),
		boolFlag("-v", "--verbose"), boolFlag("-u", "--update"),
		boolFlag("-T", "--no-target-directory"), boolFlag("-b"), optionalFlag(ValueNonPath, "--backup"),
		boolFlag("-Z"), optionalFlag(ValueNonPath, "--context"), boolFlag("--strip-trailing-slashes"),
	}
}

// rmFlags is rm's declarative flag set. -p/--parents and --ignore-fail-on-non-empty
// are rmdir flags, not rm flags (rm --help). --preserve-root/--no-preserve-root added.
func rmFlags() []FlagSpec {
	return []FlagSpec{
		recursiveFlag("-r", "-R", "--recursive"),
		boolFlag("-f", "--force"), boolFlag("-i"), boolFlag("-I"), boolFlag("--interactive"),
		boolFlag("-v", "--verbose"), boolFlag("-d", "--dir"), boolFlag("--one-file-system"),
		boolFlag("--preserve-root"), boolFlag("--no-preserve-root"),
	}
}

// rmdirFlags is rmdir's declarative flag set. Only -p/--parents, -v/--verbose, and
// --ignore-fail-on-non-empty are real rmdir flags (rmdir --help); rm's recursion and
// force flags are absent.
func rmdirFlags() []FlagSpec {
	return []FlagSpec{
		boolFlag("-p", "--parents"), boolFlag("-v", "--verbose"),
		boolFlag("--ignore-fail-on-non-empty"),
	}
}

func ownerFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueNonPath, "--from"),
		valueFlag(ValueNonPath, "--reference"),
		recursiveFlag("-R", "--recursive"),
		boolFlag("-v", "--verbose"), boolFlag("-c", "--changes"), boolFlag("-f", "--silent", "--quiet"),
		boolFlag("-h", "--no-dereference"), boolFlag("-H"), boolFlag("-L"), boolFlag("-P"),
		boolFlag("--dereference"),
		boolFlag("--preserve-root"), boolFlag("--no-preserve-root"),
	}
}

// tarFlags is the declared flag set for tar. It is used both in commandFlagSpecs["tar"]
// and inside extractTar, which re-parses the normalized argv with the same flags.
// tarFlagSet is tar's declarative flag set. It is built once (immutable) because
// extractTar re-parses with it on every call (after normalization); rebuilding the
// slice per call would be wasted work.
//
// -f/--file and -C/--directory are deliberately declared as SEPARATE entries rather
// than grouped aliases. extractTar picks a single archive/dir via firstNonEmpty with
// per-spelling precedence (captured["-f"] before captured["--file"], etc.). Grouping
// them under one canonical key would merge both spellings in argv order, so a value
// given via the lower-precedence spelling could win (or a dropped spelling could lose
// a write path), diverging from the pre-refactor extractor.
var tarFlagSet = []FlagSpec{
	valueFlag(ValueWrite, "-f"),
	valueFlag(ValueWrite, "--file"),
	valueFlag(ValueWrite, "-C"),
	valueFlag(ValueWrite, "--directory"),
	optionalFlag(ValueWrite, "--one-top-level"),
	boolFlag("-v", "--verbose"), boolFlag("-z", "--gzip"), boolFlag("-j", "--bzip2"),
	boolFlag("-J", "--xz"), boolFlag("-p", "--preserve-permissions"), boolFlag("-k", "--keep-old-files"),
	boolFlag("--no-same-owner"), boolFlag("-m", "--touch"),
	boolFlag("-x"), boolFlag("-t"), boolFlag("-c"),
	boolFlag("--extract"), boolFlag("--get"), boolFlag("--list"), boolFlag("--create"),
}

// chattrFlagSet is the declared flag set for chattr (the regular options only). The
// attribute mode tokens (+i/-a/=j) are split out before parseArgs in extractChattr, so
// they are not declared here. Built once (immutable); extractChattr derives its
// whole-token name sets from it via chattrValueNames/chattrBoolNames.
var chattrFlagSet = []FlagSpec{
	valueFlag(ValueNonPath, "-v"),
	valueFlag(ValueNonPath, "-p"),
	boolFlag("-R"), boolFlag("-f"), boolFlag("-V"), boolFlag("-H"), boolFlag("-L"), boolFlag("-P"),
}

// chattrValueNames/chattrBoolNames are the whole-token lookup sets extractChattr uses,
// computed once from chattrFlagSet (the flag knowledge still lives in the table).
var chattrValueNames, chattrBoolNames = func() (valueNames, boolNames map[string]struct{}) {
	valueNames = make(map[string]struct{})
	boolNames = make(map[string]struct{})
	for _, f := range chattrFlagSet {
		for _, n := range f.Names {
			if f.Arity == ArityNone {
				boolNames[n] = struct{}{}
			} else {
				valueNames[n] = struct{}{}
			}
		}
	}
	return valueNames, boolNames
}()

func curlFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueWrite, "-o", "--output"),
		valueFlag(ValueRead, "-T", "--upload-file"),
		valueFlag(ValueNonPath, "-H", "--header"),
		valueFlag(ValueNonPath, "-d", "--data"),
		valueFlag(ValueNonPath, "--data-raw"),
		valueFlag(ValueNonPath, "--data-binary"),
		valueFlag(ValueNonPath, "-u", "--user"),
		valueFlag(ValueNonPath, "-A", "--user-agent"),
		valueFlag(ValueNonPath, "-e", "--referer"),
		valueFlag(ValueNonPath, "-x", "--proxy"),
		valueFlag(ValueNonPath, "-b", "--cookie"),
		valueFlag(ValueNonPath, "-c", "--cookie-jar"),
		valueFlag(ValueNonPath, "-K", "--config"),
		valueFlag(ValueNonPath, "-w", "--write-out"),
		valueFlag(ValueNonPath, "-m", "--max-time"),
		valueFlag(ValueNonPath, "--connect-timeout"),
		valueFlag(ValueNonPath, "-X", "--request"),
		valueFlag(ValueNonPath, "--url"),
		valueFlag(ValueNonPath, "--retry"),
		valueFlag(ValueNonPath, "--limit-rate"),
		valueFlag(ValueNonPath, "-C", "--continue-at"),
		valueFlag(ValueNonPath, "-r", "--range"),
		valueFlag(ValueNonPath, "--cacert"),
		valueFlag(ValueNonPath, "--cert"),
		valueFlag(ValueNonPath, "--key"),
		boolFlag("-O", "--remote-name"), boolFlag("-L", "--location"), boolFlag("-s", "--silent"),
		boolFlag("-S", "--show-error"), boolFlag("-f", "--fail"), boolFlag("-k", "--insecure"),
		boolFlag("-v", "--verbose"), boolFlag("-i", "--include"), boolFlag("-I", "--head"),
		boolFlag("-g", "--progress-bar"), boolFlag("-J", "--remote-header-name"), boolFlag("-#"),
		boolFlag("-q"), boolFlag("-z"),
	}
}

func wgetFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueWrite, "-O", "--output-document"),
		valueFlag(ValueWrite, "-P", "--directory-prefix"),
		valueFlag(ValueNonPath, "-o", "--output-file"),
		valueFlag(ValueNonPath, "-a", "--append-output"),
		valueFlag(ValueNonPath, "--header"),
		valueFlag(ValueNonPath, "--user"),
		valueFlag(ValueNonPath, "--password"),
		valueFlag(ValueNonPath, "--limit-rate"),
		valueFlag(ValueNonPath, "-t", "--tries"),
		valueFlag(ValueNonPath, "-T", "--timeout"),
		valueFlag(ValueNonPath, "-U", "--user-agent"),
		valueFlag(ValueNonPath, "--referer"),
		valueFlag(ValueNonPath, "--post-data"),
		valueFlag(ValueRead, "--post-file"),
		valueFlag(ValueNonPath, "-e", "--execute"),
		valueFlag(ValueNonPath, "--ca-certificate"),
		valueFlag(ValueNonPath, "--certificate"),
		boolFlag("-q", "--quiet"), boolFlag("-v", "--verbose"), boolFlag("-c", "--continue"),
		boolFlag("-N", "--timestamping"), boolFlag("-r", "--recursive"), boolFlag("-np", "--no-parent"),
		boolFlag("-nc", "--no-clobber"), boolFlag("-nv", "--no-verbose"), boolFlag("--no-check-certificate"),
		boolFlag("-d", "--debug"), boolFlag("-b", "--background"),
	}
}

func scpFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueNonPath, "-P"), valueFlag(ValueNonPath, "-i"), valueFlag(ValueNonPath, "-o"),
		valueFlag(ValueNonPath, "-c"), valueFlag(ValueNonPath, "-F"), valueFlag(ValueNonPath, "-l"),
		valueFlag(ValueNonPath, "-S"), valueFlag(ValueNonPath, "-J"),
		boolFlag("-r"), boolFlag("-p"), boolFlag("-q"), boolFlag("-v"), boolFlag("-C"), boolFlag("-B"),
		boolFlag("-3"), boolFlag("-4"), boolFlag("-6"), boolFlag("-A"), boolFlag("-O"), boolFlag("-R"),
		boolFlag("-T"),
	}
}

func rsyncFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueNonPath, "-e", "--rsh"),
		valueFlag(ValueNonPath, "--rsync-path"),
		valueFlag(ValueNonPath, "--exclude"),
		valueFlag(ValueNonPath, "--include"),
		valueFlag(ValueNonPath, "--exclude-from"),
		valueFlag(ValueNonPath, "--include-from"),
		valueFlag(ValueNonPath, "-f", "--filter"),
		valueFlag(ValueNonPath, "--files-from"),
		valueFlag(ValueNonPath, "--compare-dest"),
		valueFlag(ValueNonPath, "--copy-dest"),
		valueFlag(ValueNonPath, "--link-dest"),
		valueFlag(ValueNonPath, "--bwlimit"),
		valueFlag(ValueNonPath, "--timeout"),
		valueFlag(ValueNonPath, "--port"),
		valueFlag(ValueNonPath, "--out-format"),
		valueFlag(ValueNonPath, "--log-file"),
		valueFlag(ValueNonPath, "-T", "--temp-dir"),
		valueFlag(ValueNonPath, "--partial-dir"),
		valueFlag(ValueNonPath, "--chmod"),
		valueFlag(ValueNonPath, "--chown"),
		valueFlag(ValueNonPath, "-M", "--remote-option"),
		valueFlag(ValueNonPath, "--max-size"),
		valueFlag(ValueNonPath, "--min-size"),
		valueFlag(ValueNonPath, "--modify-window"),
		boolFlag("-a", "--archive"), boolFlag("-v", "--verbose"), boolFlag("-r", "--recursive"),
		boolFlag("-z", "--compress"), boolFlag("-P", "--progress"), boolFlag("--partial"),
		boolFlag("-u", "--update"), boolFlag("-n", "--dry-run"), boolFlag("--delete"),
		boolFlag("--delete-after"), boolFlag("--delete-excluded"), boolFlag("-x", "--one-file-system"),
		boolFlag("-l"), boolFlag("-p"), boolFlag("-t"), boolFlag("-g"), boolFlag("-o"), boolFlag("-D"),
		boolFlag("-H"), boolFlag("-A"), boolFlag("-X"), boolFlag("-S"), boolFlag("-W"),
		boolFlag("--numeric-ids"), boolFlag("-q", "--quiet"), boolFlag("-h", "--human-readable"),
		boolFlag("-c", "--checksum"), boolFlag("--existing"), boolFlag("--ignore-existing"),
		boolFlag("-R", "--relative"), boolFlag("-L", "--copy-links"), boolFlag("-k"), boolFlag("-K"),
	}
}
