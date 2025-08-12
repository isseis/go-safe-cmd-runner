package logging

import (
	"fmt"
	"log/slog"
	"os"
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
	// ErrorTypeInvalidArguments represents invalid argument errors
	ErrorTypeInvalidArguments ErrorType = "invalid_arguments"
	// ErrorTypeSystemError represents system errors
	ErrorTypeSystemError ErrorType = "system_error"
)

// PreExecutionError represents an error that occurs before command execution
type PreExecutionError struct {
	Type      ErrorType
	Message   string
	Component string
	RunID     string
}

// Error implements the error interface
func (e *PreExecutionError) Error() string {
	return fmt.Sprintf("%s: %s (component: %s, run_id: %s)", e.Type, e.Message, e.Component, e.RunID)
}

// HandlePreExecutionError handles pre-execution errors by logging and notifying
func HandlePreExecutionError(errorType ErrorType, errorMsg, component, runID string) {
	// Log to stderr as fallback (in case logging system isn't set up yet)
	fmt.Fprintf(os.Stderr, "Error: %s - %s (run_id: %s)\n", errorType, errorMsg, runID)

	// Try to log through slog if available
	if logger := slog.Default(); logger != nil {
		slog.Error("Pre-execution error occurred",
			"error_type", string(errorType),
			"error_message", errorMsg,
			"component", component,
			"run_id", runID,
			"slack_notify", true,
			"message_type", "pre_execution_error",
		)
	}

	// Output error summary
	fmt.Printf("Error: %s\n", errorType)
	fmt.Printf("RUN_SUMMARY run_id=%s exit_code=1 status=pre_execution_error duration_ms=0 verified=0 skipped=0 failed=0 warnings=0 errors=1\n", runID)
}

// NotifyPreExecutionErrorAsync sends a pre-execution error notification asynchronously
func NotifyPreExecutionErrorAsync(errorType ErrorType, errorMsg, component, runID string) {
	go func() {
		// This will be handled by the SlackHandler if configured
		slog.Error("Pre-execution error notification",
			"error_type", string(errorType),
			"error_message", errorMsg,
			"component", component,
			"run_id", runID,
			"slack_notify", true,
			"message_type", "pre_execution_error",
		)
	}()
}

// LogCommandGroupSummary logs a command group execution summary
func LogCommandGroupSummary(group, command, status string, exitCode int, duration int64, output, runID string) {
	slog.Info("Command group execution completed",
		"group", group,
		"command", command,
		"status", status,
		"exit_code", exitCode,
		"duration_ms", duration,
		"output", output,
		"run_id", runID,
		"slack_notify", true,
		"message_type", "command_group_summary",
	)
}

// GetSlackWebhookURL gets the Slack webhook URL from environment
func GetSlackWebhookURL() string {
	// Try different environment variable names
	urls := []string{
		os.Getenv("SLACK_WEBHOOK_URL"),
		os.Getenv("SLACK_URL"),
		os.Getenv("WEBHOOK_URL"),
	}

	for _, url := range urls {
		if url != "" {
			return url
		}
	}

	return ""
}
