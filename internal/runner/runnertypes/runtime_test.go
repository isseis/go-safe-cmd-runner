package runnertypes

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeCommand_Name(t *testing.T) {
	tests := []struct {
		name string
		spec *CommandSpec
		want string
	}{
		{
			name: "basic command name",
			spec: &CommandSpec{
				Name: "test-command",
			},
			want: "test-command",
		},
		{
			name: "empty name",
			spec: &CommandSpec{
				Name: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RuntimeCommand{
				Spec: tt.spec,
			}
			got := r.Name()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntimeCommand_RunAsUser(t *testing.T) {
	tests := []struct {
		name string
		spec *CommandSpec
		want string
	}{
		{
			name: "user specified",
			spec: &CommandSpec{
				RunAsUser: "testuser",
			},
			want: "testuser",
		},
		{
			name: "no user specified",
			spec: &CommandSpec{
				RunAsUser: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RuntimeCommand{
				Spec: tt.spec,
			}
			got := r.RunAsUser()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntimeCommand_RunAsGroup(t *testing.T) {
	tests := []struct {
		name string
		spec *CommandSpec
		want string
	}{
		{
			name: "group specified",
			spec: &CommandSpec{
				RunAsGroup: "testgroup",
			},
			want: "testgroup",
		},
		{
			name: "no group specified",
			spec: &CommandSpec{
				RunAsGroup: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RuntimeCommand{
				Spec: tt.spec,
			}
			got := r.RunAsGroup()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntimeCommand_Output(t *testing.T) {
	tests := []struct {
		name string
		spec *CommandSpec
		want string
	}{
		{
			name: "output path specified",
			spec: &CommandSpec{
				OutputFile: "/tmp/output.log",
			},
			want: "/tmp/output.log",
		},
		{
			name: "no output specified",
			spec: &CommandSpec{
				OutputFile: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RuntimeCommand{
				Spec: tt.spec,
			}
			got := r.Output()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntimeCommand_GetRiskLevel(t *testing.T) {
	tests := []struct {
		name    string
		spec    *CommandSpec
		want    RiskLevel
		wantErr bool
	}{
		{
			name: "low risk level",
			spec: &CommandSpec{
				RiskLevel: "low",
			},
			want:    RiskLevelLow,
			wantErr: false,
		},
		{
			name: "medium risk level",
			spec: &CommandSpec{
				RiskLevel: "medium",
			},
			want:    RiskLevelMedium,
			wantErr: false,
		},
		{
			name: "high risk level",
			spec: &CommandSpec{
				RiskLevel: "high",
			},
			want:    RiskLevelHigh,
			wantErr: false,
		},
		{
			name: "empty defaults to low",
			spec: &CommandSpec{
				RiskLevel: "",
			},
			want:    RiskLevelLow,
			wantErr: false,
		},
		{
			name: "invalid risk level",
			spec: &CommandSpec{
				RiskLevel: "invalid",
			},
			want:    RiskLevelUnknown,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RuntimeCommand{
				Spec: tt.spec,
			}
			got, err := r.GetRiskLevel()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntimeCommand_HasUserGroupSpecification(t *testing.T) {
	tests := []struct {
		name string
		spec *CommandSpec
		want bool
	}{
		{
			name: "both user and group specified",
			spec: &CommandSpec{
				RunAsUser:  "testuser",
				RunAsGroup: "testgroup",
			},
			want: true,
		},
		{
			name: "only user specified",
			spec: &CommandSpec{
				RunAsUser:  "testuser",
				RunAsGroup: "",
			},
			want: true,
		},
		{
			name: "only group specified",
			spec: &CommandSpec{
				RunAsUser:  "",
				RunAsGroup: "testgroup",
			},
			want: true,
		},
		{
			name: "neither specified",
			spec: &CommandSpec{
				RunAsUser:  "",
				RunAsGroup: "",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RuntimeCommand{
				Spec: tt.spec,
			}
			got := r.HasUserGroupSpecification()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntimeGlobal_Structure(t *testing.T) {
	// Test that RuntimeGlobal can be created with proper structure
	spec := &GlobalSpec{
		Timeout: commontesting.Int32Ptr(300),
		EnvVars: []string{"PATH=/usr/bin"},
		Vars:    map[string]any{"VAR1": "value1"},
	}

	runtime, err := NewRuntimeGlobal(spec)
	require.NoError(t, err)

	runtime.ExpandedVerifyFiles = []string{"/usr/bin/test"}
	runtime.ExpandedEnv = map[string]string{
		"PATH": "/usr/bin",
	}
	runtime.ExpandedVars = map[string]string{
		"VAR1": "value1",
	}

	// Verify that the structure is properly created
	require.NotNil(t, runtime.Spec)
	require.NotNil(t, runtime.Spec.Timeout)
	assert.Equal(t, int32(300), *runtime.Spec.Timeout)
	require.Len(t, runtime.ExpandedEnv, 1)
	assert.Equal(t, "/usr/bin", runtime.ExpandedEnv["PATH"])
}

func TestRuntimeGroup_Structure(t *testing.T) {
	// Test that RuntimeGroup can be created with proper structure
	spec := &GroupSpec{
		Name:    "test-group",
		WorkDir: "/tmp/test",
		EnvVars: []string{"CC=gcc"},
		Vars:    map[string]any{"BUILD_TYPE": "release"},
	}

	runtime := &RuntimeGroup{
		Spec:                spec,
		ExpandedVerifyFiles: []string{"/usr/bin/gcc"},
		ExpandedEnv: map[string]string{
			"CC": "gcc",
		},
		ExpandedVars: map[string]string{
			"BUILD_TYPE": "release",
		},
		EffectiveWorkDir: "/tmp/test",
		Commands:         []*RuntimeCommand{},
	}

	// Verify that the structure is properly created
	require.NotNil(t, runtime.Spec)
	assert.Equal(t, "test-group", runtime.Spec.Name)
	assert.Equal(t, "/tmp/test", runtime.EffectiveWorkDir)
	require.Len(t, runtime.ExpandedEnv, 1)
}

func TestRuntimeCommand_Structure(t *testing.T) {
	// Test that RuntimeCommand can be created with proper structure
	spec := &CommandSpec{
		Name:    "test-cmd",
		Cmd:     "/usr/bin/echo",
		Args:    []string{"hello", "world"},
		WorkDir: "/tmp",
		Timeout: commontesting.Int32Ptr(60),
		EnvVars: []string{"TEST=value"},
	}

	runtime := &RuntimeCommand{
		Spec:             spec,
		ExpandedCmd:      "/usr/bin/echo",
		ExpandedArgs:     []string{"hello", "world"},
		ExpandedEnv:      map[string]string{"TEST": "value"},
		ExpandedVars:     map[string]string{},
		EffectiveWorkDir: "/tmp",
		EffectiveTimeout: 60,
	}

	// Verify that the structure is properly created
	require.NotNil(t, runtime.Spec)
	assert.Equal(t, "test-cmd", runtime.Name())
	assert.Equal(t, "/usr/bin/echo", runtime.ExpandedCmd)
	require.Len(t, runtime.ExpandedArgs, 2)
	assert.Equal(t, []string{"hello", "world"}, runtime.ExpandedArgs)
	require.Len(t, runtime.ExpandedEnv, 1)
	assert.Equal(t, "value", runtime.ExpandedEnv["TEST"])
	assert.Equal(t, "/tmp", runtime.EffectiveWorkDir)
	assert.Equal(t, int32(60), runtime.EffectiveTimeout)
}

// TestRuntimeCommand_HelperMethods tests the helper methods for RuntimeCommand
func TestRuntimeCommand_HelperMethods(t *testing.T) {
	spec := &CommandSpec{
		Name:    "test-cmd",
		Cmd:     "/usr/bin/echo",
		Args:    []string{"hello", "world"},
		Timeout: commontesting.Int32Ptr(60),
	}

	runtime, err := NewRuntimeCommand(spec, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit(), "test-group")
	require.NoError(t, err)

	// Test Cmd()
	assert.Equal(t, "/usr/bin/echo", runtime.Cmd())

	// Test Args()
	args := runtime.Args()
	require.Len(t, args, 2)
	assert.Equal(t, []string{"hello", "world"}, args)

	// Test Timeout()
	timeout := runtime.Timeout()
	require.True(t, timeout.IsSet())
	assert.Equal(t, int32(60), timeout.Value())
}

// TestRuntimeGlobal_HelperMethods tests the helper methods for RuntimeGlobal
func TestRuntimeGlobal_HelperMethods(t *testing.T) {
	spec := &GlobalSpec{
		Timeout:             commontesting.Int32Ptr(300),
		EnvAllowed:          []string{"PATH", "HOME"},
		VerifyStandardPaths: commontesting.BoolPtr(false),
	}

	runtime, err := NewRuntimeGlobal(spec)
	require.NoError(t, err, "NewRuntimeGlobal() should succeed")

	// Test Timeout()
	timeout := runtime.Timeout()
	require.True(t, timeout.IsSet())
	assert.Equal(t, int32(300), timeout.Value())

	// Test EnvAllowlist()
	allowlist := runtime.EnvAllowlist()
	require.Len(t, allowlist, 2)
	assert.Equal(t, "PATH", allowlist[0])
	assert.Equal(t, "HOME", allowlist[1])

	// Test SkipStandardPaths()
	assert.True(t, runtime.SkipStandardPaths())
}

// TestRuntimeGlobal_TimeoutDefault tests that Timeout() returns default value when not set
func TestRuntimeGlobal_TimeoutDefault(t *testing.T) {
	spec := &GlobalSpec{
		Timeout: nil, // Not set in TOML
	}

	runtime, err := NewRuntimeGlobal(spec)
	require.NoError(t, err)

	// Test Timeout() is unset (caller should use DefaultTimeout)
	timeout := runtime.Timeout()
	assert.False(t, timeout.IsSet())
}

// TestRuntimeGlobal_SkipStandardPaths_WithNil tests SkipStandardPaths with nil value
// This also indirectly tests the determineVerifyStandardPaths helper function
func TestRuntimeGlobal_SkipStandardPaths_WithNil(t *testing.T) {
	tests := []struct {
		name                string
		verifyStandardPaths *bool
		wantSkip            bool
	}{
		{
			name:                "nil value defaults to verify (skip=false)",
			verifyStandardPaths: nil,
			wantSkip:            false, // don't skip = verify
		},
		{
			name:                "explicit true (verify) means skip=false",
			verifyStandardPaths: commontesting.BoolPtr(true),
			wantSkip:            false,
		},
		{
			name:                "explicit false (don't verify) means skip=true",
			verifyStandardPaths: commontesting.BoolPtr(false),
			wantSkip:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &RuntimeGlobal{
				Spec: &GlobalSpec{
					VerifyStandardPaths: tt.verifyStandardPaths,
				},
			}
			got := runtime.SkipStandardPaths()
			assert.Equal(t, tt.wantSkip, got)
		})
	}
}

// TestNewRuntimeGroup tests the NewRuntimeGroup constructor
func TestNewRuntimeGroup(t *testing.T) {
	tests := []struct {
		name    string
		spec    *GroupSpec
		wantErr error
	}{
		{
			name: "valid spec",
			spec: &GroupSpec{
				Name:    "test-group",
				WorkDir: "/tmp/test",
			},
			wantErr: nil,
		},
		{
			name:    "nil spec",
			spec:    nil,
			wantErr: ErrNilSpec,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRuntimeGroup(tt.spec)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.spec, got.Spec)
				assert.NotNil(t, got.ExpandedVerifyFiles)
				assert.NotNil(t, got.ExpandedEnv)
				assert.NotNil(t, got.ExpandedVars)
				assert.NotNil(t, got.Commands)
			}
		})
	}
}

// TestRuntimeGroup_Name tests the RuntimeGroup.Name method
func TestRuntimeGroup_Name(t *testing.T) {
	tests := []struct {
		name string
		spec *GroupSpec
		want string
	}{
		{
			name: "basic group name",
			spec: &GroupSpec{
				Name: "test-group",
			},
			want: "test-group",
		},
		{
			name: "empty name",
			spec: &GroupSpec{
				Name: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RuntimeGroup{
				Spec: tt.spec,
			}
			got := r.Name()
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRuntimeGroup_Name_Panic tests that Name panics with nil receiver or spec
func TestRuntimeGroup_Name_Panic(t *testing.T) {
	tests := []struct {
		name        string
		runtimeGrp  *RuntimeGroup
		wantPanic   bool
		panicSubstr string
	}{
		{
			name:        "nil receiver",
			runtimeGrp:  nil,
			wantPanic:   true,
			panicSubstr: "RuntimeGroup.Name: nil receiver or Spec (programming error - use NewRuntimeGroup)",
		},
		{
			name: "nil spec",
			runtimeGrp: &RuntimeGroup{
				Spec: nil,
			},
			wantPanic:   true,
			panicSubstr: "RuntimeGroup.Name: nil receiver or Spec (programming error - use NewRuntimeGroup)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.PanicsWithValue(t, tt.panicSubstr, func() {
					tt.runtimeGrp.Name()
				})
			}
		})
	}
}

// TestRuntimeGroup_WorkDir tests the RuntimeGroup.WorkDir method
func TestRuntimeGroup_WorkDir(t *testing.T) {
	tests := []struct {
		name string
		spec *GroupSpec
		want string
	}{
		{
			name: "basic work directory",
			spec: &GroupSpec{
				WorkDir: "/tmp/test",
			},
			want: "/tmp/test",
		},
		{
			name: "empty work directory",
			spec: &GroupSpec{
				WorkDir: "",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RuntimeGroup{
				Spec: tt.spec,
			}
			got := r.WorkDir()
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRuntimeGroup_WorkDir_Panic tests that WorkDir panics with nil receiver or spec
func TestRuntimeGroup_WorkDir_Panic(t *testing.T) {
	tests := []struct {
		name        string
		runtimeGrp  *RuntimeGroup
		wantPanic   bool
		panicSubstr string
	}{
		{
			name:        "nil receiver",
			runtimeGrp:  nil,
			wantPanic:   true,
			panicSubstr: "RuntimeGroup.WorkDir: nil receiver or Spec (programming error - use NewRuntimeGroup)",
		},
		{
			name: "nil spec",
			runtimeGrp: &RuntimeGroup{
				Spec: nil,
			},
			wantPanic:   true,
			panicSubstr: "RuntimeGroup.WorkDir: nil receiver or Spec (programming error - use NewRuntimeGroup)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.PanicsWithValue(t, tt.panicSubstr, func() {
					tt.runtimeGrp.WorkDir()
				})
			}
		})
	}
}

// TestExtractGroupName tests the ExtractGroupName helper function
func TestExtractGroupName(t *testing.T) {
	t.Run("valid RuntimeGroup with name", func(t *testing.T) {
		runtimeGrp := &RuntimeGroup{
			Spec: &GroupSpec{
				Name: "test-group",
			},
		}
		got := ExtractGroupName(runtimeGrp)
		assert.Equal(t, "test-group", got)
	})

	t.Run("empty group name is allowed", func(t *testing.T) {
		runtimeGrp := &RuntimeGroup{
			Spec: &GroupSpec{
				Name: "",
			},
		}
		got := ExtractGroupName(runtimeGrp)
		assert.Equal(t, "", got)
	})

	t.Run("panic on nil RuntimeGroup", func(t *testing.T) {
		assert.Panics(t, func() {
			ExtractGroupName(nil)
		}, "ExtractGroupName should panic when runtimeGroup is nil")
	})

	t.Run("panic on RuntimeGroup with nil Spec", func(t *testing.T) {
		runtimeGrp := &RuntimeGroup{
			Spec: nil,
		}
		assert.Panics(t, func() {
			ExtractGroupName(runtimeGrp)
		}, "ExtractGroupName should panic when runtimeGroup.Spec is nil")
	})
}

// TestNewRuntimeGlobal_NilSpec tests that NewRuntimeGlobal returns error for nil spec
func TestNewRuntimeGlobal_NilSpec(t *testing.T) {
	got, err := NewRuntimeGlobal(nil)
	require.ErrorIs(t, err, ErrNilSpec)
	assert.Nil(t, got)
}

// TestRuntimeGlobal_Timeout_Panic tests that Timeout panics with nil receiver or spec
func TestRuntimeGlobal_Timeout_Panic(t *testing.T) {
	tests := []struct {
		name       string
		runtime    *RuntimeGlobal
		wantPanic  bool
		panicValue string
	}{
		{
			name:       "nil receiver",
			runtime:    nil,
			wantPanic:  true,
			panicValue: "RuntimeGlobal.Timeout: nil receiver or Spec (programming error - use NewRuntimeGlobal)",
		},
		{
			name: "nil spec",
			runtime: &RuntimeGlobal{
				Spec: nil,
			},
			wantPanic:  true,
			panicValue: "RuntimeGlobal.Timeout: nil receiver or Spec (programming error - use NewRuntimeGlobal)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.PanicsWithValue(t, tt.panicValue, func() {
					tt.runtime.Timeout()
				})
			}
		})
	}
}

// TestRuntimeGlobal_EnvAllowlist_Panic tests that EnvAllowlist panics with nil receiver or spec
func TestRuntimeGlobal_EnvAllowlist_Panic(t *testing.T) {
	tests := []struct {
		name       string
		runtime    *RuntimeGlobal
		wantPanic  bool
		panicValue string
	}{
		{
			name:       "nil receiver",
			runtime:    nil,
			wantPanic:  true,
			panicValue: "RuntimeGlobal.EnvAllowed: nil receiver or Spec (programming error - use NewRuntimeGlobal)",
		},
		{
			name: "nil spec",
			runtime: &RuntimeGlobal{
				Spec: nil,
			},
			wantPanic:  true,
			panicValue: "RuntimeGlobal.EnvAllowed: nil receiver or Spec (programming error - use NewRuntimeGlobal)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.PanicsWithValue(t, tt.panicValue, func() {
					tt.runtime.EnvAllowlist()
				})
			}
		})
	}
}

// TestRuntimeGlobal_SkipStandardPaths_Panic tests that SkipStandardPaths panics with nil receiver or spec
func TestRuntimeGlobal_SkipStandardPaths_Panic(t *testing.T) {
	tests := []struct {
		name       string
		runtime    *RuntimeGlobal
		wantPanic  bool
		panicValue string
	}{
		{
			name:       "nil receiver",
			runtime:    nil,
			wantPanic:  true,
			panicValue: "RuntimeGlobal.SkipStandardPaths: nil receiver or Spec (programming error - use NewRuntimeGlobal)",
		},
		{
			name: "nil spec",
			runtime: &RuntimeGlobal{
				Spec: nil,
			},
			wantPanic:  true,
			panicValue: "RuntimeGlobal.SkipStandardPaths: nil receiver or Spec (programming error - use NewRuntimeGlobal)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.PanicsWithValue(t, tt.panicValue, func() {
					tt.runtime.SkipStandardPaths()
				})
			}
		})
	}
}
