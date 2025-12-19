package config

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// Default values for configuration fields
const (
	DefaultVerifyStandardPaths = true
)

// ApplyGlobalDefaults applies default values to GlobalSpec fields
func ApplyGlobalDefaults(spec *runnertypes.GlobalSpec) {
	if spec.VerifyStandardPaths == nil {
		defaultValue := DefaultVerifyStandardPaths
		spec.VerifyStandardPaths = &defaultValue
	}
}
