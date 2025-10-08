package security

import (
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Validation errors for CommandRiskProfileNew
var (
	// ErrNetworkAlwaysRequiresMediumRiskNew is returned when NetworkTypeAlways has NetworkRisk < Medium
	ErrNetworkAlwaysRequiresMediumRiskNew = errors.New("NetworkTypeAlways commands must have NetworkRisk >= Medium")

	// ErrNetworkSubcommandsOnlyForConditionalNew is returned when NetworkSubcommands is set for non-conditional network type
	ErrNetworkSubcommandsOnlyForConditionalNew = errors.New("NetworkSubcommands should only be set for NetworkTypeConditional")
)

// CommandRiskProfileNew defines comprehensive risk information for a command
// This is the new structure that will replace CommandRiskProfile after migration.
type CommandRiskProfileNew struct {
	// Individual risk factors (explicit separation)
	PrivilegeRisk   RiskFactor // Risk from privilege escalation (sudo, su, doas)
	NetworkRisk     RiskFactor // Risk from network operations
	DestructionRisk RiskFactor // Risk from destructive operations (rm, dd, format)
	DataExfilRisk   RiskFactor // Risk from data exfiltration to external services
	SystemModRisk   RiskFactor // Risk from system modifications (systemctl, service)

	// Network behavior configuration
	NetworkType        NetworkOperationType // How network operations are determined
	NetworkSubcommands []string             // Subcommands that trigger network operations
}

// defaultRiskReasonsCap is the initial capacity used when collecting risk reasons.
const defaultRiskReasonsCap = 5

// IsPrivilege returns true if the command involves privilege escalation
func (p CommandRiskProfileNew) IsPrivilege() bool {
	return p.PrivilegeRisk.Level >= runnertypes.RiskLevelHigh
}

// BaseRiskLevel computes the overall risk level as the maximum of all risk factors
func (p CommandRiskProfileNew) BaseRiskLevel() runnertypes.RiskLevel {
	return max(
		p.PrivilegeRisk.Level,
		p.NetworkRisk.Level,
		p.DestructionRisk.Level,
		p.DataExfilRisk.Level,
		p.SystemModRisk.Level,
	)
}

// GetRiskReasons returns all non-empty reasons contributing to the risk level
func (p CommandRiskProfileNew) GetRiskReasons() []string {
	reasons := make([]string, 0, defaultRiskReasonsCap)

	// Helper function to add non-empty reasons
	addReason := func(risk RiskFactor) {
		if risk.Level > runnertypes.RiskLevelUnknown && risk.Reason != "" {
			reasons = append(reasons, risk.Reason)
		}
	}

	// Collect all risk factors in order
	addReason(p.PrivilegeRisk)
	addReason(p.NetworkRisk)
	addReason(p.DestructionRisk)
	addReason(p.DataExfilRisk)
	addReason(p.SystemModRisk)

	return reasons
}

// Validate ensures consistency between risk factors and configuration
func (p CommandRiskProfileNew) Validate() error {
	// Rule 1: NetworkTypeAlways implies NetworkRisk >= Medium
	if p.NetworkType == NetworkTypeAlways && p.NetworkRisk.Level < runnertypes.RiskLevelMedium {
		return fmt.Errorf("%w (got %v)", ErrNetworkAlwaysRequiresMediumRiskNew, p.NetworkRisk.Level)
	}

	// Rule 2: NetworkSubcommands only for NetworkTypeConditional
	if len(p.NetworkSubcommands) > 0 && p.NetworkType != NetworkTypeConditional {
		return ErrNetworkSubcommandsOnlyForConditionalNew
	}

	return nil
}
