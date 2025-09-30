// Package testhelpers provides common helper functions for tests
package testhelpers

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// PrepareCommand takes an existing Command and ensures ExpandedCmd and ExpandedArgs
// are set from Cmd and Args. This is useful when you need to set other fields
// manually but still want the expanded fields populated.
func PrepareCommand(cmd *runnertypes.Command) {
	if cmd.Name == "" {
		cmd.Name = "test-cmd"
	}
	if cmd.ExpandedCmd == "" {
		cmd.ExpandedCmd = cmd.Cmd
	}
	if cmd.ExpandedArgs == nil {
		cmd.ExpandedArgs = append([]string{}, cmd.Args...)
	}
}
