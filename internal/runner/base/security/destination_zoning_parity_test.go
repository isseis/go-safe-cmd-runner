package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
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
// past rules surfaced (AC-08). These assert explicit expected extractions, an oracle
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
