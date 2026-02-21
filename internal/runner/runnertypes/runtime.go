package runnertypes

import (
	"errors"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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

	// timeout is the converted Timeout value from Spec.Timeout
	timeout common.Timeout

	// ExpandedVerifyFiles contains the list of files to verify with variables expanded
	ExpandedVerifyFiles []string

	// ExpandedEnv contains environment variables with all variable references expanded
	// This includes variables from both env_import and vars sections
	ExpandedEnv map[string]string

	// ExpandedVars contains internal variables with all variable references expanded
	ExpandedVars map[string]string

	// ExpandedArrayVars contains array variables with all variable references expanded.
	// Each array element has been individually expanded using ExpandString.
	// This is populated by ProcessVars when processing array-type variables.
	//
	// Example:
	//   TOML: config_files = ["%{base_dir}/config.yml", "%{base_dir}/secrets.yml"]
	//   After expansion: {"config_files": ["/opt/myapp/config.yml", "/opt/myapp/secrets.yml"]}
	ExpandedArrayVars map[string][]string

	// EnvImportVars tracks variables imported from system environment via env_import.
	// This is used for conflict detection to prevent vars from redefining env_import variables.
	// Key: internal variable name, Value: expanded value from system environment
	EnvImportVars map[string]string

	// SystemEnv contains the cached system environment variables parsed from os.Environ().
	// This is populated once during ExpandGlobal to avoid repeated os.Environ() parsing
	// in ExpandGroup and ExpandCommand.
	SystemEnv map[string]string
}

// NewRuntimeGlobal creates a new RuntimeGlobal with the required spec.
// Returns ErrNilSpec if spec is nil.
func NewRuntimeGlobal(spec *GlobalSpec) (*RuntimeGlobal, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}

	return &RuntimeGlobal{
		Spec:                spec,
		timeout:             common.NewFromIntPtr(spec.Timeout),
		ExpandedVerifyFiles: []string{},
		ExpandedEnv:         make(map[string]string),
		ExpandedVars:        make(map[string]string),
		ExpandedArrayVars:   make(map[string][]string),
		EnvImportVars:       make(map[string]string),
		SystemEnv:           make(map[string]string),
	}, nil
}

// Convenience methods for RuntimeGlobal

// Timeout returns the global timeout from the spec.
// Returns the configured Timeout value, which can be unset, unlimited, or a positive value.
// Use common.ResolveTimeout() to resolve the effective timeout with proper fallback logic.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeGlobal).
func (r *RuntimeGlobal) Timeout() common.Timeout {
	if r == nil || r.Spec == nil {
		panic("RuntimeGlobal.Timeout: nil receiver or Spec (programming error - use NewRuntimeGlobal)")
	}
	return r.timeout
}

// EnvAllowlist returns the environment variable allowlist from the spec.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeGlobal).
func (r *RuntimeGlobal) EnvAllowlist() []string {
	if r == nil || r.Spec == nil {
		panic("RuntimeGlobal.EnvAllowed: nil receiver or Spec (programming error - use NewRuntimeGlobal)")
	}
	return r.Spec.EnvAllowed
}

// DetermineVerifyStandardPaths returns the effective verify_standard_paths setting.
// If verifyStandardPaths is nil, returns the security-safe default (true = verify).
// This ensures consistent behavior even if ApplyGlobalDefaults hasn't been called.
func DetermineVerifyStandardPaths(verifyStandardPaths *bool) bool {
	if verifyStandardPaths == nil {
		return true // default: verify paths (matches DefaultVerifyStandardPaths)
	}
	return *verifyStandardPaths // use explicit value
}

// SkipStandardPaths returns the skip_standard_paths setting from the spec.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeGlobal).
func (r *RuntimeGlobal) SkipStandardPaths() bool {
	if r == nil || r.Spec == nil {
		panic("RuntimeGlobal.SkipStandardPaths: nil receiver or Spec (programming error - use NewRuntimeGlobal)")
	}
	// Convert verify_standard_paths to skip_standard_paths logic (invert boolean)
	return !DetermineVerifyStandardPaths(r.Spec.VerifyStandardPaths)
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
	// This includes variables from both env_import and vars sections
	ExpandedEnv map[string]string

	// ExpandedVars contains internal variables with all variable references expanded
	ExpandedVars map[string]string

	// ExpandedArrayVars contains array variables with all variable references expanded.
	// See RuntimeGlobal.ExpandedArrayVars for details.
	ExpandedArrayVars map[string][]string

	// EnvImportVars tracks variables imported from system environment via env_import at this level.
	// This accumulates env_import variables from global and group levels for conflict detection.
	// Key: internal variable name, Value: expanded value from system environment
	EnvImportVars map[string]string

	// EffectiveWorkDir is the resolved working directory for this group
	EffectiveWorkDir string

	// ExpandedCmdAllowed is the map of allowed commands after variable expansion.
	//
	// Each key has completed the following processing:
	//   1. Variable expansion: %{var} -> actual value
	//   2. Absolute path validation: confirmed to start with '/'
	//   3. Symbolic link resolution: filepath.EvalSymlinks
	//   4. Path normalization: filepath.Clean
	//   5. Deduplication: automatic with map usage
	//
	// Using a map provides O(1) lookup time compared to O(n) for a slice,
	// improving performance when validating command paths against the allowed list.
	//
	// When empty (len == 0):
	//   - GroupSpec.CmdAllowed was nil or empty array
	//   - No group-level additional permissions are applied
	//
	// Example:
	//   {"/home/user/bin/tool1": struct{}{}, "/usr/local/bin/node": struct{}{}}
	ExpandedCmdAllowed map[string]struct{}

	// Commands contains the expanded runtime commands for this group
	Commands []*RuntimeCommand

	// EnvAllowlistInheritanceMode holds the inheritance mode for the environment variable allowlist.
	//
	// This field is set during configuration expansion (in the Expander.ExpandGroup function)
	// by calling DetermineEnvAllowlistInheritanceMode(), and is thereafter used read-only
	// throughout the runtime.
	//
	// Values:
	//   - InheritanceModeInherit (0):
	//     The group inherits the global env_allowlist configuration.
	//     This occurs when the env_allowlist field is undefined in the TOML configuration.
	//
	//   - InheritanceModeExplicit (1):
	//     The group uses its own env_allowlist configuration.
	//     This occurs when env_allowlist = ["VAR1", "VAR2"] is explicitly set
	//     with a non-empty variable list in the TOML configuration.
	//
	//   - InheritanceModeReject (2):
	//     The group rejects all environment variables.
	//     This occurs when env_allowlist = [] is explicitly set as an empty array
	//     in the TOML configuration.
	//
	// Lifecycle:
	//   1. RuntimeGroup creation: Default value (InheritanceModeInherit = 0)
	//   2. ExpandGroup execution: Set by DetermineEnvAllowlistInheritanceMode()
	//   3. Subsequent usage: Read-only reference
	//
	// Usage locations:
	//   - Debug output (debug.PrintFromEnvInheritance)
	//   - Dry-run mode output (future feature)
	//   - Other inheritance mode-dependent processing
	//
	// Invariants:
	//   - After ExpandGroup execution, this field always holds a valid value
	//   - Value must be one of: InheritanceModeInherit, InheritanceModeExplicit,
	//     or InheritanceModeReject
	//   - Once set, the value is not modified (immutable)
	EnvAllowlistInheritanceMode InheritanceMode
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
		ExpandedArrayVars:   make(map[string][]string),
		EnvImportVars:       make(map[string]string),
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

// ExtractGroupName extracts the group name from a RuntimeGroup.
// Panics if runtimeGroup or runtimeGroup.Spec is nil (programming error).
// All commands must belong to a group per TOML specification.
func ExtractGroupName(runtimeGroup *RuntimeGroup) string {
	if runtimeGroup == nil || runtimeGroup.Spec == nil {
		panic("ExtractGroupName: runtimeGroup and runtimeGroup.Spec must be non-nil (programming error)")
	}
	return runtimeGroup.Spec.Name
}

// RuntimeCommand represents the runtime-expanded command configuration.
// It contains references to the original CommandSpec along with expanded variables
// and resources that are resolved at runtime.
//
// Invariant: Spec must be non-nil. Use NewRuntimeCommand to create instances.
type RuntimeCommand struct {
	// Spec is a reference to the original spec loaded from TOML (must be non-nil)
	Spec *CommandSpec

	// timeout is the converted Timeout value from Spec.Timeout
	timeout common.Timeout

	// ExpandedCmd is the command path with all variable references expanded
	ExpandedCmd string

	// ExpandedArgs contains command arguments with all variable references expanded
	ExpandedArgs []string

	// ExpandedEnv contains environment variables with all variable references expanded
	// This includes variables from both env_import and vars sections
	ExpandedEnv map[string]string

	// ExpandedVars contains internal variables with all variable references expanded
	ExpandedVars map[string]string

	// ExpandedArrayVars contains array variables with all variable references expanded.
	// See RuntimeGlobal.ExpandedArrayVars for details.
	ExpandedArrayVars map[string][]string

	// EnvImportVars tracks variables imported from system environment via env_import at this level.
	// This accumulates env_import variables from global, group, and command levels for conflict detection.
	// Key: internal variable name, Value: expanded value from system environment
	EnvImportVars map[string]string

	// ExpandedCmdContentHash holds the prefixed content hash ("algo:hex") of the
	// command binary as computed during file verification (VerifyGroupFiles).
	// It is set by the group executor after verification completes and forwarded
	// to the ELF analyzer to avoid a redundant read of the binary.
	// Empty string means no hash is available (file was skipped or not verified).
	ExpandedCmdContentHash string

	// EffectiveWorkDir is the resolved working directory for this command
	EffectiveWorkDir string

	// EffectiveTimeout is the resolved timeout value (in seconds) for this command
	EffectiveTimeout int32

	// TimeoutResolution contains context information about timeout resolution
	TimeoutResolution common.TimeoutResolutionContext

	// EffectiveOutputSizeLimit is the resolved output size limit for this command
	// Use IsUnlimited() to check if unlimited, Value() to get the limit in bytes
	EffectiveOutputSizeLimit common.OutputSizeLimit
}

// NewRuntimeCommand creates a new RuntimeCommand with the required spec.
// The globalTimeout parameter is used for timeout resolution hierarchy.
// The globalOutputSizeLimit parameter is used for output size limit resolution.
// The groupName parameter provides context for timeout resolution logging.
// Returns ErrNilSpec if spec is nil.
func NewRuntimeCommand(spec *CommandSpec, globalTimeout common.Timeout, globalOutputSizeLimit common.OutputSizeLimit, groupName string) (*RuntimeCommand, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}

	// Resolve the effective timeout using the hierarchy with context
	commandTimeout := common.NewFromIntPtr(spec.Timeout)
	effectiveTimeout, resolutionContext := common.ResolveTimeout(
		commandTimeout,
		common.NewUnsetTimeout(), // Group timeout not yet supported
		globalTimeout,
		spec.Name,
		groupName,
	)

	// Resolve the effective output size limit
	commandOutputSizeLimit := common.NewOutputSizeLimitFromPtr(spec.OutputSizeLimit)
	effectiveOutputSizeLimit := common.ResolveOutputSizeLimit(
		commandOutputSizeLimit,
		globalOutputSizeLimit,
	)

	return &RuntimeCommand{
		Spec:                     spec,
		timeout:                  commandTimeout,
		ExpandedArgs:             []string{},
		ExpandedEnv:              make(map[string]string),
		ExpandedVars:             make(map[string]string),
		ExpandedArrayVars:        make(map[string][]string),
		EnvImportVars:            make(map[string]string),
		EffectiveTimeout:         effectiveTimeout,
		TimeoutResolution:        resolutionContext,
		EffectiveOutputSizeLimit: effectiveOutputSizeLimit,
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
// Returns empty string if OutputFile is nil.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) Output() string {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.OutputFile: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	if r.Spec.OutputFile == nil {
		return ""
	}
	return *r.Spec.OutputFile
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
// Use EffectiveTimeout for the fully resolved timeout value.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) Timeout() common.Timeout {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.Timeout: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.timeout
}

// GetRiskLevel parses and returns the maximum risk level for this command.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) GetRiskLevel() (RiskLevel, error) {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.GetRiskLevel: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.GetRiskLevel()
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) HasUserGroupSpecification() bool {
	if r == nil || r.Spec == nil {
		panic("RuntimeCommand.HasUserGroupSpecification: nil receiver or Spec (programming error - use NewRuntimeCommand)")
	}
	return r.Spec.HasUserGroupSpecification()
}
