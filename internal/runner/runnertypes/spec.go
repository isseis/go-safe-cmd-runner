// Package runnertypes defines the core data structures used throughout the command runner.
package runnertypes

// CommandTemplate represents a reusable command definition.
// Templates are defined in the [command_templates] section of TOML and
// can be referenced by CommandSpec using the Template field.
//
// Template parameters use the following syntax:
//   - ${param}   : Required string parameter
//   - ${?param}  : Optional string parameter (removed if empty)
//   - ${@param}  : Array parameter (elements are expanded in place)
//   - \$         : Literal $ character (in TOML: \\$)
//
// Example TOML:
//
//	[command_templates.restic_backup]
//	cmd = "restic"
//	args = ["${@verbose_flags}", "backup", "${path}"]
type CommandTemplate struct {
	// Cmd is the command path (may contain template parameters)
	// REQUIRED field
	Cmd string `toml:"cmd"`

	// Args is the list of command arguments (may contain template parameters)
	// Optional, defaults to empty array
	Args []string `toml:"args"`

	// Env is the list of environment variables in KEY=VALUE format
	// (may contain template parameters in the VALUE part)
	// Optional, defaults to empty array
	Env []string `toml:"env"`

	// WorkDir is the working directory for the command (optional)
	WorkDir string `toml:"workdir"`

	// Timeout specifies the command timeout in seconds (optional)
	// nil: inherit from group/global, 0: unlimited, positive: timeout in seconds
	Timeout *int32 `toml:"timeout"`

	// OutputSizeLimit specifies the maximum output size in bytes (optional)
	// nil: inherit from global, 0: unlimited, positive: limit in bytes
	OutputSizeLimit *int64 `toml:"output_size_limit"`

	// RiskLevel specifies the maximum allowed risk level (optional)
	// Valid values: "low", "medium", "high"
	RiskLevel string `toml:"risk_level"`

	// NOTE: The "name" field is NOT allowed in template definitions.
	// Command names must be specified in the [[groups.commands]] section
	// when referencing the template.
}

// ConfigSpec represents the root configuration structure loaded from TOML file.
// This is an immutable representation of the configuration file and should not be modified after loading.
//
// All fields in this struct correspond directly to TOML file structure.
// For runtime-expanded values, see RuntimeGlobal and RuntimeGroup instead.
type ConfigSpec struct {
	// Version specifies the configuration file version (e.g., "1.0")
	Version string `toml:"version"`

	// Global contains global-level configuration
	Global GlobalSpec `toml:"global"`

	// CommandTemplates contains reusable command template definitions.
	// Templates are defined using TOML table syntax:
	//   [command_templates.template_name]
	//   cmd = "..."
	//   args = [...]
	//
	// Template names must:
	//   - Start with a letter or underscore
	//   - Contain only letters, digits, and underscores
	//   - Not start with "__" (reserved for future use)
	CommandTemplates map[string]CommandTemplate `toml:"command_templates"`

	// Groups contains all command groups defined in the configuration
	Groups []GroupSpec `toml:"groups"`
}

// GlobalSpec contains global configuration options loaded from TOML file.
// This is an immutable representation and should not be modified after loading.
//
// For runtime-expanded values (e.g., ExpandedEnv, ExpandedVars), see RuntimeGlobal instead.
type GlobalSpec struct {
	// Execution control
	// Timeout specifies the default timeout for all commands
	// nil: use DefaultTimeout (60 seconds)
	// 0: unlimited execution (no timeout)
	// Positive value: timeout after N seconds
	Timeout             *int32 `toml:"timeout"`               // Global timeout in seconds (nil=default 60s, 0=unlimited)
	VerifyStandardPaths *bool  `toml:"verify_standard_paths"` // Verify files in standard system paths (nil=default true)
	OutputSizeLimit     *int64 `toml:"output_size_limit"`     // Maximum output size in bytes (nil=use default, 0=unlimited) - raw value for TOML unmarshaling

	// Security
	VerifyFiles []string `toml:"verify_files"` // Files to verify before execution (raw paths)
	EnvAllowed  []string `toml:"env_allowed"`  // Allowed environment variables

	// Variable definitions (raw values, not yet expanded)
	EnvVars   []string `toml:"env_vars"`   // Environment variables in KEY=VALUE format
	EnvImport []string `toml:"env_import"` // System env var imports in internal_name=SYSTEM_VAR format

	// Vars defines internal variables as a TOML table.
	// Each key-value pair defines a variable where:
	//   - Key: variable name (must pass ValidateVariableName)
	//   - Value: string or []string (other types are rejected)
	//
	// Example TOML:
	//   [global.vars]
	//   base_dir = "/opt/myapp"
	//   config_files = ["config.yml", "secrets.yml"]
	//
	// Changed from: Vars []string `toml:"vars"` (array-based format)
	Vars map[string]any `toml:"vars"`
}

// GroupSpec represents a command group configuration loaded from TOML file.
// This is an immutable representation and should not be modified after loading.
//
// For runtime-expanded values, see RuntimeGroup instead.
type GroupSpec struct {
	// Basic information
	Name        string `toml:"name"`        // Group name (must be unique within the config)
	Description string `toml:"description"` // Human-readable description

	// Resource management
	WorkDir string `toml:"workdir"` // Working directory for this group (raw value, not yet expanded)

	// Command definitions
	Commands []CommandSpec `toml:"commands"` // Commands in this group

	// Security
	VerifyFiles []string `toml:"verify_files"` // Files to verify for this group (raw paths)
	EnvAllowed  []string `toml:"env_allowed"`  // Group-level environment variable allowlist

	// CmdAllowed is the list of additional commands allowed to be executed in this group.
	// Each element is an absolute path (before variable expansion).
	//
	// Behavior when empty:
	//   - nil: Field was omitted (no group-level additional permissions)
	//   - []: Empty array was explicitly specified (same behavior as nil, no additional permissions)
	//
	// Example:
	//   cmd_allowed = ["/home/user/bin/tool1", "%{home}/bin/tool2"]
	CmdAllowed []string `toml:"cmd_allowed"`

	// Variable definitions (raw values, not yet expanded)
	EnvVars   []string `toml:"env_vars"`   // Group-level environment variables in KEY=VALUE format
	EnvImport []string `toml:"env_import"` // System env var imports in internal_name=SYSTEM_VAR format

	// Vars defines group-level internal variables as a TOML table.
	// See GlobalSpec.Vars for format details.
	//
	// Example TOML:
	//   [[groups]]
	//   name = "deploy"
	//   [groups.vars]
	//   deploy_target = "production"
	//
	// Changed from: Vars []string `toml:"vars"` (array-based format)
	Vars map[string]any `toml:"vars"`
}

// CommandSpec represents a single command configuration loaded from TOML file.
// This is an immutable representation and should not be modified after loading.
//
// For runtime-expanded values (e.g., ExpandedCmd, ExpandedArgs), see RuntimeCommand instead.
type CommandSpec struct {
	// Basic information
	Name        string `toml:"name"`        // Command name (REQUIRED, must be unique within group)
	Description string `toml:"description"` // Human-readable description

	// Template reference (mutually exclusive with Cmd, Args, Env, WorkDir)
	// When Template is set, the command definition is loaded from the
	// referenced CommandTemplate and Params are applied.
	Template string `toml:"template"`

	// Params contains template parameter values.
	// Each key corresponds to a parameter placeholder in the template.
	// Values can be:
	//   - string: for ${param} and ${?param} placeholders
	//   - []any: for ${@param} placeholders (elements must be string)
	//
	// Params can contain variable references (%{var}) which will be expanded
	// AFTER template expansion (see F-006 in requirements.md).
	//
	// Example TOML:
	//   [[groups.commands]]
	//   name = "backup_volumes"  # REQUIRED (must be unique within group)
	//   template = "restic_backup"
	//   params.verbose_flags = ["-q"]
	//   params.path = "%{backup_dir}/data"  # %{} is allowed in params
	Params map[string]any `toml:"params"`

	// Command definition (raw values, not yet expanded)
	// These fields are MUTUALLY EXCLUSIVE with Template:
	//   - If Template is set, these fields MUST NOT be set (validation error)
	//   - If Template is not set, Cmd is REQUIRED
	Cmd  string   `toml:"cmd"`  // Command path (may contain variables like %{VAR})
	Args []string `toml:"args"` // Command arguments (may contain variables)

	// Execution settings
	WorkDir string `toml:"workdir"` // Working directory for this command (raw value)
	// Timeout specifies command-specific timeout
	// nil: inherit from parent (group or global)
	// 0: unlimited execution (no timeout)
	// Positive value: timeout after N seconds
	Timeout         *int32 `toml:"timeout"`           // Command-specific timeout in seconds (nil=inherit, 0=unlimited)
	OutputSizeLimit *int64 `toml:"output_size_limit"` // Command-specific output size limit in bytes (nil=inherit from global, 0=unlimited) - raw value for TOML unmarshaling
	RunAsUser       string `toml:"run_as_user"`       // User to execute command as (using seteuid)
	RunAsGroup      string `toml:"run_as_group"`      // Group to execute command as (using setegid)
	RiskLevel       string `toml:"risk_level"`        // Maximum allowed risk level: low, medium, high
	OutputFile      string `toml:"output_file"`       // Standard output file path for capture

	// Variable definitions (raw values, not yet expanded)
	EnvVars   []string `toml:"env_vars"`   // Command-level environment variables in KEY=VALUE format
	EnvImport []string `toml:"env_import"` // System env var imports in internal_name=SYSTEM_VAR format

	// Vars defines command-level internal variables as a TOML table.
	// See GlobalSpec.Vars for format details.
	//
	// Example TOML:
	//   [[groups.commands]]
	//   name = "backup"
	//   [groups.commands.vars]
	//   backup_suffix = ".bak"
	//
	// Changed from: Vars []string `toml:"vars"` (array-based format)
	Vars map[string]any `toml:"vars"`
}

// GetRiskLevel parses and returns the maximum risk level for this command.
// Returns RiskLevelUnknown and an error if the risk level string is invalid.
//
// Critical risk level cannot be set in configuration (reserved for internal use only).
func (s *CommandSpec) GetRiskLevel() (RiskLevel, error) {
	return ParseRiskLevel(s.RiskLevel)
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified.
func (s *CommandSpec) HasUserGroupSpecification() bool {
	return s.RunAsUser != "" || s.RunAsGroup != ""
}
