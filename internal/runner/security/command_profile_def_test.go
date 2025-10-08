package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestCommandProfileDef_Commands_ReturnsDefensiveCopy(t *testing.T) {
	// Create a profile with some commands
	def := NewProfile("git", "svn").
		NetworkRisk(runnertypes.RiskLevelMedium, "Version control").
		Build()

	// Get the commands
	commands1 := def.Commands()
	assert.Equal(t, []string{"git", "svn"}, commands1)

	// Modify the returned slice
	commands1[0] = "modified"
	commands1[1] = "changed"

	// Get commands again and verify the original is unchanged
	commands2 := def.Commands()
	assert.Equal(t, []string{"git", "svn"}, commands2, "original commands should be unchanged")
	assert.NotEqual(t, commands1, commands2, "returned slices should be independent")
}

func TestCommandProfileDef_Commands_NilSlice(t *testing.T) {
	// Create a profile with empty commands (shouldn't normally happen, but test the nil case)
	def := CommandProfileDef{
		commands: nil,
		profile: CommandRiskProfile{
			NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelLow},
		},
	}

	commands := def.Commands()
	assert.Nil(t, commands, "nil commands should return nil")
}

func TestCommandProfileDef_Commands_EmptySlice(t *testing.T) {
	// Create a profile with empty commands slice
	def := CommandProfileDef{
		commands: []string{},
		profile: CommandRiskProfile{
			NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelLow},
		},
	}

	commands := def.Commands()
	assert.Empty(t, commands, "empty commands should return empty slice")
	assert.NotNil(t, commands, "empty slice should not be nil")
}

func TestCommandProfileDef_Profile(t *testing.T) {
	// Create a profile
	def := NewProfile("test").
		NetworkRisk(runnertypes.RiskLevelHigh, "Test risk").
		DestructionRisk(runnertypes.RiskLevelMedium, "Some destruction").
		Build()

	profile := def.Profile()
	assert.Equal(t, runnertypes.RiskLevelHigh, profile.NetworkRisk.Level)
	assert.Equal(t, "Test risk", profile.NetworkRisk.Reason)
	assert.Equal(t, runnertypes.RiskLevelMedium, profile.DestructionRisk.Level)
	assert.Equal(t, "Some destruction", profile.DestructionRisk.Reason)
}
