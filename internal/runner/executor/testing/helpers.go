//go:build test || performance

package testing

import (
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// RuntimeCommandOption is a functional option for configuring RuntimeCommand creation.
type RuntimeCommandOption func(*runtimeCommandConfig)

// runtimeCommandConfig holds the configuration for creating a RuntimeCommand.
type runtimeCommandConfig struct {
	name                string
	expandedCmd         string
	expandedArgs        []string
	workDir             string
	workDirSet          bool // Track if workDir was explicitly set
	effectiveWorkDir    string
	effectiveWorkDirSet bool // Track if effectiveWorkDir was explicitly set
	timeout             *int32
	runAsUser           string
	runAsGroup          string
	outputFile          string
	expandedEnv         map[string]string
	riskLevel           *string
}

// WithName sets the command name.
func WithName(name string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.name = name
	}
}

// WithExpandedCmd sets the expanded command path.
// If not set, the cmd parameter from CreateRuntimeCommand will be used.
func WithExpandedCmd(expandedCmd string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.expandedCmd = expandedCmd
	}
}

// WithExpandedArgs sets the expanded command arguments.
// If not set, the args parameter from CreateRuntimeCommand will be used.
func WithExpandedArgs(expandedArgs []string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.expandedArgs = expandedArgs
	}
}

// WithWorkDir sets the working directory for both Spec.WorkDir and EffectiveWorkDir.
// If not set, Spec.WorkDir will be empty and EffectiveWorkDir will default to os.TempDir().
func WithWorkDir(workDir string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.workDir = workDir
		c.workDirSet = true
		// Also set effectiveWorkDir if not already set
		if !c.effectiveWorkDirSet {
			c.effectiveWorkDir = workDir
			c.effectiveWorkDirSet = true
		}
	}
}

// WithEffectiveWorkDir sets only the EffectiveWorkDir, leaving Spec.WorkDir unchanged.
// This is useful when you want to override the effective directory without changing the spec.
func WithEffectiveWorkDir(effectiveWorkDir string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.effectiveWorkDir = effectiveWorkDir
		c.effectiveWorkDirSet = true
	}
}

// WithTimeout sets the command timeout.
func WithTimeout(timeout *int32) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.timeout = timeout
	}
}

// WithRunAsUser sets the user to run the command as.
func WithRunAsUser(user string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.runAsUser = user
	}
}

// WithRunAsGroup sets the group to run the command as.
func WithRunAsGroup(group string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.runAsGroup = group
	}
}

// WithOutputFile sets the output file path.
func WithOutputFile(outputFile string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.outputFile = outputFile
	}
}

// WithRiskLevel sets the risk level for the command.
func WithRiskLevel(riskLevel string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.riskLevel = runnertypes.StringPtr(riskLevel)
	}
}

// WithExpandedEnv sets the expanded environment variables.
func WithExpandedEnv(env map[string]string) RuntimeCommandOption {
	return func(c *runtimeCommandConfig) {
		c.expandedEnv = env
	}
}

// CreateRuntimeCommand creates a RuntimeCommand for testing with optional configuration.
// The cmd and args parameters are required and represent the Spec.Cmd and Spec.Args values.
// ExpandedCmd and ExpandedArgs default to cmd and args unless overridden with options.
// EffectiveWorkDir defaults to os.TempDir() unless overridden with WithWorkDir or WithEffectiveWorkDir.
//
// This function automatically sets ExpandedCmd, ExpandedArgs, and EffectiveWorkDir
// from the provided parameters and options.
//
// The function also handles timeout resolution properly using the common timeout logic.
//
// Usage:
//
//	// Simple usage with just command and args (EffectiveWorkDir = os.TempDir())
//	cmd := CreateRuntimeCommand("echo", []string{"hello", "world"})
//
//	// With additional options
//	cmd := CreateRuntimeCommand("/bin/echo", []string{"hello"},
//	    WithName("test-cmd"),
//	    WithWorkDir("/tmp"),
//	    WithRunAsUser("testuser"),
//	    WithRunAsGroup("testgroup"),
//	)
//
//	// Override expanded values
//	cmd := CreateRuntimeCommand("echo", []string{"hello"},
//	    WithExpandedCmd("/bin/echo"),
//	    WithExpandedArgs([]string{"hello", "world"}),
//	)
func CreateRuntimeCommand(cmd string, args []string, opts ...RuntimeCommandOption) *runnertypes.RuntimeCommand {
	// Default configuration
	cfg := &runtimeCommandConfig{
		name:         "test-command",
		expandedCmd:  cmd,  // Default to cmd parameter
		expandedArgs: args, // Default to args parameter
	}

	// Apply options
	for _, opt := range opts {
		opt(cfg)
	}

	// Set default workDir if not specified
	// Use empty string as default (which means the executor will use current directory)
	// Tests that need a specific workDir should set it explicitly with WithWorkDir

	// Build CommandSpec
	var workDir *string
	if cfg.workDir != "" {
		workDir = &cfg.workDir
	}
	var outputFile *string
	if cfg.outputFile != "" {
		outputFile = &cfg.outputFile
	}

	spec := &runnertypes.CommandSpec{
		Name:       cfg.name,
		Cmd:        cmd,
		Args:       args,
		WorkDir:    workDir,
		Timeout:    cfg.timeout,
		RunAsUser:  cfg.runAsUser,
		RunAsGroup: cfg.runAsGroup,
		OutputFile: outputFile,
		RiskLevel:  cfg.riskLevel,
	}

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

	// Initialize expanded env
	expandedEnv := cfg.expandedEnv
	if expandedEnv == nil {
		expandedEnv = make(map[string]string)
	}

	// Set effective working directory
	// Priority:
	// 1. Explicitly set effectiveWorkDir (via WithEffectiveWorkDir)
	// 2. Set workDir (via WithWorkDir) - use the value as-is
	// 3. Default to os.TempDir() for tests
	effectiveWorkDir := cfg.effectiveWorkDir
	if !cfg.effectiveWorkDirSet {
		if cfg.workDirSet {
			// Use workDir value as-is (even if empty)
			effectiveWorkDir = cfg.workDir
		} else {
			// Default to temporary directory for tests
			effectiveWorkDir = os.TempDir()
		}
	}

	return &runnertypes.RuntimeCommand{
		Spec:              spec,
		ExpandedCmd:       cfg.expandedCmd,
		ExpandedArgs:      cfg.expandedArgs,
		ExpandedEnv:       expandedEnv,
		ExpandedVars:      make(map[string]string),
		EffectiveWorkDir:  effectiveWorkDir,
		EffectiveTimeout:  effectiveTimeout,
		TimeoutResolution: resolutionContext,
	}
}

// CreateRuntimeCommandFromSpec creates a RuntimeCommand from an existing CommandSpec.
// This is useful when you already have a CommandSpec and want to convert it to RuntimeCommand.
//
// Usage:
//
//	spec := &runnertypes.CommandSpec{
//	    Name: "test-cmd",
//	    Cmd:  "/bin/echo",
//	    Args: []string{"hello"},
//	}
//	cmd := CreateRuntimeCommandFromSpec(spec)
func CreateRuntimeCommandFromSpec(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
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

	// Set effective working directory
	// If spec.WorkDir is set, use it; otherwise default to temporary directory
	effectiveWorkDir := ""
	if spec.WorkDir == nil || *spec.WorkDir == "" {
		effectiveWorkDir = os.TempDir()
	} else {
		effectiveWorkDir = *spec.WorkDir
	}

	return &runnertypes.RuntimeCommand{
		Spec:              spec,
		ExpandedCmd:       spec.Cmd,
		ExpandedArgs:      spec.Args,
		ExpandedEnv:       make(map[string]string),
		ExpandedVars:      make(map[string]string),
		EffectiveWorkDir:  effectiveWorkDir,
		EffectiveTimeout:  effectiveTimeout,
		TimeoutResolution: resolutionContext,
	}
}
