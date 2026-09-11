package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// Environment variable names
const (
	// SlackWebhookURLSuccessEnvVar is the environment variable for success webhook
	SlackWebhookURLSuccessEnvVar = "GSCR_SLACK_WEBHOOK_URL_SUCCESS"

	// SlackWebhookURLErrorEnvVar is the environment variable for error webhook
	SlackWebhookURLErrorEnvVar = "GSCR_SLACK_WEBHOOK_URL_ERROR"
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
	// ErrorTypeGroupFileVerification represents group file verification failures
	ErrorTypeGroupFileVerification ErrorType = "group_file_verification_failed"
	// ErrorTypeInvalidRunID represents a --run-id value that does not match the
	// accepted format.
	ErrorTypeInvalidRunID ErrorType = "invalid_run_id"
)

// PreExecutionError represents an error that occurs before command execution
type PreExecutionError struct {
	Type      ErrorType
	Message   string
	Component string
	RunID     string
	// NotificationContext declares where the failure originated so the
	// notification record can name the group or command. Its zero value is a
	// valid global scope, but production call sites set it explicitly with
	// common.GlobalScope() or a group scope.
	NotificationContext common.NotificationContext
	Err                 error // Wrapped error for better error context preservation
}

// Error implements the error interface
func (e *PreExecutionError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v (component: %s, run_id: %s)", e.Type, e.Message, e.Err, e.Component, e.RunID)
	}
	return fmt.Sprintf("%s: %s (component: %s, run_id: %s)", e.Type, e.Message, e.Component, e.RunID)
}

// Detail returns the user-facing description of the failure: Message followed by
// the cause carried in Err. Reporting paths that render a single string (stderr,
// the structured log, the Slack alert) must use this rather than Message alone,
// or the cause — a TOML syntax error, a hash mismatch — never reaches the user.
func (e *PreExecutionError) Detail() string {
	if e.Err == nil {
		return e.Message
	}
	if userMsg := GetUserFriendlyMessage(e.Err); userMsg != "" {
		return fmt.Sprintf("%s: %s", e.Message, userMsg)
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
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
	summaryStatus string
	// notificationAttrs are recorded with the structured log line. The caller
	// decides which notification attributes, if any, the record carries.
	notificationAttrs []slog.Attr
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
		attrs := []slog.Attr{
			slog.String(common.PreExecErrorAttrs.ErrorType, string(params.errorType)),
			slog.String(common.PreExecErrorAttrs.ErrorMessage, params.errorMsg),
			slog.String(common.PreExecErrorAttrs.Component, params.component),
			slog.String("run_id", params.runID),
		}
		attrs = append(attrs, params.notificationAttrs...)
		logger.LogAttrs(context.Background(), slog.LevelError, params.slogMessage, attrs...)
	}

	// Build stdout output atomically to prevent interleaved output in concurrent scenarios
	var stdoutBuilder strings.Builder
	fmt.Fprintf(&stdoutBuilder, "Error: %s\nRUN_SUMMARY run_id=%s exit_code=1 status=%s duration_ms=0 verified=0 skipped=0 failed=0 warnings=0 errors=1\n", params.errorType, params.runID, params.summaryStatus)
	// Write to stdout atomically
	fmt.Print(stdoutBuilder.String())
}

// HandlePreExecutionError handles pre-execution errors by logging and notifying.
// The notification context is always attached because the report boundary is
// the only place that knows where the failure originated.
func HandlePreExecutionError(preExecErr *PreExecutionError) {
	handleErrorCommon(errorHandlingParams{
		errorType:     preExecErr.Type,
		errorMsg:      preExecErr.Detail(),
		component:     preExecErr.Component,
		runID:         preExecErr.RunID,
		slogMessage:   "Pre-execution error occurred",
		summaryStatus: "pre_execution_error",
		notificationAttrs: []slog.Attr{
			slog.Bool("slack_notify", true),
			slog.String("message_type", "pre_execution_error"),
			preExecErr.NotificationContext.LogAttr(),
		},
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
		summaryStatus: "execution_error",
		notificationAttrs: []slog.Attr{
			slog.Bool("slack_notify", false),
			slog.String("message_type", "execution_error"),
		},
	})
}
