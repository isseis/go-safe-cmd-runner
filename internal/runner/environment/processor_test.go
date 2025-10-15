package environment

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVariableExpander(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"PATH", "HOME"},
		},
	}
	filter := NewFilter(config.Global.EnvAllowlist)
	expander := NewVariableExpander(filter)

	assert.NotNil(t, expander)
	assert.Equal(t, filter, expander.filter)
	assert.NotNil(t, expander.logger)
}

func TestVariableExpander_ResolveVariableReferences(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"PATH", "HOME", "USER"},
		},
	}
	filter := NewFilter(config.Global.EnvAllowlist)
	expander := NewVariableExpander(filter)

	// Set up test environment variables
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", "/home/testuser")

	tests := []struct {
		name        string
		value       string
		envVars     map[string]string
		group       *runnertypes.CommandGroup
		expected    string
		expectError bool
		expectedErr error
	}{
		{
			name:    "no variable references",
			value:   "simple_value",
			envVars: map[string]string{"FOO": "bar"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expected: "simple_value",
		},
		{
			name:    "resolve from trusted source (envVars)",
			value:   "${FOO}/bin",
			envVars: map[string]string{"FOO": "/custom/path"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expected: "/custom/path/bin",
		},
		{
			name:    "resolve from system environment with allowlist check",
			value:   "${PATH}:/custom",
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expected: "/usr/bin:/bin:/custom",
		},
		{
			name:    "reject system variable not in allowlist",
			value:   "${USER}/bin",
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"}, // USER not in allowlist
			},
			expectError: true,
			expectedErr: ErrVariableNotAllowed,
		},
		{
			name:    "variable not found - return error",
			value:   "${NONEXISTENT}/bin",
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expectError: true,
			expectedErr: ErrVariableNotFound,
		},
		{
			name:    "multiple variable references",
			value:   "${HOME}/${FOO}",
			envVars: map[string]string{"FOO": "bin"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expected: "/home/testuser/bin",
		},
		{
			name:    "unclosed variable reference",
			value:   "${UNCLOSED",
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expectError: true,
			expectedErr: ErrUnclosedVariable,
		},
		{
			name:    "empty variable name",
			value:   "${}",
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{},
			},
			expectError: true,
			expectedErr: ErrInvalidVariableName,
		},
		{
			name:    "invalid variable name",
			value:   "${3}",
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{},
			},
			expectError: true,
			expectedErr: ErrInvalidVariableName,
		},
		// Invalid variable format tests
		{
			name:    "dollar without braces",
			value:   "$HOME",
			envVars: map[string]string{"HOME": "/home/user"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"HOME"},
			},
			expectError: true,
			expectedErr: ErrInvalidVariableFormat,
		},
		{
			name:    "dollar at end",
			value:   "path$",
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{},
			},
			expectError: true,
			expectedErr: ErrInvalidVariableFormat,
		},
		{
			name:    "dollar with invalid character",
			value:   "$@INVALID",
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{},
			},
			expectError: true,
			expectedErr: ErrInvalidVariableFormat,
		},
		{
			name:    "mixed valid and invalid formats",
			value:   "${HOME} and $USER",
			envVars: map[string]string{"HOME": "/home/user", "USER": "testuser"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"HOME", "USER"},
			},
			expectError: true,
			expectedErr: ErrInvalidVariableFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expander.ExpandString(tt.value, tt.envVars, tt.group.EnvAllowlist, tt.group.Name, make(map[string]struct{}))

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr, "Expected error type %v, got %v", tt.expectedErr, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVariableExpander_ResolveVariableReferences_CircularReferences(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"CIRCULAR_VAR", "VAR1", "VAR2", "VAR3"},
		},
	}
	filter := NewFilter(config.Global.EnvAllowlist)
	expander := NewVariableExpander(filter)

	tests := []struct {
		name           string
		value          string
		envVars        map[string]string
		group          *runnertypes.CommandGroup
		expectError    bool
		expectedResult string
		description    string
	}{
		{
			name:    "direct self-referencing variable",
			value:   "${CIRCULAR_VAR}",
			envVars: map[string]string{"CIRCULAR_VAR": "${CIRCULAR_VAR}"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"CIRCULAR_VAR"},
			},
			expectError: true,
			description: "A variable that references itself should be detected as infinite loop after max iterations",
		},
		{
			name:    "indirect circular reference (VAR1 -> VAR2 -> VAR1)",
			value:   "${VAR1}",
			envVars: map[string]string{"VAR1": "${VAR2}", "VAR2": "${VAR1}"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"VAR1", "VAR2"},
			},
			expectError: true,
			description: "Two variables referencing each other should be detected as infinite loop",
		},
		{
			name:    "complex nested circular reference",
			value:   "${VAR1}",
			envVars: map[string]string{"VAR1": "prefix-${VAR2}-suffix", "VAR2": "nested-${VAR1}-nested"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"VAR1", "VAR2"},
			},
			expectError: true,
			description: "Complex nested circular references should be detected",
		},
		{
			name:  "deep but non-circular reference chain",
			value: "${VAR1}",
			envVars: map[string]string{
				"VAR1": "${VAR2}/final",
				"VAR2": "${VAR3}/middle",
				"VAR3": "base",
			},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"VAR1", "VAR2", "VAR3"},
			},
			expectError:    false,
			expectedResult: "base/middle/final",
			description:    "Deep but non-circular reference chains should resolve successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expander.ExpandString(tt.value, tt.envVars, tt.group.EnvAllowlist, tt.group.Name, make(map[string]struct{}))

			if tt.expectError {
				assert.Error(t, err, "Expected error for case: %s", tt.description)
				assert.Empty(t, result, "Result should be empty on error")
			} else {
				assert.NoError(t, err, "Expected no error for case: %s", tt.description)
				assert.Equal(t, tt.expectedResult, result, "Expected result mismatch for case: %s", tt.description)
			}
		})
	}
}

func TestVariableExpander_ValidateBasicEnvVariable(t *testing.T) {
	tests := []struct {
		name        string
		varName     string
		varValue    string
		expectError bool
		expectedErr error
	}{
		{
			name:     "valid variable",
			varName:  "VALID_VAR",
			varValue: "safe_value",
		},
		{
			name:        "invalid variable name - starts with digit",
			varName:     "123INVALID",
			varValue:    "value",
			expectError: true,
			expectedErr: ErrInvalidVariableName,
		},
		{
			name:        "empty variable name",
			varName:     "",
			varValue:    "value",
			expectError: true,
			expectedErr: ErrVariableNameEmpty,
		},
		{
			name:        "dangerous variable value",
			varName:     "DANGEROUS",
			varValue:    "value; rm -rf /",
			expectError: true,
			expectedErr: security.ErrUnsafeEnvironmentVar,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a VariableExpander to test the validation method
			filter := NewFilter([]string{})
			expander := NewVariableExpander(filter)
			err := expander.validateBasicEnvVariable(tt.varName, tt.varValue)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr, "Expected error type %v, got %v", tt.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestVariableExpander_EscapeSequences tests escape sequence handling
func TestVariableExpander_EscapeSequences(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"FOO", "BAR", "ESCAPED_VAR"},
		},
	}
	filter := NewFilter(config.Global.EnvAllowlist)
	expander := NewVariableExpander(filter)

	tests := []struct {
		name        string
		value       string
		envVars     map[string]string
		group       *runnertypes.CommandGroup
		expected    string
		expectError bool
		expectedErr error
	}{
		// Basic escape tests
		{
			name:    "escape dollar sign",
			value:   `\${FOO}`,
			envVars: map[string]string{"FOO": "value"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"FOO"},
			},
			expected: "${FOO}",
		},
		{
			name:    "escape backslash",
			value:   `\\FOO`,
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{},
			},
			expected: `\FOO`,
		},
		{
			name:    "escaped dollar with variable expansion",
			value:   `\${FOO} and ${BAR}`,
			envVars: map[string]string{"FOO": "foo", "BAR": "bar"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"FOO", "BAR"},
			},
			expected: "${FOO} and bar",
		},
		{
			name:    "backslash before variable",
			value:   `\\${FOO}`,
			envVars: map[string]string{"FOO": "value"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"FOO"},
			},
			expected: `\value`,
		},
		{
			name:    "multiple escapes",
			value:   `\${FOO} \${BAR} \\baz`,
			envVars: map[string]string{"FOO": "f", "BAR": "b"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"FOO", "BAR"},
			},
			expected: "${FOO} ${BAR} \\baz",
		},
		{
			name:    "escaped in braces context",
			value:   `\${FOO} ${BAR}`,
			envVars: map[string]string{"FOO": "foo", "BAR": "bar"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"FOO", "BAR"},
			},
			expected: "${FOO} bar",
		},
		{
			name:    "mixed escape and expansion with prefix/suffix",
			value:   `prefix \${FOO} ${BAR} suffix`,
			envVars: map[string]string{"FOO": "foo", "BAR": "bar"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"FOO", "BAR"},
			},
			expected: "prefix ${FOO} bar suffix",
		},
		// Error tests
		{
			name:        "invalid escape sequence - letter",
			value:       `\U`,
			envVars:     map[string]string{},
			group:       &runnertypes.CommandGroup{Name: "test_group"},
			expectError: true,
			expectedErr: ErrInvalidEscapeSequence,
		},
		{
			name:        "invalid escape sequence - number",
			value:       `\1`,
			envVars:     map[string]string{},
			group:       &runnertypes.CommandGroup{Name: "test_group"},
			expectError: true,
			expectedErr: ErrInvalidEscapeSequence,
		},
		{
			name:        "trailing backslash",
			value:       `FOO\`,
			envVars:     map[string]string{},
			group:       &runnertypes.CommandGroup{Name: "test_group"},
			expectError: true,
			expectedErr: ErrInvalidEscapeSequence,
		},
		{
			name:        "invalid escape at the start",
			value:       `\@invalid`,
			envVars:     map[string]string{},
			group:       &runnertypes.CommandGroup{Name: "test_group"},
			expectError: true,
			expectedErr: ErrInvalidEscapeSequence,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expander.ExpandString(tt.value, tt.envVars, tt.group.EnvAllowlist, tt.group.Name, make(map[string]struct{}))

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr, "Expected error type %v, got %v", tt.expectedErr, err)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVariableExpander_ExpandStrings(t *testing.T) {
	filter := NewFilter(nil)
	expander := NewVariableExpander(filter)

	tests := []struct {
		name        string
		texts       []string
		envVars     map[string]string
		allowlist   []string
		groupName   string
		expected    []string
		expectError bool
	}{
		{
			name:      "nil input returns nil",
			texts:     nil,
			envVars:   map[string]string{},
			allowlist: []string{"PATH"},
			groupName: "test_group",
			expected:  nil,
		},
		{
			name:      "empty slice returns empty slice",
			texts:     []string{},
			envVars:   map[string]string{},
			allowlist: []string{"PATH"},
			groupName: "test_group",
			expected:  []string{},
		},
		{
			name:      "expand single string",
			texts:     []string{"Hello, ${USER}!"},
			envVars:   map[string]string{"USER": "testuser"},
			allowlist: []string{"USER"},
			groupName: "test_group",
			expected:  []string{"Hello, testuser!"},
		},
		{
			name: "expand multiple strings",
			texts: []string{
				"Path: ${PATH}",
				"Home: ${HOME}",
				"User: ${USER}",
			},
			envVars: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/test",
				"USER": "testuser",
			},
			allowlist: []string{"PATH", "HOME", "USER"},
			groupName: "test_group",
			expected: []string{
				"Path: /usr/bin",
				"Home: /home/test",
				"User: testuser",
			},
		},
		{
			name: "mixed expanded and literal strings",
			texts: []string{
				"Literal text",
				"Variable: ${USER}",
				"Another literal",
			},
			envVars:   map[string]string{"USER": "testuser"},
			allowlist: []string{"USER"},
			groupName: "test_group",
			expected: []string{
				"Literal text",
				"Variable: testuser",
				"Another literal",
			},
		},
		{
			name:        "error in one string fails entire batch",
			texts:       []string{"Good: ${USER}", "Bad: ${INVALID}"},
			envVars:     map[string]string{"USER": "testuser"},
			allowlist:   []string{"USER"},
			groupName:   "test_group",
			expectError: true,
		},
		{
			name: "escape sequences in batch",
			texts: []string{
				"Escaped: \\${USER}",
				"Normal: ${USER}",
			},
			envVars:   map[string]string{"USER": "testuser"},
			allowlist: []string{"USER"},
			groupName: "test_group",
			expected: []string{
				"Escaped: ${USER}",
				"Normal: testuser",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expander.ExpandStrings(tt.texts, tt.envVars, tt.allowlist, tt.groupName)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Additional test for disallowed system environment variable
	t.Run("disallowed system environment variable", func(t *testing.T) {
		// Set a temporary environment variable
		t.Setenv("DISALLOWED_VAR", "system_value")

		// Try to expand it with an allowlist that doesn't include it
		texts := []string{"Value: ${DISALLOWED_VAR}"}
		result, err := expander.ExpandStrings(texts, map[string]string{}, []string{"USER"}, "test_group")

		// Should fail because DISALLOWED_VAR is in system environment but not in allowlist
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	// Additional test for allowed system environment variable
	t.Run("allowed system environment variable", func(t *testing.T) {
		// Set a temporary environment variable
		t.Setenv("ALLOWED_VAR", "system_value")

		// Try to expand it with an allowlist that includes it
		texts := []string{"Value: ${ALLOWED_VAR}"}
		result, err := expander.ExpandStrings(texts, map[string]string{}, []string{"ALLOWED_VAR"}, "test_group")

		// Should succeed because ALLOWED_VAR is in both system environment and allowlist
		require.NoError(t, err)
		assert.Equal(t, []string{"Value: system_value"}, result)
	})
}
