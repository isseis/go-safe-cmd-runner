package security

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// RiskFactor represents an individual risk factor with its level and explanation.
//
// Each risk factor consists of:
//   - Level: The severity of this specific risk (Unknown, Low, Medium, High, Critical)
//   - Reason: A human-readable explanation of why this risk exists
//
// Risk factors are combined in CommandRiskProfileNew to provide a comprehensive
// risk assessment. The overall risk level is the maximum of all factor levels.
type RiskFactor struct {
	Level  runnertypes.RiskLevel // Risk level for this specific factor
	Reason string                // Human-readable explanation of this risk
}
