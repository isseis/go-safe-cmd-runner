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
func ApplyCommandDefaults(_ *runnertypes.CommandSpec) {
	// RiskLevel is now a pointer type, so we don't need to apply defaults here.
	// nil means "use default", which is handled by GetRiskLevel() method.
	// This change allows proper inheritance from templates.
}
