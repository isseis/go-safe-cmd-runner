//go:build test

package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeCoreutilsBinary creates an executable file in the given directory and returns its path.
func makeCoreutilsBinary(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho test"), 0o755))
	return path
}

func TestCoreutilsCommandRisk_SafeCommands(t *testing.T) {
	tmp := t.TempDir()
	SetCoreutilsDirForTest(t, tmp)

	for _, cmd := range []string{"mkdir", "ls", "cat", "echo"} {
		path := makeCoreutilsBinary(t, tmp, cmd)
		risk, handled, err := CoreutilsCommandRisk(path, nil)
		assert.NoError(t, err, cmd)
		assert.True(t, handled, cmd)
		assert.Equal(t, runnertypes.RiskLevelLow, risk, cmd)
	}
}

// TestCoreutils_UnknownSubcommandHigh verifies that a subcommand that is not in
// the explicit safe set (and is not destructive) fails safe to High, rather than
// passing at Medium. Only subcommands in safeCoreutilsCommands are Low.
func TestCoreutils_UnknownSubcommandHigh(t *testing.T) {
	tmp := t.TempDir()
	SetCoreutilsDirForTest(t, tmp)

	for _, cmd := range []string{"chmod", "chown", "env", "nohup", "cp", "mv", "definitely-not-a-coreutil"} {
		path := makeCoreutilsBinary(t, tmp, cmd)
		risk, handled, err := CoreutilsCommandRisk(path, nil)
		assert.NoError(t, err, cmd)
		assert.True(t, handled, cmd)
		assert.Equal(t, runnertypes.RiskLevelHigh, risk, cmd)
	}
}

func TestCoreutilsCommandRisk_DestructiveCommands(t *testing.T) {
	tmp := t.TempDir()
	SetCoreutilsDirForTest(t, tmp)

	for _, cmd := range []string{"rm", "dd", "shred", "truncate"} {
		path := makeCoreutilsBinary(t, tmp, cmd)
		risk, handled, err := CoreutilsCommandRisk(path, nil)
		assert.NoError(t, err, cmd)
		assert.True(t, handled, cmd)
		assert.Equal(t, runnertypes.RiskLevelHigh, risk, cmd)
	}
}

func TestCoreutilsCommandRisk_MulticallEntrypoint(t *testing.T) {
	tmp := t.TempDir()
	SetCoreutilsDirForTest(t, tmp)

	path := makeCoreutilsBinary(t, tmp, "coreutils")

	tests := []struct {
		name     string
		args     []string
		expected runnertypes.RiskLevel
	}{
		{
			name:     "rm subcommand",
			args:     []string{"rm", "-rf", "/tmp/x"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "mkdir subcommand",
			args:     []string{"mkdir", "d"},
			expected: runnertypes.RiskLevelLow,
		},
		{
			// No identifiable subcommand: fail safe to High; an
			// unparseable multicall could hide a destructive subcommand.
			name:     "options only no subcommand",
			args:     []string{"--help"},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name:     "empty args no subcommand",
			args:     []string{},
			expected: runnertypes.RiskLevelHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk, handled, err := CoreutilsCommandRisk(path, tt.args)
			require.NoError(t, err)
			assert.True(t, handled)
			assert.Equal(t, tt.expected, risk)
		})
	}
}

func TestCoreutilsCommandRisk_Setuid(t *testing.T) {
	tmp := t.TempDir()
	SetCoreutilsDirForTest(t, tmp)

	path := makeCoreutilsBinary(t, tmp, "mkdir")
	require.NoError(t, os.Chmod(path, 0o755|os.ModeSetuid))

	info, err := os.Stat(path)
	require.NoError(t, err)
	if info.Mode()&os.ModeSetuid == 0 {
		t.Skip("Skipping: OS silently ignored setuid bit (non-root on macOS)")
	}

	risk, handled, err := CoreutilsCommandRisk(path, nil)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, runnertypes.RiskLevelHigh, risk)
}

func TestCoreutilsCommandRisk_NonCoreutilsPath(t *testing.T) {
	tmp := t.TempDir()
	SetCoreutilsDirForTest(t, tmp)

	for _, path := range []string{"/usr/bin/mkdir", "/bin/ls"} {
		risk, handled, err := CoreutilsCommandRisk(path, nil)
		assert.NoError(t, err, path)
		assert.False(t, handled, path)
		assert.Equal(t, runnertypes.RiskLevelUnknown, risk, path)
	}
}

// TestDestructiveCoreutilsDerivedFromBase guards against drift between the two
// destructive-command sets: destructiveCoreutilsCommands must contain every entry
// of destructiveCommandNames (it extends it with coreutils-only "truncate").
func TestDestructiveCoreutilsDerivedFromBase(t *testing.T) {
	for name := range destructiveCommandNames {
		_, ok := destructiveCoreutilsCommands[name]
		assert.Truef(t, ok, "destructiveCoreutilsCommands must include base destructive command %q", name)
	}
	_, ok := destructiveCoreutilsCommands["truncate"]
	assert.True(t, ok, "destructiveCoreutilsCommands must include coreutils-only truncate")
}
