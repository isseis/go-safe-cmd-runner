//go:build test
// +build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// BenchmarkExpandGlobal measures the performance of global configuration expansion
func BenchmarkExpandGlobal(b *testing.B) {
	spec := &runnertypes.GlobalSpec{
		Vars:    []string{"VAR1=value1", "VAR2=value2", "VAR3=%{VAR1}/subdir"},
		EnvVars: []string{"PATH=%{VAR1}/bin:%{VAR2}/bin", "HOME=/home/user"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExpandGlobal(spec)
	}
}

// BenchmarkExpandGlobalWithFromEnv measures performance with from_env processing
func BenchmarkExpandGlobalWithFromEnv(b *testing.B) {
	spec := &runnertypes.GlobalSpec{
		EnvImport:  []string{"MY_PATH=PATH", "MY_HOME=HOME"},
		Vars:       []string{"VAR1=%{MY_PATH}", "VAR2=%{MY_HOME}/local"},
		EnvVars:    []string{"NEW_PATH=%{VAR1}:%{VAR2}/bin"},
		EnvAllowed: []string{"PATH", "HOME"},
	}

	// Set environment variables for benchmark
	b.Setenv("PATH", "/usr/bin:/bin")
	b.Setenv("HOME", "/home/testuser")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExpandGlobal(spec)
	}
}

// BenchmarkExpandGroup measures the performance of group configuration expansion
func BenchmarkExpandGroup(b *testing.B) {
	spec := &runnertypes.GroupSpec{
		Name:    "test_group",
		Vars:    []string{"GROUP_VAR=group_value", "DERIVED=%{GROUP_VAR}/subdir"},
		EnvVars: []string{"GROUP_ENV=%{DERIVED}"},
	}
	globalVars := map[string]string{
		"GLOBAL_VAR": "global_value",
	}

	// Prepare a minimal RuntimeGlobal for benchmark
	rg := &runnertypes.RuntimeGlobal{
		Spec:         &runnertypes.GlobalSpec{},
		ExpandedVars: globalVars,
		ExpandedEnv:  map[string]string{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExpandGroup(spec, rg)
	}
}

// BenchmarkExpandCommand measures the performance of command expansion
func BenchmarkExpandCommand(b *testing.B) {
	spec := &runnertypes.CommandSpec{
		Name:    "test_command",
		Cmd:     "/usr/bin/test",
		Args:    []string{"%{ARG1}", "%{ARG2}"},
		EnvVars: []string{"CMD_ENV=%{CMD_VAR}"},
		Vars:    []string{"CMD_VAR=cmd_value", "ARG1=arg1", "ARG2=arg2"},
	}
	groupVars := map[string]string{
		"GROUP_VAR": "group_value",
	}

	// Prepare minimal runtimes for command benchmark
	rGroup := &runnertypes.RuntimeGroup{
		Spec:         &runnertypes.GroupSpec{Name: "bench"},
		ExpandedVars: groupVars,
		ExpandedEnv:  map[string]string{},
	}
	rGlobal := &runnertypes.RuntimeGlobal{Spec: &runnertypes.GlobalSpec{}, ExpandedVars: map[string]string{}, ExpandedEnv: map[string]string{}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExpandCommand(spec, rGroup, rGlobal, common.NewUnsetTimeout())
	}
}

// BenchmarkExpandGlobalComplex measures performance with complex variable expansion
func BenchmarkExpandGlobalComplex(b *testing.B) {
	spec := &runnertypes.GlobalSpec{
		Vars: []string{
			"BASE=/opt/app",
			"BIN=%{BASE}/bin",
			"LIB=%{BASE}/lib",
			"DATA=%{BASE}/data",
			"CONFIG=%{BASE}/config",
			"LOGS=%{BASE}/logs",
		},
		EnvVars: []string{
			"PATH=%{BIN}:/usr/bin:/bin",
			"LD_LIBRARY_PATH=%{LIB}",
			"APP_DATA=%{DATA}",
			"APP_CONFIG=%{CONFIG}",
			"APP_LOGS=%{LOGS}",
		},
		VerifyFiles: []string{
			"%{BIN}/app",
			"%{CONFIG}/app.conf",
			"%{DATA}/db.sqlite",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExpandGlobal(spec)
	}
}
