package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestProfileBuilder_Build(t *testing.T) {
	t.Run("valid privilege escalation profile", func(t *testing.T) {
		def := NewProfile("sudo").
			PrivilegeRisk(runnertypes.RiskLevelCritical, "Privilege escalation").
			Build()

		assert.Equal(t, []string{"sudo"}, def.Commands())
		assert.Equal(t, runnertypes.RiskLevelCritical, def.Profile().BaseRiskLevel())
		assert.True(t, def.Profile().IsPrivilege())
	})

	t.Run("valid network profile", func(t *testing.T) {
		def := NewProfile("curl", "wget").
			NetworkRisk(runnertypes.RiskLevelMedium, "Network operations").
			AlwaysNetwork().
			Build()

		assert.Equal(t, []string{"curl", "wget"}, def.Commands())
		assert.Equal(t, runnertypes.RiskLevelMedium, def.Profile().BaseRiskLevel())
		assert.Equal(t, NetworkTypeAlways, def.Profile().NetworkType)
	})

	t.Run("valid conditional network profile", func(t *testing.T) {
		def := NewProfile("git").
			NetworkRisk(runnertypes.RiskLevelMedium, "Network operations").
			ConditionalNetwork("clone", "fetch", "pull", "push").
			Build()

		assert.Equal(t, []string{"git"}, def.Commands())
		assert.Equal(t, NetworkTypeConditional, def.Profile().NetworkType)
		assert.Equal(t, []string{"clone", "fetch", "pull", "push"}, def.Profile().NetworkSubcommands)
	})

	t.Run("multiple risk factors", func(t *testing.T) {
		def := NewProfile("claude").
			NetworkRisk(runnertypes.RiskLevelHigh, "AI API communication").
			DataExfilRisk(runnertypes.RiskLevelHigh, "Data exfiltration").
			AlwaysNetwork().
			Build()

		assert.Equal(t, runnertypes.RiskLevelHigh, def.Profile().BaseRiskLevel())
		reasons := def.Profile().GetRiskReasons()
		assert.Contains(t, reasons, "AI API communication")
		assert.Contains(t, reasons, "Data exfiltration")
	})

	t.Run("invalid - NetworkTypeAlways with low risk should panic", func(t *testing.T) {
		assert.Panics(t, func() {
			NewProfile("test").
				NetworkRisk(runnertypes.RiskLevelLow, "test").
				AlwaysNetwork().
				Build()
		})
	})

	t.Run("default values for unset risks", func(t *testing.T) {
		def := NewProfile("test").Build()

		profile := def.Profile()
		assert.Equal(t, runnertypes.RiskLevelUnknown, profile.PrivilegeRisk.Level)
		assert.Equal(t, runnertypes.RiskLevelUnknown, profile.NetworkRisk.Level)
		assert.Equal(t, runnertypes.RiskLevelUnknown, profile.DestructionRisk.Level)
		assert.Equal(t, runnertypes.RiskLevelUnknown, profile.DataExfilRisk.Level)
		assert.Equal(t, runnertypes.RiskLevelUnknown, profile.SystemModRisk.Level)
	})
}
