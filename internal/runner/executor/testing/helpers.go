//go:build test

package testing

import (
	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// CreateRuntimeCommand creates a RuntimeCommand from a CommandSpec for testing.
// This is the primary helper function that automatically sets ExpandedCmd, ExpandedArgs,
// and EffectiveWorkDir from the spec values.
//
// The function also handles timeout resolution properly using the common timeout logic.
//
// Usage:
//
//	spec := &runnertypes.CommandSpec{
//	    Name: "test-cmd",
//	    Cmd:  "/bin/echo",
//	    Args: []string{"hello"},
//	    WorkDir: "/tmp",
//	}
//	cmd := CreateRuntimeCommand(spec)
func CreateRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
	// Use the shared timeout resolution logic with context
	commandTimeout := common.NewFromIntPtr(spec.Timeout)
	globalTimeout := common.NewUnsetTimeout() // Tests typically don't need global timeout
	effectiveTimeout, resolutionContext := common.ResolveTimeout(
		commandTimeout,
		common.NewUnsetTimeout(), // No group timeout in tests
		globalTimeout,
		spec.Name,
		"test-group",
	)

	return &runnertypes.RuntimeCommand{
		Spec:              spec,
		ExpandedCmd:       spec.Cmd,
		ExpandedArgs:      spec.Args,
		ExpandedEnv:       make(map[string]string),
		ExpandedVars:      make(map[string]string),
		EffectiveWorkDir:  spec.WorkDir,
		EffectiveTimeout:  effectiveTimeout,
		TimeoutResolution: resolutionContext,
	}
}

// CreateSimpleRuntimeCommand creates a RuntimeCommand with basic parameters for testing.
// This is a convenience wrapper around CreateRuntimeCommand for simple test cases.
//
// For more control, use CreateRuntimeCommand with a full CommandSpec.
func CreateSimpleRuntimeCommand(cmd string, args []string, workDir string, runAsUser, runAsGroup string) *runnertypes.RuntimeCommand {
	spec := &runnertypes.CommandSpec{
		Name:       "test-command",
		Cmd:        cmd,
		Args:       args,
		WorkDir:    workDir,
		Timeout:    nil,
		RunAsUser:  runAsUser,
		RunAsGroup: runAsGroup,
	}
	return CreateRuntimeCommand(spec)
}

// CreateNamedRuntimeCommand creates a RuntimeCommand with a custom name for testing.
// This is useful when you need to test behavior that depends on the command name.
func CreateNamedRuntimeCommand(name, cmd string, args []string, workDir string, runAsUser, runAsGroup string) *runnertypes.RuntimeCommand {
	spec := &runnertypes.CommandSpec{
		Name:       name,
		Cmd:        cmd,
		Args:       args,
		WorkDir:    workDir,
		Timeout:    nil,
		RunAsUser:  runAsUser,
		RunAsGroup: runAsGroup,
	}
	return CreateRuntimeCommand(spec)
}
