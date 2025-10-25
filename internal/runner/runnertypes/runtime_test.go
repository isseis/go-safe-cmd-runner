package runnertypes

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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

func TestRuntimeCommand_GetMaxRiskLevel(t *testing.T) {
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
			got, err := r.GetMaxRiskLevel()
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
		Timeout:  common.IntPtr(300),
		LogLevel: "debug",
		EnvVars:  []string{"PATH=/usr/bin"},
		Vars:     []string{"VAR1=value1"},
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
	assert.Equal(t, 300, *runtime.Spec.Timeout)
	require.Len(t, runtime.ExpandedEnv, 1)
	assert.Equal(t, "/usr/bin", runtime.ExpandedEnv["PATH"])
}

func TestRuntimeGroup_Structure(t *testing.T) {
	// Test that RuntimeGroup can be created with proper structure
	spec := &GroupSpec{
		Name:    "test-group",
		WorkDir: "/tmp/test",
		EnvVars: []string{"CC=gcc"},
		Vars:    []string{"BUILD_TYPE=release"},
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
		Timeout: common.IntPtr(60),
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
	assert.Equal(t, 60, runtime.EffectiveTimeout)
}

// TestRuntimeCommand_HelperMethods tests the helper methods for RuntimeCommand
func TestRuntimeCommand_HelperMethods(t *testing.T) {
	spec := &CommandSpec{
		Name:    "test-cmd",
		Cmd:     "/usr/bin/echo",
		Args:    []string{"hello", "world"},
		Timeout: common.IntPtr(60),
	}

	runtime, err := NewRuntimeCommand(spec, common.NewUnsetTimeout())
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
	assert.Equal(t, 60, timeout.Value())
}

// TestRuntimeGlobal_HelperMethods tests the helper methods for RuntimeGlobal
func TestRuntimeGlobal_HelperMethods(t *testing.T) {
	spec := &GlobalSpec{
		Timeout:             common.IntPtr(300),
		EnvAllowed:          []string{"PATH", "HOME"},
		VerifyStandardPaths: common.BoolPtr(false),
	}

	runtime, err := NewRuntimeGlobal(spec)
	if err != nil {
		t.Fatalf("NewRuntimeGlobal() failed: %v", err)
	}

	// Test Timeout()
	timeout := runtime.Timeout()
	require.True(t, timeout.IsSet())
	assert.Equal(t, 300, timeout.Value())

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

// TestRuntimeGroup_HelperMethods tests the helper methods for RuntimeGroup
func TestRuntimeGroup_HelperMethods(t *testing.T) {
	spec := &GroupSpec{
		Name:    "test-group",
		WorkDir: "/tmp/test",
	}

	runtime, err := NewRuntimeGroup(spec)
	require.NoError(t, err)

	// Test Name()
	assert.Equal(t, "test-group", runtime.Name())

	// Test WorkDir()
	assert.Equal(t, "/tmp/test", runtime.WorkDir())
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
			verifyStandardPaths: common.BoolPtr(true),
			wantSkip:            false,
		},
		{
			name:                "explicit false (don't verify) means skip=true",
			verifyStandardPaths: common.BoolPtr(false),
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
