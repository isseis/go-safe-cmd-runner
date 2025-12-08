package resource

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// Global sensitive patterns instance for reuse
var defaultSensitivePatterns = redaction.DefaultSensitivePatterns()

// FormatterOptions contains options for formatting dry-run results
type FormatterOptions struct {
	DetailLevel   DryRunDetailLevel
	OutputFormat  OutputFormat
	ShowSensitive bool
	MaxWidth      int // For text formatting
}

// Formatter defines the interface for formatting dry-run results
type Formatter interface {
	FormatResult(result *DryRunResult, opts FormatterOptions) (string, error)
}

// TextFormatter implements text-based formatting
type TextFormatter struct{}

// JSONFormatter implements JSON-based formatting
type JSONFormatter struct{}

// NewTextFormatter creates a new text formatter
func NewTextFormatter() *TextFormatter {
	return &TextFormatter{}
}

// NewJSONFormatter creates a new JSON formatter
func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{}
}

// FormatResult formats a dry-run result as text
func (f *TextFormatter) FormatResult(result *DryRunResult, opts FormatterOptions) (string, error) {
	if result == nil {
		return "", ErrNilResult
	}

	var buf strings.Builder

	// Header
	f.writeHeader(&buf, result)

	// Summary
	f.writeSummary(&buf, result)

	// File Verification (if available)
	if result.FileVerification != nil {
		f.writeFileVerification(&buf, result.FileVerification, opts)
	}

	// Detailed information based on detail level
	switch opts.DetailLevel {
	case DetailLevelDetailed, DetailLevelFull:
		f.writeResourceAnalyses(&buf, result.ResourceAnalyses, opts)
		f.writeSecurityAnalysis(&buf, result.SecurityAnalysis, opts)
	}

	if opts.DetailLevel == DetailLevelFull {
		f.writeEnvironmentInfo(&buf, result.EnvironmentInfo)
	}

	// Errors and warnings
	f.writeErrorsAndWarnings(&buf, result.Errors, result.Warnings)

	return buf.String(), nil
}

// writeHeader writes the header section
func (f *TextFormatter) writeHeader(buf *strings.Builder, result *DryRunResult) {
	buf.WriteString("===== Dry-Run Analysis Report =====\n\n")

	if result.Metadata != nil {
		fmt.Fprintf(buf, "Generated at: %s\n", result.Metadata.GeneratedAt.Format(time.RFC3339))
		fmt.Fprintf(buf, "Run ID: %s\n", result.Metadata.RunID)
		if result.Metadata.Duration > 0 {
			fmt.Fprintf(buf, "Analysis duration: %v\n", result.Metadata.Duration)
		}
		buf.WriteString("\n")
	}
}

// writeSummary writes the summary section
func (f *TextFormatter) writeSummary(buf *strings.Builder, result *DryRunResult) {
	buf.WriteString("----- Summary -----\n\n")

	// Resource operations count
	resourceCounts := make(map[ResourceType]int)
	for _, analysis := range result.ResourceAnalyses {
		resourceCounts[analysis.Type]++
	}

	buf.WriteString("Resource Operations:\n")
	for resourceType, count := range resourceCounts {
		fmt.Fprintf(buf, "  - %s: %d\n", resourceType, count)
	}

	// Security summary
	if result.SecurityAnalysis != nil {
		riskCounts := make(map[runnertypes.RiskLevel]int)
		for _, risk := range result.SecurityAnalysis.Risks {
			riskCounts[risk.Level]++
		}

		if len(riskCounts) > 0 {
			buf.WriteString("Security Risks:\n")
			for level, count := range riskCounts {
				fmt.Fprintf(buf, "  - %s: %d\n", strings.ToUpper(level.String()[:1])+level.String()[1:], count)
			}
		}

		if len(result.SecurityAnalysis.PrivilegeChanges) > 0 {
			fmt.Fprintf(buf, "Privilege Changes: %d\n", len(result.SecurityAnalysis.PrivilegeChanges))
		}
	}

	buf.WriteString("\n")
}

// writeFileVerification writes the file verification section
func (f *TextFormatter) writeFileVerification(buf *strings.Builder, verification *verification.FileVerificationSummary, opts FormatterOptions) {
	if verification == nil {
		return
	}

	buf.WriteString("===== File Verification =====\n\n")

	// Hash directory status
	fmt.Fprintf(buf, "Hash Directory: %s\n", verification.HashDirStatus.Path)
	fmt.Fprintf(buf, "  Exists: %t\n", verification.HashDirStatus.Exists)
	fmt.Fprintf(buf, "  Validated: %t\n", verification.HashDirStatus.Validated)

	// Summary statistics
	fmt.Fprintf(buf, "Total Files: %d\n", verification.TotalFiles)
	fmt.Fprintf(buf, "  Verified: %d\n", verification.VerifiedFiles)
	fmt.Fprintf(buf, "  Skipped: %d\n", verification.SkippedFiles)
	fmt.Fprintf(buf, "  Failed: %d\n", verification.FailedFiles)
	fmt.Fprintf(buf, "Duration: %v\n", verification.Duration)

	// Failures (if any and detail level permits)
	if len(verification.Failures) > 0 && opts.DetailLevel >= DetailLevelDetailed {
		buf.WriteString("\nFailures:\n")
		for i, failure := range verification.Failures {
			marker := f.formatLevelMarker(failure.Level)
			fmt.Fprintf(buf, "%d. [%s] %s\n", i+1, marker, failure.Path)
			fmt.Fprintf(buf, "   Reason: %s\n", f.formatReason(failure.Reason))
			fmt.Fprintf(buf, "   Context: %s\n", failure.Context)
			if failure.Message != "" {
				fmt.Fprintf(buf, "   Message: %s\n", failure.Message)
			}
		}
	}

	buf.WriteString("\n")
}

// formatReason formats a FailureReason for display
func (f *TextFormatter) formatReason(reason verification.FailureReason) string {
	switch reason {
	case verification.ReasonHashDirNotFound:
		return "Hash directory not found"
	case verification.ReasonHashFileNotFound:
		return "Hash file not found"
	case verification.ReasonHashMismatch:
		return "Hash mismatch (potential tampering)"
	case verification.ReasonFileReadError:
		return "File read error"
	case verification.ReasonPermissionDenied:
		return "Permission denied"
	default:
		return string(reason)
	}
}

// formatLevelMarker formats a log level marker for display
func (f *TextFormatter) formatLevelMarker(level string) string {
	switch level {
	case "error":
		return "ERROR"
	case "warn":
		return "WARN"
	case "info":
		return "INFO"
	default:
		return strings.ToUpper(level)
	}
}

// writeResourceAnalyses writes the resource analyses section
func (f *TextFormatter) writeResourceAnalyses(buf *strings.Builder, analyses []ResourceAnalysis, opts FormatterOptions) {
	if len(analyses) == 0 {
		return
	}

	buf.WriteString("----- Resource Operations -----\n\n")

	for i, analysis := range analyses {
		fmt.Fprintf(buf, "%d. %s [%s]\n", i+1, analysis.Impact.Description, analysis.Type)
		fmt.Fprintf(buf, "   Operation: %s\n", analysis.Operation)
		fmt.Fprintf(buf, "   Target: %s\n", analysis.Target)
		fmt.Fprintf(buf, "   Timestamp: %s\n", analysis.Timestamp.Format("15:04:05"))

		if analysis.Impact.SecurityRisk != "" {
			fmt.Fprintf(buf, "   Security Risk: %s\n", strings.ToUpper(analysis.Impact.SecurityRisk))
		}

		fmt.Fprintf(buf, "   Reversible: %t, Persistent: %t\n",
			analysis.Impact.Reversible, analysis.Impact.Persistent)

		if opts.DetailLevel == DetailLevelFull && len(analysis.Parameters) > 0 {
			buf.WriteString("   Parameters:\n")
			for key, value := range analysis.Parameters {
				if !opts.ShowSensitive && defaultSensitivePatterns.IsSensitiveKey(key) {
					fmt.Fprintf(buf, "     %s: [REDACTED]\n", key)
				} else {
					// Use String() method which handles escaping for each type
					fmt.Fprintf(buf, "     %s: %s\n", key, value.String())
				}
			}
		}

		buf.WriteString("\n")
	}
}

// writeSecurityAnalysis writes the security analysis section
func (f *TextFormatter) writeSecurityAnalysis(buf *strings.Builder, security *SecurityAnalysis, _ FormatterOptions) {
	if security == nil {
		return
	}

	if len(security.Risks) > 0 {
		buf.WriteString("----- Security Analysis -----\n\n")

		for i, risk := range security.Risks {
			fmt.Fprintf(buf, "%d. [%s] %s\n", i+1, strings.ToUpper(risk.Level.String()), risk.Description)
			fmt.Fprintf(buf, "   Type: %s\n", risk.Type)
			if risk.Command != "" {
				fmt.Fprintf(buf, "   Command: %s\n", risk.Command)
			}
			if risk.Group != "" {
				fmt.Fprintf(buf, "   Group: %s\n", risk.Group)
			}
			if risk.Mitigation != "" {
				fmt.Fprintf(buf, "   Mitigation: %s\n", risk.Mitigation)
			}
			buf.WriteString("\n")
		}
	}

	if len(security.PrivilegeChanges) > 0 {
		buf.WriteString("Privilege Changes:\n")
		for _, change := range security.PrivilegeChanges {
			fmt.Fprintf(buf, "- %s: %s → %s (%s)\n",
				change.Command, change.FromUser, change.ToUser, change.Mechanism)
		}
		buf.WriteString("\n")
	}
}

// writeEnvironmentInfo writes the environment information section
func (f *TextFormatter) writeEnvironmentInfo(buf *strings.Builder, envInfo *EnvironmentInfo) {
	if envInfo == nil {
		return
	}

	buf.WriteString("===== Environment Information =====\n")
	fmt.Fprintf(buf, "Total Variables: %d\n", envInfo.TotalVariables)
	fmt.Fprintf(buf, "Allowed Variables: %d\n", len(envInfo.AllowedVariables))
	fmt.Fprintf(buf, "Filtered Variables: %d\n", len(envInfo.FilteredVariables))

	if len(envInfo.VariableUsage) > 0 {
		buf.WriteString("Variable Usage:\n")
		for variable, commands := range envInfo.VariableUsage {
			fmt.Fprintf(buf, "  %s: used by %d commands\n", variable, len(commands))
		}
	}
	buf.WriteString("\n")
}

// writeErrorsAndWarnings writes the errors and warnings section
func (f *TextFormatter) writeErrorsAndWarnings(buf *strings.Builder, errors []DryRunError, warnings []DryRunWarning) {
	if len(errors) > 0 {
		buf.WriteString("----- Errors -----\n\n")
		for i, err := range errors {
			fmt.Fprintf(buf, "%d. [%s] %s\n", i+1, err.Type, err.Message)
			if err.Component != "" {
				fmt.Fprintf(buf, "   Component: %s\n", err.Component)
			}
			if err.Group != "" && err.Command != "" {
				fmt.Fprintf(buf, "   Location: %s/%s\n", err.Group, err.Command)
			}
			fmt.Fprintf(buf, "   Recoverable: %t\n", err.Recoverable)
			buf.WriteString("\n")
		}
	}

	if len(warnings) > 0 {
		buf.WriteString("----- Warnings -----\n\n")
		for i, warning := range warnings {
			fmt.Fprintf(buf, "%d. [%s] %s\n", i+1, warning.Type, warning.Message)
			if warning.Component != "" {
				fmt.Fprintf(buf, "   Component: %s\n", warning.Component)
			}
			buf.WriteString("\n")
		}
	}
}

// FormatResult formats a dry-run result as JSON
func (f *JSONFormatter) FormatResult(result *DryRunResult, opts FormatterOptions) (string, error) {
	if result == nil {
		return "", ErrNilResult
	}

	// Create a copy for potential modification
	resultCopy := *result

	// Redact sensitive information if requested
	if !opts.ShowSensitive {
		f.redactSensitiveInfo(&resultCopy)
	}

	// Apply detail level filtering
	switch opts.DetailLevel {
	case DetailLevelSummary:
		f.applySummaryFilter(&resultCopy)
	case DetailLevelDetailed:
		// keep all details
	}

	data, err := json.MarshalIndent(&resultCopy, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(data), nil
}

// redactSensitiveInfo redacts sensitive information from the result
func (f *JSONFormatter) redactSensitiveInfo(result *DryRunResult) {
	for i := range result.ResourceAnalyses {
		for key := range result.ResourceAnalyses[i].Parameters {
			if defaultSensitivePatterns.IsSensitiveKey(key) {
				result.ResourceAnalyses[i].Parameters[key] = NewStringValue("[REDACTED]")
			}
		}
	}
}

// applySummaryFilter applies summary-level filtering
func (f *JSONFormatter) applySummaryFilter(result *DryRunResult) {
	// Keep only basic information for summary
	result.EnvironmentInfo = nil

	// Simplify resource analyses
	for i := range result.ResourceAnalyses {
		result.ResourceAnalyses[i].Parameters = nil
	}
}
