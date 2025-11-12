// Package common provides shared types and utilities used across the application
package common

// Log field keys for CommandResult structured logging
// These constants ensure consistency between log value creation (runner.CommandResult.LogValue())
// and log attribute extraction (logging.extractCommandResults())
const (
	LogFieldName     = "name"      // string - command name
	LogFieldExitCode = "exit_code" // int - command exit code
	LogFieldOutput   = "output"    // string - command stdout
	LogFieldStderr   = "stderr"    // string - command stderr
)

// Log attribute keys for command group execution summary
// Used in runner.logGroupExecutionSummary and logging.buildCommandGroupSummary
const (
	LogAttrStatus     = "status"      // string - execution status (success/error)
	LogAttrGroup      = "group"       // string - group name
	LogAttrDurationMs = "duration_ms" // int64 - execution duration in milliseconds
	LogAttrCommands   = "commands"    // []CommandResult - list of command results
)

// Log attribute keys for pre-execution errors
// Used in logging.HandlePreExecutionError and logging.buildPreExecutionError
const (
	LogAttrErrorType    = "error_type"    // string - error type identifier
	LogAttrErrorMessage = "error_message" // string - error message details
	LogAttrComponent    = "component"     // string - component where error occurred
)

// CommandResultFields defines the structure and types for command result log fields.
// This struct serves as the canonical definition of the schema used for logging command results.
//
// Usage:
//   - runner.CommandResult should maintain the same field structure
//   - logging.commandResultInfo should maintain the same field structure
//   - Both write (LogValue) and read (extractCommandResults) operations must use this schema
//
// Type safety:
//   - Name: command name identifier
//   - ExitCode: command exit status code (0 for success, non-zero for failure)
//   - Output: standard output (stdout) from the command
//   - Stderr: standard error (stderr) from the command
type CommandResultFields struct {
	Name     string
	ExitCode int
	Output   string
	Stderr   string
}
