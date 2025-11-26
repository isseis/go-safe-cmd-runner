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

func TestValidateCommandAllowed_PatternMatchSingle(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/echo$"})
	err := v.ValidateCommandAllowed("/bin/echo", nil)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_PatternMatchMultiple(t *testing.T) {
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
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/echo$"})
	groupCmdAllowed := map[string]struct{}{resolved: {}}
	err = v.ValidateCommandAllowed("/bin/echo", groupCmdAllowed)
	assert.NoError(t, err)
}

func TestValidateCommandAllowed_ORGlobalOnly(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/echo$"})
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
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/echo$"})
	err := v.ValidateCommandAllowed("/bin/ls", nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCommandNotAllowed)
	_, isTyped := err.(*CommandNotAllowedError)
	assert.True(t, isTyped)
}

func TestValidateCommandAllowed_ErrorEmptyGroupListNoMatch(t *testing.T) {
	v := newValidatorForCmdAllowedTest(t, []string{"^/bin/echo$"})
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
