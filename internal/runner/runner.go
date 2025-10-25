// Package runner provides the core functionality for running commands
// in a safe and controlled manner with group-based execution and dependency management.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// Error definitions
var (
	ErrCommandFailed        = errors.New("command failed")
	ErrCommandNotFound      = errors.New("command not found")
	ErrGroupVerification    = errors.New("group file verification failed")
	ErrGroupNotFound        = errors.New("group not found")
	ErrVariableAccessDenied = errors.New("variable access denied")
	ErrRunIDRequired        = errors.New("runID is required")
)

// GroupExecutionStatus represents the execution status of a command group
type GroupExecutionStatus string

const (
	// GroupExecutionStatusSuccess indicates that the group execution was successful.
	GroupExecutionStatusSuccess GroupExecutionStatus = "success"

	// GroupExecutionStatusError indicates that the group execution encountered an error.
	GroupExecutionStatusError GroupExecutionStatus = "error"
)

// groupExecutionResult holds the result of group execution for notification
type groupExecutionResult struct {
	status      GroupExecutionStatus
	exitCode    int
	lastCommand string
	output      string
	errorMsg    string
}

// Runner manages the execution of command groups
type Runner struct {
	executor            executor.CommandExecutor
	config              *runnertypes.ConfigSpec    // TOML configuration
	runtimeGlobal       *runnertypes.RuntimeGlobal // Expanded global configuration
	envVars             map[string]string
	validator           *security.Validator
	verificationManager *verification.Manager
	envFilter           *environment.Filter
	privilegeManager    runnertypes.PrivilegeManager // Optional privilege manager for privileged commands
	runID               string                       // Unique identifier for this execution run
	resourceManager     resource.ResourceManager     // Manages all side-effects (commands, filesystem, privileges, etc.)
	groupExecutor       GroupExecutor                // Executes command groups
}

// Option is a function type for configuring Runner instances
type Option func(*runnerOptions)

// runnerOptions holds all configuration options for creating a Runner
type runnerOptions struct {
	securityConfig      *security.Config
	executor            executor.CommandExecutor
	verificationManager *verification.Manager
	privilegeManager    runnertypes.PrivilegeManager
	auditLogger         *audit.Logger
	runID               string
	resourceManager     resource.ResourceManager
	dryRun              bool
	dryRunOptions       *resource.DryRunOptions
	runtimeGlobal       *runnertypes.RuntimeGlobal
	keepTempDirs        bool
}

// WithSecurity sets a custom security configuration
func WithSecurity(securityConfig *security.Config) Option {
	return func(opts *runnerOptions) {
		opts.securityConfig = securityConfig
	}
}

// WithVerificationManager sets a custom verification manager
func WithVerificationManager(verificationManager *verification.Manager) Option {
	return func(opts *runnerOptions) {
		opts.verificationManager = verificationManager
	}
}

// WithExecutor sets a custom command executor
func WithExecutor(exec executor.CommandExecutor) Option {
	return func(opts *runnerOptions) {
		opts.executor = exec
	}
}

// WithPrivilegeManager sets a custom privilege manager
func WithPrivilegeManager(privMgr runnertypes.PrivilegeManager) Option {
	return func(opts *runnerOptions) {
		opts.privilegeManager = privMgr
	}
}

// WithAuditLogger sets a custom audit logger
func WithAuditLogger(auditLogger *audit.Logger) Option {
	return func(opts *runnerOptions) {
		opts.auditLogger = auditLogger
	}
}

// WithRunID sets a custom run ID for tracking execution
func WithRunID(runID string) Option {
	return func(opts *runnerOptions) {
		opts.runID = runID
	}
}

// WithResourceManager sets a custom resource manager
func WithResourceManager(resourceManager resource.ResourceManager) Option {
	return func(opts *runnerOptions) {
		opts.resourceManager = resourceManager
	}
}

// WithDryRun sets dry-run mode with optional configuration
func WithDryRun(dryRunOptions *resource.DryRunOptions) Option {
	return func(opts *runnerOptions) {
		opts.dryRun = true
		opts.dryRunOptions = dryRunOptions
	}
}

// WithKeepTempDirs sets the flag to keep temporary directories after execution
func WithKeepTempDirs(keep bool) Option {
	return func(opts *runnerOptions) {
		opts.keepTempDirs = keep
	}
}

// WithRuntimeGlobal sets a pre-expanded runtime global configuration
func WithRuntimeGlobal(runtimeGlobal *runnertypes.RuntimeGlobal) Option {
	return func(opts *runnerOptions) {
		opts.runtimeGlobal = runtimeGlobal
	}
}

// initializeRuntimeGlobal initializes the runtime global configuration
func initializeRuntimeGlobal(opts *runnerOptions, configSpec *runnertypes.ConfigSpec) (*runnertypes.RuntimeGlobal, error) {
	if opts.runtimeGlobal != nil {
		return opts.runtimeGlobal, nil
	}
	return config.ExpandGlobal(&configSpec.Global)
}

// initializeDefaultComponents initializes default privilege manager, audit logger, and executor if not provided
func initializeDefaultComponents(opts *runnerOptions, configSpec *runnertypes.ConfigSpec) {
	// Create default privilege manager and audit logger if not provided but needed
	if opts.privilegeManager == nil && hasUserGroupCommands(configSpec) {
		opts.privilegeManager = privilege.NewManager(slog.Default())
	}

	if opts.auditLogger == nil && opts.privilegeManager != nil {
		opts.auditLogger = audit.NewAuditLogger()
	}

	// Use provided components or create defaults
	if opts.executor == nil {
		executorOpts := []executor.Option{}
		if opts.privilegeManager != nil {
			executorOpts = append(executorOpts, executor.WithPrivilegeManager(opts.privilegeManager))
		}
		if opts.auditLogger != nil {
			executorOpts = append(executorOpts, executor.WithAuditLogger(opts.auditLogger))
		}
		opts.executor = executor.NewDefaultExecutor(executorOpts...)
	}
}

// createResourceManager creates a resource manager for dry-run or normal mode
func createResourceManager(opts *runnerOptions, configSpec *runnertypes.ConfigSpec, validator *security.Validator) error {
	if opts.resourceManager != nil {
		return nil
	}

	// Helper function to get path resolver
	getPathResolver := func() resource.PathResolver {
		if opts.verificationManager != nil {
			return opts.verificationManager
		}
		return verification.NewPathResolver("", validator, false)
	}

	if opts.dryRun {
		return createDryRunResourceManager(opts, getPathResolver())
	}
	return createNormalResourceManager(opts, configSpec, getPathResolver())
}

// createDryRunResourceManager creates a resource manager for dry-run mode
func createDryRunResourceManager(opts *runnerOptions, pathResolver resource.PathResolver) error {
	if opts.dryRunOptions == nil {
		opts.dryRunOptions = &resource.DryRunOptions{
			DetailLevel:  resource.DetailLevelDetailed,
			OutputFormat: resource.OutputFormatText,
		}
	}

	resourceManager, err := resource.NewDryRunResourceManager(
		opts.executor,
		opts.privilegeManager,
		pathResolver,
		opts.dryRunOptions,
	)
	if err != nil {
		return fmt.Errorf("failed to create dry-run resource manager: %w", err)
	}
	opts.resourceManager = resourceManager
	return nil
}

// createNormalResourceManager creates a resource manager for normal mode
func createNormalResourceManager(opts *runnerOptions, configSpec *runnertypes.ConfigSpec, pathResolver resource.PathResolver) error {
	fs := common.NewDefaultFileSystem()
	maxOutputSize := configSpec.Global.OutputSizeLimit
	if maxOutputSize <= 0 {
		maxOutputSize = 0 // Will use default from output package
	}

	resourceManager, err := resource.NewDefaultResourceManagerWithOutput(
		opts.executor,
		fs,
		opts.privilegeManager,
		pathResolver,
		slog.Default(),
		resource.ExecutionModeNormal,
		&resource.DryRunOptions{}, // Empty dry-run options for normal mode
		nil,                       // Use default output manager
		maxOutputSize,             // Max output size from config
	)
	if err != nil {
		return fmt.Errorf("failed to create default resource manager: %w", err)
	}
	opts.resourceManager = resourceManager
	return nil
}

// NewRunner creates a new command runner with the given configuration and optional customizations
func NewRunner(configSpec *runnertypes.ConfigSpec, options ...Option) (*Runner, error) {
	// Apply default options
	opts := &runnerOptions{}
	for _, option := range options {
		option(opts)
	}

	// Validate that runID is provided
	if opts.runID == "" {
		return nil, ErrRunIDRequired
	}

	// Create validator with provided or default security config
	validator, err := security.NewValidator(opts.securityConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create security validator: %w", err)
	}

	// Create environment filter
	envFilter := environment.NewFilter(configSpec.Global.EnvAllowed)

	// Initialize runtime global configuration
	runtimeGlobal, err := initializeRuntimeGlobal(opts, configSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to expand global configuration: %w", err)
	}

	// Initialize default components
	initializeDefaultComponents(opts, configSpec)

	// Create resource manager if not provided
	if err := createResourceManager(opts, configSpec, validator); err != nil {
		return nil, err
	}

	runner := &Runner{
		executor:            opts.executor,
		config:              configSpec,
		runtimeGlobal:       runtimeGlobal,
		envVars:             make(map[string]string),
		validator:           validator,
		verificationManager: opts.verificationManager,
		envFilter:           envFilter,
		privilegeManager:    opts.privilegeManager,
		runID:               opts.runID,
		resourceManager:     opts.resourceManager,
	}

	// Create GroupExecutor with a logging callback bound to runner.
	// Note: this callback emits a structured slog record intended for
	// consumption by notification handlers (for example `SlackHandler`).
	// The callback itself does not perform network calls.
	var detailLevel resource.DetailLevel
	if opts.dryRunOptions != nil {
		detailLevel = opts.dryRunOptions.DetailLevel
	}
	runner.groupExecutor = NewDefaultGroupExecutor(
		opts.executor,
		configSpec,
		validator,
		opts.verificationManager,
		opts.resourceManager,
		opts.runID,
		runner.logGroupExecutionSummary,
		opts.dryRun,
		detailLevel,
		opts.keepTempDirs,
	)

	return runner, nil
}

// LoadSystemEnvironment loads and filters system environment variables.
// This is a convenience method that wraps the filtering and setting operations.
func (r *Runner) LoadSystemEnvironment() error {
	sysEnv, err := r.envFilter.FilterSystemEnvironment()
	if err != nil {
		return fmt.Errorf("failed to filter system environment variables: %w", err)
	}
	r.envVars = sysEnv
	return nil
}

// ExecuteAll executes all command groups in the configured order
func (r *Runner) ExecuteAll(ctx context.Context) error {
	// Sort groups by priority (lower number = higher priority)
	groups := make([]*runnertypes.GroupSpec, len(r.config.Groups))
	for i := range r.config.Groups {
		groups[i] = &r.config.Groups[i]
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Priority < groups[j].Priority
	})

	var groupErrs []error

	// Execute all groups sequentially, collecting errors
	for _, group := range groups {
		// Check if context is already cancelled before executing next group
		select {
		case <-ctx.Done():
			// Context cancelled, don't execute remaining groups
			// Always prioritize cancellation error over previous errors
			return ctx.Err()
		default:
		}

		if err := r.ExecuteGroup(ctx, group); err != nil {
			// Check if this is a context cancellation error - if so, stop execution
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}

			// Check if this is a verification error - if so, log warning and continue
			var verErr *verification.VerificationError
			if errors.As(err, &verErr) {
				slog.Warn("Group file verification failed, skipping group",
					"group", verErr.Group,
					"total_files", verErr.TotalFiles,
					"verified_files", verErr.VerifiedFiles,
					"failed_files", verErr.FailedFiles,
					"skipped_files", verErr.SkippedFiles,
					"error", verErr.Err.Error())
				continue // Skip this group but continue with the next one
			}
			// Collect error but continue with next group
			groupErrs = append(groupErrs, fmt.Errorf("failed to execute group %s: %w", group.Name, err))
		}
	}

	// Return the first error if any occurred
	if len(groupErrs) > 0 {
		return groupErrs[0]
	}

	return nil
}

// ExecuteGroup executes all commands in a group sequentially
// This method delegates to the GroupExecutor implementation
func (r *Runner) ExecuteGroup(ctx context.Context, groupSpec *runnertypes.GroupSpec) error {
	return r.groupExecutor.ExecuteGroup(ctx, groupSpec, r.runtimeGlobal)
}

// ListCommands lists all available commands
func (r *Runner) ListCommands() {
	fmt.Println("Available commands:")
	for _, group := range r.config.Groups {
		fmt.Printf("  Group: %s (Priority: %d)\n", group.Name, group.Priority)
		if group.Description != "" {
			fmt.Printf("    Description: %s\n", group.Description)
		}
		for _, cmd := range group.Commands {
			fmt.Printf("    - %s: %s\n", cmd.Name, cmd.Description)
		}
	}
}

// GetConfig returns the current configuration
func (r *Runner) GetConfig() *runnertypes.ConfigSpec {
	return r.config
}

// CleanupAllResources cleans up all managed resources
func (r *Runner) CleanupAllResources() error {
	return r.resourceManager.CleanupAllTempDirs()
}

// GetDryRunResults returns dry-run analysis results if available
func (r *Runner) GetDryRunResults() *resource.DryRunResult {
	return r.resourceManager.GetDryRunResults()
}

// logGroupExecutionSummary emits a structured log record summarizing the
// execution of a command group. This record includes attributes (such as
// "slack_notify" and "message_type") that notification handlers (for
// example `internal/logging.SlackHandler`) can use to send alerts. The
// function itself only logs; it does not perform network I/O.
func (r *Runner) logGroupExecutionSummary(groupSpec *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration) {
	slog.Info(
		"Command group execution completed",
		"group", groupSpec.Name,
		"command", result.lastCommand,
		"status", result.status,
		"exit_code", result.exitCode,
		"duration_ms", duration.Milliseconds(),
		"output", result.output,
		"run_id", r.runID,
		"slack_notify", true,
		"message_type", "command_group_summary",
	)
}

// hasUserGroupCommands checks if the configuration contains any commands with user/group specifications
func hasUserGroupCommands(configSpec *runnertypes.ConfigSpec) bool {
	for _, group := range configSpec.Groups {
		for _, cmd := range group.Commands {
			if cmd.HasUserGroupSpecification() {
				return true
			}
		}
	}
	return false
}
