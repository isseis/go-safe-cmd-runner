package resource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	ErrPathResolverRequired      = errors.New("PathResolver is required for DryRunResourceManager")
	ErrPathTraversalDetected     = errors.New("path validation failed: path traversal detected")
	ErrNoCommandResourceAnalysis = errors.New("no command resource analysis found to update")
	ErrInvalidCommandToken       = errors.New("invalid command token")
	ErrDuplicateDebugInfoUpdate  = errors.New("UpdateCommandDebugInfo called multiple times for the same token")
)

// PathResolver interface for resolving command paths
type PathResolver interface {
	ResolvePath(command string) (string, error)
}

const (
	riskLevelHigh = "high"
)

// DryRunResourceManager implements ResourceManager for dry-run mode
type DryRunResourceManager struct {
	// Core dependencies
	executor         executor.CommandExecutor
	privilegeManager runnertypes.PrivilegeManager
	pathResolver     PathResolver

	// Output capture dependencies
	outputManager output.CaptureManager

	// Dry-run specific
	dryRunOptions *DryRunOptions
	dryRunResult  *DryRunResult
	// resourceAnalyses is an append-only slice that stores all resource analyses.
	// INVARIANT: Elements must never be deleted or reordered after being appended.
	// This guarantees that indices stored in tokenToIndex remain valid throughout the manager's lifetime.
	resourceAnalyses []ResourceAnalysis

	// Token management - maps CommandToken to index in resourceAnalyses
	tokenToIndex map[CommandToken]int
	nextTokenID  uint64

	// Execution tracking (status, phase, error)
	executionStatus ExecutionStatus
	executionPhase  ExecutionPhase
	executionError  *ExecutionError

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
		tokenToIndex:     make(map[CommandToken]int),
		nextTokenID:      1,
		executionStatus:  StatusSuccess,  // Default to success, will be updated if errors occur
		executionPhase:   PhaseCompleted, // Default to completed, will be updated if errors occur
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
// Returns a token that can be used to update the command's debug info
func (d *DryRunResourceManager) ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (CommandToken, *ExecutionResult, error) {
	start := time.Now()

	// Validate command and group for consistency with normal mode
	if err := validateCommand(cmd); err != nil {
		return "", nil, fmt.Errorf("command validation failed: %w", err)
	}

	if err := validateCommandGroup(group); err != nil {
		return "", nil, fmt.Errorf("command group validation failed: %w", err)
	}

	// Analyze the command
	analysis, err := d.analyzeCommand(ctx, cmd, group, env)
	if err != nil {
		return "", nil, fmt.Errorf("command analysis failed: %w", err)
	}

	// Check if output capture is requested and analyze it
	if cmd.Output() != "" && d.outputManager != nil {
		outputAnalysis := d.analyzeOutput(cmd)
		d.RecordAnalysis(&outputAnalysis)
	}

	// Generate token, record the analysis and store token mapping
	token := d.recordAnalysis(analysis)

	// Generate simulated output
	stdout := fmt.Sprintf("[DRY-RUN] Would execute: %s", cmd.ExpandedCmd)
	if cmd.EffectiveWorkDir != "" {
		stdout += fmt.Sprintf(" (in directory: %s)", cmd.EffectiveWorkDir)
	}
	if cmd.Output() != "" {
		stdout += fmt.Sprintf(" (output would be captured to: %s)", cmd.Output())
	}

	return token, &ExecutionResult{
		ExitCode: 0,
		Stdout:   stdout,
		Stderr:   "",
		Duration: time.Since(start).Milliseconds(),
		DryRun:   true,
		Analysis: &analysis,
	}, nil
}

// recordAnalysis records the analysis and returns a unique command token
func (d *DryRunResourceManager) recordAnalysis(analysis ResourceAnalysis) CommandToken {
	d.mu.Lock()
	defer d.mu.Unlock()

	tokenID := d.nextTokenID
	d.nextTokenID++

	token := CommandToken(fmt.Sprintf("cmd-%d-%d", time.Now().UnixNano(), tokenID))

	commandIndex := len(d.resourceAnalyses)
	d.resourceAnalyses = append(d.resourceAnalyses, analysis)
	d.tokenToIndex[token] = commandIndex
	return token
}

// analyzeCommand analyzes a command for dry-run
func (d *DryRunResourceManager) analyzeCommand(_ context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (ResourceAnalysis, error) {
	analysis := ResourceAnalysis{
		Type:      ResourceTypeCommand,
		Operation: OperationExecute,
		Target:    cmd.ExpandedCmd,
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"command":           NewStringValue(cmd.ExpandedCmd),
			"args":              NewStringSliceValue(cmd.ExpandedArgs),
			"working_directory": NewStringValue(cmd.EffectiveWorkDir),
			"timeout":           NewIntValue(int64(cmd.EffectiveTimeout)),
			"timeout_level":     NewStringValue(cmd.TimeoutResolution.Level),
		},
		Impact: ResourceImpact{
			Reversible:  false, // Commands are generally not reversible
			Persistent:  true,  // Command effects are generally persistent
			Description: fmt.Sprintf("Execute command: %s", cmd.ExpandedCmd),
		},
		Timestamp: time.Now(),
	}

	// Add environment variables to parameters if they exist
	if len(env) > 0 {
		analysis.Parameters["environment"] = NewStringMapValue(env)
	}

	// Add group information if available
	if group != nil {
		analysis.Parameters["group"] = NewStringValue(group.Name)
		analysis.Parameters["group_description"] = NewStringValue(group.Description)
	}

	// Analyze security risks first
	if err := d.analyzeCommandSecurity(cmd, &analysis); err != nil {
		return ResourceAnalysis{}, err
	}

	// Add user/group privilege specification if present (after security analysis)
	if cmd.HasUserGroupSpecification() {
		analysis.Parameters["run_as_user"] = NewStringValue(cmd.RunAsUser())
		analysis.Parameters["run_as_group"] = NewStringValue(cmd.RunAsGroup())

		// Validate user/group configuration in dry-run mode
		if d.privilegeManager != nil && d.privilegeManager.IsPrivilegedExecutionSupported() {
			// Use unified WithPrivileges API with dry-run operation for validation
			executionCtx := runnertypes.ElevationContext{
				Operation:   runnertypes.OperationUserGroupDryRun,
				CommandName: cmd.Name(),
				FilePath:    cmd.ExpandedCmd,
				RunAsUser:   cmd.RunAsUser(),
				RunAsGroup:  cmd.RunAsGroup(),
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
func (d *DryRunResourceManager) analyzeCommandSecurity(cmd *runnertypes.RuntimeCommand, analysis *ResourceAnalysis) error {
	// PathResolver is guaranteed to be non-nil due to constructor validation
	resolvedPath, err := d.pathResolver.ResolvePath(cmd.ExpandedCmd)
	if err != nil {
		return fmt.Errorf("failed to resolve command path '%s': %w. This typically occurs if the command is not found in the system PATH or there are permission issues preventing access", cmd.ExpandedCmd, err)
	}

	// Analyze security with resolved path using cached validator
	opts := &security.AnalysisOptions{
		VerifyStandardPaths: d.dryRunOptions.VerifyStandardPaths,
		HashDir:             d.dryRunOptions.HashDir,
	}
	riskLevel, pattern, reason, err := security.AnalyzeCommandSecurity(resolvedPath, cmd.ExpandedArgs, opts)
	if err != nil {
		return fmt.Errorf("security analysis failed for command '%s': %w", cmd.ExpandedCmd, err)
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
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"group_name": NewStringValue(groupName),
			"purpose":    NewStringValue("temporary_directory"),
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
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"path": NewStringValue(tempDirPath),
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
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"context": NewStringValue("privilege_escalation"),
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
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"message": NewStringValue(message),
			"details": NewAnyValue(details),
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
func (d *DryRunResourceManager) analyzeOutput(cmd *runnertypes.RuntimeCommand) ResourceAnalysis {
	analysis := ResourceAnalysis{
		Type:      ResourceTypeFilesystem,
		Operation: OperationCreate,
		Target:    cmd.Output(),
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"output_path":       NewStringValue(cmd.Output()),
			"command":           NewStringValue(cmd.ExpandedCmd),
			"working_directory": NewStringValue(cmd.EffectiveWorkDir),
		},
		Impact: ResourceImpact{
			Reversible:  false, // Output files are persistent
			Persistent:  true,
			Description: fmt.Sprintf("Capture command output to file: %s", cmd.Output()),
		},
		Timestamp: time.Now(),
	}

	// Use the output manager to analyze the output path
	outputAnalysis, err := d.outputManager.AnalyzeOutput(cmd.Output(), cmd.EffectiveWorkDir)
	if err != nil {
		analysis.Impact.Description += fmt.Sprintf(" [ERROR: %v]", err)
		analysis.Impact.SecurityRisk = riskLevelHigh
		return analysis // Return analysis with error info, but don't fail
	}

	// Add analysis results to parameters
	analysis.Parameters["resolved_path"] = NewStringValue(outputAnalysis.ResolvedPath)
	analysis.Parameters["directory_exists"] = NewBoolValue(outputAnalysis.DirectoryExists)
	analysis.Parameters["write_permission"] = NewBoolValue(outputAnalysis.WritePermission)
	analysis.Parameters["security_risk"] = NewStringValue(outputAnalysis.SecurityRisk.String())
	analysis.Parameters["max_size_limit"] = NewIntValue(outputAnalysis.MaxSizeLimit)

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

// SetExecutionStatus sets the execution status and phase
func (d *DryRunResourceManager) SetExecutionStatus(status ExecutionStatus, phase ExecutionPhase) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.executionStatus = status
	d.executionPhase = phase
}

// SetExecutionError sets the execution error and automatically updates status to StatusError
func (d *DryRunResourceManager) SetExecutionError(errType, message, component string, details map[string]any, phase ExecutionPhase) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.executionError = &ExecutionError{
		Type:      errType,
		Message:   message,
		Component: component,
		Details:   details,
	}
	d.executionStatus = StatusError
	d.executionPhase = phase
}

// calculateSummary calculates the execution summary from resource analyses
func (d *DryRunResourceManager) calculateSummary() *ExecutionSummary {
	summary := &ExecutionSummary{
		Groups:   &Counts{},
		Commands: &Counts{},
	}

	for i := range d.resourceAnalyses {
		analysis := &d.resourceAnalyses[i]

		// Count by type
		switch analysis.Type {
		case ResourceTypeGroup:
			summary.Groups.Total++
			// Skipped resources are counted separately, not as successful or failed
			if analysis.SkipReason != "" {
				summary.Groups.Skipped++
				summary.Skipped++
			} else {
				switch analysis.Status {
				case StatusSuccess:
					summary.Groups.Successful++
					summary.Successful++
				case StatusError:
					summary.Groups.Failed++
					summary.Failed++
				case "": // Legacy - no status set, assume success
					summary.Groups.Successful++
					summary.Successful++
				}
			}
			summary.TotalResources++

		case ResourceTypeCommand:
			summary.Commands.Total++
			// Skipped resources are counted separately, not as successful or failed
			if analysis.SkipReason != "" {
				summary.Commands.Skipped++
				summary.Skipped++
			} else {
				switch analysis.Status {
				case StatusSuccess:
					summary.Commands.Successful++
					summary.Successful++
				case StatusError:
					summary.Commands.Failed++
					summary.Failed++
				case "": // Legacy - no status set, assume success
					summary.Commands.Successful++
					summary.Successful++
				}
			}
			summary.TotalResources++
		}
	}

	return summary
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

	// Update execution status fields
	d.dryRunResult.Status = d.executionStatus
	d.dryRunResult.Phase = d.executionPhase
	d.dryRunResult.Error = d.executionError
	d.dryRunResult.Summary = d.calculateSummary()

	return d.dryRunResult
}

// RecordAnalysis records a resource analysis
func (d *DryRunResourceManager) RecordAnalysis(analysis *ResourceAnalysis) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.resourceAnalyses = append(d.resourceAnalyses, *analysis)
}

// RecordGroupAnalysis records a group-level resource analysis with debug info
func (d *DryRunResourceManager) RecordGroupAnalysis(groupName string, debugInfo *DebugInfo) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	analysis := ResourceAnalysis{
		Type:      ResourceTypeGroup,
		Operation: OperationAnalyze,
		Target:    groupName,
		Status:    StatusSuccess, // Default to success in dry-run mode
		Impact: ResourceImpact{
			Description: "Group configuration analysis",
			Reversible:  true,
			Persistent:  false,
		},
		Timestamp: time.Now(),
		Parameters: map[string]ParameterValue{
			"group_name": NewStringValue(groupName),
		},
		DebugInfo: debugInfo,
	}

	d.resourceAnalyses = append(d.resourceAnalyses, analysis)
	return nil
}

// UpdateCommandDebugInfo updates a specific command's debug info using its token
// This should be called after ExecuteCommand to add final environment information
func (d *DryRunResourceManager) UpdateCommandDebugInfo(token CommandToken, debugInfo *DebugInfo) error {
	if token == "" {
		return ErrInvalidCommandToken
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Find the command resource analysis by token
	index, ok := d.tokenToIndex[token]
	if !ok {
		return fmt.Errorf("%w: %s", ErrInvalidCommandToken, token)
	}

	// Validate index bounds
	if index < 0 || index >= len(d.resourceAnalyses) {
		return fmt.Errorf("%w: index out of bounds", ErrInvalidCommandToken)
	}

	// Verify it's a command resource
	if d.resourceAnalyses[index].Type != ResourceTypeCommand {
		return fmt.Errorf("%w: token does not refer to a command resource", ErrInvalidCommandToken)
	}

	// Defensive check: DebugInfo should always be nil at this point
	// UpdateCommandDebugInfo must be called exactly once per command token
	if d.resourceAnalyses[index].DebugInfo != nil {
		// This is a programming error - UpdateCommandDebugInfo was called multiple times
		slog.Error("UpdateCommandDebugInfo called multiple times for the same token",
			"command_index", index,
			"token", string(token),
			"has_existing_final_env", d.resourceAnalyses[index].DebugInfo.FinalEnvironment != nil,
			"has_existing_inheritance", d.resourceAnalyses[index].DebugInfo.InheritanceAnalysis != nil)
		return fmt.Errorf("%w: %s", ErrDuplicateDebugInfoUpdate, token)
	}

	// Set debug info (not merge - this is the first and only call)
	d.resourceAnalyses[index].DebugInfo = debugInfo

	return nil
}
