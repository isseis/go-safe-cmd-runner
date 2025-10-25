//go:build test

package executor_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// BenchmarkBuildProcessEnvironment benchmarks the BuildProcessEnvironment function
func BenchmarkBuildProcessEnvironment(b *testing.B) {
	// Create test data with 100 variables
	systemEnv := make(map[string]string)
	for i := 0; i < 30; i++ {
		systemEnv[generateVarName("SYS", i)] = generateVarValue(i)
	}

	globalEnv := make(map[string]string)
	for i := 0; i < 30; i++ {
		globalEnv[generateVarName("GLOBAL", i)] = generateVarValue(i)
	}

	groupEnv := make(map[string]string)
	for i := 0; i < 20; i++ {
		groupEnv[generateVarName("GROUP", i)] = generateVarValue(i)
	}

	cmdEnv := make(map[string]string)
	for i := 0; i < 20; i++ {
		cmdEnv[generateVarName("CMD", i)] = generateVarValue(i)
	}

	// Create allowlist for all system vars
	allowlist := make([]string, 0, len(systemEnv))
	for k := range systemEnv {
		allowlist = append(allowlist, k)
	}

	global := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{
			EnvAllowed: allowlist,
		},
		ExpandedEnv: globalEnv,
		SystemEnv:   systemEnv,
	}

	group := &runnertypes.RuntimeGroup{
		Spec: &runnertypes.GroupSpec{
			Name: "bench-group",
		},
		ExpandedEnv: groupEnv,
	}

	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name: "bench-command",
		},
		ExpandedEnv: cmdEnv,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.BuildProcessEnvironment(global, group, cmd)
	}
}

// BenchmarkBuildProcessEnvironment_Small benchmarks with small number of variables
func BenchmarkBuildProcessEnvironment_Small(b *testing.B) {
	global := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{
			EnvAllowed: []string{"HOME"},
		},
		ExpandedEnv: map[string]string{
			"GLOBAL_VAR": "value",
		},
		SystemEnv: map[string]string{
			"HOME": "/home/test",
		},
	}

	group := &runnertypes.RuntimeGroup{
		Spec: &runnertypes.GroupSpec{
			Name: "test-group",
		},
		ExpandedEnv: map[string]string{
			"GROUP_VAR": "value",
		},
	}

	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name: "test-command",
		},
		ExpandedEnv: map[string]string{
			"CMD_VAR": "value",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.BuildProcessEnvironment(global, group, cmd)
	}
}

// Helper functions
func generateVarName(prefix string, index int) string {
	return prefix + "_VAR_" + string(rune('A'+index%26))
}

func generateVarValue(index int) string {
	return "value_" + string(rune('0'+index%10))
}
