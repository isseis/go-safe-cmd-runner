package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrInvalidVariableNameDetail_Error tests the Error() method
func TestErrInvalidVariableNameDetail_Error(t *testing.T) {
	err := &ErrInvalidVariableNameDetail{
		Level:        "global",
		Field:        "vars",
		VariableName: "invalid-var",
		Reason:       "contains hyphen",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrInvalidVariableNameDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrInvalidVariableNameDetail")

	// Verify field values
	assert.Equal(t, "global", detailErr.Level)
	assert.Equal(t, "vars", detailErr.Field)
	assert.Equal(t, "invalid-var", detailErr.VariableName)
	assert.Equal(t, "contains hyphen", detailErr.Reason)
}

// TestErrInvalidVariableNameDetail_Unwrap tests the Unwrap() method
func TestErrInvalidVariableNameDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidVariableNameDetail{
		Level:        "group",
		Field:        "from_env",
		VariableName: "bad_var",
		Reason:       "test reason",
	}

	assert.ErrorIs(t, err, ErrInvalidVariableName)
}

// TestErrInvalidSystemVariableNameDetail_Error tests the Error() method
func TestErrInvalidSystemVariableNameDetail_Error(t *testing.T) {
	err := &ErrInvalidSystemVariableNameDetail{
		Level:              "command",
		Field:              "from_env",
		SystemVariableName: "SYS-VAR",
		Reason:             "contains hyphen",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrInvalidSystemVariableNameDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrInvalidSystemVariableNameDetail")

	// Verify field values
	assert.Equal(t, "command", detailErr.Level)
	assert.Equal(t, "from_env", detailErr.Field)
	assert.Equal(t, "SYS-VAR", detailErr.SystemVariableName)
	assert.Equal(t, "contains hyphen", detailErr.Reason)
}

// TestErrInvalidSystemVariableNameDetail_Unwrap tests the Unwrap() method
func TestErrInvalidSystemVariableNameDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidSystemVariableNameDetail{
		Level:              "global",
		Field:              "from_env",
		SystemVariableName: "BAD_SYS",
		Reason:             "test",
	}

	assert.ErrorIs(t, err, ErrInvalidSystemVariableName)
}

// TestErrReservedVariablePrefixDetail_Error tests the Error() method
func TestErrReservedVariablePrefixDetail_Error(t *testing.T) {
	err := &ErrReservedVariablePrefixDetail{
		Level:        "global",
		Field:        "vars",
		VariableName: "RUNNER_SECRET",
		Prefix:       "RUNNER_",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrReservedVariablePrefixDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrReservedVariablePrefixDetail")

	// Verify field values
	assert.Equal(t, "global", detailErr.Level)
	assert.Equal(t, "vars", detailErr.Field)
	assert.Equal(t, "RUNNER_SECRET", detailErr.VariableName)
	assert.Equal(t, "RUNNER_", detailErr.Prefix)
}

// TestErrReservedVariablePrefixDetail_Unwrap tests the Unwrap() method
func TestErrReservedVariablePrefixDetail_Unwrap(t *testing.T) {
	err := &ErrReservedVariablePrefixDetail{
		Level:        "command",
		Field:        "env",
		VariableName: "RUNNER_VAR",
		Prefix:       "RUNNER_",
	}

	assert.ErrorIs(t, err, ErrReservedVariablePrefix)
}

// TestErrVariableNotInAllowlistDetail_Error tests the Error() method
func TestErrVariableNotInAllowlistDetail_Error(t *testing.T) {
	err := &ErrVariableNotInAllowlistDetail{
		Level:           "group",
		SystemVarName:   "SECRET_KEY",
		InternalVarName: "my_secret",
		Allowlist:       []string{"HOME", "PATH"},
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrVariableNotInAllowlistDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrVariableNotInAllowlistDetail")

	// Verify field values
	assert.Equal(t, "group", detailErr.Level)
	assert.Equal(t, "SECRET_KEY", detailErr.SystemVarName)
	assert.Equal(t, "my_secret", detailErr.InternalVarName)
	assert.Equal(t, []string{"HOME", "PATH"}, detailErr.Allowlist)
}

// TestErrVariableNotInAllowlistDetail_Unwrap tests the Unwrap() method
func TestErrVariableNotInAllowlistDetail_Unwrap(t *testing.T) {
	err := &ErrVariableNotInAllowlistDetail{
		Level:           "command",
		SystemVarName:   "SECRET",
		InternalVarName: "sec",
		Allowlist:       []string{},
	}

	assert.ErrorIs(t, err, ErrVariableNotInAllowlist)
}

// TestErrCircularReferenceDetail_Error tests the Error() method
func TestErrCircularReferenceDetail_Error(t *testing.T) {
	err := &ErrCircularReferenceDetail{
		Level:        "command",
		Field:        "vars",
		VariableName: "A",
		Chain:        []string{"A", "B", "C", "A"},
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrCircularReferenceDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrCircularReferenceDetail")

	// Verify field values
	assert.Equal(t, "command", detailErr.Level)
	assert.Equal(t, "vars", detailErr.Field)
	assert.Equal(t, "A", detailErr.VariableName)
	assert.Equal(t, []string{"A", "B", "C", "A"}, detailErr.Chain)
}

// TestErrCircularReferenceDetail_Unwrap tests the Unwrap() method
func TestErrCircularReferenceDetail_Unwrap(t *testing.T) {
	err := &ErrCircularReferenceDetail{
		Level:        "global",
		Field:        "vars",
		VariableName: "VAR",
		Chain:        []string{"VAR"},
	}

	assert.ErrorIs(t, err, ErrCircularReference)
}

// TestErrUndefinedVariableDetail_Error tests the Error() method
func TestErrUndefinedVariableDetail_Error(t *testing.T) {
	err := &ErrUndefinedVariableDetail{
		Level:        "command",
		Field:        "command_line",
		VariableName: "MISSING_VAR",
		Context:      "in command expansion",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrUndefinedVariableDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrUndefinedVariableDetail")

	// Verify field values
	assert.Equal(t, "command", detailErr.Level)
	assert.Equal(t, "command_line", detailErr.Field)
	assert.Equal(t, "MISSING_VAR", detailErr.VariableName)
	assert.Equal(t, "in command expansion", detailErr.Context)
}

// TestErrUndefinedVariableDetail_Unwrap tests the Unwrap() method
func TestErrUndefinedVariableDetail_Unwrap(t *testing.T) {
	err := &ErrUndefinedVariableDetail{
		Level:        "global",
		Field:        "env",
		VariableName: "UNDEF",
		Context:      "test",
	}

	assert.ErrorIs(t, err, ErrUndefinedVariable)
}

// TestErrInvalidEscapeSequenceDetail_Error tests the Error() method
func TestErrInvalidEscapeSequenceDetail_Error(t *testing.T) {
	err := &ErrInvalidEscapeSequenceDetail{
		Level:    "command",
		Field:    "command_line",
		Sequence: "\\x",
		Context:  "invalid escape in string",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrInvalidEscapeSequenceDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrInvalidEscapeSequenceDetail")

	// Verify field values
	assert.Equal(t, "command", detailErr.Level)
	assert.Equal(t, "command_line", detailErr.Field)
	assert.Equal(t, "\\x", detailErr.Sequence)
	assert.Equal(t, "invalid escape in string", detailErr.Context)
}

// TestErrInvalidEscapeSequenceDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEscapeSequenceDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEscapeSequenceDetail{
		Level:    "global",
		Field:    "vars",
		Sequence: "\\q",
		Context:  "test",
	}

	assert.ErrorIs(t, err, ErrInvalidEscapeSequence)
}

// TestErrUnclosedVariableReferenceDetail_Error tests the Error() method
func TestErrUnclosedVariableReferenceDetail_Error(t *testing.T) {
	err := &ErrUnclosedVariableReferenceDetail{
		Level:   "group",
		Field:   "env",
		Context: "%{VAR without closing",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrUnclosedVariableReferenceDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrUnclosedVariableReferenceDetail")

	// Verify field values
	assert.Equal(t, "group", detailErr.Level)
	assert.Equal(t, "env", detailErr.Field)
	assert.Equal(t, "%{VAR without closing", detailErr.Context)
}

// TestErrUnclosedVariableReferenceDetail_Unwrap tests the Unwrap() method
func TestErrUnclosedVariableReferenceDetail_Unwrap(t *testing.T) {
	err := &ErrUnclosedVariableReferenceDetail{
		Level:   "command",
		Field:   "vars",
		Context: "test",
	}

	assert.ErrorIs(t, err, ErrUnclosedVariableReference)
}

// TestErrMaxRecursionDepthExceededDetail_Error tests the Error() method
func TestErrMaxRecursionDepthExceededDetail_Error(t *testing.T) {
	err := &ErrMaxRecursionDepthExceededDetail{
		Level:    "command",
		Field:    "vars",
		MaxDepth: 100,
		Context:  "deep variable expansion",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrMaxRecursionDepthExceededDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrMaxRecursionDepthExceededDetail")

	// Verify field values
	assert.Equal(t, "command", detailErr.Level)
	assert.Equal(t, "vars", detailErr.Field)
	assert.Equal(t, 100, detailErr.MaxDepth)
	assert.Equal(t, "deep variable expansion", detailErr.Context)
}

// TestErrMaxRecursionDepthExceededDetail_Unwrap tests the Unwrap() method
func TestErrMaxRecursionDepthExceededDetail_Unwrap(t *testing.T) {
	err := &ErrMaxRecursionDepthExceededDetail{
		Level:    "global",
		Field:    "env",
		MaxDepth: 50,
		Context:  "test",
	}

	assert.ErrorIs(t, err, ErrMaxRecursionDepthExceeded)
}

// TestErrInvalidEnvImportFormatDetail_Error tests the Error() method
func TestErrInvalidEnvImportFormatDetail_Error(t *testing.T) {
	err := &ErrInvalidEnvImportFormatDetail{
		Level:   "global",
		Mapping: "invalid_mapping",
		Reason:  "missing equals sign",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrInvalidEnvImportFormatDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrInvalidEnvImportFormatDetail")

	// Verify field values
	assert.Equal(t, "global", detailErr.Level)
	assert.Equal(t, "invalid_mapping", detailErr.Mapping)
	assert.Equal(t, "missing equals sign", detailErr.Reason)
}

// TestErrInvalidEnvImportFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvImportFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvImportFormatDetail{
		Level:   "command",
		Mapping: "bad",
		Reason:  "test",
	}

	assert.ErrorIs(t, err, ErrInvalidEnvImportFormat)
}

// TestErrInvalidVarsFormatDetail_Error tests the Error() method
func TestErrInvalidVarsFormatDetail_Error(t *testing.T) {
	err := &ErrInvalidVarsFormatDetail{
		Level:   "group",
		Mapping: "var_without_value",
		Reason:  "no equals sign found",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrInvalidVarsFormatDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrInvalidVarsFormatDetail")

	// Verify field values
	assert.Equal(t, "group", detailErr.Level)
	assert.Equal(t, "var_without_value", detailErr.Mapping)
	assert.Equal(t, "no equals sign found", detailErr.Reason)
}

// TestErrInvalidVarsFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidVarsFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidVarsFormatDetail{
		Level:   "command",
		Mapping: "test",
		Reason:  "reason",
	}

	assert.ErrorIs(t, err, ErrInvalidVarsFormat)
}

// TestErrInvalidEnvFormatDetail_Error tests the Error() method
func TestErrInvalidEnvFormatDetail_Error(t *testing.T) {
	err := &ErrInvalidEnvFormatDetail{
		Level:   "command",
		Mapping: "ENV_VAR",
		Reason:  "missing value",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrInvalidEnvFormatDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrInvalidEnvFormatDetail")

	// Verify field values
	assert.Equal(t, "command", detailErr.Level)
	assert.Equal(t, "ENV_VAR", detailErr.Mapping)
	assert.Equal(t, "missing value", detailErr.Reason)
}

// TestErrInvalidEnvFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvFormatDetail{
		Level:   "global",
		Mapping: "BAD",
		Reason:  "test",
	}

	assert.ErrorIs(t, err, ErrInvalidEnvFormat)
}

// TestErrInvalidEnvKeyDetail_Error tests the Error() method
func TestErrInvalidEnvKeyDetail_Error(t *testing.T) {
	err := &ErrInvalidEnvKeyDetail{
		Level:   "global",
		Key:     "BAD-KEY",
		Context: "environment variable",
		Reason:  "contains hyphen",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrInvalidEnvKeyDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrInvalidEnvKeyDetail")

	// Verify field values
	assert.Equal(t, "global", detailErr.Level)
	assert.Equal(t, "BAD-KEY", detailErr.Key)
	assert.Equal(t, "environment variable", detailErr.Context)
	assert.Equal(t, "contains hyphen", detailErr.Reason)
}

// TestErrInvalidEnvKeyDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvKeyDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvKeyDetail{
		Level:   "command",
		Key:     "INVALID",
		Context: "test",
		Reason:  "test reason",
	}

	assert.ErrorIs(t, err, ErrInvalidEnvKey)
}

// TestErrDuplicateVariableDefinitionDetail_Error tests the Error() method
func TestErrDuplicateVariableDefinitionDetail_Error(t *testing.T) {
	err := &ErrDuplicateVariableDefinitionDetail{
		Level:        "group",
		Field:        "vars",
		VariableName: "DUPLICATE_VAR",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrDuplicateVariableDefinitionDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrDuplicateVariableDefinitionDetail")

	// Verify field values
	assert.Equal(t, "group", detailErr.Level)
	assert.Equal(t, "vars", detailErr.Field)
	assert.Equal(t, "DUPLICATE_VAR", detailErr.VariableName)
}

// TestErrDuplicateVariableDefinitionDetail_Unwrap tests the Unwrap() method
func TestErrDuplicateVariableDefinitionDetail_Unwrap(t *testing.T) {
	err := &ErrDuplicateVariableDefinitionDetail{
		Level:        "command",
		Field:        "env",
		VariableName: "DUP",
	}

	assert.ErrorIs(t, err, ErrDuplicateVariableDefinition)
}

// TestErrEnvImportVarsConflictDetail_Error tests the Error() method
func TestErrEnvImportVarsConflictDetail_Error(t *testing.T) {
	err := &ErrEnvImportVarsConflictDetail{
		Level:          "group[deploy]",
		VariableName:   "CONFLICT_VAR",
		EnvImportLevel: "global",
		VarsLevel:      "group[deploy]",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrEnvImportVarsConflictDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrEnvImportVarsConflictDetail")

	// Verify field values
	assert.Equal(t, "group[deploy]", detailErr.Level)
	assert.Equal(t, "CONFLICT_VAR", detailErr.VariableName)
	assert.Equal(t, "global", detailErr.EnvImportLevel)
	assert.Equal(t, "group[deploy]", detailErr.VarsLevel)
}

// TestErrEnvImportVarsConflictDetail_Unwrap tests the Unwrap() method
func TestErrEnvImportVarsConflictDetail_Unwrap(t *testing.T) {
	err := &ErrEnvImportVarsConflictDetail{
		Level:          "global",
		VariableName:   "CONFLICT_VAR",
		EnvImportLevel: "global",
		VarsLevel:      "global",
	}

	assert.ErrorIs(t, err, ErrEnvImportVarsConflict)
}

// TestErrDuplicatePathDetail_Error tests the Error() method
func TestErrDuplicatePathDetail_Error(t *testing.T) {
	err := &ErrDuplicatePathDetail{
		Level:      "group[testgroup]",
		Field:      "cmd_allowed",
		Path:       "/usr/bin/tool",
		FirstIndex: 0,
		DupeIndex:  3,
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrDuplicatePathDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrDuplicatePathDetail")

	// Verify field values
	assert.Equal(t, "group[testgroup]", detailErr.Level)
	assert.Equal(t, "cmd_allowed", detailErr.Field)
	assert.Equal(t, "/usr/bin/tool", detailErr.Path)
	assert.Equal(t, 0, detailErr.FirstIndex)
	assert.Equal(t, 3, detailErr.DupeIndex)
}

// TestErrDuplicatePathDetail_Unwrap tests the Unwrap() method
func TestErrDuplicatePathDetail_Unwrap(t *testing.T) {
	err := &ErrDuplicatePathDetail{
		Level:      "group[test]",
		Field:      "cmd_allowed",
		Path:       "/bin/sh",
		FirstIndex: 1,
		DupeIndex:  2,
	}

	assert.ErrorIs(t, err, ErrDuplicatePath)
}

// TestErrDuplicateResolvedPathDetail_Error tests the Error() method
func TestErrDuplicateResolvedPathDetail_Error(t *testing.T) {
	err := &ErrDuplicateResolvedPathDetail{
		Level:        "group[mygroup]",
		Field:        "cmd_allowed",
		OriginalPath: "/usr/bin/tool-link",
		ResolvedPath: "/usr/bin/tool",
	}

	// Verify it's the correct error type using errors.As
	var detailErr *ErrDuplicateResolvedPathDetail
	require.True(t, errors.As(err, &detailErr), "error should be ErrDuplicateResolvedPathDetail")

	// Verify field values
	assert.Equal(t, "group[mygroup]", detailErr.Level)
	assert.Equal(t, "cmd_allowed", detailErr.Field)
	assert.Equal(t, "/usr/bin/tool-link", detailErr.OriginalPath)
	assert.Equal(t, "/usr/bin/tool", detailErr.ResolvedPath)
}

// TestErrDuplicateResolvedPathDetail_Unwrap tests the Unwrap() method
func TestErrDuplicateResolvedPathDetail_Unwrap(t *testing.T) {
	err := &ErrDuplicateResolvedPathDetail{
		Level:        "group[test]",
		Field:        "cmd_allowed",
		OriginalPath: "/link",
		ResolvedPath: "/target",
	}

	assert.ErrorIs(t, err, ErrDuplicateResolvedPath)
}
