package audit_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestRiskStatistics(t *testing.T) {
	t.Run("new statistics", func(t *testing.T) {
		stats := audit.NewRiskStatistics()
		assert.NotNil(t, stats)
		assert.Equal(t, 0, stats.TotalCommands())
	})

	t.Run("record command execution", func(t *testing.T) {
		stats := audit.NewRiskStatistics()

		stats.RecordCommand("curl", runnertypes.RiskLevelMedium, []string{"Always performs network operations"})
		stats.RecordCommand("sudo", runnertypes.RiskLevelCritical, []string{"Privilege escalation"})
		stats.RecordCommand("ls", runnertypes.RiskLevelUnknown, []string{})

		assert.Equal(t, 3, stats.TotalCommands())
	})

	t.Run("get risk level counts", func(t *testing.T) {
		stats := audit.NewRiskStatistics()

		stats.RecordCommand("curl", runnertypes.RiskLevelMedium, []string{"Network"})
		stats.RecordCommand("wget", runnertypes.RiskLevelMedium, []string{"Network"})
		stats.RecordCommand("sudo", runnertypes.RiskLevelCritical, []string{"Privilege"})
		stats.RecordCommand("rm", runnertypes.RiskLevelHigh, []string{"Destructive"})
		stats.RecordCommand("ls", runnertypes.RiskLevelUnknown, []string{})

		counts := stats.GetRiskLevelCounts()
		assert.Equal(t, 1, counts[runnertypes.RiskLevelUnknown])
		assert.Equal(t, 2, counts[runnertypes.RiskLevelMedium])
		assert.Equal(t, 1, counts[runnertypes.RiskLevelHigh])
		assert.Equal(t, 1, counts[runnertypes.RiskLevelCritical])
	})

	t.Run("get most common risk factors", func(t *testing.T) {
		stats := audit.NewRiskStatistics()

		stats.RecordCommand("curl", runnertypes.RiskLevelMedium, []string{"Network operations"})
		stats.RecordCommand("wget", runnertypes.RiskLevelMedium, []string{"Network operations"})
		stats.RecordCommand("ssh", runnertypes.RiskLevelMedium, []string{"Network operations"})
		stats.RecordCommand("rm", runnertypes.RiskLevelHigh, []string{"Destructive operations"})
		stats.RecordCommand("dd", runnertypes.RiskLevelCritical, []string{"Destructive operations"})

		topFactors := stats.GetTopRiskFactors(3)
		assert.Equal(t, 2, len(topFactors))
		assert.Equal(t, "Network operations", topFactors[0].Factor)
		assert.Equal(t, 3, topFactors[0].Count)
		assert.Equal(t, "Destructive operations", topFactors[1].Factor)
		assert.Equal(t, 2, topFactors[1].Count)
	})

	t.Run("get command counts by risk level", func(t *testing.T) {
		stats := audit.NewRiskStatistics()

		stats.RecordCommand("curl", runnertypes.RiskLevelMedium, []string{"Network"})
		stats.RecordCommand("curl", runnertypes.RiskLevelMedium, []string{"Network"})
		stats.RecordCommand("sudo", runnertypes.RiskLevelCritical, []string{"Privilege"})
		stats.RecordCommand("ls", runnertypes.RiskLevelUnknown, []string{})

		commands := stats.GetCommandsByRiskLevel(runnertypes.RiskLevelMedium)
		assert.Contains(t, commands, "curl")
		assert.Equal(t, 1, len(commands))

		commands = stats.GetCommandsByRiskLevel(runnertypes.RiskLevelCritical)
		assert.Contains(t, commands, "sudo")
		assert.Equal(t, 1, len(commands))
	})
}
