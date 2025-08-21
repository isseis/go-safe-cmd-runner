package resource

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// NormalResourceManager implements ResourceManager for normal execution mode
type NormalResourceManager struct {
	// Core dependencies
	executor         executor.CommandExecutor
	fileSystem       executor.FileSystem
	privilegeManager runnertypes.PrivilegeManager

	// State management
	mu       sync.RWMutex
	tempDirs []string
}

// NewNormalResourceManager creates a new NormalResourceManager for normal execution mode
func NewNormalResourceManager(exec executor.CommandExecutor, fs executor.FileSystem, privMgr runnertypes.PrivilegeManager) *NormalResourceManager {
	return &NormalResourceManager{
		executor:         exec,
		fileSystem:       fs,
		privilegeManager: privMgr,
		tempDirs:         make([]string, 0),
	}
}

// ExecuteCommand executes a command in normal mode
func (n *NormalResourceManager) ExecuteCommand(ctx context.Context, cmd runnertypes.Command, _ *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error) {
	start := time.Now()
	result, err := n.executor.Execute(ctx, cmd, env)
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	return &ExecutionResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		Duration: time.Since(start).Milliseconds(),
		DryRun:   false,
	}, nil
}

// CreateTempDir creates a temporary directory in normal mode
func (n *NormalResourceManager) CreateTempDir(groupName string) (string, error) {
	tempDir, err := n.fileSystem.CreateTempDir("", fmt.Sprintf("scr-%s-", groupName))
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	n.mu.Lock()
	n.tempDirs = append(n.tempDirs, tempDir)
	n.mu.Unlock()

	return tempDir, nil
}

// CleanupTempDir cleans up a specific temporary directory in normal mode
func (n *NormalResourceManager) CleanupTempDir(tempDirPath string) error {
	err := n.fileSystem.RemoveAll(tempDirPath)
	if err != nil {
		return fmt.Errorf("failed to cleanup temp dir %s: %w", tempDirPath, err)
	}

	// Remove from tracking
	n.mu.Lock()
	for i, dir := range n.tempDirs {
		if dir == tempDirPath {
			n.tempDirs = append(n.tempDirs[:i], n.tempDirs[i+1:]...)
			break
		}
	}
	n.mu.Unlock()

	return nil
}

// CleanupAllTempDirs cleans up all temporary directories in normal mode
func (n *NormalResourceManager) CleanupAllTempDirs() error {
	n.mu.RLock()
	tempDirs := make([]string, len(n.tempDirs))
	copy(tempDirs, n.tempDirs)
	n.mu.RUnlock()

	var errors []error

	for _, dir := range tempDirs {
		if err := n.CleanupTempDir(dir); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%w: %v", ErrTempDirCleanupFailed, errors)
	}

	return nil
}

// WithPrivileges executes a function with elevated privileges in normal mode
func (n *NormalResourceManager) WithPrivileges(_ context.Context, fn func() error) error {
	if n.privilegeManager == nil {
		return ErrPrivilegeManagerNotAvailable
	}
	elevationCtx := runnertypes.ElevationContext{
		// TODO: Add appropriate fields when needed
	}
	return n.privilegeManager.WithPrivileges(elevationCtx, fn)
}

// IsPrivilegeEscalationRequired checks if a command requires privilege escalation
func (n *NormalResourceManager) IsPrivilegeEscalationRequired(cmd runnertypes.Command) (bool, error) {
	// Check if command is marked as privileged
	if cmd.Privileged {
		return true, nil
	}

	// Check for sudo in command
	isSudo, err := security.IsSudoCommand(cmd.Cmd)
	if err != nil {
		if errors.Is(err, security.ErrSymlinkDepthExceeded) {
			return false, fmt.Errorf("privilege escalation check failed due to excessive symbolic link depth (potential attack): %w", err)
		}
		return false, fmt.Errorf("privilege escalation check failed due to unknown error: %w", err)
	}
	if isSudo {
		return true, nil
	}

	// Additional checks can be added here for specific command patterns
	return false, nil
}

// SendNotification sends a notification in normal mode
func (n *NormalResourceManager) SendNotification(_ string, _ map[string]any) error {
	// In normal mode, we would send actual notifications
	// For now, we just log the notification (no-op)
	return nil
}
