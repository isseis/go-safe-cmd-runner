package resource

import (
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDryRunExecutionPath verifies that dry-run mode properly analyzes
// command execution without performing actual side effects
func TestDryRunExecutionPath(t *testing.T) {
	tests := []struct {
		name             string
		commands         []runnertypes.Command
		groups           []*runnertypes.CommandGroup
		envVars          map[string]string
		expectedAnalyses int // Expected number of resource analyses
	}{
		{
			name: "basic command dry-run execution",
			commands: []runnertypes.Command{
				{
					Name:        "test-echo",
					Description: "Basic echo command",
					Cmd:         "echo hello world",
				},
			},
			groups: []*runnertypes.CommandGroup{
				{
					Name:        "test-group",
					Description: "Test command group",
					Priority:    1,
				},
			},
			envVars: map[string]string{
				"TEST_VAR": "test_value",
			},
			expectedAnalyses: 1,
		},
		{
			name: "multiple commands dry-run execution",
			commands: []runnertypes.Command{
				{
					Name:        "test-echo1",
					Description: "First echo command",
					Cmd:         "echo first",
				},
				{
					Name:        "test-echo2",
					Description: "Second echo command",
					Cmd:         "echo second",
				},
			},
			groups: []*runnertypes.CommandGroup{
				{
					Name:        "test-group",
					Description: "Test command group",
					Priority:    1,
				},
			},
			envVars: map[string]string{
				"TEST_VAR1": "value1",
				"TEST_VAR2": "value2",
			},
			expectedAnalyses: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create dry-run execution manager
			dryRunOpts := &DryRunOptions{
				DetailLevel:   DetailLevelDetailed,
				OutputFormat:  OutputFormatText,
				ShowSensitive: false,
				VerifyFiles:   true,
			}
			dryRunManager := NewDryRunResourceManager(nil, nil, dryRunOpts)
			require.NotNil(t, dryRunManager)

			// Execute commands in dry-run mode
			for _, cmd := range tt.commands {
				group := tt.groups[0] // Use first group for simplicity
				result, err := dryRunManager.ExecuteCommand(ctx, cmd, group, tt.envVars)

				// Verify that dry-run execution doesn't produce errors
				assert.NoError(t, err, "dry-run execution should not produce errors")

				// Verify that result is marked as dry-run
				if result != nil {
					assert.True(t, result.DryRun, "result should be marked as dry-run")
					assert.Equal(t, 0, result.ExitCode, "dry-run should simulate successful execution")
				}
			}

			// Get dry-run results
			dryRunResult := dryRunManager.GetDryRunResults()
			require.NotNil(t, dryRunResult, "dry-run results should be available")

			// Verify that we captured the expected number of analyses
			assert.Len(t, dryRunResult.ResourceAnalyses, tt.expectedAnalyses,
				"should capture expected number of resource analyses")

			// Verify metadata
			assert.NotNil(t, dryRunResult.Metadata, "metadata should be present")
			assert.NotZero(t, dryRunResult.Metadata.GeneratedAt, "generation time should be set")

			// Verify execution plan
			assert.NotNil(t, dryRunResult.ExecutionPlan, "execution plan should be present")

			// Verify that each analysis has required fields
			for i, analysis := range dryRunResult.ResourceAnalyses {
				assert.Equal(t, ResourceTypeCommand, analysis.Type,
					"analysis %d should be command type", i)
				assert.Equal(t, OperationExecute, analysis.Operation,
					"analysis %d should be execute operation", i)
				assert.NotEmpty(t, analysis.Target,
					"analysis %d should have target", i)
				assert.NotZero(t, analysis.Timestamp,
					"analysis %d should have timestamp", i)
			}
		})
	}
}

// TestDryRunResultConsistency verifies that dry-run results are consistent
// across multiple runs with the same configuration
func TestDryRunResultConsistency(t *testing.T) {
	ctx := context.Background()

	dryRunOpts := &DryRunOptions{
		DetailLevel:   DetailLevelDetailed,
		OutputFormat:  OutputFormatText,
		ShowSensitive: false,
		VerifyFiles:   true,
	}

	command := runnertypes.Command{
		Name:        "consistency-test",
		Description: "Consistency test command",
		Cmd:         "echo consistency test",
	}

	group := &runnertypes.CommandGroup{
		Name:        "consistency-group",
		Description: "Consistency test group",
		Priority:    1,
	}

	envVars := map[string]string{
		"CONSISTENCY_VAR": "consistent_value",
	}

	var results []*DryRunResult

	// Run the same dry-run multiple times
	for range 3 {
		manager := NewDryRunResourceManager(nil, nil, dryRunOpts)

		_, err := manager.ExecuteCommand(ctx, command, group, envVars)
		require.NoError(t, err)

		result := manager.GetDryRunResults()
		require.NotNil(t, result)
		results = append(results, result)
	}

	// Verify consistency across runs
	require.Len(t, results, 3, "should have results from 3 runs")

	// All runs should have the same number of analyses
	for i := 1; i < len(results); i++ {
		assert.Len(t, results[i].ResourceAnalyses, len(results[0].ResourceAnalyses),
			"run %d should have same number of analyses as run 0", i)
	}

	// All runs should have analyses with the same structure
	for i := 1; i < len(results); i++ {
		for j, analysis := range results[i].ResourceAnalyses {
			baseAnalysis := results[0].ResourceAnalyses[j]

			assert.Equal(t, baseAnalysis.Type, analysis.Type,
				"run %d analysis %d should have same type", i, j)
			assert.Equal(t, baseAnalysis.Operation, analysis.Operation,
				"run %d analysis %d should have same operation", i, j)
			assert.Equal(t, baseAnalysis.Target, analysis.Target,
				"run %d analysis %d should have same target", i, j)
		}
	}
}

// TestDefaultResourceManagerModeConsistency verifies that DefaultResourceManager
// properly delegates to the correct underlying manager based on mode
func TestDefaultResourceManagerModeConsistency(t *testing.T) {
	ctx := context.Background()

	command := runnertypes.Command{
		Name:        "mode-test",
		Description: "Mode consistency test",
		Cmd:         "echo mode test",
	}

	group := &runnertypes.CommandGroup{
		Name:        "mode-group",
		Description: "Mode test group",
		Priority:    1,
	}

	envVars := map[string]string{
		"MODE_VAR": "mode_value",
	}

	t.Run("normal mode delegation", func(t *testing.T) {
		dryRunOpts := &DryRunOptions{}
		manager := NewDefaultResourceManager(nil, nil, nil, slog.Default(), ExecutionModeNormal, dryRunOpts)
		require.NotNil(t, manager)

		assert.Equal(t, ExecutionModeNormal, manager.GetMode())

		// Normal mode should not provide dry-run results
		assert.Nil(t, manager.GetDryRunResults())
	})

	t.Run("dry-run mode delegation", func(t *testing.T) {
		dryRunOpts := &DryRunOptions{
			DetailLevel:  DetailLevelDetailed,
			OutputFormat: OutputFormatText,
		}

		manager := NewDefaultResourceManager(nil, nil, nil, slog.Default(), ExecutionModeDryRun, dryRunOpts)
		require.NotNil(t, manager)

		assert.Equal(t, ExecutionModeDryRun, manager.GetMode())

		// Execute a command
		_, err := manager.ExecuteCommand(ctx, command, group, envVars)
		assert.NoError(t, err)

		// Dry-run mode should provide results
		result := manager.GetDryRunResults()
		assert.NotNil(t, result)
		assert.NotEmpty(t, result.ResourceAnalyses)
	})
}
