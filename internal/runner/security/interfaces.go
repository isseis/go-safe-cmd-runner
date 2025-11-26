package security

// ValidatorInterface defines the interface for security validation
// This interface is introduced for testing purposes
type ValidatorInterface interface {
	ValidateAllEnvironmentVars(envVars map[string]string) error
	ValidateEnvironmentValue(key, value string) error
	ValidateCommand(command string) error
	// ValidateCommandAllowed checks whether a command path is permitted for execution.
	// It validates against global AllowedCommands patterns OR group-level cmd_allowed list.
	// Returns nil if allowed, or *CommandNotAllowedError if not permitted.
	ValidateCommandAllowed(cmdPath string, groupCmdAllowed []string) error
	// SanitizeOutputForLogging redacts sensitive information from command output
	// This helps prevent sensitive data from being logged or sent to external systems such as Slack.
	SanitizeOutputForLogging(output string) string
}

// Ensure Validator implements ValidatorInterface
var _ ValidatorInterface = (*Validator)(nil)
