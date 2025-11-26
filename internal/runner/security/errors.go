package security

import (
	"errors"
	"fmt"
	"strings"
)

// CommandNotAllowedError is returned when a command is not permitted by either
// global AllowedCommands patterns or the group-level cmd_allowed list.
// It wraps ErrCommandNotAllowed so callers can use errors.Is(err, ErrCommandNotAllowed).
type CommandNotAllowedError struct {
	CommandPath     string   // The command path that was attempted to execute
	AllowedPatterns []string // Global security configuration regex patterns
	GroupCmdAllowed []string // Expanded group-level allowed command paths (may be nil/empty)
}

// Error implements the error interface providing a human-friendly multi-line
// diagnostic describing why the command was rejected and what patterns/lists
// were evaluated.
func (e *CommandNotAllowedError) Error() string {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("command not allowed: %s\n", e.CommandPath))
	buf.WriteString("  - Not matched by global allowed_commands patterns:\n")
	for _, pattern := range e.AllowedPatterns {
		buf.WriteString(fmt.Sprintf("      %s\n", pattern))
	}
	if len(e.GroupCmdAllowed) > 0 {
		buf.WriteString("  - Not in group-level cmd_allowed list:\n")
		for _, allowed := range e.GroupCmdAllowed {
			buf.WriteString(fmt.Sprintf("      %s\n", allowed))
		}
	} else {
		buf.WriteString("  - Group-level cmd_allowed is not configured\n")
	}
	return buf.String()
}

// Is enables errors.Is(err, ErrCommandNotAllowed) comparisons.
func (e *CommandNotAllowedError) Is(target error) bool {
	return errors.Is(target, ErrCommandNotAllowed)
}

// Unwrap returns the sentinel ErrCommandNotAllowed for error chain checks.
func (e *CommandNotAllowedError) Unwrap() error {
	return ErrCommandNotAllowed
}
