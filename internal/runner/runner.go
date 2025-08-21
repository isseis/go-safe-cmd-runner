// Package runner provides the core functionality for running commands
// in a safe and controlled manner with group-based execution and dependency management.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/tempdir"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/joho/godotenv"
)

// Error definitions
var (
	ErrCommandFailed        = errors.New("command failed")
	ErrUnclosedVariableRef  = errors.New("unclosed variable reference")
	ErrUndefinedVariable    = errors.New("undefined variable")
	ErrCommandNotFound      = errors.New("command not found")
	ErrCircularReference    = errors.New("circular variable reference detected")
	ErrGroupVerification    = errors.New("group file verification failed")
	ErrGroupNotFound        = errors.New("group not found")
	ErrVariableAccessDenied = errors.New("variable access denied")
	ErrPrivilegedPathConfig = errors.New("privileged command path configuration error")
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

// Constants
const (
	maxResolutionDepth = 100 // Maximum number of variable resolution iterations
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
	config              *runnertypes.Config
	envVars             map[string]string
	validator           *security.Validator
	tempDirManager      *tempdir.TempDirManager
	verificationManager *verification.Manager
	envFilter           *environment.Filter
	privilegeManager    runnertypes.PrivilegeManager // Optional privilege manager for privileged commands
	runID               string                       // Unique identifier for this execution run
	resourceManager     resource.ResourceManager     // Manages all side-effects (commands, filesystem, privileges, etc.)
}

// Option is a function type for configuring Runner instances
type Option func(*runnerOptions)

// runnerOptions holds all configuration options for creating a Runner
type runnerOptions struct {
	securityConfig      *security.Config
	tempDirManager      *tempdir.TempDirManager
	executor            executor.CommandExecutor
	verificationManager *verification.Manager
	privilegeManager    runnertypes.PrivilegeManager
	auditLogger         *audit.Logger
	runID               string
	resourceManager     resource.ResourceManager
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

// WithTempDirManager sets a custom temporary directory manager
func WithTempDirManager(manager *tempdir.TempDirManager) Option {
	return func(opts *runnerOptions) {
		opts.tempDirManager = manager
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
	if opts.privilegeManager == nil && hasPrivilegedCommands(config) {
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

	if opts.tempDirManager == nil {
		opts.tempDirManager = tempdir.NewTempDirManager(config.Global.WorkDir)
	}

	// Create environment filter
	envFilter := environment.NewFilter(config)

	// Create default ResourceManager if not provided
	if opts.resourceManager == nil {
		// Create a simple FileSystem implementation
		fs := &simpleFileSystem{}
		opts.resourceManager = resource.NewDefaultResourceManager(
			opts.executor,
			fs,
			opts.privilegeManager,
			resource.ExecutionModeNormal,
			nil, // DryRunOptions will be set later if needed
		)
	}

	return &Runner{
		executor:            opts.executor,
		config:              config,
		envVars:             make(map[string]string),
		validator:           validator,
		tempDirManager:      opts.tempDirManager,
		verificationManager: opts.verificationManager,
		envFilter:           envFilter,
		privilegeManager:    opts.privilegeManager,
		runID:               opts.runID,
		resourceManager:     opts.resourceManager,
	}, nil
}

// LoadEnvironment loads environment variables from the specified .env file and system environment.
// If envFile is empty, only system environment variables will be loaded.
// If loadSystemEnv is true, system environment variables will be loaded first,
// then overridden by the .env file if specified.
// Variables undergo global filtering and validation during loading, and will be filtered
// per-group during execution.
func (r *Runner) LoadEnvironment(envFile string, loadSystemEnv bool) error {
	// Create environment map
	envMap := make(map[string]string)

	// Load system environment variables if requested
	if loadSystemEnv {
		sysEnv, err := r.envFilter.FilterSystemEnvironment()
		if err != nil {
			return fmt.Errorf("failed to filter system environment variables: %w", err)
		}
		maps.Copy(envMap, sysEnv)
	}

	// Load .env file if specified
	if envFile != "" {
		// Use SafeReadFile for secure file reading (includes path validation and permission checks)
		content, err := safefileio.SafeReadFile(envFile)
		if err != nil {
			return fmt.Errorf("failed to read environment file %s securely: %w", envFile, err)
		}

		// Parse content using godotenv.Parse
		fileEnv, err := godotenv.Parse(bytes.NewReader(content))
		if err != nil {
			return fmt.Errorf("failed to parse environment file %s: %w", envFile, err)
		}
		fileEnv, err = r.envFilter.FilterGlobalVariables(fileEnv, environment.SourceEnvFile)
		if err != nil {
			return fmt.Errorf("failed to filter environment variables from file %s: %w", envFile, err)
		}
		maps.Copy(envMap, fileEnv)
	}

	r.envVars = envMap
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
func (r *Runner) ExecuteGroup(ctx context.Context, group runnertypes.CommandGroup) error {
	// Record execution start time for notification
	startTime := time.Now()

	fmt.Printf("Executing group: %s\n", group.Name)
	if group.Description != "" {
		fmt.Printf("Description: %s\n", group.Description)
	}

	// Track temporary directories for cleanup
	groupTempDirs := make([]string, 0)
	defer func() {
		// Clean up temp directories created for this group using ResourceManager
		for _, tempDirPath := range groupTempDirs {
			if err := r.resourceManager.CleanupTempDir(tempDirPath); err != nil {
				slog.Warn("Failed to cleanup temp directory", "path", tempDirPath, "error", err)
			}
		}
	}()

	// Defer notification to ensure it's sent regardless of success or failure
	var executionResult *groupExecutionResult
	defer func() {
		if executionResult != nil {
			r.sendGroupNotification(group, executionResult, time.Since(startTime))
		}
	}()

	// Process the group without template
	processedGroup := group

	// Process new fields (TempDir, Cleanup, WorkDir)
	var tempDirPath string
	if processedGroup.TempDir {
		// Create temporary directory for this group using ResourceManager
		var err error
		tempDirPath, err = r.resourceManager.CreateTempDir(processedGroup.Name)
		if err != nil {
			return fmt.Errorf("failed to create temp directory for group %s: %w", processedGroup.Name, err)
		}
		groupTempDirs = append(groupTempDirs, tempDirPath)
	}

	// Determine and set the effective working directory for each command
	for i := range processedGroup.Commands {
		// Skip if command already has a directory specified
		if processedGroup.Commands[i].Dir != "" {
			continue
		}

		// Priority for working directory:
		// 1. TempDir (if enabled)
		// 2. Group's WorkDir
		switch {
		case tempDirPath != "":
			processedGroup.Commands[i].Dir = tempDirPath
		case processedGroup.WorkDir != "":
			processedGroup.Commands[i].Dir = processedGroup.WorkDir
		}
	}

	// Verify group files before execution
	if r.verificationManager != nil {
		result, err := r.verificationManager.VerifyGroupFiles(&processedGroup)
		if err != nil {
			return &VerificationError{
				GroupName:     processedGroup.Name,
				TotalFiles:    result.TotalFiles,
				VerifiedFiles: result.VerifiedFiles,
				FailedFiles:   len(result.FailedFiles),
				SkippedFiles:  len(result.SkippedFiles),
				Err:           err,
			}
		}

		if result.TotalFiles > 0 {
			slog.Info("Group file verification completed",
				"group", processedGroup.Name,
				"verified_files", result.VerifiedFiles,
				"skipped_files", len(result.SkippedFiles),
				"duration_ms", result.Duration.Milliseconds())
		}
	}

	// Execute commands in the group sequentially
	var lastCommand string
	var lastOutput string
	for i, cmd := range processedGroup.Commands {
		fmt.Printf("  [%d/%d] Executing command: %s\n", i+1, len(processedGroup.Commands), cmd.Name)

		// Process the command
		processedCmd := cmd
		lastCommand = processedCmd.Name

		// Create command context with timeout
		cmdCtx, cancel := r.createCommandContext(ctx, processedCmd)
		defer cancel()

		// Execute the command with group context
		result, err := r.executeCommandInGroup(cmdCtx, processedCmd, &processedGroup)
		if err != nil {
			fmt.Printf("    Command failed: %v\n", err)
			// Set failure result for notification
			executionResult = &groupExecutionResult{
				status:      GroupExecutionStatusError,
				exitCode:    1,
				lastCommand: lastCommand,
				output:      lastOutput,
				errorMsg:    err.Error(),
			}
			return fmt.Errorf("command %s failed: %w", processedCmd.Name, err)
		}

		// Display result
		fmt.Printf("    Exit code: %d\n", result.ExitCode)
		if result.Stdout != "" {
			fmt.Printf("    Stdout: %s\n", result.Stdout)
			lastOutput = result.Stdout
		}
		if result.Stderr != "" {
			fmt.Printf("    Stderr: %s\n", result.Stderr)
		}

		// Check if command succeeded
		if result.ExitCode != 0 {
			// Set failure result for notification
			executionResult = &groupExecutionResult{
				status:      GroupExecutionStatusError,
				exitCode:    result.ExitCode,
				lastCommand: lastCommand,
				output:      lastOutput,
				errorMsg:    fmt.Sprintf("command failed with exit code %d", result.ExitCode),
			}
			return fmt.Errorf("%w: command %s failed with exit code %d", ErrCommandFailed, processedCmd.Name, result.ExitCode)
		}
	}

	// Set success result for notification
	executionResult = &groupExecutionResult{
		status:      GroupExecutionStatusSuccess,
		exitCode:    0,
		lastCommand: lastCommand,
		output:      lastOutput,
		errorMsg:    "",
	}

	fmt.Printf("Group %s completed successfully\n", processedGroup.Name)
	return nil
}

// executeCommandInGroup executes a command within a specific group context
func (r *Runner) executeCommandInGroup(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup) (*executor.Result, error) {
	// Resolve environment variables for the command with group context
	envVars, err := r.resolveEnvironmentVars(cmd, group)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve environment variables: %w", err)
	}

	// Validate resolved environment variables
	if err := r.validator.ValidateAllEnvironmentVars(envVars); err != nil {
		return nil, fmt.Errorf("resolved environment variables security validation failed: %w", err)
	}

	// Validate privileged commands before any path resolution
	if cmd.Privileged {
		if !filepath.IsAbs(cmd.Cmd) {
			return nil, fmt.Errorf("%w: privileged commands must use absolute paths in configuration: %s", ErrPrivilegedPathConfig, cmd.Cmd)
		}
	}

	// Resolve and validate command path if verification manager is available
	if r.verificationManager != nil {
		resolvedPath, err := r.verificationManager.ResolvePath(cmd.Cmd)
		if err != nil {
			return nil, fmt.Errorf("command path resolution failed: %w", err)
		}

		// Only update the command path for non-privileged commands
		// Privileged commands must already be specified with absolute paths
		if !cmd.Privileged {
			cmd.Cmd = resolvedPath
		}
	}

	// Set working directory from global config if not specified
	if cmd.Dir == "" {
		cmd.Dir = r.config.Global.WorkDir
	}

	// Execute the command using ResourceManager
	result, err := r.resourceManager.ExecuteCommand(ctx, cmd, group, envVars)
	if err != nil {
		return nil, err
	}

	// Convert ResourceManager result to executor.Result
	return &executor.Result{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

// resolveEnvironmentVars resolves environment variables for a command with group context
func (r *Runner) resolveEnvironmentVars(cmd runnertypes.Command, group *runnertypes.CommandGroup) (map[string]string, error) {
	// Step 1: Resolve system and .env file variables with allowlist filtering
	systemEnvVars, err := r.envFilter.ResolveGroupEnvironmentVars(group, r.envVars)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve group environment variables: %w", err)
	}

	slog.Debug("Resolved system environment variables",
		"group", group.Name,
		"system_vars_count", len(systemEnvVars))

	// Step 2: Process Command.Env variables without allowlist checks
	processor := environment.NewCommandEnvProcessor(r.envFilter)
	finalEnvVars, err := processor.ProcessCommandEnvironment(cmd, systemEnvVars, group)
	if err != nil {
		return nil, fmt.Errorf("failed to process command environment variables: %w", err)
	}

	slog.Debug("Processed command environment variables",
		"command", cmd.Name,
		"group", group.Name,
		"final_vars_count", len(finalEnvVars))

	return finalEnvVars, nil
}

// resolveVariableReferences resolves ${VAR} references in a string
func (r *Runner) resolveVariableReferences(value string, envVars map[string]string, group *runnertypes.CommandGroup) (string, error) {
	return r.resolveVariableReferencesWithDepth(value, envVars, make(map[string]bool), 0, group)
}

// resolveVariableReferencesWithDepth resolves ${VAR} references with circular dependency detection
func (r *Runner) resolveVariableReferencesWithDepth(value string, envVars map[string]string, resolving map[string]bool, depth int, group *runnertypes.CommandGroup) (string, error) {
	// Prevent infinite recursion by limiting the depth
	if depth > maxResolutionDepth {
		return "", fmt.Errorf("%w: maximum resolution depth exceeded (%d)", ErrCircularReference, maxResolutionDepth)
	}

	result := value
	iterations := 0

	// Simple variable resolution - replace ${VAR} with value
	for strings.Contains(result, "${") {
		iterations++
		if iterations > maxResolutionDepth {
			return "", fmt.Errorf("%w: too many resolution iterations", ErrCircularReference)
		}

		start := strings.Index(result, "${")
		if start == -1 {
			break
		}

		end := strings.Index(result[start:], "}")
		if end == -1 {
			return "", fmt.Errorf("%w in: %s", ErrUnclosedVariableRef, value)
		}
		end += start

		varName := result[start+2 : end]

		// Check for circular reference first - this takes precedence over undefined variable errors
		if resolving[varName] {
			return "", fmt.Errorf("%w: variable %s references itself", ErrCircularReference, varName)
		}

		// Mark this variable as being resolved to detect cycles early
		resolving[varName] = true

		// Check if variable access is allowed for this group
		if !r.envFilter.IsVariableAccessAllowed(varName, group) {
			delete(resolving, varName) // Clean up before returning error
			return "", fmt.Errorf("%w: %s (group: %s)", ErrVariableAccessDenied, varName, group.Name)
		}

		varValue, exists := envVars[varName]
		if !exists {
			delete(resolving, varName) // Clean up before returning error
			return "", fmt.Errorf("%w: %s", ErrUndefinedVariable, varName)
		}

		// Recursively resolve the variable value
		resolvedValue, err := r.resolveVariableReferencesWithDepth(varValue, envVars, resolving, depth+1, group)
		if err != nil {
			return "", err
		}

		// Unmark the variable after resolution
		delete(resolving, varName)

		result = result[:start] + resolvedValue + result[end+1:]
	}

	return result, nil
}

// createCommandContext creates a context with timeout for command execution
func (r *Runner) createCommandContext(ctx context.Context, cmd runnertypes.Command) (context.Context, context.CancelFunc) {
	// Use command-specific timeout if specified, otherwise use global timeout
	timeout := time.Duration(r.config.Global.Timeout) * time.Second
	if cmd.Timeout > 0 {
		timeout = time.Duration(cmd.Timeout) * time.Second
	}

	return context.WithTimeout(ctx, timeout)
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

// PerformDryRun performs a dry-run analysis using the same execution path as normal execution
func (r *Runner) PerformDryRun(ctx context.Context, opts *resource.DryRunOptions) (*resource.DryRunResult, error) {
	// Create a new ResourceManager in dry-run mode
	fs := &simpleFileSystem{}
	dryRunManager := resource.NewDryRunResourceManager(
		r.executor,
		fs,
		r.privilegeManager,
		opts,
	)

	// Temporarily replace the resourceManager with the dry-run manager
	originalManager := r.resourceManager
	r.resourceManager = dryRunManager
	defer func() {
		r.resourceManager = originalManager
	}()

	// Execute the same path as normal execution
	err := r.ExecuteAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("dry-run analysis failed: %w", err)
	}

	// Get the analysis results
	return dryRunManager.GetDryRunResults(), nil
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

// hasPrivilegedCommands checks if the configuration contains any privileged commands
func hasPrivilegedCommands(config *runnertypes.Config) bool {
	for _, group := range config.Groups {
		for _, cmd := range group.Commands {
			if cmd.Privileged {
				return true
			}
		}
	}
	return false
}

// simpleFileSystem implements executor.FileSystem using standard os operations
type simpleFileSystem struct{}

func (fs *simpleFileSystem) CreateTempDir(dir, prefix string) (string, error) {
	return os.MkdirTemp(dir, prefix)
}

func (fs *simpleFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (fs *simpleFileSystem) FileExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
