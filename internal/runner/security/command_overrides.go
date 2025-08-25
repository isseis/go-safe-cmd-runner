package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// CommandRiskOverrides defines individual command risk level overrides
var CommandRiskOverrides = map[string]runnertypes.RiskLevel{
	"/usr/bin/sudo":       runnertypes.RiskLevelCritical, // Privilege escalation
	"/bin/su":             runnertypes.RiskLevelCritical, // Privilege escalation
	"/usr/bin/curl":       runnertypes.RiskLevelMedium,   // Network access
	"/usr/bin/wget":       runnertypes.RiskLevelMedium,   // Network access
	"/usr/sbin/systemctl": runnertypes.RiskLevelHigh,     // System control
	"/usr/sbin/service":   runnertypes.RiskLevelHigh,     // System control
	"/bin/rm":             runnertypes.RiskLevelHigh,     // Destructive operations
	"/usr/bin/dd":         runnertypes.RiskLevelHigh,     // Destructive operations
}

// getCommandRiskOverride retrieves the risk override for a specific command
func getCommandRiskOverride(cmdPath string) (runnertypes.RiskLevel, bool) {
	risk, exists := CommandRiskOverrides[cmdPath]
	return risk, exists
}
