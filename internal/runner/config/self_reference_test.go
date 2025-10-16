// Package config provides tests for self-reference and circular reference detection.
package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelfReference_Direct tests direct self-reference detection
func TestSelfReference_Direct(t *testing.T) {
	tests := []struct {
		name        string
		level       string // "global", "group", or "command"
		vars        []string
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name:        "Global level - direct self-reference (undefined error)",
			level:       "global",
			vars:        []string{"A=%{A}"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				// With sequential processing, A references itself before being defined
				assert.ErrorIs(t, err, config.ErrUndefinedVariable)
			},
		},
		{
			name:        "Group level - direct self-reference (undefined error)",
			level:       "group",
			vars:        []string{"VAR=%{VAR}"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrUndefinedVariable)
			},
		},
		{
			name:        "Command level - direct self-reference (undefined error)",
			level:       "command",
			vars:        []string{"PATH=%{PATH}"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrUndefinedVariable)
			},
		},
		{
			name:        "Self-reference as part of value - extension allowed",
			level:       "global",
			vars:        []string{"PATH=/custom:%{PATH}"},
			expectError: false, // This is allowed - extending existing PATH
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error

			switch tt.level {
			case "global":
				// For "extending" test case, add PATH to from_env
				var fromEnv []string
				var allowlist []string
				if tt.name == "Self-reference as part of value - extension allowed" {
					t.Setenv("PATH", "/usr/bin")
					fromEnv = []string{"PATH=PATH"}
					allowlist = []string{"PATH"}
				}

				global := &runnertypes.GlobalConfig{
					Vars:         tt.vars,
					FromEnv:      fromEnv,
					EnvAllowlist: allowlist,
				}
				filter := environment.NewFilter(allowlist)
				err = config.ExpandGlobalConfig(global, filter)

			case "group":
				global := &runnertypes.GlobalConfig{}
				filter := environment.NewFilter([]string{})
				err = config.ExpandGlobalConfig(global, filter)
				require.NoError(t, err)

				group := &runnertypes.CommandGroup{
					Name: "test_group",
					Vars: tt.vars,
				}
				err = config.ExpandGroupConfig(group, global, filter)

			case "command":
				global := &runnertypes.GlobalConfig{}
				filter := environment.NewFilter([]string{})
				err = config.ExpandGlobalConfig(global, filter)
				require.NoError(t, err)

				group := &runnertypes.CommandGroup{
					Name: "test_group",
				}
				err = config.ExpandGroupConfig(group, global, filter)
				require.NoError(t, err)

				cmd := &runnertypes.Command{
					Name: "test_command",
					Cmd:  "/bin/echo",
					Vars: tt.vars,
				}
				err = config.ExpandCommandConfig(cmd, group, global, filter)
			}

			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSelfReference_Circular_TwoVariables tests circular references with two variables
func TestSelfReference_Circular_TwoVariables(t *testing.T) {
	tests := []struct {
		name        string
		vars        []string
		baseVars    map[string]string
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name:        "2-variable circular reference A->B->A",
			vars:        []string{"C=%{A}"},
			baseVars:    map[string]string{"A": "%{B}", "B": "%{A}"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrCircularReference)
				var circularErr *config.ErrCircularReferenceDetail
				if assert.ErrorAs(t, err, &circularErr) {
					// Chain should contain evidence of the cycle
					assert.NotEmpty(t, circularErr.Chain)
				}
			},
		},
		{
			name:        "Forward reference - undefined error (sequential processing)",
			vars:        []string{"A=%{B}", "B=%{A}"},
			baseVars:    map[string]string{},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				// With sequential processing, A references undefined B
				assert.ErrorIs(t, err, config.ErrUndefinedVariable)
			},
		},
		{
			name:        "Backward reference - success (sequential processing)",
			vars:        []string{"A=value_a", "B=%{A}"},
			baseVars:    map[string]string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessVars(tt.vars, tt.baseVars, "global")

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, result)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// TestSelfReference_Circular_ThreeOrMore tests circular references with 3+ variables
func TestSelfReference_Circular_ThreeOrMore(t *testing.T) {
	tests := []struct {
		name        string
		vars        []string
		baseVars    map[string]string
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name:        "3変数の循環参照 A->B->C->A",
			vars:        []string{"D=%{A}"},
			baseVars:    map[string]string{"A": "%{B}", "B": "%{C}", "C": "%{A}"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrCircularReference)
			},
		},
		{
			name:        "4変数の循環参照 A->B->C->D->A",
			vars:        []string{"E=%{A}"},
			baseVars:    map[string]string{"A": "%{B}", "B": "%{C}", "C": "%{D}", "D": "%{A}"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrCircularReference)
			},
		},
		{
			name:        "5変数の長い循環参照",
			vars:        []string{"F=%{A}"},
			baseVars:    map[string]string{"A": "%{B}", "B": "%{C}", "C": "%{D}", "D": "%{E}", "E": "%{A}"},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrCircularReference)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessVars(tt.vars, tt.baseVars, "global")

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, result)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// TestSelfReference_CrossLevel tests circular references across levels
func TestSelfReference_CrossLevel(t *testing.T) {
	tests := []struct {
		name        string
		globalVars  []string
		groupVars   []string
		commandVars []string
		expectError bool
		errorType   error
	}{
		{
			name:        "グローバルで循環、グループで参照 (順序処理により未定義)",
			globalVars:  []string{"A=%{B}", "B=%{A}"},
			groupVars:   []string{"C=%{A}"},
			commandVars: []string{},
			expectError: true,
			errorType:   config.ErrUndefinedVariable, // A references undefined B
		},
		{
			name:        "グローバル→グループ→コマンド 正常チェーン",
			globalVars:  []string{"A=value_a"},
			groupVars:   []string{"B=%{A}/sub"},
			commandVars: []string{"C=%{B}/cmd"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					Vars: tt.globalVars,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test_group",
						Vars: tt.groupVars,
						Commands: []runnertypes.Command{
							{
								Name: "test_command",
								Cmd:  "/bin/echo",
								Vars: tt.commandVars,
							},
						},
					},
				},
			}

			// Create environment filter
			filter := environment.NewFilter([]string{})

			// Expand global config
			err := config.ExpandGlobalConfig(&cfg.Global, filter)
			if err != nil && tt.expectError {
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				return
			}
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Expand group config
			err = config.ExpandGroupConfig(&cfg.Groups[0], &cfg.Global, filter)
			if err != nil && tt.expectError {
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				return
			}
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Expand command config
			err = config.ExpandCommandConfig(&cfg.Groups[0].Commands[0], &cfg.Groups[0], &cfg.Global, filter)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSelfReference_ComplexPatterns tests complex circular reference patterns
func TestSelfReference_ComplexPatterns(t *testing.T) {
	tests := []struct {
		name        string
		vars        []string
		baseVars    map[string]string
		expectError bool
		description string
	}{
		{
			name:        "ダイヤモンド型依存 (循環なし)",
			vars:        []string{"A=a", "B=%{A}", "C=%{A}", "D=%{B}_%{C}"},
			baseVars:    map[string]string{},
			expectError: false,
			description: "A is referenced by both B and C, D references both",
		},
		{
			name:        "部分的な循環 - 独立したチェーンと循環チェーン",
			vars:        []string{"X=x", "Y=%{X}", "Z=%{Y}"},
			baseVars:    map[string]string{"A": "%{B}", "B": "%{A}"},
			expectError: false,
			description: "Independent chain X->Y->Z coexists with circular A<->B",
		},
		{
			name:        "複数の値を持つ循環",
			vars:        []string{"C=%{A}"},
			baseVars:    map[string]string{"A": "prefix_%{B}_suffix", "B": "value_%{A}_end"},
			expectError: true,
			description: "Circular reference embedded in string values",
		},
		{
			name:        "自己参照での拡張 (ベース変数あり)",
			vars:        []string{"PATH=%{PATH}:/custom"},
			baseVars:    map[string]string{"PATH": "/usr/bin"},
			expectError: false,
			description: "Extending PATH with itself is allowed",
		},
		{
			name:        "自己参照での拡張 (ベース変数なし)",
			vars:        []string{"PATH=%{PATH}:/custom"},
			baseVars:    map[string]string{},
			expectError: true,
			description: "Self-reference without base value causes circular reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ProcessVars(tt.vars, tt.baseVars, "global")

			if tt.expectError {
				require.Error(t, err, "Expected error for: %s", tt.description)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err, "Unexpected error for: %s", tt.description)
				assert.NotNil(t, result)
			}
		})
	}
}

// TestSelfReference_RecursionDepthLimit tests maximum recursion depth protection
func TestSelfReference_RecursionDepthLimit(t *testing.T) {
	// Create a very long chain in baseVars (already resolved order)
	// This tests runtime expansion depth, not definition order
	baseVars := make(map[string]string)
	for i := 0; i < 100; i++ {
		if i == 99 {
			baseVars[varName(i)] = "final_value"
		} else {
			baseVars[varName(i)] = "%{" + varName(i+1) + "}"
		}
	}

	// Try to reference VAR00 which creates a 100-deep chain
	vars := []string{"RESULT=%{" + varName(0) + "}"}

	result, err := config.ProcessVars(vars, baseVars, "global")

	// Should hit max recursion depth limit during expansion
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, config.ErrMaxRecursionDepthExceeded)
}

// TestSelfReference_EnvExpansion tests circular references in env expansion
func TestSelfReference_EnvExpansion(t *testing.T) {
	tests := []struct {
		name        string
		env         []string
		vars        []string
		expectError bool
	}{
		{
			name:        "envでの循環参照 (varsに依存)",
			env:         []string{"ENV_VAR=%{var_a}"},
			vars:        []string{"var_a=%{var_b}", "var_b=%{var_a}"},
			expectError: true,
		},
		{
			name:        "envでの正常な参照",
			env:         []string{"ENV_VAR=%{var_a}"},
			vars:        []string{"var_a=value"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global := &runnertypes.GlobalConfig{
				Vars: tt.vars,
				Env:  tt.env,
			}

			filter := environment.NewFilter([]string{})
			err := config.ExpandGlobalConfig(global, filter)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// varName returns variable name for index (e.g., varName(0) = "VAR00", varName(99) = "VAR99")
func varName(i int) string {
	if i < 0 || i > 99 {
		return "VAR00"
	}
	tens := i / 10
	ones := i % 10
	return "VAR" + string(rune('0'+tens)) + string(rune('0'+ones))
}
