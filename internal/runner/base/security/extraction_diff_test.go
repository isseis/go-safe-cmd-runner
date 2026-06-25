//go:build test

package security

import (
	"reflect"
	"testing"
)

// TestExtractionDifferential is the primary behavior-preservation gate (design §7). For
// every zoned command it runs a broad generated corpus through both the frozen legacy
// extractor (legacyZoningSpecs) and the live production path (zoningSpecs), and requires
// the whole extraction struct to match. Comparing the struct as a whole -- rather than
// listing fields -- means a missed field (e.g. applies) or a future field is covered
// automatically.
//
// Before any command is migrated (Phase 2) both sides are the same code, so this is
// tautologically green; that proves the harness is sound and, crucially, that the
// frozen copy is a faithful transcription (any transcription error shows up here as a
// mismatch). In Phase 3 each command's migration is gated on this staying green.
func TestExtractionDifferential(t *testing.T) {
	for cmd := range zoningSpecs {
		spec := commandFlagSpecs[cmd]
		for _, args := range diffCorpus(cmd, spec) {
			legacy := normalizeExtraction(legacyZoningSpecs[cmd](args))
			prod := normalizeExtraction(zoningSpecs[cmd].extract(args))
			if !reflect.DeepEqual(legacy, prod) {
				t.Errorf("cmd=%q args=%q diverged\n  legacy=%+v\n  prod  =%+v", cmd, args, legacy, prod)
			}
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
// the AC-08 regression cases.
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
// the AC-08 regression cases that the auto-generated single-flag forms do not cover.
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
	},
	"mv":       {{"s", "d"}, {"-t", "/d", "a", "b"}, {"-f", "s", "d"}, {"a"}},
	"rm":       {{"-rf", "a"}, {"-r", "a", "b"}, {"a"}, {"-f"}},
	"shred":    {{"-u", "f"}, {"-n", "3", "f"}, {"f"}},
	"ln":       {{"-s", "/t", "l"}, {"t", "l"}, {"t"}, {"-s", "a", "b", "dir"}, {"-t", "/d", "a"}, {"-s", "/t"}},
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
	},
	"unzip": {{"a.zip"}, {"-d", "/d", "a.zip"}, {"-l", "a.zip"}, {"-o", "a.zip"}},
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
	"umount": {{"/mnt"}, {"-a"}, {"-l", "/mnt"}, {"-R", "/mnt"}},
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
	},
	"find": {
		{"/p", "-delete"},
		{"-delete"},
		{".", "-name", "x", "-delete"},
		{"/p", "-fprint", "/o"},
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
