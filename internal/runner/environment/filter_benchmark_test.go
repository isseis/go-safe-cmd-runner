package environment

import (
	"testing"
)

// BenchmarkIsAllowed benchmarks the IsAllowed method with different allowlist sizes
func BenchmarkIsAllowed(b *testing.B) {
	testCases := []struct {
		name          string
		allowlistSize int
	}{
		{"10_vars", 10},
		{"50_vars", 50},
		{"100_vars", 100},
		{"500_vars", 500},
	}

	for _, tc := range testCases {
		// Create test allowlist
		allowlist := make([]string, tc.allowlistSize)
		for i := 0; i < tc.allowlistSize; i++ {
			allowlist[i] = testVariable(i)
		}

		b.Run(tc.name, func(b *testing.B) {
			filter := NewFilter([]string{"GLOBAL_VAR"})
			resolution := filter.ResolveAllowlistConfiguration(allowlist, "test-group")

			// Test variable in the middle of the allowlist (worst case for slice search)
			testVar := testVariable(tc.allowlistSize / 2)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = resolution.IsAllowed(testVar)
			}
		})
	}
}

// BenchmarkIsVariableAccessAllowed benchmarks the full IsVariableAccessAllowed flow
func BenchmarkIsVariableAccessAllowed(b *testing.B) {
	testCases := []struct {
		name          string
		allowlistSize int
	}{
		{"10_vars", 10},
		{"50_vars", 50},
		{"100_vars", 100},
		{"500_vars", 500},
	}

	for _, tc := range testCases {
		// Create test allowlist
		allowlist := make([]string, tc.allowlistSize)
		for i := 0; i < tc.allowlistSize; i++ {
			allowlist[i] = testVariable(i)
		}

		b.Run(tc.name, func(b *testing.B) {
			filter := NewFilter([]string{"GLOBAL_VAR"})

			// Test variable in the middle of the allowlist
			testVar := testVariable(tc.allowlistSize / 2)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = filter.IsVariableAccessAllowed(testVar, allowlist, "test-group")
			}
		})
	}
}

// Helper function to generate test variable names
func testVariable(index int) string {
	return "TEST_VAR_" + string(rune('A'+index%26)) + string(rune('0'+index/26))
}
