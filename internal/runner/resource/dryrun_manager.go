package resource

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// Static errors
var (
	ErrPathResolverRequired     = errors.New("PathResolver is required for DryRunResourceManager")
	ErrPathTraversalDetected    = errors.New("path validation failed: path traversal detected")
	ErrNoCommandAnalysis        = errors.New("no command resource analysis found to update")
	ErrInvalidCommandToken      = errors.New("invalid command token")
	ErrDuplicateDebugInfoUpdate = errors.New("UpdateCommandDebugInfo called multiple times for the same token")
)

// PathResolver interface for resolving command paths
type PathResolver interface {
	ResolvePath(command string) (string, error)
}

const (
	riskLevelHigh = "high"
)

// DryRunResourceManager implements Manager for dry-run mode
type DryRunResourceManager struct {
	// Core dependencies
	executor         executor.CommandExecutor
	privilegeManager runnertypes.PrivilegeManager
	pathResolver     PathResolver

	// Risk evaluation and audit. The same evaluator as normal mode is used so the
	// dry-run preview reproduces the runtime allow/deny decision (read-only).
	riskEvaluator                 risk.Evaluator
	auditLogger                   *audit.Logger
	failOnVerificationUnavailable bool

	// Output capture dependencies
	outputManager output.CaptureManager

	// Dry-run specific
	dryRunOptions *DryRunOptions
	dryRunResult  *DryRunResult
	// resourceAnalyses is an append-only slice that stores all resource analyses.
	// INVARIANT: Elements must never be deleted or reordered after being appended.
	// This guarantees that indices stored in tokenToIndex remain valid throughout the manager's lifetime.
	resourceAnalyses []Analysis

	// Token management - maps CommandToken to index in resourceAnalyses
	tokenToIndex map[CommandToken]int
	nextTokenID  uint64

	// Execution tracking (status, phase, error)
	executionStatus ExecutionStatus
	executionPhase  ExecutionPhase
	executionError  *ExecutionError

	// Preview decision tracking (guarded by mu): set as commands are previewed so
	// PreviewExitCode can report whether any command would be denied and whether a
	// deny was caused by verification being unavailable.
	previewPolicyDeny              bool
	previewVerificationUnavailable bool

	// State management
	mu sync.RWMutex
}

// NewDryRunResourceManager creates a new DryRunResourceManager for dry-run mode.
// evaluator and auditLogger are the same dependencies normal mode uses, so the
// preview reproduces the runtime decision and emits the same audit entries.
func NewDryRunResourceManager(exec executor.CommandExecutor, privMgr runnertypes.PrivilegeManager, pathResolver PathResolver, opts *DryRunOptions, evaluator risk.Evaluator, auditLogger *audit.Logger) (*DryRunResourceManager, error) {
	// Delegate to NewDryRunResourceManagerWithOutput with nil outputManager
	return NewDryRunResourceManagerWithOutput(exec, privMgr, pathResolver, nil, opts, evaluator, auditLogger)
}

// NewDryRunResourceManagerWithOutput creates a new DryRunResourceManager with output capture support
func NewDryRunResourceManagerWithOutput(exec executor.CommandExecutor, privMgr runnertypes.PrivilegeManager, pathResolver PathResolver, outputMgr output.CaptureManager, opts *DryRunOptions, evaluator risk.Evaluator, auditLogger *audit.Logger) (*DryRunResourceManager, error) {
	if pathResolver == nil {
		return nil, ErrPathResolverRequired
	}
	if evaluator == nil {
		return nil, ErrRiskEvaluatorRequired
	}
	if opts == nil {
		opts = &DryRunOptions{}
	}

	// Extract security analysis configuration from options
	return &DryRunResourceManager{
		executor:                      exec,
		privilegeManager:              privMgr,
		pathResolver:                  pathResolver,
		riskEvaluator:                 evaluator,
		auditLogger:                   auditLogger,
		failOnVerificationUnavailable: opts.FailOnVerificationUnavailable,
		outputManager:                 outputMgr,

		dryRunOptions: opts,
		dryRunResult: &DryRunResult{
			Metadata: &ResultMetadata{
				GeneratedAt: time.Now(),
				RunID:       fmt.Sprintf("dryrun-%d", time.Now().Unix()),
			},
			ResourceAnalyses: make([]Analysis, 0),
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
		resourceAnalyses: make([]Analysis, 0),
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
func (d *DryRunResourceManager) recordAnalysis(analysis Analysis) CommandToken {
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
func (d *DryRunResourceManager) analyzeCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (Analysis, error) {
	analysis := Analysis{
		Type:      TypeCommand,
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
		Impact: Impact{
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
	if err := d.evaluateCommandRisk(ctx, cmd, &analysis); err != nil {
		return Analysis{}, err
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

// evaluateCommandRisk runs the same risk evaluator normal mode uses to produce the
// dry-run allow/deny preview and the effective risk, so the preview reproduces the
// runtime decision. It is read-only: the evaluator opens the verified
// descriptor O_RDONLY and the plan is closed immediately (dry-run never execs).
//
// Failures split two ways: a hard error (path resolution failure,
// invalid risk_level, unexpected internal failure) returns an error and aborts the
// preview; a policy/blocking deny is not an error but a deny preview. A deny caused
// by analysis/verification being unavailable is tracked separately so PreviewExitCode
// can report it with a distinct exit code.
func (d *DryRunResourceManager) evaluateCommandRisk(ctx context.Context, cmd *runnertypes.RuntimeCommand, analysis *Analysis) error {
	// Resolve the command path. In production the group executor already resolved it;
	// resolving an absolute path again is idempotent. A resolution failure is a hard
	// error (the command does not exist), not a policy deny.
	resolvedPath, err := d.pathResolver.ResolvePath(cmd.ExpandedCmd)
	if err != nil {
		// Audit the hard-error deny before aborting so the error-return path is
		// auditable in dry-run too (the path could not be resolved, so the resolved
		// path is recorded best-effort from the command as given).
		d.emitDryRunErrorAudit(ctx, cmd, risktypes.ErrorClassPathResolution)
		return fmt.Errorf("failed to resolve command path '%s': %w. This typically occurs if the command is not found in the system PATH or there are permission issues preventing access", cmd.ExpandedCmd, err)
	}

	// Evaluate against a copy carrying the resolved path so the input is not mutated.
	prepared := *cmd
	prepared.ExpandedCmd = resolvedPath
	plan, err := d.riskEvaluator.EvaluateRisk(&prepared)
	if err != nil {
		// (3) unexpected internal error -> hard error in dry-run too.
		d.emitDryRunErrorAudit(ctx, &prepared, risktypes.ErrorClassRecordLoad)
		return fmt.Errorf("security analysis failed for command '%s': %w", cmd.ExpandedCmd, err)
	}
	defer func() {
		if closeErr := plan.Close(); closeErr != nil {
			slog.Warn("Failed to close dry-run command plan", "command", cmd.Name(), "error", closeErr)
		}
	}()

	maxAllowed, err := cmd.GetRiskLevel()
	if err != nil {
		// Invalid risk_level configuration is a hard error, not a deny preview.
		// Audit it as a deny (classified as a risk_level config error) correlated
		// with the evaluated identity, mirroring normal mode, before aborting.
		d.auditRiskDecision(ctx, &prepared, &plan, runnertypes.RiskLevelUnknown, risktypes.DecisionDeny, false, risktypes.ErrorClassRiskLevelConfig)
		return fmt.Errorf("invalid risk_level configuration for command '%s': %w", cmd.ExpandedCmd, err)
	}

	effectiveRisk := plan.Assessment.Level
	denied := plan.Assessment.Blocking || effectiveRisk > maxAllowed
	verificationUnavailable := plan.Assessment.Blocking && isVerificationUnavailable(plan.Assessment.BlockingReason)

	decision := risktypes.DecisionAllow
	if denied {
		decision = risktypes.DecisionDeny
	}

	// Annotate the analysis with the effective risk and the allow/deny verdict.
	analysis.Impact.SecurityRisk = effectiveRisk.String()
	if denied {
		analysis.Impact.Description += fmt.Sprintf(" [DENY: would be rejected (effective risk %s, max allowed %s)", effectiveRisk.String(), maxAllowed.String())
		if plan.Assessment.Blocking {
			analysis.Impact.Description += fmt.Sprintf("; blocking reason: %s", plan.Assessment.BlockingReason)
		}
		analysis.Impact.Description += "]"
		if verificationUnavailable {
			analysis.Impact.Description += " [NOTE: verification unavailable in this environment; under the fail-closed policy this command is denied here, and would be denied in any environment where verification is likewise unavailable]"
		}
	} else {
		analysis.Impact.Description += fmt.Sprintf(" [ALLOW: effective risk %s, max allowed %s]", effectiveRisk.String(), maxAllowed.String())
	}

	d.recordPreviewDecision(&prepared, plan.Assessment, denied, verificationUnavailable)
	d.auditRiskDecision(ctx, &prepared, &plan, maxAllowed, decision, verificationUnavailable, "")
	return nil
}

// isVerificationUnavailable reports whether a blocking reason is an environment
// condition (analysis or verification unavailable) rather than a genuine policy
// deny. These map to the distinct verification-unavailable exit status.
func isVerificationUnavailable(reason risktypes.ReasonCode) bool {
	switch reason {
	case risktypes.ReasonUncertainUnverifiedIdentity, risktypes.ReasonAnalysisDisabled:
		return true
	default:
		return false
	}
}

// recordPreviewDecision records a deny for the exit-code computation and, for a
// deny, appends a SecurityRisk so the formatter surfaces it. It locks once.
func (d *DryRunResourceManager) recordPreviewDecision(cmd *runnertypes.RuntimeCommand, assessment risktypes.RiskAssessment, denied, verificationUnavailable bool) {
	if !denied {
		return
	}
	description := "Command would be denied by the risk gate"
	if verificationUnavailable {
		description = "Command would be denied: verification/analysis unavailable in this environment"
	} else if assessment.Blocking {
		description = fmt.Sprintf("Command would be denied (blocking: %s)", assessment.BlockingReason)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if verificationUnavailable {
		d.previewVerificationUnavailable = true
	} else {
		d.previewPolicyDeny = true
	}
	d.dryRunResult.SecurityAnalysis.Risks = append(d.dryRunResult.SecurityAnalysis.Risks, SecurityRisk{
		Level:       assessment.Level,
		Type:        RiskTypeDangerousCommand,
		Description: description,
		Command:     cmd.ExpandedCmd,
	})
}

// PreviewExitCode returns the process exit code for the dry-run preview. A policy
// deny dominates and yields DryRunExitPolicyDeny. Otherwise a verification-unavailable
// deny yields DryRunExitVerificationUnavailable when FailOnVerificationUnavailable is
// set, or 0 (note-only) by default. It is 0 when every command is allowed.
func (d *DryRunResourceManager) PreviewExitCode() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.previewExitCodeLocked()
}

// previewExitCodeLocked computes the preview exit code. The caller must hold mu
// (read or write).
//
// A genuine policy deny always fails (DryRunExitPolicyDeny). A
// verification-unavailable deny is environment-induced ("could not verify here"):
// by default it does not fail the dry-run (DryRunExitAllow) and is reported only
// as a note, so a local/CI dry-run without the production hash database is not
// spuriously broken. FailOnVerificationUnavailable opts CI into failing on it with
// the distinct DryRunExitVerificationUnavailable code, which stays distinguishable
// from a real policy deny.
func (d *DryRunResourceManager) previewExitCodeLocked() int {
	if d.previewPolicyDeny {
		return DryRunExitPolicyDeny
	}
	if d.previewVerificationUnavailable && d.failOnVerificationUnavailable {
		return DryRunExitVerificationUnavailable
	}
	return DryRunExitAllow
}

// auditRiskDecision emits the dry-run command_risk_profile audit entry. The
// errClass override classifies a deny that is not carried on the assessment (e.g.
// an invalid risk_level configuration); when empty the plan's own ErrorClass is
// used. No-op when no audit logger is configured.
func (d *DryRunResourceManager) auditRiskDecision(ctx context.Context, cmd *runnertypes.RuntimeCommand, plan *risktypes.VerifiedCommandPlan, maxAllowed runnertypes.RiskLevel, decision risktypes.Decision, verificationUnavailable bool, errClass risktypes.ErrorClass) {
	if d.auditLogger == nil {
		return
	}
	d.auditLogger.LogRiskProfile(ctx, risktypes.RiskAuditEntry{
		CommandName:             cmd.Name(),
		Args:                    cmd.ExpandedArgs,
		Mode:                    risktypes.ModeDryRun,
		ResolvedPath:            planResolvedPath(plan),
		ContentHash:             planContentHash(plan),
		Assessment:              plan.Assessment,
		MaxAllowedRisk:          maxAllowed,
		Decision:                decision,
		VerificationUnavailable: verificationUnavailable,
		ErrorClass:              cmp.Or(errClass, plan.Assessment.ErrorClass),
		Chain:                   plan.Artifacts,
	})
}

// emitDryRunErrorAudit emits a minimal deny audit entry on the error-return path,
// where no plan is available. The resolved path is taken from the prepared command
// (already resolved by the caller) so the error deny stays correlatable. No-op when
// no audit logger is configured.
func (d *DryRunResourceManager) emitDryRunErrorAudit(ctx context.Context, cmd *runnertypes.RuntimeCommand, errClass risktypes.ErrorClass) {
	if d.auditLogger == nil {
		return
	}
	d.auditLogger.LogRiskProfile(ctx, risktypes.RiskAuditEntry{
		CommandName:  cmd.Name(),
		Args:         cmd.ExpandedArgs,
		Mode:         risktypes.ModeDryRun,
		ResolvedPath: optString(cmd.ExpandedCmd),
		Decision:     risktypes.DecisionDeny,
		ErrorClass:   errClass,
	})
}

// CreateTempDir simulates creating a temporary directory in dry-run mode
func (d *DryRunResourceManager) CreateTempDir(groupName string) (string, error) {
	simulatedPath := fmt.Sprintf("/tmp/scr-%s-XXXXXX", groupName)

	// Record the analysis
	analysis := Analysis{
		Type:      TypeFilesystem,
		Operation: OperationCreate,
		Target:    simulatedPath,
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"group_name": NewStringValue(groupName),
			"purpose":    NewStringValue("temporary_directory"),
		},
		Impact: Impact{
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
	analysis := Analysis{
		Type:      TypeFilesystem,
		Operation: OperationDelete,
		Target:    tempDirPath,
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"path": NewStringValue(tempDirPath),
		},
		Impact: Impact{
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
	analysis := Analysis{
		Type:      TypePrivilege,
		Operation: OperationEscalate,
		Target:    "system_privileges",
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"context": NewStringValue("privilege_escalation"),
		},
		Impact: Impact{
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
	analysis := Analysis{
		Type:      TypeNetwork,
		Operation: OperationSend,
		Target:    "notification_service",
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"message": NewStringValue(message),
			"details": NewAnyValue(details),
		},
		Impact: Impact{
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
func (d *DryRunResourceManager) analyzeOutput(cmd *runnertypes.RuntimeCommand) Analysis {
	analysis := Analysis{
		Type:      TypeFilesystem,
		Operation: OperationCreate,
		Target:    cmd.Output(),
		Status:    StatusSuccess, // Default to success in dry-run mode
		Parameters: map[string]ParameterValue{
			"output_path":       NewStringValue(cmd.Output()),
			"command":           NewStringValue(cmd.ExpandedCmd),
			"working_directory": NewStringValue(cmd.EffectiveWorkDir),
		},
		Impact: Impact{
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
		case TypeGroup:
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

		case TypeCommand:
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
	// This mutates d.dryRunResult (slice copy, status/phase/error/summary/exit code),
	// so it must hold the exclusive lock: a read lock would let concurrent callers
	// race on those writes. The helpers below (calculateSummary,
	// previewExitCodeLocked) are lock-free and run under this held lock.
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.dryRunResult == nil {
		return nil
	}

	// Update the resource analyses in the result
	d.dryRunResult.ResourceAnalyses = make([]Analysis, len(d.resourceAnalyses))
	copy(d.dryRunResult.ResourceAnalyses, d.resourceAnalyses)

	// Update execution status fields
	d.dryRunResult.Status = d.executionStatus
	d.dryRunResult.Phase = d.executionPhase
	d.dryRunResult.Error = d.executionError
	d.dryRunResult.Summary = d.calculateSummary()
	d.dryRunResult.PreviewExitCode = d.previewExitCodeLocked()

	return d.dryRunResult
}

// RecordAnalysis records a resource analysis
func (d *DryRunResourceManager) RecordAnalysis(analysis *Analysis) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.resourceAnalyses = append(d.resourceAnalyses, *analysis)
}

// RecordGroupAnalysis records a group-level resource analysis with debug info
func (d *DryRunResourceManager) RecordGroupAnalysis(groupName string, debugInfo *DebugInfo) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	analysis := Analysis{
		Type:      TypeGroup,
		Operation: OperationAnalyze,
		Target:    groupName,
		Status:    StatusSuccess, // Default to success in dry-run mode
		Impact: Impact{
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
	if d.resourceAnalyses[index].Type != TypeCommand {
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
