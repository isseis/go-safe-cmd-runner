package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExpandString_EscapeSequence tests escape sequence handling in ExpandString
func TestExpandString_EscapeSequence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		want     string
		wantErr  bool
		errorMsg string
	}{
		{
			name:    "escape percent sign",
			input:   "prefix\\%{VAR}suffix",
			vars:    map[string]string{"VAR": "value"},
			want:    "prefix%{VAR}suffix",
			wantErr: false,
		},
		{
			name:    "escape backslash",
			input:   "path\\\\file",
			vars:    map[string]string{},
			want:    "path\\file",
			wantErr: false,
		},
		{
			name:    "multiple escapes",
			input:   "\\%{A}\\\\\\%{B}",
			vars:    map[string]string{"A": "val1", "B": "val2"},
			want:    "%{A}\\%{B}",
			wantErr: false,
		},
		{
			name:     "invalid escape sequence",
			input:    "test\\x",
			vars:     map[string]string{},
			wantErr:  true,
			errorMsg: "invalid escape sequence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ExpandString(tt.input, tt.vars, "test", "field")
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestExpandString_UndefinedVariable tests undefined variable handling
func TestExpandString_UndefinedVariable(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		errorMsg string
	}{
		{
			name:     "simple undefined variable",
			input:    "%{UNDEFINED}",
			vars:     map[string]string{},
			errorMsg: "undefined variable",
		},
		{
			name:     "undefined variable in context",
			input:    "prefix_%{MISSING}_suffix",
			vars:     map[string]string{"OTHER": "value"},
			errorMsg: "MISSING",
		},
		{
			name:     "multiple undefined variables (first fails)",
			input:    "%{UNDEFINED1}_%{UNDEFINED2}",
			vars:     map[string]string{},
			errorMsg: "UNDEFINED1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandString(tt.input, tt.vars, "test", "field")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// TestExpandString_ComplexPatterns tests complex variable expansion patterns
func TestExpandString_ComplexPatterns(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		vars    map[string]string
		want    string
		wantErr bool
	}{
		{
			name:  "nested variable reference",
			input: "%{NESTED}",
			vars: map[string]string{
				"NESTED": "%{BASE}/path",
				"BASE":   "/root",
			},
			want:    "/root/path",
			wantErr: false,
		},
		{
			name:  "multiple variables in one string",
			input: "%{A}/%{B}/%{C}",
			vars: map[string]string{
				"A": "first",
				"B": "second",
				"C": "third",
			},
			want:    "first/second/third",
			wantErr: false,
		},
		{
			name:  "variable reference with text around",
			input: "prefix_%{VAR}_middle_%{OTHER}_suffix",
			vars: map[string]string{
				"VAR":   "value1",
				"OTHER": "value2",
			},
			want:    "prefix_value1_middle_value2_suffix",
			wantErr: false,
		},
		{
			name:  "empty variable value",
			input: "%{EMPTY}text",
			vars: map[string]string{
				"EMPTY": "",
			},
			want:    "text",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ExpandString(tt.input, tt.vars, "test", "field")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestExpandString_InvalidSyntax tests invalid variable syntax
func TestExpandString_InvalidSyntax(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		errorMsg string
	}{
		{
			name:     "unclosed variable reference",
			input:    "%{UNCLOSED",
			vars:     map[string]string{},
			errorMsg: "unclosed variable reference",
		},
		{
			name:     "empty variable name",
			input:    "%{}",
			vars:     map[string]string{},
			errorMsg: "invalid variable name",
		},
		{
			name:     "variable with invalid characters",
			input:    "%{VAR-WITH-DASH}",
			vars:     map[string]string{},
			errorMsg: "invalid variable name",
		},
		{
			name:     "variable with space",
			input:    "%{VAR NAME}",
			vars:     map[string]string{},
			errorMsg: "invalid variable name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandString(tt.input, tt.vars, "test", "field")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// TestProcessFromEnv_AllowlistViolation tests allowlist enforcement
func TestProcessFromEnv_AllowlistViolation(t *testing.T) {
	tests := []struct {
		name        string
		fromEnv     []string
		allowlist   []string
		systemEnv   map[string]string
		wantErr     bool
		errorMsg    string
		expectedVar string
	}{
		{
			name:      "system variable not in allowlist",
			fromEnv:   []string{"my_var=BLOCKED_VAR"},
			allowlist: []string{"ALLOWED_VAR"},
			systemEnv: map[string]string{"BLOCKED_VAR": "value"},
			wantErr:   true,
			errorMsg:  "not in allowlist",
		},
		{
			name:        "system variable in allowlist",
			fromEnv:     []string{"my_var=ALLOWED_VAR"},
			allowlist:   []string{"ALLOWED_VAR"},
			systemEnv:   map[string]string{"ALLOWED_VAR": "value"},
			wantErr:     false,
			expectedVar: "my_var",
		},
		{
			name:      "empty allowlist blocks all",
			fromEnv:   []string{"my_var=ANY_VAR"},
			allowlist: []string{},
			systemEnv: map[string]string{"ANY_VAR": "value"},
			wantErr:   true,
			errorMsg:  "not in allowlist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessFromEnv(tt.fromEnv, tt.allowlist, tt.systemEnv, "test")
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				assert.Contains(t, result, tt.expectedVar)
			}
		})
	}
}

// TestProcessFromEnv_SystemVariableNotSet tests handling of missing system variables
// Note: ProcessFromEnv returns empty string for missing variables, not an error
func TestProcessFromEnv_SystemVariableNotSet(t *testing.T) {
	fromEnv := []string{"my_var=MISSING_VAR"}
	allowlist := []string{"MISSING_VAR"}
	systemEnv := map[string]string{} // MISSING_VAR not set

	result, err := config.ProcessFromEnv(fromEnv, allowlist, systemEnv, "test")
	require.NoError(t, err, "Missing system variables should not cause an error")
	assert.Equal(t, "", result["my_var"], "Missing variable should have empty string value")
}

// TestProcessFromEnv_InvalidFormat tests invalid from_env format handling
func TestProcessFromEnv_InvalidFormat(t *testing.T) {
	tests := []struct {
		name     string
		fromEnv  []string
		errorMsg string
	}{
		{
			name:     "missing equals sign",
			fromEnv:  []string{"no_equals"},
			errorMsg: "invalid from_env format",
		},
		{
			name:     "empty mapping",
			fromEnv:  []string{""},
			errorMsg: "invalid from_env format",
		},
		{
			name:     "multiple equals signs causes invalid system var name",
			fromEnv:  []string{"var=SYS=VAR"},
			errorMsg: "invalid system variable name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ProcessFromEnv(tt.fromEnv, []string{}, map[string]string{}, "test")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// TestProcessFromEnv_InvalidInternalVariableName tests internal variable name validation
func TestProcessFromEnv_InvalidInternalVariableName(t *testing.T) {
	tests := []struct {
		name     string
		fromEnv  []string
		errorMsg string
	}{
		{
			name:     "empty internal variable name",
			fromEnv:  []string{"=SYSTEM_VAR"},
			errorMsg: "invalid from_env format",
		},
		{
			name:     "internal variable with dash",
			fromEnv:  []string{"my-var=SYSTEM_VAR"},
			errorMsg: "invalid variable name",
		},
		{
			name:     "internal variable with space",
			fromEnv:  []string{"my var=SYSTEM_VAR"},
			errorMsg: "invalid variable name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ProcessFromEnv(tt.fromEnv, []string{"SYSTEM_VAR"}, map[string]string{"SYSTEM_VAR": "value"}, "test")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// TestProcessFromEnv_DuplicateDefinition tests duplicate internal variable detection
func TestProcessFromEnv_DuplicateDefinition(t *testing.T) {
	fromEnv := []string{
		"my_var=SYSTEM_VAR1",
		"my_var=SYSTEM_VAR2", // Duplicate internal name
	}
	allowlist := []string{"SYSTEM_VAR1", "SYSTEM_VAR2"}
	systemEnv := map[string]string{
		"SYSTEM_VAR1": "value1",
		"SYSTEM_VAR2": "value2",
	}

	_, err := config.ProcessFromEnv(fromEnv, allowlist, systemEnv, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate variable")
}

// TestProcessVars_DuplicateDefinition tests duplicate variable detection in vars
// Note: With map[string]interface{}, duplicate keys are inherently impossible in Go,
// so this test is no longer applicable and has been removed.

// TestProcessVars_InvalidVariableName tests invalid variable names in vars
func TestProcessVars_InvalidVariableName(t *testing.T) {
	tests := []struct {
		name     string
		vars     map[string]interface{}
		errorMsg string
	}{
		{
			name:     "variable with dash",
			vars:     map[string]interface{}{"my-var": "value"},
			errorMsg: "invalid variable name",
		},
		{
			name:     "variable with space",
			vars:     map[string]interface{}{"my var": "value"},
			errorMsg: "invalid variable name",
		},
		{
			name:     "empty variable name",
			vars:     map[string]interface{}{"": "value"},
			errorMsg: "invalid variable name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseVars := make(map[string]string)
			_, _, err := config.ProcessVars(tt.vars, baseVars, nil, "test")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// TestProcessVars_ComplexReferenceChain tests complex variable reference chains
func TestProcessVars_ComplexReferenceChain(t *testing.T) {
	tests := []struct {
		name     string
		vars     map[string]interface{}
		baseVars map[string]string
		checkVar string
		wantVal  string
		wantErr  bool
	}{
		{
			name: "linear chain",
			vars: map[string]interface{}{
				"A": "base",
				"B": "%{A}/level1",
				"C": "%{B}/level2",
				"D": "%{C}/level3",
			},
			baseVars: make(map[string]string),
			checkVar: "D",
			wantVal:  "base/level1/level2/level3",
			wantErr:  false,
		},
		{
			name: "reference base variables",
			vars: map[string]interface{}{
				"NEW_VAR": "%{BASE1}/%{BASE2}",
			},
			baseVars: map[string]string{
				"BASE1": "first",
				"BASE2": "second",
			},
			checkVar: "NEW_VAR",
			wantVal:  "first/second",
			wantErr:  false,
		},
		{
			name: "override base variable",
			vars: map[string]interface{}{
				"BASE": "%{BASE}_extended",
			},
			baseVars: map[string]string{
				"BASE": "original",
			},
			checkVar: "BASE",
			wantVal:  "original_extended",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _, err := config.ProcessVars(tt.vars, tt.baseVars, nil, "test")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantVal, result[tt.checkVar])
			}
		})
	}
}

// TestProcessVars_UndefinedReference tests undefined variable references
func TestProcessVars_UndefinedReference(t *testing.T) {
	vars := map[string]interface{}{
		"VAR": "%{UNDEFINED}",
	}
	baseVars := make(map[string]string)

	_, _, err := config.ProcessVars(vars, baseVars, nil, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undefined variable")
}

// TestProcessEnv_VariableReference tests that env can reference vars
func TestProcessEnv_VariableReference(t *testing.T) {
	tests := []struct {
		name         string
		env          []string
		internalVars map[string]string
		checkEnv     string
		wantVal      string
		wantErr      bool
	}{
		{
			name: "simple variable reference",
			env:  []string{"PATH=%{BASE_PATH}/bin"},
			internalVars: map[string]string{
				"BASE_PATH": "/usr/local",
			},
			checkEnv: "PATH",
			wantVal:  "/usr/local/bin",
			wantErr:  false,
		},
		{
			name: "multiple variable references",
			env:  []string{"COMBINED=%{VAR1}:%{VAR2}"},
			internalVars: map[string]string{
				"VAR1": "first",
				"VAR2": "second",
			},
			checkEnv: "COMBINED",
			wantVal:  "first:second",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessEnv(tt.env, tt.internalVars, "test")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantVal, result[tt.checkEnv])
			}
		})
	}
}

// TestProcessEnv_UndefinedVariable tests that env cannot reference undefined variables
func TestProcessEnv_UndefinedVariable(t *testing.T) {
	env := []string{"ENV_VAR=%{UNDEFINED}"}
	internalVars := make(map[string]string)

	_, err := config.ProcessEnv(env, internalVars, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undefined variable")
}

// TestProcessEnv_InvalidEnvVarName tests invalid environment variable names
func TestProcessEnv_InvalidEnvVarName(t *testing.T) {
	tests := []struct {
		name     string
		env      []string
		errorMsg string
	}{
		{
			name:     "env var with dash",
			env:      []string{"MY-VAR=value"},
			errorMsg: "invalid environment variable key",
		},
		{
			name:     "env var with space",
			env:      []string{"MY VAR=value"},
			errorMsg: "invalid environment variable key",
		},
		{
			name:     "empty env var name",
			env:      []string{"=value"},
			errorMsg: "invalid env format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			internalVars := make(map[string]string)
			_, err := config.ProcessEnv(tt.env, internalVars, "test")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// TestProcessEnv_DuplicateDefinition tests duplicate env variable detection
func TestProcessEnv_DuplicateDefinition(t *testing.T) {
	env := []string{
		"MY_VAR=value1",
		"MY_VAR=value2", // Duplicate
	}
	internalVars := make(map[string]string)

	_, err := config.ProcessEnv(env, internalVars, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate variable")
}

// TestIntegration_FullExpansionChain tests the full expansion chain: from_env -> vars -> env
func TestIntegration_FullExpansionChain(t *testing.T) {
	// Create a GlobalSpec that uses all three: from_env, vars, env
	spec := &runnertypes.GlobalSpec{
		EnvImport: []string{
			"sys_path=PATH",
			"sys_home=HOME",
		},
		Vars: map[string]interface{}{
			"base_dir": "%{sys_home}/app",
			"bin_dir":  "%{base_dir}/bin",
		},
		EnvVars: []string{
			"APP_HOME=%{base_dir}",
			"PATH=%{bin_dir}:%{sys_path}",
		},
		EnvAllowed: []string{"PATH", "HOME"},
	}

	// Set system environment variables
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", "/home/testuser")

	// Expand global
	runtime, err := config.ExpandGlobal(spec)
	require.NoError(t, err)
	require.NotNil(t, runtime)

	// Verify internal variables
	assert.Equal(t, "/usr/bin:/bin", runtime.ExpandedVars["sys_path"])
	assert.Equal(t, "/home/testuser", runtime.ExpandedVars["sys_home"])
	assert.Equal(t, "/home/testuser/app", runtime.ExpandedVars["base_dir"])
	assert.Equal(t, "/home/testuser/app/bin", runtime.ExpandedVars["bin_dir"])

	// Verify environment variables
	assert.Equal(t, "/home/testuser/app", runtime.ExpandedEnv["APP_HOME"])
	assert.Equal(t, "/home/testuser/app/bin:/usr/bin:/bin", runtime.ExpandedEnv["PATH"])
}

// TestExpandGroup_SetsEnvAllowlistInheritanceMode tests that ExpandGroup correctly sets
// the EnvAllowlistInheritanceMode field based on the group's EnvAllowed configuration.
func TestExpandGroup_SetsEnvAllowlistInheritanceMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		groupEnvAllowed []string
		expectedMode    runnertypes.InheritanceMode
		description     string
	}{
		{
			name:            "Inherit mode - nil EnvAllowed",
			groupEnvAllowed: nil,
			expectedMode:    runnertypes.InheritanceModeInherit,
			description:     "Group should inherit global allowlist when EnvAllowed is nil",
		},
		{
			name:            "Reject mode - empty EnvAllowed",
			groupEnvAllowed: []string{},
			expectedMode:    runnertypes.InheritanceModeReject,
			description:     "Group should reject all environment variables when EnvAllowed is empty",
		},
		{
			name:            "Explicit mode - single element",
			groupEnvAllowed: []string{"VAR1"},
			expectedMode:    runnertypes.InheritanceModeExplicit,
			description:     "Group should use explicit allowlist with one variable",
		},
		{
			name:            "Explicit mode - multiple elements",
			groupEnvAllowed: []string{"VAR1", "VAR2", "VAR3"},
			expectedMode:    runnertypes.InheritanceModeExplicit,
			description:     "Group should use explicit allowlist with multiple variables",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create minimal group spec
			groupSpec := &runnertypes.GroupSpec{
				Name:       "test-group",
				EnvAllowed: tt.groupEnvAllowed,
				Commands:   []runnertypes.CommandSpec{},
			}

			// Create minimal global runtime
			globalSpec := &runnertypes.GlobalSpec{
				EnvAllowed: []string{"GLOBAL_VAR"},
			}
			globalRuntime, err := config.ExpandGlobal(globalSpec)
			require.NoError(t, err)

			// Expand group
			runtimeGroup, err := config.ExpandGroup(groupSpec, globalRuntime)
			require.NoError(t, err, "ExpandGroup should not return an error")
			require.NotNil(t, runtimeGroup, "ExpandGroup should return a non-nil RuntimeGroup")

			// Verify inheritance mode is set correctly
			assert.Equal(t, tt.expectedMode, runtimeGroup.EnvAllowlistInheritanceMode,
				"%s: expected mode %v, got %v", tt.description, tt.expectedMode, runtimeGroup.EnvAllowlistInheritanceMode)
		})
	}
}
