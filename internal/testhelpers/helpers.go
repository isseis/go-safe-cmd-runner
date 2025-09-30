// Package testhelpers provides common helper functions for tests
package testhelpers

import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"

// NewCommand creates a new Command with ExpandedCmd and ExpandedArgs automatically set
// from the Cmd and Args fields. This helper ensures consistency in tests after
// the removal of fallback logic from ExpandedCmd/ExpandedArgs to Cmd/Args.
func NewCommand(name, cmd string, args []string) runnertypes.Command {
	return runnertypes.Command{
		Name:         name,
		Cmd:          cmd,
		ExpandedCmd:  cmd,
		Args:         args,
		ExpandedArgs: append([]string{}, args...), // Make a copy of args slice
	}
}

// PrepareCommand takes an existing Command and ensures ExpandedCmd and ExpandedArgs
// are set from Cmd and Args. This is useful when you need to set other fields
// manually but still want the expanded fields populated.
func PrepareCommand(cmd *runnertypes.Command) {
	if cmd.ExpandedCmd == "" {
		cmd.ExpandedCmd = cmd.Cmd
	}
	if cmd.ExpandedArgs == nil {
		cmd.ExpandedArgs = append([]string{}, cmd.Args...)
	}
}

// NewCommandWithOutput creates a new Command with output path specified
func NewCommandWithOutput(name, cmd string, args []string, output string) runnertypes.Command {
	c := NewCommand(name, cmd, args)
	c.Output = output
	return c
}

// NewCommandWithTimeout creates a new Command with timeout specified
func NewCommandWithTimeout(name, cmd string, args []string, timeout int) runnertypes.Command {
	c := NewCommand(name, cmd, args)
	c.Timeout = timeout
	return c
}

// NewCommandWithOutputAndTimeout creates a new Command with both output path and timeout
func NewCommandWithOutputAndTimeout(name, cmd string, args []string, output string, timeout int) runnertypes.Command {
	c := NewCommand(name, cmd, args)
	c.Output = output
	c.Timeout = timeout
	return c
}

// NewCommandWithDir creates a new Command with working directory specified
func NewCommandWithDir(name, cmd string, args []string, dir string) runnertypes.Command {
	c := NewCommand(name, cmd, args)
	c.Dir = dir
	return c
}

// NewCommandWithUserGroup creates a new Command with user and group configured
func NewCommandWithUserGroup(name, cmd string, args []string, user, group string) runnertypes.Command {
	c := NewCommand(name, cmd, args)
	c.RunAsUser = user
	c.RunAsGroup = group
	return c
}

// NewCommandGroup creates a new CommandGroup for testing
func NewCommandGroup(name string) *runnertypes.CommandGroup {
	return &runnertypes.CommandGroup{
		Name: name,
	}
}

// NewCommandGroupWithWorkDir creates a new CommandGroup with working directory for testing
func NewCommandGroupWithWorkDir(name, workDir string) *runnertypes.CommandGroup {
	return &runnertypes.CommandGroup{
		Name:    name,
		WorkDir: workDir,
	}
}
