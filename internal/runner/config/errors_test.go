package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestErrInvalidVariableNameDetail_Error tests the Error() method
func TestErrInvalidVariableNameDetail_Error(t *testing.T) {
	err := &ErrInvalidVariableNameDetail{
		Level:        "global",
		Field:        "vars",
		VariableName: "invalid-var",
		Reason:       "contains hyphen",
	}

	expected := "invalid variable name in global.vars: 'invalid-var' (contains hyphen)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrInvalidVariableNameDetail_Unwrap tests the Unwrap() method
func TestErrInvalidVariableNameDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidVariableNameDetail{
		Level:        "group",
		Field:        "from_env",
		VariableName: "bad_var",
		Reason:       "test reason",
	}

	assert.True(t, errors.Is(err, ErrInvalidVariableName), "Unwrap() should return ErrInvalidVariableName")
}

// TestErrInvalidSystemVariableNameDetail_Error tests the Error() method
func TestErrInvalidSystemVariableNameDetail_Error(t *testing.T) {
	err := &ErrInvalidSystemVariableNameDetail{
		Level:              "command",
		Field:              "from_env",
		SystemVariableName: "SYS-VAR",
		Reason:             "contains hyphen",
	}

	expected := "invalid system variable name in command.from_env: 'SYS-VAR' (contains hyphen)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrInvalidSystemVariableNameDetail_Unwrap tests the Unwrap() method
func TestErrInvalidSystemVariableNameDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidSystemVariableNameDetail{
		Level:              "global",
		Field:              "from_env",
		SystemVariableName: "BAD_SYS",
		Reason:             "test",
	}

	assert.True(t, errors.Is(err, ErrInvalidSystemVariableName), "Unwrap() should return ErrInvalidSystemVariableName")
}

// TestErrReservedVariablePrefixDetail_Error tests the Error() method
func TestErrReservedVariablePrefixDetail_Error(t *testing.T) {
	err := &ErrReservedVariablePrefixDetail{
		Level:        "global",
		Field:        "vars",
		VariableName: "RUNNER_SECRET",
		Prefix:       "RUNNER_",
	}

	expected := "reserved variable prefix in global.vars: 'RUNNER_SECRET' (prefix 'RUNNER_' is reserved)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrReservedVariablePrefixDetail_Unwrap tests the Unwrap() method
func TestErrReservedVariablePrefixDetail_Unwrap(t *testing.T) {
	err := &ErrReservedVariablePrefixDetail{
		Level:        "command",
		Field:        "env",
		VariableName: "RUNNER_VAR",
		Prefix:       "RUNNER_",
	}

	assert.True(t, errors.Is(err, ErrReservedVariablePrefix), "Unwrap() should return ErrReservedVariablePrefix")
}

// TestErrVariableNotInAllowlistDetail_Error tests the Error() method
func TestErrVariableNotInAllowlistDetail_Error(t *testing.T) {
	err := &ErrVariableNotInAllowlistDetail{
		Level:           "group",
		SystemVarName:   "SECRET_KEY",
		InternalVarName: "my_secret",
		Allowlist:       []string{"HOME", "PATH"},
	}

	expected := "system environment variable 'SECRET_KEY' not in allowlist (referenced as 'my_secret' in group.from_env)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrVariableNotInAllowlistDetail_Unwrap tests the Unwrap() method
func TestErrVariableNotInAllowlistDetail_Unwrap(t *testing.T) {
	err := &ErrVariableNotInAllowlistDetail{
		Level:           "command",
		SystemVarName:   "SECRET",
		InternalVarName: "sec",
		Allowlist:       []string{},
	}

	assert.True(t, errors.Is(err, ErrVariableNotInAllowlist), "Unwrap() should return ErrVariableNotInAllowlist")
}

// TestErrCircularReferenceDetail_Error tests the Error() method
func TestErrCircularReferenceDetail_Error(t *testing.T) {
	err := &ErrCircularReferenceDetail{
		Level:        "command",
		Field:        "vars",
		VariableName: "A",
		Chain:        []string{"A", "B", "C", "A"},
	}

	expected := "circular reference in command.vars: 'A' (chain: [A B C A])"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrCircularReferenceDetail_Unwrap tests the Unwrap() method
func TestErrCircularReferenceDetail_Unwrap(t *testing.T) {
	err := &ErrCircularReferenceDetail{
		Level:        "global",
		Field:        "vars",
		VariableName: "VAR",
		Chain:        []string{"VAR"},
	}

	assert.True(t, errors.Is(err, ErrCircularReference), "Unwrap() should return ErrCircularReference")
}

// TestErrUndefinedVariableDetail_Error tests the Error() method
func TestErrUndefinedVariableDetail_Error(t *testing.T) {
	err := &ErrUndefinedVariableDetail{
		Level:        "command",
		Field:        "command_line",
		VariableName: "MISSING_VAR",
		Context:      "in command expansion",
	}

	expected := "undefined variable in command.command_line: 'MISSING_VAR' (context: in command expansion)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrUndefinedVariableDetail_Unwrap tests the Unwrap() method
func TestErrUndefinedVariableDetail_Unwrap(t *testing.T) {
	err := &ErrUndefinedVariableDetail{
		Level:        "global",
		Field:        "env",
		VariableName: "UNDEF",
		Context:      "test",
	}

	assert.True(t, errors.Is(err, ErrUndefinedVariable), "Unwrap() should return ErrUndefinedVariable")
}

// TestErrInvalidEscapeSequenceDetail_Error tests the Error() method
func TestErrInvalidEscapeSequenceDetail_Error(t *testing.T) {
	err := &ErrInvalidEscapeSequenceDetail{
		Level:    "command",
		Field:    "command_line",
		Sequence: "\\x",
		Context:  "invalid escape in string",
	}

	expected := "invalid escape sequence in command.command_line: '\\x' (context: invalid escape in string)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrInvalidEscapeSequenceDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEscapeSequenceDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEscapeSequenceDetail{
		Level:    "global",
		Field:    "vars",
		Sequence: "\\q",
		Context:  "test",
	}

	assert.True(t, errors.Is(err, ErrInvalidEscapeSequence), "Unwrap() should return ErrInvalidEscapeSequence")
}

// TestErrUnclosedVariableReferenceDetail_Error tests the Error() method
func TestErrUnclosedVariableReferenceDetail_Error(t *testing.T) {
	err := &ErrUnclosedVariableReferenceDetail{
		Level:   "group",
		Field:   "env",
		Context: "%{VAR without closing",
	}

	expected := "unclosed variable reference in group.env: missing closing '}' (context: %{VAR without closing)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrUnclosedVariableReferenceDetail_Unwrap tests the Unwrap() method
func TestErrUnclosedVariableReferenceDetail_Unwrap(t *testing.T) {
	err := &ErrUnclosedVariableReferenceDetail{
		Level:   "command",
		Field:   "vars",
		Context: "test",
	}

	assert.True(t, errors.Is(err, ErrUnclosedVariableReference), "Unwrap() should return ErrUnclosedVariableReference")
}

// TestErrMaxRecursionDepthExceededDetail_Error tests the Error() method
func TestErrMaxRecursionDepthExceededDetail_Error(t *testing.T) {
	err := &ErrMaxRecursionDepthExceededDetail{
		Level:    "command",
		Field:    "vars",
		MaxDepth: 100,
		Context:  "deep variable expansion",
	}

	expected := "maximum recursion depth exceeded in command.vars: limit 100 (context: deep variable expansion)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrMaxRecursionDepthExceededDetail_Unwrap tests the Unwrap() method
func TestErrMaxRecursionDepthExceededDetail_Unwrap(t *testing.T) {
	err := &ErrMaxRecursionDepthExceededDetail{
		Level:    "global",
		Field:    "env",
		MaxDepth: 50,
		Context:  "test",
	}

	assert.True(t, errors.Is(err, ErrMaxRecursionDepthExceeded), "Unwrap() should return ErrMaxRecursionDepthExceeded")
}

// TestErrInvalidFromEnvFormatDetail_Error tests the Error() method
func TestErrInvalidFromEnvFormatDetail_Error(t *testing.T) {
	err := &ErrInvalidFromEnvFormatDetail{
		Level:   "global",
		Mapping: "invalid_mapping",
		Reason:  "missing equals sign",
	}

	expected := "invalid from_env format in global: 'invalid_mapping' (missing equals sign)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrInvalidFromEnvFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidFromEnvFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidFromEnvFormatDetail{
		Level:   "command",
		Mapping: "bad",
		Reason:  "test",
	}

	assert.True(t, errors.Is(err, ErrInvalidFromEnvFormat), "Unwrap() should return ErrInvalidFromEnvFormat")
}

// TestErrInvalidVarsFormatDetail_Error tests the Error() method
func TestErrInvalidVarsFormatDetail_Error(t *testing.T) {
	err := &ErrInvalidVarsFormatDetail{
		Level:   "group",
		Mapping: "var_without_value",
		Reason:  "no equals sign found",
	}

	expected := "invalid vars format in group: 'var_without_value' (no equals sign found)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrInvalidVarsFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidVarsFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidVarsFormatDetail{
		Level:   "command",
		Mapping: "test",
		Reason:  "reason",
	}

	assert.True(t, errors.Is(err, ErrInvalidVarsFormat), "Unwrap() should return ErrInvalidVarsFormat")
}

// TestErrInvalidEnvFormatDetail_Error tests the Error() method
func TestErrInvalidEnvFormatDetail_Error(t *testing.T) {
	err := &ErrInvalidEnvFormatDetail{
		Level:   "command",
		Mapping: "ENV_VAR",
		Reason:  "missing value",
	}

	expected := "invalid env format in command: 'ENV_VAR' (missing value)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrInvalidEnvFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvFormatDetail{
		Level:   "global",
		Mapping: "BAD",
		Reason:  "test",
	}

	assert.True(t, errors.Is(err, ErrInvalidEnvFormat), "Unwrap() should return ErrInvalidEnvFormat")
}

// TestErrInvalidEnvKeyDetail_Error tests the Error() method
func TestErrInvalidEnvKeyDetail_Error(t *testing.T) {
	err := &ErrInvalidEnvKeyDetail{
		Level:   "global",
		Key:     "BAD-KEY",
		Context: "environment variable",
		Reason:  "contains hyphen",
	}

	expected := "invalid environment variable key in global: 'BAD-KEY' (context: environment variable, reason: contains hyphen)"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrInvalidEnvKeyDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvKeyDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvKeyDetail{
		Level:   "command",
		Key:     "INVALID",
		Context: "test",
		Reason:  "test reason",
	}

	assert.True(t, errors.Is(err, ErrInvalidEnvKey), "Unwrap() should return ErrInvalidEnvKey")
}

// TestErrDuplicateVariableDefinitionDetail_Error tests the Error() method
func TestErrDuplicateVariableDefinitionDetail_Error(t *testing.T) {
	err := &ErrDuplicateVariableDefinitionDetail{
		Level:        "group",
		Field:        "vars",
		VariableName: "DUPLICATE_VAR",
	}

	expected := "duplicate variable definition in group.vars: 'DUPLICATE_VAR' is defined multiple times"
	assert.Equal(t, expected, err.Error(), "Error() should return expected message")
}

// TestErrDuplicateVariableDefinitionDetail_Unwrap tests the Unwrap() method
func TestErrDuplicateVariableDefinitionDetail_Unwrap(t *testing.T) {
	err := &ErrDuplicateVariableDefinitionDetail{
		Level:        "command",
		Field:        "env",
		VariableName: "DUP",
	}

	assert.True(t, errors.Is(err, ErrDuplicateVariableDefinition), "Unwrap() should return ErrDuplicateVariableDefinition")
}
