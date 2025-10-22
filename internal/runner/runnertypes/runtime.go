package runnertypes

import "errors"

const (
	// DefaultTimeout is the default timeout in seconds when not specified in config
	DefaultTimeout = 60
)

// Error definitions for runtime types
var (
	// ErrNilSpec is returned when a nil spec is provided to a constructor
	ErrNilSpec = errors.New("spec must not be nil")
)

// RuntimeGlobal represents the runtime-expanded global configuration.
// It contains references to the original GlobalSpec along with expanded variables
// and resources that are resolved at runtime.
//
// Invariant: Spec must be non-nil. Use NewRuntimeGlobal to create instances.
type RuntimeGlobal struct {
	// Spec is a reference to the original spec loaded from TOML (must be non-nil)
	Spec *GlobalSpec

	// ExpandedVerifyFiles contains the list of files to verify with variables expanded
	ExpandedVerifyFiles []string

	// ExpandedEnv contains environment variables with all variable references expanded
	ExpandedEnv map[string]string

	// ExpandedVars contains internal variables with all variable references expanded
	ExpandedVars map[string]string
}

// NewRuntimeGlobal creates a new RuntimeGlobal with the required spec.
// Returns ErrNilSpec if spec is nil.
func NewRuntimeGlobal(spec *GlobalSpec) (*RuntimeGlobal, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}
	return &RuntimeGlobal{
		Spec:                spec,
		ExpandedVerifyFiles: []string{},
		ExpandedEnv:         make(map[string]string),
		ExpandedVars:        make(map[string]string),
	}, nil
}

// Convenience methods for RuntimeGlobal

// Timeout returns the global timeout from the spec.
// Returns DefaultTimeout (60 seconds) if not specified in config (Spec.Timeout == 0).
// Panics if r or r.Spec is nil (programming error - use NewRuntimeGlobal).
func (r *RuntimeGlobal) Timeout() int {
	if r == nil || r.Spec == nil {
		panic("RuntimeGlobal.Timeout: nil receiver or Spec (programming error - use NewRuntimeGlobal)")
	}
	if r.Spec.Timeout == 0 {
		return DefaultTimeout
	}
	return r.Spec.Timeout
}

// EnvAllowlist returns the environment variable allowlist from the spec.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeGlobal).
func (r *RuntimeGlobal) EnvAllowlist() []string {
	if r == nil || r.Spec == nil {
		panic("RuntimeGlobal.EnvAllowlist: nil receiver or Spec (programming error - use NewRuntimeGlobal)")
	}
	return r.Spec.EnvAllowlist
}

// SkipStandardPaths returns the skip_standard_paths setting from the spec.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeGlobal).
func (r *RuntimeGlobal) SkipStandardPaths() bool {
	if r == nil || r.Spec == nil {
		panic("RuntimeGlobal.SkipStandardPaths: nil receiver or Spec (programming error - use NewRuntimeGlobal)")
	}
	return r.Spec.SkipStandardPaths
}

// RuntimeGroup represents the runtime-expanded group configuration.
// It contains references to the original GroupSpec along with expanded variables
// and resources that are resolved at runtime.
//
// Invariant: Spec must be non-nil. Use NewRuntimeGroup to create instances.
type RuntimeGroup struct {
	// Spec is a reference to the original spec loaded from TOML (must be non-nil)
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

// NewRuntimeGroup creates a new RuntimeGroup with the required spec.
// Returns ErrNilSpec if spec is nil.
func NewRuntimeGroup(spec *GroupSpec) (*RuntimeGroup, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}
	return &RuntimeGroup{
		Spec:                spec,
		ExpandedVerifyFiles: []string{},
		ExpandedEnv:         make(map[string]string),
		ExpandedVars:        make(map[string]string),
		Commands:            []*RuntimeCommand{},
	}, nil
}

// Convenience methods for RuntimeGroup

// Name returns the group name from the spec.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeGroup).
func (r *RuntimeGroup) Name() string {
	if r == nil || r.Spec == nil {
		panic("RuntimeGroup.Name: nil receiver or Spec (programming error - use NewRuntimeGroup)")
	}
	return r.Spec.Name
}

// WorkDir returns the group working directory from the spec (not yet expanded).
// Panics if r or r.Spec is nil (programming error - use NewRuntimeGroup).
func (r *RuntimeGroup) WorkDir() string {
	if r == nil || r.Spec == nil {
		panic("RuntimeGroup.WorkDir: nil receiver or Spec (programming error - use NewRuntimeGroup)")
	}
	return r.Spec.WorkDir
}

// RuntimeCommand represents the runtime-expanded command configuration.
// It contains references to the original CommandSpec along with expanded variables
// and resources that are resolved at runtime.
//
// Invariant: Spec must be non-nil. Use NewRuntimeCommand to create instances.
type RuntimeCommand struct {
	// Spec is a reference to the original spec loaded from TOML (must be non-nil)
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

// NewRuntimeCommand creates a new RuntimeCommand with the required spec.
// Returns ErrNilSpec if spec is nil.
func NewRuntimeCommand(spec *CommandSpec) (*RuntimeCommand, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}
	return &RuntimeCommand{
		Spec:         spec,
		ExpandedArgs: []string{},
		ExpandedEnv:  make(map[string]string),
		ExpandedVars: make(map[string]string),
	}, nil
}

// Convenience methods for RuntimeCommand

// Name returns the command name from the spec.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) Name() string {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.Name: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.Name
}

// RunAsUser returns the user to run the command as from the spec.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) RunAsUser() string {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.RunAsUser: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.RunAsUser
}

// RunAsGroup returns the group to run the command as from the spec.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) RunAsGroup() string {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.RunAsGroup: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.RunAsGroup
}

// Output returns the output file path from the spec.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) Output() string {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.Output: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.Output
}

// Cmd returns the command path from the spec (not yet expanded).
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) Cmd() string {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.Cmd: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.Cmd
}

// Args returns the command arguments from the spec (not yet expanded).
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) Args() []string {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.Args: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.Args
}

// Timeout returns the command-specific timeout from the spec.
// Returns 0 if no timeout is specified (use EffectiveTimeout for resolved value).
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) Timeout() int {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.Timeout: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.Timeout
}

// GetMaxRiskLevel parses and returns the maximum risk level for this command.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) GetMaxRiskLevel() (RiskLevel, error) {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.GetMaxRiskLevel: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.GetMaxRiskLevel()
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) HasUserGroupSpecification() bool {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.HasUserGroupSpecification: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.HasUserGroupSpecification()
}
