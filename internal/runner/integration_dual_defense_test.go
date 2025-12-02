//go:build test

package runner

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	securitytesting "github.com/isseis/go-safe-cmd-runner/internal/runner/security/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	verificationtesting "github.com/isseis/go-safe-cmd-runner/internal/verification/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestIntegration_DualDefense tests that both Case 1 (RedactingHandler) and Case 2 (Validator sanitization)
// work together to provide defense-in-depth protection against sensitive data leakage.
func TestIntegration_DualDefense(t *testing.T) {
	// Create a buffer to capture log output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create a separate failure logger without RedactingHandler
	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)

	// Wrap with redacting handler (Case 1)
	redactingHandler := redaction.NewRedactingHandler(handler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Create test configuration with command that outputs sensitive data
	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/sh",
				Args: []string{"-c", "echo 'API response: api_key=secret123'; echo 'password=mypass' >&2"},
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: commontesting.Int32Ptr(30)},
	}

	// Create real executor and resource manager
	exec := executor.NewDefaultExecutor()
	fs := common.NewDefaultFileSystem()

	// Use REAL validator with redaction enabled (Case 2)
	// Include AllowedCommands patterns to allow /bin/sh for testing
	// Note: /bin/sh may be a symlink to /usr/bin/dash or similar, so we need both patterns
	realValidator, err := security.NewValidator(&security.Config{
		AllowedCommands: []string{"^/bin/.*", "^/usr/bin/.*"},
		LoggingOptions: security.LoggingOptions{
			RedactSensitiveInfo: true,
		},
	})
	require.NoError(t, err)

	mockVerificationManager := new(verificationtesting.MockManager)

	// Create mock path resolver
	mockPathResolver := &mockPathResolver{}
	mockPathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	// Output manager will be created by NewDefaultResourceManager
	var outputMgr output.CaptureManager

	rm, err := resource.NewDefaultResourceManager(
		exec,
		fs,
		nil,
		mockPathResolver,
		logger,
		resource.ExecutionModeNormal,
		nil,
		outputMgr,
		0,
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:              &runnertypes.ConfigSpec{},
		Executor:            exec,
		ResourceManager:     rm,
		Validator:           realValidator, // Use real validator
		VerificationManager: mockVerificationManager,
		RunID:               "test-run-dual-defense",
	})

	// Mock verification manager
	mockVerificationManager.On("VerifyGroupFiles", matchRuntimeGroupWithName("test-group")).Return(&verification.Result{}, nil)
	mockVerificationManager.On("ResolvePath", "/bin/sh").Return("/bin/sh", nil)

	ctx := context.Background()
	err = ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.NoError(t, err, "command should succeed")

	// Check log output for sensitive patterns
	logOutput := logBuffer.String()

	// Verify that sensitive data is redacted
	// Both Case 2 (early sanitization) and Case 1 (log handler) should prevent leakage
	assert.NotContains(t, logOutput, "secret123", "API key should be redacted from logs")
	assert.NotContains(t, logOutput, "mypass", "password should be redacted from logs")

	// Verify that [REDACTED] placeholder appears
	assert.Contains(t, logOutput, "[REDACTED]", "redacted placeholder should appear in logs")

	t.Logf("Log output sample: %s", logOutput[:min(len(logOutput), 500)])
}

// TestIntegration_Case1Only tests that RedactingHandler alone (Case 1) can protect
// against sensitive data leakage even when Case 2 (validator sanitization) is disabled.
func TestIntegration_Case1Only(t *testing.T) {
	// Create a buffer to capture log output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create a separate failure logger without RedactingHandler
	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)

	// Wrap with redacting handler (Case 1 - enabled)
	redactingHandler := redaction.NewRedactingHandler(handler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Create test configuration with command that outputs sensitive data
	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/sh",
				Args: []string{"-c", "echo 'token=abc123xyz'"},
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: commontesting.Int32Ptr(30)},
	}

	// Create real executor and resource manager
	exec := executor.NewDefaultExecutor()
	fs := common.NewDefaultFileSystem()

	// Use MOCK validator with sanitization disabled (Case 2 - disabled)
	// This simulates a scenario where Case 2 is not working, so Case 1 must protect alone
	mockValidator := new(securitytesting.MockValidator)
	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)
	// Mock ValidateCommandAllowed - allow all commands for this test
	mockValidator.On("ValidateCommandAllowed", mock.Anything, mock.Anything).Return(nil)
	// Return original output WITHOUT sanitization (simulating Case 2 disabled)
	// For testify mock, we need to set up the expectation to return the argument value
	mockValidator.On("SanitizeOutputForLogging", mock.MatchedBy(func(_ string) bool {
		return true // Match any input
	})).Return("token=abc123xyz") // This will be the actual output from the command

	mockVerificationManager := new(verificationtesting.MockManager)

	// Create mock path resolver
	mockPathResolver := &mockPathResolver{}
	mockPathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	// Output manager will be created by NewDefaultResourceManager
	var outputMgr output.CaptureManager

	rm, err := resource.NewDefaultResourceManager(
		exec,
		fs,
		nil,
		mockPathResolver,
		logger,
		resource.ExecutionModeNormal,
		nil,
		outputMgr,
		0,
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:              &runnertypes.ConfigSpec{},
		Executor:            exec,
		ResourceManager:     rm,
		Validator:           mockValidator, // Use mock validator (no sanitization)
		VerificationManager: mockVerificationManager,
		RunID:               "test-run-case1-only",
	})

	// Mock verification manager
	mockVerificationManager.On("VerifyGroupFiles", matchRuntimeGroupWithName("test-group")).Return(&verification.Result{}, nil)
	mockVerificationManager.On("ResolvePath", "/bin/sh").Return("/bin/sh", nil)

	ctx := context.Background()
	err = ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.NoError(t, err, "command should succeed")

	// Check log output for sensitive patterns
	logOutput := logBuffer.String()

	// Verify that Case 1 (RedactingHandler) alone protects against leakage
	assert.NotContains(t, logOutput, "abc123xyz", "token should be redacted by Case 1 even though Case 2 is disabled")

	// Verify that [REDACTED] placeholder appears
	assert.Contains(t, logOutput, "[REDACTED]", "redacted placeholder should appear in logs")

	t.Logf("Log output sample (Case 1 only): %s", logOutput[:min(len(logOutput), 500)])
}

// TestIntegration_Case2Only tests that Validator sanitization (Case 2) provides
// PARTIAL protection by sanitizing CommandResult fields.
// NOTE: This test documents a limitation - Case 2 alone is NOT sufficient because
// debug logs may still contain raw output. This is why we need Case 1 (RedactingHandler)
// as the primary defense mechanism.
func TestIntegration_Case2Only(t *testing.T) {
	// Create a buffer to capture log output WITHOUT RedactingHandler (Case 1 disabled)
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Use Info level to avoid DEBUG logs with raw output
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Create test configuration with command that outputs sensitive data
	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/sh",
				Args: []string{"-c", "echo 'password=secret999'"},
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: commontesting.Int32Ptr(30)},
	}

	// Create real executor and resource manager
	exec := executor.NewDefaultExecutor()
	fs := common.NewDefaultFileSystem()

	// Use REAL validator with redaction enabled (Case 2 - enabled)
	// Include AllowedCommands patterns to allow /bin/sh for testing
	// Note: /bin/sh may be a symlink to /usr/bin/dash or similar, so we need both patterns
	realValidator, err := security.NewValidator(&security.Config{
		AllowedCommands: []string{"^/bin/.*", "^/usr/bin/.*"},
		LoggingOptions: security.LoggingOptions{
			RedactSensitiveInfo: true,
		},
	})
	require.NoError(t, err)

	mockVerificationManager := new(verificationtesting.MockManager)

	// Create mock path resolver
	mockPathResolver := &mockPathResolver{}
	mockPathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	// Output manager will be created by NewDefaultResourceManager
	var outputMgr output.CaptureManager

	rm, err := resource.NewDefaultResourceManager(
		exec,
		fs,
		nil,
		mockPathResolver,
		logger,
		resource.ExecutionModeNormal,
		nil,
		outputMgr,
		0,
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:              &runnertypes.ConfigSpec{},
		Executor:            exec,
		ResourceManager:     rm,
		Validator:           realValidator, // Use real validator
		VerificationManager: mockVerificationManager,
		RunID:               "test-run-case2-only",
	})

	// Mock verification manager
	mockVerificationManager.On("VerifyGroupFiles", matchRuntimeGroupWithName("test-group")).Return(&verification.Result{}, nil)
	mockVerificationManager.On("ResolvePath", "/bin/sh").Return("/bin/sh", nil)

	ctx := context.Background()
	err = ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.NoError(t, err, "command should succeed")

	// Check log output for sensitive patterns
	logOutput := logBuffer.String()

	// NOTE: At INFO level (no DEBUG logs), Case 2 should protect CommandResult fields
	// However, if DEBUG logs were enabled, raw output might still leak
	// This demonstrates why Case 1 (RedactingHandler) is the PRIMARY defense

	// At INFO level, CommandResult logging will show sanitized output
	// We're testing that the sanitized fields are used in INFO-level logs
	t.Logf("Log output sample (Case 2 only, INFO level): %s", logOutput[:min(len(logOutput), 500)])

	// With INFO level, we should NOT see the raw sensitive data in CommandResult logs
	// because the Validator sanitized it before creating CommandResult
	assert.NotContains(t, logOutput, "password=secret999",
		"At INFO level, password should not appear because CommandResult was sanitized by Case 2")
}

// TestIntegration_Case2Only_DebugLeakage tests that Validator sanitization (Case 2) alone
// is NOT sufficient at DEBUG level because debug logs may contain raw output BEFORE sanitization.
// This test intentionally demonstrates the vulnerability that Case 1 (RedactingHandler) prevents.
// NOTE: This test is expected to FAIL (leakage expected) - it documents why Case 1 is essential.
func TestIntegration_Case2Only_DebugLeakage(t *testing.T) {
	// Create a buffer to capture log output WITHOUT RedactingHandler (Case 1 disabled)
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug, // Use DEBUG level to expose the vulnerability
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Create test configuration with command that outputs sensitive data
	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/sh",
				Args: []string{"-c", "echo 'api_key=leaked_secret_456'"},
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: commontesting.Int32Ptr(30)},
	}

	// Create real executor and resource manager
	exec := executor.NewDefaultExecutor()
	fs := common.NewDefaultFileSystem()

	// Use REAL validator with redaction enabled (Case 2 - enabled)
	// However, this is NOT sufficient at DEBUG level
	// Include AllowedCommands patterns to allow /bin/sh for testing
	// Note: /bin/sh may be a symlink to /usr/bin/dash or similar, so we need both patterns
	realValidator, err := security.NewValidator(&security.Config{
		AllowedCommands: []string{"^/bin/.*", "^/usr/bin/.*"},
		LoggingOptions: security.LoggingOptions{
			RedactSensitiveInfo: true,
		},
	})
	require.NoError(t, err)

	mockVerificationManager := new(verificationtesting.MockManager)

	// Create mock path resolver
	mockPathResolver := &mockPathResolver{}
	mockPathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	// Output manager will be created by NewDefaultResourceManager
	var outputMgr output.CaptureManager

	rm, err := resource.NewDefaultResourceManager(
		exec,
		fs,
		nil,
		mockPathResolver,
		logger,
		resource.ExecutionModeNormal,
		nil,
		outputMgr,
		0,
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:              &runnertypes.ConfigSpec{},
		Executor:            exec,
		ResourceManager:     rm,
		Validator:           realValidator, // Use real validator
		VerificationManager: mockVerificationManager,
		RunID:               "test-run-case2-debug-leak",
	})

	// Mock verification manager
	mockVerificationManager.On("VerifyGroupFiles", matchRuntimeGroupWithName("test-group")).Return(&verification.Result{}, nil)
	mockVerificationManager.On("ResolvePath", "/bin/sh").Return("/bin/sh", nil)

	ctx := context.Background()
	err = ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.NoError(t, err, "command should succeed")

	// Check log output for sensitive patterns
	logOutput := logBuffer.String()

	t.Logf("Log output sample (Case 2 only, DEBUG level - LEAKAGE EXPECTED): %s",
		logOutput[:min(len(logOutput), 1000)])

	// IMPORTANT: This assertion documents the VULNERABILITY when Case 1 is absent
	// At DEBUG level, even though Case 2 sanitizes CommandResult fields,
	// debug logs from executor or other components may log raw output BEFORE sanitization happens.
	// This is why Case 1 (RedactingHandler) is ESSENTIAL - it protects ALL log output at the handler level.
	//
	// If this assertion passes (sensitive data found), it proves Case 2 alone is insufficient.
	// This motivates the dual-defense strategy where Case 1 is the primary defense.
	assert.Contains(t, logOutput, "leaked_secret_456",
		"VULNERABILITY DEMONSTRATION: At DEBUG level without Case 1 (RedactingHandler), "+
			"sensitive data leaks through debug logs even though Case 2 sanitizes CommandResult. "+
			"This proves why Case 1 is the PRIMARY and ESSENTIAL defense mechanism.")

	// Also verify that the vulnerability is specifically related to DEBUG level logging
	// and that the raw data appears in debug-level log entries
	t.Logf("This test demonstrates why Case 1 (RedactingHandler) is non-negotiable for security. " +
		"Case 2 alone cannot prevent leakage at DEBUG level because debug logs may contain " +
		"raw output from various execution stages BEFORE sanitization occurs.")
}
