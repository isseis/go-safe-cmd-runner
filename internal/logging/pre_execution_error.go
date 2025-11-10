package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Environment variable names
const (
	// SlackWebhookURLEnvVar is the environment variable name for Slack webhook URL
	SlackWebhookURLEnvVar = "GSCR_SLACK_WEBHOOK_URL"
)

// ErrorType represents different types of pre-execution errors
type ErrorType string

const (
	// ErrorTypeConfigParsing represents configuration parsing failures
	ErrorTypeConfigParsing ErrorType = "config_parsing_failed"
	// ErrorTypeLogFileOpen represents log file opening failures
	ErrorTypeLogFileOpen ErrorType = "log_file_open_failed"
	// ErrorTypePrivilegeDrop represents privilege dropping failures
	ErrorTypePrivilegeDrop ErrorType = "privilege_drop_failed"
	// ErrorTypeFileAccess represents file access failures
	ErrorTypeFileAccess ErrorType = "file_access_failed"
	// ErrorTypeUserInterrupted represents user interruption
	ErrorTypeUserInterrupted ErrorType = "user_interrupted"
	// ErrorTypeRequiredArgumentMissing represents missing required argument errors
	ErrorTypeRequiredArgumentMissing ErrorType = "required_argument_missing"
	// ErrorTypeBuildConfig represents build-time configuration errors
	ErrorTypeBuildConfig ErrorType = "build_config_error"
	// ErrorTypeSystemError represents system errors
	ErrorTypeSystemError ErrorType = "system_error"
)

// PreExecutionError represents an error that occurs before command execution
type PreExecutionError struct {
	Type      ErrorType
	Message   string
	Component string
	RunID     string
	Err       error // Wrapped error for better error context preservation
}

// Error implements the error interface
func (e *PreExecutionError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v (component: %s, run_id: %s)", e.Type, e.Message, e.Err, e.Component, e.RunID)
	}
	return fmt.Sprintf("%s: %s (component: %s, run_id: %s)", e.Type, e.Message, e.Component, e.RunID)
}

// Is implements error wrapping for errors.Is
func (e *PreExecutionError) Is(target error) bool {
	_, ok := target.(*PreExecutionError)
	return ok
}

// As implements error wrapping for errors.As
func (e *PreExecutionError) As(target interface{}) bool {
	if preExecErr, ok := target.(**PreExecutionError); ok {
		*preExecErr = e
		return true
	}
	return false
}

// Unwrap implements error wrapping for errors.Unwrap
func (e *PreExecutionError) Unwrap() error {
	return e.Err
}

// handleErrorCommon is a private helper that contains the common error handling logic
// for both pre-execution and execution errors
func handleErrorCommon(errorType ErrorType, errorMsg, component, runID, slogMessage, slogMessageType, summaryStatus string) {
	// Build stderr output atomically to prevent interleaved output in concurrent scenarios
	var stderrBuilder strings.Builder
	fmt.Fprintf(&stderrBuilder, "Error: %s\n", errorType)
	if component != "" {
		fmt.Fprintf(&stderrBuilder, "  Component: %s\n", component)
	}
	fmt.Fprintf(&stderrBuilder, "  Details: %s\n", errorMsg)
	if runID != "" {
		fmt.Fprintf(&stderrBuilder, "  Run ID: %s\n", runID)
	}
	// Write to stderr atomically
	fmt.Fprint(os.Stderr, stderrBuilder.String())

	// Try to log through slog if available
	if logger := slog.Default(); logger != nil {
		slog.Error(slogMessage,
			"error_type", string(errorType),
			"error_message", errorMsg,
			"component", component,
			"run_id", runID,
			"slack_notify", true,
			"message_type", slogMessageType,
		)
	}

	// Build stdout output atomically to prevent interleaved output in concurrent scenarios
	var stdoutBuilder strings.Builder
	fmt.Fprintf(&stdoutBuilder, "Error: %s\nRUN_SUMMARY run_id=%s exit_code=1 status=%s duration_ms=0 verified=0 skipped=0 failed=0 warnings=0 errors=1\n", errorType, runID, summaryStatus)
	// Write to stdout atomically
	fmt.Print(stdoutBuilder.String())
}

// HandlePreExecutionError handles pre-execution errors by logging and notifying
func HandlePreExecutionError(errorType ErrorType, errorMsg, component, runID string) {
	handleErrorCommon(errorType, errorMsg, component, runID, "Pre-execution error occurred", "pre_execution_error", "pre_execution_error")
}

// HandleExecutionError handles execution errors (errors that occur during command execution)
// by logging and outputting appropriate summary information
func HandleExecutionError(execErr *ExecutionError) {
	// Build error message with context information
	message := execErr.Message

	// Check if this is an output size limit error and add specific information
	if isLimitError, outputPath := IsOutputSizeLimitError(execErr.Err); isLimitError {
		message = fmt.Sprintf("%s: output size limit exceeded for '%s'", message, outputPath)
	}

	// Add context information (group and command names)
	if contextStr := execErr.ContextString(); contextStr != "" {
		message = fmt.Sprintf("%s (%s)", message, contextStr)
	}

	handleErrorCommon(ErrorTypeSystemError, message, execErr.Component, execErr.RunID, "Execution error occurred", "execution_error", "execution_error")
}
