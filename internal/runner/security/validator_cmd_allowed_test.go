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

// resolveToCanonical resolves a path to its canonical (symlink-resolved) form.
// This simulates what PathResolver.ResolvePath() does before calling ValidateCommandAllowed.
func resolveToCanonical(t *testing.T, path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err, "Failed to resolve path: %s", path)
	return resolved
}

// Note: These tests pass symlink-resolved paths to ValidateCommandAllowed,
// simulating the real-world scenario where PathResolver.ResolvePath() resolves
// symlinks before calling ValidateCommandAllowed().

func TestValidateCommandAllowed_PatternMatchSingle(t *testing.T) {
	// Get the resolved path of /bin/echo (e.g., /usr/bin/echo)
	resolvedEcho := resolveToCanonical(t, "/bin/echo")

	// Create validator with pattern matching the resolved path
	v := newValidatorForCmdAllowedTest(t, []string{"^" + resolvedEcho + "$"})

	// Pass the resolved path (as PathResolver would)
	err := v.ValidateCommandAllowed(resolvedEcho, nil)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_PatternMatchMultiple(t *testing.T) {
	resolvedEcho := resolveToCanonical(t, "/bin/echo")

	// Use patterns that cover both /bin/* and /usr/bin/*
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/.*", "^/usr/bin/.*"})

	// Pass the resolved path
	err := v.ValidateCommandAllowed(resolvedEcho, nil)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_GroupExactMatchSingle(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{})
	resolvedEcho := resolveToCanonical(t, "/bin/echo")

	// groupCmdAllowed map should contain resolved paths (same as cmdPath)
	groupCmdAllowed := map[string]struct{}{resolvedEcho: {}}

	err := v.ValidateCommandAllowed(resolvedEcho, groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_GroupExactMatchMultiple(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{})
	resolvedEcho := resolveToCanonical(t, "/bin/echo")
	otherResolved := resolvedEcho + "-other" // ensure non-match extra element

	groupCmdAllowed := map[string]struct{}{
		otherResolved: {},
		resolvedEcho:  {},
	}

	err := v.ValidateCommandAllowed(resolvedEcho, groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_ORBothMatch(t *testing.T) {
	resolvedEcho := resolveToCanonical(t, "/bin/echo")

	// Pattern matches resolved path
	v := newValidatorForCmdAllowedTest(t, []string{"^" + resolvedEcho + "$"})
	groupCmdAllowed := map[string]struct{}{resolvedEcho: {}}

	err := v.ValidateCommandAllowed(resolvedEcho, groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_ORGlobalOnly(t *testing.T) {
	resolvedEcho := resolveToCanonical(t, "/bin/echo")

	// Pattern matches resolved path
	v := newValidatorForCmdAllowedTest(t, []string{"^" + resolvedEcho + "$"})
	groupCmdAllowed := map[string]struct{}{"/some/other/path": {}}

	err := v.ValidateCommandAllowed(resolvedEcho, groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_ORGroupOnly(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/something$"})
	resolvedEcho := resolveToCanonical(t, "/bin/echo")

	groupCmdAllowed := map[string]struct{}{resolvedEcho: {}}

	err := v.ValidateCommandAllowed(resolvedEcho, groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_ErrorNeitherMatches(t *testing.T) {
	resolvedEcho := resolveToCanonical(t, "/bin/echo")
	resolvedLs := resolveToCanonical(t, "/bin/ls")

	// Pattern matches resolved echo, but we test with resolved ls
	v := newValidatorForCmdAllowedTest(t, []string{"^" + resolvedEcho + "$"})

	err := v.ValidateCommandAllowed(resolvedLs, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandNotAllowed)
	_, isTyped := err.(*CommandNotAllowedError)
	assert.True(t, isTyped)
}

func TestValidateCommandAllowed_ErrorEmptyGroupListNoMatch(t *testing.T) {
	resolvedEcho := resolveToCanonical(t, "/bin/echo")
	resolvedLs := resolveToCanonical(t, "/bin/ls")

	// Pattern matches resolved echo, but we test with resolved ls
	v := newValidatorForCmdAllowedTest(t, []string{"^" + resolvedEcho + "$"})
	groupCmdAllowed := make(map[string]struct{})

	err := v.ValidateCommandAllowed(resolvedLs, groupCmdAllowed)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandNotAllowed)
}

func TestValidateCommandAllowed_ErrorEmptyCommandPath(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/echo$"})
	err := v.ValidateCommandAllowed("", nil)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrCommandNotAllowed) // structural error, not permission
}

func TestValidateCommandAllowed_ErrorMessageIncludesPath(t *testing.T) {
	// Test that when validation fails, the error structure includes the path.
	// Note: Since ValidateCommandAllowed now receives symlink-resolved paths,
	// CommandPath and ResolvedPath will be the same.

	resolvedEcho := resolveToCanonical(t, "/bin/echo")

	// Create validator with patterns that don't match
	v := newValidatorForCmdAllowedTest(t, []string{"^/nonexistent/.*"})
	err := v.ValidateCommandAllowed(resolvedEcho, nil)

	// Verify error is returned
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandNotAllowed)

	// Verify the error is of the correct type
	cmdErr, ok := err.(*CommandNotAllowedError)
	require.True(t, ok, "Error should be *CommandNotAllowedError")

	// Verify the error structure contains the path
	// Since the path is already resolved by PathResolver, both should be the same
	assert.Equal(t, resolvedEcho, cmdErr.CommandPath, "CommandPath should be the resolved path")
	assert.Equal(t, resolvedEcho, cmdErr.ResolvedPath, "ResolvedPath should be the same (already resolved)")

	// Verify error message contains the path
	errMsg := err.Error()
	assert.Contains(t, errMsg, resolvedEcho, "Error message must include the command path")
}

func TestValidateCommandAllowed_ErrorMessageNoSymlink(t *testing.T) {
	// Test that when the path is not a symlink (resolved == original),
	// the error structure is populated correctly

	// Create a validator with no matching patterns
	v := newValidatorForCmdAllowedTest(t, []string{"^/nonexistent/.*"})

	// Use a real path that exists and is typically not a symlink
	testPath := "/usr/bin/env"
	resolvedPath := resolveToCanonical(t, testPath)

	// Only run this test if the path is NOT a symlink
	if resolvedPath != testPath {
		t.Skip("Skipping test: test path is a symlink on this system")
	}

	err := v.ValidateCommandAllowed(resolvedPath, nil)

	// Verify error is returned
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandNotAllowed)

	// Verify the error is of the correct type
	cmdErr, ok := err.(*CommandNotAllowedError)
	require.True(t, ok, "Error should be *CommandNotAllowedError")

	// Verify the error structure is populated correctly
	assert.Equal(t, testPath, cmdErr.CommandPath, "CommandPath should be the path")
	assert.Equal(t, testPath, cmdErr.ResolvedPath, "ResolvedPath should be the same")

	// Verify error message contains the command path
	errMsg := err.Error()
	assert.Contains(t, errMsg, testPath, "Error message must include the command path")
}
