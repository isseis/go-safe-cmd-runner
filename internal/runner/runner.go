// Package runner provides the core functionality for running commands
// in a safe and controlled manner with group-based execution and dependency management.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/template"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/joho/godotenv"
)

// Error definitions
var (
	ErrCommandFailed       = errors.New("command failed")
	ErrUnclosedVariableRef = errors.New("unclosed variable reference")
	ErrUndefinedVariable   = errors.New("undefined variable")
	ErrCommandNotFound     = errors.New("command not found")
	ErrCircularReference   = errors.New("circular variable reference detected")
	ErrGroupVerification   = errors.New("group file verification failed")
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
	envSeparatorParts  = 2
	maxResolutionDepth = 100 // Maximum number of variable resolution iterations
)

// Runner manages the execution of command groups
type Runner struct {
	executor            executor.CommandExecutor
	config              *runnertypes.Config
	envVars             map[string]string
	validator           *security.Validator
	templateEngine      *template.Engine
	resourceManager     *resource.Manager
	verificationManager *verification.Manager
}

// Option is a function type for configuring Runner instances
type Option func(*runnerOptions)

// runnerOptions holds all configuration options for creating a Runner
type runnerOptions struct {
	securityConfig      *security.Config
	templateEngine      *template.Engine
	resourceManager     *resource.Manager
	executor            executor.CommandExecutor
	verificationManager *verification.Manager
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

// WithTemplateEngine sets a custom template engine
func WithTemplateEngine(engine *template.Engine) Option {
	return func(opts *runnerOptions) {
		opts.templateEngine = engine
	}
}

// WithResourceManager sets a custom resource manager
func WithResourceManager(manager *resource.Manager) Option {
	return func(opts *runnerOptions) {
		opts.resourceManager = manager
	}
}

// WithExecutor sets a custom command executor
func WithExecutor(exec executor.CommandExecutor) Option {
	return func(opts *runnerOptions) {
		opts.executor = exec
	}
}

// NewRunner creates a new command runner with the given configuration and optional customizations
func NewRunner(config *runnertypes.Config, options ...Option) (*Runner, error) {
	// Apply default options
	opts := &runnerOptions{}
	for _, option := range options {
		option(opts)
	}

	// Create validator with provided or default security config
	validator, err := security.NewValidator(opts.securityConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create security validator: %w", err)
	}

	// Use provided components or create defaults
	if opts.executor == nil {
		opts.executor = executor.NewDefaultExecutor()
	}
	if opts.templateEngine == nil {
		opts.templateEngine = template.NewEngine()
	}
	if opts.resourceManager == nil {
		opts.resourceManager = resource.NewManager(config.Global.WorkDir)
	}

	return &Runner{
		executor:            opts.executor,
		config:              config,
		envVars:             make(map[string]string),
		validator:           validator,
		templateEngine:      opts.templateEngine,
		resourceManager:     opts.resourceManager,
		verificationManager: opts.verificationManager,
	}, nil
}

// LoadEnvironment loads environment variables from the specified .env file and system environment.
// If envFile is empty, only system environment variables will be loaded.
// If loadSystemEnv is true, system environment variables will be loaded first,
// then overridden by the .env file if specified.
func (r *Runner) LoadEnvironment(envFile string, loadSystemEnv bool) error {
	// Validate file permissions if a file is specified
	if envFile != "" {
		if err := r.validator.ValidateFilePermissions(envFile); err != nil {
			return fmt.Errorf("security validation failed for environment file: %w", err)
		}
	}

	envMap := make(map[string]string)

	// Load system environment variables if requested
	if loadSystemEnv {
		for _, env := range os.Environ() {
			if i := strings.Index(env, "="); i >= 0 {
				envMap[env[:i]] = env[i+1:]
			}
		}
	}

	// Load .env file if specified
	if envFile != "" {
		fileEnv, err := godotenv.Read(envFile)
		if err != nil {
			return fmt.Errorf("failed to load environment file %s: %w", envFile, err)
		}
		// Override with values from .env file
		for k, v := range fileEnv {
			envMap[k] = v
		}
	}

	// Validate all environment variables for safety
	if err := r.validator.ValidateAllEnvironmentVars(envMap); err != nil {
		return fmt.Errorf("environment variable security validation failed: %w", err)
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

	// Execute groups sequentially
	for _, group := range groups {
		if err := r.ExecuteGroup(ctx, group); err != nil {
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
			return fmt.Errorf("failed to execute group %s: %w", group.Name, err)
		}
	}

	return nil
}

// ExecuteGroup executes all commands in a group sequentially
func (r *Runner) ExecuteGroup(ctx context.Context, group runnertypes.CommandGroup) error {
	fmt.Printf("Executing group: %s\n", group.Name)
	if group.Description != "" {
		fmt.Printf("Description: %s\n", group.Description)
	}

	// Apply template to the group if specified
	processedGroup := group
	if group.Template != "" {
		appliedGroup, err := r.templateEngine.ApplyTemplate(&group, group.Template)
		if err != nil {
			return fmt.Errorf("failed to apply template %s to group %s: %w", group.Template, group.Name, err)
		}
		processedGroup = *appliedGroup
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

	// Track resources for cleanup
	groupResources := make([]string, 0)
	defer func() {
		// Clean up resources created for this group
		for _, resourceID := range groupResources {
			if err := r.resourceManager.CleanupResource(resourceID); err != nil {
				slog.Warn("Failed to cleanup resource", "resource_id", resourceID, "error", err)
			}
		}
	}()

	// Execute commands in the group sequentially
	for i, cmd := range processedGroup.Commands {
		fmt.Printf("  [%d/%d] Executing command: %s\n", i+1, len(processedGroup.Commands), cmd.Name)

		// Apply resource management to the command if needed
		processedCmd := cmd
		// Check if template specified temp_dir
		if group.Template != "" {
			tmpl, err := r.templateEngine.GetTemplate(group.Template)
			if err == nil && tmpl.TempDir {
				tempResource, err := r.resourceManager.CreateTempDir(cmd.Name, tmpl.Cleanup)
				if err != nil {
					return fmt.Errorf("failed to create temp directory for command %s: %w", cmd.Name, err)
				}
				groupResources = append(groupResources, tempResource.ID)

				// Set working directory to temp directory if not already set
				if processedCmd.Dir == "" || processedCmd.Dir == "{{.temp_dir}}" {
					processedCmd.Dir = tempResource.Path
				}
			}
		}

		// Create command context with timeout
		cmdCtx, cancel := r.createCommandContext(ctx, processedCmd)
		defer cancel()

		// Execute the command
		result, err := r.executeCommand(cmdCtx, processedCmd)
		if err != nil {
			fmt.Printf("    Command failed: %v\n", err)
			return fmt.Errorf("command %s failed: %w", processedCmd.Name, err)
		}

		// Display result
		fmt.Printf("    Exit code: %d\n", result.ExitCode)
		if result.Stdout != "" {
			fmt.Printf("    Stdout: %s\n", result.Stdout)
		}
		if result.Stderr != "" {
			fmt.Printf("    Stderr: %s\n", result.Stderr)
		}

		// Check if command succeeded
		if result.ExitCode != 0 {
			return fmt.Errorf("%w: command %s failed with exit code %d", ErrCommandFailed, processedCmd.Name, result.ExitCode)
		}
	}

	fmt.Printf("Group %s completed successfully\n", processedGroup.Name)
	return nil
}

// executeCommand executes a single command with environment variable resolution
func (r *Runner) executeCommand(ctx context.Context, cmd runnertypes.Command) (*executor.Result, error) {
	// Resolve environment variables for the command
	envVars, err := r.resolveEnvironmentVars(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve environment variables: %w", err)
	}

	// Validate resolved environment variables
	if err := r.validator.ValidateAllEnvironmentVars(envVars); err != nil {
		return nil, fmt.Errorf("resolved environment variables security validation failed: %w", err)
	}

	// Resolve and validate command path if verification manager is available
	if r.verificationManager != nil {
		resolvedPath, err := r.verificationManager.ResolvePath(cmd.Cmd)
		if err != nil {
			return nil, fmt.Errorf("command path resolution failed: %w", err)
		}
		cmd.Cmd = resolvedPath
	}

	// Set working directory from global config if not specified
	if cmd.Dir == "" {
		cmd.Dir = r.config.Global.WorkDir
	}

	// Execute the command
	return r.executor.Execute(ctx, cmd, envVars)
}

// resolveEnvironmentVars resolves environment variables for a command
func (r *Runner) resolveEnvironmentVars(cmd runnertypes.Command) (map[string]string, error) {
	envVars := make(map[string]string)

	// Start with system environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", envSeparatorParts)
		if len(parts) == envSeparatorParts {
			envVars[parts[0]] = parts[1]
		}
	}

	// Add loaded environment variables from .env file
	for k, v := range r.envVars {
		envVars[k] = v
	}

	// Add command-specific environment variables
	for _, env := range cmd.Env {
		parts := strings.SplitN(env, "=", envSeparatorParts)
		if len(parts) == envSeparatorParts {
			key := parts[0]
			value := parts[1]

			// Resolve variable references in the value
			resolvedValue, err := r.resolveVariableReferences(value, envVars)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve variable %s: %w", key, err)
			}

			envVars[key] = resolvedValue
		}
	}

	return envVars, nil
}

// resolveVariableReferences resolves ${VAR} references in a string
func (r *Runner) resolveVariableReferences(value string, envVars map[string]string) (string, error) {
	return r.resolveVariableReferencesWithDepth(value, envVars, make(map[string]bool), 0)
}

// resolveVariableReferencesWithDepth resolves ${VAR} references with circular dependency detection
func (r *Runner) resolveVariableReferencesWithDepth(value string, envVars map[string]string, resolving map[string]bool, depth int) (string, error) {
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

		// Check for circular reference
		if resolving[varName] {
			return "", fmt.Errorf("%w: variable %s references itself", ErrCircularReference, varName)
		}

		varValue, exists := envVars[varName]
		if !exists {
			return "", fmt.Errorf("%w: %s", ErrUndefinedVariable, varName)
		}

		// Mark this variable as being resolved to detect cycles
		resolving[varName] = true

		// Recursively resolve the variable value
		resolvedValue, err := r.resolveVariableReferencesWithDepth(varValue, envVars, resolving, depth+1)
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

// ExecuteCommand executes a single command by name from any group
func (r *Runner) ExecuteCommand(ctx context.Context, commandName string) error {
	// Find the command in all groups
	for _, group := range r.config.Groups {
		for _, cmd := range group.Commands {
			if cmd.Name == commandName {
				fmt.Printf("Executing command: %s from group: %s\n", cmd.Name, group.Name)

				// Execute command with proper context cleanup
				result, err := func() (*executor.Result, error) {
					cmdCtx, cancel := r.createCommandContext(ctx, cmd)
					defer cancel()
					return r.executeCommand(cmdCtx, cmd)
				}()
				if err != nil {
					return fmt.Errorf("command %s failed: %w", cmd.Name, err)
				}

				// Display result
				fmt.Printf("Exit code: %d\n", result.ExitCode)
				if result.Stdout != "" {
					fmt.Printf("Stdout: %s\n", result.Stdout)
				}
				if result.Stderr != "" {
					fmt.Printf("Stderr: %s\n", result.Stderr)
				}

				// Check if command succeeded
				if result.ExitCode != 0 {
					return fmt.Errorf("%w: command %s failed with exit code %d", ErrCommandFailed, cmd.Name, result.ExitCode)
				}

				return nil
			}
		}
	}

	return fmt.Errorf("%w: %s", ErrCommandNotFound, commandName)
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

// GetSanitizedEnvironmentVars returns environment variables with sensitive values redacted
func (r *Runner) GetSanitizedEnvironmentVars() map[string]string {
	return r.validator.SanitizeEnvironmentVariables(r.envVars)
}

// GetTemplateEngine returns the template engine instance
func (r *Runner) GetTemplateEngine() *template.Engine {
	return r.templateEngine
}

// GetResourceManager returns the resource manager instance
func (r *Runner) GetResourceManager() *resource.Manager {
	return r.resourceManager
}

// CleanupAllResources cleans up all managed resources
func (r *Runner) CleanupAllResources() error {
	return r.resourceManager.CleanupAll()
}

// CleanupAutoCleanupResources cleans up resources marked for auto cleanup
func (r *Runner) CleanupAutoCleanupResources() error {
	return r.resourceManager.CleanupAutoCleanup()
}
