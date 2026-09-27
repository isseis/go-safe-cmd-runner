//go:build test

// This file contains integration tests for output capture functionality

package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	securitytestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/security/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	resourcetestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/resource/testutil"
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
					Timeout:         new(int32(30)),
					OutputSizeLimit: new(int64(1024)),
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test-group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:       "test-cmd",
								Cmd:        "echo",
								Args:       []string{"test"},
								OutputFile: new("output.txt"),
							},
						},
					},
				},
			}

			// Create mock resource manager
			mockRM := &MockResourceManager{}
			tt.setupMock(mockRM)

			// Create isolated dry-run verification manager for tests.
			verificationManager, err := verification.NewManagerForTest(
				t.TempDir(),
				verification.WithDryRunMode(),
				verification.WithFileValidatorDisabled(),
			)
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
// errPathRejected is the rejection injected into the output-path validation seam.
var errPathRejected = errors.New("output path rejected by security validation")

func TestRunner_OutputCaptureSecurityValidation(t *testing.T) {
	setupSafeTestEnv(t)

	tests := []struct {
		name       string
		outputPath string
		// injectErr is what the resource manager reports for this path; nil means
		// validation passes and the command runs.
		injectErr   error
		description string
	}{
		{
			name:        "PathTraversalAttempt",
			outputPath:  "../../../etc/passwd",
			injectErr:   errPathRejected,
			description: "Path traversal attempts should fail validation before command execution",
		},
		{
			name:        "AbsolutePathBlocked",
			outputPath:  "/etc/shadow",
			injectErr:   errPathRejected,
			description: "Absolute paths should fail validation before command execution",
		},
		{
			name:        "ValidOutputPath",
			outputPath:  "valid-output.txt",
			injectErr:   nil,
			description: "Valid output paths should pass validation and execute commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create configuration with potentially problematic output path
			cfg := &runnertypes.ConfigSpec{
				Version: "1.0",
				Global: runnertypes.GlobalSpec{
					Timeout:         new(int32(30)),
					OutputSizeLimit: new(int64(1024)),
				},
				Groups: []runnertypes.GroupSpec{
					{
						Name: "test-group",
						Commands: []runnertypes.CommandSpec{
							{
								Name:       "test-cmd",
								Cmd:        "echo",
								Args:       []string{"test"},
								OutputFile: tu.StringPtrOrNil(tt.outputPath),
							},
						},
					},
				},
			}

			// Create mock resource manager
			mockRM := &MockResourceManager{}

			// Setup mock expectations: validation always occurs first
			if tt.injectErr == nil {
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
					Return(fmt.Errorf("path validation failed: %w", tt.injectErr))

				// Note: No ExecuteCommand expectation set - it should never be called
				// The mock will panic if ExecuteCommand is unexpectedly invoked
			}

			// Create isolated dry-run verification manager for tests.
			verificationManager, err := verification.NewManagerForTest(
				t.TempDir(),
				verification.WithDryRunMode(),
				verification.WithFileValidatorDisabled(),
			)
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

			if tt.injectErr != nil {
				// The rejection reaches the caller unchanged, and ExecuteCommand is
				// never reached — the mock has no expectation for it.
				require.ErrorIs(t, err, tt.injectErr, "Security validation should prevent execution for %s", tt.description)
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

// TestRunner_ZeroOutputSizeLimitIntegration runs a real command that writes
// more than the executor's retained-output window with output_size_limit = 0
// and an output file. The command must complete without a size-limit failure,
// the output file must hold every byte, and the result's stdout must be bounded
// and carry the omission marker.
func TestRunner_ZeroOutputSizeLimitIntegration(t *testing.T) {
	setupSafeTestEnv(t)

	const bulk = 100 * 1024 // newline-free bytes, beyond the 64 KiB retained window
	full := strings.Repeat("x", bulk)
	outputFile := filepath.Join(t.TempDir(), "zero-limit.out")

	script := fmt.Sprintf("head -c %d /dev/zero | tr -c x x", bulk)
	stdout, exitCode, err := runZeroLimitCommand(t, outputFile, script)
	require.NoError(t, err, "a zero output size limit must not fail on size")

	require.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "... omitting ", "the retained stdout must carry the omission marker")
	assert.Less(t, len(stdout), bulk, "the retained stdout must be smaller than the output")

	written, err := os.ReadFile(outputFile)
	require.NoError(t, err, "the output file must exist")
	assert.Equal(t, full, string(written), "the output file must hold every byte")
}

// runZeroLimitCommand runs script under sh through the real executor and
// resource manager with output_size_limit = 0 and an output file. It returns
// the retained stdout and the exit code; the final output file is at
// outputFile.
func runZeroLimitCommand(t *testing.T, outputFile, script string) (stdout string, exitCode int, err error) {
	t.Helper()

	exec := executor.NewDefaultExecutor()
	mockValidator := new(securitytestutil.MockValidator)
	pathResolver := &mockPathResolver{}
	pathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	outputMgr := output.NewDefaultOutputCaptureManager(allowOutputWriteValidator{})
	rm, err := resourcetestutil.NewDefaultResourceManager(
		exec,
		common.NewDefaultFileSystem(),
		nil, // no privilege manager: the command carries no run_as
		pathResolver,
		slog.Default(),
		resource.ExecutionModeNormal,
		nil, // dry-run disabled
		outputMgr,
		0,
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:          &runnertypes.ConfigSpec{},
		Executor:        exec,
		ResourceManager: rm,
		Validator:       mockValidator,
		RunID:           "test-run-zero-limit",
	})
	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)
	mockValidator.On("ValidateCommandAllowed", mock.Anything, mock.Anything).Return(nil)
	mockValidator.On("SanitizeOutputForLogging", mock.Anything).Return("")

	limit, err := common.NewOutputSizeLimit(0)
	require.NoError(t, err)

	groupSpec := &runnertypes.GroupSpec{Name: "zero-limit-group"}
	cmd := &runnertypes.RuntimeCommand{
		Spec: &runnertypes.CommandSpec{
			Name:       "writes-lots",
			Cmd:        "/bin/sh",
			Args:       []string{"-c", script},
			OutputFile: &outputFile,
			RiskLevel:  runnertypes.RiskLevelMediumPtr,
		},
		ExpandedCmd:              "/bin/sh",
		ExpandedArgs:             []string{"-c", script},
		EffectiveTimeout:         30,
		EffectiveOutputSizeLimit: limit,
	}

	stdout, _, exitCode, err = ge.executeSingleCommand(
		context.Background(), cmd, groupSpec, newDefaultRuntimeGroup(groupSpec), newDefaultRuntimeGlobal())
	return stdout, exitCode, err
}
