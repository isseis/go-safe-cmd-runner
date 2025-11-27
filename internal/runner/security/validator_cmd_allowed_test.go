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
