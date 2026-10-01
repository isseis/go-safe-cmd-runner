package config

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrInvalidVariableNameDetail_Unwrap tests the Unwrap() method
func TestErrInvalidVariableNameDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidVariableNameDetail{
		Level:        groupLevel("deploy"),
		Field:        envImportField(),
		VariableName: "bad_var",
		Reason:       "test reason",
	}

	assert.ErrorIs(t, err, ErrInvalidVariableName)
}

// TestErrInvalidSystemVariableNameDetail_Unwrap tests the Unwrap() method
func TestErrInvalidSystemVariableNameDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidSystemVariableNameDetail{
		Level:              globalLevel(),
		Field:              envImportField(),
		SystemVariableName: "BAD_SYS",
		Reason:             "test",
	}

	assert.ErrorIs(t, err, ErrInvalidSystemVariableName)
}

// TestErrReservedVariablePrefixDetail_Unwrap tests the Unwrap() method
func TestErrReservedVariablePrefixDetail_Unwrap(t *testing.T) {
	err := &ErrReservedVariablePrefixDetail{
		Level:        commandLevel("build"),
		Field:        envField(),
		VariableName: "RUNNER_VAR",
		Prefix:       "RUNNER_",
	}

	assert.ErrorIs(t, err, ErrReservedVariablePrefix)
}

// TestErrVariableNotInAllowlistDetail_Unwrap tests the Unwrap() method
func TestErrVariableNotInAllowlistDetail_Unwrap(t *testing.T) {
	err := &ErrVariableNotInAllowlistDetail{
		Level:           commandLevel("build"),
		SystemVarName:   "SECRET",
		InternalVarName: "sec",
		Allowlist:       []string{},
	}

	assert.ErrorIs(t, err, ErrVariableNotInAllowlist)
}

// TestErrCircularReferenceDetail_Unwrap tests the Unwrap() method
func TestErrCircularReferenceDetail_Unwrap(t *testing.T) {
	err := &ErrCircularReferenceDetail{
		Level:        globalLevel(),
		Field:        varsField(),
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
// The expected values are rebuilt with the legacy expressions. Every case also
// checks that String() equals the rendering of parts(), so the two
// representations cannot drift, and that quoting the parts renders what %q
// rendered for the old string.
func TestLevelAndField_StringMatchesLegacyFormat(t *testing.T) {
	assertRendering := func(t *testing.T, want, got string, parts []errmsg.Part) {
		t.Helper()
		assert.Equal(t, want, got)
		assert.Equal(t, got, errmsg.NewMessage(parts...).String())
		assert.Equal(t, fmt.Sprintf("%q", want), errmsg.NewMessage(errmsg.Quoted(parts...)).String())
	}

	levelCases := []struct {
		name string
		got  Level
		want string
	}{
		{"zero", Level{}, ""},
		{"global", globalLevel(), "global"},
		{"group", groupLevel("deploy"), fmt.Sprintf("group[%s]", "deploy")},
		{"group with quote and backslash", groupLevel(`de"p\loy`), fmt.Sprintf("group[%s]", `de"p\loy`)},
		{"command", commandLevel("build"), fmt.Sprintf("command[%s]", "build")},
		{"template", templateLevel("tpl"), fmt.Sprintf("template[%s]", "tpl")},
	}
	for _, tc := range levelCases {
		t.Run("level_"+tc.name, func(t *testing.T) {
			assertRendering(t, tc.want, tc.got.String(), tc.got.parts())
		})
	}

	// Every key, with and without a name and an index: String(), parts() and
	// Quoted must agree. Only vars is ever built with a name; the pin against
	// the legacy call sites is the constructor table below. A key missing from
	// legacyKeys fails the test.
	legacyKeys := map[fieldKey]string{
		fieldCmd:         "cmd",
		fieldArgs:        "args",
		fieldEnv:         "env",
		fieldEnvVars:     "env_vars",
		fieldEnvImport:   "env_import",
		fieldWorkdir:     "workdir",
		fieldVerifyFiles: "verify_files",
		fieldCmdAllowed:  "cmd_allowed",
		fieldVars:        "vars",
		fieldOutputFile:  "output_file",
	}
	for key := range fieldKeyCount {
		for _, name := range []string{"dest", "", "a\"b\\c\u00e9"} {
			for _, hasName := range []bool{false, true} {
				for _, hasIndex := range []bool{false, true} {
					f := Field{key: key, name: name, hasName: hasName, index: 3, hasIndex: hasIndex}
					var want string
					if key != fieldNone {
						word, ok := legacyKeys[key]
						require.Truef(t, ok, "fieldKey %d has no legacy key word", key)
						switch {
						case hasName && hasIndex:
							want = fmt.Sprintf("%s.%s[%d]", word, name, 3)
						case hasName:
							want = fmt.Sprintf("%s.%s", word, name)
						case hasIndex:
							want = fmt.Sprintf("%s[%d]", word, 3)
						default:
							want = word
						}
					}
					t.Run(fmt.Sprintf("field_%d_%q_name=%t_index=%t", key, name, hasName, hasIndex), func(t *testing.T) {
						assertRendering(t, want, f.String(), f.parts())
					})
				}
			}
		}
	}

	// The constructors build the forms the old call sites rendered.
	fieldCases := []struct {
		name string
		got  Field
		want string
	}{
		{"zero", Field{}, ""},
		{"cmd", cmdField(), "cmd"},
		{"args", argsField(2), fmt.Sprintf("args[%d]", 2)},
		{"args without index", argsFieldNoIndex(), "args"},
		{"env", envField(), "env"},
		{"env_vars", envVarsField(1), fmt.Sprintf("env_vars[%d]", 1)},
		{"env_vars without index", envVarsFieldNoIndex(), "env_vars"},
		{"env_import", envImportField(), "env_import"},
		{"workdir", workdirField(), "workdir"},
		{"output_file", outputFileField(), "output_file"},
		{"verify_files", verifyFilesField(0), fmt.Sprintf("verify_files[%d]", 0)},
		{"cmd_allowed", cmdAllowedField(3), fmt.Sprintf("cmd_allowed[%d]", 3)},
		{"cmd_allowed without index", cmdAllowedFieldNoIndex(), "cmd_allowed"},
		{"vars", varsField(), "vars"},
		{"vars name", varField("dest"), fmt.Sprintf("vars.%s", "dest")},
		{"vars element", varElementField("dest", 1), fmt.Sprintf("vars.%s[%d]", "dest", 1)},
		// A quoted TOML key may be empty; the name is still present.
		{"vars empty name", varField(""), fmt.Sprintf("vars.%s", "")},
		{"vars empty name element", varElementField("", 2), fmt.Sprintf("vars.%s[%d]", "", 2)},
	}
	for _, tc := range fieldCases {
		t.Run("field_"+tc.name, func(t *testing.T) {
			assertRendering(t, tc.want, tc.got.String(), tc.got.parts())
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
		Level:    globalLevel(),
		Field:    varsField(),
		Sequence: "\\q",
		Context:  "test",
	}

	assert.ErrorIs(t, err, ErrInvalidEscapeSequence)
}

// TestErrUnclosedVariableReferenceDetail_Unwrap tests the Unwrap() method
func TestErrUnclosedVariableReferenceDetail_Unwrap(t *testing.T) {
	err := &ErrUnclosedVariableReferenceDetail{
		Level:   commandLevel("build"),
		Field:   varsField(),
		Context: "test",
	}

	assert.ErrorIs(t, err, ErrUnclosedVariableReference)
}

// TestErrMaxRecursionDepthExceededDetail_Unwrap tests the Unwrap() method
func TestErrMaxRecursionDepthExceededDetail_Unwrap(t *testing.T) {
	err := &ErrMaxRecursionDepthExceededDetail{
		Level:    globalLevel(),
		Field:    envField(),
		MaxDepth: 50,
		Context:  "test",
	}

	assert.ErrorIs(t, err, ErrMaxRecursionDepthExceeded)
}

// TestErrInvalidEnvImportFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvImportFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvImportFormatDetail{
		Level:   commandLevel("build"),
		Mapping: "bad",
		Reason:  "test",
	}

	assert.ErrorIs(t, err, ErrInvalidEnvImportFormat)
}

// TestErrInvalidEnvFormatDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvFormatDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvFormatDetail{
		Level:   globalLevel(),
		Mapping: "BAD",
		Reason:  "test",
	}

	assert.ErrorIs(t, err, ErrInvalidEnvFormat)
}

// TestErrInvalidEnvKeyDetail_Unwrap tests the Unwrap() method
func TestErrInvalidEnvKeyDetail_Unwrap(t *testing.T) {
	err := &ErrInvalidEnvKeyDetail{
		Level:   commandLevel("build"),
		Key:     "INVALID",
		Context: "test",
		Reason:  "test reason",
	}

	assert.ErrorIs(t, err, ErrInvalidEnvKey)
}

// TestErrDuplicateVariableDefinitionDetail_Unwrap tests the Unwrap() method
func TestErrDuplicateVariableDefinitionDetail_Unwrap(t *testing.T) {
	err := &ErrDuplicateVariableDefinitionDetail{
		Level:        commandLevel("build"),
		Field:        envField(),
		VariableName: "DUP",
	}

	assert.ErrorIs(t, err, ErrDuplicateVariableDefinition)
}

// TestErrEnvImportVarsConflictDetail_Unwrap tests the Unwrap() method
func TestErrEnvImportVarsConflictDetail_Unwrap(t *testing.T) {
	err := &ErrEnvImportVarsConflictDetail{
		Level:          globalLevel(),
		VariableName:   "CONFLICT_VAR",
		EnvImportLevel: globalLevel(),
		VarsLevel:      globalLevel(),
	}

	assert.ErrorIs(t, err, ErrEnvImportVarsConflict)
}

// TestErrDuplicatePathDetail_Unwrap tests the Unwrap() method
func TestErrDuplicatePathDetail_Unwrap(t *testing.T) {
	err := &ErrDuplicatePathDetail{
		Level:      groupLevel("test"),
		Field:      cmdAllowedFieldNoIndex(),
		Path:       "/bin/sh",
		FirstIndex: 1,
		DupeIndex:  2,
	}

	assert.ErrorIs(t, err, ErrDuplicatePath)
}

// TestErrDuplicateResolvedPathDetail_Unwrap tests the Unwrap() method
func TestErrDuplicateResolvedPathDetail_Unwrap(t *testing.T) {
	err := &ErrDuplicateResolvedPathDetail{
		Level:        groupLevel("test"),
		Field:        cmdAllowedFieldNoIndex(),
		OriginalPath: "/link",
		ResolvedPath: "/target",
	}

	assert.ErrorIs(t, err, ErrDuplicateResolvedPath)
}

// errorTypeCase pins one config error type's structured message against its
// legacy rendering.
type errorTypeCase struct {
	name string
	err  error
	// legacy is the string the old fmt.Sprintf Error() produced.
	legacy string
	// segments are the non-constant segments, in order: appendix A's roles for
	// the value fields, the level and the field. Constants are pinned by
	// legacy.
	segments errmsg.Segments
}

// errorTypeCases builds the shared table. Values that used to be rendered with
// %q carry a quote and a backslash so a non-quoting rewrite is caught.
func errorTypeCases() []errorTypeCase {
	// qName is the raw value; qBody is how strconv.Quote escapes it without the
	// surrounding quotes, which is what a Quoted part keeps on its inner
	// segment.
	const qName = "a\"b\\c"
	const qBody = "a\\\"b\\\\c"

	// rawChain is the raw value that appears unquoted inside an expansion path.
	const rawChain = "a\"b\\c"

	cause := errors.New("wrong scope")

	return []errorTypeCase{
		{
			name:   "ErrInvalidVariableNameDetail",
			err:    &ErrInvalidVariableNameDetail{Level: groupLevel("deploy"), Field: envField(), VariableName: "bad-var", Reason: "must match"},
			legacy: "invalid variable name in group[deploy].env: 'bad-var' (must match)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "bad-var"},
				{Role: errmsg.RoleText, Text: "must match"},
			},
		},
		{
			name:   "ErrInvalidSystemVariableNameDetail",
			err:    &ErrInvalidSystemVariableNameDetail{Level: globalLevel(), Field: envField(), SystemVariableName: "bad-sys", Reason: "bad"},
			legacy: "invalid system variable name in global.env: 'bad-sys' (bad)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleText, Text: "bad-sys"},
				{Role: errmsg.RoleText, Text: "bad"},
			},
		},
		{
			name:   "ErrReservedVariablePrefixDetail",
			err:    &ErrReservedVariablePrefixDetail{Level: groupLevel("deploy"), Field: varsField(), VariableName: "__runner_x", Prefix: "__runner_"},
			legacy: "reserved variable prefix in group[deploy].vars: '__runner_x' (prefix '__runner_' is reserved)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "__runner_x"},
				{Role: errmsg.RoleText, Text: "__runner_"},
			},
		},
		{
			name:   "ErrVariableNotInAllowlistDetail",
			err:    &ErrVariableNotInAllowlistDetail{Level: groupLevel("deploy"), SystemVarName: "GITHUB_TOKEN", InternalVarName: "gh"},
			legacy: "system environment variable 'GITHUB_TOKEN' not in allowlist (referenced as 'gh' in group[deploy].from_env)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "GITHUB_TOKEN"},
				{Role: errmsg.RoleIdentifier, Text: "gh"},
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
			},
		},
		{
			name:   "ErrCircularReferenceDetail",
			err:    &ErrCircularReferenceDetail{Level: groupLevel("deploy"), Field: varsField(), VariableName: "api_key", Chain: []string{"api_key", "token_file", "api_key"}},
			legacy: "circular reference in group[deploy].vars: 'api_key' (chain: [api_key token_file api_key])",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleIdentifier, Text: "api_key"},
				{Role: errmsg.RoleIdentifier, Text: "api_key"},
				{Role: errmsg.RoleIdentifier, Text: "token_file"},
				{Role: errmsg.RoleIdentifier, Text: "api_key"},
			},
		},
		{
			name:   "ErrUndefinedVariableDetail",
			err:    &ErrUndefinedVariableDetail{Level: groupLevel("backup"), Field: varField("dest"), VariableName: "api_key", Context: "raw", Chain: []string{"api_key"}},
			legacy: "undefined variable in group[backup].vars.dest: 'api_key' (context: raw) (expansion path: api_key)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "backup"},
				{Role: errmsg.RoleIdentifier, Text: "dest"},
				{Role: errmsg.RoleIdentifier, Text: "api_key"},
				{Role: errmsg.RoleText, Text: "raw"},
				{Role: errmsg.RoleIdentifier, Text: "api_key"},
			},
		},
		{
			name:   "ErrInvalidEscapeSequenceDetail",
			err:    &ErrInvalidEscapeSequenceDetail{Level: groupLevel("deploy"), Field: envField(), Sequence: "\\q", Context: "ctx"},
			legacy: "invalid escape sequence in group[deploy].env: '\\q' (context: ctx)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "\\q"},
				{Role: errmsg.RoleText, Text: "ctx"},
			},
		},
		{
			name:   "ErrUnclosedVariableReferenceDetail",
			err:    &ErrUnclosedVariableReferenceDetail{Level: commandLevel("build"), Field: varsField(), Context: "ctx"},
			legacy: "unclosed variable reference in command[build].vars: missing closing '}' (context: ctx)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "build"},
				{Role: errmsg.RoleText, Text: "ctx"},
			},
		},
		{
			name:   "ErrMaxRecursionDepthExceededDetail",
			err:    &ErrMaxRecursionDepthExceededDetail{Level: groupLevel("deploy"), Field: varsField(), MaxDepth: 100, Context: "ctx"},
			legacy: "maximum recursion depth exceeded in group[deploy].vars: limit 100 (context: ctx)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "100"},
				{Role: errmsg.RoleText, Text: "ctx"},
			},
		},
		{
			name:   "ErrInvalidEnvImportFormatDetail",
			err:    &ErrInvalidEnvImportFormatDetail{Level: groupLevel("deploy"), Mapping: "bad", Reason: "reason"},
			legacy: "invalid env_import format in group[deploy]: 'bad' (reason)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "bad"},
				{Role: errmsg.RoleText, Text: "reason"},
			},
		},
		{
			name:   "ErrInvalidEnvFormatDetail",
			err:    &ErrInvalidEnvFormatDetail{Level: globalLevel(), Mapping: "BAD", Reason: "reason"},
			legacy: "invalid env format in global: 'BAD' (reason)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleText, Text: "BAD"},
				{Role: errmsg.RoleText, Text: "reason"},
			},
		},
		{
			name:   "ErrInvalidEnvKeyDetail",
			err:    &ErrInvalidEnvKeyDetail{Level: groupLevel("deploy"), Key: "BAD-KEY", Context: "BAD-KEY=v", Reason: "bad"},
			legacy: "invalid environment variable key in group[deploy]: 'BAD-KEY' (context: BAD-KEY=v, reason: bad)",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "BAD-KEY"},
				{Role: errmsg.RoleText, Text: "BAD-KEY=v"},
				{Role: errmsg.RoleText, Text: "bad"},
			},
		},
		{
			name:   "ErrDuplicateVariableDefinitionDetail",
			err:    &ErrDuplicateVariableDefinitionDetail{Level: groupLevel("deploy"), Field: envField(), VariableName: "DUP"},
			legacy: "duplicate variable definition in group[deploy].env: 'DUP' is defined multiple times",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleIdentifier, Text: "DUP"},
			},
		},
		{
			name:   "InvalidPathError",
			err:    &InvalidPathError{Path: "/bad", Reason: "must be absolute"},
			legacy: "invalid path '/bad': must be absolute",
			segments: errmsg.Segments{
				{Role: errmsg.RoleText, Text: "/bad"},
				{Role: errmsg.RoleText, Text: "must be absolute"},
			},
		},
		{
			name:   "ErrDuplicatePathDetail",
			err:    &ErrDuplicatePathDetail{Level: groupLevel("deploy"), Field: cmdAllowedFieldNoIndex(), Path: "/bin/sh", FirstIndex: 1, DupeIndex: 2},
			legacy: "duplicate path in group[deploy].cmd_allowed: '/bin/sh' appears at index 1 and 2",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "/bin/sh"},
				{Role: errmsg.RoleText, Text: "1"},
				{Role: errmsg.RoleText, Text: "2"},
			},
		},
		{
			name:   "ErrDuplicateResolvedPathDetail",
			err:    &ErrDuplicateResolvedPathDetail{Level: groupLevel("deploy"), Field: cmdAllowedFieldNoIndex(), OriginalPath: "/link", ResolvedPath: "/target"},
			legacy: "duplicate resolved path in group[deploy].cmd_allowed: '/link' resolves to '/target' which is already in the list",
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: "/link"},
				{Role: errmsg.RolePath, Text: "/target"},
			},
		},
		{
			name:   "ErrTooManyVariablesDetail",
			err:    &ErrTooManyVariablesDetail{Level: globalLevel(), Count: 5, MaxCount: 1000},
			legacy: "too many variables in global: got 5, max 1000",
			segments: errmsg.Segments{
				{Role: errmsg.RoleText, Text: "5"},
				{Role: errmsg.RoleText, Text: "1000"},
			},
		},
		{
			name:   "ErrTypeMismatchDetail",
			err:    &ErrTypeMismatchDetail{Level: globalLevel(), VariableName: qName, ExpectedType: typeNameArray, ActualType: typeNameString},
			legacy: fmt.Sprintf("variable %q type mismatch in %s: already defined as %s, cannot redefine as %s", qName, globalLevel(), typeNameArray, typeNameString),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: qBody},
				{Role: errmsg.RoleText, Text: typeNameArray},
				{Role: errmsg.RoleText, Text: typeNameString},
			},
		},
		{
			name:   "ErrValueTooLongDetail",
			err:    &ErrValueTooLongDetail{Level: globalLevel(), VariableName: qName, Length: 5, MaxLength: MaxStringValueLen},
			legacy: fmt.Sprintf("variable %q value too long in %s: got %d bytes, max %d", qName, globalLevel(), 5, MaxStringValueLen),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: qBody},
				{Role: errmsg.RoleText, Text: "5"},
				{Role: errmsg.RoleText, Text: "10240"},
			},
		},
		{
			name:   "ErrArrayTooLargeDetail",
			err:    &ErrArrayTooLargeDetail{Level: globalLevel(), VariableName: qName, Count: 3, MaxCount: MaxArrayElements},
			legacy: fmt.Sprintf("variable %q array too large in %s: got %d elements, max %d", qName, globalLevel(), 3, MaxArrayElements),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: qBody},
				{Role: errmsg.RoleText, Text: "3"},
				{Role: errmsg.RoleText, Text: "1000"},
			},
		},
		{
			name:   "ErrInvalidArrayElementDetail",
			err:    &ErrInvalidArrayElementDetail{Level: globalLevel(), VariableName: qName, Index: 2, ExpectedType: typeNameString, ActualType: "int"},
			legacy: fmt.Sprintf("variable %q has invalid array element at index %d in %s: expected %s, got %s", qName, 2, globalLevel(), typeNameString, "int"),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: qBody},
				{Role: errmsg.RoleText, Text: "2"},
				{Role: errmsg.RoleText, Text: typeNameString},
				{Role: errmsg.RoleText, Text: "int"},
			},
		},
		{
			name:   "ErrArrayElementTooLongDetail",
			err:    &ErrArrayElementTooLongDetail{Level: globalLevel(), VariableName: qName, Index: 2, Length: 5, MaxLength: MaxStringValueLen},
			legacy: fmt.Sprintf("variable %q array element %d too long in %s: got %d bytes, max %d", qName, 2, globalLevel(), 5, MaxStringValueLen),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: qBody},
				{Role: errmsg.RoleText, Text: "2"},
				{Role: errmsg.RoleText, Text: "5"},
				{Role: errmsg.RoleText, Text: "10240"},
			},
		},
		{
			name:   "ErrUnsupportedTypeDetail",
			err:    &ErrUnsupportedTypeDetail{Level: globalLevel(), VariableName: qName, ActualType: "int"},
			legacy: fmt.Sprintf("variable %q has unsupported type %s in %s: only string and []string are supported", qName, "int", globalLevel()),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: qBody},
				{Role: errmsg.RoleText, Text: "int"},
			},
		},
		{
			name:   "ErrArrayVariableInStringContextDetail",
			err:    &ErrArrayVariableInStringContextDetail{Level: groupLevel("deploy"), Field: cmdField(), VariableName: qName, Chain: []string{rawChain, "second"}},
			legacy: fmt.Sprintf("cannot reference array variable %q in string context at %s.%s: array variables can only be used where array values are expected (expansion path: %s -> second)", qName, groupLevel("deploy"), cmdField(), rawChain),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: qBody},
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleIdentifier, Text: rawChain},
				{Role: errmsg.RoleIdentifier, Text: "second"},
			},
		},
		{
			name:   "ErrEnvImportVarsConflictDetail",
			err:    &ErrEnvImportVarsConflictDetail{Level: globalLevel(), VariableName: qName, EnvImportLevel: globalLevel(), VarsLevel: groupLevel("g")},
			legacy: fmt.Sprintf("variable %q conflicts between env_import and vars in %s: defined in env_import at %s and vars at %s", qName, globalLevel(), globalLevel(), groupLevel("g")),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: qBody},
				{Role: errmsg.RoleIdentifier, Text: "g"},
			},
		},
		{
			name:   "ErrLocalVariableInTemplate",
			err:    &ErrLocalVariableInTemplate{TemplateName: "t\"x\\y", Field: argsField(0), VariableName: "v\"z\\w"},
			legacy: fmt.Sprintf("template %q field %q: cannot reference local variable %q (templates can only reference global variables starting with uppercase)", "t\"x\\y", argsField(0), "v\"z\\w"),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "t\\\"x\\\\y"},
				{Role: errmsg.RoleText, Text: "0"},
				{Role: errmsg.RoleIdentifier, Text: "v\\\"z\\\\w"},
			},
		},
		{
			name:   "ErrUndefinedGlobalVariableInTemplate",
			err:    &ErrUndefinedGlobalVariableInTemplate{TemplateName: "t\"x\\y", Field: argsField(0), VariableName: "v\"z\\w"},
			legacy: fmt.Sprintf("template %q field %q: global variable %q is not defined in [global.vars]", "t\"x\\y", argsField(0), "v\"z\\w"),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "t\\\"x\\\\y"},
				{Role: errmsg.RoleText, Text: "0"},
				{Role: errmsg.RoleIdentifier, Text: "v\\\"z\\\\w"},
			},
		},
		{
			name:   "ErrInvalidVariableScopeDetail",
			err:    &ErrInvalidVariableScopeDetail{Level: groupLevel("deploy"), Field: varsField(), VariableName: qName, Err: cause},
			legacy: fmt.Sprintf("invalid variable scope in %s.%s: variable %q - %s", groupLevel("deploy"), varsField(), qName, cause),
			segments: errmsg.Segments{
				{Role: errmsg.RoleIdentifier, Text: "deploy"},
				{Role: errmsg.RoleText, Text: qBody},
				{Role: errmsg.RoleText, Text: "wrong scope"},
			},
		},
		{
			name:   "ErrIncludedFileNotFound",
			err:    &ErrIncludedFileNotFound{IncludePath: "inc.toml", ResolvedPath: "/abs/inc.toml", ReferencedFrom: "conf.toml"},
			legacy: "included file not found\n  Include path: inc.toml (as written)\n  Resolved path: /abs/inc.toml\n  Referenced from: conf.toml",
			segments: errmsg.Segments{
				{Role: errmsg.RolePath, Text: "inc.toml"},
				{Role: errmsg.RolePath, Text: "/abs/inc.toml"},
				{Role: errmsg.RolePath, Text: "conf.toml"},
			},
		},
		{
			name:   "ErrTemplateFileInvalidFormat",
			err:    &ErrTemplateFileInvalidFormat{TemplateFile: "/x/t.toml", ParseError: errors.New("toml: bad")},
			legacy: "template file contains invalid fields or sections\n  File: /x/t.toml\n  Template files can only contain 'version' and 'command_templates'\n  Detail: toml: bad",
			segments: errmsg.Segments{
				{Role: errmsg.RolePath, Text: "/x/t.toml"},
				{Role: errmsg.RoleText, Text: "toml: bad"},
			},
		},
	}
}

// nonConstantSegments returns the segments whose role declares a value, in
// order: the constants are pinned by the legacy string.
func nonConstantSegments(m errmsg.Message) errmsg.Segments {
	var out errmsg.Segments
	for _, s := range m.Segments() {
		if s.Role != errmsg.RoleConstant {
			out = append(out, s)
		}
	}
	return out
}

// TestErrorTypes_StructuredMessageSegments pins, for every config error type,
// the order and role of the declared value segments (appendix A).
func TestErrorTypes_StructuredMessageSegments(t *testing.T) {
	for _, tt := range errorTypeCases() {
		t.Run(tt.name, func(t *testing.T) {
			structured, ok := tt.err.(errmsg.Structured)
			require.Truef(t, ok, "%T must implement errmsg.Structured", tt.err)
			assert.Equal(t, tt.segments, nonConstantSegments(structured.StructuredMessage()))
		})
	}
}

// TestErrorTypes_ErrorMessageMatchesLegacyFormat pins that every config error
// type renders exactly what the old fmt.Sprintf Error() produced.
func TestErrorTypes_ErrorMessageMatchesLegacyFormat(t *testing.T) {
	for _, tt := range errorTypeCases() {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.legacy, tt.err.Error())
		})
	}
}

// TestExpandCmdAllowed_ResolvePathCauseKeepsPath pins that a failed
// EvalSymlinks keeps the expanded path as a Path segment (and the *fs.PathError
// reachable), not flattened into text.
func TestExpandCmdAllowed_ResolvePathCauseKeepsPath(t *testing.T) {
	raw := "/nonexistent-cmd-allowed-dir/run.sh"
	_, err := expandCmdAllowed([]string{raw}, map[string]string{}, "deploy")
	require.Error(t, err)

	structured, ok := err.(errmsg.Structured)
	require.True(t, ok, "%T must implement errmsg.Structured", err)

	segments := structured.StructuredMessage().Segments()
	assert.Contains(t, segments, errmsg.Segment{Role: errmsg.RolePath, Text: raw})
	assert.Contains(t, segments, errmsg.Segment{Role: errmsg.RoleIdentifier, Text: "deploy"})

	var pathErr *fs.PathError
	require.True(t, errors.As(err, &pathErr), "the *fs.PathError must stay reachable")

	// The cause must be split (its Op and Path become their own segments), not
	// flattened into one text segment that only repeats pathErr.Error().
	assert.Contains(t, segments, errmsg.Segment{Role: errmsg.RoleText, Text: pathErr.Op})
	for _, s := range segments {
		assert.NotEqual(t, pathErr.Error(), s.Text,
			"the *fs.PathError must not be flattened into a single text segment")
	}

	legacy := fmt.Sprintf("group[deploy] cmd_allowed[0] '%s': failed to resolve path: %s", raw, pathErr)
	assert.Equal(t, legacy, err.Error())
}

// TestErrorType_StructuredCauseIdentifiersSurvive pins that a structured
// cause's Identifier survives through the outer error's StructuredMessage and
// stays reachable through errors.As. Both cause-bearing config types carry an
// unstructured cause in production, so this drives the contract with a
// structured one.
func TestErrorType_StructuredCauseIdentifiersSurvive(t *testing.T) {
	inner := errmsg.NewError(
		errmsg.Const("inner: "),
		errmsg.Ident("api_key"),
		errmsg.Const(": "),
		errmsg.Cause(errors.New("boom")),
	)
	outer := &ErrInvalidVariableScopeDetail{
		Level:        groupLevel("deploy"),
		Field:        varsField(),
		VariableName: "v",
		Err:          inner,
	}

	got := nonConstantSegments(outer.StructuredMessage())
	assert.Contains(t, got, errmsg.Segment{Role: errmsg.RoleIdentifier, Text: "api_key"},
		"the inner Identifier must survive through the outer error")
	assert.Contains(t, got, errmsg.Segment{Role: errmsg.RoleIdentifier, Text: "deploy"})

	var reached *errmsg.Error
	require.True(t, errors.As(outer, &reached), "the structured cause must stay reachable")
	assert.Equal(t, inner, reached)
}
