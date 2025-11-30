package security

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to build a validator with custom allowed patterns
func newValidatorForCmdAllowedTest(t *testing.T, patterns []string) *Validator {
	cfg := DefaultConfig()
	cfg.AllowedCommands = patterns
	validator, err := NewValidator(cfg)
	require.NoError(t, err)
	return validator
}

// resolvedEchoPattern returns a regex pattern that matches the symlink-resolved
// path of /bin/echo. This is necessary because ValidateCommandAllowed now
// resolves symlinks before pattern matching for security.
func resolvedEchoPattern(t *testing.T) string {
	resolved, err := filepath.EvalSymlinks("/bin/echo")
	require.NoError(t, err)
	return "^" + resolved + "$"
}

func TestValidateCommandAllowed_PatternMatchSingle(t *testing.T) {
	// Pattern must match the resolved path (e.g., /usr/bin/echo on systems where /bin -> /usr/bin)
	v := newValidatorForCmdAllowedTest(t, []string{resolvedEchoPattern(t)})
	err := v.ValidateCommandAllowed("/bin/echo", nil)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_PatternMatchMultiple(t *testing.T) {
	// Use patterns that cover both /bin/* and /usr/bin/* to handle symlink resolution
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/.*", "^/usr/bin/.*"})
	err := v.ValidateCommandAllowed("/bin/echo", nil)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_GroupExactMatchSingle(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{})
	// We need the resolved path for groupCmdAllowed map
	resolved, err := filepath.EvalSymlinks("/bin/echo")
	require.NoError(t, err)
	groupCmdAllowed := map[string]struct{}{resolved: {}}
	err = v.ValidateCommandAllowed("/bin/echo", groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_GroupExactMatchMultiple(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{})
	resolved, err := filepath.EvalSymlinks("/bin/echo")
	require.NoError(t, err)
	otherResolved := resolved + "-other" // ensure non-match extra element
	groupCmdAllowed := map[string]struct{}{
		otherResolved: {},
		resolved:      {},
	}
	err = v.ValidateCommandAllowed("/bin/echo", groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_ORBothMatch(t *testing.T) {
	resolved, err := filepath.EvalSymlinks("/bin/echo")
	require.NoError(t, err)
	// Pattern must match the resolved path
	v := newValidatorForCmdAllowedTest(t, []string{resolvedEchoPattern(t)})
	groupCmdAllowed := map[string]struct{}{resolved: {}}
	err = v.ValidateCommandAllowed("/bin/echo", groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_ORGlobalOnly(t *testing.T) {
	// Pattern must match the resolved path
	v := newValidatorForCmdAllowedTest(t, []string{resolvedEchoPattern(t)})
	groupCmdAllowed := map[string]struct{}{"/some/other/path": {}}
	err := v.ValidateCommandAllowed("/bin/echo", groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_ORGroupOnly(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/something$"})
	resolved, err := filepath.EvalSymlinks("/bin/echo")
	require.NoError(t, err)
	groupCmdAllowed := map[string]struct{}{resolved: {}}
	err = v.ValidateCommandAllowed("/bin/echo", groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_ErrorNeitherMatches(t *testing.T) {
	// Use resolved path pattern for echo, but test with ls which resolves differently
	v := newValidatorForCmdAllowedTest(t, []string{resolvedEchoPattern(t)})
	err := v.ValidateCommandAllowed("/bin/ls", nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandNotAllowed)
	_, isTyped := err.(*CommandNotAllowedError)
	assert.True(t, isTyped)
}

func TestValidateCommandAllowed_ErrorEmptyGroupListNoMatch(t *testing.T) {
	// Use resolved path pattern for echo, but test with ls
	v := newValidatorForCmdAllowedTest(t, []string{resolvedEchoPattern(t)})
	groupCmdAllowed := make(map[string]struct{})
	err := v.ValidateCommandAllowed("/bin/ls", groupCmdAllowed)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandNotAllowed)
}

func TestValidateCommandAllowed_ErrorEmptyCommandPath(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/echo$"})
	err := v.ValidateCommandAllowed("", nil)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrCommandNotAllowed) // structural error, not permission
}

func TestValidateCommandAllowed_ErrorMessageIncludesSymlinkResolution(t *testing.T) {
	// Test that when a symlink is involved and the resolved path doesn't match,
	// the error structure includes both the original path and the resolved path
	// This simulates /bin/offlineimap -> /usr/share/offlineimap/run scenario

	// Get a real symlink on the system for testing
	// /bin/echo typically resolves to /usr/bin/echo on modern Linux systems
	originalPath := "/bin/echo"
	resolvedPath, err := filepath.EvalSymlinks(originalPath)
	require.NoError(t, err)

	// Only run this test if /bin/echo is actually a symlink
	if resolvedPath == originalPath {
		t.Skip("Skipping test: /bin/echo is not a symlink on this system")
	}

	// Create validator with patterns that don't match the resolved path
	// This will cause validation to fail and generate an error message
	v := newValidatorForCmdAllowedTest(t, []string{"^/nonexistent/.*"})
	err = v.ValidateCommandAllowed(originalPath, nil)

	// Verify error is returned
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandNotAllowed)

	// Verify the error is of the correct type
	cmdErr, ok := err.(*CommandNotAllowedError)
	require.True(t, ok, "Error should be *CommandNotAllowedError")

	// Verify the error structure contains both paths
	// This tests the data structure, not the message format
	assert.Equal(t, originalPath, cmdErr.CommandPath, "CommandPath should be the original symlink path")
	assert.Equal(t, resolvedPath, cmdErr.ResolvedPath, "ResolvedPath should be the target of the symlink")
	assert.NotEqual(t, cmdErr.CommandPath, cmdErr.ResolvedPath, "Paths should differ for symlinks")

	// Verify error message contains both paths (structural requirement)
	// We only check that both paths appear somewhere in the message,
	// without assuming specific wording or format
	errMsg := err.Error()
	assert.Contains(t, errMsg, originalPath, "Error message must include the original path")
	assert.Contains(t, errMsg, resolvedPath, "Error message must include the resolved path")
}

func TestValidateCommandAllowed_ErrorMessageNoSymlink(t *testing.T) {
	// Test that when the path is not a symlink, the error structure
	// handles the case where CommandPath equals ResolvedPath

	// Create a validator with no matching patterns
	v := newValidatorForCmdAllowedTest(t, []string{"^/nonexistent/.*"})

	// Use a real path that exists but is not a symlink
	// /usr/bin/env is typically a real file, not a symlink
	testPath := "/usr/bin/env"
	_, evalErr := filepath.EvalSymlinks(testPath)
	require.NoError(t, evalErr, "Test path should exist and be resolvable")

	// Verify it's not a symlink (or at least resolves to itself)
	// This test is meaningful when testPath equals resolvedPath
	err := v.ValidateCommandAllowed(testPath, nil)

	// Verify error is returned
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandNotAllowed)

	// Verify the error is of the correct type
	cmdErr, ok := err.(*CommandNotAllowedError)
	require.True(t, ok, "Error should be *CommandNotAllowedError")

	// Verify the error structure is populated correctly
	// For non-symlinks, CommandPath and ResolvedPath should be identical.
	assert.Equal(t, testPath, cmdErr.CommandPath, "CommandPath should be the original path")
	assert.Equal(t, testPath, cmdErr.ResolvedPath, "ResolvedPath should be the same as the original path")

	// Verify error message contains the command path (structural requirement)
	errMsg := err.Error()
	assert.Contains(t, errMsg, testPath, "Error message must include the command path")
}
