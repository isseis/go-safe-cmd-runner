//go:build test

package runner

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	runnertesting "github.com/isseis/go-safe-cmd-runner/internal/runner/testing"
	"github.com/stretchr/testify/mock"
)

// MockResourceManager is an alias to the shared mock implementation
type MockResourceManager = runnertesting.MockResourceManager

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
// This is the primary helper function that automatically sets ExpandedCmd, ExpandedArgs,
// and EffectiveWorkDir from the spec values.
//
// The function also handles timeout resolution properly using the common timeout logic.
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
//
// Note: This is a local helper for the runner package. For other packages, use
// the exported helpers in internal/runner/executor/testing package.
func createRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
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

	return &runnertypes.RuntimeCommand{
		Spec:              spec,
		ExpandedCmd:       spec.Cmd,
		ExpandedArgs:      spec.Args,
		ExpandedEnv:       make(map[string]string),
		ExpandedVars:      make(map[string]string),
		EffectiveWorkDir:  spec.WorkDir,
		EffectiveTimeout:  effectiveTimeout,
		TimeoutResolution: resolutionContext,
	}
}
