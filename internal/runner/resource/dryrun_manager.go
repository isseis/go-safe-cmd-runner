package resource

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// PathResolver interface for resolving command paths
type PathResolver interface {
	ResolvePath(command string) (string, error)
}

const (
	riskLevelHigh = "high"
)

// DryRunResourceManager implements DryRunResourceManagerInterface for dry-run mode
type DryRunResourceManager struct {
	// Core dependencies
	executor         executor.CommandExecutor
	privilegeManager runnertypes.PrivilegeManager
	pathResolver     PathResolver

	// Dry-run specific
	dryRunOptions    *DryRunOptions
	dryRunResult     *DryRunResult
	resourceAnalyses []ResourceAnalysis

	// State management
	mu sync.RWMutex
}

// NewDryRunResourceManager creates a new DryRunResourceManager for dry-run mode
func NewDryRunResourceManager(exec executor.CommandExecutor, privMgr runnertypes.PrivilegeManager, pathResolver PathResolver, opts *DryRunOptions) *DryRunResourceManager {
	if pathResolver == nil {
		panic("PathResolver is required for DryRunResourceManager")
	}

	return &DryRunResourceManager{
		executor:         exec,
		privilegeManager: privMgr,
		pathResolver:     pathResolver,
		dryRunOptions:    opts,
		dryRunResult: &DryRunResult{
			Metadata: &ResultMetadata{
				GeneratedAt: time.Now(),
				RunID:       fmt.Sprintf("dryrun-%d", time.Now().Unix()),
			},
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
		},
		resourceAnalyses: make([]ResourceAnalysis, 0),
	}
}

// ExecuteCommand simulates command execution in dry-run mode
func (d *DryRunResourceManager) ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error) {
	start := time.Now()

	// Validate command and group for consistency with normal mode
	if err := validateCommand(cmd); err != nil {
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	if err := validateCommandGroup(group); err != nil {
		return nil, fmt.Errorf("command group validation failed: %w", err)
	}

	// Analyze the command
	analysis, err := d.analyzeCommand(ctx, cmd, group, env)
	if err != nil {
		return nil, fmt.Errorf("command analysis failed: %w", err)
	}

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
func (d *DryRunResourceManager) analyzeCommand(_ context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (ResourceAnalysis, error) {
	analysis := ResourceAnalysis{
		Type:      ResourceTypeCommand,
		Operation: OperationExecute,
		Target:    cmd.Cmd,
		Parameters: map[string]any{
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

	// Analyze security risks first
	if err := d.analyzeCommandSecurity(cmd, &analysis); err != nil {
		return ResourceAnalysis{}, err
	}

	// Add user/group privilege specification if present (after security analysis)
	if cmd.HasUserGroupSpecification() {
		analysis.Parameters["run_as_user"] = cmd.RunAsUser
		analysis.Parameters["run_as_group"] = cmd.RunAsGroup

		// Validate user/group configuration in dry-run mode
		if d.privilegeManager != nil && d.privilegeManager.IsPrivilegedExecutionSupported() {
			// Use unified WithPrivileges API with dry-run operation for validation
			executionCtx := runnertypes.ElevationContext{
				Operation:   runnertypes.OperationUserGroupDryRun,
				CommandName: cmd.Name,
				FilePath:    cmd.Cmd,
				RunAsUser:   cmd.RunAsUser,
				RunAsGroup:  cmd.RunAsGroup,
			}
			err := d.privilegeManager.WithPrivileges(executionCtx, func() error {
				return nil // No-op function for dry-run validation
			})

			if err != nil {
				analysis.Impact.Description += fmt.Sprintf(" [ERROR: User/Group validation failed: %v]", err)
				// User/group validation failures are high priority - override any lower risk
				analysis.Impact.SecurityRisk = riskLevelHigh
			} else {
				analysis.Impact.Description += " [INFO: User/Group configuration validated]"
			}
		} else {
			analysis.Impact.Description += " [WARNING: User/Group privilege management not supported]"
		}
	}

	return analysis, nil
}

// analyzeCommandSecurity analyzes security aspects of a command
func (d *DryRunResourceManager) analyzeCommandSecurity(cmd runnertypes.Command, analysis *ResourceAnalysis) error {
	// PathResolver is guaranteed to be non-nil due to constructor validation
	resolvedPath, err := d.pathResolver.ResolvePath(cmd.Cmd)
	if err != nil {
		return fmt.Errorf("failed to resolve command path '%s': %w", cmd.Cmd, err)
	}

	// Analyze security with resolved path
	riskLevel, pattern, reason, err := security.AnalyzeCommandSecurity(resolvedPath, cmd.Args)
	if err != nil {
		return fmt.Errorf("security analysis failed for command '%s': %w", cmd.Cmd, err)
	}
	if riskLevel != security.RiskLevelNone {
		analysis.Impact.SecurityRisk = riskLevel.String()
		analysis.Impact.Description += fmt.Sprintf(" [WARNING: %s - %s]", reason, pattern)
	}

	return nil
}

// CreateTempDir simulates creating a temporary directory in dry-run mode
func (d *DryRunResourceManager) CreateTempDir(groupName string) (string, error) {
	simulatedPath := fmt.Sprintf("/tmp/scr-%s-XXXXXX", groupName)

	// Record the analysis
	analysis := ResourceAnalysis{
		Type:      ResourceTypeFilesystem,
		Operation: OperationCreate,
		Target:    simulatedPath,
		Parameters: map[string]any{
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

// CleanupTempDir simulates cleaning up a temporary directory in dry-run mode
func (d *DryRunResourceManager) CleanupTempDir(tempDirPath string) error {
	// Record the analysis
	analysis := ResourceAnalysis{
		Type:      ResourceTypeFilesystem,
		Operation: OperationDelete,
		Target:    tempDirPath,
		Parameters: map[string]any{
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

// CleanupAllTempDirs simulates cleaning up all temporary directories in dry-run mode
func (d *DryRunResourceManager) CleanupAllTempDirs() error {
	// In dry-run mode, there are no actual temp dirs to clean up
	// This is just a simulation
	return nil
}

// WithPrivileges simulates executing a function with elevated privileges in dry-run mode
func (d *DryRunResourceManager) WithPrivileges(_ context.Context, fn func() error) error {
	// Record the analysis
	analysis := ResourceAnalysis{
		Type:      ResourceTypePrivilege,
		Operation: OperationEscalate,
		Target:    "system_privileges",
		Parameters: map[string]any{
			"context": "privilege_escalation",
		},
		Impact: ResourceImpact{
			Reversible:   true,
			Persistent:   false,
			SecurityRisk: riskLevelHigh,
			Description:  "Execute function with elevated privileges",
		},
		Timestamp: time.Now(),
	}

	d.RecordAnalysis(&analysis)

	// In dry-run mode, we simulate the privilege escalation by just calling the function
	// This maintains the same execution path without actually escalating privileges
	return fn()
}

// SendNotification simulates sending a notification in dry-run mode
func (d *DryRunResourceManager) SendNotification(message string, details map[string]any) error {
	// Record the analysis
	analysis := ResourceAnalysis{
		Type:      ResourceTypeNetwork,
		Operation: OperationSend,
		Target:    "notification_service",
		Parameters: map[string]any{
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
}

// GetDryRunResults returns the dry-run results
func (d *DryRunResourceManager) GetDryRunResults() *DryRunResult {
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
func (d *DryRunResourceManager) RecordAnalysis(analysis *ResourceAnalysis) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.resourceAnalyses = append(d.resourceAnalyses, *analysis)
}
