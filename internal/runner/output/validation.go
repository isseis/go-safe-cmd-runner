package output

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// ConfigValidator validates output capture configuration
type ConfigValidator struct {
	securityConfig *security.Config
	riskEvaluator  *RiskEvaluator
}

// Predefined validation errors
var (
	ErrGlobalConfigNil          = errors.New("global config cannot be nil")
	ErrNegativeMaxOutputSize    = errors.New("max_output_size cannot be negative")
	ErrMaxOutputSizeExceeded    = errors.New("max_output_size exceeds absolute maximum")
	ErrCommandNil               = errors.New("command cannot be nil")
	ErrInvalidEffectiveMaxSize  = errors.New("effective max output size must be positive")
	ErrConfigurationNil         = errors.New("configuration cannot be nil")
	ErrOutputPathEmpty          = errors.New("output path cannot be empty")
	ErrPathTraversalDetected    = errors.New("path traversal detected: contains '..'")
	ErrSensitiveSystemDirectory = errors.New("output path points to sensitive system directory")
	ErrSuspiciousExecutableExt  = errors.New("output path has suspicious executable extension")
	ErrOutputPathConflict       = errors.New("output path conflict")
	ErrRiskLevelExceeded        = errors.New("output path risk level exceeds maximum allowed")
)

// NewConfigValidator creates a new ConfigValidator instance
func NewConfigValidator() *ConfigValidator {
	return NewConfigValidatorWithSecurity(nil)
}

// NewConfigValidatorWithSecurity creates a new ConfigValidator with custom security config
func NewConfigValidatorWithSecurity(secConfig *security.Config) *ConfigValidator {
	if secConfig == nil {
		secConfig = security.DefaultConfig()
	}
	riskEvaluator := NewRiskEvaluator(secConfig)
	return &ConfigValidator{
		securityConfig: secConfig,
		riskEvaluator:  riskEvaluator,
	}
}

// ValidateGlobalConfig validates the global configuration for output capture
func (v *ConfigValidator) ValidateGlobalConfig(globalConfig *runnertypes.GlobalConfig) error {
	if globalConfig == nil {
		return ErrGlobalConfigNil
	}

	// Validate MaxOutputSize
	if globalConfig.MaxOutputSize < 0 {
		return fmt.Errorf("%w: %d", ErrNegativeMaxOutputSize, globalConfig.MaxOutputSize)
	}

	// MaxOutputSize should be positive (default should be set during config loading)
	if globalConfig.MaxOutputSize == 0 {
		return fmt.Errorf("%w: max_output_size must be positive", ErrNegativeMaxOutputSize)
	}

	if globalConfig.MaxOutputSize > AbsoluteMaxOutputSize {
		return fmt.Errorf("%w (%d): %d", ErrMaxOutputSizeExceeded,
			AbsoluteMaxOutputSize, globalConfig.MaxOutputSize)
	}

	return nil
}

// ValidateCommand validates a command configuration for output capture
func (v *ConfigValidator) ValidateCommand(cmd *runnertypes.Command, globalConfig *runnertypes.GlobalConfig) error {
	if cmd == nil {
		return ErrCommandNil
	}

	// Validate output path, considering max_risk_level
	if err := v.validateOutputPathWithRiskLevel(cmd.Output, cmd); err != nil {
		return fmt.Errorf("invalid output path '%s': %w", cmd.Output, err)
	}

	// Validate effective size limit
	effectiveMaxSize := v.getEffectiveMaxSize(globalConfig)
	if effectiveMaxSize <= 0 {
		return fmt.Errorf("%w: %d", ErrInvalidEffectiveMaxSize, effectiveMaxSize)
	}

	return nil
}

// ValidateCommands validates all commands in a slice
func (v *ConfigValidator) ValidateCommands(commands []runnertypes.Command, globalConfig *runnertypes.GlobalConfig) error {
	if len(commands) == 0 {
		return nil
	}

	// Track output paths to detect conflicts
	outputPaths := make(map[string]*runnertypes.Command)

	for i := range commands {
		// Intentionally take the address of the slice element to avoid the loop variable capture issue.
		// Do NOT use 'for _, cmd := range commands' and then take '&cmd', as that would be incorrect.
		cmd := &commands[i]
		// Validate individual command
		if err := v.ValidateCommand(cmd, globalConfig); err != nil {
			return fmt.Errorf("command '%s' at index %d: %w", cmd.Name, i, err)
		}

		// Check for output path conflicts
		if cmd.Output != "" {
			resolvedPath, err := filepath.Abs(cmd.Output)
			if err != nil {
				// Use original path if resolution fails
				resolvedPath = cmd.Output
			}

			if existingCmd, exists := outputPaths[resolvedPath]; exists {
				return fmt.Errorf("%w: commands '%s' and '%s' both write to '%s'",
					ErrOutputPathConflict, existingCmd.Name, cmd.Name, resolvedPath)
			}
			outputPaths[resolvedPath] = cmd
		}
	}

	return nil
}

// ValidateConfigFile validates an entire configuration file
func (v *ConfigValidator) ValidateConfigFile(cfg *runnertypes.Config) error {
	if cfg == nil {
		return ErrConfigurationNil
	}

	// Validate global config
	if err := v.ValidateGlobalConfig(&cfg.Global); err != nil {
		return fmt.Errorf("global configuration error: %w", err)
	}

	// Validate all groups
	for _, group := range cfg.Groups {
		if err := v.ValidateCommands(group.Commands, &cfg.Global); err != nil {
			return fmt.Errorf("group '%s': %w", group.Name, err)
		}
	}

	return nil
}

// validateOutputPath performs basic validation on output paths
func (v *ConfigValidator) validateOutputPath(outputPath string) error {
	// If no output is specified, no validation needed
	if outputPath == "" {
		return nil
	}

	// Use the unified risk evaluator with default (strict) risk level
	evaluation := v.riskEvaluator.EvaluateWithMaxRiskLevel(outputPath, runnertypes.RiskLevelLow)

	if evaluation.IsBlocking {
		return v.riskEvaluator.CreateValidationError(evaluation, runnertypes.RiskLevelLow)
	}

	return nil
}

// validateOutputPathWithRiskLevel performs validation on output paths considering max_risk_level
func (v *ConfigValidator) validateOutputPathWithRiskLevel(outputPath string, cmd *runnertypes.Command) error {
	// If no output is specified, no validation needed
	if outputPath == "" {
		return nil
	}

	// Get the maximum allowed risk level for this command
	maxAllowedRisk, err := cmd.GetMaxRiskLevel()
	if err != nil {
		// If max_risk_level is invalid, default to low risk (most restrictive)
		maxAllowedRisk = runnertypes.RiskLevelLow
	}

	// Use the unified risk evaluator
	evaluation := v.riskEvaluator.EvaluateWithMaxRiskLevel(outputPath, maxAllowedRisk)

	// If the risk is blocking, create appropriate error
	if evaluation.IsBlocking {
		return v.riskEvaluator.CreateValidationError(evaluation, maxAllowedRisk)
	}

	return nil
}

// getEffectiveMaxSize returns the effective maximum output size
func (v *ConfigValidator) getEffectiveMaxSize(globalConfig *runnertypes.GlobalConfig) int64 {
	if globalConfig == nil || globalConfig.MaxOutputSize <= 0 {
		return DefaultMaxOutputSize
	}
	return globalConfig.MaxOutputSize
}

// AssessSecurityRisk assesses the security risk of an output path
func (v *ConfigValidator) AssessSecurityRisk(outputPath string) runnertypes.RiskLevel {
	evaluation := v.riskEvaluator.EvaluateOutputRisk(outputPath)
	return evaluation.Level
}

// GenerateValidationReport generates a comprehensive validation report
func (v *ConfigValidator) GenerateValidationReport(cfg *runnertypes.Config) *ValidationReport {
	report := &ValidationReport{
		Valid:        true,
		Errors:       []string{},
		Warnings:     []string{},
		CommandCount: 0,
		OutputCount:  0,
	}

	if cfg == nil {
		report.Valid = false
		report.Errors = append(report.Errors, "Configuration is nil")
		return report
	}

	// Validate global config
	if err := v.ValidateGlobalConfig(&cfg.Global); err != nil {
		report.Valid = false
		report.Errors = append(report.Errors, fmt.Sprintf("Global config: %v", err))
	}

	// Analyze size configuration
	if cfg.Global.MaxOutputSize > DefaultMaxOutputSize {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Large max output size configured: %d bytes", cfg.Global.MaxOutputSize))
	}

	// Validate all groups and collect statistics
	for _, group := range cfg.Groups {
		for _, cmd := range group.Commands {
			report.CommandCount++

			if cmd.Output != "" {
				report.OutputCount++

				// Validate command
				if err := v.ValidateCommand(&cmd, &cfg.Global); err != nil {
					report.Valid = false
					report.Errors = append(report.Errors,
						fmt.Sprintf("Command '%s' in group '%s': %v", cmd.Name, group.Name, err))
				}

				// Assess security risk and compare with max_risk_level
				maxAllowedRisk, err := cmd.GetMaxRiskLevel()
				if err != nil {
					// If max_risk_level is invalid, default to low risk (most restrictive)
					maxAllowedRisk = runnertypes.RiskLevelLow
				}

				evaluation := v.riskEvaluator.EvaluateWithMaxRiskLevel(cmd.Output, maxAllowedRisk)

				if evaluation.IsBlocking {
					// This would cause execution failure
					report.Warnings = append(report.Warnings,
						fmt.Sprintf("Command '%s' output path risk (%s) exceeds max_risk_level (%s): %s (%s)",
							cmd.Name, evaluation.Level.String(), maxAllowedRisk.String(), cmd.Output, evaluation.Reason))
				} else if evaluation.Level == runnertypes.RiskLevelHigh || evaluation.Level == runnertypes.RiskLevelCritical {
					// High/critical risk but within allowed range
					report.Warnings = append(report.Warnings,
						fmt.Sprintf("Command '%s' has %s risk output path (allowed by max_risk_level %s): %s (%s)",
							cmd.Name, evaluation.Level.String(), maxAllowedRisk.String(), cmd.Output, evaluation.Reason))
				}
			}
		}
	}

	return report
}

// ValidationReport represents the result of configuration validation
type ValidationReport struct {
	Valid        bool     `json:"valid"`
	Errors       []string `json:"errors"`
	Warnings     []string `json:"warnings"`
	CommandCount int      `json:"command_count"`
	OutputCount  int      `json:"output_count"`
}

// String returns a string representation of the validation report
func (r *ValidationReport) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Validation Report: %s\n", map[bool]string{true: "VALID", false: "INVALID"}[r.Valid]))
	sb.WriteString(fmt.Sprintf("Commands: %d, Output Capture: %d\n", r.CommandCount, r.OutputCount))

	if len(r.Errors) > 0 {
		sb.WriteString("\nErrors:\n")
		for _, err := range r.Errors {
			sb.WriteString(fmt.Sprintf("  - %s\n", err))
		}
	}

	if len(r.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, warning := range r.Warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", warning))
		}
	}

	return sb.String()
}
