package environment

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommandEnvProcessor(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"PATH", "HOME"},
		},
	}
	filter := NewFilter(config)
	processor := NewCommandEnvProcessor(filter)

	assert.NotNil(t, processor)
	assert.Equal(t, filter, processor.filter)
	assert.NotNil(t, processor.logger)
}

func TestCommandEnvProcessor_ProcessCommandEnvironment(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"PATH", "HOME", "USER"},
		},
	}
	filter := NewFilter(config)
	processor := NewCommandEnvProcessor(filter)

	tests := []struct {
		name         string
		cmd          runnertypes.Command
		baseEnvVars  map[string]string
		group        *runnertypes.CommandGroup
		expectedVars map[string]string
		expectError  bool
		expectedErr  error
	}{
		{
			name: "process simple command env variables",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"FOO=bar", "BAZ=qux"},
			},
			baseEnvVars: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/user",
			},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expectedVars: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/user",
				"FOO":  "bar",
				"BAZ":  "qux",
			},
		},
		{
			name: "override base environment variables",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"PATH=/custom/path", "NEW_VAR=value"},
			},
			baseEnvVars: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/user",
			},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expectedVars: map[string]string{
				"PATH":    "/custom/path",
				"HOME":    "/home/user",
				"NEW_VAR": "value",
			},
		},
		{
			name: "skip invalid environment variable format",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"VALID=value", "INVALID_NO_EQUALS", "ANOTHER=valid"},
			},
			baseEnvVars: map[string]string{
				"PATH": "/usr/bin",
			},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH"},
			},
			expectError: true,
			expectedErr: ErrMalformedEnvVariable,
		},
		{
			name: "reject dangerous variable value",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"DANGEROUS=value; rm -rf /"},
			},
			baseEnvVars: map[string]string{
				"PATH": "/usr/bin",
			},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH"},
			},
			expectError: true,
			expectedErr: security.ErrUnsafeEnvironmentVar,
		},
		{
			name: "reject invalid variable name",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"123INVALID=value"},
			},
			baseEnvVars: map[string]string{
				"PATH": "/usr/bin",
			},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH"},
			},
			expectError: true,
			expectedErr: ErrInvalidVariableName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ProcessCommandEnvironment(tt.cmd, tt.baseEnvVars, tt.group)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr, "Expected error type %v, got %v", tt.expectedErr, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedVars, result)
		})
	}
}

func TestCommandEnvProcessor_ResolveVariableReferences(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"PATH", "HOME", "USER"},
		},
	}
	filter := NewFilter(config)
	processor := NewCommandEnvProcessor(filter)

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
			name:    "variable not found - treat as empty string",
			value:   "${NONEXISTENT}/bin",
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expected: "/bin",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ExpandVariablesWithEscaping(tt.value, tt.envVars, tt.group)

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

func TestCommandEnvProcessor_ResolveVariableReferences_CircularReferences(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"CIRCULAR_VAR", "VAR1", "VAR2", "VAR3"},
		},
	}
	filter := NewFilter(config)
	processor := NewCommandEnvProcessor(filter)

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
			result, err := processor.ExpandVariablesWithEscaping(tt.value, tt.envVars, tt.group)

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

func TestCommandEnvProcessor_ValidateBasicEnvVariable(t *testing.T) {
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
			err := validateBasicEnvVariable(tt.varName, tt.varValue)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr, "Expected error type %v, got %v", tt.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCommandEnvProcessor_InheritanceModeIntegration(t *testing.T) {
func TestCommandEnvProcessor_InheritanceModeIntegration(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"GLOBAL_VAR"},
		},
	}
	filter := NewFilter(config)
	processor := NewCommandEnvProcessor(filter)

	// Set up system environment
	t.Setenv("GLOBAL_VAR", "global_value")
	t.Setenv("GROUP_VAR", "group_value")

	tests := []struct {
		name        string
		group       *runnertypes.CommandGroup
		cmd         runnertypes.Command
		baseEnvVars map[string]string
		expectError bool
		description string
	}{
		{
			name: "inherit mode - can access global allowlist variables",
			group: &runnertypes.CommandGroup{
				Name:         "inherit_group",
				EnvAllowlist: nil, // nil = inherit mode
			},
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"TEST_VAR=${GLOBAL_VAR}"},
			},
			baseEnvVars: map[string]string{},
			expectError: false,
			description: "Should be able to reference GLOBAL_VAR in inherit mode",
		},
		{
			name: "explicit mode - cannot access global allowlist variables",
			group: &runnertypes.CommandGroup{
				Name:         "explicit_group",
				EnvAllowlist: []string{"GROUP_VAR"}, // explicit list
			},
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"TEST_VAR=${GLOBAL_VAR}"},
			},
			baseEnvVars: map[string]string{},
			expectError: true,
			description: "Should not be able to reference GLOBAL_VAR in explicit mode",
		},
		{
			name: "reject mode - cannot access any system variables",
			group: &runnertypes.CommandGroup{
				Name:         "reject_group",
				EnvAllowlist: []string{}, // empty = reject mode
			},
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"TEST_VAR=${GLOBAL_VAR}"},
			},
			baseEnvVars: map[string]string{},
			expectError: true,
			description: "Should not be able to reference any system variables in reject mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := processor.ProcessCommandEnvironment(tt.cmd, tt.baseEnvVars, tt.group)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

// TestCommandEnvProcessor_EscapeSequences tests escape sequence handling
func TestCommandEnvProcessor_EscapeSequences(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"FOO", "BAR", "ESCAPED_VAR"},
		},
	}
	filter := NewFilter(config)
	processor := NewCommandEnvProcessor(filter)

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
			result, err := processor.ExpandVariablesWithEscaping(tt.value, tt.envVars, tt.group)

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

// TestCommandEnvProcessor_EscapeSequences_CommandEnv tests escape sequences in Command.Env context
func TestCommandEnvProcessor_EscapeSequences_CommandEnv(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"BAR"},
		},
	}
	filter := NewFilter(config)
	processor := NewCommandEnvProcessor(filter)

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
			name:    "escape dollar in command env (braces only)",
			value:   `\${FOO}`,
			envVars: map[string]string{"FOO": "value"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"FOO"},
			},
			expected: "${FOO}",
		},
		{
			name:    "escape backslash in command env",
			value:   `\\path`,
			envVars: map[string]string{},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{},
			},
			expected: `\path`,
		},
		{
			name:    "mixed escape and expansion in command env",
			value:   `prefix \${FOO} ${BAR} suffix`,
			envVars: map[string]string{"FOO": "foo", "BAR": "bar"},
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"FOO", "BAR"},
			},
			expected: "prefix ${FOO} bar suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ExpandVariablesWithEscaping(tt.value, tt.envVars, tt.group)

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
