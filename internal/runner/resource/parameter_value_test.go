//go:build test

package resource

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvironmentValue_String_SpaceEscaping tests that space characters are escaped
// to avoid ambiguity with delimiters in environment variable output
func TestEnvironmentValue_String_SpaceEscaping(t *testing.T) {
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
			envValue := NewEnvironmentValue(tt.envMap)
			result := envValue.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEnvironmentValue_String_ControlCharacterEscaping tests that control characters
// are properly escaped in environment variable values
func TestEnvironmentValue_String_ControlCharacterEscaping(t *testing.T) {
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
			envValue := NewEnvironmentValue(tt.envMap)
			result := envValue.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEnvironmentValue_String_NormalCharacters tests that normal printable characters
// (except space) are not escaped
func TestEnvironmentValue_String_NormalCharacters(t *testing.T) {
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
			envValue := NewEnvironmentValue(tt.envMap)
			result := envValue.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEnvironmentValue_String_EmptyMap tests empty environment map
func TestEnvironmentValue_String_EmptyMap(t *testing.T) {
	envValue := NewEnvironmentValue(map[string]string{})
	result := envValue.String()
	assert.Equal(t, "", result)
}

// TestEnvironmentValue_String_SortedOutput tests that keys are sorted alphabetically
func TestEnvironmentValue_String_SortedOutput(t *testing.T) {
	envMap := map[string]string{
		"ZEBRA": "z",
		"ALPHA": "a",
		"MIKE":  "m",
		"BRAVO": "b",
	}

	envValue := NewEnvironmentValue(envMap)
	result := envValue.String()

	// Keys should be sorted alphabetically
	expected := "ALPHA=a BRAVO=b MIKE=m ZEBRA=z"
	assert.Equal(t, expected, result)
}

// TestEnvironmentValue_JSONMarshaling tests that JSON marshaling preserves original values
func TestEnvironmentValue_JSONMarshaling(t *testing.T) {
	envMap := map[string]string{
		"VAR_WITH_SPACE":   "hello world",
		"VAR_WITH_NEWLINE": "line1\nline2",
		"VAR_NORMAL":       "normal_value",
	}

	envValue := NewEnvironmentValue(envMap)

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

// TestEnvironmentValue_Value tests that Value() returns the original map
func TestEnvironmentValue_Value(t *testing.T) {
	envMap := map[string]string{
		"KEY1": "value with spaces",
		"KEY2": "value\nwith\nnewlines",
	}

	envValue := NewEnvironmentValue(envMap)
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
		"env_value": NewEnvironmentValue(map[string]string{
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

// TestEnvironmentValue_String_RealWorldExample tests real-world environment variable scenarios
func TestEnvironmentValue_String_RealWorldExample(t *testing.T) {
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
			envValue := NewEnvironmentValue(tt.envMap)
			result := envValue.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}
