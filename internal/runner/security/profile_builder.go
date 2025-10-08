package security

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ProfileBuilder provides a fluent API for building CommandRiskProfileNew.
//
// Example usage:
//
//	// Simple privilege escalation command
//	NewProfile("sudo", "su").
//	    PrivilegeRisk(runnertypes.RiskLevelCritical, "Allows execution with elevated privileges").
//	    Build()
//
//	// Network command with data exfiltration risk
//	NewProfile("curl", "wget").
//	    NetworkRisk(runnertypes.RiskLevelMedium, "Downloads data from network").
//	    DataExfilRisk(runnertypes.RiskLevelLow, "Can send data to external servers").
//	    AlwaysNetwork().
//	    Build()
//
//	// Conditional network command
//	NewProfile("git").
//	    NetworkRisk(runnertypes.RiskLevelMedium, "Network operations for specific subcommands").
//	    ConditionalNetwork("clone", "fetch", "pull", "push").
//	    Build()
type ProfileBuilder struct {
	commands           []string
	privilegeRisk      *RiskFactor
	networkRisk        *RiskFactor
	destructionRisk    *RiskFactor
	dataExfilRisk      *RiskFactor
	systemModRisk      *RiskFactor
	networkType        NetworkOperationType
	networkSubcommands []string
}

// NewProfile creates a new profile builder for the given commands
func NewProfile(commands ...string) *ProfileBuilder {
	return &ProfileBuilder{
		commands:    commands,
		networkType: NetworkTypeNone,
	}
}

// PrivilegeRisk sets the privilege escalation risk factor
func (b *ProfileBuilder) PrivilegeRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
	b.privilegeRisk = &RiskFactor{Level: level, Reason: reason}
	return b
}

// NetworkRisk sets the network operation risk factor
func (b *ProfileBuilder) NetworkRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
	b.networkRisk = &RiskFactor{Level: level, Reason: reason}
	return b
}

// DestructionRisk sets the destructive operation risk factor
func (b *ProfileBuilder) DestructionRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
	b.destructionRisk = &RiskFactor{Level: level, Reason: reason}
	return b
}

// DataExfilRisk sets the data exfiltration risk factor
func (b *ProfileBuilder) DataExfilRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
	b.dataExfilRisk = &RiskFactor{Level: level, Reason: reason}
	return b
}

// SystemModRisk sets the system modification risk factor
func (b *ProfileBuilder) SystemModRisk(level runnertypes.RiskLevel, reason string) *ProfileBuilder {
	b.systemModRisk = &RiskFactor{Level: level, Reason: reason}
	return b
}

// AlwaysNetwork marks the command as always performing network operations
func (b *ProfileBuilder) AlwaysNetwork() *ProfileBuilder {
	b.networkType = NetworkTypeAlways
	return b
}

// ConditionalNetwork marks the command as conditionally performing network operations
func (b *ProfileBuilder) ConditionalNetwork(subcommands ...string) *ProfileBuilder {
	b.networkType = NetworkTypeConditional
	b.networkSubcommands = subcommands
	return b
}

// Build creates the final CommandProfileDef with validation
func (b *ProfileBuilder) Build() CommandProfileDef {
	profile := CommandRiskProfileNew{
		PrivilegeRisk:      b.getOrDefault(b.privilegeRisk),
		NetworkRisk:        b.getOrDefault(b.networkRisk),
		DestructionRisk:    b.getOrDefault(b.destructionRisk),
		DataExfilRisk:      b.getOrDefault(b.dataExfilRisk),
		SystemModRisk:      b.getOrDefault(b.systemModRisk),
		NetworkType:        b.networkType,
		NetworkSubcommands: b.networkSubcommands,
	}

	// Validate at build time
	if err := profile.Validate(); err != nil {
		panic(fmt.Sprintf("invalid profile for commands %v: %v", b.commands, err))
	}

	return CommandProfileDef{
		commands: b.commands,
		profile:  profile,
	}
}

func (b *ProfileBuilder) getOrDefault(risk *RiskFactor) RiskFactor {
	if risk == nil {
		return RiskFactor{Level: runnertypes.RiskLevelUnknown}
	}
	return *risk
}
