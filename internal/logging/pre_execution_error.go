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
	// ErrorTypeGroupPreparation represents a group or command preparation failure
	// (expansion or working directory resolution) before any command ran.
	ErrorTypeGroupPreparation ErrorType = "group_preparation_failed"
	// ErrorTypeGroupDirPermissionViolation represents a group-level directory
	// permission audit violation.
	ErrorTypeGroupDirPermissionViolation ErrorType = "group_dir_permission_violation"
	// ErrorTypeCommandVerification represents a command-level verification
	// failure (path re-resolution or dependency verification).
	ErrorTypeCommandVerification ErrorType = "command_verification_failed"
	// ErrorTypeGroupPreExecution is the generic type for a group pre-execution
	// failure whose stage was not declared.
	ErrorTypeGroupPreExecution ErrorType = "group_pre_execution_failed"
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
	// FailedFilePaths is the list of failed targets to report to notification
	// consumers. It is a copy of verification.Error.Details, which the manager
	// already normalized into ascending order. The list travels as a structured
	// attribute and is rendered by the notification builder; it is never
	// concatenated into the human-readable Message, which stays free of paths.
	FailedFilePaths []string
	Err             error // Wrapped error for better error context preservation
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
	return fmt.Sprintf("%s: %s", e.Message, formatCause(e.Err))
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

// errorRecordParams are the fields of the structured error log line. They are
// separate from errorHandlingParams because the record-only notification path
// has no RUN_SUMMARY status to carry.
type errorRecordParams struct {
	errorType   ErrorType
	errorMsg    string
	component   string
	runID       string
	slogMessage string
	// notificationAttrs are recorded with the structured log line. The caller
	// decides which notification attributes, if any, the record carries.
	notificationAttrs []slog.Attr
}

// errorHandlingParams contains parameters for error handling
type errorHandlingParams struct {
	record        errorRecordParams
	summaryStatus string
}

// stderrDetailsPrefix labels the message line of the stderr report, and
// stderrDetailsIndent is the same width in spaces.
const stderrDetailsPrefix = "  Details: "

var stderrDetailsIndent = strings.Repeat(" ", len(stderrDetailsPrefix))

// handleErrorCommon is a private helper that contains the common error handling logic
// for both pre-execution and execution errors
func handleErrorCommon(params errorHandlingParams) {
	record := params.record
	// Build stderr output atomically to prevent interleaved output in concurrent scenarios
	var stderrBuilder strings.Builder
	fmt.Fprintf(&stderrBuilder, "Error: %s\n", record.errorType)
	if record.component != "" {
		fmt.Fprintf(&stderrBuilder, "  Component: %s\n", record.component)
	}
	// Continuation lines of a multi-line message are aligned under the first
	// so they stay visibly inside the Details field.
	fmt.Fprintf(&stderrBuilder, "%s%s\n", stderrDetailsPrefix,
		strings.ReplaceAll(record.errorMsg, "\n", "\n"+stderrDetailsIndent))
	if record.runID != "" {
		fmt.Fprintf(&stderrBuilder, "  Run ID: %s\n", record.runID)
	}
	// Write to stderr atomically
	fmt.Fprint(os.Stderr, stderrBuilder.String())

	writeErrorLogRecord(record)

	// Build stdout output atomically to prevent interleaved output in concurrent scenarios
	var stdoutBuilder strings.Builder
	fmt.Fprintf(&stdoutBuilder, "Error: %s\nRUN_SUMMARY run_id=%s exit_code=1 status=%s duration_ms=0 verified=0 skipped=0 failed=0 warnings=0 errors=1\n", record.errorType, record.runID, params.summaryStatus)
	// Write to stdout atomically
	fmt.Print(stdoutBuilder.String())
}

// writeErrorLogRecord writes the structured log line for an error report. It is
// the single place that assembles the standard record attributes and appends
// the caller's notification attributes, so the report path and the record-only
// notification path cannot drift.
func writeErrorLogRecord(params errorRecordParams) {
	logger := slog.Default()
	if logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String(common.PreExecErrorAttrs.ErrorType, string(params.errorType)),
		slog.String(common.PreExecErrorAttrs.ErrorMessage, params.errorMsg),
		slog.String(common.PreExecErrorAttrs.Component, params.component),
		slog.String("run_id", params.runID),
	}
	attrs = append(attrs, params.notificationAttrs...)
	logger.LogAttrs(context.Background(), slog.LevelError, params.slogMessage, attrs...)
}

// preExecutionNotificationAttrs builds the notification attributes of a
// pre-execution error record. The failed-file list only reaches the
// notification when there is one: an empty list is left off the record entirely
// so the builder cannot mistake "no targets" for "a list with zero entries".
func preExecutionNotificationAttrs(preExecErr *PreExecutionError) []slog.Attr {
	notificationAttrs := NotificationAttrs(
		PreExecutionErrorNotification(), preExecErr.NotificationContext)
	if len(preExecErr.FailedFilePaths) > 0 {
		notificationAttrs = append(notificationAttrs,
			slog.Any(common.PreExecErrorAttrs.FailedFilePaths, preExecErr.FailedFilePaths))
	}
	return notificationAttrs
}

// preExecutionRecordParams assembles the record of a pre-execution error. Both
// the report path and the record-only notification path go through it so their
// records carry the same attributes and message body.
func preExecutionRecordParams(preExecErr *PreExecutionError, slogMessage string) errorRecordParams {
	return errorRecordParams{
		errorType:         preExecErr.Type,
		errorMsg:          preExecErr.Detail(),
		component:         preExecErr.Component,
		runID:             preExecErr.RunID,
		slogMessage:       slogMessage,
		notificationAttrs: preExecutionNotificationAttrs(preExecErr),
	}
}

// HandlePreExecutionError handles pre-execution errors by logging and notifying.
// The notification context is always attached because the report boundary is
// the only place that knows where the failure originated.
func HandlePreExecutionError(preExecErr *PreExecutionError) {
	handleErrorCommon(errorHandlingParams{
		record:        preExecutionRecordParams(preExecErr, "Pre-execution error occurred"),
		summaryStatus: preExecutionErrorSummaryStatus(),
	})
}

// NotifyPreExecutionError records the pre-execution error notification record
// only. Unlike HandlePreExecutionError it writes neither the stderr report nor
// the RUN_SUMMARY line: the caller continues, and the process-level report is
// made once at the end of the run.
func NotifyPreExecutionError(preExecErr *PreExecutionError) {
	writeErrorLogRecord(preExecutionRecordParams(preExecErr, "Pre-execution error notified"))
}

// HandleExecutionError handles execution errors (errors that occur during command execution)
// by logging and outputting appropriate summary information
func HandleExecutionError(execErr *ExecutionError) {
	// The cause is rendered by formatCause, the same helper PreExecutionError.Detail
	// uses, so both report paths agree on friendly-versus-raw text and on how a
	// joined multi-error is split. Only the surrounding "Message: cause" assembly
	// is repeated, because the two paths report different error types; see
	// issue #1156.
	message := execErr.Message

	// The context (group and command names) goes right after Message, before
	// the cause: a cause may span several lines (e.g. an errors.Join of group
	// errors), and a suffix would read as belonging to its last line only.
	if contextStr := execErr.ContextString(); contextStr != "" {
		message = fmt.Sprintf("%s (%s)", message, contextStr)
	}

	if execErr.Err != nil {
		message = fmt.Sprintf("%s: %s", message, formatCause(execErr.Err))
	}

	handleErrorCommon(errorHandlingParams{
		record: errorRecordParams{
			errorType:   ErrorTypeSystemError,
			errorMsg:    message,
			component:   execErr.Component,
			runID:       execErr.RunID,
			slogMessage: "Execution error occurred",
			notificationAttrs: []slog.Attr{
				slog.Bool("slack_notify", false),
				slog.String("message_type", "execution_error"),
			},
		},
		summaryStatus: "execution_error",
	})
}
