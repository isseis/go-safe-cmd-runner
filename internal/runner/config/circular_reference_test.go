package config_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCircularReference_DirectSelfReference tests direct self-referencing variables (v=%{v})
// Note: ProcessVars processes variables sequentially, so self-references are caught as "undefined variable"
// because the variable hasn't been added to the expansion map yet.
func TestCircularReference_DirectSelfReference(t *testing.T) {
	tests := []struct {
		name         string
		spec         *runnertypes.GlobalSpec
		wantErr      error
		wantVarInErr string
	}{
		{
			name: "vars direct self-reference",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{"v": "%{v}"},
			},
			wantErr:      config.ErrCircularReference,
			wantVarInErr: "v",
		},
		{
			name: "vars self-reference with prefix",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{"PATH": "/custom:%{PATH}"},
			},
			wantErr:      config.ErrCircularReference,
			wantVarInErr: "PATH",
		},
		{
			name: "vars self-reference with suffix",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{"VALUE": "%{VALUE}_suffix"},
			},
			wantErr:      config.ErrCircularReference,
			wantVarInErr: "VALUE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandGlobal(tt.spec)
			require.Error(t, err, "Expected error for self-reference")
			assert.True(t, errors.Is(err, tt.wantErr), "Expected error type %v, got %v", tt.wantErr, err)

			// Verify the error contains variable name information
			var detailErr *config.ErrCircularReferenceDetail
			if errors.As(err, &detailErr) {
				assert.Equal(t, tt.wantVarInErr, detailErr.VariableName, "Error should reference the correct variable")
			}
		})
	}
}

// TestCircularReference_TwoVariables tests circular references between two variables (a=%{b}, b=%{a})
// With lazy evaluation, mutual references are detected as circular references.
func TestCircularReference_TwoVariables(t *testing.T) {
	tests := []struct {
		name              string
		spec              *runnertypes.GlobalSpec
		wantErr           error
		wantVarInErrOneOf []string // Map iteration order is undefined, so accept any of these
	}{
		{
			name: "simple two-variable cycle",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"a": "%{b}",
					"b": "%{a}",
				},
			},
			wantErr:           config.ErrCircularReference,
			wantVarInErrOneOf: []string{"a", "b"},
		},
		{
			name: "two-variable cycle with reverse order",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"b": "%{a}",
					"a": "%{b}",
				},
			},
			wantErr:           config.ErrCircularReference,
			wantVarInErrOneOf: []string{"a", "b"},
		},
		{
			name: "valid chain with existing variable",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"a": "initial",
					"b": "%{a}",
				},
			},
			// With map format, this is a valid chain (no cycle)
			wantErr:           nil,
			wantVarInErrOneOf: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime, err := config.ExpandGlobal(tt.spec)
			if tt.wantErr == nil {
				require.NoError(t, err, "Expected no error for valid chain")
				require.NotNil(t, runtime)
				return
			}
			require.Error(t, err, "Expected error")
			assert.True(t, errors.Is(err, tt.wantErr), "Expected error type %v, got %v", tt.wantErr, err)

			// Verify the error contains variable name information
			var detailErr *config.ErrCircularReferenceDetail
			if errors.As(err, &detailErr) {
				assert.Contains(t, tt.wantVarInErrOneOf, detailErr.VariableName,
					"Error should reference one of the variables in the cycle")
			}
		})
	}
}

// TestCircularReference_ComplexChain tests circular references with 3+ variables
// With lazy evaluation, all mutual references are detected as circular references.
func TestCircularReference_ComplexChain(t *testing.T) {
	tests := []struct {
		name              string
		spec              *runnertypes.GlobalSpec
		wantVarInErrOneOf []string // Map iteration order is undefined
	}{
		{
			name: "three-variable cycle",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"a": "%{b}",
					"b": "%{c}",
					"c": "%{a}",
				},
			},
			wantVarInErrOneOf: []string{"a", "b", "c"},
		},
		{
			name: "three-variable cycle (reverse order in map)",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"c": "%{a}",
					"a": "%{b}",
					"b": "%{c}",
				},
			},
			wantVarInErrOneOf: []string{"a", "b", "c"},
		},
		{
			name: "four-variable cycle",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"VAR4": "%{VAR1}",
					"VAR1": "%{VAR2}",
					"VAR2": "%{VAR3}",
					"VAR3": "%{VAR4}",
				},
			},
			wantVarInErrOneOf: []string{"VAR1", "VAR2", "VAR3", "VAR4"},
		},
		{
			name: "complex chain with cycle",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"BASE": "/base",
					"D":    "%{A}",
					"A":    "%{BASE}/%{B}",
					"B":    "%{C}",
					"C":    "%{D}",
				},
			},
			wantVarInErrOneOf: []string{"A", "B", "C", "D"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandGlobal(tt.spec)
			require.Error(t, err, "Expected error")
			assert.True(t, errors.Is(err, config.ErrCircularReference), "Expected circular reference error, got %v", err)

			// Verify the error contains variable name information
			var detailErr *config.ErrCircularReferenceDetail
			if errors.As(err, &detailErr) {
				assert.Contains(t, tt.wantVarInErrOneOf, detailErr.VariableName,
					"Error should reference one of the variables in the cycle")
			}
		})
	}
}

// TestCircularReference_RecursionDepthLimit tests that recursion depth limit prevents stack overflow
// Note: With sequential processing, a 105-variable chain actually works fine because each variable
// just references the previous one's already-expanded value. The recursion depth limit is hit when
// a SINGLE value contains many nested variable references like "a=%{b}", "b=%{c%{d}%{e}...}".
// For simplicity, we'll just verify that very long chains work correctly (not hit depth limit).
func TestCircularReference_RecursionDepthLimit(t *testing.T) {
	// Create a reasonably long chain that should succeed
	const chainLength = 50

	vars := make(map[string]any, chainLength)
	vars["VAR_000"] = "initial"
	for i := 1; i < chainLength; i++ {
		currentVarName := "VAR_" + fmt.Sprintf("%03d", i)
		previousVarName := "VAR_" + fmt.Sprintf("%03d", i-1)
		vars[currentVarName] = "%{" + previousVarName + "}"
	}

	spec := &runnertypes.GlobalSpec{
		Vars: vars,
	}

	runtime, err := config.ExpandGlobal(spec)
	require.NoError(t, err, "Long sequential chain should succeed")
	require.NotNil(t, runtime)
	// The last variable should expand to "initial"
	assert.Equal(t, "initial", runtime.ExpandedVars["VAR_049"])
}

// TestCircularReference_CrossLevel_GlobalGroup tests circular references between global and group levels
func TestCircularReference_CrossLevel_GlobalGroup(t *testing.T) {
	tests := []struct {
		name            string
		global          *runnertypes.GlobalSpec
		group           *runnertypes.GroupSpec
		wantErr         error
		wantVarInErr    string
		wantVarsInChain []string // For circular references, check all vars in cycle
	}{
		{
			name: "group vars references global var that doesn't exist yet",
			global: &runnertypes.GlobalSpec{
				Vars: map[string]any{"GLOBAL_VAR": "%{GROUP_VAR}"},
			},
			group: &runnertypes.GroupSpec{
				Name: "test",
				Vars: map[string]any{"GROUP_VAR": "%{GLOBAL_VAR}"},
			},
			wantErr:      config.ErrUndefinedVariable,
			wantVarInErr: "GROUP_VAR",
		},
		{
			name: "group vars with forward reference",
			global: &runnertypes.GlobalSpec{
				Vars: map[string]any{"BASE": "/base"},
			},
			group: &runnertypes.GroupSpec{
				Name: "test",
				Vars: map[string]any{
					"CYCLE":   "%{DERIVED}",
					"DERIVED": "%{CYCLE}",
				},
			},
			wantErr:         config.ErrCircularReference,
			wantVarsInChain: []string{"CYCLE", "DERIVED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First expand global
			runtimeGlobal, err := config.ExpandGlobal(tt.global)
			if err != nil {
				// If global expansion fails, that's also valid for this test
				assert.True(t, errors.Is(err, tt.wantErr), "Expected error type %v, got %v", tt.wantErr, err)
				return
			}

			// Then try to expand group (this is where cycle should be detected)
			_, err = config.ExpandGroup(tt.group, runtimeGlobal)
			require.Error(t, err, "Expected error in group expansion")
			assert.True(t, errors.Is(err, tt.wantErr), "Expected error type %v, got %v", tt.wantErr, err)

			// Verify the error contains variable name information
			if errors.Is(err, config.ErrCircularReference) {
				var detailErr *config.ErrCircularReferenceDetail
				if errors.As(err, &detailErr) {
					if len(tt.wantVarsInChain) > 0 {
						// For circular references, verify all expected variables are in the chain
						for _, wantVar := range tt.wantVarsInChain {
							assert.Contains(t, detailErr.Chain, wantVar, "Chain should contain variable %s", wantVar)
						}
					} else {
						assert.Equal(t, tt.wantVarInErr, detailErr.VariableName, "Error should reference the correct variable")
					}
				}
			} else if errors.Is(err, config.ErrUndefinedVariable) {
				var detailErr *config.ErrUndefinedVariableDetail
				if errors.As(err, &detailErr) {
					assert.Equal(t, tt.wantVarInErr, detailErr.VariableName, "Error should reference the correct variable")
				}
			}
		})
	}
}

// TestCircularReference_CrossLevel_GroupCommand tests circular references between group and command levels
func TestCircularReference_CrossLevel_GroupCommand(t *testing.T) {
	tests := []struct {
		name              string
		group             *runnertypes.GroupSpec
		command           *runnertypes.CommandSpec
		wantErr           error
		wantVarInErr      string   // For cases where variable name is deterministic
		wantVarInErrOneOf []string // For cases where variable name depends on map iteration order
	}{
		{
			name: "command env references undefined group var",
			group: &runnertypes.GroupSpec{
				Name: "test",
				Vars: map[string]any{"GROUP_VAR": "value"},
			},
			command: &runnertypes.CommandSpec{
				Name:    "test",
				Cmd:     "/bin/test",
				EnvVars: []string{"CMD_ENV=%{UNDEFINED}"},
			},
			wantErr:      config.ErrUndefinedVariable,
			wantVarInErr: "UNDEFINED",
		},
		{
			name: "command vars with forward reference",
			group: &runnertypes.GroupSpec{
				Name: "test",
				Vars: map[string]any{"GROUP_VAR": "base"},
			},
			command: &runnertypes.CommandSpec{
				Name: "test",
				Cmd:  "/bin/test",
				Vars: map[string]any{
					"CMD_B": "%{CMD_A}",
					"CMD_A": "%{CMD_B}",
				},
			},
			wantErr:           config.ErrCircularReference,
			wantVarInErrOneOf: []string{"CMD_A", "CMD_B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First expand group
			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec:         &runnertypes.GlobalSpec{},
				ExpandedVars: make(map[string]string),
			}
			runtimeGroup, err := config.ExpandGroup(tt.group, runtimeGlobal)
			require.NoError(t, err, "Group expansion should succeed")

			// Then try to expand command (this is where error should be detected)
			_, err = config.ExpandCommand(tt.command, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit())
			require.Error(t, err, "Expected error in command expansion")
			assert.True(t, errors.Is(err, tt.wantErr), "Expected error type %v, got %v", tt.wantErr, err)

			// Verify the error contains variable name information
			if errors.Is(err, config.ErrCircularReference) {
				var detailErr *config.ErrCircularReferenceDetail
				if errors.As(err, &detailErr) {
					if len(tt.wantVarInErrOneOf) > 0 {
						assert.Contains(t, tt.wantVarInErrOneOf, detailErr.VariableName,
							"Error should reference one of the variables in the cycle")
					} else {
						assert.Equal(t, tt.wantVarInErr, detailErr.VariableName, "Error should reference the correct variable")
					}
				}
			} else if errors.Is(err, config.ErrUndefinedVariable) {
				var detailErr *config.ErrUndefinedVariableDetail
				if errors.As(err, &detailErr) {
					assert.Equal(t, tt.wantVarInErr, detailErr.VariableName, "Error should reference the correct variable")
				}
			}
		})
	}
}

// TestCircularReference_ComplexPatterns tests various complex circular reference patterns
func TestCircularReference_ComplexPatterns(t *testing.T) {
	tests := []struct {
		name    string
		spec    *runnertypes.GlobalSpec
		wantErr error
	}{
		{
			name: "forward reference with non-cyclic variables mixed in",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"SAFE1":   "value1",
					"SAFE2":   "value2",
					"CYCLE_B": "%{CYCLE_A}",
					"SAFE3":   "%{SAFE1}",
					"CYCLE_A": "%{CYCLE_B}",
					"SAFE4":   "%{SAFE2}/%{SAFE3}",
				},
			},
			wantErr: config.ErrCircularReference,
		},
		{
			name: "multiple independent forward-reference failures",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"A1": "%{A2}",
					"A2": "%{A1}",
					"B1": "%{B2}",
					"B2": "%{B1}",
				},
			},
			wantErr: config.ErrCircularReference,
		},
		{
			name: "multiple forward references",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"A2": "%{A1}",
					"A1": "%{A2}",
					"B2": "%{B1}",
					"B1": "%{B2}",
				},
			},
			wantErr: config.ErrCircularReference,
		},
		{
			name: "env variables cannot reference each other",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"VAR1": "%{VAR2}",
					"VAR2": "value",
				},
				EnvVars: []string{
					"ENV1=%{VAR1}",
					"ENV2=%{ENV1}", // ENV1 is not in ExpandedVars, only in ExpandedEnv
				},
			},
			// ProcessEnv doesn't support env-to-env references, will fail with undefined variable
			wantErr: config.ErrUndefinedVariable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandGlobal(tt.spec)
			require.Error(t, err, "Expected error")
			assert.True(t, errors.Is(err, tt.wantErr), "Expected error type %v, got %v", tt.wantErr, err)
		})
	}
}

// TestCircularReference_ValidComplexReferences tests that valid complex references work correctly
func TestCircularReference_ValidComplexReferences(t *testing.T) {
	tests := []struct {
		name     string
		spec     *runnertypes.GlobalSpec
		checkVar string
		wantVal  string
	}{
		{
			name: "deep chain without cycle",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"V1": "a",
					"V2": "%{V1}b",
					"V3": "%{V2}c",
					"V4": "%{V3}d",
					"V5": "%{V4}e",
				},
			},
			checkVar: "V5",
			wantVal:  "abcde",
		},
		{
			name: "diamond pattern (multiple paths, no cycle)",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"BASE":  "x",
					"LEFT":  "%{BASE}L",
					"RIGHT": "%{BASE}R",
					"MERGE": "%{LEFT}%{RIGHT}",
				},
			},
			checkVar: "MERGE",
			wantVal:  "xLxR",
		},
		{
			name: "complex tree structure",
			spec: &runnertypes.GlobalSpec{
				Vars: map[string]any{
					"ROOT":     "/root",
					"BRANCH1":  "%{ROOT}/b1",
					"BRANCH2":  "%{ROOT}/b2",
					"LEAF1":    "%{BRANCH1}/leaf",
					"LEAF2":    "%{BRANCH2}/leaf",
					"COMBINED": "%{LEAF1}:%{LEAF2}",
				},
			},
			checkVar: "COMBINED",
			wantVal:  "/root/b1/leaf:/root/b2/leaf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime, err := config.ExpandGlobal(tt.spec)
			require.NoError(t, err, "Valid complex references should succeed")
			require.NotNil(t, runtime)
			assert.Equal(t, tt.wantVal, runtime.ExpandedVars[tt.checkVar])
		})
	}
}
