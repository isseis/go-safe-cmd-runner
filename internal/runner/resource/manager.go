// Package resource provides the ResourceManager interface and related types
// for managing all side-effects in both normal execution and dry-run modes.
package resource

import (
	"context"
	"errors"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

const (
	unknownString = "unknown"
	sudoCommand   = "sudo"
)

// Static errors for better error handling
var (
	ErrPrivilegeManagerNotAvailable = errors.New("privilege manager not available")
	ErrTempDirCleanupFailed         = errors.New("failed to cleanup some temp directories")
	ErrNilResult                    = errors.New("result cannot be nil")
	// Validation errors for consistent behavior across execution modes
	ErrEmptyCommand     = errors.New("command cannot be empty")
	ErrEmptyCommandName = errors.New("command name cannot be empty")
	ErrNilCommandGroup  = errors.New("command group cannot be nil")
	ErrEmptyGroupName   = errors.New("command group name cannot be empty")
)

// ExecutionMode determines how all operations are handled
type ExecutionMode int

const (
	// ExecutionModeNormal indicates normal execution with actual side-effects
	ExecutionModeNormal ExecutionMode = iota
	// ExecutionModeDryRun indicates dry-run mode with simulated operations
	ExecutionModeDryRun
)

// String returns the string representation of ExecutionMode
func (e ExecutionMode) String() string {
	switch e {
	case ExecutionModeNormal:
		return "normal"
	case ExecutionModeDryRun:
		return "dry-run"
	default:
		return unknownString
	}
}

// ResourceManager manages all side-effects (commands, filesystem, privileges, etc.)
// nolint:revive // ResourceManager is intentionally named to be clear about its purpose
type ResourceManager interface {
	// Command execution
	ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (*ExecutionResult, error)

	// Output validation - validates output paths before command execution
	ValidateOutputPath(outputPath, workDir string) error

	// Filesystem operations
	CreateTempDir(groupName string) (string, error)
	CleanupTempDir(tempDirPath string) error
	CleanupAllTempDirs() error

	// Privilege management
	WithPrivileges(ctx context.Context, fn func() error) error

	// Network operations
	SendNotification(message string, details map[string]any) error

	// Dry-run results (returns nil for normal execution mode)
	GetDryRunResults() *DryRunResult
}

// DryRunResourceManagerInterface extends ResourceManager with dry-run specific functionality
type DryRunResourceManagerInterface interface {
	ResourceManager

	// Dry-run specific
	RecordAnalysis(analysis *ResourceAnalysis)
}

// ExecutionResult unified result for both normal and dry-run
type ExecutionResult struct {
	ExitCode int               `json:"exit_code"`
	Stdout   string            `json:"stdout"`
	Stderr   string            `json:"stderr"`
	Duration int64             `json:"duration_ms"` // Duration in milliseconds
	DryRun   bool              `json:"dry_run"`
	Analysis *ResourceAnalysis `json:"analysis,omitempty"`
}

// validateCommand validates command for consistency across execution modes
func validateCommand(cmd *runnertypes.RuntimeCommand) error {
	if cmd.Cmd() == "" {
		return ErrEmptyCommand
	}
	if cmd.Name() == "" {
		return ErrEmptyCommandName
	}
	return nil
}

// validateCommandGroup validates command group for consistency across execution modes
func validateCommandGroup(group *runnertypes.GroupSpec) error {
	if group == nil {
		return ErrNilCommandGroup
	}
	if group.Name == "" {
		return ErrEmptyGroupName
	}
	return nil
}
