//go:build test

package testing

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/mock"
)

// SetupTestEnv sets up a clean test environment.
func SetupTestEnv(t *testing.T, envVars map[string]string) {
	t.Helper()

	// Set up the test environment variables
	for key, value := range envVars {
		t.Setenv(key, value)
	}
}

// SetupSafeTestEnv sets up a minimal safe environment for tests.
// This is useful for security-related tests where we want to ensure a clean, minimal environment.
func SetupSafeTestEnv(t *testing.T) {
	t.Helper()
	safeEnv := map[string]string{
		"PATH": "/usr/bin:/bin",
		"HOME": "/home/test",
		"USER": "test",
	}
	SetupTestEnv(t, safeEnv)
}

// SetupFailedMockExecution sets up mock for failed command execution with custom error
func SetupFailedMockExecution(m *MockResourceManager, err error) {
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, err)
}

// TestGroupExecutorConfig holds configuration for test group executor creation.
type TestGroupExecutorConfig struct {
	Executor            executor.CommandExecutor
	Config              *runnertypes.ConfigSpec
	Validator           security.ValidatorInterface
	VerificationManager verification.ManagerInterface
	ResourceManager     resource.ResourceManager
	RunID               string
}
