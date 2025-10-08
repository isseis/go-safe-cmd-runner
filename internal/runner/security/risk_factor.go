package security

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// RiskFactor represents an individual risk factor with its level and explanation
type RiskFactor struct {
	Level  runnertypes.RiskLevel // Risk level for this specific factor
	Reason string                // Human-readable explanation of this risk
}
