package debug

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
)

// BenchmarkPrintFinalEnvironment benchmarks the PrintFinalEnvironment function
func BenchmarkPrintFinalEnvironment(b *testing.B) {
	// Create test data with 100 variables
	envMap := make(map[string]executor.EnvVar)

	for i := 0; i < 100; i++ {
		varName := fmt.Sprintf("VAR_%d", i)
		var origin string

		switch i % 4 {
		case 0:
			origin = "System (filtered by allowlist)"
		case 1:
			origin = "Global"
		case 2:
			origin = "Group[test-group]"
		case 3:
			origin = "Command[test-command]"
		}

		envMap[varName] = executor.EnvVar{
			Value:  fmt.Sprintf("value_%d", i),
			Origin: origin,
		}
	}

	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		PrintFinalEnvironment(&buf, envMap, false)
	}
}

// BenchmarkPrintFinalEnvironment_Small benchmarks with small number of variables
func BenchmarkPrintFinalEnvironment_Small(b *testing.B) {
	envMap := map[string]executor.EnvVar{
		"HOME": {
			Value:  "/home/test",
			Origin: "System (filtered by allowlist)",
		},
		"PATH": {
			Value:  "/usr/bin:/bin",
			Origin: "System (filtered by allowlist)",
		},
		"GLOBAL_VAR": {
			Value:  "global_value",
			Origin: "Global",
		},
		"GROUP_VAR": {
			Value:  "group_value",
			Origin: "Group[test-group]",
		},
		"CMD_VAR": {
			Value:  "cmd_value",
			Origin: "Command[test-command]",
		},
	}

	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		PrintFinalEnvironment(&buf, envMap, false)
	}
}

// BenchmarkPrintFinalEnvironment_LongValues benchmarks with long values
func BenchmarkPrintEnvironment_LongValues(b *testing.B) {
	longValue := ""
	for i := 0; i < 200; i++ {
		longValue += "a"
	}

	envMap := make(map[string]executor.EnvVar)

	for i := 0; i < 50; i++ {
		varName := fmt.Sprintf("LONG_VAR_%d", i)
		envMap[varName] = executor.EnvVar{
			Value:  longValue,
			Origin: "Global",
		}
	}

	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		PrintFinalEnvironment(&buf, envMap, false)
	}
}
