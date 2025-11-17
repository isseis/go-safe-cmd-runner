// Package runner provides the core functionality for running commands
// in a safe and controlled manner with group-based execution and dependency management.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/user"
	"sort"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
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
	ErrDependencyNotFound   = errors.New("group dependency not found")
	ErrCircularDependency   = errors.New("circular group dependency detected")
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
	status   GroupExecutionStatus
	commands common.CommandResults // All commands executed in the group
	errorMsg string
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
	executor                executor.CommandExecutor
	verificationManager     *verification.Manager
	privilegeManager        runnertypes.PrivilegeManager
	auditLogger             *audit.Logger
	runID                   string
	resourceManager         resource.ResourceManager
	dryRun                  bool
	dryRunOptions           *resource.DryRunOptions
	runtimeGlobal           *runnertypes.RuntimeGlobal
	keepTempDirs            bool
	groupMembershipProvider *groupmembership.GroupMembership
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

// WithGroupMembershipProvider sets a custom group membership provider
func WithGroupMembershipProvider(provider *groupmembership.GroupMembership) Option {
	return func(opts *runnerOptions) {
		opts.groupMembershipProvider = provider
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
		return createDryRunResourceManager(opts, getPathResolver(), validator)
	}
	return createNormalResourceManager(opts, configSpec, getPathResolver(), validator)
}

// createDryRunResourceManager creates a resource manager for dry-run mode
func createDryRunResourceManager(opts *runnerOptions, pathResolver resource.PathResolver, validator *security.Validator) error {
	if opts.dryRunOptions == nil {
		opts.dryRunOptions = &resource.DryRunOptions{
			DetailLevel:  resource.DetailLevelDetailed,
			OutputFormat: resource.OutputFormatText,
		}
	}

	// Create output manager with the same validator that has group membership support
	outputMgr := output.NewDefaultOutputCaptureManager(validator)

	resourceManager, err := resource.NewDryRunResourceManagerWithOutput(
		opts.executor,
		opts.privilegeManager,
		pathResolver,
		outputMgr,
		opts.dryRunOptions,
	)
	if err != nil {
		return fmt.Errorf("failed to create dry-run resource manager: %w", err)
	}
	opts.resourceManager = resourceManager
	return nil
}

// createNormalResourceManager creates a resource manager for normal mode
func createNormalResourceManager(opts *runnerOptions, _ *runnertypes.ConfigSpec, pathResolver resource.PathResolver, validator *security.Validator) error {
	fs := common.NewDefaultFileSystem()
	// Note: maxOutputSize is no longer used here as output size limit is now resolved per-command
	// via RuntimeCommand.EffectiveOutputSizeLimit. Pass 0 to ResourceManager.
	maxOutputSize := int64(0)

	// Create output manager with the same validator that has group membership support
	outputMgr := output.NewDefaultOutputCaptureManager(validator)

	resourceManager, err := resource.NewDefaultResourceManager(
		opts.executor,
		fs,
		opts.privilegeManager,
		pathResolver,
		slog.Default(),
		resource.ExecutionModeNormal,
		&resource.DryRunOptions{}, // Empty dry-run options for normal mode
		outputMgr,                 // Pass output manager with validator
		maxOutputSize,             // Not used anymore (per-command limit is used instead)
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

	// Initialize group membership provider if not provided
	gmProvider := opts.groupMembershipProvider
	if gmProvider == nil {
		gmProvider = groupmembership.New()
	}

	// Create validator with default security config
	validator, err := security.NewValidator(nil, security.WithGroupMembership(gmProvider))
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
	var groupOptions []GroupExecutorOption
	groupOptions = append(groupOptions, WithGroupNotificationFunc(runner.logGroupExecutionSummary))

	if opts.dryRunOptions != nil {
		groupOptions = append(groupOptions, WithGroupDryRun(opts.dryRunOptions))
	}

	if opts.keepTempDirs {
		groupOptions = append(groupOptions, WithGroupKeepTempDirs(true))
	}

	// Get current user for security logging
	currentUser := getCurrentUser()
	groupOptions = append(groupOptions, WithCurrentUser(currentUser))

	runner.groupExecutor = NewDefaultGroupExecutor(
		opts.executor,
		configSpec,
		validator,
		opts.verificationManager,
		opts.resourceManager,
		opts.runID,
		groupOptions...,
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

// ExecuteFiltered executes only the specified groups (including their dependencies)
// groupNames が nil または空の場合は全グループを実行（ExecuteAll と同じ動作）
//
// Parameters:
//   - ctx: コンテキスト
//   - groupNames: 実行するグループ名のリスト（nil の場合は全グループ）
//
// Returns:
//   - error: 実行エラー
func (r *Runner) ExecuteFiltered(ctx context.Context, groupNames []string) error {
	// グループ名が指定されていない場合は全グループを実行
	if len(groupNames) == 0 {
		return r.ExecuteAll(ctx)
	}

	// 指定されたグループと依存関係のみを含む設定を作成
	filteredConfig, err := r.filterConfigGroups(groupNames)
	if err != nil {
		return err
	}

	// フィルタリングされた設定で実行
	// ExecuteAll のロジックを再利用（依存関係解決を含む）
	// 一時的にr.configを置き換えて実行
	originalConfig := r.config
	r.config = filteredConfig
	defer func() {
		r.config = originalConfig
	}()

	return r.ExecuteAll(ctx)
}

// filterConfigGroups は指定されたグループ名と、その依存関係を含む設定を生成する
// 内部使用のみ（非公開メソッド）
//
// Parameters:
//   - groupNames: フィルターするグループ名
//
// Returns:
//   - *runnertypes.ConfigSpec: フィルタリングされた設定
//   - error: 依存関係解決時のエラー
func (r *Runner) filterConfigGroups(groupNames []string) (*runnertypes.ConfigSpec, error) {
	if len(groupNames) == 0 {
		cloned := *r.config
		return &cloned, nil
	}

	selected, err := r.collectGroupsWithDependencies(groupNames)
	if err != nil {
		return nil, err
	}

	filteredConfig := *r.config
	filteredGroups := make([]runnertypes.GroupSpec, 0, len(selected))
	for _, group := range r.config.Groups {
		if _, ok := selected[group.Name]; ok {
			filteredGroups = append(filteredGroups, group)
		}
	}

	filteredConfig.Groups = filteredGroups
	return &filteredConfig, nil
}

// collectGroupsWithDependencies resolves the dependency closure for the requested group names.
// It returns a set of group names (including the original requests) or an error if a dependency
// is missing or a cycle is detected.
func (r *Runner) collectGroupsWithDependencies(groupNames []string) (map[string]struct{}, error) {
	groupIndex := make(map[string]*runnertypes.GroupSpec, len(r.config.Groups))
	for i := range r.config.Groups {
		group := &r.config.Groups[i]
		groupIndex[group.Name] = group
	}

	requested := make(map[string]struct{}, len(groupNames))
	for _, name := range groupNames {
		requested[name] = struct{}{}
	}

	included := make(map[string]struct{}, len(groupNames))
	for name := range requested {
		included[name] = struct{}{}
	}

	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("%w: detected cycle involving group %q", ErrCircularDependency, name)
		}

		group, ok := groupIndex[name]
		if !ok {
			return fmt.Errorf("%w: group %q not found in configuration", ErrGroupNotFound, name)
		}

		visiting[name] = true
		for _, dep := range group.DependsOn {
			if dep == "" {
				continue
			}

			if _, exists := groupIndex[dep]; !exists {
				return fmt.Errorf("%w: dependency %q required by %q not found in configuration", ErrDependencyNotFound, dep, group.Name)
			}

			if _, alreadyIncluded := included[dep]; !alreadyIncluded {
				included[dep] = struct{}{}
				if _, explicitlyRequested := requested[dep]; !explicitlyRequested {
					slog.Info("Adding dependent group to execution list",
						"group", dep,
						"required_by", group.Name,
						"run_id", r.runID,
					)
				}
			}

			if err := visit(dep); err != nil {
				return err
			}
		}

		visiting[name] = false
		visited[name] = true
		return nil
	}

	for _, name := range groupNames {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return included, nil
}

// ExecuteGroup executes all commands in a group sequentially
// This method delegates to the GroupExecutor implementation
func (r *Runner) ExecuteGroup(ctx context.Context, groupSpec *runnertypes.GroupSpec) error {
	return r.groupExecutor.ExecuteGroup(ctx, groupSpec, r.runtimeGlobal)
}

// CleanupAllResources cleans up all managed resources
func (r *Runner) CleanupAllResources() error {
	return r.resourceManager.CleanupAllTempDirs()
}

// GetDryRunResults returns dry-run analysis results if available
func (r *Runner) GetDryRunResults() *resource.DryRunResult {
	return r.resourceManager.GetDryRunResults()
}

// executionErrorSetter is an interface for setting execution errors in dry-run mode.
// This interface allows type-safe checking without depending on concrete types.
type executionErrorSetter interface {
	SetExecutionError(errType, message, component string, details map[string]any, phase resource.ExecutionPhase)
}

// SetDryRunExecutionError sets the execution error for dry-run mode.
// This method should be called when an error occurs during dry-run execution.
// This is a no-op in normal execution mode.
func (r *Runner) SetDryRunExecutionError(errType, message, component string, details map[string]any, phase resource.ExecutionPhase) {
	// Use interface-based type assertion instead of concrete type check
	// This allows any ResourceManager implementation to provide error setting capability
	if setter, ok := r.resourceManager.(executionErrorSetter); ok {
		setter.SetExecutionError(errType, message, component, details, phase)
	}
	// Silently ignore if the resource manager doesn't support error setting (normal mode)
}

// logGroupExecutionSummary emits a structured log record summarizing the
// execution of a command group. This record includes attributes (such as
// "slack_notify" and "message_type") that notification handlers (for
// example `internal/logging.SlackHandler`) can use to send alerts. The
// function itself only logs; it does not perform network I/O.
func (r *Runner) logGroupExecutionSummary(groupSpec *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration) {
	slog.Info(
		"Command group execution completed",
		common.GroupSummaryAttrs.Group, groupSpec.Name,
		common.GroupSummaryAttrs.Status, result.status,
		common.GroupSummaryAttrs.Commands, result.commands,
		common.GroupSummaryAttrs.DurationMs, duration.Milliseconds(),
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

// getCurrentUser returns the current system user name.
// This uses os/user.Current() which is more secure than environment variables
// that can be spoofed. Returns "unknown" if the user cannot be determined.
func getCurrentUser() string {
	currentUser, err := user.Current()
	if err != nil {
		slog.Warn("Failed to get current user", slog.Any("error", err))
		return "unknown"
	}
	return currentUser.Username
}
