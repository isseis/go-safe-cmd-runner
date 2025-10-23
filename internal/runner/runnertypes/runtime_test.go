package runnertypes

import (
	"testing"
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
			if got != tt.want {
				t.Errorf("Name() = %v, want %v", got, tt.want)
			}
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
			if got != tt.want {
				t.Errorf("RunAsUser() = %v, want %v", got, tt.want)
			}
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
			if got != tt.want {
				t.Errorf("RunAsGroup() = %v, want %v", got, tt.want)
			}
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
			if got != tt.want {
				t.Errorf("Output() = %v, want %v", got, tt.want)
			}
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
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMaxRiskLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetMaxRiskLevel() = %v, want %v", got, tt.want)
			}
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
			if got != tt.want {
				t.Errorf("HasUserGroupSpecification() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRuntimeGlobal_Structure(t *testing.T) {
	// Test that RuntimeGlobal can be created with proper structure
	spec := &GlobalSpec{
		Timeout:  300,
		LogLevel: "debug",
		EnvVars:  []string{"PATH=/usr/bin"},
		Vars:     []string{"VAR1=value1"},
	}

	runtime := &RuntimeGlobal{
		Spec:                spec,
		ExpandedVerifyFiles: []string{"/usr/bin/test"},
		ExpandedEnv: map[string]string{
			"PATH": "/usr/bin",
		},
		ExpandedVars: map[string]string{
			"VAR1": "value1",
		},
	}

	// Verify that the structure is properly created
	if runtime.Spec == nil {
		t.Error("RuntimeGlobal.Spec should not be nil")
	}
	if runtime.Spec.Timeout != 300 {
		t.Errorf("Spec.Timeout = %d, want 300", runtime.Spec.Timeout)
	}
	if len(runtime.ExpandedEnv) != 1 {
		t.Errorf("len(ExpandedEnv) = %d, want 1", len(runtime.ExpandedEnv))
	}
	if val, exists := runtime.ExpandedEnv["PATH"]; !exists {
		t.Error("ExpandedEnv[PATH] key not found")
	} else if val != "/usr/bin" {
		t.Errorf("ExpandedEnv[PATH] = %s, want /usr/bin", val)
	}
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
	if runtime.Spec == nil {
		t.Error("RuntimeGroup.Spec should not be nil")
	}
	if runtime.Spec.Name != "test-group" {
		t.Errorf("Spec.Name = %s, want test-group", runtime.Spec.Name)
	}
	if runtime.EffectiveWorkDir != "/tmp/test" {
		t.Errorf("EffectiveWorkDir = %s, want /tmp/test", runtime.EffectiveWorkDir)
	}
	if len(runtime.ExpandedEnv) != 1 {
		t.Errorf("len(ExpandedEnv) = %d, want 1", len(runtime.ExpandedEnv))
	}
}

func TestRuntimeCommand_Structure(t *testing.T) {
	// Test that RuntimeCommand can be created with proper structure
	spec := &CommandSpec{
		Name:    "test-cmd",
		Cmd:     "/usr/bin/echo",
		Args:    []string{"hello", "world"},
		WorkDir: "/tmp",
		Timeout: 60,
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
	if runtime.Spec == nil {
		t.Error("RuntimeCommand.Spec should not be nil")
	}
	if runtime.Name() != "test-cmd" {
		t.Errorf("Name() = %s, want test-cmd", runtime.Name())
	}
	if runtime.ExpandedCmd != "/usr/bin/echo" {
		t.Errorf("ExpandedCmd = %s, want /usr/bin/echo", runtime.ExpandedCmd)
	}
	if len(runtime.ExpandedArgs) != 2 {
		t.Errorf("len(ExpandedArgs) = %d, want 2", len(runtime.ExpandedArgs))
	}
	// Verify the content of ExpandedArgs
	expectedArgs := []string{"hello", "world"}
	for i, arg := range runtime.ExpandedArgs {
		if i < len(expectedArgs) && arg != expectedArgs[i] {
			t.Errorf("ExpandedArgs[%d] = %s, want %s", i, arg, expectedArgs[i])
		}
	}
	// Verify ExpandedEnv content
	if len(runtime.ExpandedEnv) != 1 {
		t.Errorf("len(ExpandedEnv) = %d, want 1", len(runtime.ExpandedEnv))
	}
	if val, exists := runtime.ExpandedEnv["TEST"]; !exists {
		t.Error("ExpandedEnv[TEST] key not found")
	} else if val != "value" {
		t.Errorf("ExpandedEnv[TEST] = %s, want value", val)
	}
	if runtime.EffectiveWorkDir != "/tmp" {
		t.Errorf("EffectiveWorkDir = %s, want /tmp", runtime.EffectiveWorkDir)
	}
	if runtime.EffectiveTimeout != 60 {
		t.Errorf("EffectiveTimeout = %d, want 60", runtime.EffectiveTimeout)
	}
}

// TestRuntimeCommand_HelperMethods tests the helper methods for RuntimeCommand
func TestRuntimeCommand_HelperMethods(t *testing.T) {
	spec := &CommandSpec{
		Name:    "test-cmd",
		Cmd:     "/usr/bin/echo",
		Args:    []string{"hello", "world"},
		Timeout: 60,
	}

	runtime, err := NewRuntimeCommand(spec)
	if err != nil {
		t.Fatalf("NewRuntimeCommand() failed: %v", err)
	}

	// Test Cmd()
	if got := runtime.Cmd(); got != "/usr/bin/echo" {
		t.Errorf("Cmd() = %s, want /usr/bin/echo", got)
	}

	// Test Args()
	args := runtime.Args()
	if len(args) != 2 {
		t.Errorf("len(Args()) = %d, want 2", len(args))
	}
	if args[0] != "hello" || args[1] != "world" {
		t.Errorf("Args() = %v, want [hello world]", args)
	}

	// Test Timeout()
	if got := runtime.Timeout(); got != 60 {
		t.Errorf("Timeout() = %d, want 60", got)
	}
}

// TestRuntimeGlobal_HelperMethods tests the helper methods for RuntimeGlobal
func TestRuntimeGlobal_HelperMethods(t *testing.T) {
	spec := &GlobalSpec{
		Timeout:             300,
		EnvAllowed:          []string{"PATH", "HOME"},
		VerifyStandardPaths: func() *bool { b := false; return &b }(),
	}

	runtime, err := NewRuntimeGlobal(spec)
	if err != nil {
		t.Fatalf("NewRuntimeGlobal() failed: %v", err)
	}

	// Test Timeout()
	if got := runtime.Timeout(); got != 300 {
		t.Errorf("Timeout() = %d, want 300", got)
	}

	// Test EnvAllowlist()
	allowlist := runtime.EnvAllowlist()
	if len(allowlist) != 2 {
		t.Errorf("len(EnvAllowlist()) = %d, want 2", len(allowlist))
	}
	if allowlist[0] != "PATH" || allowlist[1] != "HOME" {
		t.Errorf("EnvAllowlist() = %v, want [PATH HOME]", allowlist)
	}

	// Test SkipStandardPaths()
	if got := runtime.SkipStandardPaths(); !got {
		t.Errorf("SkipStandardPaths() = %v, want true", got)
	}
}

// TestRuntimeGlobal_TimeoutDefault tests that Timeout() returns default value when not set
func TestRuntimeGlobal_TimeoutDefault(t *testing.T) {
	spec := &GlobalSpec{
		Timeout: 0, // Not set in TOML
	}

	runtime, err := NewRuntimeGlobal(spec)
	if err != nil {
		t.Fatalf("NewRuntimeGlobal() failed: %v", err)
	}

	// Test Timeout() returns default value
	if got := runtime.Timeout(); got != DefaultTimeout {
		t.Errorf("Timeout() = %d, want %d (DefaultTimeout)", got, DefaultTimeout)
	}
}

// TestRuntimeGroup_HelperMethods tests the helper methods for RuntimeGroup
func TestRuntimeGroup_HelperMethods(t *testing.T) {
	spec := &GroupSpec{
		Name:    "test-group",
		WorkDir: "/tmp/test",
	}

	runtime, err := NewRuntimeGroup(spec)
	if err != nil {
		t.Fatalf("NewRuntimeGroup() failed: %v", err)
	}

	// Test Name()
	if got := runtime.Name(); got != "test-group" {
		t.Errorf("Name() = %s, want test-group", got)
	}

	// Test WorkDir()
	if got := runtime.WorkDir(); got != "/tmp/test" {
		t.Errorf("WorkDir() = %s, want /tmp/test", got)
	}
}
