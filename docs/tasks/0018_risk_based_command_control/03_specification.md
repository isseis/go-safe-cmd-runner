# 詳細設計書: Normal Mode 統合リスクベースコマンド制御

## 1. 概要

### 1.1 目的
Normal Mode での実行時にセキュリティ分析を統合し、リスクレベルに基づく単一の制御機構でコマンド実行を安全に管理する機能の詳細設計を定義する。

### 1.2 設計方針
従来の二重制御機構（直接ブロック + リスク評価）を廃止し、統一されたリスク評価システムによる制御を実装する。特権昇格コマンド（sudo/su/doas）はCriticalリスクレベルとして分類され、設定可能な最大リスクレベル（none/low/medium/high）による統一制御で安全性を担保する。

### 1.3 設計範囲
- 統一リスク評価機能の実装詳細
- 特権昇格分析のリスク分類統合
- Normal Manager への統合方法
- 設定ファイル拡張の詳細
- 統一エラーハンドリングの実装

## 2. 統合リスクベースコマンド制御の詳細設計

### 2.1 Risk Evaluation Package の実装

```go
// internal/runner/risk/evaluator.go

// RiskEvaluator evaluates the security risk of commands using unified approach
type RiskEvaluator interface {
    EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error)
}

// StandardEvaluator implements unified risk evaluation including privilege escalation
type StandardEvaluator struct {
    logger *slog.Logger
}

// EvaluateRisk analyzes a command and returns its risk level using unified classification
func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error) {
    // Check for privilege escalation commands (automatic Critical risk classification)
    isPrivEsc, err := security.IsPrivilegeEscalationCommand(cmd.Cmd)
    if err != nil {
        return runnertypes.RiskLevelUnknown, err
    }
    if isPrivEsc {
        // Unified approach: privilege escalation commands are classified as Critical
        // and controlled by max_risk_level configuration (which cannot be set to "critical")
        return runnertypes.RiskLevelCritical, nil
    }

    // Check for destructive file operations
    if security.IsDestructiveFileOperation(cmd.Cmd, cmd.Args) {
        return runnertypes.RiskLevelHigh, nil
    }

    // Check for network operations
    isNetwork, isHighRisk := security.IsNetworkOperation(cmd.Cmd, cmd.Args)
    if isHighRisk {
        return runnertypes.RiskLevelHigh, nil
    }
    if isNetwork {
        return runnertypes.RiskLevelMedium, nil
    }

    // Check for system modification commands
    if security.IsSystemModification(cmd.Cmd, cmd.Args) {
        return runnertypes.RiskLevelMedium, nil
    }

    // Default to low risk for safe commands
    return runnertypes.RiskLevelLow, nil
}
```

### 2.2 Risk Level Classification System

```go
// internal/runner/runnertypes/config.go

type RiskLevel int

const (
    // RiskLevelUnknown indicates commands whose risk level cannot be determined
    RiskLevelUnknown RiskLevel = iota
    // RiskLevelLow indicates commands with minimal security risk
    RiskLevelLow
    // RiskLevelMedium indicates commands with moderate security risk
    RiskLevelMedium
    // RiskLevelHigh indicates commands with high security risk
    RiskLevelHigh
    // RiskLevelCritical indicates commands that should be blocked (e.g., privilege escalation)
    // NOTE: This level is for internal classification only and cannot be set in configuration
    RiskLevelCritical
)
```

### 2.3 Enhanced Privilege Management

```go
// internal/runner/privilege/unix.go

// WithUserGroup executes a function with specified user and group privileges
func (m *UnixPrivilegeManager) WithUserGroup(user, group string, fn func() error) error {
    elevationCtx := runnertypes.ElevationContext{
        Operation:  runnertypes.OperationUserGroupExecution,
        RunAsUser:  user,
        RunAsGroup: group,
    }
    return m.WithPrivileges(elevationCtx, fn)
}

// IsUserGroupSupported checks if user/group privilege changes are supported
func (m *UnixPrivilegeManager) IsUserGroupSupported() bool {
    return m.privilegeSupported
}
```
### 2.4 Security Analysis Integration

```go
// internal/runner/security/command_analysis.go - Enhanced Functions

// IsPrivilegeEscalationCommand checks if the given command is a privilege escalation command
func IsPrivilegeEscalationCommand(cmdName string) (bool, error) {
    commandNames, exceededDepth := extractAllCommandNames(cmdName)
    if exceededDepth {
        return false, ErrSymlinkDepthExceeded
    }

    // Check for any privilege escalation commands
    for _, cmd := range privilegeCommands {
        if _, exists := commandNames[cmd]; exists {
            return true, nil
        }
    }
    return false, nil
}

// IsDestructiveFileOperation checks if the command performs destructive file operations
func IsDestructiveFileOperation(cmdName string, args []string) bool {
    // Implementation leverages existing security analysis
    destructiveCommands := map[string]bool{
        "rm": true, "rmdir": true, "unlink": true, "shred": true, "dd": true,
    }
    return destructiveCommands[cmdName] || hasDestructiveArgs(args)
}

// IsNetworkOperation checks if the command performs network operations
func IsNetworkOperation(cmdName string, args []string) (bool, bool) {
    // Returns (isNetwork, isHighRisk)
    // Implementation includes smart detection for git/rsync operations
}

// IsSystemModification checks if the command modifies system settings
func IsSystemModification(cmdName string, args []string) bool {
    // Covers package managers, service management, system configuration
}

// initializePatterns sets up the privilege escalation patterns
func (a *DefaultPrivilegeEscalationAnalyzer) initializePatterns() {
    a.privilegePatterns = map[string]*PrivilegePattern{
        "sudo": {
            Type:        PrivilegeEscalationSudo,
            Pattern:     regexp.MustCompile(`^sudo$`),
            RiskLevel:   RiskLevelHigh,
            Description: "Execute command with elevated privileges using sudo",
            Validator:   a.validateSudoArgs,
        },
        "su": {
            Type:        PrivilegeEscalationSu,
            Pattern:     regexp.MustCompile(`^su$`),
            RiskLevel:   RiskLevelHigh,
            Description: "Switch user context",
            Validator:   a.validateSuArgs,
        },
        "doas": {
            Type:        PrivilegeEscalationSudo, // Use same type as sudo
            Pattern:     regexp.MustCompile(`^doas$`),
            RiskLevel:   RiskLevelHigh,
            Description: "Execute command with elevated privileges using doas",
            Validator:   a.validateDoasArgs,
        },
        "systemctl": {
            Type:        PrivilegeEscalationSystemd,
            Pattern:     regexp.MustCompile(`^systemctl$`),
            RiskLevel:   RiskLevelMedium,
            Description: "System service management",
            Validator:   a.validateSystemctlArgs,
        },
        "service": {
            Type:        PrivilegeEscalationService,
            Pattern:     regexp.MustCompile(`^service$`),
            RiskLevel:   RiskLevelMedium,
            Description: "Service management command",
            Validator:   a.validateServiceArgs,
        },
        "chmod": {
            Type:        PrivilegeEscalationChmod,
            Pattern:     regexp.MustCompile(`^chmod$`),
            RiskLevel:   RiskLevelMedium,
            Description: "Change file permissions",
            Validator:   a.validateChmodArgs,
        },
        "chown": {
            Type:        PrivilegeEscalationChown,
            Pattern:     regexp.MustCompile(`^chown$`),
            RiskLevel:   RiskLevelMedium,
            Description: "Change file ownership",
            Validator:   a.validateChownArgs,
        },
    }
}

// initializeSystemCommands sets up inherently privileged commands
func (a *DefaultPrivilegeEscalationAnalyzer) initializeSystemCommands() {
    a.systemCommands = map[string]bool{
        "mount":   true,
        "umount":  true,
        "fdisk":   true,
        "parted":  true,
        "mkfs":    true,
        "fsck":    true,
        "iptables": true,
        "ufw":     true,
        "firewall-cmd": true,
    }
}

// AnalyzePrivilegeEscalation analyzes a command for privilege escalation
func (a *DefaultPrivilegeEscalationAnalyzer) AnalyzePrivilegeEscalation(
    ctx context.Context,
    cmdName string,
    args []string,
) (*PrivilegeEscalationResult, error) {
    if ctx == nil {
        return nil, fmt.Errorf("context cannot be nil")
    }

    result := &PrivilegeEscalationResult{
        Context: make(map[string]interface{}),
    }

    // Check for privilege escalation patterns
    if pattern, exists := a.privilegePatterns[cmdName]; exists {
        result.HasPrivilegeEscalation = true
        result.EscalationType = pattern.Type
        result.RiskLevel = pattern.RiskLevel
        result.Description = pattern.Description
        result.DetectedPatterns = []string{cmdName}

        // Validate arguments if validator exists
        if pattern.Validator != nil && !pattern.Validator(args) {
            result.Context["validation_failed"] = true
            result.RiskLevel = RiskLevelHigh
        }

        // Get required privileges
        privileges, err := a.GetRequiredPrivileges(cmdName, args)
        if err != nil {
            return nil, fmt.Errorf("failed to get required privileges: %w", err)
        }
        result.RequiredPrivileges = privileges

        return result, nil
    }

    // Check for inherently privileged system commands
    if a.systemCommands[cmdName] {
        result.HasPrivilegeEscalation = true
        result.EscalationType = PrivilegeEscalationOther
        result.RiskLevel = RiskLevelHigh
        result.Description = fmt.Sprintf("Inherently privileged system command: %s", cmdName)
        result.DetectedPatterns = []string{cmdName}
        result.RequiredPrivileges = []string{"root"}

        return result, nil
    }

    // No privilege escalation detected
    result.HasPrivilegeEscalation = false
    result.EscalationType = PrivilegeEscalationNone
    result.RiskLevel = RiskLevelNone
    result.Description = "No privilege escalation detected"

    return result, nil
}

// Validator functions for specific commands
func (a *DefaultPrivilegeEscalationAnalyzer) validateSudoArgs(args []string) bool {
    // Validate sudo arguments - check for dangerous combinations
    if len(args) == 0 {
        return false // sudo without arguments is suspicious
    }

    // Check for shell escapes or dangerous patterns
    for _, arg := range args {
        if strings.Contains(arg, ";") || strings.Contains(arg, "|") || strings.Contains(arg, "&") {
            return false
        }
    }

    return true
}

func (a *DefaultPrivilegeEscalationAnalyzer) validateDoasArgs(args []string) bool {
    // Validate doas arguments - similar to sudo validation
    if len(args) == 0 {
        return false // doas without arguments is suspicious
    }

    // Check for shell escapes or dangerous patterns
    for _, arg := range args {
        if strings.Contains(arg, ";") || strings.Contains(arg, "|") || strings.Contains(arg, "&") {
            return false
        }
    }

    return true
}

func (a *DefaultPrivilegeEscalationAnalyzer) validateSystemctlArgs(args []string) bool {
    if len(args) == 0 {
        return true
    }

    // Check for dangerous systemctl operations
    dangerousOps := map[string]bool{
        "daemon-reload": true,
        "start":        true,
        "stop":         true,
        "restart":      true,
        "enable":       true,
        "disable":      true,
    }

    return !dangerousOps[args[0]]
}

func (a *DefaultPrivilegeEscalationAnalyzer) validateChmodArgs(args []string) bool {
    if len(args) < 2 {
        return false
    }

    // Check for dangerous chmod patterns (setuid/setgid)
    mode := args[0]
    if strings.Contains(mode, "4") || strings.Contains(mode, "2") {
        // Setuid or setgid bit detected
        return false
    }

    return true
}

// Additional validation functions...
```

### 2.3 テストケース設計

```go
// internal/runner/security/privilege_test.go
func TestPrivilegeEscalationAnalyzer(t *testing.T) {
    tests := []struct {
        name                    string
        cmdName                 string
        args                    []string
        expectedHasEscalation   bool
        expectedType           PrivilegeEscalationType
        expectedRiskLevel      RiskLevel
        expectedError          error
    }{
        {
            name:                  "sudo_command_detected",
            cmdName:               "sudo",
            args:                  []string{"ls", "-la"},
            expectedHasEscalation: true,
            expectedType:          PrivilegeEscalationSudo,
            expectedRiskLevel:     RiskLevelHigh,
        },
        {
            name:                  "su_command_detected",
            cmdName:               "su",
            args:                  []string{"-", "root"},
            expectedHasEscalation: true,
            expectedType:          PrivilegeEscalationSu,
            expectedRiskLevel:     RiskLevelHigh,
        },
        {
            name:                  "doas_command_detected",
            cmdName:               "doas",
            args:                  []string{"ls", "-la"},
            expectedHasEscalation: true,
            expectedType:          PrivilegeEscalationSudo, // Same type as sudo
            expectedRiskLevel:     RiskLevelHigh,
        },
        {
            name:                  "systemctl_dangerous_operation",
            cmdName:               "systemctl",
            args:                  []string{"restart", "nginx"},
            expectedHasEscalation: true,
            expectedType:          PrivilegeEscalationSystemd,
            expectedRiskLevel:     RiskLevelHigh, // Elevated due to validation failure
        },
        {
            name:                  "normal_command_no_escalation",
            cmdName:               "ls",
            args:                  []string{"-la"},
            expectedHasEscalation: false,
            expectedType:          PrivilegeEscalationNone,
            expectedRiskLevel:     RiskLevelNone,
        },
    }

    analyzer := NewDefaultPrivilegeEscalationAnalyzer()
    ctx := context.Background()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := analyzer.AnalyzePrivilegeEscalation(ctx, tt.cmdName, tt.args)

            if tt.expectedError != nil {
                assert.Error(t, err)
                assert.True(t, errors.Is(err, tt.expectedError))
                return
            }

            assert.NoError(t, err)
            assert.Equal(t, tt.expectedHasEscalation, result.HasPrivilegeEscalation)
            assert.Equal(t, tt.expectedType, result.EscalationType)
            assert.Equal(t, tt.expectedRiskLevel, result.RiskLevel)
        })
    }
}
```

## 3. Risk Evaluator 詳細設計

### 3.1 拡張インターフェース

```go
// internal/runner/security/evaluator.go
package security

// EnhancedRiskEvaluator extends the basic risk evaluation with privilege analysis
type EnhancedRiskEvaluator interface {
    // EvaluateCommandExecution evaluates if a command should be executed based on risk
    EvaluateCommandExecution(
        ctx context.Context,
        riskLevel RiskLevel,
        detectedPattern string,
        reason string,
        privilegeResult *PrivilegeEscalationResult,
        command *config.Command,
    ) error

    // CalculateEffectiveRisk calculates the effective risk level considering privilege flags
    CalculateEffectiveRisk(
        baseRisk RiskLevel,
        privilegeResult *PrivilegeEscalationResult,
        command *config.Command,
    ) (RiskLevel, error)

    // IsPrivilegeEscalationAllowed checks if privilege escalation is allowed for a command
    IsPrivilegeEscalationAllowed(command *config.Command) bool
}

// DefaultEnhancedRiskEvaluator provides the default implementation
type DefaultEnhancedRiskEvaluator struct {
    logger Logger
}

// NewDefaultEnhancedRiskEvaluator creates a new enhanced risk evaluator
func NewDefaultEnhancedRiskEvaluator(logger Logger) *DefaultEnhancedRiskEvaluator {
    return &DefaultEnhancedRiskEvaluator{
        logger: logger,
    }
}
```

### 3.2 実装詳細

```go
// EvaluateCommandExecution evaluates command execution based on comprehensive risk analysis
func (e *DefaultEnhancedRiskEvaluator) EvaluateCommandExecution(
    ctx context.Context,
    baseRiskLevel RiskLevel,
    detectedPattern string,
    reason string,
    privilegeResult *PrivilegeEscalationResult,
    command *config.Command,
) error {
    if ctx == nil {
        return fmt.Errorf("context cannot be nil")
    }

    if command == nil {
        return fmt.Errorf("command cannot be nil")
    }

    // Calculate effective risk level including privilege escalation analysis
    effectiveRisk, err := e.CalculateEffectiveRisk(baseRiskLevel, privilegeResult, command)
    if err != nil {
        return fmt.Errorf("failed to calculate effective risk: %w", err)
    }

    // Get maximum allowed risk level for this command
    maxRiskLevel, err := e.getMaxAllowedRiskLevel(command)
    if err != nil {
        return fmt.Errorf("failed to get max allowed risk level: %w", err)
    }

    // Check if effective risk exceeds allowed level (unified control)
    if effectiveRisk > maxRiskLevel {
        return e.createSecurityViolationError(
            command,
            effectiveRisk,
            maxRiskLevel,
            detectedPattern,
            reason,
            privilegeResult,
        )
    }

    // Log the successful risk evaluation
    e.logger.Info("Command risk evaluation passed",
        "command", command.Name,
        "effective_risk", effectiveRisk.String(),
        "max_allowed_risk", maxRiskLevel.String(),
        "has_privilege_escalation", privilegeResult != nil && privilegeResult.HasPrivilegeEscalation,
    )

    return nil
}

// CalculateEffectiveRisk calculates the effective risk considering privilege escalation
func (e *DefaultEnhancedRiskEvaluator) CalculateEffectiveRisk(
    baseRisk RiskLevel,
    privilegeResult *PrivilegeEscalationResult,
    command *config.Command,
) (RiskLevel, error) {
    // If no privilege escalation detected, return base risk
    if privilegeResult == nil || !privilegeResult.HasPrivilegeEscalation {
        return baseRisk, nil
    }

    // Privilege escalation detected - classify as Critical risk unless explicitly allowed
    if !command.Privileged {
        // sudo/su/doas commands without explicit privilege flag are Critical risk
        return RiskLevelCritical, nil
    }

    // For privileged commands, calculate risk without privilege escalation component
    return e.calculateNonPrivilegeRisk(baseRisk, privilegeResult)
}

// calculateNonPrivilegeRisk calculates risk excluding privilege escalation components
func (e *DefaultEnhancedRiskEvaluator) calculateNonPrivilegeRisk(
    baseRisk RiskLevel,
    privilegeResult *PrivilegeEscalationResult,
) (RiskLevel, error) {
    // If the base risk is entirely due to privilege escalation, return None
    if baseRisk == privilegeResult.RiskLevel {
        return RiskLevelNone, nil
    }

    // If base risk is higher than privilege escalation risk,
    // there are other security concerns
    if baseRisk > privilegeResult.RiskLevel {
        return baseRisk, nil
    }

    // Conservative approach: return lower risk but not None
    if baseRisk > RiskLevelNone {
        return RiskLevelLow, nil
    }

    return RiskLevelNone, nil
}

// combineRisks combines multiple risk levels to get overall risk
func (e *DefaultEnhancedRiskEvaluator) combineRisks(risk1, risk2 RiskLevel) RiskLevel {
    if risk1 > risk2 {
        return risk1
    }
    return risk2
}
        Command:         command.Cmd,
        DetectedCommand: privilegeResult.DetectedPatterns[0],
        Reason:          "Privilege escalation commands (sudo, su, doas) are prohibited in TOML files",
        Alternative:     "Use 'run_as_user'/'run_as_group' setting for safe privilege escalation",
        CommandPath:     fmt.Sprintf("groups.%s.commands.%s", command.GroupName, command.Name),
        RunID:           command.RunID,
    }
}

// getMaxAllowedRiskLevel determines the maximum allowed risk level for a command
func (e *DefaultEnhancedRiskEvaluator) getMaxAllowedRiskLevel(command *config.Command) (RiskLevel, error) {
    if command.MaxRiskLevel == "" {
        return RiskLevelNone, nil // Default: only allow no-risk commands
    }

    level, err := ParseRiskLevel(command.MaxRiskLevel)
    if err != nil {
        return RiskLevelNone, fmt.Errorf("invalid max_risk_level '%s': %w", command.MaxRiskLevel, err)
    }

    return level, nil
}

// createSecurityViolationError creates a detailed security violation error
func (e *DefaultEnhancedRiskEvaluator) createSecurityViolationError(
    command *config.Command,
    effectiveRisk RiskLevel,
    maxAllowedRisk RiskLevel,
    detectedPattern string,
    reason string,
    privilegeResult *PrivilegeEscalationResult,
) error {
    violation := &SecurityViolationError{
        Command:         fmt.Sprintf("%s %s", command.Cmd, strings.Join(command.Args, " ")),
        DetectedRisk:    effectiveRisk.String(),
        MaxAllowedRisk:  maxAllowedRisk.String(),
        DetectedPattern: detectedPattern,
        Reason:          reason,
        CommandPath:     command.Name,
    }

    // Add privilege escalation details if applicable
    if privilegeResult != nil && privilegeResult.HasPrivilegeEscalation {
        violation.PrivilegeEscalation = &PrivilegeEscalationDetails{
            Type:               privilegeResult.EscalationType.String(),
            RequiredPrivileges: privilegeResult.RequiredPrivileges,
            Description:        privilegeResult.Description,
        }

        // Suggest run_as_user/run_as_group if not set
        if command.RunAsUser == "" && command.RunAsGroup == "" {
            violation.Suggestion = fmt.Sprintf(
                "Consider setting 'run_as_user'/'run_as_group' in the command configuration if this privilege escalation is intended, or set 'max_risk_level = \"%s\"' to allow this risk level",
                effectiveRisk.String(),
            )
        }
    }

    return violation
}
```

### 3.3 エラータイプ拡張

```go
// SecurityViolationError represents a security policy violation
type SecurityViolationError struct {
    Command             string
    DetectedRisk        string
    MaxAllowedRisk      string
    DetectedPattern     string
    Reason              string
    CommandPath         string
    RunID               string
    PrivilegeEscalation *PrivilegeEscalationDetails
    Suggestion          string
}

// PrivilegeEscalationDetails provides details about privilege escalation
type PrivilegeEscalationDetails struct {
    Type               string
    RequiredPrivileges []string
    Description        string
}

// Error implements the error interface for SecurityViolationError
func (e *SecurityViolationError) Error() string {
    var parts []string

    parts = append(parts, fmt.Sprintf("security violation: command '%s' has risk level '%s' but maximum allowed is '%s'",
        e.Command, e.DetectedRisk, e.MaxAllowedRisk))

    if e.DetectedPattern != "" {
        parts = append(parts, fmt.Sprintf("detected pattern: %s", e.DetectedPattern))
    }

    if e.PrivilegeEscalation != nil {
        parts = append(parts, fmt.Sprintf("privilege escalation: %s (%s)",
            e.PrivilegeEscalation.Type, e.PrivilegeEscalation.Description))
    }

    if e.Suggestion != "" {
        parts = append(parts, fmt.Sprintf("suggestion: %s", e.Suggestion))
    }

    return strings.Join(parts, "; ")
}

// Is implements error equality checking for SecurityViolationError
func (e *SecurityViolationError) Is(target error) bool {
    _, ok := target.(*SecurityViolationError)
    return ok
}
```

## 4. Normal Manager 統合詳細設計

### 4.1 拡張構造体

```go
// internal/runner/resource/normal_manager.go
type NormalResourceManager struct {
    executor             CommandExecutor
    outputWriter         OutputWriter
    securityAnalyzer     SecurityAnalyzer              // EXISTING
    riskEvaluator        EnhancedRiskEvaluator          // NEW
    privilegeAnalyzer    PrivilegeEscalationAnalyzer    // NEW
    logger               Logger
}

// NewNormalResourceManager creates a new enhanced normal resource manager
func NewNormalResourceManager(
    executor CommandExecutor,
    outputWriter OutputWriter,
    securityAnalyzer SecurityAnalyzer,
    riskEvaluator EnhancedRiskEvaluator,
    privilegeAnalyzer PrivilegeEscalationAnalyzer,
    logger Logger,
) *NormalResourceManager {
    return &NormalResourceManager{
        executor:          executor,
        outputWriter:      outputWriter,
        securityAnalyzer:  securityAnalyzer,
        riskEvaluator:     riskEvaluator,
        privilegeAnalyzer: privilegeAnalyzer,
        logger:            logger,
    }
}
```

### 4.2 統合実行フロー

```go
// ExecuteCommand executes a command with comprehensive security analysis
func (m *NormalResourceManager) ExecuteCommand(
    ctx context.Context,
    command *config.Command,
    env map[string]string,
) (*ExecutionResult, error) {
    if ctx == nil {
        return nil, fmt.Errorf("context cannot be nil")
    }

    if command == nil {
        return nil, fmt.Errorf("command cannot be nil")
    }

    // Log command execution start
    m.logger.Info("Starting command execution with security analysis",
        "command", command.Name,
        "cmd", command.Cmd,
        "args", command.Args,
    )

    // 1. Basic Security Analysis (EXISTING, enhanced logging)
    riskLevel, detectedPattern, reason, err := m.securityAnalyzer.AnalyzeCommandSecurity(
        command.Cmd, command.Args)
    if err != nil {
        m.logger.Error("Security analysis failed", "error", err, "command", command.Name)
        return nil, fmt.Errorf("security analysis failed: %w", err)
    }

    m.logger.Debug("Basic security analysis completed",
        "command", command.Name,
        "risk_level", riskLevel.String(),
        "detected_pattern", detectedPattern,
        "reason", reason,
    )

    // 2. Privilege Escalation Analysis (NEW)
    privilegeResult, err := m.privilegeAnalyzer.AnalyzePrivilegeEscalation(
        ctx, command.Cmd, command.Args)
    if err != nil {
        m.logger.Error("Privilege escalation analysis failed", "error", err, "command", command.Name)
        return nil, fmt.Errorf("privilege escalation analysis failed: %w", err)
    }

    m.logger.Debug("Privilege escalation analysis completed",
        "command", command.Name,
        "has_privilege_escalation", privilegeResult.HasPrivilegeEscalation,
        "escalation_type", privilegeResult.EscalationType.String(),
        "escalation_risk", privilegeResult.RiskLevel.String(),
    )

    // 3. Comprehensive Risk Evaluation (NEW)
    if err := m.riskEvaluator.EvaluateCommandExecution(
        ctx, riskLevel, detectedPattern, reason, privilegeResult, command); err != nil {

        m.logger.Error("Command execution denied due to security policy violation",
            "error", err,
            "command", command.Name,
            "risk_level", riskLevel.String(),
            "privilege_escalation", privilegeResult.HasPrivilegeEscalation,
        )

        return nil, err
    }

    m.logger.Info("Security evaluation passed, proceeding with command execution",
        "command", command.Name)

    // 4. Execute Command (EXISTING)
    result, err := m.executor.Execute(ctx, command, env)
    if err != nil {
        m.logger.Error("Command execution failed", "error", err, "command", command.Name)
        return nil, fmt.Errorf("command execution failed: %w", err)
    }

    m.logger.Info("Command executed successfully",
        "command", command.Name,
        "exit_code", result.ExitCode,
    )

    return result, nil
}
```

## 5. 設定拡張詳細設計

### 5.1 Command 構造体拡張

```go
// internal/runner/config/command.go
type Command struct {
    Name         string   `toml:"name"`
    Description  string   `toml:"description"`
    Cmd          string   `toml:"cmd"`
    Args         []string `toml:"args"`

    // Security Configuration (NEW)
    MaxRiskLevel string   `toml:"max_risk_level"` // "none", "low", "medium", "high" (NOT "critical")

    // Privilege Configuration (EXISTING, enhanced validation)
    Privileged   bool     `toml:"privileged"`

    // ... existing fields ...
    Env          map[string]string `toml:"env"`
    WorkingDir   string            `toml:"working_dir"`
    Timeout      Duration          `toml:"timeout"`
}

// ValidateSecurityConfig validates the security-related configuration
func (c *Command) ValidateSecurityConfig() error {
    var errors []string

    // Validate MaxRiskLevel
    if c.MaxRiskLevel != "" {
        // Explicitly reject "critical" setting
        if c.MaxRiskLevel == "critical" {
            errors = append(errors, "max_risk_level='critical' is not allowed (use run_as_user/run_as_group for privilege escalation)")
        } else if _, err := ParseRiskLevel(c.MaxRiskLevel); err != nil {
            errors = append(errors, fmt.Sprintf("invalid max_risk_level '%s': %v", c.MaxRiskLevel, err))
        }
    }

    // Validate run_as_user/run_as_group with MaxRiskLevel
    if (c.RunAsUser != "" || c.RunAsGroup != "") && c.MaxRiskLevel == "none" {
        errors = append(errors, "run_as_user/run_as_group conflicts with max_risk_level='none'")
    }

    if len(errors) > 0 {
        return fmt.Errorf("security configuration validation failed: %s", strings.Join(errors, "; ")))
    }

    return nil
}

// GetEffectiveMaxRiskLevel returns the effective maximum risk level
func (c *Command) GetEffectiveMaxRiskLevel() RiskLevel {
    if c.MaxRiskLevel == "" {
        return RiskLevelNone // Default: most restrictive
    }

    level, err := ParseRiskLevel(c.MaxRiskLevel)
    if err != nil {
        // Should not happen if validation was performed
        return RiskLevelNone
    }

    return level
}
```

### 5.2 設定ファイル例

```toml
# sample/enhanced_security_test.toml
[meta]
title = "Enhanced Security Configuration Test"
version = "1.0.0"

[[groups]]
name = "secure_operations"
description = "Operations with various security levels"

  [[groups.commands]]
  name = "safe_list"
  description = "Safe file listing"
  cmd = "ls"
  args = ["-la", "/tmp"]
  # max_risk_level not set - defaults to "none"

  [[groups.commands]]
  name = "medium_risk_cleanup"
  description = "Cleanup with medium risk tolerance"
  cmd = "rm"
  args = ["-rf", "/tmp/safe_to_delete"]
  max_risk_level = "medium"

  [[groups.commands]]
  name = "high_risk_with_privilege"
  description = "System operation requiring privileges"
  cmd = "systemctl"
  args = ["restart", "nginx"]
  max_risk_level = "high"
  run_as_user = "root"

  # ❌ This configuration is PROHIBITED and will always fail
  # [[groups.commands]]
  # name = "prohibited_sudo"
  # description = "Sudo commands are prohibited in TOML files"
  # cmd = "sudo"
  # args = ["systemctl", "status", "nginx"]
  # max_risk_level = "high"  # This setting has no effect for sudo/su/doas
  # run_as_user = "root"        # This setting has no effect for sudo/su/doas

  # ✅ RECOMMENDED: Use run_as_user for safe privilege escalation
  [[groups.commands]]
  name = "safe_privileged_operation"
  description = "Privileged operation using safe mechanism"
  cmd = "systemctl"
  args = ["status", "nginx"]
  run_as_user = "root"
  max_risk_level = "medium"

  [[groups.commands]]
  name = "safe_privileged_cleanup"
  description = "Privileged cleanup using safe mechanism"
  cmd = "rm"
  args = ["-rf", "/tmp/app_temp_files"]
  run_as_user = "root"
  max_risk_level = "high"
```

## 6. エラーハンドリング詳細設計

### 6.1 エラー階層

```go
// internal/runner/security/errors.go
package security

// SecurityError represents the base type for all security-related errors
type SecurityError interface {
    error
    SecurityErrorType() string
    SecurityContext() map[string]interface{}
}

// BaseSecurityError provides common functionality for security errors
type BaseSecurityError struct {
    ErrorType string
    Context   map[string]interface{}
    Cause     error
}

func (e *BaseSecurityError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %v", e.ErrorType, e.Cause)
    }
    return e.ErrorType
}

func (e *BaseSecurityError) SecurityErrorType() string {
    return e.ErrorType
}

func (e *BaseSecurityError) SecurityContext() map[string]interface{} {
    return e.Context
}

func (e *BaseSecurityError) Unwrap() error {
    return e.Cause
}

// Specific error types
const (
    ErrorTypeAnalysisFailed    = "security_analysis_failed"
    ErrorTypeRiskTooHigh      = "security_risk_too_high"
    ErrorTypeConfigInvalid    = "security_config_invalid"
    ErrorTypePrivilegeViolation = "privilege_escalation_violation"
)

// AnalysisFailedError represents a security analysis failure
type AnalysisFailedError struct {
    *BaseSecurityError
    Command     string
    AnalysisType string
}

func NewAnalysisFailedError(command, analysisType string, cause error) *AnalysisFailedError {
    return &AnalysisFailedError{
        BaseSecurityError: &BaseSecurityError{
            ErrorType: ErrorTypeAnalysisFailed,
            Context: map[string]interface{}{
                "command":       command,
                "analysis_type": analysisType,
            },
            Cause: cause,
        },
        Command:     command,
        AnalysisType: analysisType,
    }
}

// ConfigInvalidError represents invalid security configuration
type ConfigInvalidError struct {
    *BaseSecurityError
    ConfigField string
    ConfigValue string
}

func NewConfigInvalidError(field, value string, cause error) *ConfigInvalidError {
    return &ConfigInvalidError{
        BaseSecurityError: &BaseSecurityError{
            ErrorType: ErrorTypeConfigInvalid,
            Context: map[string]interface{}{
                "config_field": field,
                "config_value": value,
            },
            Cause: cause,
        },
        ConfigField: field,
        ConfigValue: value,
    }
}
```

### 6.2 エラー処理フロー

```go
// internal/runner/resource/error_handler.go
package resource

// SecurityErrorHandler handles security-related errors
type SecurityErrorHandler struct {
    logger Logger
}

// HandleSecurityError processes security errors and returns appropriate user-facing errors
func (h *SecurityErrorHandler) HandleSecurityError(err error, command *config.Command) error {
    if err == nil {
        return nil
    }

    // Log the original error for debugging
    h.logger.Debug("Handling security error", "error", err, "command", command.Name)

    var secErr security.SecurityError
    if errors.As(err, &secErr) {
        return h.handleTypedSecurityError(secErr, command)
    }

    // Handle wrapped errors
    if errors.Is(err, security.ErrRiskTooHigh) {
        return h.createUserFriendlyRiskError(err, command)
    }

    // Generic security error
    return fmt.Errorf("security check failed for command '%s': %w", command.Name, err)
}

// handleTypedSecurityError handles typed security errors
func (h *SecurityErrorHandler) handleTypedSecurityError(secErr security.SecurityError, command *config.Command) error {
    context := secErr.SecurityContext()

    switch secErr.SecurityErrorType() {
    case security.ErrorTypeRiskTooHigh:
        return h.createRiskViolationError(secErr, command, context)

    case security.ErrorTypePrivilegeViolation:
        return h.createPrivilegeViolationError(secErr, command, context)

    case security.ErrorTypeAnalysisFailed:
        return h.createAnalysisFailedError(secErr, command, context)

    case security.ErrorTypeConfigInvalid:
        return h.createConfigInvalidError(secErr, command, context)

    default:
        return fmt.Errorf("unknown security error for command '%s': %w", command.Name, secErr)
    }
}

// createRiskViolationError creates a user-friendly risk violation error
func (h *SecurityErrorHandler) createRiskViolationError(
    secErr security.SecurityError,
    command *config.Command,
    context map[string]interface{},
) error {
    message := fmt.Sprintf("Command '%s' exceeds maximum allowed risk level", command.Name)

    if detectedRisk, ok := context["detected_risk"]; ok {
        if maxAllowed, ok := context["max_allowed_risk"]; ok {
            message = fmt.Sprintf("%s (detected: %v, max allowed: %v)",
                message, detectedRisk, maxAllowed)
        }
    }

    if pattern, ok := context["detected_pattern"]; ok {
        message = fmt.Sprintf("%s - Pattern: %v", message, pattern)
    }

    if suggestion, ok := context["suggestion"]; ok {
        message = fmt.Sprintf("%s\nSuggestion: %v", message, suggestion)
    }

    return fmt.Errorf("%s", message)
}
```

## 7. テスト設計詳細

### 7.1 統合テスト

```go
// internal/runner/resource/normal_manager_integration_test.go
func TestNormalManagerSecurityIntegration(t *testing.T) {
    tests := []struct {
        name           string
        command        *config.Command
        expectedError  error
        shouldExecute  bool
        setupMocks     func(*testing.T) (*NormalResourceManager, *MockExecutor)
    }{
        {
            name: "safe_command_executes",
            command: &config.Command{
                Name: "safe_ls",
                Cmd:  "ls",
                Args: []string{"-la"},
                // No max_risk_level set - defaults to "none"
            },
            shouldExecute: true,
            expectedError: nil,
        },
        {
            name: "high_risk_command_blocked",
            command: &config.Command{
                Name: "dangerous_rm",
                Cmd:  "rm",
                Args: []string{"-rf", "/"},
                // No max_risk_level set - defaults to "none"
            },
            shouldExecute: false,
            expectedError: &security.SecurityViolationError{},
        },
        {
            name: "sudo_command_blocked_by_critical_risk",
            command: &config.Command{
                Name:         "sudo_operation",
                Cmd:          "sudo",
                Args:         []string{"systemctl", "status", "nginx"},
                MaxRiskLevel: "high",     // Allows up to High risk
                Privileged:   false,     // No explicit privilege
                // sudo commands are classified as Critical risk and blocked
            },
            shouldExecute: false,
            expectedError: &security.SecurityViolationError{},
        },
        {
            name: "su_command_blocked_by_critical_risk",
            command: &config.Command{
                Name:         "su_operation",
                Cmd:          "su",
                Args:         []string{"-", "root"},
                MaxRiskLevel: "high",     // Allows up to High risk
                Privileged:   false,     // No explicit privilege
                // su commands are classified as Critical risk and blocked
            },
            shouldExecute: false,
            expectedError: &security.SecurityViolationError{},
        },
        {
            name: "doas_command_blocked_by_critical_risk",
            command: &config.Command{
                Name:         "doas_operation",
                Cmd:          "doas",
                Args:         []string{"ls", "-la"},
                MaxRiskLevel: "high",     // Allows up to High risk
                Privileged:   false,     // No explicit privilege
                // doas commands are classified as Critical risk and blocked
            },
            shouldExecute: false,
            expectedError: &security.SecurityViolationError{},
        },
        {
            name: "privileged_rm_with_explicit_permission",
            command: &config.Command{
                Name:         "privileged_cleanup",
                Cmd:          "rm",
                Args:         []string{"-rf", "/tmp/app_files"},
                MaxRiskLevel: "high",    // Explicitly allow high risk
                Privileged:   true,      // Run with privileges
            },
            shouldExecute: true,
            expectedError: nil,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            manager, mockExecutor := tt.setupMocks(t)

            if tt.shouldExecute {
                mockExecutor.On("Execute", mock.Anything, tt.command, mock.Anything).
                    Return(&ExecutionResult{ExitCode: 0}, nil)
            }

            ctx := context.Background()
            result, err := manager.ExecuteCommand(ctx, tt.command, nil)

            if tt.expectedError != nil {
                assert.Error(t, err)
                assert.True(t, errors.Is(err, tt.expectedError))
                assert.Nil(t, result)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, result)
            }

            mockExecutor.AssertExpectations(t)
        })
    }
}
```

### 7.2 エンドツーエンドテスト

```go
// test/e2e/security_integration_test.go
func TestSecurityIntegrationE2E(t *testing.T) {
    // Create temporary config file
    configContent := `
[meta]
title = "Security Integration Test"

[[groups]]
name = "security_test"

  [[groups.commands]]
  name = "safe_echo"
  cmd = "echo"
  args = ["hello world"]

  [[groups.commands]]
  name = "dangerous_rm_blocked"
  cmd = "rm"
  args = ["-rf", "/tmp/nonexistent"]
  # No max_risk_level - should be blocked

  [[groups.commands]]
  name = "dangerous_rm_allowed"
  cmd = "rm"
  args = ["-rf", "/tmp/test_file"]
  max_risk_level = "high"

  [[groups.commands]]
  name = "sudo_with_privilege"
  cmd = "sudo"
  args = ["echo", "privileged operation"]
  run_as_user = "root"
`

    configFile := createTempConfigFile(t, configContent)
    defer os.Remove(configFile)

    tests := []struct {
        name          string
        command       string
        shouldSucceed bool
        expectedOutput string
    }{
        {
            name:           "safe_command_succeeds",
            command:        "safe_echo",
            shouldSucceed:  true,
            expectedOutput: "hello world",
        },
        {
            name:          "dangerous_command_blocked",
            command:       "dangerous_rm_blocked",
            shouldSucceed: false,
        },
        {
            name:          "dangerous_command_with_permission_succeeds",
            command:       "dangerous_rm_allowed",
            shouldSucceed: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd := exec.Command("./build/runner",
                "-config", configFile,
                "run", "security_test."+tt.command)

            output, err := cmd.CombinedOutput()

            if tt.shouldSucceed {
                assert.NoError(t, err, "Command should succeed: %s", string(output))
                if tt.expectedOutput != "" {
                    assert.Contains(t, string(output), tt.expectedOutput)
                }
            } else {
                assert.Error(t, err, "Command should fail")
                assert.Contains(t, string(output), "security")
            }
        })
    }
}
```

## 8. 実装計画

### 8.1 Phase 1: 基本実装
- [x] **Privilege Escalation Analyzer の基本実装** - `security.DefaultPrivilegeEscalationAnalyzer`完全実装済み
- [x] **Enhanced Risk Evaluator の実装** - `risk.StandardEvaluator`および`security.DefaultRiskEvaluator`完全実装済み
- [x] **Security Error Types の定義** - `ErrCommandSecurityViolation`等のエラータイプ完全実装済み
- [x] **Normal Manager への統合** - 統合リスクベース制御完全実装済み（Phase 3で統合アプローチ完成）
- [x] **基本テストケースの作成** - 包括的テストスイート完全実装済み

### 8.2 Phase 2: 高度な機能
- [x] **詳細な特権昇格分析** - sudo/su/doas検出、シンボリックリンク解決、Critical riskレベル分類完全実装済み
- [x] **設定検証の拡張** - max_risk_level/run_as_user/run_as_groupフィールド、ParseRiskLevel完全実装済み
- [x] **エラーハンドリングの改善** - 統合エラーハンドリング（ErrCommandSecurityViolation）完全実装済み
- [x] **統合テストの充実** - Dry-run/Normal mode両方完全実装済み

### 8.3 Phase 3: 最適化とドキュメント - **✅ 完全実装済み**
- [x] **max_risk_level制御の完全実装** - 統合アプローチによる完全実装済み（Low/Medium/High制御）
- [x] **Critical level設定禁止** - ParseRiskLevel関数で明示的拒否実装済み
- [x] **統合リスクベース制御** - 二重制御機構廃止、単一制御パス実装済み
- [x] **Normal modeでのrun_as_user/run_as_group実行機能** - Executor/Privilege Managerで完全実装済み
- [x] **パフォーマンス最適化** - 既存セキュリティ関数の活用により効率化済み
- [x] **エンドツーエンドテスト** - make test全通過、comprehensive.toml動作確認済み
- [x] **ドキュメント整備** - 要件/アーキテクチャ/実装状況すべて最新反映済み
- [x] **運用ガイドライン作成** - sample/risk-based-control.tomlで完全な設定例提供済み

### 🎯 Phase 4: プロダクション品質向上 - **✅ 完全実装済み**
- [x] **リンティング対応** - make lint 0 issues達成済み
- [x] **後方互換性保証** - 既存TOML設定ファイル動作確認済み
- [x] **セキュリティ監査** - 特権昇格コマンド完全ブロック、リスクレベル制御動作確認済み
- [x] **統合アプローチ完成** - Phase 3で二重制御機構を統一制御に変更、アーキテクチャ簡素化達成済み

### 🚀 **実装完了ステータス**
**すべてのPhaseが完了済み** - 統合リスクベースコマンド制御システムがプロダクション品質で実装されています。

**主要達成事項:**
- ✅ **統一制御アーキテクチャ**: 複雑な二重制御機構を統一アプローチに変更
- ✅ **完全なmax_risk_level制御**: Low/Medium/High設定による制御完全実装
- ✅ **Critical level保護**: ユーザー設定不可、内部分類専用として実装
- ✅ **特権昇格コマンド完全ブロック**: sudo/su/doas自動Critical分類・ブロック
- ✅ **run_as_user/run_as_group実行**: Normal mode実行機能完全実装
- ✅ **設定ファイル検証**: 不正設定の明示的拒否機能完全実装
- ✅ **包括的テスト**: 全機能テスト通過、エラーケース対応完了
- ✅ **ドキュメント完全性**: 要件/設計/実装状況すべて最新化

**品質保証:**
- 🔒 **セキュリティ**: 特権昇格攻撃防御、リスクレベル制御動作確認
- 🧪 **テスト**: 包括的テストスイート、エッジケース検証済み
- 📚 **文書**: 完全な設計文書、運用ガイド、設定例
- 🔄 **互換性**: 既存設定ファイル完全サポート
- ⚡ **性能**: 既存機能活用による高効率実装

この詳細設計書に基づいて、段階的な実装を進めることができます。各コンポーネントは独立してテスト可能な設計となっており、TDD アプローチでの開発に適しています。
