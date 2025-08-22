package resource

import (
	"context"
	"strconv"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorScenarios tests various error conditions and edge cases
func TestErrorScenarios(t *testing.T) {
	tests := []struct {
		name         string
		setup        func() (*DefaultResourceManager, error)
		command      runnertypes.Command
		group        *runnertypes.CommandGroup
		envVars      map[string]string
		expectError  bool
		expectResult bool
	}{
		{
			name: "nil command group",
			setup: func() (*DefaultResourceManager, error) {
				opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
				return NewDefaultResourceManager(nil, nil, nil, ExecutionModeDryRun, opts), nil
			},
			command: runnertypes.Command{
				Name: "test-cmd",
				Cmd:  "echo test",
			},
			group:        nil,
			envVars:      map[string]string{"TEST": "value"},
			expectError:  false, // Dry-run mode handles nil group gracefully
			expectResult: true,
		},
		{
			name: "empty command",
			setup: func() (*DefaultResourceManager, error) {
				opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
				return NewDefaultResourceManager(nil, nil, nil, ExecutionModeDryRun, opts), nil
			},
			command: runnertypes.Command{
				Name: "",
				Cmd:  "",
			},
			group: &runnertypes.CommandGroup{
				Name: "test-group",
			},
			envVars:      map[string]string{},
			expectError:  false, // Dry-run mode handles empty command gracefully
			expectResult: true,
		},
		{
			name: "large environment variables",
			setup: func() (*DefaultResourceManager, error) {
				opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
				return NewDefaultResourceManager(nil, nil, nil, ExecutionModeDryRun, opts), nil
			},
			command: runnertypes.Command{
				Name: "large-env-test",
				Cmd:  "echo $LARGE_VAR",
			},
			group: &runnertypes.CommandGroup{
				Name: "test-group",
			},
			envVars: func() map[string]string {
				largeValue := make([]byte, 10000)
				for i := range largeValue {
					largeValue[i] = 'A'
				}
				return map[string]string{
					"LARGE_VAR": string(largeValue),
				}
			}(),
			expectError:  false,
			expectResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			manager, err := tt.setup()
			require.NoError(t, err, "setup failed: %v", err)

			result, err := manager.ExecuteCommand(ctx, tt.command, tt.group, tt.envVars)

			if tt.expectError {
				assert.Error(t, err, "expected error but got none")
			} else {
				assert.NoError(t, err, "unexpected error: %v", err)
			}

			if tt.expectResult {
				assert.NotNil(t, result, "expected result but got nil")
			}
		})
	}
}

// TestFormatterErrorScenarios tests formatter error handling
func TestFormatterErrorScenarios(t *testing.T) {
	tests := []struct {
		name        string
		result      *DryRunResult
		options     FormatterOptions
		expectError bool
	}{
		{
			name:        "nil result",
			result:      nil,
			options:     FormatterOptions{DetailLevel: DetailLevelDetailed},
			expectError: true,
		},
		{
			name: "invalid detail level",
			result: &DryRunResult{
				Metadata: &ResultMetadata{},
			},
			options: FormatterOptions{
				DetailLevel: DetailLevel(999),
			},
			expectError: false, // Should fallback gracefully
		},
		{
			name: "invalid output format",
			result: &DryRunResult{
				Metadata: &ResultMetadata{},
			},
			options: FormatterOptions{
				OutputFormat: OutputFormat(999),
			},
			expectError: false, // Should fallback gracefully
		},
		{
			name: "corrupted resource analyses",
			result: &DryRunResult{
				Metadata: &ResultMetadata{},
				ResourceAnalyses: []ResourceAnalysis{
					{
						Type:      ResourceType("invalid"),
						Operation: ResourceOperation("invalid"),
						Target:    "",
					},
				},
			},
			options: FormatterOptions{
				DetailLevel:  DetailLevelDetailed,
				OutputFormat: OutputFormatText,
			},
			expectError: false, // Should handle gracefully
		},
	}

	formatters := []struct {
		name      string
		formatter Formatter
	}{
		{"TextFormatter", NewTextFormatter()},
		{"JSONFormatter", NewJSONFormatter()},
	}

	for _, formatter := range formatters {
		for _, tt := range tests {
			t.Run(formatter.name+"_"+tt.name, func(t *testing.T) {
				output, err := formatter.formatter.FormatResult(tt.result, tt.options)

				if tt.expectError {
					assert.Error(t, err, "expected error but got none")
				} else {
					assert.NoError(t, err, "unexpected error: %v", err)
					if err == nil {
						assert.NotEmpty(t, output, "expected non-empty output")
					}
				}
			})
		}
	}
}

// TestConcurrentExecution tests concurrent dry-run execution
func TestConcurrentExecution(t *testing.T) {
	const numGoroutines = 10
	const commandsPerGoroutine = 5

	dryRunOpts := &DryRunOptions{
		DetailLevel:   DetailLevelDetailed,
		OutputFormat:  OutputFormatText,
		ShowSensitive: false,
		VerifyFiles:   true,
	}

	results := make(chan *DryRunResult, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			ctx := context.Background()
			manager := NewDryRunResourceManager(nil, nil, dryRunOpts)

			group := &runnertypes.CommandGroup{
				Name:        "concurrent-test-group",
				Description: "Concurrent test group",
				Priority:    1,
			}

			envVars := map[string]string{
				"GOROUTINE_ID": strconv.Itoa(goroutineID),
			}

			// Execute multiple commands in this goroutine
			for j := 0; j < commandsPerGoroutine; j++ {
				command := runnertypes.Command{
					Name:        "concurrent-cmd",
					Description: "Concurrent test command",
					Cmd:         "echo concurrent test",
				}

				_, err := manager.ExecuteCommand(ctx, command, group, envVars)
				if err != nil {
					errors <- err
					return
				}
			}

			result := manager.GetDryRunResults()
			results <- result
		}(i)
	}

	// Collect results
	var collectedResults []*DryRunResult
	var collectedErrors []error

	for i := 0; i < numGoroutines; i++ {
		select {
		case result := <-results:
			collectedResults = append(collectedResults, result)
		case err := <-errors:
			collectedErrors = append(collectedErrors, err)
		}
	}

	// Verify results
	assert.Empty(t, collectedErrors, "should not have any errors during concurrent execution")
	assert.Len(t, collectedResults, numGoroutines, "should have results from all goroutines")

	// Verify each result has the expected number of analyses
	for i, result := range collectedResults {
		assert.NotNil(t, result, "result %d should not be nil", i)
		if result != nil {
			assert.Len(t, result.ResourceAnalyses, commandsPerGoroutine,
				"result %d should have %d analyses", i, commandsPerGoroutine)
		}
	}
}

// TestResourceManagerStateConsistency tests that resource manager maintains consistent state
func TestResourceManagerStateConsistency(t *testing.T) {
	ctx := context.Background()

	dryRunOpts := &DryRunOptions{
		DetailLevel:   DetailLevelDetailed,
		OutputFormat:  OutputFormatText,
		ShowSensitive: false,
		VerifyFiles:   true,
	}

	manager := NewDefaultResourceManager(nil, nil, nil, ExecutionModeDryRun, dryRunOpts)
	require.NotNil(t, manager)

	command := runnertypes.Command{
		Name:        "state-test",
		Description: "State consistency test",
		Cmd:         "echo state test",
	}

	group := &runnertypes.CommandGroup{
		Name:        "state-test-group",
		Description: "State test group",
		Priority:    1,
	}

	envVars := map[string]string{
		"STATE_VAR": "state_value",
	}

	// Execute the same command multiple times
	const numExecutions = 5
	for i := 0; i < numExecutions; i++ {
		result, err := manager.ExecuteCommand(ctx, command, group, envVars)
		assert.NoError(t, err, "execution %d should not error", i)
		assert.NotNil(t, result, "execution %d should return result", i)

		// Verify mode consistency
		assert.Equal(t, ExecutionModeDryRun, manager.GetMode(),
			"mode should remain consistent after execution %d", i)

		// Verify dry-run results accumulate
		dryRunResult := manager.GetDryRunResults()
		require.NotNil(t, dryRunResult, "should have dry-run results after execution %d", i)
		assert.Len(t, dryRunResult.ResourceAnalyses, i+1,
			"should have %d analyses after execution %d", i+1, i)
	}
}

// TestEdgeCaseInputs tests handling of edge case inputs
func TestEdgeCaseInputs(t *testing.T) {
	tests := []struct {
		name        string
		command     runnertypes.Command
		group       *runnertypes.CommandGroup
		envVars     map[string]string
		expectError bool
	}{
		{
			name: "unicode command",
			command: runnertypes.Command{
				Name: "unicode-test",
				Cmd:  "echo 'こんにちは世界'",
			},
			group: &runnertypes.CommandGroup{Name: "test"},
			envVars: map[string]string{
				"UNICODE_VAR": "値",
			},
			expectError: false,
		},
		{
			name: "very long command",
			command: runnertypes.Command{
				Name: "long-cmd",
				Cmd:  "echo " + string(make([]byte, 1000)),
			},
			group:       &runnertypes.CommandGroup{Name: "test"},
			envVars:     map[string]string{},
			expectError: false,
		},
		{
			name: "special characters in command",
			command: runnertypes.Command{
				Name: "special-chars",
				Cmd:  "echo 'test with $@#%^&*()[]{}|\\;:\"<>?/'",
			},
			group:       &runnertypes.CommandGroup{Name: "test"},
			envVars:     map[string]string{},
			expectError: false,
		},
		{
			name: "null bytes in environment",
			command: runnertypes.Command{
				Name: "null-test",
				Cmd:  "echo $NULL_VAR",
			},
			group: &runnertypes.CommandGroup{Name: "test"},
			envVars: map[string]string{
				"NULL_VAR": "test\x00null",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			dryRunOpts := &DryRunOptions{
				DetailLevel:  DetailLevelDetailed,
				OutputFormat: OutputFormatText,
			}

			manager := NewDryRunResourceManager(nil, nil, dryRunOpts)
			require.NotNil(t, manager)

			result, err := manager.ExecuteCommand(ctx, tt.command, tt.group, tt.envVars)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
