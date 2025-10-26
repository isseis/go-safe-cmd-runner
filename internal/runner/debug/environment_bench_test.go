package debug

import (
	"bytes"
	"fmt"
	"testing"
)

// BenchmarkPrintFinalEnvironment benchmarks the PrintFinalEnvironment function
func BenchmarkPrintFinalEnvironment(b *testing.B) {
	// Create test data with 100 variables
	envVars := make(map[string]string)
	origins := make(map[string]string)

	for i := 0; i < 100; i++ {
		varName := fmt.Sprintf("VAR_%d", i)
		envVars[varName] = fmt.Sprintf("value_%d", i)

		switch i % 4 {
		case 0:
			origins[varName] = "System (filtered by allowlist)"
		case 1:
			origins[varName] = "Global"
		case 2:
			origins[varName] = "Group[test-group]"
		case 3:
			origins[varName] = "Command[test-command]"
		}
	}

	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		PrintFinalEnvironment(&buf, envVars, origins, false)
	}
}

// BenchmarkPrintFinalEnvironment_Small benchmarks with small number of variables
func BenchmarkPrintFinalEnvironment_Small(b *testing.B) {
	envVars := map[string]string{
		"HOME":       "/home/test",
		"PATH":       "/usr/bin:/bin",
		"GLOBAL_VAR": "global_value",
		"GROUP_VAR":  "group_value",
		"CMD_VAR":    "cmd_value",
	}

	origins := map[string]string{
		"HOME":       "System (filtered by allowlist)",
		"PATH":       "System (filtered by allowlist)",
		"GLOBAL_VAR": "Global",
		"GROUP_VAR":  "Group[test-group]",
		"CMD_VAR":    "Command[test-command]",
	}

	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		PrintFinalEnvironment(&buf, envVars, origins, false)
	}
}

// BenchmarkPrintFinalEnvironment_LongValues benchmarks with long values
func BenchmarkPrintEnvironment_LongValues(b *testing.B) {
	longValue := ""
	for i := 0; i < 200; i++ {
		longValue += "a"
	}

	envVars := make(map[string]string)
	origins := make(map[string]string)

	for i := 0; i < 50; i++ {
		varName := fmt.Sprintf("LONG_VAR_%d", i)
		envVars[varName] = longValue
		origins[varName] = "Global"
	}

	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		PrintFinalEnvironment(&buf, envVars, origins, false)
	}
}
