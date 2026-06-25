//go:build test

package security

import (
	"reflect"
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
// to diverge from the frozen legacy oracle ON PURPOSE. Phase 3 populates it for the one
// documented deviation -- long-form recursion flags (cp/mv --recursive and --archive,
// rm/rmdir/unlink --recursive) flip recognized=false->true; see the decision history in
// 02_architecture.md. Keep each predicate as narrow as possible (match only the exact
// diverging argv shape) so it never masks a real regression on the same flag.
var diffExclusions = map[string]func(args []string) bool{
	// Documented intended deviation (decision 2026-06-25, see 02_architecture.md decision
	// history / 03_implementation_plan.md Phase 2): the legacy scanFlags registered the
	// long-form recursion spellings (--recursive / --archive) only as recursion flags, not
	// as boolean flags, so a long form fell through to the unknown-"--"-flag branch and set
	// recognized=false (fail-closed). The short forms -r/-R/-a went through the cluster
	// path and stayed recognized=true. The declarative spec collapses every spelling into
	// one FlagSpec, so the long form is now recognized=true (a precision improvement that
	// removes a spurious High). These predicates excuse ONLY that exact recognized flip on
	// the long-form-recursion inputs the corpus generates (`<flag> a b`); the short forms
	// and every other input are still compared.
	"cp": func(args []string) bool { return isLongRecursionDeviation(args, "--recursive", "--archive") },
	"mv": func(args []string) bool { return isLongRecursionDeviation(args, "--recursive", "--archive") },
	// rm/rmdir/unlink share removeFlags(), which declares --recursive as a recursion flag;
	// legacy extractRemove failed closed on it for all three commands.
	"rm":     func(args []string) bool { return isLongRecursionDeviation(args, "--recursive") },
	"rmdir":  func(args []string) bool { return isLongRecursionDeviation(args, "--recursive") },
	"unlink": func(args []string) bool { return isLongRecursionDeviation(args, "--recursive") },
}

// isLongRecursionDeviation reports whether args is exactly one of the long-form recursion
// inputs the corpus auto-generates for a recursion flag (`<longFlag> a b`). It matches
// only that precise shape so the exclusion never masks a regression on the same flag in
// any other position or form (e.g. the short forms -r/-R/-a, or attached/= variants).
func isLongRecursionDeviation(args []string, longFlags ...string) bool {
	if len(args) != 3 || args[1] != "a" || args[2] != "b" {
		return false
	}
	for _, f := range longFlags {
		if args[0] == f {
			return true
		}
	}
	return false
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
	"mv":       {{"s", "d"}, {"-t", "/d", "a", "b"}, {"-f", "s", "d"}, {"a"}, {"-ra", "s", "d"}},
	"rm":       {{"-rf", "a"}, {"-r", "a", "b"}, {"a"}, {"-f"}, {"-rfv", "a"}},
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
	"touch":  {{"f"}, {"-r", "ref", "f"}, {"-t", "2401010000", "f"}, {"-c", "f"}},
	"mkdir":  {{"d"}, {"-m", "0777", "d"}, {"-m", "u+s", "d"}, {"-p", "a/b"}},
	"tee":    {{"f"}, {"-a", "f"}, {}},
	"sponge": {{"f"}, {}},
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
