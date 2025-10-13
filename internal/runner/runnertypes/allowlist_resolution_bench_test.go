//go:build test

package runnertypes

import (
	"fmt"
	"testing"
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

		factory := TestAllowlistResolutionFactory{}
		resolution := factory.CreateWithMode(InheritanceModeInherit, globalVars, []string{})

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

// BenchmarkAllowlistResolutionGetters benchmarks the getter methods
func BenchmarkAllowlistResolutionGetters(b *testing.B) {
	size := 1000
	globalVars := make([]string, size)
	groupVars := make([]string, size)

	for i := 0; i < size; i++ {
		globalVars[i] = fmt.Sprintf("GLOBAL_VAR_%d", i)
		groupVars[i] = fmt.Sprintf("GROUP_VAR_%d", i)
	}

	factory := TestAllowlistResolutionFactory{}
	resolution := factory.CreateWithMode(InheritanceModeInherit, globalVars, groupVars)

	b.Run("GetEffectiveList", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = resolution.GetEffectiveList()
		}
	})

	b.Run("GetEffectiveSize", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = resolution.GetEffectiveSize()
		}
	})

	b.Run("GetGroupAllowlist", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = resolution.GetGroupAllowlist()
		}
	})

	b.Run("GetGlobalAllowlist", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = resolution.GetGlobalAllowlist()
		}
	})
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

		factory := TestAllowlistResolutionFactory{}

		b.Run(fmt.Sprintf("factory_simple_size_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = factory.CreateSimple(globalVars, groupVars)
			}
		})

		b.Run(fmt.Sprintf("factory_explicit_size_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = factory.CreateWithMode(InheritanceModeExplicit, globalVars, groupVars)
			}
		})

		b.Run(fmt.Sprintf("builder_size_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = NewAllowlistResolutionBuilder().
					WithMode(InheritanceModeInherit).
					WithGroupName("benchmark-group").
					WithGlobalVariables(globalVars).
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

	factory := TestAllowlistResolutionFactory{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resolution := factory.CreateSimple(globalVars, groupVars)

		// Use the resolution to prevent optimization
		_ = resolution.IsAllowed("GLOBAL_VAR_500")
		_ = resolution.GetEffectiveSize()
	}
}
