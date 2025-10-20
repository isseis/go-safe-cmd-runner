// Package config provides tests for the variable expansion functionality.
package config_test

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandString_Basic(t *testing.T) {
	// Test basic variable expansion with %{VAR} syntax
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
		wantErr  bool
	}{
		{
			name:     "single variable expansion",
			input:    "prefix_%{var1}_suffix",
			vars:     map[string]string{"var1": "value1"},
			expected: "prefix_value1_suffix",
			wantErr:  false,
		},
		{
			name:     "variable at start",
			input:    "%{var1}_suffix",
			vars:     map[string]string{"var1": "start"},
			expected: "start_suffix",
			wantErr:  false,
		},
		{
			name:     "variable at end",
			input:    "prefix_%{var1}",
			vars:     map[string]string{"var1": "end"},
			expected: "prefix_end",
			wantErr:  false,
		},
		{
			name:     "variable only",
			input:    "%{var1}",
			vars:     map[string]string{"var1": "only"},
			expected: "only",
			wantErr:  false,
		},
		{
			name:     "no variables",
			input:    "plain text",
			vars:     map[string]string{"var1": "unused"},
			expected: "plain text",
			wantErr:  false,
		},
		{
			name:     "empty string",
			input:    "",
			vars:     map[string]string{"var1": "unused"},
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandString_Multiple(t *testing.T) {
	// Test multiple variable expansions in a single string
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:     "two variables",
			input:    "%{var1}/%{var2}",
			vars:     map[string]string{"var1": "a", "var2": "b"},
			expected: "a/b",
		},
		{
			name:     "three variables",
			input:    "%{var1}/%{var2}/%{var3}",
			vars:     map[string]string{"var1": "x", "var2": "y", "var3": "z"},
			expected: "x/y/z",
		},
		{
			name:     "same variable multiple times",
			input:    "%{var1}_%{var1}_%{var1}",
			vars:     map[string]string{"var1": "repeat"},
			expected: "repeat_repeat_repeat",
		},
		{
			name:     "variables with text",
			input:    "start_%{a}_middle_%{b}_end",
			vars:     map[string]string{"a": "AAA", "b": "BBB"},
			expected: "start_AAA_middle_BBB_end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandString_Nested(t *testing.T) {
	// Test nested variable expansions (variable values containing %{VAR} references)
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:  "two-level nesting",
			input: "%{var2}",
			vars: map[string]string{
				"var1": "x",
				"var2": "%{var1}/y",
			},
			expected: "x/y",
		},
		{
			name:  "three-level nesting",
			input: "%{var3}",
			vars: map[string]string{
				"var1": "x",
				"var2": "%{var1}/y",
				"var3": "%{var2}/z",
			},
			expected: "x/y/z",
		},
		{
			name:  "complex nested expansion",
			input: "%{final}",
			vars: map[string]string{
				"base":  "/opt/app",
				"logs":  "%{base}/logs",
				"temp":  "%{logs}/temp",
				"final": "%{temp}/output.log",
			},
			expected: "/opt/app/logs/temp/output.log",
		},
		{
			name:  "nested with multiple references",
			input: "%{combined}",
			vars: map[string]string{
				"a":        "A",
				"b":        "B",
				"combined": "%{a}_%{b}",
			},
			expected: "A_B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "vars")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandString_UndefinedVariable(t *testing.T) {
	// Test error handling for undefined variables
	tests := []struct {
		name        string
		input       string
		vars        map[string]string
		expectedVar string
	}{
		{
			name:        "undefined variable",
			input:       "%{undefined}",
			vars:        map[string]string{"defined": "value"},
			expectedVar: "undefined",
		},
		{
			name:        "undefined in middle",
			input:       "start_%{missing}_end",
			vars:        map[string]string{},
			expectedVar: "missing",
		},
		{
			name:        "one defined, one undefined",
			input:       "%{defined}/%{undefined}",
			vars:        map[string]string{"defined": "ok"},
			expectedVar: "undefined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")

			require.Error(t, err)
			assert.Empty(t, result)

			var undefinedErr *config.ErrUndefinedVariableDetail
			assert.ErrorAs(t, err, &undefinedErr)
			assert.Equal(t, tt.expectedVar, undefinedErr.VariableName)
			assert.Equal(t, "global", undefinedErr.Level)
			assert.Equal(t, "test_field", undefinedErr.Field)
		})
	}
}

func TestExpandString_CircularReference(t *testing.T) {
	// Test circular reference detection
	tests := []struct {
		name            string
		input           string
		vars            map[string]string
		expectedVarName string
	}{
		{
			name:  "direct self-reference",
			input: "%{A}",
			vars: map[string]string{
				"A": "%{A}",
			},
			expectedVarName: "A",
		},
		{
			name:  "two-variable cycle",
			input: "%{A}",
			vars: map[string]string{
				"A": "%{B}",
				"B": "%{A}",
			},
			expectedVarName: "A",
		},
		{
			name:  "three-variable cycle",
			input: "%{A}",
			vars: map[string]string{
				"A": "%{B}",
				"B": "%{C}",
				"C": "%{A}",
			},
			expectedVarName: "A",
		},
		{
			name:  "cycle with prefix",
			input: "%{B}",
			vars: map[string]string{
				"A": "prefix_%{B}",
				"B": "suffix_%{A}",
			},
			expectedVarName: "B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "vars")

			require.Error(t, err)
			assert.Empty(t, result)

			// Use structured error checking instead of string matching
			assert.ErrorIs(t, err, config.ErrCircularReference)

			var circularErr *config.ErrCircularReferenceDetail
			// Use require.ErrorAs to ensure the typed error value is set for further assertions
			require.ErrorAs(t, err, &circularErr)
			require.NotNil(t, circularErr)
			assert.Equal(t, "global", circularErr.Level)
			assert.Equal(t, "vars", circularErr.Field)
			// Verify the chain is recorded in the error
			assert.NotEmpty(t, circularErr.Chain)
			// Verify the variable name reported matches the expected one from the test case
			assert.Equal(t, tt.expectedVarName, circularErr.VariableName)
		})
	}
}

func TestExpandString_MaxRecursionDepth(t *testing.T) {
	// Test maximum recursion depth limit to prevent stack overflow

	// Create a chain of variables that exceeds MaxRecursionDepth
	// var1 -> var2 -> var3 -> ... -> var101
	vars := make(map[string]string)
	for i := 1; i <= config.MaxRecursionDepth+1; i++ {
		varName := fmt.Sprintf("var%d", i)
		if i < config.MaxRecursionDepth+1 {
			nextVarName := fmt.Sprintf("var%d", i+1)
			vars[varName] = fmt.Sprintf("value_%s", "%{"+nextVarName+"}")
		} else {
			vars[varName] = "final_value"
		}
	}

	result, err := config.ExpandString("%{var1}", vars, "global", "vars")

	require.Error(t, err)
	assert.Empty(t, result)

	// Use structured error checking instead of string matching
	assert.ErrorIs(t, err, config.ErrMaxRecursionDepthExceeded)

	var maxDepthErr *config.ErrMaxRecursionDepthExceededDetail
	assert.ErrorAs(t, err, &maxDepthErr)
	assert.Equal(t, "global", maxDepthErr.Level)
	assert.Equal(t, "vars", maxDepthErr.Field)
	assert.Equal(t, config.MaxRecursionDepth, maxDepthErr.MaxDepth)
	assert.NotEmpty(t, maxDepthErr.Context)
}

func TestExpandString_EscapeSequence(t *testing.T) {
	// Test escape sequence handling
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:     "escape percent",
			input:    `literal \%{var1}`,
			vars:     map[string]string{"var1": "value1"},
			expected: "literal %{var1}",
		},
		{
			name:     "escape backslash",
			input:    `path\\name`,
			vars:     map[string]string{},
			expected: `path\name`,
		},
		{
			name:     "mixed escapes",
			input:    `\%{var1} and \\path`,
			vars:     map[string]string{"var1": "value"},
			expected: `%{var1} and \path`,
		},
		{
			name:     "escape before variable",
			input:    `\\%{var1}`,
			vars:     map[string]string{"var1": "test"},
			expected: `\test`,
		},
		{
			name:     "multiple escapes",
			input:    `\%{a} \%{b} \\c`,
			vars:     map[string]string{"a": "A", "b": "B"},
			expected: `%{a} %{b} \c`,
		},
		{
			name:     "escape and expansion",
			input:    `\%{literal} %{var1}`,
			vars:     map[string]string{"literal": "L", "var1": "expanded"},
			expected: `%{literal} expanded`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandString_InvalidEscape(t *testing.T) {
	// Test invalid escape sequence handling
	tests := []struct {
		name             string
		input            string
		vars             map[string]string
		expectedSequence string
	}{
		{
			name:             "invalid escape x",
			input:            `\xtest`,
			vars:             map[string]string{},
			expectedSequence: `\x`,
		},
		{
			name:             "invalid escape n",
			input:            `\ntest`,
			vars:             map[string]string{},
			expectedSequence: `\n`,
		},
		{
			name:             "invalid escape in middle",
			input:            `prefix_\t_suffix`,
			vars:             map[string]string{},
			expectedSequence: `\t`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")

			require.Error(t, err)
			assert.Empty(t, result)

			var escapeErr *config.ErrInvalidEscapeSequenceDetail
			assert.ErrorAs(t, err, &escapeErr)
			assert.Equal(t, tt.expectedSequence, escapeErr.Sequence)
			assert.Equal(t, "global", escapeErr.Level)
			assert.Equal(t, "test_field", escapeErr.Field)
		})
	}
}

func TestExpandString_UnclosedVariableReference(t *testing.T) {
	// Test unclosed variable reference detection
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unclosed at end",
			input: "prefix_%{var",
		},
		{
			name:  "unclosed in middle",
			input: "start_%{var_middle",
		},
		{
			name:  "only opening brace",
			input: "%{",
		},
		{
			name:  "unclosed with content after",
			input: "%{\var_more text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ExpandString(tt.input, nil, "global", "test_field")

			require.Error(t, err)
			assert.Empty(t, result)

			var unclosedErr *config.ErrUnclosedVariableReferenceDetail
			assert.ErrorAs(t, err, &unclosedErr)
			assert.Equal(t, "global", unclosedErr.Level)
			assert.Equal(t, "test_field", unclosedErr.Field)
			assert.Equal(t, tt.input, unclosedErr.Context)
		})
	}
}

func TestProcessFromEnv_Basic(t *testing.T) {
	// Test basic system env var import
	tests := []struct {
		name      string
		fromEnv   []string
		systemEnv map[string]string
		allowlist []string
		expected  map[string]string
	}{
		{
			name:      "single mapping",
			fromEnv:   []string{"home=HOME"},
			systemEnv: map[string]string{"HOME": "/home/test"},
			allowlist: []string{"HOME"},
			expected:  map[string]string{"home": "/home/test"},
		},
		{
			name:      "multiple mappings",
			fromEnv:   []string{"home=HOME", "user=USER"},
			systemEnv: map[string]string{"HOME": "/home/test", "USER": "testuser"},
			allowlist: []string{"HOME", "USER"},
			expected:  map[string]string{"home": "/home/test", "user": "testuser"},
		},
		{
			name:      "empty fromEnv",
			fromEnv:   []string{},
			systemEnv: map[string]string{"HOME": "/home/test"},
			allowlist: []string{"HOME"},
			expected:  map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessFromEnv(tt.fromEnv, tt.allowlist, tt.systemEnv, "global")

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessFromEnv_NotInAllowlist(t *testing.T) {
	// Test error when system var is not in allowlist
	tests := []struct {
		name      string
		fromEnv   []string
		systemEnv map[string]string
		allowlist []string
	}{
		{
			name:      "secret not in allowlist",
			fromEnv:   []string{"secret=SECRET"},
			systemEnv: map[string]string{"SECRET": "confidential"},
			allowlist: []string{"HOME"},
		},
		{
			name:      "multiple vars one not allowed",
			fromEnv:   []string{"home=HOME", "secret=SECRET"},
			systemEnv: map[string]string{"HOME": "/home/test", "SECRET": "confidential"},
			allowlist: []string{"HOME"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessFromEnv(tt.fromEnv, tt.allowlist, tt.systemEnv, "global")

			require.Error(t, err)
			assert.Nil(t, result)

			var allowlistErr *config.ErrVariableNotInAllowlistDetail
			assert.ErrorAs(t, err, &allowlistErr)
			assert.Equal(t, "global", allowlistErr.Level)
		})
	}
}

func TestProcessFromEnv_SystemVarNotSet(t *testing.T) {
	// Test when system variable is not set (should result in empty string)
	tests := []struct {
		name      string
		fromEnv   []string
		systemEnv map[string]string
		allowlist []string
		expected  map[string]string
	}{
		{
			name:      "missing var returns empty string",
			fromEnv:   []string{"missing=MISSING_VAR"},
			systemEnv: map[string]string{},
			allowlist: []string{"MISSING_VAR"},
			expected:  map[string]string{"missing": ""},
		},
		{
			name:      "partially missing vars",
			fromEnv:   []string{"home=HOME", "missing=MISSING"},
			systemEnv: map[string]string{"HOME": "/home/test"},
			allowlist: []string{"HOME", "MISSING"},
			expected:  map[string]string{"home": "/home/test", "missing": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessFromEnv(tt.fromEnv, tt.allowlist, tt.systemEnv, "global")

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessFromEnv_InvalidInternalName(t *testing.T) {
	// Test invalid internal variable name
	tests := []struct {
		name      string
		fromEnv   []string
		systemEnv map[string]string
		allowlist []string
	}{
		{
			name:      "name starts with number",
			fromEnv:   []string{"123invalid=HOME"},
			systemEnv: map[string]string{"HOME": "/home/test"},
			allowlist: []string{"HOME"},
		},
		{
			name:      "name contains hyphen",
			fromEnv:   []string{"my-var=HOME"},
			systemEnv: map[string]string{"HOME": "/home/test"},
			allowlist: []string{"HOME"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessFromEnv(tt.fromEnv, tt.allowlist, tt.systemEnv, "global")

			require.Error(t, err)
			assert.Nil(t, result)

			var varNameErr *config.ErrInvalidVariableNameDetail
			assert.ErrorAs(t, err, &varNameErr)
			assert.Equal(t, "global", varNameErr.Level)
			assert.Equal(t, "from_env", varNameErr.Field)
		})
	}
}

func TestProcessFromEnv_ReservedPrefix(t *testing.T) {
	// Test reserved prefix error
	tests := []struct {
		name      string
		fromEnv   []string
		systemEnv map[string]string
		allowlist []string
	}{
		{
			name:      "reserved prefix __runner_",
			fromEnv:   []string{"__runner_home=HOME"},
			systemEnv: map[string]string{"HOME": "/home/test"},
			allowlist: []string{"HOME"},
		},
		{
			name:      "reserved prefix in second mapping",
			fromEnv:   []string{"valid=HOME", "__runner_test=USER"},
			systemEnv: map[string]string{"HOME": "/home/test", "USER": "testuser"},
			allowlist: []string{"HOME", "USER"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessFromEnv(tt.fromEnv, tt.allowlist, tt.systemEnv, "global")

			require.Error(t, err)
			assert.Nil(t, result)

			var reservedErr *config.ErrReservedVariableNameDetail
			assert.ErrorAs(t, err, &reservedErr)
			assert.Equal(t, "global", reservedErr.Level)
			assert.Equal(t, "from_env", reservedErr.Field)
			assert.Equal(t, "__runner_", reservedErr.Prefix)
		})
	}
}

func TestProcessFromEnv_DuplicateDefinition(t *testing.T) {
	// Test duplicate variable definitions in from_env
	tests := []struct {
		name      string
		fromEnv   []string
		systemEnv map[string]string
		allowlist []string
	}{
		{
			name:      "duplicate internal variable name",
			fromEnv:   []string{"home=HOME", "home=USER"},
			systemEnv: map[string]string{"HOME": "/home/foo", "USER": "bar"},
			allowlist: []string{"HOME", "USER"},
		},
		{
			name:      "duplicate among three definitions",
			fromEnv:   []string{"var1=VAR1", "var2=VAR2", "var1=VAR3"},
			systemEnv: map[string]string{"VAR1": "value1", "VAR2": "value2", "VAR3": "value3"},
			allowlist: []string{"VAR1", "VAR2", "VAR3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessFromEnv(tt.fromEnv, tt.allowlist, tt.systemEnv, "global")

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, config.ErrDuplicateVariableDefinition, "error should be ErrDuplicateVariableDefinition")

			var detailErr *config.ErrDuplicateVariableDefinitionDetail
			assert.ErrorAs(t, err, &detailErr, "should be ErrDuplicateVariableDefinitionDetail")
			assert.Equal(t, "global", detailErr.Level)
			assert.Equal(t, "from_env", detailErr.Field)
		})
	}
}

func TestProcessFromEnv_InvalidFormat(t *testing.T) {
	// Test invalid format (missing '=', empty key, or invalid system var)
	tests := []struct {
		name        string
		fromEnv     []string
		systemEnv   map[string]string
		allowlist   []string
		expectedErr error
	}{
		{
			name:        "no equals sign",
			fromEnv:     []string{"invalid_format"},
			systemEnv:   map[string]string{"HOME": "/home/test"},
			allowlist:   []string{"HOME"},
			expectedErr: config.ErrInvalidFromEnvFormat,
		},
		{
			name:        "empty internal name",
			fromEnv:     []string{"=HOME"},
			systemEnv:   map[string]string{"HOME": "/home/test"},
			allowlist:   []string{"HOME"},
			expectedErr: config.ErrInvalidFromEnvFormat,
		},
		{
			name:        "multiple equals signs (invalid system var name)",
			fromEnv:     []string{"var=VAR=extra"},
			systemEnv:   map[string]string{"VAR": "value"},
			allowlist:   []string{"VAR=extra"},
			expectedErr: config.ErrInvalidSystemVariableName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessFromEnv(tt.fromEnv, tt.allowlist, tt.systemEnv, "global")

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, tt.expectedErr, "error should be of expected type")

			// For system variable name errors, also check the detail struct
			if tt.expectedErr == config.ErrInvalidSystemVariableName {
				var detailErr *config.ErrInvalidSystemVariableNameDetail
				assert.ErrorAs(t, err, &detailErr, "should be ErrInvalidSystemVariableNameDetail")
				assert.Equal(t, "global", detailErr.Level)
				assert.Equal(t, "from_env", detailErr.Field)
				assert.NotEmpty(t, detailErr.SystemVariableName)
				assert.NotEmpty(t, detailErr.Reason)
			}
		})
	}
}

// TestProcessVars_Basic tests basic variable definitions in vars field
func TestProcessVars_Basic(t *testing.T) {
	vars := []string{"var1=value1", "var2=value2"}
	baseVars := map[string]string{}

	result, err := config.ProcessVars(vars, baseVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "value1", result["var1"])
	assert.Equal(t, "value2", result["var2"])
	assert.Len(t, result, 2)
}

// TestProcessVars_ReferenceBase tests referencing base variables from parent level
func TestProcessVars_ReferenceBase(t *testing.T) {
	vars := []string{"var2=%{var1}/sub"}
	baseVars := map[string]string{"var1": "base"}

	result, err := config.ProcessVars(vars, baseVars, "group")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "base", result["var1"], "base variable should be inherited")
	assert.Equal(t, "base/sub", result["var2"], "new variable should reference base")
	assert.Len(t, result, 2)
}

// TestProcessVars_ReferenceOther tests referencing other variables defined in same vars array
func TestProcessVars_ReferenceOther(t *testing.T) {
	vars := []string{"var1=a", "var2=%{var1}/b", "var3=%{var2}/c"}
	baseVars := map[string]string{}

	result, err := config.ProcessVars(vars, baseVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "a", result["var1"])
	assert.Equal(t, "a/b", result["var2"])
	assert.Equal(t, "a/b/c", result["var3"])
	assert.Len(t, result, 3)
}

// TestProcessVars_CircularReference tests detection of undefined variables due to ordering
// Note: With sequential processing, forward references result in "undefined variable" errors
// since variables are processed in order and can only reference previously defined variables
// or base variables
func TestProcessVars_CircularReference(t *testing.T) {
	tests := []struct {
		name     string
		vars     []string
		baseVars map[string]string
	}{
		{
			name:     "forward reference A->B (B not defined yet)",
			vars:     []string{"A=%{B}", "B=%{A}"},
			baseVars: map[string]string{},
		},
		{
			name:     "forward reference chain",
			vars:     []string{"A=%{B}", "B=%{C}", "C=value"},
			baseVars: map[string]string{},
		},
		{
			name:     "self reference without base",
			vars:     []string{"A=%{A}"},
			baseVars: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessVars(tt.vars, tt.baseVars, "global")

			require.Error(t, err)
			assert.Nil(t, result)

			// Sequential processing results in undefined variable error
			var undefinedErr *config.ErrUndefinedVariableDetail
			assert.ErrorAs(t, err, &undefinedErr)
			assert.Equal(t, "global", undefinedErr.Level)
			assert.Equal(t, "vars", undefinedErr.Field)
		})
	}
}

// TestProcessVars_TrueCircularReference tests true circular reference detection
// This happens when base vars create a cycle that gets expanded
func TestProcessVars_TrueCircularReference(t *testing.T) {
	// Base vars create a circular chain: A -> B -> A
	baseVars := map[string]string{
		"A": "%{B}",
		"B": "%{A}",
	}

	// Try to reference A
	vars := []string{"C=%{A}"}

	result, err := config.ProcessVars(vars, baseVars, "global")

	require.Error(t, err)
	assert.Nil(t, result)

	// Should detect circular reference during expansion
	var circularErr *config.ErrCircularReferenceDetail
	assert.ErrorAs(t, err, &circularErr)
	assert.Equal(t, "global", circularErr.Level)
	assert.Equal(t, "vars", circularErr.Field)
}

// TestProcessVars_SelfReference tests extending a variable with itself
func TestProcessVars_SelfReference(t *testing.T) {
	vars := []string{"path=%{path}:/custom"}
	baseVars := map[string]string{"path": "/usr/bin"}

	result, err := config.ProcessVars(vars, baseVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "/usr/bin:/custom", result["path"])
	assert.Len(t, result, 1)
}

// TestProcessVars_InvalidFormat tests handling of invalid format definitions
func TestProcessVars_DuplicateDefinition(t *testing.T) {
	// Test duplicate variable definitions in vars
	tests := []struct {
		name     string
		vars     []string
		baseVars map[string]string
	}{
		{
			name:     "duplicate variable name",
			vars:     []string{"home=/home/foo", "home=/home/bar"},
			baseVars: map[string]string{},
		},
		{
			name:     "duplicate among three definitions",
			vars:     []string{"var1=value1", "var2=value2", "var1=value3"},
			baseVars: map[string]string{},
		},
		{
			name:     "duplicate with base variable (should be allowed - override)",
			vars:     []string{"existing=new_value"},
			baseVars: map[string]string{"existing": "old_value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessVars(tt.vars, tt.baseVars, "global")

			if tt.name == "duplicate with base variable (should be allowed - override)" {
				// Override of base variable should be allowed
				require.NoError(t, err)
				assert.Equal(t, "new_value", result["existing"])
			} else {
				require.Error(t, err)
				assert.Nil(t, result)
				assert.ErrorIs(t, err, config.ErrDuplicateVariableDefinition, "error should be ErrDuplicateVariableDefinition")

				var detailErr *config.ErrDuplicateVariableDefinitionDetail
				assert.ErrorAs(t, err, &detailErr, "should be ErrDuplicateVariableDefinitionDetail")
				assert.Equal(t, "global", detailErr.Level)
				assert.Equal(t, "vars", detailErr.Field)
			}
		})
	}
}

func TestProcessVars_InvalidFormat(t *testing.T) {
	tests := []struct {
		name string
		vars []string
	}{
		{
			name: "no equals sign",
			vars: []string{"invalid_format"},
		},
		{
			name: "empty value is ok",
			vars: []string{"empty_var="},
		},
		{
			name: "empty key",
			vars: []string{"=value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessVars(tt.vars, map[string]string{}, "global")

			if tt.name == "empty value is ok" {
				require.NoError(t, err)
				assert.Equal(t, "", result["empty_var"])
			} else {
				require.Error(t, err)
				assert.Nil(t, result)
			}
		})
	}
}

// TestProcessVars_InvalidVariableName tests handling of invalid variable names
func TestProcessVars_InvalidVariableName(t *testing.T) {
	tests := []struct {
		name    string
		vars    []string
		varName string
	}{
		{
			name:    "starts with number",
			vars:    []string{"123invalid=value"},
			varName: "123invalid",
		},
		{
			name:    "contains hyphen",
			vars:    []string{"invalid-name=value"},
			varName: "invalid-name",
		},
		{
			name:    "contains space",
			vars:    []string{"invalid name=value"},
			varName: "invalid name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessVars(tt.vars, map[string]string{}, "global")

			require.Error(t, err)
			assert.Nil(t, result)

			var invalidErr *config.ErrInvalidVariableNameDetail
			assert.ErrorAs(t, err, &invalidErr)
			assert.Equal(t, "global", invalidErr.Level)
			assert.Equal(t, "vars", invalidErr.Field)
			assert.Equal(t, tt.varName, invalidErr.VariableName)
		})
	}
}

// TestProcessVars_ReservedPrefix tests handling of reserved variable name prefixes
func TestProcessVars_ReservedPrefix(t *testing.T) {
	vars := []string{"__runner_test=value"}

	result, err := config.ProcessVars(vars, map[string]string{}, "global")

	require.Error(t, err)
	assert.Nil(t, result)

	var reservedErr *config.ErrReservedVariablePrefixDetail
	assert.ErrorAs(t, err, &reservedErr)
	assert.Equal(t, "global", reservedErr.Level)
	assert.Equal(t, "vars", reservedErr.Field)
	assert.Equal(t, "__runner_test", reservedErr.VariableName)
}

// TestProcessVars_ComplexChain tests complex variable reference chains
func TestProcessVars_ComplexChain(t *testing.T) {
	baseVars := map[string]string{
		"home":     "/home/user",
		"app_name": "myapp",
	}

	vars := []string{
		"app_dir=%{home}/%{app_name}",
		"data_dir=%{app_dir}/data",
		"input_dir=%{data_dir}/input",
		"output_dir=%{data_dir}/output",
		"temp_dir=%{input_dir}/temp",
	}

	result, err := config.ProcessVars(vars, baseVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "/home/user", result["home"])
	assert.Equal(t, "myapp", result["app_name"])
	assert.Equal(t, "/home/user/myapp", result["app_dir"])
	assert.Equal(t, "/home/user/myapp/data", result["data_dir"])
	assert.Equal(t, "/home/user/myapp/data/input", result["input_dir"])
	assert.Equal(t, "/home/user/myapp/data/output", result["output_dir"])
	assert.Equal(t, "/home/user/myapp/data/input/temp", result["temp_dir"])
	assert.Len(t, result, 7)
}

// TestProcessVars_UndefinedVariable tests handling of undefined variable references
func TestProcessVars_UndefinedVariable(t *testing.T) {
	vars := []string{"var1=%{undefined_var}"}
	baseVars := map[string]string{}

	result, err := config.ProcessVars(vars, baseVars, "global")

	require.Error(t, err)
	assert.Nil(t, result)

	var undefinedErr *config.ErrUndefinedVariableDetail
	assert.ErrorAs(t, err, &undefinedErr)
	assert.Equal(t, "global", undefinedErr.Level)
	assert.Equal(t, "vars", undefinedErr.Field)
	assert.Equal(t, "undefined_var", undefinedErr.VariableName)
}

// TestProcessVars_EmptyVarsArray tests processing empty vars array
func TestProcessVars_EmptyVarsArray(t *testing.T) {
	vars := []string{}
	baseVars := map[string]string{"existing": "value"}

	result, err := config.ProcessVars(vars, baseVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "value", result["existing"])
	assert.Len(t, result, 1)
}

// TestProcessVars_OverrideBaseVariable tests overriding base variable
func TestProcessVars_OverrideBaseVariable(t *testing.T) {
	vars := []string{"var1=new_value"}
	baseVars := map[string]string{"var1": "old_value"}

	result, err := config.ProcessVars(vars, baseVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "new_value", result["var1"], "should override base variable")
	assert.Len(t, result, 1)
}

// TestProcessVars_MultipleReferences tests multiple variable references in single value
func TestProcessVars_MultipleReferences(t *testing.T) {
	vars := []string{
		"prefix=pre",
		"suffix=suf",
		"combined=%{prefix}_middle_%{suffix}",
	}
	baseVars := map[string]string{}

	result, err := config.ProcessVars(vars, baseVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "pre", result["prefix"])
	assert.Equal(t, "suf", result["suffix"])
	assert.Equal(t, "pre_middle_suf", result["combined"])
	assert.Len(t, result, 3)
}

// TestProcessEnv_Basic tests basic env expansion without internal variables
func TestProcessEnv_Basic(t *testing.T) {
	env := []string{"VAR1=value1", "VAR2=value2"}
	internalVars := map[string]string{}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "value1", result["VAR1"])
	assert.Equal(t, "value2", result["VAR2"])
	assert.Len(t, result, 2)
}

// TestProcessEnv_ReferenceInternalVars tests env expansion with internal variable references
func TestProcessEnv_ReferenceInternalVars(t *testing.T) {
	env := []string{"BASE_DIR=%{app_dir}", "LOG_DIR=%{app_dir}/logs"}
	internalVars := map[string]string{"app_dir": "/opt/myapp"}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "/opt/myapp", result["BASE_DIR"])
	assert.Equal(t, "/opt/myapp/logs", result["LOG_DIR"])
	assert.Len(t, result, 2)
}

// TestProcessEnv_UndefinedInternalVar tests error when referencing undefined internal variable
func TestProcessEnv_UndefinedInternalVar(t *testing.T) {
	env := []string{"BASE_DIR=%{undefined_var}"}
	internalVars := map[string]string{}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.Error(t, err)
	assert.Nil(t, result)

	var undefinedErr *config.ErrUndefinedVariableDetail
	assert.ErrorAs(t, err, &undefinedErr)
	assert.Equal(t, "global", undefinedErr.Level)
	assert.Equal(t, "env", undefinedErr.Field)
	assert.Equal(t, "undefined_var", undefinedErr.VariableName)
}

// TestProcessEnv_InvalidEnvVarName tests error for invalid environment variable name
func TestProcessEnv_InvalidEnvVarName(t *testing.T) {
	env := []string{"123_INVALID=value"}
	internalVars := map[string]string{}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.Error(t, err)
	assert.Nil(t, result)

	var invalidKeyErr *config.ErrInvalidEnvKeyDetail
	assert.ErrorAs(t, err, &invalidKeyErr)
	assert.Equal(t, "global", invalidKeyErr.Level)
	assert.Equal(t, "123_INVALID", invalidKeyErr.Key)
	assert.Equal(t, "123_INVALID=value", invalidKeyErr.Context)
}

// TestProcessEnv_InvalidFormat tests error for invalid env definition format
func TestProcessEnv_DuplicateDefinition(t *testing.T) {
	// Test duplicate environment variable definitions in env
	tests := []struct {
		name         string
		env          []string
		internalVars map[string]string
	}{
		{
			name:         "duplicate env variable name",
			env:          []string{"HOME=/home/foo", "HOME=/home/bar"},
			internalVars: map[string]string{},
		},
		{
			name:         "duplicate among three definitions",
			env:          []string{"VAR1=value1", "VAR2=value2", "VAR1=value3"},
			internalVars: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessEnv(tt.env, tt.internalVars, "global")

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, config.ErrDuplicateVariableDefinition, "error should be ErrDuplicateVariableDefinition")

			var detailErr *config.ErrDuplicateVariableDefinitionDetail
			assert.ErrorAs(t, err, &detailErr, "should be ErrDuplicateVariableDefinitionDetail")
			assert.Equal(t, "global", detailErr.Level)
			assert.Equal(t, "env", detailErr.Field)
		})
	}
}

func TestProcessEnv_InvalidFormat(t *testing.T) {
	env := []string{"INVALID_FORMAT"}
	internalVars := map[string]string{}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.Error(t, err)
	assert.Nil(t, result)

	// Use structured error checking instead of string matching
	assert.ErrorIs(t, err, config.ErrInvalidEnvFormat)

	var detailErr *config.ErrInvalidEnvFormatDetail
	if assert.ErrorAs(t, err, &detailErr) {
		assert.Equal(t, "INVALID_FORMAT", detailErr.Mapping)
		assert.Equal(t, "global", detailErr.Level)
		// Verify the reason contains format requirement information
		assert.NotEmpty(t, detailErr.Reason)
	}
}

// TestProcessEnv_EmptyEnvArray tests processing empty env array
func TestProcessEnv_EmptyEnvArray(t *testing.T) {
	env := []string{}
	internalVars := map[string]string{"app_dir": "/opt/myapp"}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result, 0)
}

// TestProcessEnv_ComplexReferences tests complex internal variable references
func TestProcessEnv_ComplexReferences(t *testing.T) {
	env := []string{
		"APP_DIR=%{home}/%{app_name}",
		"DATA_DIR=%{home}/%{app_name}/data",
		"LOG_DIR=%{home}/%{app_name}/logs",
	}
	internalVars := map[string]string{
		"home":     "/home/user",
		"app_name": "myapp",
	}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "/home/user/myapp", result["APP_DIR"])
	assert.Equal(t, "/home/user/myapp/data", result["DATA_DIR"])
	assert.Equal(t, "/home/user/myapp/logs", result["LOG_DIR"])
	assert.Len(t, result, 3)
}

// TestProcessEnv_NoVariableReferences tests env without any variable references
func TestProcessEnv_NoVariableReferences(t *testing.T) {
	env := []string{"STATIC_VAR=/opt/static", "ANOTHER_VAR=value"}
	internalVars := map[string]string{"unused": "value"}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "/opt/static", result["STATIC_VAR"])
	assert.Equal(t, "value", result["ANOTHER_VAR"])
	assert.Len(t, result, 2)
}

// TestProcessEnv_EscapeSequence tests escape sequences in env values
func TestProcessEnv_EscapeSequence(t *testing.T) {
	env := []string{
		"PATH_WITH_ESCAPED=\\%{home}/path",
		"PATH_WITH_BACKSLASH=%{home}\\\\bin",
	}
	internalVars := map[string]string{"home": "/home/user"}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "%{home}/path", result["PATH_WITH_ESCAPED"])
	assert.Equal(t, "/home/user\\bin", result["PATH_WITH_BACKSLASH"])
	assert.Len(t, result, 2)
}

// Note: env field creates process environment variables, not internal variables.
// Therefore, reserved prefix check (__runner_*) is not applicable to env field.
// Reserved prefix check is only for internal variables (vars, from_env).

// TestProcessEnv_EmptyValue tests env with empty value
func TestProcessEnv_EmptyValue(t *testing.T) {
	env := []string{"EMPTY_VAR=", "ANOTHER_VAR=value"}
	internalVars := map[string]string{}

	result, err := config.ProcessEnv(env, internalVars, "global")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "", result["EMPTY_VAR"])
	assert.Equal(t, "value", result["ANOTHER_VAR"])
	assert.Len(t, result, 2)
}

// TestExpandGlobalConfig_Basic tests basic flow of global config expansion
func TestExpandGlobalConfig_Basic(t *testing.T) {
	// Set up system environment
	t.Setenv("HOME", "/home/testuser")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		FromEnv:      []string{"home=HOME"},
		Vars:         []string{"app_dir=%{home}/app"},
		Env:          []string{"APP_DIR=%{app_dir}"},
		VerifyFiles:  []string{"%{app_dir}/config.toml"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	err := config.ExpandGlobalConfig(global, filter)

	require.NoError(t, err)

	// Check ExpandedVars
	require.NotNil(t, global.ExpandedVars)
	assert.Equal(t, "/home/testuser", global.ExpandedVars["home"])
	assert.Equal(t, "/home/testuser/app", global.ExpandedVars["app_dir"])

	// Check ExpandedEnv
	require.NotNil(t, global.ExpandedEnv)
	assert.Equal(t, "/home/testuser/app", global.ExpandedEnv["APP_DIR"])

	// Check ExpandedVerifyFiles
	require.NotNil(t, global.ExpandedVerifyFiles)
	require.Len(t, global.ExpandedVerifyFiles, 1)
	assert.Equal(t, "/home/testuser/app/config.toml", global.ExpandedVerifyFiles[0])
}

// TestExpandGlobalConfig_NoFromEnv tests expansion when from_env is not defined
func TestExpandGlobalConfig_NoFromEnv(t *testing.T) {
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{},
		Vars:         []string{"app_dir=/opt/myapp"},
		Env:          []string{"APP_DIR=%{app_dir}"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	err := config.ExpandGlobalConfig(global, filter)

	require.NoError(t, err)

	// Check ExpandedVars
	require.NotNil(t, global.ExpandedVars)
	assert.Equal(t, "/opt/myapp", global.ExpandedVars["app_dir"])
	// Auto variables are always present (lowercase only)
	assert.Contains(t, global.ExpandedVars, "__runner_datetime")
	assert.Contains(t, global.ExpandedVars, "__runner_pid")
	assert.Len(t, global.ExpandedVars, 3) // app_dir + 2 auto vars

	// Check ExpandedEnv
	require.NotNil(t, global.ExpandedEnv)
	assert.Equal(t, "/opt/myapp", global.ExpandedEnv["APP_DIR"])
}

// TestExpandGlobalConfig_NoVars tests expansion when vars is not defined
func TestExpandGlobalConfig_NoVars(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"PATH"},
		FromEnv:      []string{"path=PATH"},
		Env:          []string{"PATH=%{path}"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	err := config.ExpandGlobalConfig(global, filter)

	require.NoError(t, err)

	// Check ExpandedVars
	require.NotNil(t, global.ExpandedVars)
	assert.Equal(t, "/usr/bin:/bin", global.ExpandedVars["path"])
	// Auto variables are always present (lowercase only)
	assert.Contains(t, global.ExpandedVars, "__runner_datetime")
	assert.Contains(t, global.ExpandedVars, "__runner_pid")
	assert.Len(t, global.ExpandedVars, 3) // path + 2 auto vars

	// Check ExpandedEnv
	require.NotNil(t, global.ExpandedEnv)
	assert.Equal(t, "/usr/bin:/bin", global.ExpandedEnv["PATH"])
}

// TestExpandGlobalConfig_NoEnv tests expansion when env is not defined
func TestExpandGlobalConfig_NoEnv(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		FromEnv:      []string{"home=HOME"},
		Vars:         []string{"app_dir=%{home}/app"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	err := config.ExpandGlobalConfig(global, filter)

	require.NoError(t, err)

	// Check ExpandedVars
	require.NotNil(t, global.ExpandedVars)
	assert.Equal(t, "/home/testuser", global.ExpandedVars["home"])
	assert.Equal(t, "/home/testuser/app", global.ExpandedVars["app_dir"])

	// Check ExpandedEnv (should be empty)
	require.NotNil(t, global.ExpandedEnv)
	assert.Len(t, global.ExpandedEnv, 0)
}

// TestExpandGlobalConfig_ComplexChain tests complex variable reference chain
func TestExpandGlobalConfig_ComplexChain(t *testing.T) {
	t.Setenv("HOME", "/home/user")
	t.Setenv("LANG", "en_US.UTF-8")

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "LANG"},
		FromEnv:      []string{"home=HOME", "lang=LANG"},
		Vars: []string{
			"base=%{home}/base",
			"app=%{base}/app",
			"data=%{app}/data",
			"logs=%{data}/logs",
		},
		Env: []string{
			"BASE_DIR=%{base}",
			"APP_DIR=%{app}",
			"DATA_DIR=%{data}",
			"LOG_DIR=%{logs}",
			"LANG=%{lang}",
		},
		VerifyFiles: []string{
			"%{app}/config.toml",
			"%{data}/input.txt",
		},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	err := config.ExpandGlobalConfig(global, filter)

	require.NoError(t, err)

	// Check ExpandedVars
	require.NotNil(t, global.ExpandedVars)
	assert.Equal(t, "/home/user", global.ExpandedVars["home"])
	assert.Equal(t, "en_US.UTF-8", global.ExpandedVars["lang"])
	assert.Equal(t, "/home/user/base", global.ExpandedVars["base"])
	assert.Equal(t, "/home/user/base/app", global.ExpandedVars["app"])
	assert.Equal(t, "/home/user/base/app/data", global.ExpandedVars["data"])
	assert.Equal(t, "/home/user/base/app/data/logs", global.ExpandedVars["logs"])

	// Check ExpandedEnv
	require.NotNil(t, global.ExpandedEnv)
	assert.Equal(t, "/home/user/base", global.ExpandedEnv["BASE_DIR"])
	assert.Equal(t, "/home/user/base/app", global.ExpandedEnv["APP_DIR"])
	assert.Equal(t, "/home/user/base/app/data", global.ExpandedEnv["DATA_DIR"])
	assert.Equal(t, "/home/user/base/app/data/logs", global.ExpandedEnv["LOG_DIR"])
	assert.Equal(t, "en_US.UTF-8", global.ExpandedEnv["LANG"])

	// Check ExpandedVerifyFiles
	require.NotNil(t, global.ExpandedVerifyFiles)
	require.Len(t, global.ExpandedVerifyFiles, 2)
	assert.Equal(t, "/home/user/base/app/config.toml", global.ExpandedVerifyFiles[0])
	assert.Equal(t, "/home/user/base/app/data/input.txt", global.ExpandedVerifyFiles[1])
}

// TestExpandGlobalConfig_EmptyFields tests expansion with empty fields
func TestExpandGlobalConfig_EmptyFields(t *testing.T) {
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{},
		FromEnv:      []string{},
		Vars:         []string{},
		Env:          []string{},
		VerifyFiles:  []string{},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	err := config.ExpandGlobalConfig(global, filter)

	require.NoError(t, err)

	// All expanded fields should be empty but not nil (except auto variables)
	require.NotNil(t, global.ExpandedVars)
	// Auto variables are always present even with empty fields (lowercase only)
	assert.Contains(t, global.ExpandedVars, "__runner_datetime")
	assert.Contains(t, global.ExpandedVars, "__runner_pid")
	assert.Len(t, global.ExpandedVars, 2) // 2 auto vars

	require.NotNil(t, global.ExpandedEnv)
	assert.Len(t, global.ExpandedEnv, 0)

	require.NotNil(t, global.ExpandedVerifyFiles)
	assert.Len(t, global.ExpandedVerifyFiles, 0)
}

// TestExpandGroupConfig_InheritFromEnv tests from_env inheritance from Global
func TestExpandGroupConfig_InheritFromEnv(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("PATH", "/usr/bin:/bin")

	// Setup Global config with from_env
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "PATH"},
		FromEnv:      []string{"home=HOME", "path=PATH"},
		Vars:         []string{},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with NO from_env defined (should inherit)
	group := &runnertypes.CommandGroup{
		Name: "inherit_group",
		// FromEnv is nil → should inherit Global.ExpandedVars
		Vars: []string{"config=%{home}/.config"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: group should have inherited from_env variables from global
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"], "home should be inherited from global")
	assert.Equal(t, "/usr/bin:/bin", group.ExpandedVars["path"], "path should be inherited from global")
	assert.Equal(t, "/home/testuser/.config", group.ExpandedVars["config"], "config should reference inherited home")
}

// TestExpandGroupConfig_OverrideFromEnv tests from_env merge behavior with override
// (Changed from Override to Merge: now global.from_env variables are merged with group.from_env)
func TestExpandGroupConfig_OverrideFromEnv(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("CUSTOM_VAR", "/custom/path")

	// Setup Global config
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "CUSTOM_VAR"},
		FromEnv:      []string{"home=HOME"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with explicit from_env (should merge, not override)
	group := &runnertypes.CommandGroup{
		Name:         "override_group",
		EnvAllowlist: []string{"HOME", "CUSTOM_VAR"}, // Now includes HOME to allow merging
		FromEnv:      []string{"custom=CUSTOM_VAR"},
		Vars:         []string{"custom_path=%{custom}/data"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: group should have merged from_env variables
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "/custom/path", group.ExpandedVars["custom"], "custom should come from group's from_env")
	assert.Equal(t, "/custom/path/data", group.ExpandedVars["custom_path"])

	// Important: 'home' from global SHOULD now be available (merge behavior)
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"], "home from global.from_env should be inherited and merged")
}

// TestExpandGroupConfig_EmptyFromEnv tests empty from_env array behavior
// (Changed from Override to Merge: now empty array means inherit global's from_env)
func TestExpandGroupConfig_EmptyFromEnv(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	// Setup Global config
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		FromEnv:      []string{"home=HOME"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with explicit empty from_env array (should inherit global's from_env in merge mode)
	group := &runnertypes.CommandGroup{
		Name:    "empty_group",
		FromEnv: []string{}, // Explicitly empty → should now inherit global's from_env
		Vars:    []string{"static_var=static_value"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: group should inherit from_env variables from global (merge behavior)
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "static_value", group.ExpandedVars["static_var"])

	// 'home' from global should now be available (merge mode inheritance)
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"], "home should be inherited when from_env is explicitly empty")
}

// TestExpandGroupConfig_FromEnvMerge_Addition tests merging by adding new variables
func TestExpandGroupConfig_FromEnvMerge_Addition(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("USER", "testuser")
	t.Setenv("PATH", "/usr/bin:/bin")

	// Setup Global config with from_env
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "USER", "PATH"},
		FromEnv:      []string{"home=HOME", "user=USER"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with additional from_env variables
	group := &runnertypes.CommandGroup{
		Name:    "merge_add_group",
		FromEnv: []string{"path=PATH"}, // Add new variable
		Vars:    []string{"env_info=%{home}:%{user}:%{path}"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: group should have merged from_env variables (global + group)
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"], "home from global should be inherited")
	assert.Equal(t, "testuser", group.ExpandedVars["user"], "user from global should be inherited")
	assert.Equal(t, "/usr/bin:/bin", group.ExpandedVars["path"], "path from group should be added")
	assert.Equal(t, "/home/testuser:testuser:/usr/bin:/bin", group.ExpandedVars["env_info"], "all merged variables should be available")
}

// TestExpandGroupConfig_FromEnvMerge_Override tests merging with override of specific variables
func TestExpandGroupConfig_FromEnvMerge_Override(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("USER", "testuser")
	t.Setenv("LANG", "en_US.UTF-8")

	// Setup Global config with from_env
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "USER", "LANG"},
		FromEnv:      []string{"home=HOME", "user=USER"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with from_env that overrides global
	group := &runnertypes.CommandGroup{
		Name:    "merge_override_group",
		FromEnv: []string{"home=USER", "lang=LANG"}, // Override 'home' with USER value, add 'lang'
		Vars:    []string{"info=%{home}:%{user}:%{lang}"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: group should have merged variables with group override
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "testuser", group.ExpandedVars["home"], "home should be overridden by group's from_env (USER value)")
	assert.Equal(t, "testuser", group.ExpandedVars["user"], "user from global should still be inherited")
	assert.Equal(t, "en_US.UTF-8", group.ExpandedVars["lang"], "lang from group should be added")
	assert.Equal(t, "testuser:testuser:en_US.UTF-8", group.ExpandedVars["info"], "override should take effect")
}

// TestExpandGroupConfig_FromEnvNilInherits tests nil from_env inheritance (existing behavior, should not change)
func TestExpandGroupConfig_FromEnvNilInherits(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("USER", "testuser")

	// Setup Global config with from_env
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "USER"},
		FromEnv:      []string{"home=HOME", "user=USER"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with nil from_env (should inherit)
	group := &runnertypes.CommandGroup{
		Name:    "nil_group",
		FromEnv: nil, // nil → inherit global's from_env
		Vars:    []string{"combined=%{home}:%{user}"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: group should inherit global's from_env variables
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"], "home should be inherited from global")
	assert.Equal(t, "testuser", group.ExpandedVars["user"], "user should be inherited from global")
	assert.Equal(t, "/home/testuser:testuser", group.ExpandedVars["combined"], "both variables should be available")
}

// TestExpandGroupConfig_FromEnvEmptyInherits tests empty from_env array now inherits (new merge behavior)
func TestExpandGroupConfig_FromEnvEmptyInherits(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("USER", "testuser")

	// Setup Global config with from_env
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "USER"},
		FromEnv:      []string{"home=HOME", "user=USER"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with empty from_env array (should now inherit in merge mode)
	group := &runnertypes.CommandGroup{
		Name:    "empty_inherits_group",
		FromEnv: []string{}, // empty [] → now inherit global's from_env (merge behavior)
		Vars:    []string{"combined=%{home}:%{user}"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: group should inherit global's from_env variables (new merge behavior)
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"], "home should be inherited from global")
	assert.Equal(t, "testuser", group.ExpandedVars["user"], "user should be inherited from global")
	assert.Equal(t, "/home/testuser:testuser", group.ExpandedVars["combined"], "both variables should be available")
}

// TestExpandGroupConfig_VarsMerge tests vars merging with global
func TestExpandGroupConfig_VarsMerge(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	// Setup Global config
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		FromEnv:      []string{"home=HOME"},
		Vars:         []string{"app_dir=%{home}/app"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with additional vars
	group := &runnertypes.CommandGroup{
		Name: "merge_group",
		// FromEnv is nil → inherits global.from_env
		Vars: []string{"log_dir=%{app_dir}/logs"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: group should have both global and group vars
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"], "home from global")
	assert.Equal(t, "/home/testuser/app", group.ExpandedVars["app_dir"], "app_dir from global")
	assert.Equal(t, "/home/testuser/app/logs", group.ExpandedVars["log_dir"], "log_dir from group, referencing global vars")
}

// TestExpandGroupConfig_AllowlistInherit tests allowlist inheritance
func TestExpandGroupConfig_AllowlistInherit(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("USER", "testuser")

	// Setup Global config
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "USER"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group without its own allowlist (should inherit global)
	group := &runnertypes.CommandGroup{
		Name: "inherit_allowlist_group",
		// EnvAllowlist is nil → should inherit global
		FromEnv: []string{"home=HOME", "user=USER"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: should succeed because USER is in global allowlist
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "/home/testuser", group.ExpandedVars["home"])
	assert.Equal(t, "testuser", group.ExpandedVars["user"])
}

// TestExpandGroupConfig_AllowlistOverride tests allowlist override
func TestExpandGroupConfig_AllowlistOverride(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	t.Setenv("CUSTOM_VAR", "/custom")

	// Setup Global config
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with its own allowlist (should override global)
	group := &runnertypes.CommandGroup{
		Name:         "override_allowlist_group",
		EnvAllowlist: []string{"CUSTOM_VAR"}, // Override global allowlist
		FromEnv:      []string{"custom=CUSTOM_VAR"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify: should succeed with group's allowlist
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "/custom", group.ExpandedVars["custom"])
}

// TestExpandGroupConfig_WithEnv tests env expansion in group
func TestExpandGroupConfig_WithEnv(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	// Setup Global config
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		FromEnv:      []string{"home=HOME"},
		Vars:         []string{"app_dir=%{home}/app"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with env that references internal vars
	group := &runnertypes.CommandGroup{
		Name: "env_group",
		Vars: []string{"log_dir=%{app_dir}/logs"},
		Env:  []string{"LOG_DIR=%{log_dir}", "APP_DIR=%{app_dir}"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify ExpandedVars
	require.NotNil(t, group.ExpandedVars)
	assert.Equal(t, "/home/testuser/app/logs", group.ExpandedVars["log_dir"])

	// Verify ExpandedEnv
	require.NotNil(t, group.ExpandedEnv)
	assert.Equal(t, "/home/testuser/app/logs", group.ExpandedEnv["LOG_DIR"])
	assert.Equal(t, "/home/testuser/app", group.ExpandedEnv["APP_DIR"])
}

// TestExpandGroupConfig_WithVerifyFiles tests verify_files expansion in group
func TestExpandGroupConfig_WithVerifyFiles(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	// Setup Global config
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		FromEnv:      []string{"home=HOME"},
		Vars:         []string{"app_dir=%{home}/app"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	// Expand global first
	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Setup Group with verify_files that references internal vars
	group := &runnertypes.CommandGroup{
		Name:        "verify_group",
		Vars:        []string{"config_dir=%{app_dir}/config"},
		VerifyFiles: []string{"%{config_dir}/app.toml", "%{app_dir}/script.sh"},
	}

	// Expand group
	err = config.ExpandGroupConfig(group, global, filter)
	require.NoError(t, err)

	// Verify ExpandedVerifyFiles
	require.NotNil(t, group.ExpandedVerifyFiles)
	require.Len(t, group.ExpandedVerifyFiles, 2)
	assert.Equal(t, "/home/testuser/app/config/app.toml", group.ExpandedVerifyFiles[0])
	assert.Equal(t, "/home/testuser/app/script.sh", group.ExpandedVerifyFiles[1])
}

func TestExpandCommandConfig_Basic(t *testing.T) {
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"PATH", "HOME"},
	}
	filter := environment.NewFilter(global.EnvAllowlist)

	group := &runnertypes.CommandGroup{
		Name: "test_group",
		ExpandedVars: map[string]string{
			"log_dir": "/var/log/app",
		},
	}

	cmd := &runnertypes.Command{
		Name: "test_cmd",
		Vars: []string{"temp=%{log_dir}/temp"},
		Env:  []string{"TEMP_DIR=%{temp}"},
		Cmd:  "%{temp}/script.sh",
		Args: []string{"--log", "%{log_dir}"},
	}

	err := config.ExpandCommandConfig(cmd, group, global, filter)
	require.NoError(t, err)

	// Verify ExpandedVars
	assert.Equal(t, "/var/log/app", cmd.ExpandedVars["log_dir"], "log_dir should be inherited from group")
	assert.Equal(t, "/var/log/app/temp", cmd.ExpandedVars["temp"], "temp should be expanded")

	// Verify ExpandedEnv
	assert.Equal(t, "/var/log/app/temp", cmd.ExpandedEnv["TEMP_DIR"], "TEMP_DIR should be expanded")

	// Verify ExpandedCmd
	assert.Equal(t, "/var/log/app/temp/script.sh", cmd.ExpandedCmd, "cmd should be expanded")

	// Verify ExpandedArgs
	require.Len(t, cmd.ExpandedArgs, 2)
	assert.Equal(t, "--log", cmd.ExpandedArgs[0])
	assert.Equal(t, "/var/log/app", cmd.ExpandedArgs[1])
}

func TestExpandCommandConfig_InheritGroupVars(t *testing.T) {
	group := &runnertypes.CommandGroup{
		Name: "test_group",
		ExpandedVars: map[string]string{
			"app_dir":  "/opt/myapp",
			"data_dir": "/opt/myapp/data",
		},
	}

	cmd := &runnertypes.Command{
		Name: "process",
		Cmd:  "/usr/bin/process",
		Args: []string{"--data", "%{data_dir}"},
		Env:  []string{"APP_DIR=%{app_dir}"},
	}

	global := &runnertypes.GlobalConfig{EnvAllowlist: []string{"PATH", "HOME"}}
	filter := environment.NewFilter(global.EnvAllowlist)
	err := config.ExpandCommandConfig(cmd, group, global, filter)
	require.NoError(t, err)

	// Verify inherited vars
	assert.Equal(t, "/opt/myapp", cmd.ExpandedVars["app_dir"])
	assert.Equal(t, "/opt/myapp/data", cmd.ExpandedVars["data_dir"])

	// Verify expansion
	assert.Equal(t, "/usr/bin/process", cmd.ExpandedCmd)
	assert.Equal(t, "/opt/myapp/data", cmd.ExpandedArgs[1])
	assert.Equal(t, "/opt/myapp", cmd.ExpandedEnv["APP_DIR"])
}

func TestExpandCommandConfig_NoVars(t *testing.T) {
	group := &runnertypes.CommandGroup{
		Name: "test_group",
		ExpandedVars: map[string]string{
			"base": "/base",
		},
	}

	cmd := &runnertypes.Command{
		Name: "simple",
		Cmd:  "/bin/echo",
		Args: []string{"hello", "world"},
		Env:  []string{"VAR1=value1"},
	}

	global := &runnertypes.GlobalConfig{EnvAllowlist: []string{"PATH", "HOME"}}
	filter := environment.NewFilter(global.EnvAllowlist)
	err := config.ExpandCommandConfig(cmd, group, global, filter)
	require.NoError(t, err)

	// Verify inherited vars only
	assert.Equal(t, "/base", cmd.ExpandedVars["base"])

	// Verify no expansion needed
	assert.Equal(t, "/bin/echo", cmd.ExpandedCmd)
	assert.Equal(t, []string{"hello", "world"}, cmd.ExpandedArgs)
	assert.Equal(t, "value1", cmd.ExpandedEnv["VAR1"])
}

func TestExpandCommandConfig_CmdExpansion(t *testing.T) {
	group := &runnertypes.CommandGroup{
		Name: "test_group",
		ExpandedVars: map[string]string{
			"bin_dir":   "/usr/local/bin",
			"tool_name": "mytool",
		},
	}

	cmd := &runnertypes.Command{
		Name: "run_tool",
		Cmd:  "%{bin_dir}/%{tool_name}",
		Args: []string{},
	}

	global := &runnertypes.GlobalConfig{EnvAllowlist: []string{"PATH", "HOME"}}
	filter := environment.NewFilter(global.EnvAllowlist)
	err := config.ExpandCommandConfig(cmd, group, global, filter)
	require.NoError(t, err)

	assert.Equal(t, "/usr/local/bin/mytool", cmd.ExpandedCmd)
}

func TestExpandCommandConfig_ArgsExpansion(t *testing.T) {
	group := &runnertypes.CommandGroup{
		Name: "test_group",
		ExpandedVars: map[string]string{
			"input_file": "/data/input.txt",
			"output_dir": "/data/output",
		},
	}

	cmd := &runnertypes.Command{
		Name: "converter",
		Cmd:  "/usr/bin/convert",
		Args: []string{"--input", "%{input_file}", "--output", "%{output_dir}/result.txt"},
	}

	global := &runnertypes.GlobalConfig{EnvAllowlist: []string{"PATH", "HOME"}}
	filter := environment.NewFilter(global.EnvAllowlist)
	err := config.ExpandCommandConfig(cmd, group, global, filter)
	require.NoError(t, err)

	require.Len(t, cmd.ExpandedArgs, 4)
	assert.Equal(t, "--input", cmd.ExpandedArgs[0])
	assert.Equal(t, "/data/input.txt", cmd.ExpandedArgs[1])
	assert.Equal(t, "--output", cmd.ExpandedArgs[2])
	assert.Equal(t, "/data/output/result.txt", cmd.ExpandedArgs[3])
}

func TestExpandCommandConfig_UndefinedVariable(t *testing.T) {
	group := &runnertypes.CommandGroup{
		Name: "test_group",
		ExpandedVars: map[string]string{
			"defined": "value",
		},
	}

	cmd := &runnertypes.Command{
		Name: "fail_cmd",
		Cmd:  "/bin/%{undefined}",
		Args: []string{},
	}

	global := &runnertypes.GlobalConfig{EnvAllowlist: []string{"PATH", "HOME"}}
	filter := environment.NewFilter(global.EnvAllowlist)
	err := config.ExpandCommandConfig(cmd, group, global, filter)
	require.Error(t, err)

	// Use structured error checking instead of string matching
	assert.ErrorIs(t, err, config.ErrUndefinedVariable)

	var detailErr *config.ErrUndefinedVariableDetail
	if assert.ErrorAs(t, err, &detailErr) {
		assert.Equal(t, "undefined", detailErr.VariableName)
		assert.NotEmpty(t, detailErr.Level)
		assert.NotEmpty(t, detailErr.Context)
	}
}

func TestExpandCommandConfig_VarsReferenceError(t *testing.T) {
	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		ExpandedVars: map[string]string{},
	}

	cmd := &runnertypes.Command{
		Name: "fail_cmd",
		Vars: []string{"temp=%{missing}/temp"},
		Cmd:  "/bin/echo",
	}

	global := &runnertypes.GlobalConfig{EnvAllowlist: []string{"PATH", "HOME"}}
	filter := environment.NewFilter(global.EnvAllowlist)
	err := config.ExpandCommandConfig(cmd, group, global, filter)
	require.Error(t, err)

	// Use structured error checking instead of string matching
	assert.ErrorIs(t, err, config.ErrUndefinedVariable)

	var detailErr *config.ErrUndefinedVariableDetail
	if assert.ErrorAs(t, err, &detailErr) {
		assert.Equal(t, "missing", detailErr.VariableName)
		assert.NotEmpty(t, detailErr.Level)
		assert.NotEmpty(t, detailErr.Context)
	}
}

func TestExpandCommandConfig_NilGroup(t *testing.T) {
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"PATH", "HOME"},
	}
	filter := environment.NewFilter(global.EnvAllowlist)

	cmd := &runnertypes.Command{
		Name: "test_cmd",
		Cmd:  "/bin/echo",
	}

	err := config.ExpandCommandConfig(cmd, nil, global, filter)
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrNilGroup)
}

// TestExpandGlobalConfig_WithAutoVariables tests that auto variables are available in Global expansion.
func TestExpandGlobalConfig_WithAutoVariables(t *testing.T) {
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		FromEnv:      []string{},
		Vars:         []string{"log_file=/var/log/app_%{__runner_datetime}.log"},
		Env:          []string{"LOG_FILE=%{log_file}"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	err := config.ExpandGlobalConfig(global, filter)
	require.NoError(t, err)

	// Check that auto variables are set (lowercase only)
	require.Contains(t, global.ExpandedVars, "__runner_datetime")
	require.Contains(t, global.ExpandedVars, "__runner_pid")

	// Check that log_file uses auto variable
	logFile := global.ExpandedVars["log_file"]
	assert.Contains(t, logFile, "/var/log/app_")
	// DatetimeLayout format: YYYYMMDDHHmmSS.msec (18 chars: 14 digits + 1 dot + 3 digits)
	assert.Len(t, logFile, len("/var/log/app_")+18+4) // prefix + datetime (18) + .log

	// Check that env uses expanded log_file
	assert.Equal(t, logFile, global.ExpandedEnv["LOG_FILE"])
}

// TestAutoVariables_CannotBeOverridden tests that auto variables cannot be overridden by user definitions.
func TestAutoVariables_CannotBeOverridden(t *testing.T) {
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		FromEnv:      []string{},
		Vars:         []string{"__runner_datetime=custom_value"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)

	err := config.ExpandGlobalConfig(global, filter)
	require.Error(t, err)

	// Should get reserved variable name error
	var reservedErr *config.ErrReservedVariableNameDetail
	assert.ErrorAs(t, err, &reservedErr)
	assert.Equal(t, "__runner_datetime", reservedErr.VariableName)
}

// TestExpandString_ChainNotModified tests that the expansion chain is not modified
// when circular references are detected. This is a regression test for a bug where
// append() was used without properly creating a new slice, which could modify the
// caller's chain if the backing array had sufficient capacity.
//
// The bug occurs when:
//  1. A string contains multiple variable references: "%{VAR1} %{VAR2}"
//  2. During expansion of VAR1, a chain is built with spare capacity
//  3. During expansion of VAR2 in the same recursion level, append() reuses
//     the backing array from VAR1's chain if capacity allows
//  4. This causes VAR1's chain to be inadvertently modified
func TestExpandString_ChainNotModified(t *testing.T) {
	_ = slog.Default()

	// Create variables where one expands to another, creating a chain with capacity
	// The first variable reference will create a chain like [PREFIX, A]
	// If append() is used incorrectly, the second variable might corrupt this chain
	vars := map[string]string{
		"PREFIX": "start",
		"A":      "%{PREFIX}_a",   // Expands PREFIX, creating chain [PREFIX, A]
		"B":      "%{PREFIX}_b",   // Expands PREFIX, creating chain [PREFIX, B]
		"TEST":   "%{A} and %{B}", // Expands both A and B in sequence
	}

	// Expand TEST which contains two variable references
	result, err := config.ExpandString("%{TEST}", vars, "global", "vars")
	require.NoError(t, err)
	assert.Equal(t, "start_a and start_b", result)

	// Now test with circular reference to expose the chain corruption
	vars2 := map[string]string{
		"X": "%{Y}",
		"Y": "%{X}", // Circular reference
		// First we expand X (which tries to expand Y)
		// If the chain from a previous expansion had spare capacity,
		// append() might reuse it and corrupt the error message
	}

	_, err2 := config.ExpandString("%{X}", vars2, "global", "vars")
	require.Error(t, err2)

	var circErr *config.ErrCircularReferenceDetail
	require.ErrorAs(t, err2, &circErr)

	// Verify the chain is correct and not corrupted
	assert.Equal(t, []string{"X", "Y", "X"}, circErr.Chain)
}

// TestExpandString_ChainIsolation tests that multiple concurrent expansions
// maintain isolated expansion chains without interference.
func TestExpandString_ChainIsolation(t *testing.T) {
	_ = slog.Default()

	// Test case 1: Simple two-level chain
	vars1 := map[string]string{
		"A": "%{B}",
		"B": "%{A}",
	}

	// Test case 2: Three-level chain
	vars2 := map[string]string{
		"X": "%{Y}",
		"Y": "%{Z}",
		"Z": "%{X}",
	}

	// Get errors from both cases
	_, err1 := config.ExpandString("%{A}", vars1, "test1", "field1")
	require.Error(t, err1)
	var circErr1 *config.ErrCircularReferenceDetail
	require.ErrorAs(t, err1, &circErr1)

	_, err2 := config.ExpandString("%{X}", vars2, "test2", "field2")
	require.Error(t, err2)
	var circErr2 *config.ErrCircularReferenceDetail
	require.ErrorAs(t, err2, &circErr2)

	// Verify chains have expected lengths
	assert.Len(t, circErr1.Chain, 3, "Two-variable cycle should have chain: [A, B, A]")
	assert.Len(t, circErr2.Chain, 4, "Three-variable cycle should have chain: [X, Y, Z, X]")

	// Verify chains are completely independent
	assert.Equal(t, []string{"A", "B", "A"}, circErr1.Chain)
	assert.Equal(t, []string{"X", "Y", "Z", "X"}, circErr2.Chain)
}

// TestExpandString_MultipleVariablesInSameString tests the critical case where
// a single string contains multiple variable references at the same recursion level.
// This is the most likely scenario to expose the append() backing array bug.
func TestExpandString_MultipleVariablesInSameString(t *testing.T) {
	_ = slog.Default()

	// Create a scenario where:
	// 1. A string has two variable references: "%{A} %{B}"
	// 2. Both A and B expand to values that require recursion
	// 3. Each expansion builds a chain
	// 4. If append() shares backing arrays, the chains could interfere
	vars := map[string]string{
		"COMMON": "shared",
		"A":      "%{COMMON}_valueA",
		"B":      "%{COMMON}_valueB",
	}

	result, err := config.ExpandString("%{A} and %{B}", vars, "global", "vars")
	require.NoError(t, err)
	assert.Equal(t, "shared_valueA and shared_valueB", result)

	// Now test with a circular reference in a multi-variable string
	// This will expose if the chain from the first variable expansion
	// is corrupted by the second variable expansion
	vars2 := map[string]string{
		"P":  "%{Q}",
		"Q":  "%{P}", // Circular reference
		"OK": "valid",
	}

	// The string "%{OK} %{P}" will:
	// 1. First expand OK successfully, creating chain [OK]
	// 2. Then try to expand P, which will detect circular reference
	// If append() was used incorrectly, the chain for P might be corrupted
	_, err2 := config.ExpandString("%{OK} %{P}", vars2, "global", "vars")
	require.Error(t, err2)

	var circErr *config.ErrCircularReferenceDetail
	require.ErrorAs(t, err2, &circErr)

	// Verify the circular reference chain is correct
	// It should be [P, Q, P], not corrupted by the previous expansion of OK
	assert.Equal(t, []string{"P", "Q", "P"}, circErr.Chain,
		"Chain should only contain P->Q->P cycle, not be corrupted by previous OK expansion")
}

// TestExpandString_BackingArrayBug is a regression test for the critical append() bug
// where multiple variables at the same recursion level could share a backing array.
//
// The bug occurs when:
// 1. A value contains multiple variable references: "prefix_%{A} suffix_%{B}"
// 2. During expansion, expansionChain is passed to both A and B expansions
// 3. If expansionChain has spare capacity, append() reuses the backing array
// 4. This causes the chain created for A to be corrupted when B is expanded
//
// This test creates a scenario that reliably triggers the bug with naive append().
func TestExpandString_BackingArrayBug(t *testing.T) {
	_ = slog.Default()

	// Create a nested structure that will build up a chain with capacity
	// ROOT expands to a value containing both A and B at the same level
	vars := map[string]string{
		"LEVEL1":   "%{LEVEL2}",
		"LEVEL2":   "%{ROOT}",
		"ROOT":     "value_%{A}_and_%{B}", // Both A and B at same recursion level
		"A":        "%{A_NESTED}",
		"A_NESTED": "%{A}", // Circular reference in A
		"B":        "simple_b",
	}

	// Expand LEVEL1, which will create a deep chain before hitting the circular reference
	_, err := config.ExpandString("%{LEVEL1}", vars, "global", "vars")
	require.Error(t, err)

	var circErr *config.ErrCircularReferenceDetail
	require.ErrorAs(t, err, &circErr)

	// The chain should be: [LEVEL1, LEVEL2, ROOT, A, A_NESTED, A]
	// If append() was used incorrectly, the chain might be corrupted by
	// the intermediate expansions at the ROOT level
	expectedChain := []string{"LEVEL1", "LEVEL2", "ROOT", "A", "A_NESTED", "A"}
	assert.Equal(t, expectedChain, circErr.Chain,
		"Chain should show the full path to the circular reference without corruption")
}

// TestExpandGlobal tests the ExpandGlobal function for Spec/Runtime separation
func TestExpandGlobal(t *testing.T) {
	tests := []struct {
		name    string
		spec    *runnertypes.GlobalSpec
		want    *runnertypes.RuntimeGlobal
		wantErr bool
	}{
		{
			name: "basic expansion",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{"PREFIX=/opt"},
				Env:  []string{"PATH=%{PREFIX}/bin"},
			},
			want: &runnertypes.RuntimeGlobal{
				ExpandedVars: map[string]string{
					"PREFIX": "/opt",
				},
				ExpandedEnv: map[string]string{
					"PATH": "/opt/bin",
				},
				ExpandedVerifyFiles: []string{},
			},
			wantErr: false,
		},
		{
			name: "with verify_files",
			spec: &runnertypes.GlobalSpec{
				Vars:        []string{"APP=/opt/app"},
				VerifyFiles: []string{"%{APP}/config.toml", "%{APP}/data.db"},
			},
			want: &runnertypes.RuntimeGlobal{
				ExpandedVars: map[string]string{
					"APP": "/opt/app",
				},
				ExpandedEnv:         map[string]string{},
				ExpandedVerifyFiles: []string{"/opt/app/config.toml", "/opt/app/data.db"},
			},
			wantErr: false,
		},
		{
			name: "undefined variable error",
			spec: &runnertypes.GlobalSpec{
				Env: []string{"PATH=%{UNDEFINED}"},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty spec",
			spec: &runnertypes.GlobalSpec{},
			want: &runnertypes.RuntimeGlobal{
				ExpandedVars:        map[string]string{},
				ExpandedEnv:         map[string]string{},
				ExpandedVerifyFiles: []string{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ExpandGlobal(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandGlobal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				assert.NotNil(t, got)
				assert.Equal(t, tt.spec, got.Spec, "Spec reference should be set")
				assert.Equal(t, tt.want.ExpandedVars, got.ExpandedVars, "ExpandedVars mismatch")
				assert.Equal(t, tt.want.ExpandedEnv, got.ExpandedEnv, "ExpandedEnv mismatch")
				assert.Equal(t, tt.want.ExpandedVerifyFiles, got.ExpandedVerifyFiles, "ExpandedVerifyFiles mismatch")
			}
		})
	}
}

// TestExpandGroup tests the ExpandGroup function for Spec/Runtime separation
func TestExpandGroup(t *testing.T) {
	globalVars := map[string]string{
		"GLOBAL_VAR": "global_value",
		"BASE_DIR":   "/opt/base",
	}

	tests := []struct {
		name    string
		spec    *runnertypes.GroupSpec
		want    *runnertypes.RuntimeGroup
		wantErr bool
	}{
		{
			name: "basic expansion with inheritance",
			spec: &runnertypes.GroupSpec{
				Name: "test-group",
				Vars: []string{"GROUP_VAR=group_value"},
				Env:  []string{"GROUP_ENV=%{GROUP_VAR}"},
			},
			want: &runnertypes.RuntimeGroup{
				ExpandedVars: map[string]string{
					"GLOBAL_VAR": "global_value",
					"BASE_DIR":   "/opt/base",
					"GROUP_VAR":  "group_value",
				},
				ExpandedEnv: map[string]string{
					"GROUP_ENV": "group_value",
				},
				ExpandedVerifyFiles: []string{},
			},
			wantErr: false,
		},
		{
			name: "reference global variables",
			spec: &runnertypes.GroupSpec{
				Name: "test-group",
				Vars: []string{"APP_DIR=%{BASE_DIR}/app"},
				Env:  []string{"PATH=%{APP_DIR}/bin"},
			},
			want: &runnertypes.RuntimeGroup{
				ExpandedVars: map[string]string{
					"GLOBAL_VAR": "global_value",
					"BASE_DIR":   "/opt/base",
					"APP_DIR":    "/opt/base/app",
				},
				ExpandedEnv: map[string]string{
					"PATH": "/opt/base/app/bin",
				},
				ExpandedVerifyFiles: []string{},
			},
			wantErr: false,
		},
		{
			name: "override global variable",
			spec: &runnertypes.GroupSpec{
				Name: "test-group",
				Vars: []string{"BASE_DIR=/custom/base"},
			},
			want: &runnertypes.RuntimeGroup{
				ExpandedVars: map[string]string{
					"GLOBAL_VAR": "global_value",
					"BASE_DIR":   "/custom/base",
				},
				ExpandedEnv:         map[string]string{},
				ExpandedVerifyFiles: []string{},
			},
			wantErr: false,
		},
		{
			name: "with verify_files",
			spec: &runnertypes.GroupSpec{
				Name:        "test-group",
				VerifyFiles: []string{"%{BASE_DIR}/group.conf"},
			},
			want: &runnertypes.RuntimeGroup{
				ExpandedVars: map[string]string{
					"GLOBAL_VAR": "global_value",
					"BASE_DIR":   "/opt/base",
				},
				ExpandedEnv:         map[string]string{},
				ExpandedVerifyFiles: []string{"/opt/base/group.conf"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ExpandGroup(tt.spec, globalVars)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandGroup() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				assert.NotNil(t, got)
				assert.Equal(t, tt.spec, got.Spec, "Spec reference should be set")
				assert.Equal(t, tt.want.ExpandedVars, got.ExpandedVars, "ExpandedVars mismatch")
				assert.Equal(t, tt.want.ExpandedEnv, got.ExpandedEnv, "ExpandedEnv mismatch")
				assert.Equal(t, tt.want.ExpandedVerifyFiles, got.ExpandedVerifyFiles, "ExpandedVerifyFiles mismatch")
				assert.Empty(t, got.Commands, "Commands should be empty (not expanded by ExpandGroup)")
			}
		})
	}
}

// TestExpandCommand tests the ExpandCommand function for Spec/Runtime separation
func TestExpandCommand(t *testing.T) {
	groupVars := map[string]string{
		"GROUP_VAR": "group_value",
		"BUILD_DIR": "/tmp/build",
	}

	tests := []struct {
		name      string
		spec      *runnertypes.CommandSpec
		groupName string
		want      *runnertypes.RuntimeCommand
		wantErr   bool
	}{
		{
			name: "basic command expansion",
			spec: &runnertypes.CommandSpec{
				Name: "test-cmd",
				Cmd:  "/usr/bin/make",
				Args: []string{"test", "-C", "%{BUILD_DIR}"},
			},
			groupName: "test-group",
			want: &runnertypes.RuntimeCommand{
				ExpandedCmd:  "/usr/bin/make",
				ExpandedArgs: []string{"test", "-C", "/tmp/build"},
				ExpandedVars: map[string]string{
					"GROUP_VAR": "group_value",
					"BUILD_DIR": "/tmp/build",
				},
				ExpandedEnv: map[string]string{},
			},
			wantErr: false,
		},
		{
			name: "command with vars and env",
			spec: &runnertypes.CommandSpec{
				Name: "test-cmd",
				Cmd:  "/bin/echo",
				Args: []string{"%{OUTPUT}"},
				Vars: []string{"OUTPUT=%{BUILD_DIR}/output.txt"},
				Env:  []string{"LOG_FILE=%{OUTPUT}"},
			},
			groupName: "test-group",
			want: &runnertypes.RuntimeCommand{
				ExpandedCmd:  "/bin/echo",
				ExpandedArgs: []string{"/tmp/build/output.txt"},
				ExpandedVars: map[string]string{
					"GROUP_VAR": "group_value",
					"BUILD_DIR": "/tmp/build",
					"OUTPUT":    "/tmp/build/output.txt",
				},
				ExpandedEnv: map[string]string{
					"LOG_FILE": "/tmp/build/output.txt",
				},
			},
			wantErr: false,
		},
		{
			name: "command with variable in cmd",
			spec: &runnertypes.CommandSpec{
				Name: "test-cmd",
				Cmd:  "%{BUILD_DIR}/custom-tool",
				Args: []string{"arg1"},
			},
			groupName: "test-group",
			want: &runnertypes.RuntimeCommand{
				ExpandedCmd:  "/tmp/build/custom-tool",
				ExpandedArgs: []string{"arg1"},
				ExpandedVars: map[string]string{
					"GROUP_VAR": "group_value",
					"BUILD_DIR": "/tmp/build",
				},
				ExpandedEnv: map[string]string{},
			},
			wantErr: false,
		},
		{
			name: "undefined variable in args",
			spec: &runnertypes.CommandSpec{
				Name: "test-cmd",
				Cmd:  "/bin/echo",
				Args: []string{"%{UNDEFINED}"},
			},
			groupName: "test-group",
			want:      nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ExpandCommand(tt.spec, groupVars, tt.groupName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				assert.NotNil(t, got)
				assert.Equal(t, tt.spec, got.Spec, "Spec reference should be set")
				assert.Equal(t, tt.want.ExpandedCmd, got.ExpandedCmd, "ExpandedCmd mismatch")
				assert.Equal(t, tt.want.ExpandedArgs, got.ExpandedArgs, "ExpandedArgs mismatch")
				assert.Equal(t, tt.want.ExpandedVars, got.ExpandedVars, "ExpandedVars mismatch")
				assert.Equal(t, tt.want.ExpandedEnv, got.ExpandedEnv, "ExpandedEnv mismatch")
				// EffectiveWorkDir and EffectiveTimeout are not set by ExpandCommand
				assert.Equal(t, "", got.EffectiveWorkDir, "EffectiveWorkDir should not be set")
				assert.Equal(t, 0, got.EffectiveTimeout, "EffectiveTimeout should not be set")
			}
		})
	}
}
