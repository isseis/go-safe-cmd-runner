package security

import (
	"path/filepath"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// StandardDirectories defines standard system directories with predefined risk levels
var StandardDirectories = []string{
	"/bin",
	"/usr/bin",
	"/usr/local/bin",
	"/sbin",
	"/usr/sbin",
	"/usr/local/sbin",
}

// DefaultRiskLevels defines default risk levels for standard directories
var DefaultRiskLevels = map[string]runnertypes.RiskLevel{
	"/bin":            runnertypes.RiskLevelLow,
	"/usr/bin":        runnertypes.RiskLevelLow,
	"/usr/local/bin":  runnertypes.RiskLevelLow,
	"/sbin":           runnertypes.RiskLevelMedium,
	"/usr/sbin":       runnertypes.RiskLevelMedium,
	"/usr/local/sbin": runnertypes.RiskLevelMedium,
}

// getDefaultRiskByDirectory returns the default risk level based on command path
func getDefaultRiskByDirectory(cmdPath string) runnertypes.RiskLevel {
	dir := filepath.Dir(cmdPath)

	// Exact match check
	if risk, exists := DefaultRiskLevels[dir]; exists {
		return risk
	}

	// Prefix match for subdirectories
	for stdDir, risk := range DefaultRiskLevels {
		if strings.HasPrefix(cmdPath, stdDir+"/") {
			return risk
		}
	}

	// Default: defer to individual pattern analysis
	return runnertypes.RiskLevelUnknown
}

// isStandardDirectory checks if the command path is in a standard directory
func isStandardDirectory(cmdPath string) bool {
	dir := filepath.Dir(cmdPath)

	for _, stdDir := range StandardDirectories {
		if dir == stdDir || strings.HasPrefix(cmdPath, stdDir+"/") {
			return true
		}
	}
	return false
}
