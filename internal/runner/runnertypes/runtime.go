package runnertypes

// RuntimeGlobal represents the runtime-expanded global configuration.
// It contains references to the original GlobalSpec along with expanded variables
// and resources that are resolved at runtime.
type RuntimeGlobal struct {
	// Spec is a reference to the original spec loaded from TOML
	Spec *GlobalSpec

	// ExpandedVerifyFiles contains the list of files to verify with variables expanded
	ExpandedVerifyFiles []string

	// ExpandedEnv contains environment variables with all variable references expanded
	ExpandedEnv map[string]string

	// ExpandedVars contains internal variables with all variable references expanded
	ExpandedVars map[string]string
}

// RuntimeGroup represents the runtime-expanded group configuration.
// It contains references to the original GroupSpec along with expanded variables
// and resources that are resolved at runtime.
type RuntimeGroup struct {
	// Spec is a reference to the original spec loaded from TOML
	Spec *GroupSpec

	// ExpandedVerifyFiles contains the list of files to verify with variables expanded
	ExpandedVerifyFiles []string

	// ExpandedEnv contains environment variables with all variable references expanded
	ExpandedEnv map[string]string

	// ExpandedVars contains internal variables with all variable references expanded
	ExpandedVars map[string]string

	// EffectiveWorkDir is the resolved working directory for this group
	EffectiveWorkDir string

	// Commands contains the expanded runtime commands for this group
	Commands []*RuntimeCommand
}

// RuntimeCommand represents the runtime-expanded command configuration.
// It contains references to the original CommandSpec along with expanded variables
// and resources that are resolved at runtime.
type RuntimeCommand struct {
	// Spec is a reference to the original spec loaded from TOML
	Spec *CommandSpec

	// ExpandedCmd is the command path with all variable references expanded
	ExpandedCmd string

	// ExpandedArgs contains command arguments with all variable references expanded
	ExpandedArgs []string

	// ExpandedEnv contains environment variables with all variable references expanded
	ExpandedEnv map[string]string

	// ExpandedVars contains internal variables with all variable references expanded
	ExpandedVars map[string]string

	// EffectiveWorkDir is the resolved working directory for this command
	EffectiveWorkDir string

	// EffectiveTimeout is the resolved timeout value (in seconds) for this command
	EffectiveTimeout int
}

// Convenience methods for RuntimeCommand

// Name returns the command name from the spec.
func (r *RuntimeCommand) Name() string {
	return r.Spec.Name
}

// RunAsUser returns the user to run the command as from the spec.
func (r *RuntimeCommand) RunAsUser() string {
	return r.Spec.RunAsUser
}

// RunAsGroup returns the group to run the command as from the spec.
func (r *RuntimeCommand) RunAsGroup() string {
	return r.Spec.RunAsGroup
}

// Output returns the output file path from the spec.
func (r *RuntimeCommand) Output() string {
	return r.Spec.Output
}

// GetMaxRiskLevel parses and returns the maximum risk level for this command.
func (r *RuntimeCommand) GetMaxRiskLevel() (RiskLevel, error) {
	return r.Spec.GetMaxRiskLevel()
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified.
func (r *RuntimeCommand) HasUserGroupSpecification() bool {
	return r.Spec.HasUserGroupSpecification()
}
