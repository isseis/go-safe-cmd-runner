package config

import "errors"

// Configuration loading and expansion errors
var (
	// ErrGlobalEnvExpansionFailed is returned when global environment variable expansion fails
	ErrGlobalEnvExpansionFailed = errors.New("global environment variable expansion failed")

	// ErrGroupEnvExpansionFailed is returned when group environment variable expansion fails
	ErrGroupEnvExpansionFailed = errors.New("group environment variable expansion failed")

	// ErrDuplicateEnvVariable is returned when duplicate environment variable keys are detected
	ErrDuplicateEnvVariable = errors.New("duplicate environment variable key")

	// ErrMalformedEnvVariable is returned when an environment variable is not in KEY=VALUE format
	ErrMalformedEnvVariable = errors.New("malformed environment variable (expected KEY=VALUE format)")

	// ErrInvalidEnvKey is returned when an environment variable key contains invalid characters
	ErrInvalidEnvKey = errors.New("invalid environment variable key")

	// ErrReservedEnvPrefix is returned when an environment variable key uses a reserved prefix
	ErrReservedEnvPrefix = errors.New("environment variable key uses reserved prefix")
)
