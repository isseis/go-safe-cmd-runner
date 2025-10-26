package security

// ValidatorInterface defines the interface for security validation
// This interface is introduced for testing purposes
type ValidatorInterface interface {
	ValidateAllEnvironmentVars(envVars map[string]string) error
	ValidateEnvironmentValue(key, value string) error
	ValidateCommand(command string) error
}

// Ensure Validator implements ValidatorInterface
var _ ValidatorInterface = (*Validator)(nil)
