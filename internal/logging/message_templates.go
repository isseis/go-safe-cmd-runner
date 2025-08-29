package logging

// MessageTemplates provides common message templates and formatting utilities
// for consistent log message formatting across the application.
type MessageTemplates struct{}

// NewMessageTemplates creates a new MessageTemplates instance.
func NewMessageTemplates() *MessageTemplates {
	return &MessageTemplates{}
}

// Common message templates for different log scenarios
const (
	// Command execution messages
	CommandStartTemplate    = "Starting command execution"
	CommandCompleteTemplate = "Command execution completed"
	CommandFailedTemplate   = "Command execution failed"

	// Environment processing messages
	EnvLoadTemplate     = "Loading environment variables"
	EnvProcessTemplate  = "Processing environment configuration"
	EnvValidateTemplate = "Validating environment variables"

	// File operation messages
	FileReadTemplate     = "Reading file"
	FileWriteTemplate    = "Writing file"
	FileValidateTemplate = "Validating file integrity"

	// Security messages
	SecurityCheckTemplate   = "Performing security validation"
	SecurityDeniedTemplate  = "Security check denied access"
	SecurityWarningTemplate = "Security warning detected"

	// System messages
	SystemStartTemplate    = "System initialization started"
	SystemReadyTemplate    = "System ready for operation"
	SystemShutdownTemplate = "System shutdown initiated"
)

// LogFileHintTemplates provides templates for log file hints
const (
	LogFileHintPrefix     = "Check log file around line"
	LogFileHintSuffix     = "for more details"
	LogFileHintFullFormat = LogFileHintPrefix + " %d " + LogFileHintSuffix
)

// FormatCommandMessage formats command-related messages with consistent structure
func (t *MessageTemplates) FormatCommandMessage(template, commandName string, _ map[string]any) string {
	// This could be expanded to use text/template for more complex formatting
	// For now, keep it simple
	return template + " command=" + commandName
}

// FormatSecurityMessage formats security-related messages with appropriate severity indicators
func (t *MessageTemplates) FormatSecurityMessage(template, operation string, severity string) string {
	return template + " operation=" + operation + " severity=" + severity
}

// FormatSystemMessage formats system-level messages with context
func (t *MessageTemplates) FormatSystemMessage(template, component string) string {
	return template + " component=" + component
}
