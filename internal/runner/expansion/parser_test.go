package expansion

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariableParser_ReplaceVariables(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		env       map[string]string
		expected  string
		expectErr bool
	}{
		{
			name:     "simple variable",
			input:    "$HOME",
			env:      map[string]string{"HOME": "/home/user"},
			expected: "/home/user",
		},
		{
			name:     "braced variable",
			input:    "${USER}",
			env:      map[string]string{"USER": "testuser"},
			expected: "testuser",
		},
		{
			name:     "mixed variables",
			input:    "$HOME/bin/${APP_NAME}",
			env:      map[string]string{"HOME": "/home/user", "APP_NAME": "myapp"},
			expected: "/home/user/bin/myapp",
		},
		{
			name:     "prefix_$VAR_suffix problem case",
			input:    "prefix_$HOME_suffix",
			env:      map[string]string{"HOME": "user", "HOME_suffix": "fallback"},
			expected: "prefix_fallback", // $HOME_suffix is recognized as full variable name
		},
		{
			name:     "JSON with variable - unified pattern processing",
			input:    `{"key": "$VALUE"}`,
			env:      map[string]string{"VALUE": "test"},
			expected: `{"key": "test"}`, // Unified pattern correctly expands $VALUE in JSON
		},
		{
			name:     "mixed braced and simple - no overlap issues",
			input:    `{"user": "$USER", "home": "${HOME}"}`,
			env:      map[string]string{"USER": "testuser", "HOME": "/home/testuser"},
			expected: `{"user": "testuser", "home": "/home/testuser"}`, // No overlap issues
		},
		{
			name:     "unified pattern handles complex cases",
			input:    "before_${VAR}_middle_$VAR2_after",
			env:      map[string]string{"VAR": "value1", "VAR2_after": "value2_after"},
			expected: "before_value1_middle_value2_after", // $VAR2_after is treated as single variable name
		},
		{
			name:     "edge case with similar variable names",
			input:    "$HOME and ${HOME_DIR} and $HOME_suffix",
			env:      map[string]string{"HOME": "user", "HOME_DIR": "/home/user", "HOME_suffix": "fallback"},
			expected: "user and /home/user and fallback", // Unified pattern properly separates
		},
		{
			name:     "recommended braced format",
			input:    "prefix_${HOME}_suffix",
			env:      map[string]string{"HOME": "user"},
			expected: "prefix_user_suffix",
		},
		{
			name:     "glob patterns as literals",
			input:    "$HOME/*.txt",
			env:      map[string]string{"HOME": "/home/user"},
			expected: "/home/user/*.txt", // * is treated as literal character
		},
		{
			name:     "no variables",
			input:    "/usr/bin/ls",
			env:      map[string]string{},
			expected: "/usr/bin/ls",
		},
		{
			name:      "undefined variable",
			input:     "$UNDEFINED",
			env:       map[string]string{},
			expectErr: true,
		},
		{
			name:      "circular reference",
			input:     "$A",
			env:       map[string]string{"A": "$B", "B": "$A"},
			expectErr: true,
		},
		{
			name:     "nested variables",
			input:    "$A",
			env:      map[string]string{"A": "${B}/final", "B": "base"},
			expected: "base/final",
		},
	}

	// Test resolver implementation
	parser := NewVariableParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &testVariableResolver{env: tt.env}
			result, err := parser.ReplaceVariables(tt.input, resolver)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// testVariableResolver is a test VariableResolver implementation
type testVariableResolver struct {
	env map[string]string
}

func (r *testVariableResolver) ResolveVariable(name string) (string, error) {
	if value, exists := r.env[name]; exists {
		return value, nil
	}
	return "", fmt.Errorf("variable not found: %s", name)
}

func TestVariableParser_CircularReferenceDetection(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		testValue string
		expectErr bool
	}{
		{
			name: "no circular reference - both formats",
			env: map[string]string{
				"A": "value_a",
				"B": "$A",
				"C": "${B}/suffix",
			},
			testValue: "${C}",
		},
		{
			name: "direct circular reference - $VAR format",
			env: map[string]string{
				"A": "$B",
				"B": "$A",
			},
			testValue: "$A",
			expectErr: true, // Detected by existing iterative approach
		},
		{
			name: "indirect circular reference - ${VAR} format",
			env: map[string]string{
				"A": "${B}",
				"B": "${C}",
				"C": "${A}",
			},
			testValue: "${A}",
			expectErr: true, // Detected by existing iterative approach
		},
		{
			name: "mixed format circular reference",
			env: map[string]string{
				"A": "$B",
				"B": "${A}",
			},
			testValue: "$A",
			expectErr: true, // Both format circular references also detected
		},
		{
			name: "self reference",
			env: map[string]string{
				"A": "$A",
			},
			testValue: "$A",
			expectErr: true,
		},
		{
			name: "JSON with variables - no false circular detection",
			env: map[string]string{
				"USER": "testuser",
				"HOME": "/home/testuser",
			},
			testValue: `{"user": "$USER", "home": "${HOME}"}`,
		},
	}

	parser := NewVariableParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &testVariableResolver{env: tt.env}
			_, err := parser.ReplaceVariables(tt.testValue, resolver)

			if tt.expectErr {
				assert.Error(t, err)
				// Confirm that existing ErrCircularReference is returned
				assert.True(t, IsCircularReferenceError(err), "Expected circular reference error")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVariableParser_EdgeCases(t *testing.T) {
	parser := NewVariableParser()

	tests := []struct {
		name     string
		input    string
		env      map[string]string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			env:      map[string]string{},
			expected: "",
		},
		{
			name:     "no dollar signs",
			input:    "plain text",
			env:      map[string]string{},
			expected: "plain text",
		},
		{
			name:     "dollar at end",
			input:    "text$",
			env:      map[string]string{},
			expected: "text$",
		},
		{
			name:     "malformed braces",
			input:    "${UNCLOSED",
			env:      map[string]string{},
			expected: "${UNCLOSED",
		},
		{
			name:     "empty braces",
			input:    "${}",
			env:      map[string]string{},
			expected: "${}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &testVariableResolver{env: tt.env}
			result, err := parser.ReplaceVariables(tt.input, resolver)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
