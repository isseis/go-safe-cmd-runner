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

// VerificationError contains detailed information about verification failures
type VerificationError struct {
	GroupName     string
	TotalFiles    int
	VerifiedFiles int
	FailedFiles   int
	SkippedFiles  int
	Err           error
}

func (e *VerificationError) Error() string {
	return fmt.Sprintf("group file verification failed for group %s: %v", e.GroupName, e.Err)
}

func (e *VerificationError) Unwrap() error {
	return e.Err
}

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
	config              *runnertypes.Config
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

// NewRunner creates a new command runner with the given configuration and optional customizations
func NewRunner(config *runnertypes.Config, options ...Option) (*Runner, error) {
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

	// Create default privilege manager and audit logger if not provided but needed
	if opts.privilegeManager == nil && hasUserGroupCommands(config) {
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

	// Create environment filter
	envFilter := environment.NewFilter(config.Global.EnvAllowlist)

	// Create default ResourceManager if not provided
	if opts.resourceManager == nil {
		// Check if dry-run mode is requested
		if opts.dryRun {
			// Ensure dryRunOptions has default values if nil
			if opts.dryRunOptions == nil {
				opts.dryRunOptions = &resource.DryRunOptions{
					DetailLevel:  resource.DetailLevelDetailed,
					OutputFormat: resource.OutputFormatText,
				}
			}
			// For dry-run mode, create a simple path resolver using verification manager
			var pathResolver resource.PathResolver
			if opts.verificationManager != nil {
				pathResolver = opts.verificationManager
			} else {
				// Create a default PathResolver when verification manager is not provided
				pathResolver = verification.NewPathResolver("", validator, false)
			}
			resourceManager, err := resource.NewDryRunResourceManager(
				opts.executor,
				opts.privilegeManager,
				pathResolver,
				opts.dryRunOptions,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create dry-run resource manager: %w", err)
			}
			opts.resourceManager = resourceManager
		} else {
			// Use common.DefaultFileSystem for normal mode
			fs := common.NewDefaultFileSystem()
			// For normal mode, create a simple path resolver using verification manager
			var pathResolver resource.PathResolver
			if opts.verificationManager != nil {
				pathResolver = opts.verificationManager
			} else {
				// Create a default PathResolver when verification manager is not provided
				pathResolver = verification.NewPathResolver("", validator, false)
			}
			// Get max output size from config (use default if not specified)
			maxOutputSize := config.Global.MaxOutputSize
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
				return nil, fmt.Errorf("failed to create default resource manager: %w", err)
			}
			opts.resourceManager = resourceManager
		}
	}

	runner := &Runner{
		executor:            opts.executor,
		config:              config,
		envVars:             make(map[string]string),
		validator:           validator,
		verificationManager: opts.verificationManager,
		envFilter:           envFilter,
		privilegeManager:    opts.privilegeManager,
		runID:               opts.runID,
		resourceManager:     opts.resourceManager,
	}

	// Create GroupExecutor with notification function bound to runner
	runner.groupExecutor = NewDefaultGroupExecutor(
		opts.executor,
		config,
		validator,
		opts.verificationManager,
		opts.resourceManager,
		opts.runID,
		runner.sendGroupNotification,
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
	groups := make([]runnertypes.CommandGroup, len(r.config.Groups))
	copy(groups, r.config.Groups)
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
			var verErr *VerificationError
			if errors.As(err, &verErr) {
				slog.Warn("Group file verification failed, skipping group",
					"group", verErr.GroupName,
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
func (r *Runner) ExecuteGroup(ctx context.Context, group runnertypes.CommandGroup) error {
	return r.groupExecutor.ExecuteGroup(ctx, group)
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
func (r *Runner) GetConfig() *runnertypes.Config {
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

// sendGroupNotification sends a Slack notification for group execution completion
func (r *Runner) sendGroupNotification(group runnertypes.CommandGroup, result *groupExecutionResult, duration time.Duration) {
	slog.Info(
		"Command group execution completed",
		"group", group.Name,
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
func hasUserGroupCommands(config *runnertypes.Config) bool {
	for _, group := range config.Groups {
		for _, cmd := range group.Commands {
			if cmd.HasUserGroupSpecification() {
				return true
			}
		}
	}
	return false
}
