package environment

import (
	"errors"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ValidateUserEnvNames validates that user-defined env variable names do not use reserved prefixes.
// It returns an error if any variable name starts with the reserved prefix.
func ValidateUserEnvNames(userEnv map[string]string) error {
	var errs []error
	for name := range userEnv {
		if strings.HasPrefix(name, AutoEnvPrefix) {
			errs = append(errs, runnertypes.NewReservedEnvPrefixError(name, AutoEnvPrefix))
		}
	}
	return errors.Join(errs...)
}
