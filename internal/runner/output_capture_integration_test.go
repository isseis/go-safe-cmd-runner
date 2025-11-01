// This file contains integration tests for output capture functionality

package runner

import (
	"context"
	"fmt"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestRunner_OutputCaptureIntegration tests basic output capture integration
func TestRunner_OutputCaptureIntegration(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		setupMock   func(*MockResourceManager)
		expectError bool
		description string
	}{
		{
			name: "BasicOutputCapture",
			setupMock: func(mockRM *MockResourceManager) {
				mockRM.On("ValidateOutputPath", "output.txt", mock.Anything).Return(nil)
				result := &resource.ExecutionResult{
					ExitCode: 0,
					Stdout:   "test output",
					Stderr:   "",
				}
				mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(resource.CommandToken(""), result, nil)
			},
			expectError: false,
			description: "Basic output capture should work with valid configuration",
		},
		{
			name: "OutputCaptureError",
			setupMock: func(mockRM *MockResourceManager) {
				mockRM.On("ValidateOutputPath", "output.txt", mock.Anything).Return(nil)
				mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(resource.CommandToken(""), nil, fmt.Errorf("output capture failed"))
			},
			expectError: true,
			description: "Output capture errors should be properly handled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create basic configuration with output capture
			cfg := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout:         common.IntPtr(30),
					OutputSizeLimit: 1024,
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test-group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:       "test-cmd",
								Cmd:        "echo",
								Args:       []string{"test"},
								OutputFile: "output.txt",
							},
						},
					},
				},
			}

			// Create mock resource manager
			mockRM := &MockResourceManager{}
			tt.setupMock(mockRM)

			// Create verification manager for dry-run (skips actual verification)
			verificationManager, err := verification.NewManagerForDryRun()
			require.NoError(t, err)

			// Create runner with proper options (using existing pattern)
			options := []Option{
				WithResourceManager(mockRM),
				WithVerificationManager(verificationManager),
				WithRunID("test-run-output-capture"),
			}

			// Create runner
			runner, err := NewRunner(cfg, options...)
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &cfg.Groups[0])

			if tt.expectError {
				require.Error(t, err, "Should return error for %s", tt.description)
			} else {
				require.NoError(t, err, "Should not return error for %s", tt.description)
			}

			// Verify mock expectations
			mockRM.AssertExpectations(t)
		})
	}
}

// TestRunner_OutputCaptureSecurityValidation tests that security validation
// occurs BEFORE command execution, creating a proper security boundary.
//
// This test verifies that:
// 1. Invalid output paths are rejected during validation phase
// 2. ExecuteCommand is never called for invalid paths
// 3. Only valid paths proceed to command execution
func TestRunner_OutputCaptureSecurityValidation(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name        string
		outputPath  string
		expectError string
		description string
	}{
		{
			name:        "PathTraversalAttempt",
			outputPath:  "../../../etc/passwd",
			expectError: "security validation failed",
			description: "Path traversal attempts should fail validation before command execution",
		},
		{
			name:        "AbsolutePathBlocked",
			outputPath:  "/etc/shadow",
			expectError: "security validation failed",
			description: "Absolute paths should fail validation before command execution",
		},
		{
			name:        "ValidOutputPath",
			outputPath:  "valid-output.txt",
			expectError: "",
			description: "Valid output paths should pass validation and execute commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create configuration with potentially problematic output path
			cfg := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout:         common.IntPtr(30),
					OutputSizeLimit: 1024,
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test-group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:       "test-cmd",
								Cmd:        "echo",
								Args:       []string{"test"},
								OutputFile: tt.outputPath,
							},
						},
					},
				},
			}

			// Create mock resource manager
			mockRM := &MockResourceManager{}

			// Setup mock expectations: validation always occurs first
			if tt.expectError == "" {
				// Success case: validation passes, then command executes
				mockRM.On("ValidateOutputPath", tt.outputPath, mock.Anything).Return(nil)

				// Only after successful validation should ExecuteCommand be called
				result := &resource.ExecutionResult{
					ExitCode: 0,
					Stdout:   "test",
					Stderr:   "",
				}
				mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(resource.CommandToken(""), result, nil)
			} else {
				// Failure case: validation fails, ExecuteCommand never gets called
				mockRM.On("ValidateOutputPath", tt.outputPath, mock.Anything).
					Return(fmt.Errorf("path validation failed: %s", tt.expectError))

				// Note: No ExecuteCommand expectation set - it should never be called
				// The mock will panic if ExecuteCommand is unexpectedly invoked
			}

			// Create verification manager for dry-run (skips actual verification)
			verificationManager, err := verification.NewManagerForDryRun()
			require.NoError(t, err)

			// Create runner with proper options
			var options []Option
			options = append(options, WithResourceManager(mockRM))
			options = append(options, WithVerificationManager(verificationManager))
			options = append(options, WithRunID("test-run-security"))

			// Create runner
			runner, err := NewRunner(cfg, options...)
			require.NoError(t, err)

			// Execute the group
			ctx := context.Background()
			err = runner.ExecuteGroup(ctx, &cfg.Groups[0])

			if tt.expectError != "" {
				require.Error(t, err, "Security validation should prevent execution for %s", tt.description)
				assert.Contains(t, err.Error(), tt.expectError)
				assert.Contains(t, err.Error(), "output path validation failed",
					"Error should indicate validation occurred before command execution")
			} else {
				require.NoError(t, err, "Valid paths should pass validation and execute successfully: %s", tt.description)
			}

			// Critical verification: ensure the security boundary works as expected
			// - ValidateOutputPath should always be called first
			// - ExecuteCommand should only be called after successful validation
			mockRM.AssertExpectations(t)
		})
	}
}
