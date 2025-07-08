// Package runner provides the core functionality for running commands
// in a safe and controlled manner with group-based execution and dependency management.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/joho/godotenv"
)

// Error definitions
var (
	ErrCommandFailed       = errors.New("command failed")
	ErrUnclosedVariableRef = errors.New("unclosed variable reference")
	ErrUndefinedVariable   = errors.New("undefined variable")
	ErrCommandNotFound     = errors.New("command not found")
	ErrCircularReference   = errors.New("circular variable reference detected")
)

// Constants
const (
	envSeparatorParts  = 2
	maxResolutionDepth = 100 // Maximum number of variable resolution iterations
)

// Runner manages the execution of command groups
type Runner struct {
	executor executor.CommandExecutor
	config   *runnertypes.Config
	envVars  map[string]string
}

// NewRunner creates a new command runner with the given configuration
func NewRunner(config *runnertypes.Config) *Runner {
	return &Runner{
		executor: executor.NewDefaultExecutor(),
		config:   config,
		envVars:  make(map[string]string),
	}
}

// LoadEnvironment loads environment variables from the specified .env file and system environment.
// If envFile is empty, only system environment variables will be loaded.
// If loadSystemEnv is true, system environment variables will be loaded first,
// then overridden by the .env file if specified.
func (r *Runner) LoadEnvironment(envFile string, loadSystemEnv bool) error {
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

	// Execute commands in the group sequentially
	for i, cmd := range group.Commands {
		fmt.Printf("  [%d/%d] Executing command: %s\n", i+1, len(group.Commands), cmd.Name)

		// Create command context with timeout
		cmdCtx, cancel := r.createCommandContext(ctx, cmd)
		defer cancel()

		// Execute the command
		result, err := r.executeCommand(cmdCtx, cmd)
		if err != nil {
			fmt.Printf("    Command failed: %v\n", err)
			return fmt.Errorf("command %s failed: %w", cmd.Name, err)
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
			return fmt.Errorf("%w: command %s failed with exit code %d", ErrCommandFailed, cmd.Name, result.ExitCode)
		}
	}

	fmt.Printf("Group %s completed successfully\n", group.Name)
	return nil
}

// executeCommand executes a single command with environment variable resolution
func (r *Runner) executeCommand(ctx context.Context, cmd runnertypes.Command) (*executor.Result, error) {
	// Resolve environment variables for the command
	envVars, err := r.resolveEnvironmentVars(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve environment variables: %w", err)
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
