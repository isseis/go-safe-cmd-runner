package security

import (
	"errors"
	"fmt"
	"slices"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// Validation errors for CommandRiskProfile
var (
	// ErrNetworkAlwaysRequiresMediumRisk is returned when NetworkTypeAlways has NetworkRisk < Medium
	ErrNetworkAlwaysRequiresMediumRisk = errors.New("NetworkTypeAlways commands must have NetworkRisk >= Medium")

	// ErrNetworkSubcommandsOnlyForConditional is returned when NetworkSubcommands is set for non-conditional network type
	ErrNetworkSubcommandsOnlyForConditional = errors.New("NetworkSubcommands should only be set for NetworkTypeConditional")
)

// CommandRiskProfile defines comprehensive risk information for a command.
// This is the new structure that will replace CommandRiskProfile after migration.
//
// The profile separates risk into distinct factors:
//   - PrivilegeRisk: Privilege escalation (sudo, su, doas)
//   - NetworkRisk: Network operations (curl, wget, ssh)
//   - DestructionRisk: Destructive operations (rm, dd, format)
//   - DataExfilRisk: Data exfiltration to external services
//   - SystemModRisk: System modifications (systemctl, apt)
//
// The overall risk level is computed as the maximum of all risk factors.
// Use ProfileBuilder and NewProfile() to create instances with validation.
type CommandRiskProfile struct {
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
func (p CommandRiskProfile) IsPrivilege() bool {
	return p.PrivilegeRisk.Level >= runnertypes.RiskLevelHigh
}

// BaseRiskLevel computes the overall risk level as the maximum of all risk factors
func (p CommandRiskProfile) BaseRiskLevel() runnertypes.RiskLevel {
	return max(
		p.PrivilegeRisk.Level,
		p.NetworkRisk.Level,
		p.DestructionRisk.Level,
		p.DataExfilRisk.Level,
		p.SystemModRisk.Level,
	)
}

// GetRiskReasons returns all non-empty reasons contributing to the risk level
func (p CommandRiskProfile) GetRiskReasons() []string {
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

// ResolveProfile returns the risk profile matched by any name in the command's
// pre-resolved name set (the caller resolves the symlink chain once with its own
// policy). When multiple names match (a symlink pointing at a differently-named
// profiled command), the factors are merged by taking the maximum of each factor.
// found is false when no name matches.
func ResolveProfile(names map[string]struct{}) (profile CommandRiskProfile, found bool) {
	for name := range names {
		p, exists := commandRiskProfiles[name]
		if !exists {
			continue
		}
		if !found {
			profile = p
			found = true
			continue
		}
		profile = mergeProfilesMax(profile, p)
	}
	return profile, found
}

// maxFactor returns the higher-level of two risk factors, preserving its reason.
func maxFactor(a, b RiskFactor) RiskFactor {
	if b.Level > a.Level {
		return b
	}
	return a
}

// mergeProfilesMax merges two profiles factor-by-factor using the maximum level.
// NetworkType is merged toward the stronger behavior (Always > Conditional > None)
// and conditional subcommand lists are unioned.
func mergeProfilesMax(a, b CommandRiskProfile) CommandRiskProfile {
	merged := CommandRiskProfile{
		PrivilegeRisk:   maxFactor(a.PrivilegeRisk, b.PrivilegeRisk),
		NetworkRisk:     maxFactor(a.NetworkRisk, b.NetworkRisk),
		DestructionRisk: maxFactor(a.DestructionRisk, b.DestructionRisk),
		DataExfilRisk:   maxFactor(a.DataExfilRisk, b.DataExfilRisk),
		SystemModRisk:   maxFactor(a.SystemModRisk, b.SystemModRisk),
	}
	switch {
	case a.NetworkType == NetworkTypeAlways || b.NetworkType == NetworkTypeAlways:
		merged.NetworkType = NetworkTypeAlways
	case a.NetworkType == NetworkTypeConditional || b.NetworkType == NetworkTypeConditional:
		merged.NetworkType = NetworkTypeConditional
		merged.NetworkSubcommands = slices.Concat(a.NetworkSubcommands, b.NetworkSubcommands)
	default:
		merged.NetworkType = NetworkTypeNone
	}
	return merged
}

// ProfileNetworkApplies reports whether the profile's NetworkRisk factor applies
// to this invocation: always for NetworkTypeAlways, and for NetworkTypeConditional
// only when a network subcommand or a network-style argument (URL/SSH address) is
// present.
func ProfileNetworkApplies(profile CommandRiskProfile, args []string) bool {
	switch profile.NetworkType {
	case NetworkTypeAlways:
		return true
	case NetworkTypeConditional:
		if len(profile.NetworkSubcommands) > 0 {
			sub := findFirstSubcommand(args)
			if sub != "" && slices.Contains(profile.NetworkSubcommands, sub) {
				return true
			}
		}
		return hasNetworkArguments(args)
	default:
		return false
	}
}

// ProfileFactorRisk returns the non-privilege, non-system-modification risk
// factors of a command's profile (destruction, data exfiltration, and applicable
// network) as a single folded level plus the reason code for each contributing
// factor. Privilege is handled by the dedicated privilege gate and system
// modification by SystemModificationRisk, so neither is folded here. It returns
// RiskLevelUnknown with no codes when no factor applies. This is the single
// source for the profile dimension, used by the evaluator and the wrapped-inner
// indirect-execution path.
func ProfileFactorRisk(profile CommandRiskProfile, args []string) (runnertypes.RiskLevel, []risktypes.ReasonCode) {
	level := runnertypes.RiskLevelUnknown
	var codes []risktypes.ReasonCode
	if profile.DestructionRisk.Level > runnertypes.RiskLevelLow {
		level = max(level, profile.DestructionRisk.Level)
		codes = append(codes, risktypes.ReasonProfileDestruction)
	}
	if profile.DataExfilRisk.Level > runnertypes.RiskLevelLow {
		level = max(level, profile.DataExfilRisk.Level)
		codes = append(codes, risktypes.ReasonProfileDataExfil)
	}
	if profile.NetworkRisk.Level > runnertypes.RiskLevelLow && ProfileNetworkApplies(profile, args) {
		level = max(level, profile.NetworkRisk.Level)
		codes = append(codes, risktypes.ReasonProfileNetwork)
	}
	return level, codes
}

// Validate ensures consistency between risk factors and configuration
func (p CommandRiskProfile) Validate() error {
	// Rule 1: NetworkTypeAlways implies NetworkRisk >= Medium
	if p.NetworkType == NetworkTypeAlways && p.NetworkRisk.Level < runnertypes.RiskLevelMedium {
		return fmt.Errorf("%w (got %v)", ErrNetworkAlwaysRequiresMediumRisk, p.NetworkRisk.Level)
	}

	// Rule 2: NetworkSubcommands only for NetworkTypeConditional
	if len(p.NetworkSubcommands) > 0 && p.NetworkType != NetworkTypeConditional {
		return ErrNetworkSubcommandsOnlyForConditional
	}

	return nil
}
