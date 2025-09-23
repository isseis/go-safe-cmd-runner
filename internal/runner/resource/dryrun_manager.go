package resource

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// Static errors
var (
	ErrPathResolverRequired  = errors.New("PathResolver is required for DryRunResourceManager")
	ErrPathTraversalDetected = errors.New("path validation failed: path traversal detected")
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

	// Output capture dependencies
	outputManager output.CaptureManager

	// Dry-run specific
	dryRunOptions    *DryRunOptions
	dryRunResult     *DryRunResult
	resourceAnalyses []ResourceAnalysis

	// State management
	mu sync.RWMutex
}

// NewDryRunResourceManager creates a new DryRunResourceManager for dry-run mode
func NewDryRunResourceManager(exec executor.CommandExecutor, privMgr runnertypes.PrivilegeManager, pathResolver PathResolver, opts *DryRunOptions) (*DryRunResourceManager, error) {
	// Delegate to NewDryRunResourceManagerWithOutput with nil outputManager
	return NewDryRunResourceManagerWithOutput(exec, privMgr, pathResolver, nil, opts)
}

// NewDryRunResourceManagerWithOutput creates a new DryRunResourceManager with output capture support
func NewDryRunResourceManagerWithOutput(exec executor.CommandExecutor, privMgr runnertypes.PrivilegeManager, pathResolver PathResolver, outputMgr output.CaptureManager, opts *DryRunOptions) (*DryRunResourceManager, error) {
	if pathResolver == nil {
		return nil, ErrPathResolverRequired
	}
	if opts == nil {
		opts = &DryRunOptions{}
	}

	// Extract security analysis configuration from options
	return &DryRunResourceManager{
		executor:         exec,
		privilegeManager: privMgr,
		pathResolver:     pathResolver,
		outputManager:    outputMgr,

		dryRunOptions: opts,
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
	}, nil
}

// ValidateOutputPath validates an output path in dry-run mode
func (d *DryRunResourceManager) ValidateOutputPath(outputPath, workDir string) error {
	if outputPath == "" {
		return nil // No output path to validate
	}

	if d.outputManager == nil {
		// In dry-run mode, we can still perform basic validation without an output manager
		// Check for path traversal by analyzing path components
		if common.ContainsPathTraversalSegment(outputPath) {
			return fmt.Errorf("%w: %s", ErrPathTraversalDetected, outputPath)
		}
		return nil
	}

	// Use output manager's validation if available
	return d.outputManager.ValidateOutputPath(outputPath, workDir)
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

	// Check if output capture is requested and analyze it
	if cmd.Output != "" && d.outputManager != nil {
		outputAnalysis := d.analyzeOutput(cmd, group)
		d.RecordAnalysis(&outputAnalysis)
	}

	// Record the analysis
	d.RecordAnalysis(&analysis)

	// Generate simulated output
	stdout := fmt.Sprintf("[DRY-RUN] Would execute: %s", cmd.Cmd)
	if cmd.Dir != "" {
		stdout += fmt.Sprintf(" (in directory: %s)", cmd.Dir)
	}
	if cmd.Output != "" {
		stdout += fmt.Sprintf(" (output would be captured to: %s)", cmd.Output)
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

// analyzeCommandSecurity resolves the command path and performs security analysis
// using the configuration stored in the DryRunResourceManager.
func (d *DryRunResourceManager) analyzeCommandSecurity(cmd runnertypes.Command, analysis *ResourceAnalysis) error {
	// PathResolver is guaranteed to be non-nil due to constructor validation
	resolvedPath, err := d.pathResolver.ResolvePath(cmd.Cmd)
	if err != nil {
		return fmt.Errorf("failed to resolve command path '%s': %w. This typically occurs if the command is not found in the system PATH or there are permission issues preventing access", cmd.Cmd, err)
	}

	// Analyze security with resolved path using cached validator
	opts := &security.AnalysisOptions{
		SkipStandardPaths: d.dryRunOptions.SkipStandardPaths,
		HashDir:           d.dryRunOptions.HashDir,
	}
	riskLevel, pattern, reason, err := security.AnalyzeCommandSecurity(resolvedPath, cmd.Args, opts)
	if err != nil {
		return fmt.Errorf("security analysis failed for command '%s': %w", cmd.Cmd, err)
	}
	if riskLevel != runnertypes.RiskLevelUnknown {
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

// analyzeOutput analyzes output capture configuration for dry-run
func (d *DryRunResourceManager) analyzeOutput(cmd runnertypes.Command, group *runnertypes.CommandGroup) ResourceAnalysis {
	analysis := ResourceAnalysis{
		Type:      ResourceTypeFilesystem,
		Operation: OperationCreate,
		Target:    cmd.Output,
		Parameters: map[string]any{
			"output_path":       cmd.Output,
			"command":           cmd.Cmd,
			"working_directory": group.WorkDir,
		},
		Impact: ResourceImpact{
			Reversible:  false, // Output files are persistent
			Persistent:  true,
			Description: fmt.Sprintf("Capture command output to file: %s", cmd.Output),
		},
		Timestamp: time.Now(),
	}

	// Use the output manager to analyze the output path
	outputAnalysis, err := d.outputManager.AnalyzeOutput(cmd.Output, group.WorkDir)
	if err != nil {
		analysis.Impact.Description += fmt.Sprintf(" [ERROR: %v]", err)
		analysis.Impact.SecurityRisk = riskLevelHigh
		return analysis // Return analysis with error info, but don't fail
	}

	// Add analysis results to parameters
	analysis.Parameters["resolved_path"] = outputAnalysis.ResolvedPath
	analysis.Parameters["directory_exists"] = outputAnalysis.DirectoryExists
	analysis.Parameters["write_permission"] = outputAnalysis.WritePermission
	analysis.Parameters["security_risk"] = outputAnalysis.SecurityRisk.String()
	analysis.Parameters["max_size_limit"] = outputAnalysis.MaxSizeLimit

	// Set security risk based on analysis
	analysis.Impact.SecurityRisk = outputAnalysis.SecurityRisk.String()

	// Update description based on analysis
	if !outputAnalysis.WritePermission {
		analysis.Impact.Description += " [WARNING: No write permission]"
	}
	if !outputAnalysis.DirectoryExists {
		analysis.Impact.Description += " [INFO: Directory will be created]"
	}
	if outputAnalysis.ErrorMessage != "" {
		analysis.Impact.Description += fmt.Sprintf(" [ERROR: %s]", outputAnalysis.ErrorMessage)
	}

	return analysis
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
