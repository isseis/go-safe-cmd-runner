package security

import (
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runExtraction runs a command's production extraction (the declarative path) the same
// way the dispatcher does: parse with the command's flags, then map to an extraction.
func runExtraction(t *testing.T, cmd string, args ...string) extraction {
	t.Helper()
	s, ok := commandFlagSpecs[cmd]
	require.True(t, ok, "no spec for command %q", cmd)
	return s.ToExtraction(parseArgs(s.Flags, args), args)
}

// TestExtractionRegressionCases pins the representative cases that 0142's reviews and
// past rules surfaced. These assert explicit expected extractions, an oracle
// independent of the legacy-vs-new differential test (which would not catch a defect
// shared by both the frozen copy and the new code). Cases already covered by
// destination_zoning_test.go are not repeated here.
func TestExtractionRegressionCases(t *testing.T) {
	write := risktypes.OperandRoleWrite
	read := risktypes.OperandRoleRead

	t.Run("flag alias yields identical result", func(t *testing.T) {
		short := runExtraction(t, "cp", "-t", "/dst", "a")
		long := runExtraction(t, "cp", "--target-directory", "/dst", "a")
		assert.Equal(t, short, long, "-t and --target-directory must extract identically")
		assert.Equal(t, []rawOperand{{raw: "/dst", role: write}, {raw: "a", role: read}}, short.operands)
		assert.True(t, short.recognized)
	})

	t.Run("optional-arg flag is not misparsed", func(t *testing.T) {
		// --one-top-level takes an optional attached value only; the separate -xf must
		// not be consumed, and extraction stays recognized.
		bare := runExtraction(t, "tar", "--one-top-level", "-xf", "a.tar")
		assert.True(t, bare.applies)
		assert.True(t, bare.recognized)
		assert.Equal(t, []rawOperand{{raw: ".", role: write}}, bare.operands)

		attached := runExtraction(t, "tar", "--one-top-level=/top", "-xf", "a.tar")
		assert.Equal(t, []rawOperand{{raw: "/top", role: write}}, attached.operands)
	})

	t.Run("sed -e script not confused with file", func(t *testing.T) {
		withFlag := runExtraction(t, "sed", "-i", "-e", "s/a/b/", "f")
		assert.True(t, withFlag.applies)
		assert.Equal(t, []rawOperand{{raw: "f", role: write}}, withFlag.operands)

		// Without -e the first positional is the inline script, the rest are files.
		inline := runExtraction(t, "sed", "-i", "s/a/b/", "f")
		assert.Equal(t, []rawOperand{{raw: "f", role: write}}, inline.operands)
	})

	t.Run("chmod symbolic setuid detection per clause", func(t *testing.T) {
		assert.True(t, runExtraction(t, "chmod", "u+s", "f").grantsPermission)
		assert.True(t, runExtraction(t, "chmod", "u+xs", "f").grantsPermission, "combined u+xs grants setuid")
		assert.False(t, runExtraction(t, "chmod", "u-s", "f").grantsPermission, "removal is not a grant")
	})

	t.Run("setfacl default ACL group write", func(t *testing.T) {
		assert.True(t, runExtraction(t, "setfacl", "-m", "default:g:staff:rwx", "f").grantsPermission)
		assert.True(t, runExtraction(t, "setfacl", "-m", "d:g:s:w", "f").grantsPermission, "d: prefix shifts the who field")
		assert.False(t, runExtraction(t, "setfacl", "-m", "u:bob:r", "f").grantsPermission, "user read is not a group/other write")
	})

	t.Run("chown --from keeps spec positional, --reference drops it", func(t *testing.T) {
		from := runExtraction(t, "chown", "--from=alice", "bob", "f")
		assert.True(t, from.recognized)
		assert.Equal(t, []rawOperand{{raw: "f", role: write}}, from.operands, "--from is a filter; bob is the owner spec, f is the target")

		ref := runExtraction(t, "chown", "--reference=src", "f")
		assert.True(t, ref.recognized)
		assert.Equal(t, []rawOperand{{raw: "f", role: write}}, ref.operands, "--reference removes the owner spec positional")
	})

	t.Run("ln symbolic vs hard link target base", func(t *testing.T) {
		sym := runExtraction(t, "ln", "-s", "/t", "l")
		assert.Equal(t, []rawOperand{{raw: "/t", role: read, base: "."}, {raw: "l", role: write}}, sym.operands,
			"a symlink's relative target resolves against the link's parent")

		hard := runExtraction(t, "ln", "t", "l")
		assert.Equal(t, []rawOperand{{raw: "t", role: read, base: ""}, {raw: "l", role: write}}, hard.operands,
			"a hard link's target resolves against the working directory (no base override)")
	})

	t.Run("over-recognition removal: removed flags yield recognized=false", func(t *testing.T) {
		// Representative inputs for over-recognition removal (0145): flags present
		// in the legacy shared-helper set but absent from the real CLI (man pages).
		// Each was recognized=true before the fix; after cleanup it must be recognized=false.
		cases := []struct{ cmd, flag, operand string }{
			{"sponge", "-r", "file"},
			{"mkdir", "-a", "dir"},
			{"touch", "-p", "file"},
			{"unlink", "-r", "file"},
			{"rmdir", "-r", "dir"},
			{"mv", "-s", "src"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.cmd+"/"+tc.flag, func(t *testing.T) {
				e := runExtraction(t, tc.cmd, tc.flag, tc.operand)
				assert.False(t, e.recognized,
					"cmd=%q flag=%q should be recognized=false after over-recognition removal", tc.cmd, tc.flag)
			})
		}
	})

	t.Run("tar mode parsed from the first word only", func(t *testing.T) {
		extract := runExtraction(t, "tar", "xzf", "a.tar")
		assert.True(t, extract.applies)
		assert.Equal(t, []rawOperand{{raw: ".", role: write}}, extract.operands)

		list := runExtraction(t, "tar", "tf", "a.tar")
		assert.False(t, list.applies, "listing does not write")

		create := runExtraction(t, "tar", "cf", "out.tar", "src")
		assert.Equal(t, []rawOperand{{raw: "out.tar", role: write}}, create.operands)
	})
}

// TestRemovedOverRecognizedFlagsFailClosed verifies that every flag removed by the
// 0145 over-recognition cleanup yields recognized=false on the production path. The
// source-of-truth set removedOverRecognizedFlags is ranged directly so that a forgotten
// entry fails here rather than silently going untested.
func TestRemovedOverRecognizedFlagsFailClosed(t *testing.T) {
	// removedOverRecognizedFlags maps each de-shared command to the flags that existed in
	// the legacy shared-helper set but are absent from the real CLI (per Appendix A of
	// 03_implementation_plan.md). Each flag is tested as a standalone argv token.
	removedOverRecognizedFlags := map[string][]string{
		// mv --help: recursion/link/dereference flags absent from real mv
		"mv": {
			"-r", "-R", "--recursive", "-a", "--archive", "-d",
			"-L", "--dereference", "-P", "--no-dereference", "-H",
			"-s", "--symbolic-link", "-l", "--link", "-x", "--one-file-system",
		},
		// rm --help: rmdir-specific flags absent from real rm
		"rm": {"-p", "--parents", "--ignore-fail-on-non-empty"},
		// rmdir --help: only -p/-v/--ignore-fail-on-non-empty are real rmdir flags
		"rmdir": {
			"-r", "-R", "--recursive", "-f", "--force",
			"-i", "-I", "--interactive", "-d", "--dir", "--one-file-system",
		},
		// unlink --help: no options whatsoever
		"unlink": {
			"-r", "-R", "--recursive", "-f", "--force",
			"-i", "-I", "--interactive", "-d", "--dir", "--one-file-system",
			"-p", "--parents", "--ignore-fail-on-non-empty",
		},
		// mkdir --help: only -m/-p/-v/-Z are real mkdir flags
		"mkdir": {
			"-a", "--append", "-c", "--no-create", "-h", "--no-dereference",
			"-f", "-i", "-r",
		},
		// sponge(1): only -a/--append is a real sponge flag
		"sponge": {
			"-c", "--no-create", "-h", "--no-dereference",
			"-p", "--parents", "-v", "--verbose", "-f", "-i", "-r",
		},
		// touch --help: -p/-v/-i are not real touch flags
		"touch": {"-p", "--parents", "-v", "--verbose", "-i", "--append"},
		// mknod --help: -v/--verbose is not a real mknod flag
		"mknod": {"-v", "--verbose"},
	}

	for cmd, flags := range removedOverRecognizedFlags {
		for _, flag := range flags {
			cmd, flag := cmd, flag
			t.Run(cmd+"/"+flag, func(t *testing.T) {
				e := runExtraction(t, cmd, flag, "target")
				assert.False(t, e.recognized,
					"cmd=%q flag=%q: expected recognized=false (over-recognition removed)", cmd, flag)
			})
		}
	}
}

// TestLocationResultParity pins the LocationResult for a representative invocation of
// EVERY registered command. The case table is ranged against commandFlagSpecs (not a
// hardcoded count), so a newly registered command without a parity case fails here.
// Each representative writes under the Trusted work dir, so every operand resolves into
// the Trusted safe-zone (Level Low, no reason codes) unless a network egress raises it;
// the per-case `roles` pins the operand count and ordering (Index/Role), and the loop
// pins the resolved zone fields (Zone/Trusted/MatchedCritical) and ReasonCodes.
// Resolved is the temp work-dir path (asserted non-empty, not by exact
// value). Operation floors that raise the level are pinned by TestLocationResultFloors.
func TestLocationResultParity(t *testing.T) {
	low := runnertypes.RiskLevelLow
	medium := runnertypes.RiskLevelMedium
	w := risktypes.OperandRoleWrite
	r := risktypes.OperandRoleRead
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent()) // foreign ident over a TrustedDirectory => safe-zone Low
	p := func(name string) string { return filepath.Join(wd, name) }

	cases := map[string]struct {
		args    []string
		level   runnertypes.RiskLevel
		roles   []risktypes.OperandRole // expected operand order/roles
		reasons []risktypes.ReasonCode  // expected ReasonCodes (nil => none)
	}{
		"cp":       {[]string{p("s"), p("d")}, low, []risktypes.OperandRole{w, r}, nil},
		"mv":       {[]string{p("s"), p("d")}, low, []risktypes.OperandRole{w, w}, nil},
		"rm":       {[]string{p("f")}, low, []risktypes.OperandRole{w}, nil},
		"rmdir":    {[]string{p("d")}, low, []risktypes.OperandRole{w}, nil},
		"unlink":   {[]string{p("f")}, low, []risktypes.OperandRole{w}, nil},
		"shred":    {[]string{p("f")}, low, []risktypes.OperandRole{w}, nil},
		"ln":       {[]string{"-s", p("t"), p("l")}, low, []risktypes.OperandRole{r, w}, nil},
		"truncate": {[]string{"-s", "0", p("f")}, low, []risktypes.OperandRole{w}, nil},
		"sed":      {[]string{"-i", "s/a/b/", p("f")}, low, []risktypes.OperandRole{w}, nil},
		"touch":    {[]string{p("f")}, low, []risktypes.OperandRole{w}, nil},
		"mkdir":    {[]string{p("d")}, low, []risktypes.OperandRole{w}, nil},
		"tee":      {[]string{p("f")}, low, []risktypes.OperandRole{w}, nil},
		"sponge":   {[]string{p("f")}, low, []risktypes.OperandRole{w}, nil},
		"install":  {[]string{p("s"), p("d")}, low, []risktypes.OperandRole{w, r}, nil},
		"tar":      {[]string{"-xf", p("a.tar"), "-C", wd}, low, []risktypes.OperandRole{w}, nil},
		"unzip":    {[]string{"-d", wd, p("a.zip")}, low, []risktypes.OperandRole{w}, nil},
		"dd":       {[]string{"of=" + p("x")}, low, []risktypes.OperandRole{w}, nil},
		"mknod":    {[]string{p("n"), "c", "1", "3"}, low, []risktypes.OperandRole{w}, nil},
		"mount":    {[]string{p("dev"), p("mnt")}, low, []risktypes.OperandRole{w, w}, nil},
		"umount":   {[]string{p("mnt")}, low, []risktypes.OperandRole{w}, nil},
		"chmod":    {[]string{"644", p("f")}, low, []risktypes.OperandRole{w}, nil},
		"chown":    {[]string{"root", p("f")}, low, []risktypes.OperandRole{w}, nil},
		"chgrp":    {[]string{"staff", p("f")}, low, []risktypes.OperandRole{w}, nil},
		"setfacl":  {[]string{"-m", "u:bob:r", p("f")}, low, []risktypes.OperandRole{w}, nil},
		"chattr":   {[]string{"+a", p("f")}, low, []risktypes.OperandRole{w}, nil},
		"find":     {[]string{wd, "-delete"}, low, []risktypes.OperandRole{w}, nil},
		"curl":     {[]string{"-o", p("out"), "http://example/x"}, low, []risktypes.OperandRole{w}, nil},
		"wget":     {[]string{"-O", p("out"), "http://example/x"}, low, []risktypes.OperandRole{w}, nil},
		"scp":      {[]string{p("s"), p("d")}, low, []risktypes.OperandRole{w, r}, nil},
		// sftp is always a network egress: no local operand, network-argument Medium floor.
		"sftp":  {[]string{p("d")}, medium, nil, []risktypes.ReasonCode{risktypes.ReasonNetworkArgument}},
		"rsync": {[]string{p("s"), p("d")}, low, []risktypes.OperandRole{w, r}, nil},
	}

	for cmd := range commandFlagSpecs {
		tc, ok := cases[cmd]
		require.Truef(t, ok, "no LocationResult parity case for command %q (add one)", cmd)
		t.Run(cmd, func(t *testing.T) {
			got := classify(in, cmd, tc.args...)
			assert.True(t, got.Applies, "Applies")
			assert.True(t, got.Recognized, "Recognized; operands=%+v reasons=%v", got.Operands, got.ReasonCodes)
			assert.Equal(t, tc.level, got.Level, "Level; operands=%+v reasons=%v", got.Operands, got.ReasonCodes)
			assert.ElementsMatch(t, tc.reasons, got.ReasonCodes, "ReasonCodes")

			require.Len(t, got.Operands, len(tc.roles), "operand count")
			for i, oz := range got.Operands {
				assert.Equal(t, i, oz.Index, "operand %d Index", i)
				assert.Equal(t, tc.roles[i], oz.Role, "operand %d Role", i)
				// Every representative resolves into the Trusted safe-zone.
				assert.Equal(t, risktypes.ZoneSafeZone, oz.Zone, "operand %d Zone", i)
				assert.True(t, oz.Trusted, "operand %d Trusted", i)
				assert.Empty(t, oz.MatchedCritical, "operand %d MatchedCritical", i)
				assert.NotEmpty(t, oz.Resolved, "operand %d Resolved", i)
			}
		})
	}
}

// TestLocationResultFloors pins the non-Low operation floors: permission grant,
// non-writing forms (Applies=false), unconditional umount -a, and remote network egress.
func TestLocationResultFloors(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())
	p := func(name string) string { return filepath.Join(wd, name) }

	t.Run("chmod setuid grants permission -> High", func(t *testing.T) {
		assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chmod", "u+s", p("f")).Level)
	})
	t.Run("tar listing does not apply", func(t *testing.T) {
		assert.False(t, classify(in, "tar", "-tf", p("a.tar")).Applies)
	})
	t.Run("sed without -i does not apply", func(t *testing.T) {
		assert.False(t, classify(in, "sed", "s/a/b/", p("f")).Applies)
	})
	t.Run("umount -a is unconditional High", func(t *testing.T) {
		assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "umount", "-a").Level)
	})
	t.Run("scp to a remote destination is a network egress", func(t *testing.T) {
		got := classify(in, "scp", p("s"), "host:/remote/d")
		assert.Equal(t, runnertypes.RiskLevelMedium, got.Level)
	})
	t.Run("rsync to a remote destination is a network egress", func(t *testing.T) {
		got := classify(in, "rsync", "-a", p("s"), "host:/remote/d")
		assert.Equal(t, runnertypes.RiskLevelMedium, got.Level)
	})
}

// TestFailClosed pins the fail-closed contract: an unknown flag, a value flag
// missing its value, a missing required spec/target positional, and an unresolvable
// operand all yield Recognized=false (and the unresolvable write also folds to High).
func TestFailClosed(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())
	p := func(name string) string { return filepath.Join(wd, name) }

	t.Run("unknown flag", func(t *testing.T) {
		assert.False(t, classify(in, "cp", "--bogus-zzz", p("s"), p("d")).Recognized)
	})
	t.Run("value flag missing its value", func(t *testing.T) {
		assert.False(t, classify(in, "tar", "-x", "-C").Recognized)
	})
	t.Run("missing required spec and target", func(t *testing.T) {
		// chmod needs a mode plus at least one target file.
		assert.False(t, classify(in, "chmod", "644").Recognized)
	})
	t.Run("unresolvable write operand -> High and not recognized", func(t *testing.T) {
		res := classifyDestinationZone(in, cmdNameSet("rm"), "rm", []string{"/x"}, erroringResolver())
		assert.False(t, res.Recognized)
		assert.Equal(t, runnertypes.RiskLevelHigh, res.Level)
	})
}
