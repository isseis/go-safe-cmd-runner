// Package debug provides debug information collection and formatting for dry-run mode.
package debug

import (
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// CollectInheritanceAnalysis collects environment variable inheritance analysis information
// This function is the single source of truth for inheritance analysis data
// Returns nil for DetailLevelSummary
func CollectInheritanceAnalysis(
	runtimeGlobal *runnertypes.RuntimeGlobal,
	runtimeGroup *runnertypes.RuntimeGroup,
	detailLevel resource.DryRunDetailLevel,
) *resource.InheritanceAnalysis {
	// Return nil for summary level
	if detailLevel == resource.DetailLevelSummary {
		return nil
	}

	// Extract group spec safely
	groupSpec := runtimeGroup.Spec
	if groupSpec == nil {
		groupSpec = &runnertypes.GroupSpec{}
	}

	// Build base analysis with configuration and computed fields
	analysis := &resource.InheritanceAnalysis{
		// Configuration fields from global
		GlobalEnvImport: common.CloneOrEmpty(runtimeGlobal.Spec.EnvImport),
		GlobalAllowlist: common.CloneOrEmpty(runtimeGlobal.Spec.EnvAllowed),

		// Configuration fields from group
		GroupEnvImport: common.CloneOrEmpty(groupSpec.EnvImport),
		GroupAllowlist: common.CloneOrEmpty(groupSpec.EnvAllowed),

		// Computed field
		InheritanceMode: runtimeGroup.EnvAllowlistInheritanceMode,
	}

	// Add difference fields only for DetailLevelFull
	if detailLevel == resource.DetailLevelFull {
		// Calculate inherited variables
		if runtimeGroup.EnvAllowlistInheritanceMode == runnertypes.InheritanceModeInherit {
			analysis.InheritedVariables = common.CloneOrEmpty(runtimeGlobal.Spec.EnvAllowed)
		} else {
			analysis.InheritedVariables = []string{}
		}

		// Calculate removed allowlist variables
		if runtimeGroup.EnvAllowlistInheritanceMode == runnertypes.InheritanceModeExplicit ||
			runtimeGroup.EnvAllowlistInheritanceMode == runnertypes.InheritanceModeReject {
			globalSet := common.SliceToSet(runtimeGlobal.Spec.EnvAllowed)
			groupSet := common.SliceToSet(groupSpec.EnvAllowed)
			analysis.RemovedAllowlistVariables = common.SetDifferenceToSlice(globalSet, groupSet)
		} else {
			analysis.RemovedAllowlistVariables = []string{}
		}

		// Calculate unavailable env_import variables
		if len(groupSpec.EnvImport) > 0 && len(runtimeGlobal.Spec.EnvImport) > 0 {
			globalVars := extractInternalVarNames(runtimeGlobal.Spec.EnvImport)
			groupVars := extractInternalVarNames(groupSpec.EnvImport)
			globalSet := common.SliceToSet(globalVars)
			groupSet := common.SliceToSet(groupVars)
			analysis.UnavailableEnvImportVariables = common.SetDifferenceToSlice(globalSet, groupSet)
		} else {
			analysis.UnavailableEnvImportVariables = []string{}
		}
	}

	return analysis
}

// CollectFinalEnvironment collects final resolved environment variables
// Returns nil for DetailLevelSummary and DetailLevelDetailed
func CollectFinalEnvironment(
	envMap map[string]executor.EnvVar,
	detailLevel resource.DryRunDetailLevel,
	showSensitive bool,
) *resource.FinalEnvironment {
	// Only collect for DetailLevelFull
	if detailLevel != resource.DetailLevelFull {
		return nil
	}

	finalEnv := &resource.FinalEnvironment{
		Variables: make(map[string]resource.EnvironmentVariable, len(envMap)),
	}

	// Create patterns for sensitive information detection
	patterns := redaction.DefaultSensitivePatterns()

	for name, envVar := range envMap {
		variable := resource.EnvironmentVariable{
			Source: envVar.Origin,
		}

		// Include value only if showSensitive is true
		if showSensitive {
			variable.Value = envVar.Value
		} else {
			// Check if variable is sensitive
			if patterns.IsSensitiveEnvVar(name) {
				variable.Value = "" // Omit value
				variable.Masked = true
			} else {
				variable.Value = envVar.Value
			}
		}

		finalEnv.Variables[name] = variable
	}

	return finalEnv
}

// Helper functions

// extractInternalVarNames extracts internal variable names from env_import mappings
// Example: "db_host=DB_HOST" -> "db_host"
//
// Precondition: envImport must contain only validated "key=value" format strings.
// This is guaranteed by ProcessFromEnv() validation during RuntimeGlobal/RuntimeGroup creation.
// If parsing fails, it indicates a program invariant violation and will panic.
func extractInternalVarNames(envImport []string) []string {
	var result []string
	for _, mapping := range envImport {
		key, _, ok := common.ParseKeyValue(mapping)
		if !ok {
			// This should never happen as envImport is validated during expansion.
			// If it does, it indicates a serious programming error.
			panic("invalid env_import format: " + mapping + " (should be validated by ProcessFromEnv)")
		}
		result = append(result, key)
	}
	sort.Strings(result) // Sort for consistent output
	return result
}
