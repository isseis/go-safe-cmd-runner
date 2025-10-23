package config

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// Default values for configuration fields
const (
	DefaultVerifyStandardPaths = true
	DefaultRiskLevel           = "low"
)

// ApplyGlobalDefaults applies default values to GlobalSpec fields
func ApplyGlobalDefaults(spec *runnertypes.GlobalSpec) {
	if spec.VerifyStandardPaths == nil {
		defaultValue := DefaultVerifyStandardPaths
		spec.VerifyStandardPaths = &defaultValue
	}
}

// ApplyCommandDefaults applies default values to CommandSpec fields
func ApplyCommandDefaults(spec *runnertypes.CommandSpec) {
	if spec.RiskLevel == "" {
		spec.RiskLevel = DefaultRiskLevel
	}
}
