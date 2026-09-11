// Package common provides shared types and utilities used across the application
//
//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"fmt"
	"log/slog"
)

// Log field keys for CommandResult structured logging
// These constants ensure consistency between log value creation (runner.CommandResult.LogValue())
// and log attribute extraction (logging.extractCommandResults())
const (
	LogFieldName     = "name"      // string - command name
	LogFieldExitCode = "exit_code" // int - command exit code
	LogFieldOutput   = "output"    // string - command stdout
	LogFieldStderr   = "stderr"    // string - command stderr
)

// GroupSummaryAttrs contains attribute keys for command group execution summary logs.
// Used in runner.logGroupExecutionSummary (write) and logging.buildCommandGroupSummary (read).
var GroupSummaryAttrs = struct {
	Status     string // execution status (success/error)
	Group      string // group name
	DurationMs string // execution duration in milliseconds (int64)
	Commands   string // list of command results ([]CommandResult)
}{
	Status:     "status",
	Group:      "group",
	DurationMs: "duration_ms",
	Commands:   "commands",
}

// PreExecErrorAttrs contains attribute keys for pre-execution error logs.
// Used in logging.HandlePreExecutionError (write) and logging.buildPreExecutionError (read).
var PreExecErrorAttrs = struct {
	ErrorType    string // error type identifier
	ErrorMessage string // error message details
	Component    string // component where error occurred
}{
	ErrorType:    "error_type",
	ErrorMessage: "error_message",
	Component:    "component",
}

// UserGroupCommandFailureAttrs contains attribute keys for user/group command
// failure logs. Used in audit.Logger.LogUserGroupExecution (write) and the
// user_group_command_failure message builder (read), so both sides share the
// attribute names instead of repeating the literals.
var UserGroupCommandFailureAttrs = struct {
	CommandName string // command name
	ExitCode    string // command exit code (int)
	Stdout      string // command standard output
	Stderr      string // command standard error
}{
	CommandName: "command_name",
	ExitCode:    "exit_code",
	Stdout:      "stdout",
	Stderr:      "stderr",
}

// NotificationContextAttrs contains the attribute key and sub-key names of the
// encoded notification context group. NotificationContext.LogValue writes this
// group and the Slack handler reads it back, so the writer and the reader share
// these names instead of repeating the literals.
var NotificationContextAttrs = struct {
	Key     string // outer attribute carrying the encoded context
	Scope   string // sub-key holding the scope name
	Group   string // sub-key holding the group name (always present, empty for global)
	Command string // sub-key holding the command name (omitted when empty)
}{
	Key:     "notification_context",
	Scope:   "scope",
	Group:   "group",
	Command: "command",
}

// NotificationScopeNames contains the encoded names of the NotificationScope
// values. The names are part of the record encoding, so the constructors and
// the decoder share these constants.
var NotificationScopeNames = struct {
	Global  string
	Group   string
	Command string
}{
	Global:  "global",
	Group:   "group",
	Command: "command",
}

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

// CommandResult holds the result of a single command execution
// This type is used across packages (runner, logging) to avoid circular dependencies
type CommandResult struct {
	CommandResultFields
}

// LogValue implements slog.LogValuer to provide structured logging support
// Field keys are defined in LogField* constants to ensure consistency
func (c CommandResult) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String(LogFieldName, c.Name),
		slog.Int(LogFieldExitCode, c.ExitCode),
		slog.String(LogFieldOutput, c.Output),
		slog.String(LogFieldStderr, c.Stderr),
	)
}

const (
	// MaxLoggedCommands bounds the number of command results that will be emitted in a single log record.
	// This keeps log payload sizes predictable when groups execute very large command sets.
	MaxLoggedCommands = 100

	// CommandResultsMetadataAttrCount is the number of metadata attributes in CommandResults.LogValue() output.
	// These metadata attributes (total_count, truncated) appear before the command entries.
	// This constant is used for slice capacity pre-allocation in both LogValue() and extraction logic.
	CommandResultsMetadataAttrCount = 2
)

// CommandResults is a slice wrapper that implements slog.LogValuer for the entire collection.
// It produces a stable Group structure so downstream handlers do not need to inspect individual elements.
type CommandResults []CommandResult

// Compile-time guard to ensure CommandResults implements slog.LogValuer.
var _ slog.LogValuer = CommandResults(nil)

// LogValue structures command results as a slog.GroupValue with metadata and truncated command entries.
// Sensitive data can then be redacted at the Group level without triggering slog's slice processing path.
func (cr CommandResults) LogValue() slog.Value {
	commandsToLog := cr
	truncated := false
	if len(cr) > MaxLoggedCommands {
		commandsToLog = cr[:MaxLoggedCommands]
		truncated = true
	}

	attrs := make([]slog.Attr, 0, len(commandsToLog)+CommandResultsMetadataAttrCount)
	attrs = append(attrs,
		slog.Int("total_count", len(cr)),
		slog.Bool("truncated", truncated),
	)

	for i, cmd := range commandsToLog {
		attrs = append(attrs, slog.Group(
			fmt.Sprintf("cmd_%d", i),
			slog.String(LogFieldName, cmd.Name),
			slog.Int(LogFieldExitCode, cmd.ExitCode),
			slog.String(LogFieldOutput, cmd.Output),
			slog.String(LogFieldStderr, cmd.Stderr),
		))
	}

	return slog.GroupValue(attrs...)
}
