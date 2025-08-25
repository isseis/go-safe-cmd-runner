package resource

import (
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// BenchmarkDryRunPerformance benchmarks dry-run performance with various numbers of commands
func BenchmarkDryRunPerformance(b *testing.B) {
	benchmarks := []struct {
		name        string
		numCommands int
	}{
		{"SingleCommand", 1},
		{"TenCommands", 10},
		{"HundredCommands", 100},
		{"ThousandCommands", 1000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create test commands
			commands := make([]runnertypes.Command, bm.numCommands)
			for i := 0; i < bm.numCommands; i++ {
				commands[i] = runnertypes.Command{
					Name:        "test-cmd",
					Description: "Benchmark test command",
					Cmd:         "echo test",
				}
			}

			group := &runnertypes.CommandGroup{
				Name:        "benchmark-group",
				Description: "Benchmark test group",
				Priority:    1,
			}

			envVars := map[string]string{
				"BENCH_VAR": "bench_value",
			}

			ctx := context.Background()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				dryRunOpts := &DryRunOptions{
					DetailLevel:   DetailLevelDetailed,
					OutputFormat:  OutputFormatText,
					ShowSensitive: false,
					VerifyFiles:   true,
				}

				mockPathResolver := &MockPathResolver{}
				setupStandardCommandPaths(mockPathResolver)
				mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
				manager, err := NewDryRunResourceManager(nil, nil, mockPathResolver, dryRunOpts)
				require.NoError(b, err)

				// Execute all commands
				for _, cmd := range commands {
					_, err := manager.ExecuteCommand(ctx, cmd, group, envVars)
					if err != nil {
						b.Fatalf("unexpected error: %v", err)
					}
				}

				// Get results to ensure full execution path
				result := manager.GetDryRunResults()
				if result == nil {
					b.Fatal("expected dry-run results")
				}
			}
		})
	}
}

// BenchmarkFormatterPerformance benchmarks formatter performance
func BenchmarkFormatterPerformance(b *testing.B) {
	// Create a substantial dry-run result for benchmarking
	result := &DryRunResult{
		Metadata: &ResultMetadata{
			RunID:      "benchmark-run",
			ConfigPath: "/test/config.toml",
			Version:    "test",
		},
		ResourceAnalyses: make([]ResourceAnalysis, 100),
	}

	// Fill with test data
	for i := 0; i < 100; i++ {
		result.ResourceAnalyses[i] = ResourceAnalysis{
			Type:      ResourceTypeCommand,
			Operation: OperationExecute,
			Target:    "echo test command",
			Parameters: map[string]interface{}{
				"working_directory": "/test",
				"timeout":           30,
			},
			Impact: ResourceImpact{
				Reversible:  true,
				Persistent:  false,
				Description: "Execute echo command",
			},
		}
	}

	b.Run("TextFormatter", func(b *testing.B) {
		formatter := NewTextFormatter()
		opts := FormatterOptions{
			DetailLevel:   DetailLevelDetailed,
			OutputFormat:  OutputFormatText,
			ShowSensitive: false,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := formatter.FormatResult(result, opts)
			if err != nil {
				b.Fatalf("formatting error: %v", err)
			}
		}
	})

	b.Run("JSONFormatter", func(b *testing.B) {
		formatter := NewJSONFormatter()
		opts := FormatterOptions{
			DetailLevel:   DetailLevelDetailed,
			OutputFormat:  OutputFormatJSON,
			ShowSensitive: false,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := formatter.FormatResult(result, opts)
			if err != nil {
				b.Fatalf("formatting error: %v", err)
			}
		}
	})
}

// BenchmarkResourceManagerModeSwitch benchmarks mode switching performance
func BenchmarkResourceManagerModeSwitch(b *testing.B) {
	command := runnertypes.Command{
		Name:        "switch-test",
		Description: "Mode switch test",
		Cmd:         "echo switch test",
	}

	group := &runnertypes.CommandGroup{
		Name:        "switch-group",
		Description: "Switch test group",
		Priority:    1,
	}

	envVars := map[string]string{
		"SWITCH_VAR": "switch_value",
	}

	ctx := context.Background()

	// Skip Normal Mode test due to dependency requirements
	// This test requires real executors and would be better suited for integration tests

	b.Run("DryRunMode", func(b *testing.B) {
		dryRunOpts := &DryRunOptions{
			DetailLevel:  DetailLevelDetailed,
			OutputFormat: OutputFormatText,
		}
		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager, err := NewDefaultResourceManager(nil, nil, nil, mockPathResolver, slog.Default(), ExecutionModeDryRun, dryRunOpts)
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := manager.ExecuteCommand(ctx, command, group, envVars)
			if err != nil {
				b.Fatalf("unexpected error: %v", err)
			}
		}
	})
}

// BenchmarkMemoryUsage benchmarks memory usage during dry-run execution
func BenchmarkMemoryUsage(b *testing.B) {
	commands := make([]runnertypes.Command, 1000)
	for i := 0; i < 1000; i++ {
		commands[i] = runnertypes.Command{
			Name:        "memory-test",
			Description: "Memory usage test command",
			Cmd:         "echo memory test",
		}
	}

	group := &runnertypes.CommandGroup{
		Name:        "memory-group",
		Description: "Memory test group",
		Priority:    1,
	}

	envVars := map[string]string{
		"MEMORY_VAR": "memory_value",
	}

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dryRunOpts := &DryRunOptions{
			DetailLevel:   DetailLevelDetailed,
			OutputFormat:  OutputFormatText,
			ShowSensitive: false,
			VerifyFiles:   true,
		}

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager, err := NewDryRunResourceManager(nil, nil, mockPathResolver, dryRunOpts)
		require.NoError(b, err)

		// Execute all commands
		for _, cmd := range commands {
			_, err := manager.ExecuteCommand(ctx, cmd, group, envVars)
			if err != nil {
				b.Fatalf("unexpected error: %v", err)
			}
		}

		// Get results
		result := manager.GetDryRunResults()
		if result == nil {
			b.Fatal("expected dry-run results")
		}

		// Format results to measure full memory usage
		formatter := NewTextFormatter()
		_, err = formatter.FormatResult(result, FormatterOptions{
			DetailLevel:  DetailLevelDetailed,
			OutputFormat: OutputFormatText,
		})
		if err != nil {
			b.Fatalf("formatting error: %v", err)
		}
	}
}
