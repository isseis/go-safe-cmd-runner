package security

// ValidatorInterface defines the interface for security validation
// This interface is introduced for testing purposes
type ValidatorInterface interface {
	ValidateAllEnvironmentVars(envVars map[string]string) error
	ValidateEnvironmentValue(key, value string) error
	ValidateCommand(command string) error
	// SanitizeOutputForLogging redacts sensitive information from command output
	// This is used for Case 2 (supplementary measure) in task 0055
	SanitizeOutputForLogging(output string) string
}

// Ensure Validator implements ValidatorInterface
var _ ValidatorInterface = (*Validator)(nil)
