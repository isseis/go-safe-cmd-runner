//go:build test

package runner

import (
	"context"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	runnertesting "github.com/isseis/go-safe-cmd-runner/internal/runner/testing"
	"github.com/stretchr/testify/mock"
)

// MockResourceManager is an alias to the shared mock implementation
type MockResourceManager = runnertesting.MockResourceManager

// MockGroupExecutor is a mock implementation of GroupExecutor for testing
type MockGroupExecutor struct {
	mock.Mock
}

// ExecuteGroup executes all commands in a group sequentially
func (m *MockGroupExecutor) ExecuteGroup(ctx context.Context, groupSpec *runnertypes.GroupSpec, runtimeGlobal *runnertypes.RuntimeGlobal) error {
	args := m.Called(ctx, groupSpec, runtimeGlobal)
	return args.Error(0)
}

// setupTestEnv sets up a clean test environment.
func setupTestEnv(t *testing.T, envVars map[string]string) {
	t.Helper()

	// Set up the test environment variables
	for key, value := range envVars {
		t.Setenv(key, value)
	}
}

// setupSafeTestEnv sets up a minimal safe environment for tests.
// This is useful for security-related tests where we want to ensure a clean, minimal environment.
func setupSafeTestEnv(t *testing.T) {
	t.Helper()
	safeEnv := map[string]string{
		"PATH": "/usr/bin:/bin",
		"HOME": "/home/test",
		"USER": "test",
	}
	setupTestEnv(t, safeEnv)
}

// setupFailedMockExecution sets up mock for failed command execution with custom error
func setupFailedMockExecution(m *MockResourceManager, err error) {
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(resource.CommandToken(""), nil, err)
}

// matchRuntimeGroupWithName creates a mock.ArgumentMatcher that validates RuntimeGroup with expected group name.
// This is a helper function to avoid code duplication in mock expectations.
//
// Usage:
//
//	mockVerificationManager.On("VerifyGroupFiles", matchRuntimeGroupWithName("test-group")).Return(...)
func matchRuntimeGroupWithName(expectedName string) any {
	return mock.MatchedBy(func(rg *runnertypes.RuntimeGroup) bool {
		return rg != nil && rg.Spec != nil && rg.Spec.Name == expectedName
	})
}

// WithSecurityLogger sets the security logger for timeout-related security events.
func WithSecurityLogger(logger *logging.SecurityLogger) GroupExecutorOption {
	return func(opts *groupExecutorOptions) {
		opts.securityLogger = logger
	}
}
