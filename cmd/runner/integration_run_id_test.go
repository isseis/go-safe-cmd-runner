//go:build test

package main

import (
	"path/filepath"
	"testing"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_ValidRunIDIsAdopted verifies the accepting side of the --run-id
// boundary check: a value in the accepted format survives validation and is the
// one the run actually uses.
//
// The adopted run ID is observed through the generated log file name
// ({hostname}_{timestamp}_{runID}.json) rather than through RUN_SUMMARY, because
// RUN_SUMMARY is only emitted on the error paths.
//
// This test lives here rather than in integration_logger_test.go because it needs
// the go:build test helpers (newGoRunCmdWithHashDir, recordHash) that that
// untagged file cannot reference.
func TestE2E_ValidRunIDIsAdopted(t *testing.T) {
	const runID = "backup-20260805-143000"

	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	configFile := setupTempConfig(t, configContent)
	logDir := tu.SafeTempDir(t)

	// A dry run only exits 0 when the config and the command are both verified,
	// so record their hashes first.
	hashDir := tu.SafeTempDir(t)
	recordHash(t, hashDir, configFile)
	recordHash(t, hashDir, "/bin/echo")

	cmd := newGoRunCmdWithHashDir(t, hashDir, "-config", configFile, "-dry-run",
		"-dry-run-detail", "summary", "-log-dir", logDir, "-run-id", runID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command output:\n%s", string(output))
	}
	require.NoError(t, err, "runner should accept a run ID in the accepted format")
	assert.Equal(t, 0, cmd.ProcessState.ExitCode())

	matches, err := filepath.Glob(filepath.Join(logDir, "*_"+runID+".json"))
	require.NoError(t, err)
	assert.Len(t, matches, 1, "exactly one log file named after the supplied run ID should exist")
}
