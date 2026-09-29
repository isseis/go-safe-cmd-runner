package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/stretchr/testify/assert"
)

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

// TestErrUndefinedVariableDetail_Unwrap tests the Unwrap() method
func TestErrUndefinedVariableDetail_Unwrap(t *testing.T) {
	err := &ErrUndefinedVariableDetail{
		Level:        globalLevel(),
		Field:        envField(),
		VariableName: "UNDEF",
		Context:      "test",
	}

	assert.ErrorIs(t, err, ErrUndefinedVariable)
}

// TestLevelAndField_StringMatchesLegacyFormat pins that the typed Level and
// Field render exactly the strings the old fmt.Sprintf call sites produced.
func TestLevelAndField_StringMatchesLegacyFormat(t *testing.T) {
	levelCases := []struct {
		name string
		got  Level
		want string
	}{
		{"zero", Level{}, ""},
		{"global", globalLevel(), "global"},
		{"group", groupLevel("deploy"), "group[deploy]"},
		{"command", commandLevel("build"), "command[build]"},
		{"template", templateLevel("tpl"), "template[tpl]"},
	}
	for _, tc := range levelCases {
		t.Run("level_"+tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.got.String())
		})
	}

	fieldCases := []struct {
		name string
		got  Field
		want string
	}{
		{"zero", Field{}, ""},
		{"cmd", cmdField(), "cmd"},
		{"args", argsField(2), "args[2]"},
		{"env", envField(), "env"},
		{"env_vars", envVarsField(1), "env_vars[1]"},
		{"env_import", envImportField(), "env_import"},
		{"workdir", workdirField(), "workdir"},
		{"verify_files", verifyFilesField(0), "verify_files[0]"},
		{"cmd_allowed", cmdAllowedField(3), "cmd_allowed[3]"},
		{"vars", varsField(), "vars"},
		{"vars name", varField("dest"), "vars.dest"},
		{"vars element", varElementField("dest", 1), "vars.dest[1]"},
	}
	for _, tc := range fieldCases {
		t.Run("field_"+tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.got.String())
		})
	}
}

// TestErrUndefinedVariableDetail_StructuredMessage pins the order and the role
// of every segment, with and without an expansion chain, and that the rendered
// string matches the legacy format.
func TestErrUndefinedVariableDetail_StructuredMessage(t *testing.T) {
	tests := []struct {
		name  string
		chain []string
		want  errmsg.Segments
	}{
		{
			name: "no chain",
			want: errmsg.Segments{
				{Role: errmsg.RoleConstant, Text: "undefined variable in "},
				{Role: errmsg.RoleConstant, Text: "group["},
				{Role: errmsg.RoleIdentifier, Text: "backup"},
				{Role: errmsg.RoleConstant, Text: "]"},
				{Role: errmsg.RoleConstant, Text: "."},
				{Role: errmsg.RoleConstant, Text: "vars"},
				{Role: errmsg.RoleConstant, Text: "."},
				{Role: errmsg.RoleIdentifier, Text: "dest"},
				{Role: errmsg.RoleConstant, Text: ": '"},
				{Role: errmsg.RoleIdentifier, Text: "api_key"},
				{Role: errmsg.RoleConstant, Text: "' (context: "},
				{Role: errmsg.RoleText, Text: "raw %{api_key}"},
				{Role: errmsg.RoleConstant, Text: ")"},
			},
		},
		{
			name:  "with chain",
			chain: []string{"first", "second"},
			want: errmsg.Segments{
				{Role: errmsg.RoleConstant, Text: "undefined variable in "},
				{Role: errmsg.RoleConstant, Text: "group["},
				{Role: errmsg.RoleIdentifier, Text: "backup"},
				{Role: errmsg.RoleConstant, Text: "]"},
				{Role: errmsg.RoleConstant, Text: "."},
				{Role: errmsg.RoleConstant, Text: "vars"},
				{Role: errmsg.RoleConstant, Text: "."},
				{Role: errmsg.RoleIdentifier, Text: "dest"},
				{Role: errmsg.RoleConstant, Text: ": '"},
				{Role: errmsg.RoleIdentifier, Text: "api_key"},
				{Role: errmsg.RoleConstant, Text: "' (context: "},
				{Role: errmsg.RoleText, Text: "raw %{api_key}"},
				{Role: errmsg.RoleConstant, Text: ")"},
				{Role: errmsg.RoleConstant, Text: " (expansion path: "},
				{Role: errmsg.RoleIdentifier, Text: "first"},
				{Role: errmsg.RoleConstant, Text: " -> "},
				{Role: errmsg.RoleIdentifier, Text: "second"},
				{Role: errmsg.RoleConstant, Text: ")"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail := &ErrUndefinedVariableDetail{
				Level:        groupLevel("backup"),
				Field:        varField("dest"),
				VariableName: "api_key",
				Context:      "raw %{api_key}",
				Chain:        tt.chain,
			}

			assert.Equal(t, tt.want, detail.StructuredMessage().Segments())

			wantMsg := "undefined variable in group[backup].vars.dest: 'api_key' (context: raw %{api_key})"
			if len(tt.chain) > 0 {
				wantMsg += " (expansion path: first -> second)"
			}
			assert.Equal(t, wantMsg, detail.Error())
			assert.ErrorIs(t, detail, ErrUndefinedVariable)
		})
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

	assert.ErrorIs(t, err, ErrInvalidEscapeSequence)
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

// TestErrInvalidEnvImportFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvImportFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvImportFormatDetail{
		Level:   "command",
		Mapping: "bad",
		Reason:  "test",
	}

	assert.ErrorIs(t, err, ErrInvalidEnvImportFormat)
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

// TestErrDuplicateVariableDefinitionDetail_Unwrap tests the Unwrap() method
func TestErrDuplicateVariableDefinitionDetail_Unwrap(t *testing.T) {
	err := &ErrDuplicateVariableDefinitionDetail{
		Level:        "command",
		Field:        "env",
		VariableName: "DUP",
	}

	assert.ErrorIs(t, err, ErrDuplicateVariableDefinition)
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
