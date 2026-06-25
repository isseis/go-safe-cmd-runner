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

// commandFlagSpecs is the declarative flag table for every zoned command. Phase 2
// introduces the data and the meta-tests that validate it; the per-command
// ToExtraction that consumes it is wired command-by-command in Phase 3, so
// ToExtraction is nil here. Keys mirror zoningSpecs. Each value flag's role records
// how the legacy extractor uses its captured value: ValueWrite/ValueRead when the
// value becomes an operand, ValueNonPath when the value is consumed only for a floor
// signal or ignored. Arity mirrors the legacy behavior (a flag that consumes the next
// token is ArityRequired) so the migration preserves parsing exactly.
var commandFlagSpecs = map[string]CommandFlagSpec{
	// cp/mv share the same flag grammar (extractCopyMove); only ToExtraction differs.
	"cp": {Kind: KindCopyMove, Flags: copyMoveFlags()},
	"mv": {Kind: KindCopyMove, Flags: copyMoveFlags()},

	"rm":     {Kind: KindRemove, Flags: removeFlags()},
	"rmdir":  {Kind: KindRemove, Flags: removeFlags()},
	"unlink": {Kind: KindRemove, Flags: removeFlags()},

	"shred": {Kind: KindRemove, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-n", "--iterations"),
		valueFlag(ValueNonPath, "-s", "--size"),
		boolFlag("-f", "--force"), boolFlag("-u", "--remove"), boolFlag("-v", "--verbose"),
		boolFlag("-z", "--zero"), boolFlag("-x", "--exact"),
	}},

	"ln": {Kind: KindLink, Flags: []FlagSpec{
		valueFlag(ValueWrite, "-t", "--target-directory"),
		valueFlag(ValueNonPath, "-S", "--suffix"),
		boolFlag("-s", "--symbolic"), boolFlag("-f", "--force"), boolFlag("-n", "--no-dereference"),
		boolFlag("-r", "--relative"), boolFlag("-v", "--verbose"), boolFlag("-i", "--interactive"),
		boolFlag("-T", "--no-target-directory"), boolFlag("-b"), boolFlag("-L"), boolFlag("-P"),
	}},

	"truncate": {Kind: KindInPlaceEdit, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-s", "--size"),
		valueFlag(ValueNonPath, "-r", "--reference"),
		boolFlag("-c", "--no-create"), boolFlag("-o", "--io-blocks"),
	}},

	"sed": {Kind: KindInPlaceEdit, Flags: []FlagSpec{
		optionalFlag(ValueNonPath, "-i", "--in-place"),
		valueFlag(ValueNonPath, "-e", "--expression"),
		valueFlag(ValueNonPath, "-f", "--file"),
		valueFlag(ValueNonPath, "-l", "--line-length"),
		boolFlag("-n", "--quiet", "--silent"), boolFlag("-r", "-E", "--regexp-extended"),
		boolFlag("-s", "--separate"), boolFlag("-z", "--null-data"), boolFlag("-u", "--unbuffered"),
		boolFlag("--posix"), boolFlag("--debug"), boolFlag("--sandbox"), boolFlag("--follow-symlinks"),
	}},

	// touch: -r takes a reference file (valueFlags shadows the shared -r boolean).
	"touch": {Kind: KindWriteFile, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-r", "--reference"),
		valueFlag(ValueNonPath, "-d", "--date"),
		valueFlag(ValueNonPath, "-t"),
		boolFlag("-a", "--append"), boolFlag("-c", "--no-create"), boolFlag("-h", "--no-dereference"),
		boolFlag("-p", "--parents"), boolFlag("-v", "--verbose"), boolFlag("-f"), boolFlag("-i"),
	}},

	"mkdir":  {Kind: KindWriteFile, Flags: simpleWriteFlags(valueFlag(ValueNonPath, "-m", "--mode"))},
	"sponge": {Kind: KindWriteFile, Flags: simpleWriteFlags()},

	"tee": {Kind: KindWriteFile, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "--output-error"),
		boolFlag("-a", "--append"), boolFlag("-i", "--ignore-interrupts"), boolFlag("-p"),
	}},

	"install": {Kind: KindWriteFile, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-m", "--mode"),
		valueFlag(ValueNonPath, "-o", "--owner"),
		valueFlag(ValueNonPath, "-g", "--group"),
		valueFlag(ValueWrite, "-t", "--target-directory"),
		valueFlag(ValueNonPath, "-S", "--suffix"),
		valueFlag(ValueNonPath, "-b", "--backup"),
		boolFlag("-d", "--directory"), boolFlag("-D"), boolFlag("-v", "--verbose"),
		boolFlag("-p", "--preserve-timestamps"), boolFlag("-c"), boolFlag("-C", "--compare"),
		boolFlag("-s", "--strip"), boolFlag("-T", "--no-target-directory"),
	}},

	"tar": {Kind: KindArchiveExtract, Flags: []FlagSpec{
		valueFlag(ValueWrite, "-f", "--file"),
		valueFlag(ValueWrite, "-C", "--directory"),
		optionalFlag(ValueWrite, "--one-top-level"),
		boolFlag("-v", "--verbose"), boolFlag("-z", "--gzip"), boolFlag("-j", "--bzip2"),
		boolFlag("-J", "--xz"), boolFlag("-p", "--preserve-permissions"), boolFlag("-k", "--keep-old-files"),
		boolFlag("--no-same-owner"), boolFlag("-m", "--touch"),
		boolFlag("-x"), boolFlag("-t"), boolFlag("-c"),
		boolFlag("--extract"), boolFlag("--get"), boolFlag("--list"), boolFlag("--create"),
	}},

	"unzip": {Kind: KindArchiveExtract, Flags: []FlagSpec{
		valueFlag(ValueWrite, "-d"),
		valueFlag(ValueNonPath, "-x"),
		// -l/-Z select listing (non-writing) mode. Legacy detects them via hasAny
		// (outside its flag sets) and returns applies=false,recognized=true. Declaring
		// them as booleans is faithful -- the legacy listing path already yields
		// recognized=true -- and lets Phase 3's ToExtraction detect listing via HasFlag.
		boolFlag("-l"), boolFlag("-Z"),
		boolFlag("-o"), boolFlag("-n"), boolFlag("-q"), boolFlag("-qq"), boolFlag("-v"),
		boolFlag("-j"), boolFlag("-a"), boolFlag("-u"), boolFlag("-f"),
	}},

	// dd has no getopt flags: its if=/of= key=value grammar is parsed in ToExtraction.
	"dd": {Kind: KindDeviceIO, Flags: nil},

	"mknod": {Kind: KindWriteFile, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-m", "--mode"),
		valueFlag(ValueNonPath, "-Z", "--context"),
		boolFlag("-v", "--verbose"),
	}},

	"mount": {Kind: KindMount, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-t", "--types"),
		valueFlag(ValueNonPath, "-o", "--options"),
		valueFlag(ValueNonPath, "-O"),
		boolFlag("-a", "--all"), boolFlag("-r", "--read-only"), boolFlag("-w", "--rw"),
		boolFlag("-v", "--verbose"), boolFlag("-n"), boolFlag("--bind"), boolFlag("--rbind"),
		boolFlag("--move"), boolFlag("-B"), boolFlag("-R"), boolFlag("-M"), boolFlag("-f", "--fake"),
	}},

	"umount": {Kind: KindMount, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-t", "--types"),
		valueFlag(ValueNonPath, "-O"),
		boolFlag("-a", "--all"), boolFlag("-r", "--read-only"), boolFlag("-v", "--verbose"),
		boolFlag("-n"), boolFlag("-l", "--lazy"), boolFlag("-f", "--force"),
		boolFlag("-R", "--recursive"), boolFlag("-d"),
	}},

	"chmod": {Kind: KindPermission, Flags: []FlagSpec{
		recursiveFlag("-R", "--recursive"),
		boolFlag("-v", "--verbose"), boolFlag("-c", "--changes"), boolFlag("-f", "--silent", "--quiet"),
	}},

	"chown": {Kind: KindPermission, Flags: ownerFlags()},
	"chgrp": {Kind: KindPermission, Flags: ownerFlags()},

	"setfacl": {Kind: KindPermission, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-m", "--modify"),
		valueFlag(ValueNonPath, "-x", "--remove"),
		valueFlag(ValueNonPath, "-M", "--modify-file"),
		valueFlag(ValueNonPath, "-X", "--restore"),
		valueFlag(ValueNonPath, "-n"),
		recursiveFlag("-R", "--recursive"),
		boolFlag("-b", "--remove-all"), boolFlag("-k", "--remove-default"), boolFlag("-d", "--default"),
		boolFlag("-v", "--version"), boolFlag("-t"), boolFlag("-p", "--restore-stdin"),
	}},

	// chattr: attribute mode tokens (+i/-a/=j) are split out before parseArgs; the
	// remaining options are declared here.
	"chattr": {Kind: KindPermission, Flags: []FlagSpec{
		valueFlag(ValueNonPath, "-v"),
		valueFlag(ValueNonPath, "-p"),
		boolFlag("-R"), boolFlag("-f"), boolFlag("-V"), boolFlag("-H"), boolFlag("-L"), boolFlag("-P"),
	}},

	// find parses roots and predicates positionally, not as getopt flags.
	"find": {Kind: KindFindDestructive, Flags: nil},

	"curl":  {Kind: KindDataTransferWrite, Flags: curlFlags()},
	"wget":  {Kind: KindDataTransferWrite, Flags: wgetFlags()},
	"scp":   {Kind: KindDataTransferWrite, Flags: scpFlags()},
	"rsync": {Kind: KindDataTransferWrite, Flags: rsyncFlags()},

	// sftp's writes live in an interactive session / -b batch file, not argv.
	"sftp": {Kind: KindDataTransferWrite, Flags: nil},
}

func copyMoveFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueWrite, "-t", "--target-directory"),
		valueFlag(ValueNonPath, "-S", "--suffix"),
		recursiveFlag("-r", "-R", "--recursive"),
		recursiveFlag("-a", "--archive"),
		boolFlag("-f", "--force"), boolFlag("-i", "--interactive"), boolFlag("-n", "--no-clobber"),
		boolFlag("-v", "--verbose"), boolFlag("-u", "--update"), boolFlag("-d"),
		boolFlag("-L", "--dereference"), boolFlag("-P", "--no-dereference"), boolFlag("-H"),
		boolFlag("-s", "--symbolic-link"), boolFlag("-l", "--link"), boolFlag("-T", "--no-target-directory"),
		boolFlag("-b", "--backup"), boolFlag("-x", "--one-file-system"),
	}
}

func removeFlags() []FlagSpec {
	return []FlagSpec{
		recursiveFlag("-r", "-R", "--recursive"),
		boolFlag("-f", "--force"), boolFlag("-i"), boolFlag("-I"), boolFlag("--interactive"),
		boolFlag("-v", "--verbose"), boolFlag("-d", "--dir"), boolFlag("--one-file-system"),
		boolFlag("-p", "--parents"), boolFlag("--ignore-fail-on-non-empty"),
	}
}

// simpleWriteFlags returns the shared boolean set used by mkdir and sponge, plus any
// command-specific value flags supplied by the caller. touch does NOT use this helper:
// it is declared explicitly because its -r is value-taking (a reference file), which
// shadows the shared boolean -r.
func simpleWriteFlags(extra ...FlagSpec) []FlagSpec {
	flags := append([]FlagSpec{}, extra...)
	return append(flags,
		boolFlag("-a", "--append"), boolFlag("-c", "--no-create"), boolFlag("-h", "--no-dereference"),
		boolFlag("-p", "--parents"), boolFlag("-v", "--verbose"), boolFlag("-f"), boolFlag("-i"),
		boolFlag("-r"),
	)
}

func ownerFlags() []FlagSpec {
	return []FlagSpec{
		valueFlag(ValueNonPath, "--from"),
		valueFlag(ValueNonPath, "--reference"),
		recursiveFlag("-R", "--recursive"),
		boolFlag("-v", "--verbose"), boolFlag("-c", "--changes"), boolFlag("-f", "--silent", "--quiet"),
		boolFlag("-h", "--no-dereference"), boolFlag("-H"), boolFlag("-L"), boolFlag("-P"),
		boolFlag("--dereference"),
	}
}

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
		valueFlag(ValueNonPath, "--user-agent"),
		valueFlag(ValueNonPath, "-U"),
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
