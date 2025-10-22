//go:build unix

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_CommandLevelWorkdir tests command-level workdir override
// This test uses 'pwd' command which is Unix-specific
func TestIntegration_CommandLevelWorkdir(t *testing.T) {
	// Create separate fixed workdirs for different commands
	fixedWorkdir1, err := os.MkdirTemp("", "test-cmd-workdir1-*")
	require.NoError(t, err)
	defer os.RemoveAll(fixedWorkdir1)

	fixedWorkdir2, err := os.MkdirTemp("", "test-cmd-workdir2-*")
	require.NoError(t, err)
	defer os.RemoveAll(fixedWorkdir2)

	// Escape paths for TOML (Windows compatibility)
	escapedPath1 := strings.ReplaceAll(fixedWorkdir1, `\`, `\\`)
	escapedPath2 := strings.ReplaceAll(fixedWorkdir2, `\`, `\\`)

	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "cmd_with_custom_workdir1"
cmd = "pwd"
args = []
workdir = "` + escapedPath1 + `"
max_risk_level = "medium"

[[groups.commands]]
name = "cmd_with_custom_workdir2"
cmd = "pwd"
args = []
workdir = "` + escapedPath2 + `"
max_risk_level = "medium"

[[groups.commands]]
name = "cmd_with_group_default"
cmd = "pwd"
args = []
max_risk_level = "medium"
`

	// 1. Create Runner with output capture enabled
	r, outputBuf := createRunnerWithOutputCapture(t, configContent, false)

	// 2. Execute all groups
	executeRunnerWithTimeout(t, r, 30*time.Second)

	// 3. Parse output and extract workdir paths from pwd output using common helper
	output := outputBuf.String()
	// Pattern to match absolute paths (starting with /)
	pathPattern := regexp.MustCompile(`^(/[^\s]+)$`)
	workdirPaths := extractPathsFromOutput(t, output, pathPattern)

	// 4. Verify we found exactly 3 workdir paths (one per command)
	require.Len(t, workdirPaths, 3, "Expected 3 workdir paths from 3 commands, got: %v", workdirPaths)

	// 5. Verify first command uses fixedWorkdir1
	assert.Equal(t, fixedWorkdir1, workdirPaths[0],
		"Expected cmd1 to use fixedWorkdir1: %s, got: %s", fixedWorkdir1, workdirPaths[0])

	// 6. Verify second command uses fixedWorkdir2
	assert.Equal(t, fixedWorkdir2, workdirPaths[1],
		"Expected cmd2 to use fixedWorkdir2: %s, got: %s", fixedWorkdir2, workdirPaths[1])

	// 7. Verify third command uses group default (temp dir)
	assert.True(t, isTempDirPattern(workdirPaths[2]),
		"Expected cmd3 to use temp dir (group default), but got: %s", workdirPaths[2])

	// 8. Verify temp dir is auto-deleted (keepTempDirs=false)
	_, err = os.Stat(workdirPaths[2])
	assert.True(t, os.IsNotExist(err),
		"Temp dir should be auto-deleted with keepTempDirs=false, but exists: %s",
		workdirPaths[2])

	// 9. Cleanup all resources
	err = r.CleanupAllResources()
	require.NoError(t, err)
}
