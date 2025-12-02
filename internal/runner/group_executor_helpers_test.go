package runner

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// NewTestGroupExecutor creates a DefaultGroupExecutor with common test defaults.
func NewTestGroupExecutor(
	config *runnertypes.ConfigSpec,
	resourceManager resource.ResourceManager,
	options ...GroupExecutorOption,
) *DefaultGroupExecutor {
	return NewDefaultGroupExecutor(
		nil, // executor
		config,
		nil, // validator
		nil, // verificationManager
		resourceManager,
		"test-run-123", // runID
		options...,
	)
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

// NewTestGroupExecutorWithConfig creates a DefaultGroupExecutor with custom configuration.
func NewTestGroupExecutorWithConfig(
	cfg TestGroupExecutorConfig,
	options ...GroupExecutorOption,
) *DefaultGroupExecutor {
	// Apply defaults for unset fields
	runID := cfg.RunID
	if runID == "" {
		runID = "test-run-123"
	}

	return NewDefaultGroupExecutor(
		cfg.Executor,
		cfg.Config,
		cfg.Validator,
		cfg.VerificationManager,
		cfg.ResourceManager,
		runID,
		options...,
	)
}
