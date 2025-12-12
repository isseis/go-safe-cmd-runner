package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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
func (e *PreExecutionError) As(target any) bool {
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

// errorHandlingParams contains parameters for error handling
type errorHandlingParams struct {
	errorType     ErrorType
	errorMsg      string
	component     string
	runID         string
	slogMessage   string
	slogMsgType   string
	summaryStatus string
	slackNotify   bool
}

// handleErrorCommon is a private helper that contains the common error handling logic
// for both pre-execution and execution errors
func handleErrorCommon(params errorHandlingParams) {
	// Build stderr output atomically to prevent interleaved output in concurrent scenarios
	var stderrBuilder strings.Builder
	fmt.Fprintf(&stderrBuilder, "Error: %s\n", params.errorType)
	if params.component != "" {
		fmt.Fprintf(&stderrBuilder, "  Component: %s\n", params.component)
	}
	fmt.Fprintf(&stderrBuilder, "  Details: %s\n", params.errorMsg)
	if params.runID != "" {
		fmt.Fprintf(&stderrBuilder, "  Run ID: %s\n", params.runID)
	}
	// Write to stderr atomically
	fmt.Fprint(os.Stderr, stderrBuilder.String())

	// Try to log through slog if available
	if logger := slog.Default(); logger != nil {
		slog.Error(params.slogMessage,
			slog.String(common.PreExecErrorAttrs.ErrorType, string(params.errorType)),
			slog.String(common.PreExecErrorAttrs.ErrorMessage, params.errorMsg),
			slog.String(common.PreExecErrorAttrs.Component, params.component),
			slog.String("run_id", params.runID),
			slog.Bool("slack_notify", params.slackNotify),
			slog.String("message_type", params.slogMsgType),
		)
	}

	// Build stdout output atomically to prevent interleaved output in concurrent scenarios
	var stdoutBuilder strings.Builder
	fmt.Fprintf(&stdoutBuilder, "Error: %s\nRUN_SUMMARY run_id=%s exit_code=1 status=%s duration_ms=0 verified=0 skipped=0 failed=0 warnings=0 errors=1\n", params.errorType, params.runID, params.summaryStatus)
	// Write to stdout atomically
	fmt.Print(stdoutBuilder.String())
}

// HandlePreExecutionError handles pre-execution errors by logging and notifying
func HandlePreExecutionError(errorType ErrorType, errorMsg, component, runID string) {
	handleErrorCommon(errorHandlingParams{
		errorType:     errorType,
		errorMsg:      errorMsg,
		component:     component,
		runID:         runID,
		slogMessage:   "Pre-execution error occurred",
		slogMsgType:   "pre_execution_error",
		summaryStatus: "pre_execution_error",
		slackNotify:   true,
	})
}

// HandleExecutionError handles execution errors (errors that occur during command execution)
// by logging and outputting appropriate summary information
func HandleExecutionError(execErr *ExecutionError) {
	// Build error message with context information
	message := execErr.Message

	// Check if the error provides a user-friendly message
	if userMsg := GetUserFriendlyMessage(execErr.Err); userMsg != "" {
		message = fmt.Sprintf("%s: %s", message, userMsg)
	} else if execErr.Err != nil {
		// If no user-friendly message, include the raw error
		message = fmt.Sprintf("%s: %v", message, execErr.Err)
	}

	// Add context information (group and command names)
	if contextStr := execErr.ContextString(); contextStr != "" {
		message = fmt.Sprintf("%s (%s)", message, contextStr)
	}

	handleErrorCommon(errorHandlingParams{
		errorType:     ErrorTypeSystemError,
		errorMsg:      message,
		component:     execErr.Component,
		runID:         execErr.RunID,
		slogMessage:   "Execution error occurred",
		slogMsgType:   "execution_error",
		summaryStatus: "execution_error",
		slackNotify:   false,
	})
}
