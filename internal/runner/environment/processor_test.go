package environment

import (
	"os"
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
	originalPath := os.Getenv("PATH")
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalPath != "" {
			os.Setenv("PATH", originalPath)
		}
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		}
	}()

	os.Setenv("PATH", "/usr/bin:/bin")
	os.Setenv("HOME", "/home/testuser")

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
			name:    "variable not found",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.resolveVariableReferencesForCommandEnv(tt.value, tt.envVars, tt.group)

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

func TestCommandEnvProcessor_ValidateBasicEnvVariable(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"PATH"},
		},
	}
	filter := NewFilter(config)
	processor := NewCommandEnvProcessor(filter)

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
			err := processor.validateBasicEnvVariable(tt.varName, tt.varValue)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr, "Expected error type %v, got %v", tt.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCommandEnvProcessor_InheritanceModeIntegration(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"GLOBAL_VAR"},
		},
	}
	filter := NewFilter(config)
	processor := NewCommandEnvProcessor(filter)

	// Set up system environment
	os.Setenv("GLOBAL_VAR", "global_value")
	os.Setenv("GROUP_VAR", "group_value")
	defer func() {
		os.Unsetenv("GLOBAL_VAR")
		os.Unsetenv("GROUP_VAR")
	}()

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
