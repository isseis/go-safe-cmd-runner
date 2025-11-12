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

// PrivilegeEscalationFailureAttrs contains attribute keys for privilege escalation failure logs.
// Used in audit.Logger.LogPrivilegeEscalation (write) and logging.buildPrivilegeEscalationFailure (read).
var PrivilegeEscalationFailureAttrs = struct {
	Operation   string // operation being performed
	CommandName string // command name
	OriginalUID string // original user ID
	TargetUID   string // target user ID
}{
	Operation:   "operation",
	CommandName: "command_name",
	OriginalUID: "original_uid",
	TargetUID:   "target_uid",
}

// PrivilegedCommandFailureAttrs contains attribute keys for privileged command failure logs.
// Used in logging.buildPrivilegedCommandFailure (read). Write side not yet implemented.
var PrivilegedCommandFailureAttrs = struct {
	CommandName string // command name
	CommandPath string // command path
	Stderr      string // standard error output
	ExitCode    string // exit code
}{
	CommandName: "command_name",
	CommandPath: "command_path",
	Stderr:      "stderr",
	ExitCode:    "exit_code",
}

// SecurityAlertAttrs contains attribute keys for security alert logs.
// Used in audit.Logger.LogSecurityEvent (write) and logging.buildSecurityAlert (read).
var SecurityAlertAttrs = struct {
	EventType string // event type identifier
	Severity  string // severity level (critical/high/medium/low)
	Message   string // alert message details
}{
	EventType: "event_type",
	Severity:  "severity",
	Message:   "message",
}

// SecuritySeverity defines severity levels for security events
const (
	SeverityCritical = "critical" // Critical severity - requires immediate attention
	SeverityHigh     = "high"     // High severity - requires prompt attention
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
