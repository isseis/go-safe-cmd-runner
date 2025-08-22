package resource

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing
type mockCommandExecutor struct{}

func (m *mockCommandExecutor) Execute(_ context.Context, cmd runnertypes.Command, _ map[string]string) (*executor.Result, error) {
	// Normal mode test implementation
	return &executor.Result{
		ExitCode: 0,
		Stdout:   fmt.Sprintf("Mock execution: %s", cmd.Cmd),
		Stderr:   "",
	}, nil
}

func (m *mockCommandExecutor) Validate(_ runnertypes.Command) error {
	// Executor level validation is minimal since validation happens at ResourceManager level
	return nil
}

type mockFileSystem struct{}

func (m *mockFileSystem) CreateTempDir(_, prefix string) (string, error) {
	return fmt.Sprintf("/tmp/%s-mock", prefix), nil
}

func (m *mockFileSystem) RemoveAll(_ string) error {
	return nil
}

func (m *mockFileSystem) FileExists(_ string) (bool, error) {
	return true, nil
}

// TestErrorScenariosConsistency tests error handling consistency between execution modes
func TestErrorScenariosConsistency(t *testing.T) {
	tests := []struct {
		name          string
		command       runnertypes.Command
		group         *runnertypes.CommandGroup
		envVars       map[string]string
		expectError   bool
		expectedError error
		description   string
	}{
		{
			name: "empty command",
			command: runnertypes.Command{
				Name: "test-cmd",
				Cmd:  "",
			},
			group:         &runnertypes.CommandGroup{Name: "test-group"},
			envVars:       map[string]string{},
			expectError:   true,
			expectedError: ErrEmptyCommand,
			description:   "Both modes should reject empty commands",
		},
		{
			name: "empty command name",
			command: runnertypes.Command{
				Name: "",
				Cmd:  "echo test",
			},
			group:         &runnertypes.CommandGroup{Name: "test-group"},
			envVars:       map[string]string{},
			expectError:   true,
			expectedError: ErrEmptyCommandName,
			description:   "Both modes should reject commands with empty names",
		},
		{
			name: "nil command group",
			command: runnertypes.Command{
				Name: "test-cmd",
				Cmd:  "echo test",
			},
			group:         nil,
			envVars:       map[string]string{},
			expectError:   true,
			expectedError: ErrNilCommandGroup,
			description:   "Both modes should reject nil command groups",
		},
		{
			name: "empty group name",
			command: runnertypes.Command{
				Name: "test-cmd",
				Cmd:  "echo test",
			},
			group:         &runnertypes.CommandGroup{Name: ""},
			envVars:       map[string]string{},
			expectError:   true,
			expectedError: ErrEmptyGroupName,
			description:   "Both modes should reject groups with empty names",
		},
		{
			name: "valid command",
			command: runnertypes.Command{
				Name: "test-cmd",
				Cmd:  "echo test",
			},
			group:         &runnertypes.CommandGroup{Name: "test-group"},
			envVars:       map[string]string{"TEST": "value"},
			expectError:   false,
			expectedError: nil,
			description:   "Both modes should accept valid commands",
		},
		{
			name: "large environment variables",
			command: runnertypes.Command{
				Name: "large-env-test",
				Cmd:  "echo $LARGE_VAR",
			},
			group: &runnertypes.CommandGroup{Name: "test-group"},
			envVars: func() map[string]string {
				largeValue := make([]byte, 10000)
				for i := range largeValue {
					largeValue[i] = 'A'
				}
				return map[string]string{
					"LARGE_VAR": string(largeValue),
				}
			}(),
			expectError:   false,
			expectedError: nil,
			description:   "Both modes should handle large environment variables",
		},
		{
			name: "unicode command",
			command: runnertypes.Command{
				Name: "unicode-test",
				Cmd:  "echo 'こんにちは世界'",
			},
			group: &runnertypes.CommandGroup{Name: "test-group"},
			envVars: map[string]string{
				"UNICODE_VAR": "値",
			},
			expectError:   false,
			expectedError: nil,
			description:   "Both modes should handle unicode characters",
		},
		{
			name: "special characters in command",
			command: runnertypes.Command{
				Name: "special-chars",
				Cmd:  "echo 'test with $@#%^&*()[]{}|\\;:\"<>?/'",
			},
			group:         &runnertypes.CommandGroup{Name: "test-group"},
			envVars:       map[string]string{},
			expectError:   false,
			expectedError: nil,
			description:   "Both modes should handle special characters",
		},
		{
			name: "very long command",
			command: runnertypes.Command{
				Name: "long-cmd",
				Cmd:  "echo " + string(make([]byte, 1000)),
			},
			group:         &runnertypes.CommandGroup{Name: "test-group"},
			envVars:       map[string]string{},
			expectError:   false,
			expectedError: nil,
			description:   "Both modes should handle very long commands",
		},
		{
			name: "nil environment variables",
			command: runnertypes.Command{
				Name: "nil-env-test",
				Cmd:  "echo test",
			},
			group:         &runnertypes.CommandGroup{Name: "test-group"},
			envVars:       nil,
			expectError:   false,
			expectedError: nil,
			description:   "Both modes should handle nil environment variables",
		},
	}

	executionModes := []struct {
		name     string
		setup    func() ResourceManager
		isDryRun bool
	}{
		{
			name: "DryRun",
			setup: func() ResourceManager {
				opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
				return NewDryRunResourceManager(nil, nil, opts)
			},
			isDryRun: true,
		},
		{
			name: "Normal",
			setup: func() ResourceManager {
				mockExecutor := &mockCommandExecutor{}
				mockFS := &mockFileSystem{}
				return NewNormalResourceManager(mockExecutor, mockFS, nil)
			},
			isDryRun: false,
		},
	}

	for _, mode := range executionModes {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s_%s", mode.name, tt.name), func(t *testing.T) {
				ctx := context.Background()
				manager := mode.setup()

				result, err := manager.ExecuteCommand(ctx, tt.command, tt.group, tt.envVars)

				if tt.expectError {
					assert.Error(t, err, "expected error for %s", tt.description)
					if tt.expectedError != nil {
						assert.ErrorIs(t, err, tt.expectedError,
							"expected error %v, got %v for %s", tt.expectedError, err, tt.description)
					}
					assert.Nil(t, result, "result should be nil when error occurs")
				} else {
					assert.NoError(t, err, "unexpected error for %s: %v", tt.description, err)
					assert.NotNil(t, result, "result should not be nil for valid command")
					assert.Equal(t, mode.isDryRun, result.DryRun, "DryRun flag should match execution mode")
				}
			})
		}
	}
}

// TestConcurrentExecutionConsistency tests concurrent execution consistency between modes
func TestConcurrentExecutionConsistency(t *testing.T) {
	const numGoroutines = 5
	const commandsPerGoroutine = 3

	executionModes := []struct {
		name     string
		setup    func() ResourceManager
		isDryRun bool
	}{
		{
			name: "DryRun",
			setup: func() ResourceManager {
				opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
				return NewDryRunResourceManager(nil, nil, opts)
			},
			isDryRun: true,
		},
		{
			name: "Normal",
			setup: func() ResourceManager {
				mockExecutor := &mockCommandExecutor{}
				mockFS := &mockFileSystem{}
				return NewNormalResourceManager(mockExecutor, mockFS, nil)
			},
			isDryRun: false,
		},
	}

	for _, mode := range executionModes {
		t.Run(mode.name+"_concurrent", func(t *testing.T) {
			results := make(chan *ExecutionResult, numGoroutines*commandsPerGoroutine)
			errors := make(chan error, numGoroutines*commandsPerGoroutine)

			for i := range numGoroutines {
				go func(goroutineID int) {
					ctx := context.Background()
					manager := mode.setup()

					group := &runnertypes.CommandGroup{
						Name: "concurrent-test-group",
					}

					envVars := map[string]string{
						"GOROUTINE_ID": strconv.Itoa(goroutineID),
					}

					for j := range commandsPerGoroutine {
						command := runnertypes.Command{
							Name: fmt.Sprintf("concurrent-cmd-%d-%d", goroutineID, j),
							Cmd:  "echo concurrent test",
						}

						result, err := manager.ExecuteCommand(ctx, command, group, envVars)
						if err != nil {
							errors <- err
						} else {
							results <- result
						}
					}
				}(i)
			}

			// Collect results
			var collectedResults []*ExecutionResult
			var collectedErrors []error

			expectedResults := numGoroutines * commandsPerGoroutine
			for range expectedResults {
				select {
				case result := <-results:
					collectedResults = append(collectedResults, result)
				case err := <-errors:
					collectedErrors = append(collectedErrors, err)
				}
			}

			// Verify results
			assert.Empty(t, collectedErrors, "should not have any errors during concurrent execution")
			assert.Len(t, collectedResults, expectedResults, "should have results from all executions")

			// Verify mode consistency
			for i, result := range collectedResults {
				assert.Equal(t, mode.isDryRun, result.DryRun,
					"result %d should have correct DryRun flag", i)
			}
		})
	}
}

// TestDryRunManagerErrorHandling tests various error conditions and edge cases for DryRunResourceManager.
// This test focuses specifically on dry-run implementation behavior, while TestErrorScenariosConsistency
// tests consistency between normal and dry-run modes.
func TestDryRunManagerErrorHandling(t *testing.T) {
	tests := []struct {
		name         string
		setup        func() (*DryRunResourceManager, error)
		command      runnertypes.Command
		group        *runnertypes.CommandGroup
		envVars      map[string]string
		expectError  bool
		expectResult bool
	}{
		{
			name: "concurrent analysis recording",
			setup: func() (*DryRunResourceManager, error) {
				opts := &DryRunOptions{DetailLevel: DetailLevelDetailed}
				return NewDryRunResourceManager(nil, nil, opts), nil
			},
			command: runnertypes.Command{
				Name: "concurrent-test",
				Cmd:  "echo concurrent",
			},
			group: &runnertypes.CommandGroup{
				Name: "test-group",
			},
			envVars:      map[string]string{},
			expectError:  false,
			expectResult: true,
		},
		{
			name: "invalid dry-run options",
			setup: func() (*DryRunResourceManager, error) {
				// Test with invalid detail level
				opts := &DryRunOptions{DetailLevel: DetailLevel(999)}
				return NewDryRunResourceManager(nil, nil, opts), nil
			},
			command: runnertypes.Command{
				Name: "options-test",
				Cmd:  "echo test",
			},
			group: &runnertypes.CommandGroup{
				Name: "test-group",
			},
			envVars:      map[string]string{},
			expectError:  false, // Should handle gracefully
			expectResult: true,
		},
		{
			name: "analysis recording with nil options",
			setup: func() (*DryRunResourceManager, error) {
				// Test with nil options
				return NewDryRunResourceManager(nil, nil, nil), nil
			},
			command: runnertypes.Command{
				Name: "nil-options-test",
				Cmd:  "echo test",
			},
			group: &runnertypes.CommandGroup{
				Name: "test-group",
			},
			envVars:      map[string]string{},
			expectError:  false,
			expectResult: true,
		},
		{
			name: "dry-run result consistency",
			setup: func() (*DryRunResourceManager, error) {
				opts := &DryRunOptions{
					DetailLevel:   DetailLevelDetailed,
					OutputFormat:  OutputFormatJSON,
					ShowSensitive: true,
					VerifyFiles:   true,
				}
				return NewDryRunResourceManager(nil, nil, opts), nil
			},
			command: runnertypes.Command{
				Name: "consistency-test",
				Cmd:  "echo consistency",
			},
			group: &runnertypes.CommandGroup{
				Name: "test-group",
			},
			envVars:      map[string]string{"SENSITIVE": "secret"},
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

				// Dry-run specific validations
				if result != nil {
					assert.True(t, result.DryRun, "result should indicate dry-run mode")

					// Check that dry-run results are available
					dryRunResult := manager.GetDryRunResults()
					assert.NotNil(t, dryRunResult, "dry-run results should be available")

					// Validate metadata
					if dryRunResult != nil && dryRunResult.Metadata != nil {
						assert.NotEmpty(t, dryRunResult.Metadata.RunID, "run ID should be set")
						assert.False(t, dryRunResult.Metadata.GeneratedAt.IsZero(), "generated time should be set")
					}
				}
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

	for i := range numGoroutines {
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
			for range commandsPerGoroutine {
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

	for range numGoroutines {
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
	for i := range numExecutions {
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

	// Test edge case: null bytes in environment variables
	// This tests both normal and dry-run modes for proper handling
	t.Run("null_bytes_in_environment", func(t *testing.T) {
		nullCommand := runnertypes.Command{
			Name: "null-test",
			Cmd:  "echo $NULL_VAR",
		}

		nullEnvVars := map[string]string{
			"NULL_VAR": "test\x00null",
		}

		// Test with dry-run mode
		dryRunManager := NewDefaultResourceManager(nil, nil, nil, ExecutionModeDryRun, dryRunOpts)
		result, err := dryRunManager.ExecuteCommand(ctx, nullCommand, group, nullEnvVars)
		assert.NoError(t, err, "dry-run mode should handle null bytes in environment")
		assert.NotNil(t, result, "dry-run mode should return result")
		assert.True(t, result.DryRun, "result should indicate dry-run mode")

		// Test with normal mode
		normalManager := NewDefaultResourceManager(&mockCommandExecutor{}, &mockFileSystem{}, nil, ExecutionModeNormal, nil)
		result, err = normalManager.ExecuteCommand(ctx, nullCommand, group, nullEnvVars)
		assert.NoError(t, err, "normal mode should handle null bytes in environment")
		assert.NotNil(t, result, "normal mode should return result")
		assert.False(t, result.DryRun, "result should indicate normal mode")
	})
}
