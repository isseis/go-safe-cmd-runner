//go:build test

package config

import (
	"errors"
	"testing"
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrInvalidVariableNameDetail_Unwrap tests the Unwrap() method
func TestErrInvalidVariableNameDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidVariableNameDetail{
		Level:        "group",
		Field:        "from_env",
		VariableName: "bad_var",
		Reason:       "test reason",
	}

	if !errors.Is(err, ErrInvalidVariableName) {
		t.Errorf("Unwrap() should return ErrInvalidVariableName")
	}
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrInvalidSystemVariableNameDetail_Unwrap tests the Unwrap() method
func TestErrInvalidSystemVariableNameDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidSystemVariableNameDetail{
		Level:              "global",
		Field:              "from_env",
		SystemVariableName: "BAD_SYS",
		Reason:             "test",
	}

	if !errors.Is(err, ErrInvalidSystemVariableName) {
		t.Errorf("Unwrap() should return ErrInvalidSystemVariableName")
	}
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrReservedVariablePrefixDetail_Unwrap tests the Unwrap() method
func TestErrReservedVariablePrefixDetail_Unwrap(t *testing.T) {
	err := &ErrReservedVariablePrefixDetail{
		Level:        "command",
		Field:        "env",
		VariableName: "RUNNER_VAR",
		Prefix:       "RUNNER_",
	}

	if !errors.Is(err, ErrReservedVariablePrefix) {
		t.Errorf("Unwrap() should return ErrReservedVariablePrefix")
	}
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrVariableNotInAllowlistDetail_Unwrap tests the Unwrap() method
func TestErrVariableNotInAllowlistDetail_Unwrap(t *testing.T) {
	err := &ErrVariableNotInAllowlistDetail{
		Level:           "command",
		SystemVarName:   "SECRET",
		InternalVarName: "sec",
		Allowlist:       []string{},
	}

	if !errors.Is(err, ErrVariableNotInAllowlist) {
		t.Errorf("Unwrap() should return ErrVariableNotInAllowlist")
	}
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrCircularReferenceDetail_Unwrap tests the Unwrap() method
func TestErrCircularReferenceDetail_Unwrap(t *testing.T) {
	err := &ErrCircularReferenceDetail{
		Level:        "global",
		Field:        "vars",
		VariableName: "VAR",
		Chain:        []string{"VAR"},
	}

	if !errors.Is(err, ErrCircularReference) {
		t.Errorf("Unwrap() should return ErrCircularReference")
	}
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrUndefinedVariableDetail_Unwrap tests the Unwrap() method
func TestErrUndefinedVariableDetail_Unwrap(t *testing.T) {
	err := &ErrUndefinedVariableDetail{
		Level:        "global",
		Field:        "env",
		VariableName: "UNDEF",
		Context:      "test",
	}

	if !errors.Is(err, ErrUndefinedVariable) {
		t.Errorf("Unwrap() should return ErrUndefinedVariable")
	}
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrInvalidEscapeSequenceDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEscapeSequenceDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEscapeSequenceDetail{
		Level:    "global",
		Field:    "vars",
		Sequence: "\\q",
		Context:  "test",
	}

	if !errors.Is(err, ErrInvalidEscapeSequence) {
		t.Errorf("Unwrap() should return ErrInvalidEscapeSequence")
	}
}

// TestErrUnclosedVariableReferenceDetail_Error tests the Error() method
func TestErrUnclosedVariableReferenceDetail_Error(t *testing.T) {
	err := &ErrUnclosedVariableReferenceDetail{
		Level:   "group",
		Field:   "env",
		Context: "%{VAR without closing",
	}

	expected := "unclosed variable reference in group.env: missing closing '}' (context: %{VAR without closing)"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrUnclosedVariableReferenceDetail_Unwrap tests the Unwrap() method
func TestErrUnclosedVariableReferenceDetail_Unwrap(t *testing.T) {
	err := &ErrUnclosedVariableReferenceDetail{
		Level:   "command",
		Field:   "vars",
		Context: "test",
	}

	if !errors.Is(err, ErrUnclosedVariableReference) {
		t.Errorf("Unwrap() should return ErrUnclosedVariableReference")
	}
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrMaxRecursionDepthExceededDetail_Unwrap tests the Unwrap() method
func TestErrMaxRecursionDepthExceededDetail_Unwrap(t *testing.T) {
	err := &ErrMaxRecursionDepthExceededDetail{
		Level:    "global",
		Field:    "env",
		MaxDepth: 50,
		Context:  "test",
	}

	if !errors.Is(err, ErrMaxRecursionDepthExceeded) {
		t.Errorf("Unwrap() should return ErrMaxRecursionDepthExceeded")
	}
}

// TestErrInvalidFromEnvFormatDetail_Error tests the Error() method
func TestErrInvalidFromEnvFormatDetail_Error(t *testing.T) {
	err := &ErrInvalidFromEnvFormatDetail{
		Level:   "global",
		Mapping: "invalid_mapping",
		Reason:  "missing equals sign",
	}

	expected := "invalid from_env format in global: 'invalid_mapping' (missing equals sign)"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrInvalidFromEnvFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidFromEnvFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidFromEnvFormatDetail{
		Level:   "command",
		Mapping: "bad",
		Reason:  "test",
	}

	if !errors.Is(err, ErrInvalidFromEnvFormat) {
		t.Errorf("Unwrap() should return ErrInvalidFromEnvFormat")
	}
}

// TestErrInvalidVarsFormatDetail_Error tests the Error() method
func TestErrInvalidVarsFormatDetail_Error(t *testing.T) {
	err := &ErrInvalidVarsFormatDetail{
		Level:   "group",
		Mapping: "var_without_value",
		Reason:  "no equals sign found",
	}

	expected := "invalid vars format in group: 'var_without_value' (no equals sign found)"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrInvalidVarsFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidVarsFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidVarsFormatDetail{
		Level:   "command",
		Mapping: "test",
		Reason:  "reason",
	}

	if !errors.Is(err, ErrInvalidVarsFormat) {
		t.Errorf("Unwrap() should return ErrInvalidVarsFormat")
	}
}

// TestErrInvalidEnvFormatDetail_Error tests the Error() method
func TestErrInvalidEnvFormatDetail_Error(t *testing.T) {
	err := &ErrInvalidEnvFormatDetail{
		Level:   "command",
		Mapping: "ENV_VAR",
		Reason:  "missing value",
	}

	expected := "invalid env format in command: 'ENV_VAR' (missing value)"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrInvalidEnvFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvFormatDetail{
		Level:   "global",
		Mapping: "BAD",
		Reason:  "test",
	}

	if !errors.Is(err, ErrInvalidEnvFormat) {
		t.Errorf("Unwrap() should return ErrInvalidEnvFormat")
	}
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
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrInvalidEnvKeyDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvKeyDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvKeyDetail{
		Level:   "command",
		Key:     "INVALID",
		Context: "test",
		Reason:  "test reason",
	}

	if !errors.Is(err, ErrInvalidEnvKey) {
		t.Errorf("Unwrap() should return ErrInvalidEnvKey")
	}
}

// TestErrDuplicateVariableDefinitionDetail_Error tests the Error() method
func TestErrDuplicateVariableDefinitionDetail_Error(t *testing.T) {
	err := &ErrDuplicateVariableDefinitionDetail{
		Level:        "group",
		Field:        "vars",
		VariableName: "DUPLICATE_VAR",
	}

	expected := "duplicate variable definition in group.vars: 'DUPLICATE_VAR' is defined multiple times"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

// TestErrDuplicateVariableDefinitionDetail_Unwrap tests the Unwrap() method
func TestErrDuplicateVariableDefinitionDetail_Unwrap(t *testing.T) {
	err := &ErrDuplicateVariableDefinitionDetail{
		Level:        "command",
		Field:        "env",
		VariableName: "DUP",
	}

	if !errors.Is(err, ErrDuplicateVariableDefinition) {
		t.Errorf("Unwrap() should return ErrDuplicateVariableDefinition")
	}
}
