//go:build test

package runner

import (
	"testing"

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

// setupDefaultMockBehavior sets up common default mock expectations for basic test scenarios
//
//nolint:unused // Will be used in subsequent test migrations
func setupDefaultMockBehavior(m *MockResourceManager) {
	// Default ValidateOutputPath behavior - allows any output path
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Maybe()

	// Default ExecuteCommand behavior - returns successful execution
	defaultResult := &resource.ExecutionResult{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
	}
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(defaultResult, nil).Maybe()
}

// setupSuccessfulMockExecution sets up mock for successful command execution with custom output
//
//nolint:unused // Will be used in subsequent test migrations
func setupSuccessfulMockExecution(m *MockResourceManager, stdout, stderr string) {
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	result := &resource.ExecutionResult{
		ExitCode: 0,
		Stdout:   stdout,
		Stderr:   stderr,
	}
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(result, nil)
}

// setupFailedMockExecution sets up mock for failed command execution with custom error
//
//nolint:unused // Will be used in subsequent test migrations
func setupFailedMockExecution(m *MockResourceManager, err error) {
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, err)
}

// newMockResourceManagerWithDefaults creates a new MockResourceManager with default behavior setup
//
//nolint:unused // Will be used in subsequent test migrations
func newMockResourceManagerWithDefaults() *MockResourceManager {
	mockRM := &MockResourceManager{}
	setupDefaultMockBehavior(mockRM)
	return mockRM
}

// createConfigSpec creates a ConfigSpec from test parameters for easy migration
//
//nolint:unused // Will be used in subsequent test migrations
func createConfigSpec(timeout int, workDir string, groups []runnertypes.GroupSpec) *runnertypes.ConfigSpec {
	return &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: timeout,
			WorkDir: workDir,
		},
		Groups: groups,
	}
}

// createSimpleGroupSpec creates a simple GroupSpec for testing
//
//nolint:unused // Will be used in subsequent test migrations
func createSimpleGroupSpec(name string, commands []runnertypes.CommandSpec) runnertypes.GroupSpec {
	return runnertypes.GroupSpec{
		Name:     name,
		Commands: commands,
	}
}

// createSimpleCommandSpec creates a simple CommandSpec for testing
//
//nolint:unused // Will be used in subsequent test migrations
func createSimpleCommandSpec(name, cmd string, args []string) runnertypes.CommandSpec {
	return runnertypes.CommandSpec{
		Name: name,
		Cmd:  cmd,
		Args: args,
	}
}
