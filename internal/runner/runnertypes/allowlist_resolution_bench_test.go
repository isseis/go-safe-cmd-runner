package runnertypes

import (
	"fmt"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// BenchmarkAllowlistResolutionIsAllowed benchmarks the IsAllowed method with various sizes
func BenchmarkAllowlistResolutionIsAllowed(b *testing.B) {
	sizes := []int{10, 100, 1000, 10000}

	for _, size := range sizes {
		// Create large allowlists for performance testing
		globalVars := make([]string, size)
		for i := 0; i < size; i++ {
			globalVars[i] = fmt.Sprintf("GLOBAL_VAR_%d", i)
		}

		resolution := NewTestAllowlistResolutionWithMode(InheritanceModeInherit, globalVars, []string{})

		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			// Test variable that exists (best case)
			testVar := globalVars[size/2] // Middle element

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = resolution.IsAllowed(testVar)
			}
		})

		b.Run(fmt.Sprintf("size_%d_not_found", size), func(b *testing.B) {
			// Test variable that doesn't exist (worst case for some implementations)
			testVar := "NONEXISTENT_VAR"

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = resolution.IsAllowed(testVar)
			}
		})
	}
}

// BenchmarkAllowlistResolutionConstruction benchmarks object construction
func BenchmarkAllowlistResolutionConstruction(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		globalVars := make([]string, size)
		groupVars := make([]string, size)

		for i := 0; i < size; i++ {
			globalVars[i] = fmt.Sprintf("GLOBAL_VAR_%d", i)
			groupVars[i] = fmt.Sprintf("GROUP_VAR_%d", i)
		}

		b.Run(fmt.Sprintf("simple_size_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = NewTestAllowlistResolutionSimple(globalVars, groupVars)
			}
		})

		b.Run(fmt.Sprintf("explicit_size_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = NewTestAllowlistResolutionWithMode(InheritanceModeExplicit, globalVars, groupVars)
			}
		})

		b.Run(fmt.Sprintf("builder_size_%d", size), func(b *testing.B) {
			globalSet := common.SliceToSet(globalVars)
			for i := 0; i < b.N; i++ {
				_ = NewAllowlistResolutionBuilder().
					WithMode(InheritanceModeInherit).
					WithGroupName("benchmark-group").
					WithGlobalVariablesSet(globalSet).
					WithGroupVariables(groupVars).
					Build()
			}
		})
	}
}

// BenchmarkAllowlistResolutionMemory tests memory allocation behavior
func BenchmarkAllowlistResolutionMemory(b *testing.B) {
	size := 1000
	globalVars := make([]string, size)
	groupVars := make([]string, size)

	for i := range size {
		globalVars[i] = fmt.Sprintf("GLOBAL_VAR_%d", i)
		groupVars[i] = fmt.Sprintf("GROUP_VAR_%d", i)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resolution := NewTestAllowlistResolutionSimple(globalVars, groupVars)

		// Use the resolution to prevent optimization
		_ = resolution.IsAllowed("GLOBAL_VAR_500")
	}
}
