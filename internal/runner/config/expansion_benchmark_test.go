//go:build test
// +build test

package config

import (
	"fmt"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
)

// BenchmarkAllowlistLookup compares the performance of slices.Contains vs map lookup
// for environment variable allowlist checking.
func BenchmarkAllowlistLookup(b *testing.B) {
	// Create test data with varying allowlist sizes
	testCases := []struct {
		name          string
		allowlistSize int
		envVarCount   int
	}{
		{"small_allowlist_10vars", 10, 100},
		{"medium_allowlist_50vars", 50, 100},
		{"large_allowlist_100vars", 100, 100},
		{"xlarge_allowlist_500vars", 500, 100},
	}

	for _, tc := range testCases {
		// Create allowlist
		allowlist := make([]string, tc.allowlistSize)
		for i := 0; i < tc.allowlistSize; i++ {
			allowlist[i] = fmt.Sprintf("TEST_VAR_%d", i)
		}

		// Set up environment variables
		for i := 0; i < tc.envVarCount; i++ {
			varName := fmt.Sprintf("TEST_VAR_%d", i%tc.allowlistSize)
			os.Setenv(varName, fmt.Sprintf("value_%d", i))
		}
		defer func() {
			for i := 0; i < tc.envVarCount; i++ {
				varName := fmt.Sprintf("TEST_VAR_%d", i%tc.allowlistSize)
				os.Unsetenv(varName)
			}
		}()

		// Benchmark current implementation (slice-based)
		b.Run(fmt.Sprintf("slice_%s", tc.name), func(b *testing.B) {
			filter := environment.NewFilter(allowlist)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = filter.ParseSystemEnvironment(func(varName string) bool {
					// This simulates the current O(n) lookup using slices.Contains
					for _, allowed := range allowlist {
						if allowed == varName {
							return true
						}
					}
					return false
				})
			}
		})

		// Benchmark optimized implementation (map-based)
		b.Run(fmt.Sprintf("map_%s", tc.name), func(b *testing.B) {
			filter := environment.NewFilter(allowlist)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Convert to map once for O(1) lookups
				allowlistSet := make(map[string]bool, len(allowlist))
				for _, varName := range allowlist {
					allowlistSet[varName] = true
				}
				_ = filter.ParseSystemEnvironment(func(varName string) bool {
					return allowlistSet[varName]
				})
			}
		})
	}
}

// BenchmarkExpandEnvInternalAllowlistLookup benchmarks the expandEnvInternal function
// focusing on the allowlist lookup performance.
func BenchmarkExpandEnvInternalAllowlistLookup(b *testing.B) {
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
			allowlist[i] = fmt.Sprintf("ALLOWED_VAR_%d", i)
		}

		// Set up environment variables
		for i := 0; i < tc.allowlistSize; i++ {
			varName := fmt.Sprintf("ALLOWED_VAR_%d", i)
			os.Setenv(varName, fmt.Sprintf("value_%d", i))
		}
		defer func() {
			for i := 0; i < tc.allowlistSize; i++ {
				varName := fmt.Sprintf("ALLOWED_VAR_%d", i)
				os.Unsetenv(varName)
			}
		}()

		// Create test environment list
		envList := []string{"MY_VAR=${ALLOWED_VAR_0}"}

		b.Run(tc.name, func(b *testing.B) {
			filter := environment.NewFilter(allowlist)
			expander := environment.NewVariableExpander(filter)
			var expandedEnv map[string]string

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				err := expandEnvInternal(
					envList,
					"test",
					&expandedEnv,
					expander,
					nil,       // autoEnv
					nil,       // globalEnv
					nil,       // globalAllowlist
					nil,       // groupEnv
					allowlist, // groupAllowlist
					ErrCommandEnvExpansionFailed,
				)
				if err != nil {
					b.Fatalf("expandEnvInternal failed: %v", err)
				}
			}
		})
	}
}
