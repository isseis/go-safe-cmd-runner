// Package common provides timeout resolution functionality for the command runner.
package common

// TimeoutResolutionContext provides context information for timeout resolution logging and debugging.
// This struct contains metadata about where the timeout value is being resolved from.
type TimeoutResolutionContext struct {
	// CommandName is the name of the command being resolved
	CommandName string

	// GroupName is the name of the group containing the command (empty if not applicable)
	GroupName string

	// Level indicates which level in the hierarchy provided the effective timeout
	// Possible values: "command", "group", "global", "default"
	Level string
}

// ResolveTimeout resolves the effective timeout value from the hierarchy.
// It follows the precedence: command > group > global > default.
//
// Parameters:
// - cmdTimeout: Command-level timeout (highest priority)
// - groupTimeout: Group-level timeout (medium priority) - currently not used but prepared for future implementation
// - globalTimeout: Global-level timeout (lower priority)
//
// Returns:
// - The resolved timeout value in seconds
// - A value of 0 means unlimited execution
// - A positive value means timeout after N seconds
//
// Resolution logic:
// 1. If cmdTimeout is set (not nil), use its value (even if 0)
// 2. Else if groupTimeout is set (not nil), use its value (even if 0)
// 3. Else if globalTimeout is set (not nil), use its value (even if 0)
// 4. Else use DefaultTimeout (60 seconds)
func ResolveTimeout(cmdTimeout, groupTimeout, globalTimeout *int) int {
	// Determine which timeout pointer to use based on hierarchy
	switch {
	case cmdTimeout != nil:
		return *cmdTimeout
	case groupTimeout != nil:
		return *groupTimeout
	case globalTimeout != nil:
		return *globalTimeout
	default:
		return DefaultTimeout
	}
}

// ResolveTimeoutWithContext resolves the effective timeout value and returns context information.
// This is useful for logging and debugging timeout resolution.
//
// Returns both the resolved timeout value and context information about which level provided it.
func ResolveTimeoutWithContext(cmdTimeout, groupTimeout, globalTimeout *int, commandName, groupName string) (int, TimeoutResolutionContext) {
	var resolvedValue int
	var level string

	// Determine which timeout pointer to use based on hierarchy
	switch {
	case cmdTimeout != nil:
		resolvedValue = *cmdTimeout
		level = "command"
	case groupTimeout != nil:
		resolvedValue = *groupTimeout
		level = "group"
	case globalTimeout != nil:
		resolvedValue = *globalTimeout
		level = "global"
	default:
		resolvedValue = DefaultTimeout
		level = "default"
	}

	context := TimeoutResolutionContext{
		CommandName: commandName,
		GroupName:   groupName,
		Level:       level,
	}

	return resolvedValue, context
}

// IsUnlimitedTimeout returns true if the given timeout value represents unlimited execution.
// A timeout value of 0 means unlimited execution.
func IsUnlimitedTimeout(timeout int) bool {
	return timeout == 0
}

// IsDefaultTimeout returns true if the given timeout value is the system default.
func IsDefaultTimeout(timeout int) bool {
	return timeout == DefaultTimeout
}
