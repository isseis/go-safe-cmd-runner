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
	ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error)

	// Filesystem operations
	CreateTempDir(groupName string) (string, error)
	CleanupTempDir(tempDirPath string) error
	CleanupAllTempDirs() error

	// Privilege management
	WithPrivileges(ctx context.Context, fn func() error) error
	IsPrivilegeEscalationRequired(cmd runnertypes.Command) (bool, error)

	// Network operations
	SendNotification(message string, details map[string]any) error
}

// DryRunResourceManager extends ResourceManager with dry-run specific functionality
type DryRunResourceManager interface {
	ResourceManager

	// Dry-run specific
	GetDryRunResults() *DryRunResult
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
