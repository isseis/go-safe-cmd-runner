//go:build test

package runner

import (
	"context"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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

// createRuntimeCommand creates a RuntimeCommand from a CommandSpec for testing.
// This is a local helper for the runner package. For other packages, use
// the exported helpers in internal/runner/executor/testing package.
//
// Usage:
//
//	spec := &runnertypes.CommandSpec{
//	    Name: "test-cmd",
//	    Cmd:  "/bin/echo",
//	    Args: []string{"hello"},
//	    WorkDir: "/tmp",
//	}
//	cmd := createRuntimeCommand(spec)
func createRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
	// Use the shared CreateRuntimeCommandFromSpec from executor/testing package
	// to avoid code duplication
	// Note: We can't import executor/testing package from runner package directly
	// because it would create a circular dependency, so we duplicate the implementation here

	// Use the shared timeout resolution logic with context
	commandTimeout := common.NewFromIntPtr(spec.Timeout)
	globalTimeout := common.NewUnsetTimeout() // Tests typically don't need global timeout
	effectiveTimeout, resolutionContext := common.ResolveTimeout(
		commandTimeout,
		common.NewUnsetTimeout(), // No group timeout in tests
		globalTimeout,
		spec.Name,
		"test-group",
	)

	// Set default workDir if not specified
	workDir := spec.WorkDir
	if workDir == "" {
		workDir = "/tmp" // Use /tmp as default for runner package tests
	}

	return &runnertypes.RuntimeCommand{
		Spec:              spec,
		ExpandedCmd:       spec.Cmd,
		ExpandedArgs:      spec.Args,
		ExpandedEnv:       make(map[string]string),
		ExpandedVars:      make(map[string]string),
		EffectiveWorkDir:  workDir,
		EffectiveTimeout:  effectiveTimeout,
		TimeoutResolution: resolutionContext,
	}
}

// matchRuntimeGroupWithName creates a mock.ArgumentMatcher that validates RuntimeGroup with expected group name.
// This is a helper function to avoid code duplication in mock expectations.
//
// Usage:
//
//	mockVerificationManager.On("VerifyGroupFiles", matchRuntimeGroupWithName("test-group")).Return(...)
func matchRuntimeGroupWithName(expectedName string) interface{} {
	return mock.MatchedBy(func(rg *runnertypes.RuntimeGroup) bool {
		return rg != nil && rg.Spec != nil && rg.Spec.Name == expectedName
	})
}
