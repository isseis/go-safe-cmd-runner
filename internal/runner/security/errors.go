package security

import (
	"fmt"
	"strings"
)

// CommandNotAllowedError is returned when a command is not permitted by either
// global AllowedCommands patterns or the group-level cmd_allowed list.
// It wraps ErrCommandNotAllowed so callers can use errors.Is(err, ErrCommandNotAllowed).
type CommandNotAllowedError struct {
	CommandPath     string   // The command path that was attempted to execute (original path before symlink resolution)
	ResolvedPath    string   // The resolved command path after symlink resolution (may be same as CommandPath)
	AllowedPatterns []string // Global security configuration regex patterns
	GroupCmdAllowed []string // Expanded group-level allowed command paths (may be nil/empty)
}

// Error implements the error interface providing a human-friendly multi-line
// diagnostic describing why the command was rejected and what patterns/lists
// were evaluated.
func (e *CommandNotAllowedError) Error() string {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("command not allowed: %s\n", e.CommandPath))

	// If the command path was a symlink, show the resolved path
	if e.ResolvedPath != e.CommandPath {
		buf.WriteString(fmt.Sprintf("  - Resolved symlink to: %s\n", e.ResolvedPath))
		buf.WriteString("  - Validation performed against the resolved path\n")
	}

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
	return target == ErrCommandNotAllowed
}

// Unwrap returns the sentinel ErrCommandNotAllowed for error chain checks.
func (e *CommandNotAllowedError) Unwrap() error {
	return ErrCommandNotAllowed
}
