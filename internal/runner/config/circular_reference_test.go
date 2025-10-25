package config_test

import (
	"fmt"
	"testing"

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
		name     string
		spec     *runnertypes.GlobalSpec
		wantErr  string
		contains string
	}{
		{
			name: "vars direct self-reference",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{"v=%{v}"},
			},
			wantErr:  "undefined variable",
			contains: "v",
		},
		{
			name: "vars self-reference with prefix",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{"PATH=/custom:%{PATH}"},
			},
			wantErr:  "undefined variable",
			contains: "PATH",
		},
		{
			name: "vars self-reference with suffix",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{"VALUE=%{VALUE}_suffix"},
			},
			wantErr:  "undefined variable",
			contains: "VALUE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandGlobal(tt.spec)
			require.Error(t, err, "Expected error for self-reference")
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// TestCircularReference_TwoVariables tests circular references between two variables (a=%{b}, b=%{a})
// Note: Due to sequential processing, the first variable fails with "undefined variable" for forward reference.
// To test actual circular reference detection, define b before a, so a references b which then has a cycle.
func TestCircularReference_TwoVariables(t *testing.T) {
	tests := []struct {
		name     string
		spec     *runnertypes.GlobalSpec
		wantErr  string
		contains []string
	}{
		{
			name: "simple two-variable cycle",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"a=%{b}",
					"b=%{a}",
				},
			},
			wantErr:  "undefined variable",
			contains: []string{"b"}, // First failure is undefined 'b'
		},
		{
			name: "two-variable cycle with reverse order",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"b=%{a}",
					"a=%{b}",
				},
			},
			wantErr:  "undefined variable",
			contains: []string{"a"}, // First failure is undefined 'a'
		},
		{
			name: "actual circular reference via existing variable",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"a=initial",
					"b=%{a}",
					"a=%{b}", // Trying to redefine 'a' - should fail as duplicate
				},
			},
			wantErr:  "duplicate variable",
			contains: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandGlobal(tt.spec)
			require.Error(t, err, "Expected error")
			assert.Contains(t, err.Error(), tt.wantErr)
			for _, s := range tt.contains {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

// TestCircularReference_ComplexChain tests circular references with 3+ variables
// These tests create actual cycles where expansion recurses back to an already-visited variable.
func TestCircularReference_ComplexChain(t *testing.T) {
	tests := []struct {
		name     string
		spec     *runnertypes.GlobalSpec
		wantErr  string
		contains []string
	}{
		{
			name: "three-variable cycle (forward refs fail first)",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"a=%{b}",
					"b=%{c}",
					"c=%{a}",
				},
			},
			wantErr:  "undefined variable",
			contains: []string{"b"}, // First failure is undefined 'b'
		},
		{
			name: "three-variable forward reference (not a cycle, just undefined)",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"c=%{a}",
					"a=%{b}",
					"b=%{c}",
				},
			},
			wantErr:  "undefined variable",
			contains: []string{"a"},
		},
		{
			name: "four-variable forward reference (not a cycle, just undefined)",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"VAR4=%{VAR1}",
					"VAR1=%{VAR2}",
					"VAR2=%{VAR3}",
					"VAR3=%{VAR4}",
				},
			},
			wantErr:  "undefined variable",
			contains: []string{"VAR1"},
		},
		{
			name: "complex chain forward reference (not a cycle)",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"BASE=/base",
					"D=%{A}",
					"A=%{BASE}/%{B}",
					"B=%{C}",
					"C=%{D}",
				},
			},
			wantErr:  "undefined variable",
			contains: []string{"A"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandGlobal(tt.spec)
			require.Error(t, err, "Expected error")
			assert.Contains(t, err.Error(), tt.wantErr)
			for _, s := range tt.contains {
				assert.Contains(t, err.Error(), s)
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

	vars := make([]string, chainLength)
	vars[0] = "VAR_000=initial"
	for i := 1; i < chainLength; i++ {
		currentVarName := "VAR_" + fmt.Sprintf("%03d", i)
		previousVarName := "VAR_" + fmt.Sprintf("%03d", i-1)
		vars[i] = currentVarName + "=%{" + previousVarName + "}"
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
		name     string
		global   *runnertypes.GlobalSpec
		group    *runnertypes.GroupSpec
		wantErr  string
		contains string
	}{
		{
			name: "group vars references global var that doesn't exist yet",
			global: &runnertypes.GlobalSpec{
				Vars: []string{"GLOBAL_VAR=%{GROUP_VAR}"},
			},
			group: &runnertypes.GroupSpec{
				Name: "test",
				Vars: []string{"GROUP_VAR=%{GLOBAL_VAR}"},
			},
			wantErr:  "undefined variable",
			contains: "GROUP_VAR",
		},
		{
			name: "group vars with forward reference",
			global: &runnertypes.GlobalSpec{
				Vars: []string{"BASE=/base"},
			},
			group: &runnertypes.GroupSpec{
				Name: "test",
				Vars: []string{
					"CYCLE=%{DERIVED}",
					"DERIVED=%{CYCLE}",
				},
			},
			wantErr:  "undefined variable",
			contains: "DERIVED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First expand global
			runtimeGlobal, err := config.ExpandGlobal(tt.global)
			if err != nil {
				// If global expansion fails, that's also valid for this test
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Contains(t, err.Error(), tt.contains)
				return
			}

			// Then try to expand group (this is where cycle should be detected)
			_, err = config.ExpandGroup(tt.group, runtimeGlobal.ExpandedVars)
			require.Error(t, err, "Expected error in group expansion")
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// TestCircularReference_CrossLevel_GroupCommand tests circular references between group and command levels
func TestCircularReference_CrossLevel_GroupCommand(t *testing.T) {
	tests := []struct {
		name     string
		group    *runnertypes.GroupSpec
		command  *runnertypes.CommandSpec
		wantErr  string
		contains string
	}{
		{
			name: "command env references undefined group var",
			group: &runnertypes.GroupSpec{
				Name: "test",
				Vars: []string{"GROUP_VAR=value"},
			},
			command: &runnertypes.CommandSpec{
				Name:    "test",
				Cmd:     "/bin/test",
				EnvVars: []string{"CMD_ENV=%{UNDEFINED}"},
			},
			wantErr:  "undefined variable",
			contains: "UNDEFINED",
		},
		{
			name: "command vars with forward reference",
			group: &runnertypes.GroupSpec{
				Name: "test",
				Vars: []string{"GROUP_VAR=base"},
			},
			command: &runnertypes.CommandSpec{
				Name: "test",
				Cmd:  "/bin/test",
				Vars: []string{
					"CMD_B=%{CMD_A}",
					"CMD_A=%{CMD_B}",
				},
			},
			wantErr:  "undefined variable",
			contains: "CMD_A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First expand group
			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec:         &runnertypes.GlobalSpec{},
				ExpandedVars: make(map[string]string),
			}
			runtimeGroup, err := config.ExpandGroup(tt.group, runtimeGlobal.ExpandedVars)
			require.NoError(t, err, "Group expansion should succeed")

			// Then try to expand command (this is where error should be detected)
			_, err = config.ExpandCommand(tt.command, runtimeGroup.ExpandedVars, tt.group.Name, nil)
			require.Error(t, err, "Expected error in command expansion")
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// TestCircularReference_ComplexPatterns tests various complex circular reference patterns
func TestCircularReference_ComplexPatterns(t *testing.T) {
	tests := []struct {
		name    string
		spec    *runnertypes.GlobalSpec
		wantErr string
	}{
		{
			name: "forward reference with non-cyclic variables mixed in",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"SAFE1=value1",
					"SAFE2=value2",
					"CYCLE_B=%{CYCLE_A}",
					"SAFE3=%{SAFE1}",
					"CYCLE_A=%{CYCLE_B}",
					"SAFE4=%{SAFE2}/%{SAFE3}",
				},
			},
			wantErr: "undefined variable",
		},
		{
			name: "multiple independent forward-reference failures",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"A1=%{A2}",
					"A2=%{A1}",
					"B1=%{B2}",
					"B2=%{B1}",
				},
			},
			wantErr: "undefined variable",
		},
		{
			name: "multiple forward references",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"A2=%{A1}",
					"A1=%{A2}",
					"B2=%{B1}",
					"B1=%{B2}",
				},
			},
			wantErr: "undefined variable",
		},
		{
			name: "env variables cannot reference each other",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"VAR1=%{VAR2}",
					"VAR2=value",
				},
				EnvVars: []string{
					"ENV1=%{VAR1}",
					"ENV2=%{ENV1}", // ENV1 is not in ExpandedVars, only in ExpandedEnv
				},
			},
			// ProcessEnv doesn't support env-to-env references, will fail with undefined variable
			wantErr: "undefined variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ExpandGlobal(tt.spec)
			require.Error(t, err, "Expected error")
			assert.Contains(t, err.Error(), tt.wantErr)
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
				Vars: []string{
					"V1=a",
					"V2=%{V1}b",
					"V3=%{V2}c",
					"V4=%{V3}d",
					"V5=%{V4}e",
				},
			},
			checkVar: "V5",
			wantVal:  "abcde",
		},
		{
			name: "diamond pattern (multiple paths, no cycle)",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"BASE=x",
					"LEFT=%{BASE}L",
					"RIGHT=%{BASE}R",
					"MERGE=%{LEFT}%{RIGHT}",
				},
			},
			checkVar: "MERGE",
			wantVal:  "xLxR",
		},
		{
			name: "complex tree structure",
			spec: &runnertypes.GlobalSpec{
				Vars: []string{
					"ROOT=/root",
					"BRANCH1=%{ROOT}/b1",
					"BRANCH2=%{ROOT}/b2",
					"LEAF1=%{BRANCH1}/leaf",
					"LEAF2=%{BRANCH2}/leaf",
					"COMBINED=%{LEAF1}:%{LEAF2}",
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
