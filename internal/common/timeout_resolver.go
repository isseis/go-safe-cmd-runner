// Package common provides timeout resolution functionality for the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
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

// ResolveTimeout resolves the effective timeout value and returns context information.
// This is useful for logging and debugging timeout resolution.
//
// It follows the precedence: command > group > global > default.
//
// Parameters:
// - cmdTimeout: Command-level timeout (highest priority)
// - groupTimeout: Group-level timeout (medium priority) - currently not used but prepared for future implementation
// - globalTimeout: Global-level timeout (lower priority)
// - commandName: Name of the command for context
// - groupName: Name of the group for context
//
// Returns:
// - The resolved timeout value in seconds
// - TimeoutResolutionContext with metadata about the resolution
//
// A value of 0 means unlimited execution.
// A positive value means timeout after N seconds.
//
// Resolution logic:
// 1. If cmdTimeout is set, use its value (even if 0)
// 2. Else if groupTimeout is set, use its value (even if 0)
// 3. Else if globalTimeout is set, use its value (even if 0)
// 4. Else use DefaultTimeout (60 seconds)
func ResolveTimeout(cmdTimeout, groupTimeout, globalTimeout Timeout, commandName, groupName string) (int32, TimeoutResolutionContext) {
	var resolvedValue int32
	var level string

	// Determine which timeout to use based on hierarchy
	switch {
	case cmdTimeout.IsSet():
		resolvedValue = cmdTimeout.Value()
		level = "command"
	case groupTimeout.IsSet():
		resolvedValue = groupTimeout.Value()
		level = "group"
	case globalTimeout.IsSet():
		resolvedValue = globalTimeout.Value()
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
