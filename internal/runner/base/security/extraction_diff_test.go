//go:build test

package security

import (
	"reflect"
	"slices"
	"testing"
)

// TestExtractionDifferential is the primary behavior-preservation gate. For
// every zoned command it runs a broad generated corpus through both the frozen legacy
// extractor (legacyZoningSpecs) and the live production path (commandFlagSpecs'
// ToExtraction over parseArgs), and requires the whole extraction struct to match.
// Comparing the struct as a whole -- rather than listing fields -- means a missed field
// (e.g. applies) or a future field is covered automatically.
//
// In Phase 2 both sides were the same code, so this was tautologically green; that proved
// the harness is sound and that the frozen copy is a faithful transcription. In Phase 3
// each command's migration to the declarative parser is gated on this staying green
// (modulo the documented intended deviations in diffExclusions).
func TestExtractionDifferential(t *testing.T) {
	for cmd := range commandFlagSpecs {
		// Fail fast and cleanly if a command lacks a frozen oracle or a flag spec,
		// rather than panicking on a nil func below (test order is not guaranteed, so
		// TestLegacyZoningSpecsCoverage may not have reported the mismatch yet).
		legacyFn, ok := legacyZoningSpecs[cmd]
		if !ok {
			t.Errorf("no legacy extractor for %q (legacyZoningSpecs is missing it)", cmd)
			continue
		}
		spec, ok := commandFlagSpecs[cmd]
		if !ok {
			t.Errorf("no commandFlagSpec for %q", cmd)
			continue
		}
		for _, args := range diffCorpus(cmd, spec) {
			if excludedFromDiff(cmd, args) {
				continue
			}
			legacy := normalizeExtraction(legacyFn(args))
			prod := normalizeExtraction(spec.ToExtraction(parseArgs(spec.Flags, args), args))
			if !reflect.DeepEqual(legacy, prod) {
				t.Errorf("cmd=%q args=%v diverged\n  legacy=%+v\n  prod  =%+v", cmd, args, legacy, prod)
			}
		}
	}
}

// diffExclusions lists, per command, the inputs where the new implementation is allowed
// to diverge from the frozen legacy oracle ON PURPOSE. Keep each predicate as narrow as
// possible (match only the exact diverging argv shape, all tokens and length) so it
// never masks a real regression on the same flag.
var diffExclusions = map[string]func(args []string) bool{
	// 0144 deviation (long-form recursion, recognized=false->true): the legacy scanFlags
	// registered --recursive/--archive as recursion flags only, so the long form fell
	// through to the unknown-"--"-flag branch (recognized=false). The declarative spec
	// collapses every spelling into one FlagSpec, so the long form is now recognized=true.
	// cp keeps all former copyMoveFlags entries, so only the long-form deviation applies.
	"cp": func(args []string) bool {
		return isLongRecursionDeviation(args, "--recursive", "--archive")
	},

	// mv 0145 deviations: (a) over-recognition removal (recognized=true->false) for
	// flags absent from real mv; (b) additions of -Z/--context and
	// --strip-trailing-slashes (recognized=false->true; mv --help).
	// The 0144 long-form recursion predicate is retained but dead (--recursive/--archive
	// are not in mvFlags; kept for documentation continuity).
	"mv": func(args []string) bool {
		return isLongRecursionDeviation(args, "--recursive", "--archive") ||
			// removal: cluster of removed -r and -a (mv --help)
			exactArgvMatch(args, "-ra", "s", "d") ||
			// AC-02 representative: mv -s SRC DST (mv --help: no -s flag)
			exactArgvMatch(args, "-s", "s", "d") ||
			// removal: cluster with removed -r and kept -f (mv --help: no -r flag)
			exactArgvMatch(args, "-rf", "s", "d") ||
			// addition: -Z/--context and --strip-trailing-slashes (mv --help)
			exactArgvMatch(args, "-Z", "a", "b") ||
			exactArgvMatch(args, "--context", "a", "b") ||
			exactArgvMatch(args, "--strip-trailing-slashes", "a", "b")
	},

	// rm 0144 deviation (--recursive recognized=false->true) plus 0145 additions
	// (--preserve-root/--no-preserve-root recognized=false->true; rm --help).
	"rm": func(args []string) bool {
		return isLongRecursionDeviation(args, "--recursive") ||
			exactArgvMatch(args, "--preserve-root", "a", "b") ||
			exactArgvMatch(args, "--no-preserve-root", "a", "b")
	},

	// rmdir 0145 deviation (over-recognition removal, recognized=true->false): rmdir now
	// has only -p/-v/--ignore-fail-on-non-empty (rmdir --help). Inputs using the removed
	// rm-heritage flags yield recognized=false.
	// The 0144 --recursive predicate is retained but now dead (--recursive is not in
	// rmdirFlags, so the auto-corpus never generates {"--recursive","a","b"} for rmdir).
	"rmdir": func(args []string) bool {
		return isLongRecursionDeviation(args, "--recursive") ||
			// AC-02 representative: rmdir -r DIR (rmdir --help: no -r flag)
			exactArgvMatch(args, "-r", "d") ||
			// cluster with removed -r and kept -p (rmdir --help: no -r flag)
			exactArgvMatch(args, "-rp", "d")
	},

	// unlink 0145 deviation (over-recognition removal, recognized=true->false): unlink has
	// no flags (unlink --help). Any flag token now yields recognized=false.
	// The 0144 --recursive predicate is retained but dead (nil Flags → auto-corpus never
	// generates {"--recursive","a","b"} for unlink).
	"unlink": func(args []string) bool {
		return isLongRecursionDeviation(args, "--recursive") ||
			// AC-02 representative: unlink -r FILE (unlink --help: no options)
			exactArgvMatch(args, "-r", "f") ||
			// cluster of two removed flags (unlink --help: no options)
			exactArgvMatch(args, "-rf", "f")
	},

	// sponge 0145 deviation (over-recognition removal, recognized=true->false): sponge
	// has only -a/--append (moreutils sponge(1)). Other flags now yield recognized=false.
	"sponge": func(args []string) bool {
		// AC-02 representative: sponge -r FILE (sponge(1): no -r flag)
		return exactArgvMatch(args, "-r", "f") ||
			// cluster of two removed flags (sponge(1): only -a/--append exists)
			exactArgvMatch(args, "-rv", "f")
	},

	// mkdir 0145 deviations: (a) over-recognition removal for flags absent from real mkdir;
	// (b) addition of -Z/--context (recognized=false->true; mkdir --help).
	"mkdir": func(args []string) bool {
		return exactArgvMatch(args, "-a", "d") || // AC-02: no -a flag (mkdir --help)
			exactArgvMatch(args, "-af", "d") || // cluster of two removed flags
			// addition: -Z/--context (mkdir --help)
			exactArgvMatch(args, "-Z", "a", "b") ||
			exactArgvMatch(args, "--context", "a", "b")
	},

	// touch 0145 deviations: (a) over-recognition removal for -p (touch --help);
	// (b) additions of -m and --time (recognized=false->true; touch --help).
	"touch": func(args []string) bool {
		return exactArgvMatch(args, "-p", "f") || // AC-02: no -p flag (touch --help)
			// addition: -m boolean (touch --help)
			exactArgvMatch(args, "-m", "a", "b") ||
			// addition: --time value flag (touch --help)
			exactArgvMatch(args, "--time", "v", "a", "b") ||
			exactArgvMatch(args, "--time=v", "a", "b")
	},

	// ln 0145: added --backup/--logical/--physical long forms for -b/-L/-P (ln --help).
	"ln": func(args []string) bool {
		return exactArgvMatch(args, "--backup", "a", "b") ||
			exactArgvMatch(args, "--logical", "a", "b") ||
			exactArgvMatch(args, "--physical", "a", "b")
	},

	// chmod 0145: added -H/-L/-P and -h/--no-dereference/--dereference (chmod --help,
	// uutils 0.8.0 and GNU+uutils union policy).
	"chmod": func(args []string) bool {
		return exactArgvMatch(args, "-H", "a", "b") ||
			exactArgvMatch(args, "-L", "a", "b") ||
			exactArgvMatch(args, "-P", "a", "b") ||
			exactArgvMatch(args, "-h", "a", "b") ||
			exactArgvMatch(args, "--no-dereference", "a", "b") ||
			exactArgvMatch(args, "--dereference", "a", "b")
	},

	// chown/chgrp 0145: added --preserve-root/--no-preserve-root (chown/chgrp --help).
	"chown": func(args []string) bool {
		return exactArgvMatch(args, "--preserve-root", "a", "b") ||
			exactArgvMatch(args, "--no-preserve-root", "a", "b")
	},
	"chgrp": func(args []string) bool {
		return exactArgvMatch(args, "--preserve-root", "a", "b") ||
			exactArgvMatch(args, "--no-preserve-root", "a", "b")
	},

	// mknod 0145: -Z/--context changed from ArityRequired to ArityOptional (mknod --help);
	// the bare optional form {"-Z","a","b"} now leaves "a" as a non-flag arg instead of
	// consuming it as the required context value.
	"mknod": func(args []string) bool {
		return exactArgvMatch(args, "-Z", "a", "b") ||
			exactArgvMatch(args, "--context", "a", "b")
	},

	// shred 0145: added --random-source value flag (shred --help).
	"shred": func(args []string) bool {
		return exactArgvMatch(args, "--random-source", "v", "a", "b") ||
			exactArgvMatch(args, "--random-source=v", "a", "b")
	},

	// install 0145: (a) -b/--backup changed ArityRequired->ArityOptional — bare form no
	// longer consumes the next token as the backup suffix (install --help);
	// (b) new flags added: --strip-program, -P/--preserve-context, -U/--unprivileged,
	// -Z/--context (optional).
	"install": func(args []string) bool {
		// -b/--backup arity change: bare form, value is now a non-flag arg (install --help)
		return exactArgvMatch(args, "-b", "a", "b") ||
			exactArgvMatch(args, "--backup", "a", "b") ||
			exactArgvMatch(args, "-b", "ctl", "s", "d") ||
			// new --strip-program (install --help)
			exactArgvMatch(args, "--strip-program", "v", "a", "b") ||
			exactArgvMatch(args, "--strip-program=v", "a", "b") ||
			// new -P/--preserve-context (install --help)
			exactArgvMatch(args, "-P", "a", "b") ||
			exactArgvMatch(args, "--preserve-context", "a", "b") ||
			// new -U/--unprivileged (install --help)
			exactArgvMatch(args, "-U", "a", "b") ||
			exactArgvMatch(args, "--unprivileged", "a", "b") ||
			// new -Z/--context optional (install --help); includes cluster -ZZQ
			exactArgvMatch(args, "-Z", "a", "b") ||
			exactArgvMatch(args, "-Z=v", "a", "b") ||
			exactArgvMatch(args, "-Zv", "a", "b") ||
			exactArgvMatch(args, "--context", "a", "b") ||
			exactArgvMatch(args, "--context=v", "a", "b") ||
			exactArgvMatch(args, "-ZZQ", "a")
	},
}

// exactArgvMatch reports whether args is exactly the specified tokens (length and every
// element must match). Used by diffExclusions predicates to achieve full-argv precision:
// unlike position-only checks (args[0]=="-r"), this never matches a different argv shape
// where the same token appears at a different index or with different surrounding tokens.
func exactArgvMatch(args []string, tokens ...string) bool {
	if len(args) != len(tokens) {
		return false
	}
	for i, tok := range tokens {
		if args[i] != tok {
			return false
		}
	}
	return true
}

// isLongRecursionDeviation reports whether args is exactly one of the long-form recursion
// inputs the corpus auto-generates for a recursion flag (`<longFlag> a b`). It matches
// only that precise shape so the exclusion never masks a regression on the same flag in
// any other position or form (e.g. the short forms -r/-R/-a, or attached/= variants).
func isLongRecursionDeviation(args []string, longFlags ...string) bool {
	if len(args) != 3 || args[1] != "a" || args[2] != "b" {
		return false
	}
	return slices.Contains(longFlags, args[0])
}

// excludedFromDiff reports whether (cmd, args) is an intentional, documented divergence.
func excludedFromDiff(cmd string, args []string) bool {
	if pred, ok := diffExclusions[cmd]; ok {
		return pred(args)
	}
	return false
}

// TestLegacyZoningSpecsCoverage asserts the frozen oracle covers exactly the live
// command set, so a command added to zoningSpecs without a frozen counterpart fails
// here cleanly instead of panicking on a nil func inside TestExtractionDifferential.
func TestLegacyZoningSpecsCoverage(t *testing.T) {
	for cmd := range commandFlagSpecs {
		_, ok := legacyZoningSpecs[cmd]
		if !ok {
			t.Errorf("commandFlagSpecs has %q but legacyZoningSpecs does not", cmd)
		}
	}
	for cmd := range legacyZoningSpecs {
		_, ok := commandFlagSpecs[cmd]
		if !ok {
			t.Errorf("legacyZoningSpecs has %q but commandFlagSpecs does not", cmd)
		}
	}
}

// normalizeExtraction collapses an empty operands slice to nil so reflect.DeepEqual
// treats a nil and an empty (non-nil) slice as equal -- they are observably identical
// downstream, but DeepEqual distinguishes them. All other fields are compared exactly.
func normalizeExtraction(e extraction) extraction {
	if len(e.operands) == 0 {
		e.operands = nil
	}
	return e
}

// shortFlag reports whether name is a single-dash short flag (e.g. -t), eligible for
// the attached-value form -tVALUE.
func shortFlag(name string) bool {
	return len(name) >= 2 && name[0] == '-' && name[1] != '-'
}

// diffCorpus builds a broad set of argv inputs for one command: generic edge forms,
// every declared flag in each of its surface forms, and the per-command fixtures that
// exercise the special grammars (dd key=value, chattr/tar/sed/find, remote copies) and
// the documented regression cases.
func diffCorpus(cmd string, spec CommandFlagSpec) [][]string {
	var corpus [][]string
	add := func(args ...string) { corpus = append(corpus, args) }

	// Generic edge forms applied to every command.
	add()
	add("--")
	add("-")
	add("")
	add("a")
	add("a", "b")
	add("a", "b", "c")
	add("--", "-x", "a")
	add("--unknown-zzz", "a")
	add("-ZZQ", "a") // unknown short cluster

	for _, f := range spec.Flags {
		for _, name := range f.Names {
			switch f.Arity {
			case ArityNone:
				add(name, "a", "b")
			case ArityRequired:
				add(name, "v", "a", "b") // separated value
				add(name+"=v", "a", "b") // attached (= form)
				add(name)                // value missing at end of argv
				if shortFlag(name) {
					add(name+"v", "a", "b") // attached short value
				}
			case ArityOptional:
				add(name, "a", "b")      // bare (no value)
				add(name+"=v", "a", "b") // attached value
				if shortFlag(name) {
					add(name+"v", "a", "b") // attached short value
				}
			}
		}
	}

	corpus = append(corpus, diffFixtures[cmd]...)
	return corpus
}

// diffFixtures are hand-written argvs per command targeting the special grammars and
// the documented regression cases that the auto-generated single-flag forms do not cover.
var diffFixtures = map[string][][]string{
	"cp": {
		{"-r", "s", "d"},
		{"-rf", "s", "d"},
		{"-a", "s", "d"},
		{"-p", "s", "d"},
		{"-t", "/d", "a", "b"},
		{"--target-directory=/d", "a"},
		{"-t", "/d"},
		{"s", "d"},
		{"a"},
		// Multi-flag clusters exercise the cluster-blind hasAny preserveMeta path:
		// -ra has -a in a cluster (preserveMeta must be true via raw whole-token? No:
		// hasAny is whole-token, so "-ra" does NOT match "-a"; preserveMeta=false),
		// while -rap likewise does not match. These pin the cluster-blind behavior.
		{"-ra", "s", "d"},
		{"-rap", "s", "d"},
		{"-rp", "s", "d"},
	},
	"mv": {
		{"s", "d"},
		{"-t", "/d", "a", "b"},
		{"-f", "s", "d"},
		{"a"},
		// existing: cluster of removed -r and -a (mv --help has neither)
		{"-ra", "s", "d"},
		// AC-02 representative (mv --help: no -s/--symbolic-link flag)
		{"-s", "s", "d"},
		// cluster with removed -r and kept -f (mv --help: no -r flag)
		{"-rf", "s", "d"},
	},
	"rm": {{"-rf", "a"}, {"-r", "a", "b"}, {"a"}, {"-f"}, {"-rfv", "a"}},
	// rmdir 0145: -r and clusters with removed flags (rmdir --help: no -r/-f flags)
	"rmdir": {
		{"d"},
		{"-p", "d"},
		{"-v", "d"},
		// AC-02 representative (rmdir --help: no -r flag)
		{"-r", "d"},
		// cluster with removed -r and kept -p (rmdir --help: no -r flag)
		{"-rp", "d"},
	},
	// unlink 0145: unlink has no flags (unlink --help); any flag is unrecognized
	"unlink": {
		{"f"},
		// AC-02 representative (unlink --help: no options at all)
		{"-r", "f"},
		// cluster of two removed flags (unlink --help: no options at all)
		{"-rf", "f"},
	},
	"shred":    {{"-u", "f"}, {"-n", "3", "f"}, {"f"}},
	"ln":       {{"-s", "/t", "l"}, {"t", "l"}, {"t"}, {"-s", "a", "b", "dir"}, {"-t", "/d", "a"}, {"-s", "/t"}, {"-sf", "/t", "l"}, {"-sf", "a", "b", "dir"}},
	"truncate": {{"-s", "0", "f"}, {"-r", "ref", "f"}, {"f"}},
	"sed": {
		{"-i", "s/a/b/", "f"},
		{"-i.bak", "s/a/b/", "f"},
		{"-e", "s/a/b/", "f"},
		{"s/a/b/", "f"},
		{"-n", "s/a/b/", "f"},
		{"-ir", "f"},
		{"s/a/b/"},
		{"-i", "s/a/b/"},
	},
	"touch": {
		{"f"},
		{"-r", "ref", "f"},
		{"-t", "2401010000", "f"},
		{"-c", "f"},
		// AC-02 representative (touch --help: no -p/--parents flag)
		{"-p", "f"},
	},
	"mkdir": {
		{"d"},
		{"-m", "0777", "d"},
		{"-m", "u+s", "d"},
		{"-p", "a/b"},
		// AC-02 representative (mkdir --help: no -a flag)
		{"-a", "d"},
		// cluster of two removed flags (mkdir --help: no -a or -f flags)
		{"-af", "d"},
	},
	"tee": {{"f"}, {"-a", "f"}, {}},
	"sponge": {
		{"f"},
		{},
		// AC-02 representative (moreutils sponge(1): no -r flag)
		{"-r", "f"},
		// cluster of two removed flags (sponge(1): only -a/--append exists)
		{"-rv", "f"},
	},
	"install": {
		{"-m", "4755", "s", "d"},
		{"-o", "root", "s", "d"},
		{"-d", "dir"},
		{"-t", "/d", "s"},
		{"-b", "ctl", "s", "d"},
		{"s", "d"},
		// -dv clusters the directory-mode -d with -v: hasAny is whole-token, so it does
		// NOT match the clustered -d (directory mode off), exercising the cluster-blind path.
		{"-dv", "dir"},
		{"-vd", "dir"},
	},
	"tar": {
		{"xzf", "a.tar"},
		{"-xzf", "a.tar"},
		{"-C", "/d", "-xf", "a.tar"},
		{"--one-top-level=/d", "-xf", "a.tar"},
		{"--one-top-level", "-xf", "a.tar"},
		{"cf", "a.tar", "src"},
		{"-tf", "a.tar"},
		{"--extract", "-f", "a.tar"},
		{"-cf", "out.tar", "s"},
		// Mixed-spelling and hidden-value cases that must keep per-spelling precedence
		// and read --one-top-level from raw argv (extractTar drops to a single dir/
		// archive, so a wrong/dropped pick would change the write path).
		{"--directory=/d", "-C", "/e", "-xf", "a.tar"},
		{"-C", "/e", "--directory=/d", "-xf", "a.tar"},
		{"--file", "b", "-f", "a", "-c", "src"},
		{"f", "--one-top-level=/d", "-x"},
		{"--", "--one-top-level=/d", "-x"},
	},
	"unzip": {
		{"a.zip"},
		{"-d", "/d", "a.zip"},
		{"-l", "a.zip"},
		{"-o", "a.zip"},
		// -lo / -ld cluster the (undeclared) listing flag -l with another flag: hasAny is
		// whole-token so it does NOT see -l inside the cluster (listing off), while the
		// cluster scan hits the unknown -l and sets recognized=false. Pins cluster-blind.
		{"-lo", "a.zip"},
		{"-ld", "/d", "a.zip"},
	},
	"dd": {
		{"if=/etc/passwd", "of=/tmp/x"},
		{"of=/dev/sda"},
		{"if=/x"},
		{"bs=512"},
		{"garbage"},
		{"if=/a", "of=/b", "bs=1M"},
		{},
	},
	"mknod":  {{"n", "c", "1", "3"}, {"-m", "0666", "n", "b", "8", "0"}, {"n"}},
	"mount":  {{"/dev/sda1", "/mnt"}, {"-t", "ext4", "/dev/sda1", "/mnt"}, {"-a"}, {"/mnt"}},
	"umount": {{"/mnt"}, {"-a"}, {"-l", "/mnt"}, {"-R", "/mnt"}, {"-af"}, {"-lf", "/mnt"}},
	"chmod":  {{"u+s", "f"}, {"0777", "f"}, {"-R", "g+w", "f"}, {"u-s", "f"}, {"f"}, {"4755", "f"}},
	"chown":  {{"--from=alice", "bob", "f"}, {"--reference=ref", "f"}, {"root:root", "f"}, {"-R", "root", "f"}, {"f"}},
	"chgrp":  {{"staff", "f"}, {"--reference=ref", "f"}, {"-R", "staff", "f"}, {"f"}},
	"setfacl": {
		{"-m", "g:staff:rwx", "f"},
		{"-m", "default:g:s:w", "f"},
		{"-x", "u:bob", "f"},
		{"-b", "f"},
		{"-m", "u:bob:r", "f"},
		{"f"},
	},
	"chattr": {
		{"+i", "f"},
		{"-i", "f"},
		{"=j", "f"},
		{"-R", "+i", "f"},
		{"-v", "1", "+a", "f"},
		{"+i"},
		{"-V", "f"},
		{"+a", "f1", "f2"},
		// chattr is parsed whole-token (no cluster split): -VR and -Rf are unknown
		// tokens (recognized=false), NOT -V -R / -R -f. These pin that behavior.
		{"-VR", "+i", "f"},
		{"-Rf", "+i", "f"},
		{"--", "-x", "f"},
	},
	"find": {
		{"/p", "-delete"},
		{"-delete"},
		{".", "-name", "x", "-delete"},
		{"/p", "-fprint", "/o"},
		{"/p", "-fprint0", "/o"},
		{"/p", "-fprintf", "/o", "%p"},
		{"/p", "-exec", "rm", ";"},
		{"/p", "-print"},
		{"/p", "-fprint"},
	},
	"curl": {
		{"-o", "out", "http://x"},
		{"-O", "http://x"},
		{"-T", "up", "http://x"},
		{"http://x"},
		{"-fsSL", "http://x"},
		{"-o", "-", "http://x"},
		// -OL clusters -O (whole-token hasAny detects it? No: hasAny is whole-token, so
		// "-OL" does NOT match "-O"; -O write defaulting off) with -L; the cluster scan
		// recognizes both as bool. Pins the cluster-blind -O detection.
		{"-OL", "http://x"},
		{"-LO", "http://x"},
	},
	"wget": {
		{"-O", "out", "http://x"},
		{"-P", "/dir", "http://x"},
		{"http://x"},
		{"--post-file", "up", "http://x"},
		{"-O", "-", "http://x"},
	},
	"scp": {{"a", "host:b"}, {"host:a", "b"}, {"-r", "a", "b"}, {"-P", "22", "a", "host:b"}, {"a"}},
	"rsync": {
		{"-a", "src", "host:dst"},
		{"src", "dst"},
		{"--delete", "src", "dst"},
		{"-e", "ssh", "src", "dst"},
		{"src", "rsync://h/m"},
		{"src"},
	},
	"sftp": {{"host:"}, {"-b", "batch", "host"}, {}},
}
