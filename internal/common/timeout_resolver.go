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
// Returns both the resolved timeout value and context information about which level provided it.
func ResolveTimeout(cmdTimeout, groupTimeout, globalTimeout *int, commandName, groupName string) (int, TimeoutResolutionContext) {
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
