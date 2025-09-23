package output

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// ConfigValidator validates output capture configuration
type ConfigValidator struct {
	securityConfig *security.Config
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
	return &ConfigValidator{
		securityConfig: secConfig,
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

	// If no output is specified, no validation needed
	if cmd.Output == "" {
		return nil
	}

	// Validate output path
	if err := v.validateOutputPath(cmd.Output); err != nil {
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

// checkPatternMatch checks if a path matches any of the given patterns using case-insensitive comparison
// Returns the matching pattern if found, empty string if no match
func (v *ConfigValidator) checkPatternMatch(path string, patterns []string) string {
	lowerPath := strings.ToLower(path)

	for _, pattern := range patterns {
		patternLower := strings.ToLower(pattern)
		// Only check directory patterns (ending with /) using prefix matching
		if strings.HasSuffix(patternLower, "/") {
			if strings.HasPrefix(lowerPath, patternLower) {
				return pattern
			}
		} else {
			// For file patterns, use contains matching
			if strings.Contains(lowerPath, patternLower) {
				return pattern
			}
		}
	}
	return ""
}

// validateOutputPath performs basic validation on output paths
func (v *ConfigValidator) validateOutputPath(outputPath string) error {
	if outputPath == "" {
		return ErrOutputPathEmpty
	}

	// Check for path traversal attempts
	if common.ContainsPathTraversalSegment(outputPath) {
		return ErrPathTraversalDetected
	}

	// Check for critical and high-risk directory patterns using security config
	criticalPatterns := v.securityConfig.GetPathPatternsByRisk(runnertypes.RiskLevelCritical)
	highRiskPatterns := v.securityConfig.GetPathPatternsByRisk(runnertypes.RiskLevelHigh)

	// Check critical directory patterns
	if matchedPattern := v.checkPatternMatch(outputPath, criticalPatterns); matchedPattern != "" {
		return fmt.Errorf("%w: %s", ErrSensitiveSystemDirectory, matchedPattern)
	}

	// Check high-risk directory patterns
	if matchedPattern := v.checkPatternMatch(outputPath, highRiskPatterns); matchedPattern != "" {
		return fmt.Errorf("%w: %s", ErrSensitiveSystemDirectory, matchedPattern)
	}

	// Check for suspicious file extensions using security config
	suspiciousExtensions := v.securityConfig.GetSuspiciousExtensions()

	lowerPath := strings.ToLower(outputPath)
	for _, ext := range suspiciousExtensions {
		if strings.HasSuffix(lowerPath, strings.ToLower(ext)) {
			return fmt.Errorf("%w: %s", ErrSuspiciousExecutableExt, ext)
		}
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
func (v *ConfigValidator) AssessSecurityRisk(outputPath string, _ string) runnertypes.RiskLevel {
	if outputPath == "" {
		return runnertypes.RiskLevelHigh
	}

	// Check for absolute paths outside safe directories
	if filepath.IsAbs(outputPath) {
		// System directories are high risk - use security config patterns
		criticalPatterns := v.securityConfig.GetPathPatternsByRisk(runnertypes.RiskLevelCritical)
		highRiskPatterns := v.securityConfig.GetPathPatternsByRisk(runnertypes.RiskLevelHigh)

		// Check critical patterns first
		if v.checkPatternMatch(outputPath, criticalPatterns) != "" {
			return runnertypes.RiskLevelCritical
		}

		// Check high-risk patterns - system directories are always critical
		if v.checkPatternMatch(outputPath, highRiskPatterns) != "" {
			return runnertypes.RiskLevelCritical
		}

		// /tmp and /var/tmp are medium risk
		if strings.HasPrefix(outputPath, "/tmp") || strings.HasPrefix(outputPath, "/var/tmp") {
			return runnertypes.RiskLevelMedium
		}

		// Other absolute paths are high risk
		return runnertypes.RiskLevelHigh
	}

	// Relative paths with traversal are high risk
	if common.ContainsPathTraversalSegment(outputPath) {
		return runnertypes.RiskLevelHigh
	}

	// Check for suspicious patterns in relative paths using security config
	suspiciousPatterns := v.securityConfig.GetSuspiciousFilePatterns()

	lowerPath := strings.ToLower(outputPath)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerPath, strings.ToLower(pattern)) {
			return runnertypes.RiskLevelHigh
		}
	}

	// Relative paths within working directory are low risk
	return runnertypes.RiskLevelLow
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

				// Assess security risk
				risk := v.AssessSecurityRisk(cmd.Output, cfg.Global.WorkDir)
				if risk == runnertypes.RiskLevelHigh || risk == runnertypes.RiskLevelCritical {
					report.Warnings = append(report.Warnings,
						fmt.Sprintf("Command '%s' has %s risk output path: %s",
							cmd.Name, risk.String(), cmd.Output))
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
