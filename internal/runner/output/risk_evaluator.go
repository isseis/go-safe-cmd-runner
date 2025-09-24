// Package output provides output path risk evaluation functionality.
package output

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// RiskEvaluation contains the result of output path risk assessment
type RiskEvaluation struct {
	Level      runnertypes.RiskLevel
	Reason     string // Human-readable reason for the risk level
	Pattern    string // The specific pattern that matched (if any)
	Category   string // Category of risk (e.g., "system_directory", "suspicious_extension")
	IsBlocking bool   // Whether this should block execution (considering max_risk_level)
}

// RiskEvaluator evaluates security risks for output paths
type RiskEvaluator struct {
	securityConfig *security.Config
}

// NewRiskEvaluator creates a new output risk evaluator
func NewRiskEvaluator(secConfig *security.Config) *RiskEvaluator {
	if secConfig == nil {
		secConfig = security.DefaultConfig()
	}
	return &RiskEvaluator{
		securityConfig: secConfig,
	}
}

// EvaluateOutputRisk performs comprehensive risk assessment of an output path
func (e *RiskEvaluator) EvaluateOutputRisk(outputPath string) *RiskEvaluation {
	// Empty path is high risk (stdout/stderr redirection without explicit file)
	if outputPath == "" {
		return &RiskEvaluation{
			Level:    runnertypes.RiskLevelHigh,
			Reason:   "No output file specified",
			Category: "empty_path",
		}
	}

	// Check for path traversal attempts (always high risk)
	if common.ContainsPathTraversalSegment(outputPath) {
		return &RiskEvaluation{
			Level:    runnertypes.RiskLevelHigh,
			Reason:   "Path traversal detected",
			Pattern:  "..",
			Category: "path_traversal",
		}
	}

	// For absolute paths, check system directories and other absolute path risks
	if filepath.IsAbs(outputPath) {
		return e.evaluateAbsolutePath(outputPath)
	}

	// For relative paths, check suspicious patterns and extensions
	return e.evaluateRelativePath(outputPath)
}

// EvaluateWithMaxRiskLevel evaluates risk and determines if it should block execution
func (e *RiskEvaluator) EvaluateWithMaxRiskLevel(outputPath string, maxAllowedRisk runnertypes.RiskLevel) *RiskEvaluation {
	eval := e.EvaluateOutputRisk(outputPath)
	eval.IsBlocking = eval.Level > maxAllowedRisk
	return eval
}

// evaluateAbsolutePath evaluates risks for absolute paths
//
// Security Design Rationale:
// For absolute paths, the primary security concern is "WHERE" the output is written.
// Writing to system directories poses immediate system-wide security risks regardless
// of file content or extension. Therefore, this function focuses exclusively on path
// location validation and does not check file extensions or suspicious patterns.
// The location-based risk assessment takes precedence over content-based concerns.
func (e *RiskEvaluator) evaluateAbsolutePath(outputPath string) *RiskEvaluation {
	// Check critical patterns first
	criticalPatterns := e.securityConfig.GetPathPatternsByRisk(runnertypes.RiskLevelCritical)
	if matchedPattern := e.checkPatternMatch(outputPath, criticalPatterns); matchedPattern != "" {
		return &RiskEvaluation{
			Level:    runnertypes.RiskLevelCritical,
			Reason:   "Output path points to critical system directory",
			Pattern:  matchedPattern,
			Category: "critical_system_directory",
		}
	}

	// Check high-risk patterns
	highRiskPatterns := e.securityConfig.GetPathPatternsByRisk(runnertypes.RiskLevelHigh)
	if matchedPattern := e.checkPatternMatch(outputPath, highRiskPatterns); matchedPattern != "" {
		return &RiskEvaluation{
			Level:    runnertypes.RiskLevelHigh,
			Reason:   "Output path points to high-risk system directory",
			Pattern:  matchedPattern,
			Category: "high_risk_system_directory",
		}
	}

	// /tmp and /var/tmp are medium risk
	if strings.HasPrefix(outputPath, "/tmp/") || strings.HasPrefix(outputPath, "/var/tmp/") {
		return &RiskEvaluation{
			Level:    runnertypes.RiskLevelMedium,
			Reason:   "Output path in temporary directory",
			Category: "temporary_directory",
		}
	}

	// Other absolute paths are high risk
	return &RiskEvaluation{
		Level:    runnertypes.RiskLevelHigh,
		Reason:   "Absolute path outside of working directory",
		Category: "absolute_path",
	}
}

// evaluateRelativePath evaluates risks for relative paths
//
// Security Design Rationale:
// For relative paths, the location is considered safe (within working directory),
// but the primary security concern is "WHAT TYPE OF FILE" is being created.
// Malicious actors could create executable files, configuration files, or other
// dangerous file types that could be exploited later. Therefore, this function
// focuses on content-based validation (file extensions and suspicious patterns)
// rather than location-based validation, which is the inverse of absolute path evaluation.
func (e *RiskEvaluator) evaluateRelativePath(outputPath string) *RiskEvaluation {
	lowerPath := strings.ToLower(outputPath)

	// Check for suspicious file patterns
	suspiciousPatterns := e.securityConfig.GetSuspiciousFilePatterns()
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerPath, strings.ToLower(pattern)) {
			return &RiskEvaluation{
				Level:    runnertypes.RiskLevelHigh,
				Reason:   "Output path contains suspicious file pattern",
				Pattern:  pattern,
				Category: "suspicious_file_pattern",
			}
		}
	}

	// Check for suspicious file extensions
	suspiciousExtensions := e.securityConfig.GetSuspiciousExtensions()
	for _, ext := range suspiciousExtensions {
		if strings.HasSuffix(lowerPath, strings.ToLower(ext)) {
			return &RiskEvaluation{
				Level:    runnertypes.RiskLevelHigh,
				Reason:   "Output path has suspicious executable extension",
				Pattern:  ext,
				Category: "suspicious_extension",
			}
		}
	}

	// Relative paths within working directory are low risk
	return &RiskEvaluation{
		Level:    runnertypes.RiskLevelLow,
		Reason:   "Safe relative path within working directory",
		Category: "safe_relative_path",
	}
}

// checkPatternMatch checks if a path matches any of the given patterns using case-insensitive comparison
// Returns the matching pattern if found, empty string if no match
func (e *RiskEvaluator) checkPatternMatch(path string, patterns []string) string {
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

// CreateValidationError creates an appropriate validation error based on the risk evaluation
func (e *RiskEvaluator) CreateValidationError(eval *RiskEvaluation, maxAllowedRisk runnertypes.RiskLevel) error {
	if !eval.IsBlocking {
		return nil
	}

	switch eval.Category {
	case "path_traversal":
		return ErrPathTraversalDetected
	case "critical_system_directory", "high_risk_system_directory":
		return fmt.Errorf("%w: %s (risk level: %s, max allowed: %s)",
			ErrSensitiveSystemDirectory, eval.Pattern, eval.Level.String(), maxAllowedRisk.String())
	case "suspicious_extension":
		return fmt.Errorf("%w: %s (risk level: %s, max allowed: %s)",
			ErrSuspiciousExecutableExt, eval.Pattern, eval.Level.String(), maxAllowedRisk.String())
	default:
		return fmt.Errorf("%w: %s exceeds %s (%s)",
			ErrRiskLevelExceeded, eval.Level.String(), maxAllowedRisk.String(), eval.Reason)
	}
}
