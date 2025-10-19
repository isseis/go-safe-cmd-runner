//go:build test || performance || security

package runnertypes

// PrepareCommand takes an existing Command and ensures ExpandedCmd and ExpandedArgs
// are set from Cmd and Args. This is useful when you need to set other fields
// manually but still want the expanded fields populated.
func PrepareCommand(cmd *Command) {
	if cmd.Name == "" {
		cmd.Name = "test-cmd"
	}
	if cmd.ExpandedCmd == "" {
		cmd.ExpandedCmd = cmd.Cmd
	}
	if cmd.ExpandedArgs == nil {
		cmd.ExpandedArgs = append([]string{}, cmd.Args...)
	}
	if cmd.EffectiveWorkdir == "" {
		cmd.EffectiveWorkdir = cmd.Dir
	}
}
