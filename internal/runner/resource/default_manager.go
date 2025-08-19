package resource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Static errors for better error handling
var (
	ErrUnknownExecutionMode         = errors.New("unknown execution mode")
	ErrPrivilegeManagerNotAvailable = errors.New("privilege manager not available")
	ErrTempDirCleanupFailed         = errors.New("failed to cleanup some temp directories")
)

// DefaultResourceManager is the default implementation of ResourceManager
type DefaultResourceManager struct {
	// Core dependencies
	executor         executor.CommandExecutor
	fileSystem       executor.FileSystem
	privilegeManager runnertypes.PrivilegeManager

	// Current execution mode
	mode          ExecutionMode
	dryRunOptions *DryRunOptions

	// State management
	mu               sync.RWMutex
	tempDirs         []string
	dryRunResult     *DryRunResult
	resourceAnalyses []ResourceAnalysis
}

// NewDefaultResourceManager creates a new DefaultResourceManager
func NewDefaultResourceManager(exec executor.CommandExecutor, fs executor.FileSystem, privMgr runnertypes.PrivilegeManager) *DefaultResourceManager {
	return &DefaultResourceManager{
		executor:         exec,
		fileSystem:       fs,
		privilegeManager: privMgr,
		mode:             ExecutionModeNormal,
		tempDirs:         make([]string, 0),
		resourceAnalyses: make([]ResourceAnalysis, 0),
	}
}

// SetMode sets the execution mode and options
func (d *DefaultResourceManager) SetMode(mode ExecutionMode, opts *DryRunOptions) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.mode = mode
	d.dryRunOptions = opts

	// Initialize dry-run result if switching to dry-run mode
	if mode == ExecutionModeDryRun {
		d.dryRunResult = &DryRunResult{
			Metadata: &ResultMetadata{
				GeneratedAt: time.Now(),
				RunID:       fmt.Sprintf("dryrun-%d", time.Now().Unix()),
			},
			ExecutionPlan:    &ExecutionPlan{},
			ResourceAnalyses: make([]ResourceAnalysis, 0),
			SecurityAnalysis: &SecurityAnalysis{
				Risks:             make([]SecurityRisk, 0),
				PrivilegeChanges:  make([]PrivilegeChange, 0),
				EnvironmentAccess: make([]EnvironmentAccess, 0),
				FileAccess:        make([]FileAccess, 0),
			},
			EnvironmentInfo: &EnvironmentInfo{
				VariableUsage: make(map[string][]string),
			},
			Errors:   make([]DryRunError, 0),
			Warnings: make([]DryRunWarning, 0),
		}
		d.resourceAnalyses = make([]ResourceAnalysis, 0)
	}
}

// GetMode returns the current execution mode
func (d *DefaultResourceManager) GetMode() ExecutionMode {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.mode
}

// ExecuteCommand executes a command or simulates it in dry-run mode
func (d *DefaultResourceManager) ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error) {
	d.mu.RLock()
	currentMode := d.mode
	d.mu.RUnlock()

	start := time.Now()

	switch currentMode {
	case ExecutionModeNormal:
		return d.executeCommandNormal(ctx, cmd, env, start)

	case ExecutionModeDryRun:
		return d.executeCommandDryRun(ctx, cmd, group, env, start)

	default:
		return nil, fmt.Errorf("%w: %v", ErrUnknownExecutionMode, currentMode)
	}
}

// executeCommandNormal executes command in normal mode
func (d *DefaultResourceManager) executeCommandNormal(ctx context.Context, cmd runnertypes.Command, env map[string]string, start time.Time) (*ExecutionResult, error) {
	result, err := d.executor.Execute(ctx, cmd, env)
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

// executeCommandDryRun simulates command execution in dry-run mode
func (d *DefaultResourceManager) executeCommandDryRun(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string, start time.Time) (*ExecutionResult, error) {
	// Analyze the command
	analysis := d.analyzeCommand(ctx, cmd, group, env)

	// Record the analysis
	d.RecordAnalysis(&analysis)

	// Generate simulated output
	stdout := fmt.Sprintf("[DRY-RUN] Would execute: %s", cmd.Cmd)
	if cmd.Dir != "" {
		stdout += fmt.Sprintf(" (in directory: %s)", cmd.Dir)
	}

	return &ExecutionResult{
		ExitCode: 0,
		Stdout:   stdout,
		Stderr:   "",
		Duration: time.Since(start).Milliseconds(),
		DryRun:   true,
		Analysis: &analysis,
	}, nil
}

// analyzeCommand analyzes a command for dry-run
func (d *DefaultResourceManager) analyzeCommand(_ context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) ResourceAnalysis {
	analysis := ResourceAnalysis{
		Type:      ResourceTypeCommand,
		Operation: OperationExecute,
		Target:    cmd.Cmd,
		Parameters: map[string]interface{}{
			"command":           cmd.Cmd,
			"working_directory": cmd.Dir,
			"timeout":           cmd.Timeout,
		},
		Impact: ResourceImpact{
			Reversible:  false, // Commands are generally not reversible
			Persistent:  true,  // Command effects are generally persistent
			Description: fmt.Sprintf("Execute command: %s", cmd.Cmd),
		},
		Timestamp: time.Now(),
	}

	// Add environment variables to parameters if they exist
	if len(env) > 0 {
		analysis.Parameters["environment"] = env
	}

	// Add group information if available
	if group != nil {
		analysis.Parameters["group"] = group.Name
		analysis.Parameters["group_description"] = group.Description
	}

	// Analyze security risks
	d.analyzeCommandSecurity(cmd, &analysis)

	return analysis
}

// analyzeCommandSecurity analyzes security aspects of a command
func (d *DefaultResourceManager) analyzeCommandSecurity(cmd runnertypes.Command, analysis *ResourceAnalysis) {
	cmdLower := strings.ToLower(cmd.Cmd)

	// Initialize with no risk
	currentRisk := ""

	// Check for privilege escalation requirements first (lower priority)
	if cmd.Privileged || strings.Contains(cmdLower, "sudo") {
		currentRisk = "medium"
		analysis.Impact.Description += " [PRIVILEGE: Requires elevated privileges]"
	}

	// Check for potentially dangerous commands (higher priority - can override privilege risk)
	dangerousPatterns := []string{
		"rm -rf", "sudo rm", "format", "mkfs", "fdisk",
		"dd if=", "chmod 777", "chown root",
		"wget", "curl", "nc -", "netcat",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(cmdLower, pattern) {
			currentRisk = "high"
			analysis.Impact.Description += fmt.Sprintf(" [WARNING: Potentially dangerous command pattern: %s]", pattern)
			break
		}
	}

	// Set the final risk level
	analysis.Impact.SecurityRisk = currentRisk
}

// CreateTempDir creates a temporary directory
func (d *DefaultResourceManager) CreateTempDir(groupName string) (string, error) {
	d.mu.RLock()
	currentMode := d.mode
	d.mu.RUnlock()

	switch currentMode {
	case ExecutionModeNormal:
		return d.createTempDirNormal(groupName)

	case ExecutionModeDryRun:
		return d.createTempDirDryRun(groupName)

	default:
		return "", fmt.Errorf("%w: %v", ErrUnknownExecutionMode, currentMode)
	}
}

// createTempDirNormal creates a temporary directory in normal mode
func (d *DefaultResourceManager) createTempDirNormal(groupName string) (string, error) {
	tempDir, err := d.fileSystem.CreateTempDir("", fmt.Sprintf("scr-%s-", groupName))
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	d.mu.Lock()
	d.tempDirs = append(d.tempDirs, tempDir)
	d.mu.Unlock()

	return tempDir, nil
}

// createTempDirDryRun simulates creating a temporary directory in dry-run mode
func (d *DefaultResourceManager) createTempDirDryRun(groupName string) (string, error) {
	simulatedPath := fmt.Sprintf("/tmp/scr-%s-XXXXXX", groupName)

	// Record the analysis
	analysis := ResourceAnalysis{
		Type:      ResourceTypeFilesystem,
		Operation: OperationCreate,
		Target:    simulatedPath,
		Parameters: map[string]interface{}{
			"group_name": groupName,
			"purpose":    "temporary_directory",
		},
		Impact: ResourceImpact{
			Reversible:  true,
			Persistent:  false,
			Description: fmt.Sprintf("Create temporary directory for group: %s", groupName),
		},
		Timestamp: time.Now(),
	}

	d.RecordAnalysis(&analysis)

	return simulatedPath, nil
}

// CleanupTempDir cleans up a specific temporary directory
func (d *DefaultResourceManager) CleanupTempDir(tempDirPath string) error {
	d.mu.RLock()
	currentMode := d.mode
	d.mu.RUnlock()

	switch currentMode {
	case ExecutionModeNormal:
		return d.cleanupTempDirNormal(tempDirPath)

	case ExecutionModeDryRun:
		return d.cleanupTempDirDryRun(tempDirPath)

	default:
		return fmt.Errorf("%w: %v", ErrUnknownExecutionMode, currentMode)
	}
}

// cleanupTempDirNormal cleans up a temporary directory in normal mode
func (d *DefaultResourceManager) cleanupTempDirNormal(tempDirPath string) error {
	err := d.fileSystem.RemoveAll(tempDirPath)
	if err != nil {
		return fmt.Errorf("failed to cleanup temp dir %s: %w", tempDirPath, err)
	}

	// Remove from tracking
	d.mu.Lock()
	for i, dir := range d.tempDirs {
		if dir == tempDirPath {
			d.tempDirs = append(d.tempDirs[:i], d.tempDirs[i+1:]...)
			break
		}
	}
	d.mu.Unlock()

	return nil
}

// cleanupTempDirDryRun simulates cleaning up a temporary directory in dry-run mode
func (d *DefaultResourceManager) cleanupTempDirDryRun(tempDirPath string) error {
	// Record the analysis
	analysis := ResourceAnalysis{
		Type:      ResourceTypeFilesystem,
		Operation: OperationDelete,
		Target:    tempDirPath,
		Parameters: map[string]interface{}{
			"path": tempDirPath,
		},
		Impact: ResourceImpact{
			Reversible:  false,
			Persistent:  true,
			Description: fmt.Sprintf("Cleanup temporary directory: %s", tempDirPath),
		},
		Timestamp: time.Now(),
	}

	d.RecordAnalysis(&analysis)
	return nil
}

// CleanupAllTempDirs cleans up all temporary directories
func (d *DefaultResourceManager) CleanupAllTempDirs() error {
	d.mu.RLock()
	tempDirs := make([]string, len(d.tempDirs))
	copy(tempDirs, d.tempDirs)
	currentMode := d.mode
	d.mu.RUnlock()

	var errors []error

	for _, dir := range tempDirs {
		switch currentMode {
		case ExecutionModeNormal:
			if err := d.cleanupTempDirNormal(dir); err != nil {
				errors = append(errors, err)
			}
		case ExecutionModeDryRun:
			if err := d.cleanupTempDirDryRun(dir); err != nil {
				errors = append(errors, err)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%w: %v", ErrTempDirCleanupFailed, errors)
	}

	return nil
}

// WithPrivileges executes a function with elevated privileges
func (d *DefaultResourceManager) WithPrivileges(_ context.Context, fn func() error) error {
	d.mu.RLock()
	currentMode := d.mode
	privMgr := d.privilegeManager
	d.mu.RUnlock()

	switch currentMode {
	case ExecutionModeNormal:
		if privMgr == nil {
			return ErrPrivilegeManagerNotAvailable
		}
		elevationCtx := runnertypes.ElevationContext{
			// TODO: Add appropriate fields when needed
		}
		return privMgr.WithPrivileges(elevationCtx, fn)

	case ExecutionModeDryRun:
		// Record the analysis
		analysis := ResourceAnalysis{
			Type:      ResourceTypePrivilege,
			Operation: OperationEscalate,
			Target:    "system_privileges",
			Parameters: map[string]interface{}{
				"context": "privilege_escalation",
			},
			Impact: ResourceImpact{
				Reversible:   true,
				Persistent:   false,
				SecurityRisk: "high",
				Description:  "Execute function with elevated privileges",
			},
			Timestamp: time.Now(),
		}

		d.RecordAnalysis(&analysis)

		// In dry-run mode, we simulate the privilege escalation by just calling the function
		// This maintains the same execution path without actually escalating privileges
		return fn()

	default:
		return fmt.Errorf("%w: %v", ErrUnknownExecutionMode, currentMode)
	}
}

// IsPrivilegeEscalationRequired checks if a command requires privilege escalation
func (d *DefaultResourceManager) IsPrivilegeEscalationRequired(cmd runnertypes.Command) (bool, error) {
	// Check if command is marked as privileged
	if cmd.Privileged {
		return true, nil
	}

	// Check for sudo in command
	if strings.Contains(strings.ToLower(cmd.Cmd), "sudo") {
		return true, nil
	}

	// Additional checks can be added here for specific command patterns
	return false, nil
}

// SendNotification sends a notification
func (d *DefaultResourceManager) SendNotification(message string, details map[string]interface{}) error {
	d.mu.RLock()
	currentMode := d.mode
	d.mu.RUnlock()

	switch currentMode {
	case ExecutionModeNormal:
		// In normal mode, we would send actual notifications
		// For now, we just log the notification (no-op)
		return nil

	case ExecutionModeDryRun:
		// Record the analysis
		analysis := ResourceAnalysis{
			Type:      ResourceTypeNetwork,
			Operation: OperationSend,
			Target:    "notification_service",
			Parameters: map[string]interface{}{
				"message": message,
				"details": details,
			},
			Impact: ResourceImpact{
				Reversible:  false,
				Persistent:  false,
				Description: fmt.Sprintf("Send notification: %s", message),
			},
			Timestamp: time.Now(),
		}

		d.RecordAnalysis(&analysis)
		return nil

	default:
		return fmt.Errorf("%w: %v", ErrUnknownExecutionMode, currentMode)
	}
}

// GetDryRunResults returns the dry-run results
func (d *DefaultResourceManager) GetDryRunResults() *DryRunResult {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.dryRunResult == nil {
		return nil
	}

	// Update the resource analyses in the result
	d.dryRunResult.ResourceAnalyses = make([]ResourceAnalysis, len(d.resourceAnalyses))
	copy(d.dryRunResult.ResourceAnalyses, d.resourceAnalyses)

	return d.dryRunResult
}

// RecordAnalysis records a resource analysis
func (d *DefaultResourceManager) RecordAnalysis(analysis *ResourceAnalysis) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.resourceAnalyses = append(d.resourceAnalyses, *analysis)

	// Also update dry-run result if we're in dry-run mode
	if d.dryRunResult != nil {
		d.dryRunResult.ResourceAnalyses = make([]ResourceAnalysis, len(d.resourceAnalyses))
		copy(d.dryRunResult.ResourceAnalyses, d.resourceAnalyses)
	}
}
