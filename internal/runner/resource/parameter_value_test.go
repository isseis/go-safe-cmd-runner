//go:build test

package resource

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStringMapValue_String_SpaceEscaping tests that space characters are escaped
// to avoid ambiguity with delimiters in environment variable output
func TestStringMapValue_String_SpaceEscaping(t *testing.T) {
	tests := []struct {
		name     string
		envMap   map[string]string
		expected string
	}{
		{
			name: "single variable with spaces",
			envMap: map[string]string{
				"MESSAGE": "hello world",
			},
			expected: `MESSAGE=hello\x20world`,
		},
		{
			name: "multiple variables with spaces",
			envMap: map[string]string{
				"FIRST":  "value one",
				"SECOND": "value two",
			},
			// Keys are sorted alphabetically
			expected: `FIRST=value\x20one SECOND=value\x20two`,
		},
		{
			name: "variable with multiple consecutive spaces",
			envMap: map[string]string{
				"VAR": "a  b   c",
			},
			expected: `VAR=a\x20\x20b\x20\x20\x20c`,
		},
		{
			name: "variable with leading and trailing spaces",
			envMap: map[string]string{
				"VAR": " value ",
			},
			expected: `VAR=\x20value\x20`,
		},
		{
			name: "variable with only spaces",
			envMap: map[string]string{
				"VAR": "   ",
			},
			expected: `VAR=\x20\x20\x20`,
		},
		{
			name: "PATH-like value with colons (no spaces)",
			envMap: map[string]string{
				"PATH": "/usr/bin:/bin:/usr/local/bin",
			},
			expected: `PATH=/usr/bin:/bin:/usr/local/bin`,
		},
		{
			name: "mixed: spaces and special characters",
			envMap: map[string]string{
				"CONFIG": "key=value option=test",
			},
			expected: `CONFIG=key=value\x20option=test`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envValue := NewStringMapValue(tt.envMap)
			result := envValue.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStringMapValue_String_ControlCharacterEscaping tests that control characters
// are properly escaped in environment variable values
func TestStringMapValue_String_ControlCharacterEscaping(t *testing.T) {
	tests := []struct {
		name     string
		envMap   map[string]string
		expected string
	}{
		{
			name: "newline escaping",
			envMap: map[string]string{
				"VAR": "line1\nline2",
			},
			expected: `VAR=line1\nline2`,
		},
		{
			name: "tab escaping",
			envMap: map[string]string{
				"VAR": "col1\tcol2",
			},
			expected: `VAR=col1\tcol2`,
		},
		{
			name: "carriage return escaping",
			envMap: map[string]string{
				"VAR": "text\rmore",
			},
			expected: `VAR=text\rmore`,
		},
		{
			name: "null byte escaping",
			envMap: map[string]string{
				"VAR": "text\x00null",
			},
			expected: `VAR=text\x00null`,
		},
		{
			name: "escape sequence (ANSI color) escaping",
			envMap: map[string]string{
				"VAR": "\x1b[31mred\x1b[0m",
			},
			expected: `VAR=\x1b[31mred\x1b[0m`,
		},
		{
			name: "bell character escaping",
			envMap: map[string]string{
				"VAR": "text\abell",
			},
			expected: `VAR=text\abell`,
		},
		{
			name: "backspace escaping",
			envMap: map[string]string{
				"VAR": "text\bback",
			},
			expected: `VAR=text\bback`,
		},
		{
			name: "form feed escaping",
			envMap: map[string]string{
				"VAR": "text\fform",
			},
			expected: `VAR=text\fform`,
		},
		{
			name: "vertical tab escaping",
			envMap: map[string]string{
				"VAR": "text\vvert",
			},
			expected: `VAR=text\vvert`,
		},
		{
			name: "mixed control characters and spaces",
			envMap: map[string]string{
				"VAR": "hello world\nline2\ttab",
			},
			expected: `VAR=hello\x20world\nline2\ttab`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envValue := NewStringMapValue(tt.envMap)
			result := envValue.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStringMapValue_String_NormalCharacters tests that normal printable characters
// (except space) are not escaped
func TestStringMapValue_String_NormalCharacters(t *testing.T) {
	tests := []struct {
		name     string
		envMap   map[string]string
		expected string
	}{
		{
			name: "alphanumeric characters",
			envMap: map[string]string{
				"VAR": "abc123XYZ",
			},
			expected: `VAR=abc123XYZ`,
		},
		{
			name: "special symbols",
			envMap: map[string]string{
				"VAR": "!@#$%^&*()_+-=[]{}|;:,.<>?",
			},
			expected: `VAR=!@#$%^&*()_+-=[]{}|;:,.<>?`,
		},
		{
			name: "quotes (single and double)",
			envMap: map[string]string{
				"VAR": `value "with" 'quotes'`,
			},
			expected: `VAR=value\x20"with"\x20'quotes'`,
		},
		{
			name: "unicode characters",
			envMap: map[string]string{
				"VAR": "こんにちは世界",
			},
			expected: `VAR=こんにちは世界`,
		},
		{
			name: "mixed unicode and ASCII",
			envMap: map[string]string{
				"VAR": "Hello こんにちは 123",
			},
			expected: `VAR=Hello\x20こんにちは\x20123`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envValue := NewStringMapValue(tt.envMap)
			result := envValue.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStringMapValue_String_EmptyMap tests empty environment map
func TestStringMapValue_String_EmptyMap(t *testing.T) {
	envValue := NewStringMapValue(map[string]string{})
	result := envValue.String()
	assert.Equal(t, "", result)
}

// TestStringMapValue_String_SortedOutput tests that keys are sorted alphabetically
func TestStringMapValue_String_SortedOutput(t *testing.T) {
	envMap := map[string]string{
		"ZEBRA": "z",
		"ALPHA": "a",
		"MIKE":  "m",
		"BRAVO": "b",
	}

	envValue := NewStringMapValue(envMap)
	result := envValue.String()

	// Keys should be sorted alphabetically
	expected := "ALPHA=a BRAVO=b MIKE=m ZEBRA=z"
	assert.Equal(t, expected, result)
}

// TestStringMapValue_JSONMarshaling tests that JSON marshaling preserves original values
func TestStringMapValue_JSONMarshaling(t *testing.T) {
	envMap := map[string]string{
		"VAR_WITH_SPACE":   "hello world",
		"VAR_WITH_NEWLINE": "line1\nline2",
		"VAR_NORMAL":       "normal_value",
	}

	envValue := NewStringMapValue(envMap)

	// Marshal to JSON
	jsonBytes, err := json.Marshal(envValue)
	require.NoError(t, err)

	// Unmarshal back
	var unmarshaled map[string]string
	err = json.Unmarshal(jsonBytes, &unmarshaled)
	require.NoError(t, err)

	// Values should be preserved without escaping in JSON
	assert.Equal(t, "hello world", unmarshaled["VAR_WITH_SPACE"], "Space should not be escaped in JSON")
	assert.Equal(t, "line1\nline2", unmarshaled["VAR_WITH_NEWLINE"], "Newline should not be escaped in JSON")
	assert.Equal(t, "normal_value", unmarshaled["VAR_NORMAL"])
}

// TestStringMapValue_Value tests that Value() returns the original map
func TestStringMapValue_Value(t *testing.T) {
	envMap := map[string]string{
		"KEY1": "value with spaces",
		"KEY2": "value\nwith\nnewlines",
	}

	envValue := NewStringMapValue(envMap)
	result := envValue.Value()

	// Value() should return the original map without any escaping
	resultMap, ok := result.(map[string]string)
	require.True(t, ok, "Value() should return map[string]string")
	assert.Equal(t, envMap, resultMap)
}

// TestParametersMap_JSONRoundtrip tests that ParametersMap can be marshaled and unmarshaled
func TestParametersMap_JSONRoundtrip(t *testing.T) {
	original := ParametersMap{
		"string_value": NewStringValue("test"),
		"int_value":    NewIntValue(42),
		"bool_value":   NewBoolValue(true),
		"float_value":  NewFloatValue(3.14),
		"env_value": NewStringMapValue(map[string]string{
			"VAR": "value with spaces",
		}),
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var restored ParametersMap
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	// Check values
	assert.Equal(t, "test", restored["string_value"].Value())
	assert.Equal(t, int64(42), restored["int_value"].Value())
	assert.Equal(t, true, restored["bool_value"].Value())
	assert.Equal(t, 3.14, restored["float_value"].Value())

	// Environment value should be restored correctly
	envMap, ok := restored["env_value"].Value().(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "value with spaces", envMap["VAR"], "Space should be preserved in JSON roundtrip")
}

// TestAnyToParameterValue_EmptyMap tests that empty map[string]any with all string values
// is converted to EnvironmentValue (not AnyValue) for consistent formatting
func TestAnyToParameterValue_EmptyMap(t *testing.T) {
	// Empty map[string]any should be treated as EnvironmentValue
	emptyMap := map[string]any{}
	result := anyToParameterValue(emptyMap)

	// Verify it's an EnvironmentValue (not AnyValue)
	envValue, ok := result.(StringMapValue)
	require.True(t, ok, "Empty map[string]any should be converted to EnvironmentValue")

	// Verify it formats as empty string (not "map[]")
	assert.Equal(t, "", envValue.String(), "Empty EnvironmentValue should format as empty string")

	// Verify the underlying value is an empty map
	valueMap, ok := envValue.Value().(map[string]string)
	require.True(t, ok, "Value() should return map[string]string")
	assert.Empty(t, valueMap, "Underlying map should be empty")
}

// TestStringMapValue_String_RealWorldExample tests real-world environment variable scenarios
func TestStringMapValue_String_RealWorldExample(t *testing.T) {
	tests := []struct {
		name     string
		envMap   map[string]string
		expected string
	}{
		{
			name: "PATH variable with multiple directories",
			envMap: map[string]string{
				"PATH": "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
			},
			expected: `PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin`,
		},
		{
			name: "shell command with options",
			envMap: map[string]string{
				"COMMAND": "docker run --rm -v /data:/data image:latest",
			},
			expected: `COMMAND=docker\x20run\x20--rm\x20-v\x20/data:/data\x20image:latest`,
		},
		{
			name: "multiple variables in actual use",
			envMap: map[string]string{
				"HOME":    "/home/user",
				"PATH":    "/usr/bin:/bin",
				"LANG":    "en_US.UTF-8",
				"COMMAND": "ls -la",
			},
			// Keys are sorted alphabetically
			expected: `COMMAND=ls\x20-la HOME=/home/user LANG=en_US.UTF-8 PATH=/usr/bin:/bin`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envValue := NewStringMapValue(tt.envMap)
			result := envValue.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStringSliceValue_String tests the String() method for various cases
func TestStringSliceValue_String(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		expected string
	}{
		{
			name:     "empty slice",
			slice:    []string{},
			expected: "[]",
		},
		{
			name:     "single element",
			slice:    []string{"hello"},
			expected: `["hello"]`,
		},
		{
			name:     "multiple elements",
			slice:    []string{"arg1", "arg2", "arg3"},
			expected: `["arg1", "arg2", "arg3"]`,
		},
		{
			name:     "elements with spaces",
			slice:    []string{"hello world", "foo bar"},
			expected: `["hello world", "foo bar"]`,
		},
		{
			name:     "elements with special characters",
			slice:    []string{"arg=value", "--option=test"},
			expected: `["arg=value", "--option=test"]`,
		},
		{
			name:     "elements with quotes",
			slice:    []string{`"quoted"`, `'single'`},
			expected: `["\"quoted\"", "'single'"]`,
		},
		{
			name:     "elements with newlines",
			slice:    []string{"line1\nline2", "text"},
			expected: `["line1\nline2", "text"]`,
		},
		{
			name:     "elements with tabs",
			slice:    []string{"col1\tcol2", "data"},
			expected: `["col1\tcol2", "data"]`,
		},
		{
			name:     "real-world command arguments",
			slice:    []string{"-la", "/home/user"},
			expected: `["-la", "/home/user"]`,
		},
		{
			name:     "variable expansion result",
			slice:    []string{"DateTime=20251208033257.063", "PID=546452"},
			expected: `["DateTime=20251208033257.063", "PID=546452"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sliceValue := NewStringSliceValue(tt.slice)
			result := sliceValue.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStringSliceValue_Value tests that Value() returns the original slice
func TestStringSliceValue_Value(t *testing.T) {
	slice := []string{"arg1", "arg2", "arg3"}
	sliceValue := NewStringSliceValue(slice)
	result := sliceValue.Value()

	// Value() should return the original slice
	resultSlice, ok := result.([]string)
	require.True(t, ok, "Value() should return []string")
	assert.Equal(t, slice, resultSlice)
}

// TestStringSliceValue_JSONMarshaling tests JSON marshaling of StringSliceValue
func TestStringSliceValue_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		expected string
	}{
		{
			name:     "empty slice",
			slice:    []string{},
			expected: `[]`,
		},
		{
			name:     "single element",
			slice:    []string{"hello"},
			expected: `["hello"]`,
		},
		{
			name:     "multiple elements",
			slice:    []string{"arg1", "arg2", "arg3"},
			expected: `["arg1","arg2","arg3"]`,
		},
		{
			name:     "elements with spaces",
			slice:    []string{"hello world", "foo bar"},
			expected: `["hello world","foo bar"]`,
		},
		{
			name:     "elements with special characters",
			slice:    []string{"arg=value", "--option=test"},
			expected: `["arg=value","--option=test"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sliceValue := NewStringSliceValue(tt.slice)

			// Marshal to JSON
			jsonBytes, err := json.Marshal(sliceValue)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(jsonBytes))

			// Unmarshal back
			var unmarshaled []string
			err = json.Unmarshal(jsonBytes, &unmarshaled)
			require.NoError(t, err)
			assert.Equal(t, tt.slice, unmarshaled)
		})
	}
}

// TestAnyToParameterValue_StringSlice tests conversion of []any to StringSliceValue
func TestAnyToParameterValue_StringSlice(t *testing.T) {
	tests := []struct {
		name          string
		input         any
		expectedType  string
		expectedValue any
		expectedStr   string
	}{
		{
			name:          "empty slice",
			input:         []any{},
			expectedType:  "StringSliceValue",
			expectedValue: []string{},
			expectedStr:   "[]",
		},
		{
			name:          "single string element",
			input:         []any{"hello"},
			expectedType:  "StringSliceValue",
			expectedValue: []string{"hello"},
			expectedStr:   `["hello"]`,
		},
		{
			name:          "multiple string elements",
			input:         []any{"arg1", "arg2", "arg3"},
			expectedType:  "StringSliceValue",
			expectedValue: []string{"arg1", "arg2", "arg3"},
			expectedStr:   `["arg1", "arg2", "arg3"]`,
		},
		{
			name:          "mixed types - strings and integers",
			input:         []any{"arg1", 42, "arg3"},
			expectedType:  "AnyValue",
			expectedValue: []any{"arg1", 42, "arg3"},
			expectedStr:   "[arg1 42 arg3]",
		},
		{
			name:          "mixed types - strings and booleans",
			input:         []any{"true", true, "false"},
			expectedType:  "AnyValue",
			expectedValue: []any{"true", true, "false"},
			expectedStr:   "[true true false]",
		},
		{
			name:          "mixed types - strings and floats",
			input:         []any{"3.14", 3.14, "2.71"},
			expectedType:  "AnyValue",
			expectedValue: []any{"3.14", 3.14, "2.71"},
			expectedStr:   "[3.14 3.14 2.71]",
		},
		{
			name:          "nested slice (should be AnyValue)",
			input:         []any{[]any{"nested"}},
			expectedType:  "AnyValue",
			expectedValue: []any{[]any{"nested"}},
			expectedStr:   "[[nested]]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := anyToParameterValue(tt.input)

			// Check type
			switch tt.expectedType {
			case "StringSliceValue":
				sliceValue, ok := result.(StringSliceValue)
				require.True(t, ok, "Expected StringSliceValue but got %T", result)
				assert.Equal(t, tt.expectedValue, sliceValue.Value())
				assert.Equal(t, tt.expectedStr, sliceValue.String())
			case "AnyValue":
				anyValue, ok := result.(AnyValue)
				require.True(t, ok, "Expected AnyValue but got %T", result)
				assert.Equal(t, tt.expectedValue, anyValue.Value())
				assert.Equal(t, tt.expectedStr, anyValue.String())
			default:
				t.Fatalf("Unknown expected type: %s", tt.expectedType)
			}
		})
	}
}

// TestParametersMap_JSONRoundtrip_WithStringSlice tests that ParametersMap with StringSliceValue
// can be marshaled and unmarshaled correctly
func TestParametersMap_JSONRoundtrip_WithStringSlice(t *testing.T) {
	original := ParametersMap{
		"command": NewStringValue("/bin/echo"),
		"args":    NewStringSliceValue([]string{"arg1", "arg2", "arg3"}),
		"count":   NewIntValue(3),
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var restored ParametersMap
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	// Check values
	assert.Equal(t, "/bin/echo", restored["command"].Value())
	assert.Equal(t, int64(3), restored["count"].Value())

	// Args should be restored as []any (which will become StringSliceValue)
	args := restored["args"].Value()
	argsSlice, ok := args.([]string)
	require.True(t, ok, "args should be []string, got %T", args)
	assert.Equal(t, []string{"arg1", "arg2", "arg3"}, argsSlice)
}
